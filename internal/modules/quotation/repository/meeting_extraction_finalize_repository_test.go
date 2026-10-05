//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestFinalizeP5AtomicallyChangesSchemeAndIsIdempotent(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	companyID := uuid.New()
	cleanupCompanies(t, pool, companyID)
	draftParams := validDraftParams(companyID)
	draftParams.ClientID = nil
	draftParams.ClientName = "P5 Integration Client"
	draft, err := repo.CreateDraftTransaction(ctx, draftParams)
	require.NoError(t, err)

	params := repository.P5FinalizeCommit{
		CompanyID: companyID, QuotationID: draft.QuotationID, RevisionID: draft.RevisionID,
		Scheme: "white_label", SolutionType: "white_label_software",
		PromptState: []byte(`{"schema_version":"prompt_quotation_v1","commercial_scheme":{"value":"white_label"}}`),
		Artifacts: repository.P5FinalizationArtifacts{
			ContentHTML: "<section>white label</section>", ContentHash: stringPtr("p5-content-hash"),
			ResolvedProfileSnapshot: []byte(`{"profile":"p5"}`), CalculationSnapshot: []byte(`{"total":100}`),
			ModuleSnapshot: []byte(`[{"name":"module"}]`), TimelineSnapshot: []byte(`[{"name":"timeline"}]`),
		},
	}
	first, err := repo.FinalizeP5(ctx, params)
	require.NoError(t, err)
	require.False(t, first.AlreadyCompleted)

	var scheme, solution, masterStatus, revisionStatus, promptState, html, profile, calculation, module, timeline string
	err = pool.QueryRow(ctx, `SELECT q.commercial_scheme, q.solution_type, q.status, r.generation_status, r.prompt_state::text, r.content_html, r.resolved_profile_snapshot::text, r.calculation_snapshot::text, r.module_snapshot::text, r.timeline_snapshot::text FROM quotations q JOIN quotation_revisions r ON r.quotation_id=q.id WHERE q.id=$1 AND r.id=$2`, draft.QuotationID, draft.RevisionID).Scan(&scheme, &solution, &masterStatus, &revisionStatus, &promptState, &html, &profile, &calculation, &module, &timeline)
	require.NoError(t, err)
	require.Equal(t, "white_label", scheme)
	require.Equal(t, "white_label_software", solution)
	require.Equal(t, repository.PublicationStatusDraft, masterStatus)
	require.Equal(t, repository.GenerationStatusCompleted, revisionStatus)
	require.Contains(t, promptState, "white_label")
	require.Equal(t, "<section>white label</section>", html)
	for _, snapshot := range []string{profile, calculation, module, timeline} {
		require.NotEmpty(t, snapshot)
	}

	params.Artifacts.ContentHTML = "<section>duplicate</section>"
	second, err := repo.FinalizeP5(ctx, params)
	require.NoError(t, err)
	require.True(t, second.AlreadyCompleted)
	err = pool.QueryRow(ctx, `SELECT content_html FROM quotation_revisions WHERE id=$1`, draft.RevisionID).Scan(&html)
	require.NoError(t, err)
	require.Equal(t, "<section>white label</section>", html)
}

func stringPtr(value string) *string { return &value }
