//go:build integration

package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestSaveDraft_Success_Unmodified(t *testing.T) {
	pool := getTestPool(t)
	qid, _, fx := generateOnce(t, pool)
	snap := captureSnapshot(t, pool, qid)
	require.Equal(t, 1, snap.version)
	require.NotEmpty(t, snap.html)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	userID := uuid.New()

	res, err := svc.SaveDraft(context.Background(), fx.companyID, qid, &userID, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
		ContentVersion: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, qid, res.QuotationID)
	assert.Equal(t, 2, res.ContentVersion)
	assert.False(t, res.LegalContentModified)
	assert.False(t, res.CommercialContentModified)
	assert.False(t, res.UpdatedAt.IsZero())

	// Verify DB state
	snapAfter := captureSnapshot(t, pool, qid)
	assert.Equal(t, 2, snapAfter.version)
}

func TestSaveDraft_LegalModified_SetsFlag(t *testing.T) {
	pool := getTestPool(t)
	qid, _, fx := generateOnce(t, pool)
	snap := captureSnapshot(t, pool, qid)

	// Modify the legal block inside content_html
	modifiedHTML := strings.Replace(
		snap.html,
		"Syarat dan ketentuan penawaran resmi berlaku 30 hari.",
		"Syarat dan ketentuan khusus telah dinegosiasikan 90 hari.",
		1,
	)
	require.NotEqual(t, snap.html, modifiedHTML)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	userID := uuid.New()

	res, err := svc.SaveDraft(context.Background(), fx.companyID, qid, &userID, dto.SaveDraftRequest{
		ContentHTML:    modifiedHTML,
		ContentVersion: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.ContentVersion)
	assert.True(t, res.LegalContentModified)
	assert.False(t, res.CommercialContentModified)

	// Verify DB audit fields
	var legalMod, commMod bool
	var legalBy *uuid.UUID
	err = pool.QueryRow(context.Background(), `
		SELECT legal_content_modified, commercial_content_modified, legal_modified_by
		FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&legalMod, &commMod, &legalBy)
	require.NoError(t, err)
	assert.True(t, legalMod)
	assert.False(t, commMod)
	require.NotNil(t, legalBy)
	assert.Equal(t, userID, *legalBy)
}

func TestSaveDraft_CommercialModified_SetsFlag(t *testing.T) {
	pool := getTestPool(t)
	qid, _, fx := generateOnce(t, pool)
	snap := captureSnapshot(t, pool, qid)

	// Modify total investment inside content_html
	modifiedHTML := strings.Replace(
		snap.html,
		"63.000.000",
		"75.000.000",
		1,
	)
	require.NotEqual(t, snap.html, modifiedHTML)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	userID := uuid.New()

	res, err := svc.SaveDraft(context.Background(), fx.companyID, qid, &userID, dto.SaveDraftRequest{
		ContentHTML:    modifiedHTML,
		ContentVersion: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.ContentVersion)
	assert.False(t, res.LegalContentModified)
	assert.True(t, res.CommercialContentModified)
}

func TestSaveDraft_PaymentModified_SetsCommercialFlag(t *testing.T) {
	pool := getTestPool(t)
	qid, _, fx := generateOnce(t, pool)
	snap := captureSnapshot(t, pool, qid)
	modifiedHTML := strings.Replace(snap.html, `data-field="payment_percentage">50%`, `data-field="payment_percentage">60%`, 1)
	require.NotEqual(t, snap.html, modifiedHTML)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	res, err := svc.SaveDraft(context.Background(), fx.companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML: modifiedHTML, ContentVersion: 1,
	})
	require.NoError(t, err)
	assert.True(t, res.CommercialContentModified)
}

func TestSaveDraft_MalformedSnapshots_RejectsWithoutUpdating(t *testing.T) {
	for _, column := range []string{"allocation_snapshot", "legal_template_snapshot"} {
		t.Run(column, func(t *testing.T) {
			pool := getTestPool(t)
			qid, _, fx := generateOnce(t, pool)
			before := captureSnapshot(t, pool, qid)
			_, err := pool.Exec(context.Background(), fmt.Sprintf(
				`UPDATE quotation_revisions SET %s = 'null'::jsonb WHERE quotation_id = $1`, column,
			), qid)
			require.NoError(t, err)

			svc, _ := newSvc(pool, &scriptedAIProvider{})
			_, err = svc.SaveDraft(context.Background(), fx.companyID, qid, nil, dto.SaveDraftRequest{
				ContentHTML: before.html, ContentVersion: before.version,
			})
			require.Error(t, err)
			assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

			after := captureSnapshot(t, pool, qid)
			assert.Equal(t, before.version, after.version)
			assert.Equal(t, before.html, after.html)
		})
	}
}

func TestSaveDraft_StructurallyInvalidSnapshots_RejectsWithoutUpdating(t *testing.T) {
	tests := []struct {
		name   string
		column string
		value  string
	}{
		{
			name: "allocation resource is incomplete", column: "allocation_snapshot",
			value: `{"total_investment":63000000,"currency":"IDR","resources":[{"role":"","seniority":"","employee_count":0,"period_months":0,"amount_per_month":0,"line_total":0}]}`,
		},
		{
			name: "payment percentage is invalid", column: "generation_input",
			value: `{"payment_terms":{"mode":"milestone","items":[{"percentage":0}]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := getTestPool(t)
			qid, _, fx := generateOnce(t, pool)
			before := captureSnapshot(t, pool, qid)
			_, err := pool.Exec(context.Background(), fmt.Sprintf(
				`UPDATE quotation_revisions SET %s = $1::jsonb WHERE quotation_id = $2`, tt.column,
			), tt.value, qid)
			require.NoError(t, err)

			svc, _ := newSvc(pool, &scriptedAIProvider{})
			_, err = svc.SaveDraft(context.Background(), fx.companyID, qid, nil, dto.SaveDraftRequest{
				ContentHTML: before.html, ContentVersion: before.version,
			})
			require.Error(t, err)
			assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

			after := captureSnapshot(t, pool, qid)
			assert.Equal(t, before.version, after.version)
			assert.Equal(t, before.html, after.html)
		})
	}
}

func TestSaveDraft_StaleVersion_FailsWithConflict(t *testing.T) {
	pool := getTestPool(t)
	qid, _, fx := generateOnce(t, pool)
	snap := captureSnapshot(t, pool, qid)

	svc, _ := newSvc(pool, &scriptedAIProvider{})

	// Pass stale version 99 (current is 1)
	_, err := svc.SaveDraft(context.Background(), fx.companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
		ContentVersion: 99,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, service.ErrCodeRevisionConflict)

	// Save once to bump version to 2
	res, err := svc.SaveDraft(context.Background(), fx.companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
		ContentVersion: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.ContentVersion)

	// Attempting to save again with version 1 must now fail
	_, err = svc.SaveDraft(context.Background(), fx.companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
		ContentVersion: 1,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, service.ErrCodeRevisionConflict)
}

func TestSaveDraft_Sanitization_StripsMaliciousScript(t *testing.T) {
	pool := getTestPool(t)
	qid, _, fx := generateOnce(t, pool)
	snap := captureSnapshot(t, pool, qid)

	maliciousHTML := fmt.Sprintf("<script>alert('pwned')</script>%s", snap.html)
	svc, _ := newSvc(pool, &scriptedAIProvider{})

	res, err := svc.SaveDraft(context.Background(), fx.companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    maliciousHTML,
		ContentVersion: 1,
	})
	require.NoError(t, err)
	assert.NotContains(t, res.ContentHTML, "<script>")
	assert.NotContains(t, res.ContentHTML, "alert('pwned')")
}

func TestSaveDraft_ForbiddenTerms_RejectsWithUnsafeHTML(t *testing.T) {
	pool := getTestPool(t)
	qid, _, fx := generateOnce(t, pool)
	snap := captureSnapshot(t, pool, qid)

	withForbidden := fmt.Sprintf("%s<p>Internal note: budget utilization is 90%%</p>", snap.html)
	svc, _ := newSvc(pool, &scriptedAIProvider{})

	_, err := svc.SaveDraft(context.Background(), fx.companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    withForbidden,
		ContentVersion: 1,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)
}

func TestSaveDraft_BudgetLeak_RejectsWithUnsafeHTML(t *testing.T) {
	pool := getTestPool(t)
	qid, _, fx := generateOnce(t, pool)
	snap := captureSnapshot(t, pool, qid)

	// fixBudget is 100_000_000 in seedPipelineFixtures while totalInvestment is 63_000_000
	withBudgetLeak := fmt.Sprintf("%s<p>Target budget: 100.000.000</p>", snap.html)
	svc, _ := newSvc(pool, &scriptedAIProvider{})

	_, err := svc.SaveDraft(context.Background(), fx.companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    withBudgetLeak,
		ContentVersion: 1,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)
}

// ---------------------------------------------------------------------------
// White Label Save Draft Integration Tests
// ---------------------------------------------------------------------------

func TestSaveDraft_WhiteLabel_Success_UnmodifiedNarrative(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateWhiteLabelDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)
	require.Equal(t, 1, snap.version)
	require.NotEmpty(t, snap.html)

	// Modify narrative in opening paragraph without touching commercial, rights, or legal
	modifiedHTML := strings.Replace(
		snap.html,
		"Melalui penawaran ini, Venturo menyampaikan",
		"Melalui surat penawaran resmi ini, Venturo menyampaikan",
		1,
	)
	require.NotEqual(t, snap.html, modifiedHTML)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	userID := uuid.New()

	res, err := svc.SaveDraft(context.Background(), companyID, qid, &userID, dto.SaveDraftRequest{
		ContentHTML:    modifiedHTML,
		ContentVersion: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, qid, res.QuotationID)
	assert.Equal(t, 2, res.ContentVersion)
	assert.False(t, res.LegalContentModified)
	assert.False(t, res.CommercialContentModified)
	assert.False(t, res.UpdatedAt.IsZero())

	// Verify in DB
	snapAfter := captureSnapshot(t, pool, qid)
	assert.Equal(t, 2, snapAfter.version)
}

func TestSaveDraft_WhiteLabel_CommercialModified(t *testing.T) {
	pool := getTestPool(t)

	t.Run("LicenseRateModified", func(t *testing.T) {
		qid, companyID, _ := generateWhiteLabelDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(snap.html, "Rp 35.000.000", "Rp 40.000.000", 1)
		require.NotEqual(t, snap.html, modifiedHTML)

		svc, _ := newSvc(pool, &scriptedAIProvider{})
		userID := uuid.New()

		res, err := svc.SaveDraft(context.Background(), companyID, qid, &userID, dto.SaveDraftRequest{
			ContentHTML:    modifiedHTML,
			ContentVersion: 1,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, res.ContentVersion)
		assert.True(t, res.CommercialContentModified)
		assert.False(t, res.LegalContentModified)
	})

	t.Run("MaintenanceRateModified", func(t *testing.T) {
		qid, companyID, _ := generateWhiteLabelDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(snap.html, "Rp 16.170.000", "Rp 20.000.000", 1)
		require.NotEqual(t, snap.html, modifiedHTML)

		svc, _ := newSvc(pool, &scriptedAIProvider{})
		userID := uuid.New()

		res, err := svc.SaveDraft(context.Background(), companyID, qid, &userID, dto.SaveDraftRequest{
			ContentHTML:    modifiedHTML,
			ContentVersion: 1,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, res.ContentVersion)
		assert.True(t, res.CommercialContentModified)
		assert.False(t, res.LegalContentModified)
	})

	t.Run("HostingRateModified", func(t *testing.T) {
		qid, companyID, _ := generateWhiteLabelDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(snap.html, "Rp 350.000", "Rp 750.000", 1)
		require.NotEqual(t, snap.html, modifiedHTML)

		svc, _ := newSvc(pool, &scriptedAIProvider{})
		userID := uuid.New()

		res, err := svc.SaveDraft(context.Background(), companyID, qid, &userID, dto.SaveDraftRequest{
			ContentHTML:    modifiedHTML,
			ContentVersion: 1,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, res.ContentVersion)
		assert.True(t, res.CommercialContentModified)
		assert.False(t, res.LegalContentModified)
	})

	t.Run("PaymentTotalModified", func(t *testing.T) {
		qid, companyID, _ := generateWhiteLabelDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			`<td data-field="payment_total"><strong>Rp 60.305.000</strong></td>`,
			`<td data-field="payment_total"><strong>Rp 80.000.000</strong></td>`,
			1,
		)
		require.NotEqual(t, snap.html, modifiedHTML)

		svc, _ := newSvc(pool, &scriptedAIProvider{})
		userID := uuid.New()

		res, err := svc.SaveDraft(context.Background(), companyID, qid, &userID, dto.SaveDraftRequest{
			ContentHTML:    modifiedHTML,
			ContentVersion: 1,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, res.ContentVersion)
		assert.True(t, res.CommercialContentModified)
		assert.False(t, res.LegalContentModified)
	})
}

func TestSaveDraft_WhiteLabel_RightsModified_SetsLegalFlag(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateWhiteLabelDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	modifiedHTML := strings.Replace(
		snap.html,
		"Klien berhak menggunakan source code untuk kebutuhan internal.",
		"Klien berhak menjual kembali source code ke pihak ketiga secara bebas.",
		1,
	)
	require.NotEqual(t, snap.html, modifiedHTML)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	userID := uuid.New()

	res, err := svc.SaveDraft(context.Background(), companyID, qid, &userID, dto.SaveDraftRequest{
		ContentHTML:    modifiedHTML,
		ContentVersion: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.ContentVersion)
	assert.True(t, res.LegalContentModified)
	assert.False(t, res.CommercialContentModified)
}

func TestSaveDraft_WhiteLabel_MalformedSnapshots_RejectsWithoutUpdating(t *testing.T) {
	for _, column := range []string{"calculation_snapshot", "generation_input"} {
		t.Run(column, func(t *testing.T) {
			pool := getTestPool(t)
			qid, companyID, _ := generateWhiteLabelDraft(t, pool)
			before := captureSnapshot(t, pool, qid)

			_, err := pool.Exec(context.Background(), fmt.Sprintf(
				`UPDATE quotation_revisions SET %s = 'null'::jsonb WHERE quotation_id = $1`, column,
			), qid)
			require.NoError(t, err)

			svc, _ := newSvc(pool, &scriptedAIProvider{})
			_, err = svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
				ContentHTML:    before.html,
				ContentVersion: before.version,
			})
			require.Error(t, err)
			assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

			after := captureSnapshot(t, pool, qid)
			assert.Equal(t, before.version, after.version)
			assert.Equal(t, before.html, after.html)
		})
	}
}

func TestSaveDraft_WhiteLabel_Sanitization_StripsMaliciousScript(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateWhiteLabelDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	maliciousHTML := fmt.Sprintf("<script>alert('pwned')</script>%s", snap.html)
	svc, _ := newSvc(pool, &scriptedAIProvider{})

	res, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    maliciousHTML,
		ContentVersion: 1,
	})
	require.NoError(t, err)
	assert.NotContains(t, res.ContentHTML, "<script>")
	assert.NotContains(t, res.ContentHTML, "alert('pwned')")
}

func TestSaveDraft_WhiteLabel_StaleVersion_FailsWithConflict(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateWhiteLabelDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	svc, _ := newSvc(pool, &scriptedAIProvider{})

	// Pass stale version 99 (current is 1)
	_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
		ContentVersion: 99,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, service.ErrCodeRevisionConflict)

	// Save once to bump version to 2
	res, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
		ContentVersion: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.ContentVersion)

	// Attempting to save again with version 1 must now fail
	_, err = svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
		ContentVersion: 1,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, service.ErrCodeRevisionConflict)
}

