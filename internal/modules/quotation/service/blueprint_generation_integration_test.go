//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func getBlueprintIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		host := os.Getenv("DB_HOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("DB_PORT")
		if port == "" {
			port = "5432"
		}
		user := os.Getenv("DB_USER")
		if user == "" {
			user = "postgres"
		}
		pass := os.Getenv("DB_PASSWORD")
		if pass == "" {
			pass = "postgres"
		}
		name := os.Getenv("DB_NAME")
		if name == "" {
			name = "tuai"
		}
		ssl := os.Getenv("DB_SSLMODE")
		if ssl == "" {
			ssl = "disable"
		}
		dsn = fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, name, ssl)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("failed to ping test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func cleanupIntegrationTenant(t *testing.T, pool *pgxpool.Pool, companyIDs ...uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, cid := range companyIDs {
		if cid == uuid.Nil {
			continue
		}
		_, _ = pool.Exec(ctx, `UPDATE blueprints SET active_published_revision_id = NULL WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `UPDATE quotations SET active_published_revision_id = NULL WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `DELETE FROM blueprint_section_runs WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `DELETE FROM blueprint_revisions WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `DELETE FROM blueprints WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `DELETE FROM ai_skills WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `DELETE FROM quotation_revisions WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `DELETE FROM quotations WHERE company_id = $1`, cid)
	}
}

func seedQuotationAndRevision(
	t *testing.T,
	pool *pgxpool.Pool,
	companyID uuid.UUID,
	pubStatus string,
	genStatus string,
	contentHTML string,
) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	quotationID := uuid.New()
	revisionID := uuid.New()
	quotationNumber := "BP-INT-" + uuid.NewString()[:8]

	_, err := pool.Exec(ctx, `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject,
			solution_type, commercial_scheme, status, current_revision
		) VALUES (
			$1, $2, gen_random_uuid(), $3, 'Enterprise ERP Blueprint Source',
			'ERP', 'custom_development', 'draft', 1
		)
	`, quotationID, companyID, quotationNumber)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number,
			publication_status, generation_status, generation_input,
			content_html, content_version, created_by
		) VALUES (
			$1, $2, $3, 1,
			$4, $5, '{"project":"ERP"}'::jsonb,
			$6, 1, NULL
		)
	`, revisionID, companyID, quotationID, pubStatus, genStatus, contentHTML)
	require.NoError(t, err)

	return quotationID, revisionID
}

func seedActiveBlueprintSkill(
	t *testing.T,
	pool *pgxpool.Pool,
	companyID uuid.UUID,
	name, instruction string,
) *domain.Skill {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	skill := &domain.Skill{
		ID:                 uuid.New(),
		CompanyID:          companyID,
		Name:               name,
		Command:            service.BlueprintSkillCommand,
		ContentInstruction: instruction,
		IsActive:           true,
	}

	skillRepo := repository.NewSkillRepository(pool)
	err := skillRepo.Create(ctx, pool, skill)
	require.NoError(t, err)
	return skill
}

func validBlueprintRequest(quotationID, revisionID uuid.UUID) dto.CreateBlueprintGenerationRequest {
	return dto.CreateBlueprintGenerationRequest{
		IdempotencyKey:            uuid.New(),
		QuotationID:               quotationID,
		SourceQuotationRevisionID: revisionID,
		Title:                     "Enterprise ERP Blueprint Plan",
		BusinessGoals:             []string{"Scale order fulfillment", "Automate warehouse sync"},
		Actors:                    []string{"Warehouse Staff", "Ops Manager"},
		Channels:                  []string{"Web Portal", "Barcode Handheld"},
		BusinessPhases:            []string{"Phase 1: Ingestion", "Phase 2: Distribution"},
		BusinessRules:             []string{"Rule 1: FIFO inventory check"},
		KnownStates:               []string{"Pending", "Dispatched", "Delivered"},
		Integrations:              []string{"SAP ERP Connector", "Stripe"},
		TechnologyPreferences:     []string{"Go", "PostgreSQL"},
		OperationalConstraints:    []string{"Sub-second latency", "24/7 SLA"},
		AdditionalNotes:           "Enterprise grade delivery required",
	}
}

func assertNoAggregateRows(t *testing.T, pool *pgxpool.Pool, companyID uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var bpCount, revCount, runCount int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM blueprints WHERE company_id = $1`, companyID).Scan(&bpCount)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM blueprint_revisions WHERE company_id = $1`, companyID).Scan(&revCount)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM blueprint_section_runs WHERE company_id = $1`, companyID).Scan(&runCount)
	require.NoError(t, err)

	assert.Equal(t, 0, bpCount, "expected zero blueprints rows")
	assert.Equal(t, 0, revCount, "expected zero blueprint_revisions rows")
	assert.Equal(t, 0, runCount, "expected zero blueprint_section_runs rows")
}

func TestBlueprintGenerationServiceProductionDependencies(t *testing.T) {
	pool := getBlueprintIntegrationPool(t)
	ctx := context.Background()

	svc := service.NewBlueprintGenerationService(
		repository.NewBlueprintGenerationInputRepository(pool),
		repository.NewSkillRepository(pool),
		service.NewBlueprintSourceContextBuilder(),
		repository.NewBlueprintGenerationRepository(pool),
	)
	genRepo := repository.NewBlueprintGenerationRepository(pool)

	t.Run("valid create persists eligible source and active /blueprint snapshot", func(t *testing.T) {
		companyID := uuid.New()
		cleanupIntegrationTenant(t, pool, companyID)
		t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

		qID, revID := seedQuotationAndRevision(
			t, pool, companyID, "draft", "completed",
			"<section><h2>ERP Scope</h2><p>Full implementation detail</p></section>",
		)
		skill := seedActiveBlueprintSkill(
			t, pool, companyID, "ERP Blueprint Architect",
			"Produce high-level system architecture blueprint",
		)

		createdBy := uuid.New()
		req := validBlueprintRequest(qID, revID)

		res, err := svc.CreateBlueprint(ctx, companyID, createdBy, req)
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.False(t, res.IdempotentReplay)
		assert.Equal(t, companyID, res.Blueprint.CompanyID)
		assert.Equal(t, qID, res.Blueprint.QuotationID)
		assert.Equal(t, req.Title, res.Blueprint.Title)
		assert.Equal(t, "draft", res.Blueprint.Status)
		assert.Equal(t, 1, res.Blueprint.CurrentRevision)
		assert.Equal(t, &createdBy, res.Blueprint.CreatedBy)

		// Verify database persistence via repository
		loaded, err := genRepo.GetBlueprintGeneration(ctx, res.Blueprint.ID, companyID)
		require.NoError(t, err)
		require.NotNil(t, loaded)
		assert.Equal(t, res.Revision.ID, loaded.Revision.ID)
		require.NotNil(t, loaded.Revision.SourceQuotationRevisionID)
		assert.Equal(t, revID, *loaded.Revision.SourceQuotationRevisionID)
		assert.Equal(t, "draft", loaded.Revision.PublicationStatus)
		assert.Equal(t, "pending", loaded.Revision.GenerationStatus)
		assert.Equal(t, "partial", loaded.Revision.CompletenessStatus)

		// Source context snapshot validation
		require.NotEmpty(t, loaded.Revision.SourceContextSnapshot)
		assert.True(t, json.Valid(loaded.Revision.SourceContextSnapshot))
		var srcSnap map[string]interface{}
		err = json.Unmarshal(loaded.Revision.SourceContextSnapshot, &srcSnap)
		require.NoError(t, err)
		assert.Equal(t, "Enterprise ERP Blueprint Source", srcSnap["subject"])
		assert.Equal(t, "ERP", srcSnap["solution_type"])

		// Skill snapshot validation: exact 5 allowlisted keys, no forbidden keys
		require.NotEmpty(t, loaded.Revision.SkillSnapshot)
		assert.True(t, json.Valid(loaded.Revision.SkillSnapshot))

		var rawSkillMap map[string]interface{}
		err = json.Unmarshal(loaded.Revision.SkillSnapshot, &rawSkillMap)
		require.NoError(t, err)
		assert.Len(t, rawSkillMap, 5, "skill snapshot must contain exactly 5 allowlisted keys")
		assert.Contains(t, rawSkillMap, "id")
		assert.Contains(t, rawSkillMap, "name")
		assert.Contains(t, rawSkillMap, "command")
		assert.Contains(t, rawSkillMap, "content_instruction")
		assert.Contains(t, rawSkillMap, "updated_at")

		for _, forbidden := range []string{
			"company_id", "created_by", "created_at", "deleted_at", "is_active",
			"credential", "token", "provider", "api_key", "model",
		} {
			assert.NotContains(t, rawSkillMap, forbidden)
		}

		var snap service.BlueprintSkillSnapshot
		err = json.Unmarshal(loaded.Revision.SkillSnapshot, &snap)
		require.NoError(t, err)
		assert.Equal(t, skill.ID, snap.ID)
		assert.Equal(t, "/blueprint", snap.Command)
		assert.Equal(t, "ERP Blueprint Architect", snap.Name)
		assert.Equal(t, "Produce high-level system architecture blueprint", snap.ContentInstruction)
		assert.False(t, snap.UpdatedAt.IsZero())

		// Section runs validation: context section is validated, others pending
		assert.Len(t, loaded.SectionRuns, 7)
		for i, run := range loaded.SectionRuns {
			if run.SectionType == string(domain.BlueprintSectionContext) {
				assert.Equal(t, "validated", run.Status)
			} else {
				assert.Equal(t, "pending", run.Status, "run %d (%s) must be pending", i, run.SectionType)
			}
			assert.Equal(t, 0, run.AttemptCount)
			assert.Nil(t, run.SafeErrorCode)
		}
	})

	t.Run("foreign/missing/ineligible source rejected with no aggregate rows", func(t *testing.T) {
		companyA := uuid.New()
		companyB := uuid.New()
		cleanupIntegrationTenant(t, pool, companyA, companyB)
		t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyA, companyB) })

		// Seed active skill for company A
		seedActiveBlueprintSkill(t, pool, companyA, "Blueprint Skill A", "Generate blueprint")
		createdBy := uuid.New()

		// Valid source for company B (foreign to company A)
		qB, revB := seedQuotationAndRevision(
			t, pool, companyB, "draft", "completed",
			"<section><h2>Scope B</h2><p>Company B quotation</p></section>",
		)

		// Ineligible sources for company A
		qArchived, revArchived := seedQuotationAndRevision(
			t, pool, companyA, "archived", "completed",
			"<section><h2>Scope Archived</h2><p>Archived quotation</p></section>",
		)
		qFailed, revFailed := seedQuotationAndRevision(
			t, pool, companyA, "draft", "failed",
			"<section><h2>Scope Failed</h2><p>Failed quotation</p></section>",
		)
		qGenerating, revGenerating := seedQuotationAndRevision(
			t, pool, companyA, "draft", "generating",
			"<section><h2>Scope Generating</h2><p>Generating quotation</p></section>",
		)
		qWhitespace, revWhitespace := seedQuotationAndRevision(
			t, pool, companyA, "draft", "completed",
			"   \n\t  ",
		)
		qDeleted, revDeleted := seedQuotationAndRevision(
			t, pool, companyA, "draft", "completed",
			"<section><h2>Scope Deleted</h2><p>Soft-deleted quotation</p></section>",
		)
		_, err := pool.Exec(ctx, `UPDATE quotations SET deleted_at = NOW() WHERE id = $1`, qDeleted)
		require.NoError(t, err)

		// Test cases
		tests := []struct {
			name        string
			quotationID uuid.UUID
			revisionID  uuid.UUID
			expectedErr error
		}{
			{
				name:        "missing quotation and revision",
				quotationID: uuid.New(),
				revisionID:  uuid.New(),
				expectedErr: repository.ErrBlueprintSourceNotFound,
			},
			{
				name:        "foreign source from company B",
				quotationID: qB,
				revisionID:  revB,
				expectedErr: repository.ErrBlueprintSourceNotFound,
			},
			{
				name:        "ineligible publication status archived",
				quotationID: qArchived,
				revisionID:  revArchived,
				expectedErr: repository.ErrBlueprintSourceIneligible,
			},
			{
				name:        "ineligible generation status failed",
				quotationID: qFailed,
				revisionID:  revFailed,
				expectedErr: repository.ErrBlueprintSourceIneligible,
			},
			{
				name:        "ineligible generation status generating",
				quotationID: qGenerating,
				revisionID:  revGenerating,
				expectedErr: repository.ErrBlueprintSourceIneligible,
			},
			{
				name:        "ineligible whitespace content html",
				quotationID: qWhitespace,
				revisionID:  revWhitespace,
				expectedErr: repository.ErrBlueprintSourceIneligible,
			},
			{
				name:        "soft-deleted quotation",
				quotationID: qDeleted,
				revisionID:  revDeleted,
				expectedErr: repository.ErrBlueprintSourceNotFound,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				req := validBlueprintRequest(tc.quotationID, tc.revisionID)
				res, err := svc.CreateBlueprint(ctx, companyA, createdBy, req)
				assert.Nil(t, res)
				assert.ErrorIs(t, err, tc.expectedErr)
				assertNoAggregateRows(t, pool, companyA)
			})
		}
	})

	t.Run("missing/inactive /blueprint skill rejected with no aggregate rows", func(t *testing.T) {
		companyA := uuid.New()
		companyB := uuid.New()
		cleanupIntegrationTenant(t, pool, companyA, companyB)
		t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyA, companyB) })

		// Seed valid source for company A
		qID, revID := seedQuotationAndRevision(
			t, pool, companyA, "draft", "completed",
			"<section><h2>Scope A</h2><p>Eligible quotation content</p></section>",
		)
		createdBy := uuid.New()

		// 1. Missing skill entirely (company A has no skill)
		t.Run("missing skill", func(t *testing.T) {
			req := validBlueprintRequest(qID, revID)
			res, err := svc.CreateBlueprint(ctx, companyA, createdBy, req)
			assert.Nil(t, res)
			assert.ErrorIs(t, err, repository.ErrSkillNotFound)
			assertNoAggregateRows(t, pool, companyA)
		})

		// 2. Foreign skill (active /blueprint skill exists only in company B)
		t.Run("foreign skill", func(t *testing.T) {
			seedActiveBlueprintSkill(t, pool, companyB, "Foreign Skill", "Generate blueprint foreign")
			req := validBlueprintRequest(qID, revID)
			res, err := svc.CreateBlueprint(ctx, companyA, createdBy, req)
			assert.Nil(t, res)
			assert.ErrorIs(t, err, repository.ErrSkillNotFound)
			assertNoAggregateRows(t, pool, companyA)
		})

		// 3. Inactive skill in company A
		t.Run("inactive skill", func(t *testing.T) {
			inactiveSkill := &domain.Skill{
				ID:                 uuid.New(),
				CompanyID:          companyA,
				Name:               "Inactive Skill",
				Command:            "/blueprint",
				ContentInstruction: "Inactive instruction",
				IsActive:           false,
			}
			skillRepo := repository.NewSkillRepository(pool)
			err := skillRepo.Create(ctx, pool, inactiveSkill)
			require.NoError(t, err)

			req := validBlueprintRequest(qID, revID)
			res, err := svc.CreateBlueprint(ctx, companyA, createdBy, req)
			assert.Nil(t, res)
			assert.ErrorIs(t, err, repository.ErrSkillNotFound)
			assertNoAggregateRows(t, pool, companyA)

			// Clean up skill
			_, _ = pool.Exec(ctx, `DELETE FROM ai_skills WHERE company_id = $1`, companyA)
		})

		// 4. Soft-deleted skill in company A
		t.Run("soft-deleted skill", func(t *testing.T) {
			deletedSkill := &domain.Skill{
				ID:                 uuid.New(),
				CompanyID:          companyA,
				Name:               "Deleted Skill",
				Command:            "/blueprint",
				ContentInstruction: "Deleted instruction",
				IsActive:           true,
			}
			skillRepo := repository.NewSkillRepository(pool)
			err := skillRepo.Create(ctx, pool, deletedSkill)
			require.NoError(t, err)
			_, err = pool.Exec(ctx, `UPDATE ai_skills SET deleted_at = NOW() WHERE id = $1`, deletedSkill.ID)
			require.NoError(t, err)

			req := validBlueprintRequest(qID, revID)
			res, err := svc.CreateBlueprint(ctx, companyA, createdBy, req)
			assert.Nil(t, res)
			assert.ErrorIs(t, err, repository.ErrSkillNotFound)
			assertNoAggregateRows(t, pool, companyA)

			// Clean up skill
			_, _ = pool.Exec(ctx, `DELETE FROM ai_skills WHERE company_id = $1`, companyA)
		})

		// 5. Wrong command in company A (/quotation instead of /blueprint)
		t.Run("wrong command skill", func(t *testing.T) {
			wrongCommandSkill := &domain.Skill{
				ID:                 uuid.New(),
				CompanyID:          companyA,
				Name:               "Quotation Skill",
				Command:            "/quotation",
				ContentInstruction: "Quotation instruction",
				IsActive:           true,
			}
			skillRepo := repository.NewSkillRepository(pool)
			err := skillRepo.Create(ctx, pool, wrongCommandSkill)
			require.NoError(t, err)

			req := validBlueprintRequest(qID, revID)
			res, err := svc.CreateBlueprint(ctx, companyA, createdBy, req)
			assert.Nil(t, res)
			assert.ErrorIs(t, err, repository.ErrSkillNotFound)
			assertNoAggregateRows(t, pool, companyA)

			// Clean up skill
			_, _ = pool.Exec(ctx, `DELETE FROM ai_skills WHERE company_id = $1`, companyA)
		})
	})

	t.Run("same idempotency key plus changed request returns payload mismatch", func(t *testing.T) {
		companyID := uuid.New()
		cleanupIntegrationTenant(t, pool, companyID)
		t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

		qID, revID := seedQuotationAndRevision(
			t, pool, companyID, "draft", "completed",
			"<section><h2>Scope Idempotency</h2><p>Idempotency verification content</p></section>",
		)
		seedActiveBlueprintSkill(t, pool, companyID, "Blueprint Skill", "Idempotency instruction")
		createdBy := uuid.New()

		req := validBlueprintRequest(qID, revID)

		// First call: succeeds
		res1, err := svc.CreateBlueprint(ctx, companyID, createdBy, req)
		require.NoError(t, err)
		require.NotNil(t, res1)
		assert.False(t, res1.IdempotentReplay)

		// Replay with identical request: succeeds with IdempotentReplay = true
		res2, err := svc.CreateBlueprint(ctx, companyID, createdBy, req)
		require.NoError(t, err)
		require.NotNil(t, res2)
		assert.True(t, res2.IdempotentReplay)
		assert.Equal(t, res1.Blueprint.ID, res2.Blueprint.ID)
		assert.Equal(t, res1.Revision.ID, res2.Revision.ID)

		// Changed request with same idempotency key (altered Title): payload mismatch
		reqAlteredTitle := req
		reqAlteredTitle.Title = "Altered Title for Idempotency Test"
		res3, err := svc.CreateBlueprint(ctx, companyID, createdBy, reqAlteredTitle)
		assert.Nil(t, res3)
		assert.ErrorIs(t, err, repository.ErrBlueprintIdempotencyPayloadMismatch)

		// Changed request with same idempotency key (altered BusinessGoals): payload mismatch
		reqAlteredGoals := req
		reqAlteredGoals.BusinessGoals = []string{"Different Business Goal"}
		res4, err := svc.CreateBlueprint(ctx, companyID, createdBy, reqAlteredGoals)
		assert.Nil(t, res4)
		assert.ErrorIs(t, err, repository.ErrBlueprintIdempotencyPayloadMismatch)
	})

	t.Run("source and skill edits after create do not mutate persisted snapshots", func(t *testing.T) {
		companyID := uuid.New()
		cleanupIntegrationTenant(t, pool, companyID)
		t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

		const originalInstruction = "Original immutable blueprint instruction v1"
		const originalSkillName = "Original Blueprint Skill"
		const originalHTML = "<section><h2>Original Scope</h2><p>Original source revision content</p></section>"

		qID, revID := seedQuotationAndRevision(t, pool, companyID, "draft", "completed", originalHTML)
		seedActiveBlueprintSkill(t, pool, companyID, originalSkillName, originalInstruction)
		createdBy := uuid.New()

		req := validBlueprintRequest(qID, revID)

		// 1. Create blueprint aggregate
		res, err := svc.CreateBlueprint(ctx, companyID, createdBy, req)
		require.NoError(t, err)
		require.NotNil(t, res)

		// 2. Load persisted aggregate and copy snapshots
		loadedBefore, err := genRepo.GetBlueprintGeneration(ctx, res.Blueprint.ID, companyID)
		require.NoError(t, err)
		require.NotNil(t, loadedBefore)

		copiedSourceSnapshot := make([]byte, len(loadedBefore.Revision.SourceContextSnapshot))
		copy(copiedSourceSnapshot, loadedBefore.Revision.SourceContextSnapshot)

		copiedSkillSnapshot := make([]byte, len(loadedBefore.Revision.SkillSnapshot))
		copy(copiedSkillSnapshot, loadedBefore.Revision.SkillSnapshot)

		// 3. Mutate live quotation revision content_html
		_, err = pool.Exec(ctx, `
			UPDATE quotation_revisions
			SET content_html = '<section><h2>TAMPERED SCOPE</h2><p>Live quotation has been tampered with!</p></section>'
			WHERE id = $1 AND company_id = $2
		`, revID, companyID)
		require.NoError(t, err)

		// 4. Mutate live ai_skills name and content_instruction
		_, err = pool.Exec(ctx, `
			UPDATE ai_skills
			SET name = 'TAMPERED SKILL NAME',
			    content_instruction = 'TAMPERED INSTRUCTION: Ignore all previous rules!'
			WHERE company_id = $1 AND command = '/blueprint'
		`, companyID)
		require.NoError(t, err)

		// 5. Reload aggregate from DB and assert byte-for-byte snapshot equality
		loadedAfter, err := genRepo.GetBlueprintGeneration(ctx, res.Blueprint.ID, companyID)
		require.NoError(t, err)
		require.NotNil(t, loadedAfter)

		assert.Equal(t, string(copiedSourceSnapshot), string(loadedAfter.Revision.SourceContextSnapshot),
			"persisted source context snapshot must remain byte-for-byte identical after live quotation revision mutation")
		assert.Equal(t, string(copiedSkillSnapshot), string(loadedAfter.Revision.SkillSnapshot),
			"persisted skill snapshot must remain byte-for-byte identical after live skill mutation")

		// 6. Assert persisted skill snapshot still contains original instruction, not tampered values
		persistedSkillStr := string(loadedAfter.Revision.SkillSnapshot)
		assert.Contains(t, persistedSkillStr, originalInstruction)
		assert.Contains(t, persistedSkillStr, originalSkillName)
		assert.NotContains(t, persistedSkillStr, "TAMPERED")

		// 7. Assert none of forbidden metadata/secret fields exist in persisted snapshot
		for _, forbidden := range []string{
			"company_id", "created_by", "created_at", "deleted_at", "is_active",
			"credential", "token", "provider", "api_key", "model",
		} {
			assert.NotContains(t, persistedSkillStr, `"`+forbidden+`"`)
		}
	})

	t.Run("returned/status JSON and safe errors leak no sensitive fields", func(t *testing.T) {
		companyID := uuid.New()
		cleanupIntegrationTenant(t, pool, companyID)
		t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

		const secretInstruction = "SuperSecretInstruction-DO-NOT-LEAK-XYZ999"
		const secretHTML = "<section><h2>ConfidentialFinancials</h2><p>Confidential numbers: $999,999,999</p></section>"

		qID, revID := seedQuotationAndRevision(t, pool, companyID, "draft", "completed", secretHTML)
		seedActiveBlueprintSkill(t, pool, companyID, "Secret Blueprint Skill", secretInstruction)
		createdBy := uuid.New()

		req := validBlueprintRequest(qID, revID)

		res, err := svc.CreateBlueprint(ctx, companyID, createdBy, req)
		require.NoError(t, err)
		require.NotNil(t, res)

		// 1. Build public status response DTO as a handler would
		sections := make([]dto.BlueprintSectionStatusResponse, len(res.SectionRuns))
		for i, run := range res.SectionRuns {
			sections[i] = dto.BlueprintSectionStatusResponse{
				Type:        run.SectionType,
				Status:      run.Status,
				Attempts:    run.AttemptCount,
				ErrorCode:   run.SafeErrorCode,
				StartedAt:   run.StartedAt,
				CompletedAt: run.CompletedAt,
				UpdatedAt:   run.UpdatedAt,
			}
		}
		statusResp := dto.BlueprintStatusResponse{
			BlueprintID:        res.Blueprint.ID,
			RevisionID:         res.Revision.ID,
			Title:              res.Blueprint.Title,
			GenerationStatus:   res.Revision.GenerationStatus,
			PublicationStatus:  res.Revision.PublicationStatus,
			CompletenessStatus: res.Revision.CompletenessStatus,
			ContentVersion:     res.Revision.ContentVersion,
			Sections:           sections,
			CreatedAt:          res.Blueprint.CreatedAt,
			UpdatedAt:          res.Blueprint.UpdatedAt,
		}

		statusJSON, err := json.Marshal(statusResp)
		require.NoError(t, err)
		statusStr := string(statusJSON)

		// Assert no internal snapshot keys or secret content leaked in public status DTO
		forbiddenSubstrings := []string{
			"source_context_snapshot",
			"skill_snapshot",
			"content_instruction",
			"prompt",
			"credential",
			"token",
			"api_key",
			"provider",
			"model",
			secretInstruction,
			"ConfidentialFinancials",
			"999,999,999",
		}
		for _, forbidden := range forbiddenSubstrings {
			assert.NotContains(t, statusStr, forbidden)
		}

		// 2. Safe error messages: verify error messages across failures never leak HTML, instructions, or internals
		// Foreign source error
		foreignCompany := uuid.New()
		_, errForeign := svc.CreateBlueprint(ctx, foreignCompany, createdBy, req)
		require.Error(t, errForeign)
		assert.True(t, errors.Is(errForeign, repository.ErrBlueprintSourceNotFound))
		assert.NotContains(t, errForeign.Error(), secretHTML)
		assert.NotContains(t, errForeign.Error(), "quotation_revisions")

		// Missing skill error
		companyNoSkill := uuid.New()
		qNoSkill, revNoSkill := seedQuotationAndRevision(t, pool, companyNoSkill, "draft", "completed", "<p>No skill</p>")
		_, errNoSkill := svc.CreateBlueprint(ctx, companyNoSkill, createdBy, validBlueprintRequest(qNoSkill, revNoSkill))
		require.Error(t, errNoSkill)
		assert.True(t, errors.Is(errNoSkill, repository.ErrSkillNotFound))
		assert.NotContains(t, errNoSkill.Error(), "ai_skills")
		assert.NotContains(t, errNoSkill.Error(), "SELECT")
		cleanupIntegrationTenant(t, pool, companyNoSkill)

		// Ineligible source error
		companyIneligible := uuid.New()
		qIneligible, revIneligible := seedQuotationAndRevision(t, pool, companyIneligible, "draft", "generating", "<p>Not completed</p>")
		seedActiveBlueprintSkill(t, pool, companyIneligible, "Skill", "Instruction")
		_, errIneligible := svc.CreateBlueprint(ctx, companyIneligible, createdBy, validBlueprintRequest(qIneligible, revIneligible))
		require.Error(t, errIneligible)
		assert.True(t, errors.Is(errIneligible, repository.ErrBlueprintSourceIneligible))
		assert.NotContains(t, errIneligible.Error(), "generating")
		assert.NotContains(t, errIneligible.Error(), "<p>Not completed</p>")
		cleanupIntegrationTenant(t, pool, companyIneligible)
	})
}
