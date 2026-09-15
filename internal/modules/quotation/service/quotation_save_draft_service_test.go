//go:build integration

package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

// ---------------------------------------------------------------------------
// Cloud Infrastructure Save Draft Integration Tests
// ---------------------------------------------------------------------------

func cloudDraftInput() domain.CloudInfrastructureInput {
	return domain.CloudInfrastructureInput{
		DocumentNumber: "CLOUD-DRAFT-" + uuid.NewString(), ServiceName: "Venturo Cloud",
		QuotationDate: time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC), TaxTreatment: domain.CloudTaxExclusive,
		PackageItems:    []domain.CloudPackageItemInput{{Category: domain.CloudPackageVPS, Name: "Cloud VPS Small", VCPU: 4, RAMGB: 4, StorageGB: 32, UnitRate: 900000}},
		SetupItems:      []domain.CloudSetupItemInput{{Name: "Provisioning", Amount: 150000}},
		ScopeRows:       []domain.CloudScopeRowInput{{Sections: []domain.CloudScopeSectionInput{{Title: "Performance Test", Activities: []string{"Run load test"}}}, Timeline: "2 Minggu"}},
		PaymentSchedule: []domain.CloudPaymentRowInput{{WorkPeriod: "Setup", Terms: "Di muka", InvoiceDateLabel: "24 Jun 2026", DueDateLabel: "8 Jul 2026", Component: domain.CloudPaymentSetup, Nominal: 150000}, {WorkPeriod: "Bulan 1", Terms: "Bulanan", InvoiceDateLabel: "30 Jun 2026", DueDateLabel: "14 Jul 2026", Component: domain.CloudPaymentMonthly, Nominal: 900000}},
		PaymentDueDays:  14, LimitationCodes: []domain.CloudLimitationCode{domain.CloudLimitOutOfPocket},
	}
}

func generateCloudInfrastructureDraft(t *testing.T, pool *pgxpool.Pool) (uuid.UUID, uuid.UUID) {
	t.Helper()
	fx := seedPipelineFixtures(t, pool)
	input := cloudDraftInput()
	req := fx.request()
	req.CommercialScheme = domain.CommercialSchemeCloudInfrastructure
	req.SolutionType = "cloud_infrastructure"
	req.RateCardID = uuid.Nil
	req.DedicatedSquad, req.SubscriptionSaaS, req.WhiteLabel, req.StandaloneMaintenance, req.CustomDevelopment, req.KioskQueueSystem, req.TalentOutsourcing = nil, nil, nil, nil, nil, nil, nil
	req.BudgetMaximum, req.ProjectDurationMonths = 0, 0
	req.MandatoryScope, req.OptionalScope, req.AllowedStandardActivities = nil, nil, nil
	req.PaymentTerms = dto.PaymentTermsInput{}
	req.CloudInfrastructure = &input
	svc, _ := newSvc(pool, &panicAIProvider{})
	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)
	return qid, fx.companyID
}

func TestSaveDraftCloudInfrastructureClassifiesNarrativeCommercialAndLegalEdits(t *testing.T) {
	tests := []struct {
		name              string
		old               string
		new               string
		legal, commercial bool
	}{
		{"narrative", "Run load test", "Run endurance test", false, false},
		{"package", "Cloud VPS Small", "Cloud VPS Medium", false, true},
		{"monthly subtotal", "Rp 900.000", "Rp 950.000", false, true},
		{"setup fee", "Rp 150.000", "Rp 175.000", false, true},
		{"payment terms", "Bulanan", "Termin bulanan", false, true},
		{"limitations", "Biaya di luar penawaran", "Biaya tambahan dinegosiasikan", true, false},
		{"tax clause", "belum termasuk PPN", "sudah termasuk PPN", true, false},
		{"legal template", "Syarat dan ketentuan penawaran resmi berlaku 30 hari.", "Syarat berlaku 60 hari.", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := getTestPool(t)
			qid, companyID := generateCloudInfrastructureDraft(t, pool)
			snap := captureSnapshot(t, pool, qid)
			modified := strings.Replace(snap.html, tt.old, tt.new, 1)
			require.NotEqual(t, snap.html, modified)
			svc, _ := newSvc(pool, &scriptedAIProvider{})
			res, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{ContentHTML: modified, ContentVersion: snap.version})
			require.NoError(t, err)
			assert.Equal(t, tt.legal, res.LegalContentModified)
			assert.Equal(t, tt.commercial, res.CommercialContentModified)
		})
	}
}

func TestSaveDraftCloudInfrastructureRejectsRemovedRequiredSection(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID := generateCloudInfrastructureDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)
	modified := strings.Replace(snap.html, `<section data-block-type="commercial" data-section-code="payment"`, `<section data-block-type="commercial" data-section-code="removed"`, 1)
	svc, _ := newSvc(pool, &scriptedAIProvider{})
	_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{ContentHTML: modified, ContentVersion: snap.version})
	require.Error(t, err)
	assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)
}

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
		"Melalui penawaran ini, kami menegaskan",
		"Melalui surat penawaran resmi ini, kami menegaskan",
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

// ---------------------------------------------------------------------------
// Standalone Maintenance Save Draft Integration Tests
// ---------------------------------------------------------------------------

func TestSaveDraft_StandaloneMaintenance_Success_Unmodified(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)
	require.Equal(t, 1, snap.version)
	require.NotEmpty(t, snap.html)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	userID := uuid.New()

	res, err := svc.SaveDraft(context.Background(), companyID, qid, &userID, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
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

	var legalMod, commMod bool
	err = pool.QueryRow(context.Background(), `
		SELECT legal_content_modified, commercial_content_modified
		FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&legalMod, &commMod)
	require.NoError(t, err)
	assert.False(t, legalMod)
	assert.False(t, commMod)
}

func TestSaveDraft_StandaloneMaintenance_ScopeEdits_AllowedWithoutFlag(t *testing.T) {
	pool := getTestPool(t)

	t.Run("RetainerDefectScopeEdits", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		// Edit function, screen flow, existing, and improvement in defect table
		modifiedHTML := strings.Replace(
			snap.html,
			"Melihat riwayat medis pasien di luar sesi konsultasi.",
			"Melihat riwayat medis pasien terintegrasi antar departemen rumah sakit.",
			1,
		)
		modifiedHTML = strings.Replace(
			modifiedHTML,
			"Pasien &gt; Data Pasien dan Riwayat &gt; Lihat Detail Pasien &gt; Medical History.",
			"Pasien &gt; Rekam Medis &gt; Riwayat Terpadu.",
			1,
		)
		modifiedHTML = strings.Replace(
			modifiedHTML,
			"Hanya mencantumkan catatan registrasi awal.",
			"Hanya menampilkan ringkasan registrasi kunjungan pertama.",
			1,
		)
		modifiedHTML = strings.Replace(
			modifiedHTML,
			"Mencantumkan seluruh riwayat SOAP (Dokter, Terapis, dan Perawat).",
			"Mencantumkan seluruh riwayat SOAP lengkap beserta catatan tindakan medis.",
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
		assert.False(t, res.CommercialContentModified)
		assert.False(t, res.LegalContentModified)
	})

	t.Run("HourlyTaskScopeEdits", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool, func(input *domain.StandaloneMaintenanceInput) {
			*input = liputan6HourlyInputForCalculator()
		})
		snap := captureSnapshot(t, pool, qid)

		// Edit hourly task description
		modifiedHTML := strings.Replace(
			snap.html,
			"Perbaikan crash login",
			"Perbaikan crash login saat SSO OAuth2 Google dan Apple ID",
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
		assert.False(t, res.CommercialContentModified)
		assert.False(t, res.LegalContentModified)
	})
}

func TestSaveDraft_StandaloneMaintenance_CommercialModified(t *testing.T) {
	pool := getTestPool(t)

	t.Run("RetainerRateModified", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(snap.html, "Rp 16.050.000", "Rp 18.000.000", 1)
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

	t.Run("RetainerFTEModified", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(snap.html, "<td>Senior Web Programmer</td><td>Rp 16.050.000</td><td>1</td>", "<td>Senior Web Programmer</td><td>Rp 16.050.000</td><td>2</td>", 1)
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

	t.Run("RetainerPeriodMonthsModified", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(snap.html, "<td>1 Bulan</td>", "<td>3 Bulan</td>", 1)
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

	t.Run("RetainerLineTotalModified", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			`<td data-field="cost_line_total">Rp 16.050.000</td>`,
			`<td data-field="cost_line_total">Rp 20.000.000</td>`,
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

	t.Run("TotalInvestmentModified", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			`<td data-field="total_investment"><strong>Rp 23.085.000</strong></td>`,
			`<td data-field="total_investment"><strong>Rp 30.000.000</strong></td>`,
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

	t.Run("PaymentNominalModified", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			`<td data-field="payment_nominal">Rp 23.085.000</td>`,
			`<td data-field="payment_nominal">Rp 25.000.000</td>`,
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

	t.Run("PaymentTotalModified", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			`<td data-field="payment_total"><strong>Rp 23.085.000</strong></td>`,
			`<td data-field="payment_total"><strong>Rp 25.000.000</strong></td>`,
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

	t.Run("HourlyRateAndHoursModified", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool, func(input *domain.StandaloneMaintenanceInput) {
			*input = liputan6HourlyInputForCalculator()
		})
		snap := captureSnapshot(t, pool, qid)

		// Modify hourly display rate
		modifiedHTML := strings.Replace(snap.html, "Rp 16.050.000", "Rp 18.000.000", 1)
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

	t.Run("HourlySubtotalAndDiscountModified", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool, func(input *domain.StandaloneMaintenanceInput) {
			*input = liputan6HourlyInputForCalculator()
			input.ShowDiscountRow = true
			input.DiscountAmount = 70_000
			input.PaymentSchedule[0].Nominal = 800_000
		})
		snap := captureSnapshot(t, pool, qid)

		// Modify discount amount in commercial table
		modifiedHTML := strings.Replace(
			snap.html,
			`<td data-field="discount_amount"><strong>Rp 70.000</strong></td>`,
			`<td data-field="discount_amount"><strong>Rp 50.000</strong></td>`,
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

func TestSaveDraft_StandaloneMaintenance_LegalModified(t *testing.T) {
	pool := getTestPool(t)

	t.Run("RightsModified_SetsLegalFlag", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool, func(input *domain.StandaloneMaintenanceInput) {
			input.RightsStatements = []string{"Klien berhak atas akses konfigurasi maintenance."}
		})
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			"Klien berhak atas akses konfigurasi maintenance.",
			"Klien memiliki kepemilikan penuh atas seluruh source code dan hak cipta tanpa batas.",
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
	})

	t.Run("LegalTemplateBlockModified_SetsLegalFlag", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			"Syarat dan ketentuan penawaran resmi berlaku 30 hari.",
			"Syarat dan ketentuan khusus telah dinegosiasikan 90 hari.",
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
	})
}

func TestSaveDraft_StandaloneMaintenance_UnsafeDOM_Rejection(t *testing.T) {
	pool := getTestPool(t)

	t.Run("DuplicateCommercialBlock", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		before := captureSnapshot(t, pool, qid)

		duplicated := before.html + `<section data-block-type="commercial"><table><tr><td>Duplicate</td></tr></table></section>`
		svc, _ := newSvc(pool, &scriptedAIProvider{})

		_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
			ContentHTML:    duplicated,
			ContentVersion: before.version,
		})
		require.Error(t, err)
		assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

		after := captureSnapshot(t, pool, qid)
		assert.Equal(t, before.version, after.version)
	})

	t.Run("MissingPaymentBlock", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		before := captureSnapshot(t, pool, qid)

		// Remove payment block
		start := strings.Index(before.html, `<section data-block-type="payment"`)
		require.True(t, start > 0)
		end := strings.Index(before.html[start:], `</section>`)
		require.True(t, end > 0)
		withoutPayment := before.html[:start] + before.html[start+end+len("</section>"):]

		svc, _ := newSvc(pool, &scriptedAIProvider{})

		_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
			ContentHTML:    withoutPayment,
			ContentVersion: before.version,
		})
		require.Error(t, err)
		assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

		after := captureSnapshot(t, pool, qid)
		assert.Equal(t, before.version, after.version)
	})

	t.Run("ScriptOnlyHTML", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		before := captureSnapshot(t, pool, qid)

		svc, _ := newSvc(pool, &scriptedAIProvider{})

		_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
			ContentHTML:    "<script>alert('pwned')</script>",
			ContentVersion: before.version,
		})
		require.Error(t, err)
		assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

		after := captureSnapshot(t, pool, qid)
		assert.Equal(t, before.version, after.version)
	})

	t.Run("IframeOnlyHTML", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		before := captureSnapshot(t, pool, qid)

		svc, _ := newSvc(pool, &scriptedAIProvider{})

		_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
			ContentHTML:    `<iframe src="https://evil.example.com"></iframe>`,
			ContentVersion: before.version,
		})
		require.Error(t, err)
		assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

		after := captureSnapshot(t, pool, qid)
		assert.Equal(t, before.version, after.version)
	})

	t.Run("SanitizationStripsMaliciousScript", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
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
	})
}

func TestSaveDraft_StandaloneMaintenance_SnapshotCorruption_Rejection(t *testing.T) {
	pool := getTestPool(t)

	cases := []struct {
		name   string
		column string
		value  string
	}{
		{name: "calculation_snapshot_null", column: "calculation_snapshot", value: "null"},
		{name: "calculation_snapshot_zero_investment", column: "calculation_snapshot", value: `{"mode":"retainer","total_investment":0}`},
		{name: "calculation_snapshot_negative_investment", column: "calculation_snapshot", value: `{"mode":"retainer","total_investment":-100}`},
		{name: "calculation_snapshot_corrupted_structure", column: "calculation_snapshot", value: `{"invalid_structure":true}`},
		{name: "generation_input_null", column: "generation_input", value: "null"},
		{name: "generation_input_nil_maintenance", column: "generation_input", value: `{"standalone_maintenance":null}`},
		{name: "generation_input_invalid_maintenance", column: "generation_input", value: `{"standalone_maintenance":{"mode":"retainer","system_name":""}}`},
		{name: "legal_template_snapshot_null", column: "legal_template_snapshot", value: "null"},
		{name: "legal_template_snapshot_incomplete", column: "legal_template_snapshot", value: `{"code":"","content_html":""}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
			before := captureSnapshot(t, pool, qid)

			// Tamper snapshot in DB
			_, err := pool.Exec(context.Background(), fmt.Sprintf(
				`UPDATE quotation_revisions SET %s = $1::jsonb WHERE quotation_id = $2`, tc.column,
			), tc.value, qid)
			require.NoError(t, err)

			svc, _ := newSvc(pool, &scriptedAIProvider{})
			_, err = svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
				ContentHTML:    before.html,
				ContentVersion: before.version,
			})
			require.Error(t, err)
			assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

			// Assert no JSON or internal SQL error leaking
			errMsg := err.Error()
			assert.NotContains(t, errMsg, "{")
			assert.NotContains(t, errMsg, "}")
			assert.NotContains(t, errMsg, "SELECT")
			assert.NotContains(t, errMsg, "UPDATE")
			assert.NotContains(t, errMsg, "pq:")
			assert.NotContains(t, errMsg, "pgx:")

			after := captureSnapshot(t, pool, qid)
			assert.Equal(t, before.version, after.version)
			assert.Equal(t, before.html, after.html)
		})
	}
}

// ---------------------------------------------------------------------------
// Custom Development Save Draft Integration Tests
// ---------------------------------------------------------------------------

func TestSaveDraft_CustomDevelopment_Success_Unmodified(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)
	require.Equal(t, 1, snap.version)
	require.NotEmpty(t, snap.html)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	userID := uuid.New()

	res, err := svc.SaveDraft(context.Background(), companyID, qid, &userID, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
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

	var legalMod, commMod bool
	err = pool.QueryRow(context.Background(), `
		SELECT legal_content_modified, commercial_content_modified
		FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&legalMod, &commMod)
	require.NoError(t, err)
	assert.False(t, legalMod)
	assert.False(t, commMod)
}

func TestSaveDraft_CustomDevelopment_NarrativeEdits_AllowedWithoutFlag(t *testing.T) {
	pool := getTestPool(t)

	t.Run("ScopeNarrativeEdit", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		// Narrative edit in scope module features
		modifiedHTML := strings.Replace(
			snap.html,
			"<li>Checkout</li>",
			"<li>Checkout dan Integrasi Payment Gateway Midtrans</li>",
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
		assert.False(t, res.CommercialContentModified)
		assert.False(t, res.LegalContentModified)
	})

	t.Run("TimelineNarrativeEdit", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		// Narrative edit in timeline description / timeline text
		modifiedHTML := strings.Replace(
			snap.html,
			"<td>Fase 1</td><td>1 Bulan</td>",
			"<td>Fase 1: Implementasi Modul Transaksi Utama</td><td>1.5 Bulan (3 Sprint)</td>",
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
		assert.False(t, res.CommercialContentModified)
		assert.False(t, res.LegalContentModified)
	})
}

func TestSaveDraft_CustomDevelopment_CommercialModified(t *testing.T) {
	pool := getTestPool(t)

	t.Run("DevelopmentLineTotalModified", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			`data-quotation-field="development_line_0_0">Rp 15.000.000`,
			`data-quotation-field="development_line_0_0">Rp 16.000.000`,
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

	t.Run("DevelopmentSubtotalModified", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			`data-quotation-field="development_subtotal"><strong>Rp 15.000.000</strong>`,
			`data-quotation-field="development_subtotal"><strong>Rp 18.000.000</strong>`,
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

	t.Run("TotalInvestmentModified", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			`data-quotation-field="total_investment"><strong>Rp 15.000.000</strong>`,
			`data-quotation-field="total_investment"><strong>Rp 20.000.000</strong>`,
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

	t.Run("MonthlyAmountModified", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool, func(input *domain.CustomDevelopmentInput) {
			input.MonthlyItems = []domain.CustomDevelopmentMonthlyItemInput{
				{Name: "Cloud Hosting", Specification: "4 vCPU / 8GB RAM", UnitRate: 1_000_000, Quantity: 1},
			}
			input.PaymentSchedule = append(input.PaymentSchedule, domain.CustomDevelopmentPaymentRowInput{
				WorkPeriod:       "Bulan 1",
				Terms:            "Cloud Hosting & Support",
				InvoiceDateLabel: "Awal bulan berjalan",
				DueDateLabel:     "H+7",
				Component:        domain.CustomDevelopmentPaymentMonthly,
				Nominal:          1_000_000,
			})
		})
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			`data-quotation-field="monthly_line_0">Rp 1.000.000`,
			`data-quotation-field="monthly_line_0">Rp 1.500.000`,
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

	t.Run("PaymentNominalModified", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			`data-quotation-field="payment_nominal_0">Rp 15.000.000`,
			`data-quotation-field="payment_nominal_0">Rp 12.000.000`,
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

func TestSaveDraft_CustomDevelopment_LegalModified(t *testing.T) {
	pool := getTestPool(t)

	t.Run("RightsModified_SetsLegalFlag", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool, func(input *domain.CustomDevelopmentInput) {
			input.RightsStatements = []string{"Hak cipta dan source code sistem diserahkan penuh kepada klien setelah pelunasan."}
		})
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			"Hak cipta dan source code sistem diserahkan penuh kepada klien setelah pelunasan.",
			"Hak cipta dan kepemilikan source code tetap milik Venturo selamanya.",
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
	})

	t.Run("LegalTemplateBlockModified_SetsLegalFlag", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			"Syarat dan ketentuan penawaran resmi berlaku 30 hari.",
			"Syarat dan ketentuan khusus telah dinegosiasikan 90 hari.",
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
	})

	t.Run("AdditionalLimitationModified_SetsLegalFlag", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool, func(input *domain.CustomDevelopmentInput) {
			input.AdditionalLimitations = []string{"Koneksi internet stabil dan perangkat kasir disediakan oleh klien."}
		})
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			"Koneksi internet stabil dan perangkat kasir disediakan oleh klien.",
			"Koneksi internet dan perangkat kasir disediakan secara gratis oleh pihak vendor.",
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
	})
}

func TestSaveDraft_CustomDevelopment_CommercialAndRightsModified(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateCustomDevelopmentDraft(t, pool, func(input *domain.CustomDevelopmentInput) {
		input.RightsStatements = []string{"Hak cipta dan source code sistem diserahkan penuh kepada klien setelah pelunasan."}
	})
	snap := captureSnapshot(t, pool, qid)

	// Tamper both commercial total AND rights statement
	modifiedHTML := strings.Replace(
		snap.html,
		`data-quotation-field="total_investment"><strong>Rp 15.000.000</strong>`,
		`data-quotation-field="total_investment"><strong>Rp 25.000.000</strong>`,
		1,
	)
	modifiedHTML = strings.Replace(
		modifiedHTML,
		"Hak cipta dan source code sistem diserahkan penuh kepada klien setelah pelunasan.",
		"Hak cipta dan kepemilikan source code tetap milik Venturo selamanya.",
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
	assert.True(t, res.LegalContentModified)
}

func TestSaveDraft_CustomDevelopment_UnsafeDOM_Rejection(t *testing.T) {
	pool := getTestPool(t)

	t.Run("DuplicateSectionMarker", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
		before := captureSnapshot(t, pool, qid)

		duplicated := before.html + `<section data-quotation-section="commercial-development"><table><tr><td>Duplicate</td></tr></table></section>`
		svc, _ := newSvc(pool, &scriptedAIProvider{})

		_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
			ContentHTML:    duplicated,
			ContentVersion: before.version,
		})
		require.Error(t, err)
		assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

		after := captureSnapshot(t, pool, qid)
		assert.Equal(t, before.version, after.version)
	})

	t.Run("MissingPaymentSection", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
		before := captureSnapshot(t, pool, qid)

		// Remove payment section
		start := strings.Index(before.html, `<section data-quotation-section="payment"`)
		require.True(t, start > 0)
		end := strings.Index(before.html[start:], `</section>`)
		require.True(t, end > 0)
		withoutPayment := before.html[:start] + before.html[start+end+len("</section>"):]

		svc, _ := newSvc(pool, &scriptedAIProvider{})

		_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
			ContentHTML:    withoutPayment,
			ContentVersion: before.version,
		})
		require.Error(t, err)
		assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

		after := captureSnapshot(t, pool, qid)
		assert.Equal(t, before.version, after.version)
	})

	t.Run("ScriptOnlyHTML", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
		before := captureSnapshot(t, pool, qid)

		svc, _ := newSvc(pool, &scriptedAIProvider{})

		_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
			ContentHTML:    "<script>alert('pwned')</script>",
			ContentVersion: before.version,
		})
		require.Error(t, err)
		assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

		after := captureSnapshot(t, pool, qid)
		assert.Equal(t, before.version, after.version)
	})

	t.Run("IframeOnlyHTML", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
		before := captureSnapshot(t, pool, qid)

		svc, _ := newSvc(pool, &scriptedAIProvider{})

		_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
			ContentHTML:    `<iframe src="https://evil.example.com"></iframe>`,
			ContentVersion: before.version,
		})
		require.Error(t, err)
		assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

		after := captureSnapshot(t, pool, qid)
		assert.Equal(t, before.version, after.version)
	})

	t.Run("SanitizationStripsMaliciousScript", func(t *testing.T) {
		qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
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
	})
}

func TestSaveDraft_CustomDevelopment_SnapshotCorruption_Rejection(t *testing.T) {
	pool := getTestPool(t)

	cases := []struct {
		name   string
		column string
		value  string
	}{
		{name: "calculation_snapshot_null", column: "calculation_snapshot", value: "null"},
		{name: "calculation_snapshot_zero_investment", column: "calculation_snapshot", value: `{"development_subtotal":15000000,"total_investment":0}`},
		{name: "calculation_snapshot_negative_investment", column: "calculation_snapshot", value: `{"development_subtotal":15000000,"total_investment":-100}`},
		{name: "calculation_snapshot_corrupted_structure", column: "calculation_snapshot", value: `{"invalid_structure":true}`},
		{name: "generation_input_null", column: "generation_input", value: "null"},
		{name: "generation_input_nil_custom_dev", column: "generation_input", value: `{"custom_development":null}`},
		{name: "generation_input_invalid_custom_dev", column: "generation_input", value: `{"custom_development":{"system_name":""}}`},
		{name: "generation_input_corrupted_json", column: "generation_input", value: `{"unknown_field":true}`},
		{name: "legal_template_snapshot_null", column: "legal_template_snapshot", value: "null"},
		{name: "legal_template_snapshot_incomplete", column: "legal_template_snapshot", value: `{"code":"","content_html":""}`},
		{name: "legal_template_snapshot_corrupted_json", column: "legal_template_snapshot", value: `{"unknown_field":true}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			qid, companyID, _ := generateCustomDevelopmentDraft(t, pool)
			before := captureSnapshot(t, pool, qid)

			// Tamper snapshot in DB
			_, err := pool.Exec(context.Background(), fmt.Sprintf(
				`UPDATE quotation_revisions SET %s = $1::jsonb WHERE quotation_id = $2`, tc.column,
			), tc.value, qid)
			require.NoError(t, err)

			svc, _ := newSvc(pool, &scriptedAIProvider{})
			_, err = svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
				ContentHTML:    before.html,
				ContentVersion: before.version,
			})
			require.Error(t, err)
			assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

			// Assert no JSON or internal SQL error leaking
			errMsg := err.Error()
			assert.NotContains(t, errMsg, "{")
			assert.NotContains(t, errMsg, "}")
			assert.NotContains(t, errMsg, "SELECT")
			assert.NotContains(t, errMsg, "UPDATE")
			assert.NotContains(t, errMsg, "pq:")
			assert.NotContains(t, errMsg, "pgx:")

			after := captureSnapshot(t, pool, qid)
			assert.Equal(t, before.version, after.version)
			assert.Equal(t, before.html, after.html)
		})
	}
}

func TestSaveDraft_CustomDevelopment_CrossTenant_Rejection(t *testing.T) {
	pool := getTestPool(t)
	qid, _, _ := generateCustomDevelopmentDraft(t, pool)
	before := captureSnapshot(t, pool, qid)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	foreignCompanyID := uuid.New()

	_, err := svc.SaveDraft(context.Background(), foreignCompanyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    before.html,
		ContentVersion: before.version,
	})
	require.Error(t, err)

	after := captureSnapshot(t, pool, qid)
	assert.Equal(t, before.version, after.version)
	assert.Equal(t, before.html, after.html)
}

// ---------------------------------------------------------------------------
// Kiosk Queue System Save Draft Integration Tests
// ---------------------------------------------------------------------------

func TestSaveDraftKioskQueueUnchangedKeepsAuditFlagsFalse(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)
	require.Equal(t, 1, snap.version)
	require.NotEmpty(t, snap.html)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	userID := uuid.New()

	res, err := svc.SaveDraft(context.Background(), companyID, qid, &userID, dto.SaveDraftRequest{
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

func TestSaveDraftKioskQueueScopeOnlyDoesNotMarkCommercial(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)
	require.Equal(t, 1, snap.version)

	modifiedHTML := strings.Replace(
		snap.html,
		"Dokumen Analisis Kebutuhan",
		"Dokumen Analisis Kebutuhan Terintegrasi",
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
	assert.False(t, res.LegalContentModified)
	assert.False(t, res.CommercialContentModified)

	snapAfter := captureSnapshot(t, pool, qid)
	assert.Equal(t, 2, snapAfter.version)
}

func TestSaveDraftKioskQueueCostChangeMarksCommercial(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	modifiedHTML := strings.Replace(snap.html, `data-field="service_line_total">Rp 3.342.500`, `data-field="service_line_total">Rp 5.000.000`, 1)
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
}

func TestSaveDraftKioskQueueHardwareChangeMarksCommercial(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	modifiedHTML := strings.Replace(snap.html, `data-field="hardware_line_total">Rp 24.500.000`, `data-field="hardware_line_total">Rp 26.000.000`, 1)
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
}

func TestSaveDraftKioskQueueTaxChangeMarksCommercial(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	taxText := "Total investasi telah termasuk PPN 11% sesuai regulasi perpajakan yang berlaku di Indonesia."
	require.Contains(t, snap.html, taxText)
	modifiedHTML := strings.Replace(snap.html, taxText, "Total investasi belum termasuk PPN.", 1)
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
}

func TestSaveDraftKioskQueuePaymentChangeMarksCommercial(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	modifiedHTML := strings.Replace(snap.html, "Rp 51.615.000", "Rp 55.000.000", 1)
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
}

func TestSaveDraftKioskQueueRightsChangeMarksLegal(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool, func(input *domain.KioskQueueSystemInput) {
		input.RightsStatements = []string{"Hak Cipta sistem antrean adalah milik vendor sepenuhnya."}
	})
	snap := captureSnapshot(t, pool, qid)
	require.Contains(t, snap.html, "Hak Cipta sistem antrean adalah milik vendor sepenuhnya.")

	modifiedHTML := strings.Replace(
		snap.html,
		"Hak Cipta sistem antrean adalah milik vendor sepenuhnya.",
		"Hak Cipta sistem antrean dialihkan kepada pemda sepenuhnya.",
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
	assert.False(t, res.CommercialContentModified)
	assert.True(t, res.LegalContentModified)
}

func TestSaveDraftKioskQueueLegalTemplateChangeMarksLegal(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	require.Contains(t, snap.html, "Syarat dan ketentuan penawaran resmi berlaku 30 hari.")
	modifiedHTML := strings.Replace(
		snap.html,
		"Syarat dan ketentuan penawaran resmi berlaku 30 hari.",
		"Syarat dan ketentuan penawaran resmi berlaku 60 hari.",
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
	assert.False(t, res.CommercialContentModified)
	assert.True(t, res.LegalContentModified)
}

func TestSaveDraftKioskQueueRejectsStaleContentVersion(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	svc, _ := newSvc(pool, &scriptedAIProvider{})

	// Pass stale version (99 instead of 1)
	_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
		ContentVersion: 99,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, service.ErrCodeRevisionConflict)

	// Save successfully once to reach version 2
	res, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
		ContentVersion: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.ContentVersion)

	// Attempt save again with old version 1
	_, err = svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
		ContentVersion: 1,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, service.ErrCodeRevisionConflict)
}

func TestSaveDraftKioskQueueRejectsRemovedCivilClause(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateKioskQueueDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	civilClause := service.KioskQueueLimitationCatalog[domain.KioskLimitCivilElectrical]
	require.Contains(t, snap.html, civilClause)

	modifiedHTML := strings.Replace(snap.html, civilClause, "", 1)
	require.NotEqual(t, snap.html, modifiedHTML)

	svc, _ := newSvc(pool, &scriptedAIProvider{})

	_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    modifiedHTML,
		ContentVersion: 1,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)

	after := captureSnapshot(t, pool, qid)
	assert.Equal(t, 1, after.version)
}

// ---------------------------------------------------------------------------
// Dedicated Squad Modern Save Draft Integration Tests
// ---------------------------------------------------------------------------

func generateDedicatedSquadModernDraft(t *testing.T, pool *pgxpool.Pool) (uuid.UUID, uuid.UUID) {
	t.Helper()
	fx := seedPipelineFixtures(t, pool)

	ctx := context.Background()
	rateCardID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO rate_cards (id, company_id, code, name, market, currency, effective_from, is_active)
		VALUES ($1, $2, 'VENTURO-STD-2026', 'Venturo Standard 2026', 'national', 'IDR', NOW(), true)
	`, rateCardID, fx.companyID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO rate_card_items (company_id, rate_card_id, role, seniority, term_type, amount_per_month, kickoff_fee)
		VALUES
			($1, $2, 'backend_programmer', 'middle', 'short_term', 18000000, 3300000),
			($1, $2, 'backend_programmer', 'middle', 'long_term', 18000000, 3300000)
	`, fx.companyID, rateCardID)
	require.NoError(t, err)

	anchor := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	input := domain.DedicatedSquadInput{
		Model:       domain.ModelSOWSmall,
		Requirement: "Build backend platform API and quotation service",
		DevelopmentSquad: []domain.SquadLineInput{
			{
				Role:              "backend_programmer",
				Seniority:         "middle",
				HeadcountQuarters: 4,
				PeriodQuarters:    12,
			},
		},
		PaymentCount:      3,
		PaymentAnchorDate: &anchor,
		Timeline: domain.TimelineInput{
			Mode: domain.TimelineRelative,
		},
	}

	req := fx.request()
	req.CommercialScheme = domain.CommercialSchemeDedicatedSquad
	req.SolutionType = "dedicated_squad"
	req.RateCardID = rateCardID
	req.RateCardCode = "VENTURO-STD-2026"
	req.DedicatedSquad = &input
	req.SubscriptionSaaS, req.WhiteLabel, req.StandaloneMaintenance, req.CustomDevelopment, req.KioskQueueSystem, req.TalentOutsourcing, req.CloudInfrastructure = nil, nil, nil, nil, nil, nil, nil
	req.BudgetMaximum, req.ProjectDurationMonths = 0, 0
	req.MandatoryScope, req.OptionalScope, req.AllowedStandardActivities = nil, nil, nil
	req.PaymentTerms = dto.PaymentTermsInput{}

	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: `{"modules":[{"name":"Platform API","sub_features":["Authentication","Quotation Service"]}]}`},
	}}
	svc, _ := newSvc(pool, mock)
	qid, err := svc.Generate(ctx, fx.companyID, nil, req)
	require.NoError(t, err)
	return qid, fx.companyID
}

func TestSaveDraftDedicatedModern_Success_Unmodified(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID := generateDedicatedSquadModernDraft(t, pool)

	// Verify AllocationSnapshot is strictly NULL in database
	var allocSnap *string
	err := pool.QueryRow(context.Background(), `SELECT allocation_snapshot FROM quotation_revisions WHERE quotation_id = $1`, qid).Scan(&allocSnap)
	require.NoError(t, err)
	assert.Nil(t, allocSnap, "modern dedicated squad must have NULL allocation_snapshot")

	snap := captureSnapshot(t, pool, qid)
	require.Equal(t, 1, snap.version)
	require.NotEmpty(t, snap.html)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	userID := uuid.New()

	res, err := svc.SaveDraft(context.Background(), companyID, qid, &userID, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
		ContentVersion: snap.version,
	})
	if err != nil {
		t.Fatalf("SaveDraft failed: %v (detail: %#v)", err, err)
	}
	require.NoError(t, err)
	assert.Equal(t, qid, res.QuotationID)
	assert.Equal(t, 2, res.ContentVersion)
	assert.False(t, res.LegalContentModified)
	assert.False(t, res.CommercialContentModified)
	assert.False(t, res.UpdatedAt.IsZero())

	// Verify DB state and verify allocation_snapshot remains NULL
	snapAfter := captureSnapshot(t, pool, qid)
	assert.Equal(t, 2, snapAfter.version)
	err = pool.QueryRow(context.Background(), `SELECT allocation_snapshot FROM quotation_revisions WHERE quotation_id = $1`, qid).Scan(&allocSnap)
	require.NoError(t, err)
	assert.Nil(t, allocSnap, "allocation_snapshot must remain NULL after save draft")
}

func TestSaveDraftDedicatedModern_CommercialModified(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID := generateDedicatedSquadModernDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	modified := strings.Replace(snap.html, "54.000.000", "58.000.000", 1)
	require.NotEqual(t, snap.html, modified)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	res, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    modified,
		ContentVersion: snap.version,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.ContentVersion)
	assert.False(t, res.LegalContentModified)
	assert.True(t, res.CommercialContentModified)
}

func TestSaveDraftDedicatedModern_LegalModified(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID := generateDedicatedSquadModernDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	modified := strings.Replace(snap.html, "Biaya akomodasi, transportasi luar kota", "Biaya akomodasi khusus dinegosiasikan", 1)
	require.NotEqual(t, snap.html, modified)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	res, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    modified,
		ContentVersion: snap.version,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.ContentVersion)
	assert.True(t, res.LegalContentModified)
	assert.False(t, res.CommercialContentModified)
}

func TestSaveDraftDedicatedModern_RejectsMissingRequiredSection(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID := generateDedicatedSquadModernDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	modified := strings.Replace(snap.html, `<section data-block-type="payment"`, `<section data-block-type="removed"`, 1)
	require.NotEqual(t, snap.html, modified)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    modified,
		ContentVersion: snap.version,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)
}

func TestSaveDraftDedicatedModern_RejectsDuplicateMarker(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID := generateDedicatedSquadModernDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	modified := strings.Replace(snap.html, `<td data-field="total_investment">`, `<td data-field="total_investment"><td data-field="total_investment">`, 1)
	require.NotEqual(t, snap.html, modified)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    modified,
		ContentVersion: snap.version,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, service.ErrCodeUnsafeHTML)
}
