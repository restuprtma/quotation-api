//go:build integration

package repository_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func seedPromptDraft(t *testing.T, pool *pgxpool.Pool) regenFixture {
	t.Helper()
	ctx := context.Background()
	companyID := uuid.New()
	cleanupCompanies(t, pool, companyID)

	params := validDraftParams(companyID)
	params.ClientID = nil
	params.ClientName = "Prompt lifecycle client"
	res, err := repository.NewGenerationRepository(pool).CreateDraftTransaction(ctx, params)
	require.NoError(t, err)
	return regenFixture{
		companyID:   companyID,
		quotationID: res.QuotationID,
		revisionID:  res.RevisionID,
		number:      params.QuotationNumber,
	}
}

func TestUpdatePromptDraftCAS_TenantAndVersions(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedPromptDraft(t, pool)

	params := repository.UpdatePromptDraftCASParams{
		CompanyID: fx.companyID, QuotationID: fx.quotationID, RevisionID: fx.revisionID,
		ExpectedContentVersion: 1, ExpectedStateVersion: 0,
		State:       []byte(`{"schema_version":"prompt_quotation_v1","meeting_notes":"notes"}`),
		ContentHTML: "<p>prompt draft</p>",
		Snapshots: repository.PromptDraftSnapshots{
			AllocationSnapshot: []byte(`{}`), RateCardSnapshot: []byte(`{}`),
			LegalTemplateSnapshot: []byte(`{}`), SkillSnapshot: []byte(`{}`),
			ResolvedProfile: []byte(`{}`), Calculation: []byte(`{}`),
			Module: []byte(`[]`), Timeline: []byte(`[]`), SectionHashes: []byte(`{"scope":"a"}`),
		},
	}

	result, err := repo.UpdatePromptDraftCAS(ctx, params)
	require.NoError(t, err)
	assert.Equal(t, 2, result.ContentVersion)
	assert.Equal(t, int64(1), result.StateVersion)

	params.ExpectedContentVersion = 1
	_, err = repo.UpdatePromptDraftCAS(ctx, params)
	assert.ErrorIs(t, err, repository.ErrRevisionConflict)

	params.ExpectedContentVersion = 2
	params.ExpectedStateVersion = 1
	_, err = repo.UpdatePromptDraftCAS(ctx, params)
	assert.NoError(t, err)

	_, err = repo.UpdatePromptDraftCAS(ctx, repository.UpdatePromptDraftCASParams{
		CompanyID: fx.companyID, QuotationID: fx.quotationID, RevisionID: fx.revisionID,
		ExpectedContentVersion: 3, ExpectedStateVersion: 2, State: []byte(`{`), ContentHTML: "<p>must rollback</p>",
	})
	assert.Error(t, err)

	foreign := params
	foreign.CompanyID = uuid.New()
	foreign.ExpectedContentVersion = 3
	foreign.ExpectedStateVersion = 2
	foreign.QuotationID = fx.quotationID
	foreign.RevisionID = fx.revisionID
	_, err = repo.UpdatePromptDraftCAS(ctx, foreign)
	assert.ErrorIs(t, err, repository.ErrRevisionConflict)
}

func TestLoadPromptDraftReadsTextSkillSnapshot(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedPromptDraft(t, pool)

	_, err := repo.CompletePromptDraft(ctx, repository.CompletePromptDraftParams{
		CompanyID:               fx.companyID,
		QuotationID:             fx.quotationID,
		RevisionID:              fx.revisionID,
		ContentHTML:             `<article><p>Draft prompt</p></article>`,
		PromptState:             json.RawMessage(`{"schema_version":"prompt_quotation_v1","meeting_notes":"safe note","scope":[],"team":[],"issues":[],"readiness":"incomplete"}`),
		PromptSectionHashes:     json.RawMessage(`{"scope":"hash"}`),
		RateCardSnapshot:        json.RawMessage(`{}`),
		LegalTemplateSnapshot:   json.RawMessage(`{}`),
		SkillSnapshot:           json.RawMessage(`{"code":"prompt-first"}`),
		ResolvedProfileSnapshot: json.RawMessage(`null`),
		CalculationSnapshot:     json.RawMessage(`null`),
		ModuleSnapshot:          json.RawMessage(`[]`),
		TimelineSnapshot:        json.RawMessage(`[]`),
	})
	require.NoError(t, err)

	record, err := repo.LoadPromptDraft(ctx, fx.companyID, fx.quotationID)

	require.NoError(t, err)
	require.NotNil(t, record)
	assert.JSONEq(t, `{"code":"prompt-first"}`, string(record.SkillSnapshot))
	assert.Equal(t, fx.quotationID, record.QuotationID)
	assert.Equal(t, fx.revisionID, record.RevisionID)

	_, err = pool.Exec(ctx, `UPDATE quotation_revisions SET skill_snapshot = NULL WHERE id = $1`, fx.revisionID)
	require.NoError(t, err)

	record, err = repo.LoadPromptDraft(ctx, fx.companyID, fx.quotationID)
	require.NoError(t, err)
	assert.JSONEq(t, `null`, string(record.SkillSnapshot))
}

func TestClaimPromptDraft_LifecycleAndRetry(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedPromptDraft(t, pool)

	var inputJSON []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT generation_input FROM quotation_revisions WHERE id = $1`, fx.revisionID).Scan(&inputJSON))

	claim, err := repo.ClaimPromptDraft(ctx, fx.companyID, fx.quotationID)
	require.NoError(t, err)
	assert.Equal(t, fx.quotationID, claim.QuotationID)
	assert.Equal(t, fx.revisionID, claim.RevisionID)
	assert.JSONEq(t, string(inputJSON), string(claim.GenerationInput))

	_, err = repo.ClaimPromptDraft(ctx, fx.companyID, fx.quotationID)
	assert.ErrorIs(t, err, repository.ErrGenerationInProgress)

	require.NoError(t, repo.FailPromptDraft(ctx, fx.companyID, fx.quotationID, fx.revisionID, "provider_timeout"))
	failed, err := repo.GetGenerationState(ctx, fx.companyID, fx.quotationID)
	require.NoError(t, err)
	assert.Equal(t, repository.GenerationStatusFailed, failed.GenerationStatus)
	assert.Equal(t, "provider_timeout", failed.ErrorCode)

	retry, err := repo.ClaimPromptDraft(ctx, fx.companyID, fx.quotationID)
	require.NoError(t, err)
	assert.Equal(t, fx.revisionID, retry.RevisionID)

	var status, errorCode string
	require.NoError(t, pool.QueryRow(ctx, `SELECT generation_status, COALESCE(generation_error, '') FROM quotation_revisions WHERE id = $1`, fx.revisionID).Scan(&status, &errorCode))
	assert.Equal(t, repository.GenerationStatusInProgress, status)
	assert.Empty(t, errorCode)
}

func TestClaimPromptDraft_RejectsForeignTenant(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedPromptDraft(t, pool)

	_, err := repo.ClaimPromptDraft(ctx, uuid.New(), fx.quotationID)
	assert.ErrorIs(t, err, repository.ErrRevisionNotFound)
}

func TestClaimPromptDraft_RejectsCompletedDraft(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedPromptDraft(t, pool)

	_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET generation_status = $1 WHERE id = $2`, repository.GenerationStatusCompleted, fx.revisionID)
	require.NoError(t, err)

	_, err = repo.ClaimPromptDraft(ctx, fx.companyID, fx.quotationID)
	assert.ErrorIs(t, err, repository.ErrGenerationAlreadyCompleted)
}

func TestFailPromptDraft_AllowlistsErrorCodeAndGenerationStatusIsSafe(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedPromptDraft(t, pool)

	require.NoError(t, repo.FailPromptDraft(ctx, fx.companyID, fx.quotationID, fx.revisionID, "provider response: secret text"))

	state, err := repo.GetGenerationState(ctx, fx.companyID, fx.quotationID)
	require.NoError(t, err)
	assert.Equal(t, repository.GenerationStatusFailed, state.GenerationStatus)
	assert.Equal(t, "generation_failed", state.ErrorCode)

	var masterStatus, persistedError string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT q.status, COALESCE(r.generation_error, '')
		FROM quotations q JOIN quotation_revisions r ON r.quotation_id = q.id
		WHERE r.id = $1
	`, fx.revisionID).Scan(&masterStatus, &persistedError))
	assert.Equal(t, repository.QuotationStatusFailed, masterStatus)
	assert.Equal(t, "generation_failed", persistedError)
}
