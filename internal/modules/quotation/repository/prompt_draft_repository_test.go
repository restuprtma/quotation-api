//go:build integration

package repository_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestUpdatePromptDraftCAS_TenantAndVersions(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

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
	fx := seedRegenDraft(t, pool)

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
