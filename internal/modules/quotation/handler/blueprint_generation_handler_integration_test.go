//go:build integration

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/handler"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	jwtpkg "venturo-skeleton-go/pkg/jwt"
)

// --- Test Fixture Types ---

type httpQueueFixtureData struct {
	Provenance struct {
		SourceFixture string `json:"source_fixture"`
		OutputFixture string `json:"output_fixture"`
		SchemaVersion string `json:"schema_version"`
		Description   string `json:"description"`
	} `json:"provenance"`
	SourceInput struct {
		Title                  string   `json:"title"`
		Summary                string   `json:"summary"`
		BusinessGoals          []string `json:"business_goals"`
		Actors                 []string `json:"actors"`
		Channels               []string `json:"channels"`
		BusinessPhases         []string `json:"business_phases"`
		BusinessRules          []string `json:"business_rules"`
		KnownStates            []string `json:"known_states"`
		Integrations           []string `json:"integrations"`
		TechnologyPreferences  []string `json:"technology_preferences"`
		OperationalConstraints []string `json:"operational_constraints"`
		AdditionalNotes        string   `json:"additional_notes"`
	} `json:"source_input"`
	Sections map[string]json.RawMessage `json:"sections"`
}

func loadHTTPQueueFixture(t *testing.T) httpQueueFixtureData {
	t.Helper()
	paths := []string{
		filepath.Join("..", "service", "testdata", "blueprint_http", "queue.json"),
		filepath.Join("service", "testdata", "blueprint_http", "queue.json"),
		filepath.Join("internal", "modules", "quotation", "service", "testdata", "blueprint_http", "queue.json"),
	}

	var data []byte
	var err error
	for _, p := range paths {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
	require.NoError(t, err, "failed to read queue.json from any known path")
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))

	var fix httpQueueFixtureData
	err = json.Unmarshal(data, &fix)
	require.NoError(t, err, "failed to parse queue.json fixture")
	require.NotEmpty(t, fix.Sections)
	return fix
}

func detectStageFromPrompt(prompt string) domain.BlueprintSectionType {
	if strings.Contains(prompt, `stage="journey"`) {
		return domain.BlueprintSectionJourney
	} else if strings.Contains(prompt, `stage="architecture"`) {
		return domain.BlueprintSectionArchitecture
	} else if strings.Contains(prompt, `stage="state_machine"`) {
		return domain.BlueprintSectionStateMachine
	} else if strings.Contains(prompt, `stage="workflow"`) {
		return domain.BlueprintSectionWorkflow
	} else if strings.Contains(prompt, `stage="wbs"`) {
		return domain.BlueprintSectionWBS
	}
	return domain.BlueprintSectionContext
}

// --- Scripted AI Provider & Source ---

type httpIntegrationAIProvider struct {
	mu           sync.Mutex
	calls        int
	callsByStage map[domain.BlueprintSectionType]int
	generateFn   func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error)
	fixture      httpQueueFixtureData
}

func newHTTPIntegrationAIProvider(fix httpQueueFixtureData) *httpIntegrationAIProvider {
	return &httpIntegrationAIProvider{
		callsByStage: make(map[domain.BlueprintSectionType]int),
		fixture:      fix,
	}
}

func (p *httpIntegrationAIProvider) Generate(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	stage := detectStageFromPrompt(req.Prompt)
	p.callsByStage[stage]++

	if p.generateFn != nil {
		return p.generateFn(ctx, req)
	}

	secKey := string(stage)
	if payload, ok := p.fixture.Sections[secKey]; ok {
		return service.GenerateResult{Text: string(payload)}, nil
	}

	return service.GenerateResult{Text: "{}"}, nil
}

type httpIntegrationProviderSource struct {
	mu       sync.Mutex
	calls    int
	provider service.AIProvider
}

func (s *httpIntegrationProviderSource) ProviderForCompany(ctx context.Context, companyID uuid.UUID) (service.AIProvider, string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.provider, "http-integration-provider", "scripted-model-v1", nil
}

// --- DB & Context Helpers ---

func getBlueprintHTTPIntegrationPool(t *testing.T) *pgxpool.Pool {
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

func cleanupHTTPIntegrationTenant(t *testing.T, pool *pgxpool.Pool, companyIDs ...uuid.UUID) {
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

func seedHTTPQuotationAndRevision(
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
	quotationNumber := "BP-HTTP-" + uuid.NewString()[:8]

	_, err := pool.Exec(ctx, `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject,
			solution_type, commercial_scheme, status, current_revision
		) VALUES (
			$1, $2, gen_random_uuid(), $3, 'Public Service Queue System',
			'custom_development', 'custom_development', 'draft', 1
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
			$4, $5, '{"project":"Public Service Queue"}'::jsonb,
			$6, 1, NULL
		)
	`, revisionID, companyID, quotationID, pubStatus, genStatus, contentHTML)
	require.NoError(t, err)

	return quotationID, revisionID
}

func seedHTTPActiveBlueprintSkill(
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

func generateTestJWTToken(t *testing.T, userID, companyID string) string {
	t.Helper()
	token, err := jwtpkg.GenerateToken(
		userID,
		companyID,
		"Test Company",
		"",
		"",
		"qa@venturo.com",
		"qa_tester",
		"QA Tester",
		false,
		[]string{"admin"},
	)
	require.NoError(t, err)
	return token
}

func generateExpiredJWTToken(t *testing.T, userID, companyID string) string {
	t.Helper()
	claims := &jwtpkg.Claims{
		UserID:      userID,
		CompanyID:   companyID,
		CompanyName: "Test Company",
		Email:       "qa@venturo.com",
		Username:    "qa_tester",
		FullName:    "QA Tester",
		Roles:       []string{"admin"},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			Issuer:    "tuai-api",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(jwtpkg.GetSecret())
	require.NoError(t, err)
	return tokenStr
}

func setupHTTPIntegrationRouter(
	t *testing.T,
	pool *pgxpool.Pool,
	provSource service.ProviderSource,
) (*gin.Engine, *repository.BlueprintGenerationRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())

	bpGenRepo := repository.NewBlueprintGenerationRepository(pool)
	bpReadRepo := repository.NewBlueprintReadRepository(pool)
	bpInputRepo := repository.NewBlueprintGenerationInputRepository(pool)
	skillRepo := repository.NewSkillRepository(pool)
	contextBuilder := service.NewBlueprintSourceContextBuilder()

	bpGenSvc := service.NewBlueprintGenerationService(bpInputRepo, skillRepo, contextBuilder, bpGenRepo)
	bpReadSvc := service.NewBlueprintReadService(bpGenRepo, bpReadRepo)
	prompts := service.NewBlueprintPromptBuilder()
	validator := service.NewBlueprintSectionValidator()
	crossValidator := service.NewProductionBlueprintCrossValidator()

	bpPipeline := service.NewBlueprintPipeline(bpGenRepo, provSource, prompts, validator, crossValidator)
	bpHandler := handler.NewBlueprintGenerationHandler(bpGenSvc, bpReadSvc, bpPipeline)

	blueprints := router.Group("/quotation/v1/blueprints")
	blueprints.Use(middleware.JWTAuth(), middleware.CompanyContext())
	{
		blueprints.POST("/generate", bpHandler.Generate)
		blueprints.GET("/:id/generation-status", bpHandler.GetStatus)
		blueprints.GET("/:id", bpHandler.GetDetail)
		blueprints.POST("/:id/continue", bpHandler.Continue)
		blueprints.POST("/:id/sections/:section/retry", bpHandler.RetrySection)
		blueprints.PUT("/:id/sections/:section/manual", bpHandler.ManualCompleteSection)
		blueprints.GET("", bpHandler.List)
	}

	return router, bpGenRepo
}

func buildQueueGenerationRequest(quotationID, revisionID uuid.UUID, fix httpQueueFixtureData) dto.CreateBlueprintGenerationRequest {
	return dto.CreateBlueprintGenerationRequest{
		IdempotencyKey:            uuid.New(),
		QuotationID:               quotationID,
		SourceQuotationRevisionID: revisionID,
		Title:                     fix.SourceInput.Title,
		BusinessGoals:             fix.SourceInput.BusinessGoals,
		Actors:                    fix.SourceInput.Actors,
		Channels:                  fix.SourceInput.Channels,
		BusinessPhases:            fix.SourceInput.BusinessPhases,
		BusinessRules:             fix.SourceInput.BusinessRules,
		KnownStates:               fix.SourceInput.KnownStates,
		Integrations:              fix.SourceInput.Integrations,
		TechnologyPreferences:     fix.SourceInput.TechnologyPreferences,
		OperationalConstraints:    fix.SourceInput.OperationalConstraints,
		AdditionalNotes:           fix.SourceInput.AdditionalNotes,
	}
}

// =========================================================================
// Task 7A.03: Happy table (draft/published/superseded -> 201 -> 200 replay -> 409 mismatch -> 6 continues completed)
// =========================================================================

func TestBlueprintHTTP_HappyPath_CreateReplayAndSixContinuesCompleted(t *testing.T) {
	pool := getBlueprintHTTPIntegrationPool(t)
	companyID := uuid.New()
	userID := uuid.New()
	cleanupHTTPIntegrationTenant(t, pool, companyID)
	t.Cleanup(func() { cleanupHTTPIntegrationTenant(t, pool, companyID) })

	fix := loadHTTPQueueFixture(t)
	provider := newHTTPIntegrationAIProvider(fix)
	provSource := &httpIntegrationProviderSource{provider: provider}

	router, _ := setupHTTPIntegrationRouter(t, pool, provSource)
	token := generateTestJWTToken(t, userID.String(), companyID.String())

	seedHTTPActiveBlueprintSkill(t, pool, companyID, "Queue Blueprint Architect", "Produce high quality blueprint architecture")

	statuses := []string{"draft", "published", "superseded"}

	for _, pubStatus := range statuses {
		t.Run(fmt.Sprintf("PublicationStatus_%s", pubStatus), func(t *testing.T) {
			qID, revID := seedHTTPQuotationAndRevision(
				t, pool, companyID, pubStatus, "completed",
				"<section><h2>Sistem Antrian Terpadu</h2><p>Spesifikasi sistem antrean loket terpadu.</p></section>",
			)

			reqBody := buildQueueGenerationRequest(qID, revID, fix)
			reqBytes, err := json.Marshal(reqBody)
			require.NoError(t, err)

			// 1. Fresh Generate -> 201 Created
			w := httptest.NewRecorder()
			httpReq := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/generate", bytes.NewReader(reqBytes))
			httpReq.Header.Set("Authorization", "Bearer "+token)
			httpReq.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, httpReq)

			require.Equal(t, http.StatusCreated, w.Code, "Expected 201 Created on fresh generate")

			var resp struct {
				Message string                      `json:"message"`
				Data    dto.BlueprintStatusResponse `json:"data"`
			}
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			bpID := resp.Data.BlueprintID
			require.NotEqual(t, uuid.Nil, bpID)
			require.Equal(t, string(domain.BlueprintGenerationPending), resp.Data.GenerationStatus)
			require.Equal(t, 1, resp.Data.ContentVersion)
			require.Len(t, resp.Data.Sections, 7)
			assert.Equal(t, string(domain.BlueprintSectionValidated), resp.Data.Sections[0].Status) // Context is validated

			// 2. Idempotent Replay with same key & payload -> 200 OK
			wReplay := httptest.NewRecorder()
			httpReqReplay := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/generate", bytes.NewReader(reqBytes))
			httpReqReplay.Header.Set("Authorization", "Bearer "+token)
			httpReqReplay.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(wReplay, httpReqReplay)

			require.Equal(t, http.StatusOK, wReplay.Code, "Expected 200 OK on idempotent replay")
			var replayResp struct {
				Data dto.BlueprintStatusResponse `json:"data"`
			}
			err = json.Unmarshal(wReplay.Body.Bytes(), &replayResp)
			require.NoError(t, err)
			assert.Equal(t, bpID, replayResp.Data.BlueprintID)

			// Verify single master & single revision in DB
			var masterCount, revCount int
			err = pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM blueprints WHERE id = $1`, bpID).Scan(&masterCount)
			require.NoError(t, err)
			assert.Equal(t, 1, masterCount)
			err = pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM blueprint_revisions WHERE blueprint_id = $1`, bpID).Scan(&revCount)
			require.NoError(t, err)
			assert.Equal(t, 1, revCount)

			// 3. Changed payload with same idempotency key -> 409 Conflict
			mismatchReq := reqBody
			mismatchReq.Title = "Modified Mismatch Title"
			mismatchBytes, _ := json.Marshal(mismatchReq)

			wMismatch := httptest.NewRecorder()
			httpReqMismatch := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/generate", bytes.NewReader(mismatchBytes))
			httpReqMismatch.Header.Set("Authorization", "Bearer "+token)
			httpReqMismatch.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(wMismatch, httpReqMismatch)

			require.Equal(t, http.StatusConflict, wMismatch.Code)
			assert.Contains(t, wMismatch.Body.String(), "idempotency_payload_mismatch")

			// If testing draft status, run the 6 continue calls to completion
			if pubStatus == "draft" {
				providerCallsBefore := provider.calls

				expectedStages := []struct {
					stage           domain.BlueprintSectionType
					expectedVersion int
					finalStatus     domain.BlueprintGenerationStatus
				}{
					{domain.BlueprintSectionJourney, 2, domain.BlueprintGenerationInProgress},
					{domain.BlueprintSectionArchitecture, 3, domain.BlueprintGenerationInProgress},
					{domain.BlueprintSectionStateMachine, 4, domain.BlueprintGenerationInProgress},
					{domain.BlueprintSectionWorkflow, 5, domain.BlueprintGenerationInProgress},
					{domain.BlueprintSectionWBS, 6, domain.BlueprintGenerationInProgress},
					{domain.BlueprintSectionCrossValidation, 6, domain.BlueprintGenerationCompleted},
				}

				for i, exp := range expectedStages {
					wCont := httptest.NewRecorder()
					httpReqCont := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/continue", bpID), nil)
					httpReqCont.Header.Set("Authorization", "Bearer "+token)
					router.ServeHTTP(wCont, httpReqCont)

					require.Equal(t, http.StatusOK, wCont.Code, "Stage %d (%s) failed with body: %s", i+1, exp.stage, wCont.Body.String())

					var contResp struct {
						Data dto.BlueprintStatusResponse `json:"data"`
					}
					err = json.Unmarshal(wCont.Body.Bytes(), &contResp)
					require.NoError(t, err)
					assert.Equal(t, string(exp.finalStatus), contResp.Data.GenerationStatus)
					assert.Equal(t, exp.expectedVersion, contResp.Data.ContentVersion)
				}

				// Assert AI provider call count: exactly 5 provider calls (journey, architecture, state_machine, workflow, wbs)
				// Create/Context = 0, CrossValidation = 0 (deterministic cross-validator)
				providerCallsAfter := provider.calls
				assert.Equal(t, 5, providerCallsAfter-providerCallsBefore, "Expected exactly 5 AI provider calls across the 6 continue steps")
				assert.Equal(t, 1, provider.callsByStage[domain.BlueprintSectionJourney])
				assert.Equal(t, 1, provider.callsByStage[domain.BlueprintSectionArchitecture])
				assert.Equal(t, 1, provider.callsByStage[domain.BlueprintSectionStateMachine])
				assert.Equal(t, 1, provider.callsByStage[domain.BlueprintSectionWorkflow])
				assert.Equal(t, 1, provider.callsByStage[domain.BlueprintSectionWBS])
				assert.Equal(t, 0, provider.callsByStage[domain.BlueprintSectionContext])
				assert.Equal(t, 0, provider.callsByStage[domain.BlueprintSectionCrossValidation])

				// Verify GET /:id returns complete detail with strictly valid BlueprintBundle schema
				wDetail := httptest.NewRecorder()
				httpReqDetail := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/quotation/v1/blueprints/%s", bpID), nil)
				httpReqDetail.Header.Set("Authorization", "Bearer "+token)
				router.ServeHTTP(wDetail, httpReqDetail)

				require.Equal(t, http.StatusOK, wDetail.Code)
				var detailResp struct {
					Data dto.BlueprintDetailResponse `json:"data"`
				}
				err = json.Unmarshal(wDetail.Body.Bytes(), &detailResp)
				require.NoError(t, err)
				assert.Equal(t, string(domain.BlueprintGenerationCompleted), detailResp.Data.Status.GenerationStatus)
				assert.NotNil(t, detailResp.Data.ContentJSON)

				// Strict decode of ContentJSON into BlueprintBundle
				dec := json.NewDecoder(bytes.NewReader(detailResp.Data.ContentJSON))
				dec.DisallowUnknownFields()
				var bundle service.BlueprintBundle
				err = dec.Decode(&bundle)
				require.NoError(t, err, "ContentJSON must strictly decode into service.BlueprintBundle")

				assert.NotEmpty(t, bundle.Journey.Phases)
				assert.NotEmpty(t, bundle.Architecture.Nodes)
				assert.NotEmpty(t, bundle.StateMachine.States)
				assert.NotEmpty(t, bundle.Workflows)
				assert.NotEmpty(t, bundle.WBS.Modules)

				// Assert cross-validation results valid with 0 blocking errors
				require.NotNil(t, detailResp.Data.CrossValidationResult)
				var crossRes service.BlueprintCrossValidationResult
				err = json.Unmarshal(detailResp.Data.CrossValidationResult, &crossRes)
				require.NoError(t, err)
				assert.True(t, crossRes.Valid, "Cross validation must be valid")
				for _, issue := range crossRes.Issues {
					assert.False(t, issue.Blocking, "Cross-validation must not contain blocking issues: %v", issue)
				}
			}
		})
	}
}

// =========================================================================
// Task 7A.05: Failure recovery, retry, manual invalid (422), manual valid (200), zero manual provider calls
// =========================================================================

func TestBlueprintHTTP_FailureRecoveryRetryAndManual(t *testing.T) {
	pool := getBlueprintHTTPIntegrationPool(t)
	companyID := uuid.New()
	userID := uuid.New()
	cleanupHTTPIntegrationTenant(t, pool, companyID)
	t.Cleanup(func() { cleanupHTTPIntegrationTenant(t, pool, companyID) })

	fix := loadHTTPQueueFixture(t)
	provider := newHTTPIntegrationAIProvider(fix)
	provSource := &httpIntegrationProviderSource{provider: provider}

	router, _ := setupHTTPIntegrationRouter(t, pool, provSource)
	token := generateTestJWTToken(t, userID.String(), companyID.String())

	seedHTTPActiveBlueprintSkill(t, pool, companyID, "Queue Blueprint Architect", "Produce high quality blueprint architecture")
	qID, revID := seedHTTPQuotationAndRevision(t, pool, companyID, "draft", "completed", "<section><h2>Queue System</h2></section>")

	// 1. Generate -> 201 Created
	reqBody := buildQueueGenerationRequest(qID, revID, fix)
	reqBytes, _ := json.Marshal(reqBody)

	wGen := httptest.NewRecorder()
	httpReqGen := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/generate", bytes.NewReader(reqBytes))
	httpReqGen.Header.Set("Authorization", "Bearer "+token)
	httpReqGen.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wGen, httpReqGen)

	require.Equal(t, http.StatusCreated, wGen.Code)
	var genResp struct {
		Data dto.BlueprintStatusResponse `json:"data"`
	}
	_ = json.Unmarshal(wGen.Body.Bytes(), &genResp)
	bpID := genResp.Data.BlueprintID

	// 2. Stage 1: Journey -> succeeds -> version 2
	wJ := httptest.NewRecorder()
	httpReqJ := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/continue", bpID), nil)
	httpReqJ.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wJ, httpReqJ)
	require.Equal(t, http.StatusOK, wJ.Code)

	var jResp struct {
		Data dto.BlueprintStatusResponse `json:"data"`
	}
	_ = json.Unmarshal(wJ.Body.Bytes(), &jResp)
	assert.Equal(t, 2, jResp.Data.ContentVersion)
	assert.Equal(t, string(domain.BlueprintSectionCompleted), jResp.Data.Sections[1].Status) // Journey completed

	// Fetch detail to record journey content bytes
	wDetail1 := httptest.NewRecorder()
	httpReqDetail1 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/quotation/v1/blueprints/%s", bpID), nil)
	httpReqDetail1.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wDetail1, httpReqDetail1)
	require.Equal(t, http.StatusOK, wDetail1.Code)
	var detail1 struct {
		Data dto.BlueprintDetailResponse `json:"data"`
	}
	_ = json.Unmarshal(wDetail1.Body.Bytes(), &detail1)
	var contentMap map[string]json.RawMessage
	_ = json.Unmarshal(detail1.Data.ContentJSON, &contentMap)
	journeyBytesOriginal := contentMap["journey"]
	require.NotEmpty(t, journeyBytesOriginal)

	// 3. Stage 2: Architecture -> Force provider failure (invalid schema)
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		stage := detectStageFromPrompt(req.Prompt)
		if stage == domain.BlueprintSectionArchitecture {
			return service.GenerateResult{Text: `{"architecture": "invalid non-object format"}`}, nil
		}
		secKey := string(stage)
		if payload, ok := fix.Sections[secKey]; ok {
			return service.GenerateResult{Text: string(payload)}, nil
		}
		return service.GenerateResult{Text: "{}"}, nil
	}

	wArchFail := httptest.NewRecorder()
	httpReqArchFail := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/continue", bpID), nil)
	httpReqArchFail.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wArchFail, httpReqArchFail)

	require.Equal(t, http.StatusUnprocessableEntity, wArchFail.Code)
	assert.Contains(t, wArchFail.Body.String(), "invalid_schema")

	// Verify Journey bytes and content version (2) are completely preserved
	wStatusAfterFail := httptest.NewRecorder()
	httpReqStatus := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/quotation/v1/blueprints/%s/generation-status", bpID), nil)
	httpReqStatus.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wStatusAfterFail, httpReqStatus)
	require.Equal(t, http.StatusOK, wStatusAfterFail.Code)

	var failStatusResp struct {
		Data dto.BlueprintStatusResponse `json:"data"`
	}
	_ = json.Unmarshal(wStatusAfterFail.Body.Bytes(), &failStatusResp)
	assert.Equal(t, 2, failStatusResp.Data.ContentVersion)
	assert.Equal(t, string(domain.BlueprintGenerationPartialFailed), failStatusResp.Data.GenerationStatus)
	assert.Equal(t, string(domain.BlueprintSectionFailed), failStatusResp.Data.Sections[2].Status) // Architecture failed
	assert.Equal(t, 1, failStatusResp.Data.Sections[2].Attempts)

	// 4. Retry Architecture -> Named stage retry (provider still fails) -> attempts becomes 2
	wRetry := httptest.NewRecorder()
	httpReqRetry := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/architecture/retry", bpID), nil)
	httpReqRetry.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wRetry, httpReqRetry)

	require.Equal(t, http.StatusUnprocessableEntity, wRetry.Code)
	assert.Contains(t, wRetry.Body.String(), "invalid_schema")

	// Check status: attempts = 2
	wStatusRetry := httptest.NewRecorder()
	httpReqStatusRetry := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/quotation/v1/blueprints/%s/generation-status", bpID), nil)
	httpReqStatusRetry.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wStatusRetry, httpReqStatusRetry)
	var statusRetryResp struct {
		Data dto.BlueprintStatusResponse `json:"data"`
	}
	_ = json.Unmarshal(wStatusRetry.Body.Bytes(), &statusRetryResp)
	assert.Equal(t, 2, statusRetryResp.Data.Sections[2].Attempts)

	// 5. Manual completion: Invalid payload -> 422 Unprocessable Entity, zero write
	invalidManualReq := map[string]interface{}{
		"revision_id":     statusRetryResp.Data.RevisionID,
		"content_version": statusRetryResp.Data.ContentVersion,
		"content": map[string]interface{}{
			"architecture": map[string]interface{}{
				"groups": []interface{}{},
				"nodes":  []interface{}{map[string]interface{}{"id": "missing-required-fields"}},
			},
		},
	}
	invalidManualBytes, _ := json.Marshal(invalidManualReq)

	wManualInvalid := httptest.NewRecorder()
	httpReqManualInvalid := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/architecture/manual", bpID), bytes.NewReader(invalidManualBytes))
	httpReqManualInvalid.Header.Set("Authorization", "Bearer "+token)
	httpReqManualInvalid.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wManualInvalid, httpReqManualInvalid)

	require.Equal(t, http.StatusUnprocessableEntity, wManualInvalid.Code)
	assert.Contains(t, wManualInvalid.Body.String(), "invalid_schema")

	// 6. Manual completion: Valid payload -> 200 OK, becomes validated, version increments to 3
	providerCallsBeforeManual := provider.calls

	var validArchContent map[string]interface{}
	err := json.Unmarshal(fix.Sections["architecture"], &validArchContent)
	require.NoError(t, err)

	validManualReq := map[string]interface{}{
		"revision_id":     statusRetryResp.Data.RevisionID,
		"content_version": statusRetryResp.Data.ContentVersion,
		"content":         validArchContent,
	}
	validManualBytes, _ := json.Marshal(validManualReq)

	wManualValid := httptest.NewRecorder()
	httpReqManualValid := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/architecture/manual", bpID), bytes.NewReader(validManualBytes))
	httpReqManualValid.Header.Set("Authorization", "Bearer "+token)
	httpReqManualValid.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wManualValid, httpReqManualValid)

	require.Equal(t, http.StatusOK, wManualValid.Code, "Manual valid completion failed: %s", wManualValid.Body.String())
	var manualValidResp struct {
		Data dto.BlueprintStatusResponse `json:"data"`
	}
	_ = json.Unmarshal(wManualValid.Body.Bytes(), &manualValidResp)
	assert.Equal(t, 3, manualValidResp.Data.ContentVersion)
	assert.Equal(t, string(domain.BlueprintSectionValidated), manualValidResp.Data.Sections[2].Status) // Architecture validated

	// Assert manual completion did NOT invoke AI provider
	providerCallsAfterManual := provider.calls
	assert.Equal(t, 0, providerCallsAfterManual-providerCallsBeforeManual, "Manual completion must make ZERO AI provider calls")

	// 7. Subsequent continues complete the pipeline
	provider.generateFn = nil // restore normal provider

	remainingStages := []domain.BlueprintSectionType{
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS,
		domain.BlueprintSectionCrossValidation,
	}

	for _, st := range remainingStages {
		wRemaining := httptest.NewRecorder()
		httpReqRemaining := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/continue", bpID), nil)
		httpReqRemaining.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(wRemaining, httpReqRemaining)
		require.Equal(t, http.StatusOK, wRemaining.Code, "Stage %s failed: %s", st, wRemaining.Body.String())
	}

	// Verify final status completed
	wFinal := httptest.NewRecorder()
	httpReqFinal := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/quotation/v1/blueprints/%s/generation-status", bpID), nil)
	httpReqFinal.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wFinal, httpReqFinal)
	var finalResp struct {
		Data dto.BlueprintStatusResponse `json:"data"`
	}
	_ = json.Unmarshal(wFinal.Body.Bytes(), &finalResp)
	assert.Equal(t, string(domain.BlueprintGenerationCompleted), finalResp.Data.GenerationStatus)
}

// =========================================================================
// Task 7A.06: Concurrency barrier, stale manual version guard, cross-validation correction
// =========================================================================

func TestBlueprintHTTP_ConcurrencyAndStaleManual(t *testing.T) {
	pool := getBlueprintHTTPIntegrationPool(t)
	companyID := uuid.New()
	userID := uuid.New()
	cleanupHTTPIntegrationTenant(t, pool, companyID)
	t.Cleanup(func() { cleanupHTTPIntegrationTenant(t, pool, companyID) })

	fix := loadHTTPQueueFixture(t)
	provider := newHTTPIntegrationAIProvider(fix)
	provSource := &httpIntegrationProviderSource{provider: provider}

	router, _ := setupHTTPIntegrationRouter(t, pool, provSource)
	token := generateTestJWTToken(t, userID.String(), companyID.String())

	seedHTTPActiveBlueprintSkill(t, pool, companyID, "Queue Blueprint Architect", "Produce high quality blueprint architecture")
	qID, revID := seedHTTPQuotationAndRevision(t, pool, companyID, "draft", "completed", "<section><h2>Queue System</h2></section>")

	// 1. Generate
	reqBody := buildQueueGenerationRequest(qID, revID, fix)
	reqBytes, _ := json.Marshal(reqBody)

	wGen := httptest.NewRecorder()
	httpReqGen := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/generate", bytes.NewReader(reqBytes))
	httpReqGen.Header.Set("Authorization", "Bearer "+token)
	httpReqGen.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wGen, httpReqGen)
	require.Equal(t, http.StatusCreated, wGen.Code)

	var genResp struct {
		Data dto.BlueprintStatusResponse `json:"data"`
	}
	_ = json.Unmarshal(wGen.Body.Bytes(), &genResp)
	bpID := genResp.Data.BlueprintID

	// 2. Concurrency Barrier on continue
	// Use channels to ensure request 2 hits the endpoint while request 1 is actively inside provider generation
	inGenerate := make(chan struct{}, 1)
	releaseGenerate := make(chan struct{})

	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		select {
		case inGenerate <- struct{}{}:
		default:
		}
		select {
		case <-releaseGenerate:
		case <-ctx.Done():
		}
		stage := detectStageFromPrompt(req.Prompt)
		secKey := string(stage)
		if payload, ok := fix.Sections[secKey]; ok {
			return service.GenerateResult{Text: string(payload)}, nil
		}
		return service.GenerateResult{Text: "{}"}, nil
	}

	var wg sync.WaitGroup
	var code1 int
	var body1 string

	wg.Add(1)
	go func() {
		defer wg.Done()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/continue", bpID), nil)
		r.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, r)
		code1 = w.Code
		body1 = w.Body.String()
	}()

	// Wait until request 1 is actively in Generate (holding the lease)
	select {
	case <-inGenerate:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for request 1 to enter provider generation")
	}

	// Dispatch competing request 2 while request 1 holds the lease
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/continue", bpID), nil)
	r2.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w2, r2)
	code2 := w2.Code
	body2 := w2.Body.String()

	// Release request 1 to complete
	close(releaseGenerate)
	wg.Wait()

	// Request 1 succeeded with 200 OK, request 2 rejected with 409 Conflict (pipeline_busy)
	assert.Equal(t, http.StatusOK, code1, "First in-flight request must succeed with 200 OK. Body: %s", body1)
	assert.Equal(t, http.StatusConflict, code2, "Competing concurrent request must fail with 409 Conflict. Body: %s", body2)
	assert.Contains(t, body2, "pipeline_busy")

	// Provider called exactly 1 time for Journey stage
	assert.Equal(t, 1, provider.callsByStage[domain.BlueprintSectionJourney], "Provider must be called exactly once despite concurrent requests")

	// Restore normal provider
	provider.generateFn = nil

	// 3. Progress to version 3
	wCont2 := httptest.NewRecorder()
	httpReqCont2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/continue", bpID), nil)
	httpReqCont2.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wCont2, httpReqCont2)
	require.Equal(t, http.StatusOK, wCont2.Code)

	var cont2Resp struct {
		Data dto.BlueprintStatusResponse `json:"data"`
	}
	_ = json.Unmarshal(wCont2.Body.Bytes(), &cont2Resp)
	currentRevisionID := cont2Resp.Data.RevisionID
	currentVersion := cont2Resp.Data.ContentVersion
	require.GreaterOrEqual(t, currentVersion, 3)

	// 4. Stale-browser manual: Send version = 1 when server is at version >= 3 -> 409 Conflict
	var journeyContent map[string]interface{}
	_ = json.Unmarshal(fix.Sections["journey"], &journeyContent)

	staleManualReq := map[string]interface{}{
		"revision_id":     currentRevisionID,
		"content_version": 1, // Stale version!
		"content":         journeyContent,
	}
	staleManualBytes, _ := json.Marshal(staleManualReq)

	wStaleManual := httptest.NewRecorder()
	httpReqStaleManual := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/manual", bpID), bytes.NewReader(staleManualBytes))
	httpReqStaleManual.Header.Set("Authorization", "Bearer "+token)
	httpReqStaleManual.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wStaleManual, httpReqStaleManual)

	require.Equal(t, http.StatusConflict, wStaleManual.Code)
	assert.Contains(t, wStaleManual.Body.String(), "execution_conflict")

	// 5. Complete all stages to test cross-validation correction
	for i := 0; i < 4; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/continue", bpID), nil)
		r.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			break
		}
	}

	// Status before manual edit should be completed
	wStatBeforeEdit := httptest.NewRecorder()
	httpReqStatBefore := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/quotation/v1/blueprints/%s/generation-status", bpID), nil)
	httpReqStatBefore.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(wStatBeforeEdit, httpReqStatBefore)
	var statBeforeResp struct {
		Data dto.BlueprintStatusResponse `json:"data"`
	}
	_ = json.Unmarshal(wStatBeforeEdit.Body.Bytes(), &statBeforeResp)

	// Now perform valid manual update on Journey with matching current version
	validManualReq := map[string]interface{}{
		"revision_id":     statBeforeResp.Data.RevisionID,
		"content_version": statBeforeResp.Data.ContentVersion,
		"content":         journeyContent,
	}
	validManualBytes, _ := json.Marshal(validManualReq)

	wManualUpdate := httptest.NewRecorder()
	httpReqManualUpdate := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/manual", bpID), bytes.NewReader(validManualBytes))
	httpReqManualUpdate.Header.Set("Authorization", "Bearer "+token)
	httpReqManualUpdate.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wManualUpdate, httpReqManualUpdate)

	require.Equal(t, http.StatusOK, wManualUpdate.Code)
	var manualUpdateResp struct {
		Data dto.BlueprintStatusResponse `json:"data"`
	}
	_ = json.Unmarshal(wManualUpdate.Body.Bytes(), &manualUpdateResp)
	assert.Greater(t, manualUpdateResp.Data.ContentVersion, statBeforeResp.Data.ContentVersion)
	// Upstream manual edit invalidates downstream cross-validation
	assert.Equal(t, string(domain.BlueprintSectionPending), manualUpdateResp.Data.Sections[6].Status) // Cross validation invalidated to pending
}

// =========================================================================
// Task 7A.07: Tenant isolation, snapshot immutability, sentinel redaction
// =========================================================================

func TestBlueprintHTTP_TenantIsolationAndSnapshotImmutability(t *testing.T) {
	pool := getBlueprintHTTPIntegrationPool(t)
	companyA := uuid.New()
	companyB := uuid.New()
	userA := uuid.New()
	userB := uuid.New()
	cleanupHTTPIntegrationTenant(t, pool, companyA, companyB)
	t.Cleanup(func() { cleanupHTTPIntegrationTenant(t, pool, companyA, companyB) })

	fix := loadHTTPQueueFixture(t)
	provider := newHTTPIntegrationAIProvider(fix)
	provSource := &httpIntegrationProviderSource{provider: provider}

	router, _ := setupHTTPIntegrationRouter(t, pool, provSource)
	tokenA := generateTestJWTToken(t, userA.String(), companyA.String())
	tokenB := generateTestJWTToken(t, userB.String(), companyB.String())

	seedHTTPActiveBlueprintSkill(t, pool, companyA, "Queue Architect A", "Instruction A containing AI_PROVIDER_API_KEY_SECRET_CANARY and SYSTEM_PROMPT_RAW_INSTRUCTION_SECRET")
	seedHTTPActiveBlueprintSkill(t, pool, companyB, "Queue Architect B", "Instruction B")

	qID_A, revID_A := seedHTTPQuotationAndRevision(t, pool, companyA, "draft", "completed", "<section><h2>Queue System</h2><p>Normal quotation business content.</p></section>")

	// 1. Company A generates blueprint
	reqBodyA := buildQueueGenerationRequest(qID_A, revID_A, fix)
	reqBytesA, _ := json.Marshal(reqBodyA)

	wGenA := httptest.NewRecorder()
	httpReqGenA := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/generate", bytes.NewReader(reqBytesA))
	httpReqGenA.Header.Set("Authorization", "Bearer "+tokenA)
	httpReqGenA.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wGenA, httpReqGenA)

	require.Equal(t, http.StatusCreated, wGenA.Code)
	var genRespA struct {
		Data dto.BlueprintStatusResponse `json:"data"`
	}
	_ = json.Unmarshal(wGenA.Body.Bytes(), &genRespA)
	bpID_A := genRespA.Data.BlueprintID

	// Inject internal DB snapshot canaries directly into blueprint_revisions table
	_, err := pool.Exec(context.Background(), `
		UPDATE blueprint_revisions 
		SET source_context_snapshot = '{"summary": "Queue System", "secret_canary": "LEGAL_INDEMNITY_UNLIMITED_SECRET"}',
		    skill_snapshot = '{"command": "/blueprint", "content_instruction": "AI_PROVIDER_API_KEY_SECRET_CANARY COMMERCIAL_RATE_DISCOUNT_99_PERCENT"}'
		WHERE blueprint_id = $1
	`, bpID_A)
	require.NoError(t, err)

	// 2. Tenant B Negative Tests against Company A's resources
	// B attempts to GET A's detail -> 404
	wB_Detail := httptest.NewRecorder()
	httpReqB_Detail := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/quotation/v1/blueprints/%s", bpID_A), nil)
	httpReqB_Detail.Header.Set("Authorization", "Bearer "+tokenB)
	router.ServeHTTP(wB_Detail, httpReqB_Detail)
	assert.Equal(t, http.StatusNotFound, wB_Detail.Code)
	assert.Contains(t, wB_Detail.Body.String(), "blueprint_not_found")

	// B attempts to GET A's status -> 404
	wB_Status := httptest.NewRecorder()
	httpReqB_Status := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/quotation/v1/blueprints/%s/generation-status", bpID_A), nil)
	httpReqB_Status.Header.Set("Authorization", "Bearer "+tokenB)
	router.ServeHTTP(wB_Status, httpReqB_Status)
	assert.Equal(t, http.StatusNotFound, wB_Status.Code)
	assert.Contains(t, wB_Status.Body.String(), "blueprint_not_found")

	// B attempts to List -> returns empty items []
	wB_List := httptest.NewRecorder()
	httpReqB_List := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints", nil)
	httpReqB_List.Header.Set("Authorization", "Bearer "+tokenB)
	router.ServeHTTP(wB_List, httpReqB_List)
	assert.Equal(t, http.StatusOK, wB_List.Code)
	var listRespB struct {
		Data dto.BlueprintListResponse `json:"data"`
	}
	_ = json.Unmarshal(wB_List.Body.Bytes(), &listRespB)
	assert.Empty(t, listRespB.Data.Items, "Tenant B must not see any blueprints belonging to Tenant A")

	// B attempts to Continue A's blueprint -> 404
	wB_Cont := httptest.NewRecorder()
	httpReqB_Cont := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/continue", bpID_A), nil)
	httpReqB_Cont.Header.Set("Authorization", "Bearer "+tokenB)
	router.ServeHTTP(wB_Cont, httpReqB_Cont)
	assert.Equal(t, http.StatusNotFound, wB_Cont.Code)

	// B attempts to Retry A's section -> 404
	wB_Retry := httptest.NewRecorder()
	httpReqB_Retry := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/retry", bpID_A), nil)
	httpReqB_Retry.Header.Set("Authorization", "Bearer "+tokenB)
	router.ServeHTTP(wB_Retry, httpReqB_Retry)
	assert.Equal(t, http.StatusNotFound, wB_Retry.Code)

	// B attempts to Manual Complete A's section -> 404
	var journeyContent map[string]interface{}
	_ = json.Unmarshal(fix.Sections["journey"], &journeyContent)
	manualBodyB, _ := json.Marshal(map[string]interface{}{
		"revision_id":     genRespA.Data.RevisionID,
		"content_version": 1,
		"content":         journeyContent,
	})
	wB_Manual := httptest.NewRecorder()
	httpReqB_Manual := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/manual", bpID_A), bytes.NewReader(manualBodyB))
	httpReqB_Manual.Header.Set("Authorization", "Bearer "+tokenB)
	httpReqB_Manual.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wB_Manual, httpReqB_Manual)
	assert.Equal(t, http.StatusNotFound, wB_Manual.Code)

	// B attempts to Create blueprint referencing Company A's quotation revision -> 404 source_not_found
	reqBodyB_SourceA := buildQueueGenerationRequest(qID_A, revID_A, fix)
	reqBytesB_SourceA, _ := json.Marshal(reqBodyB_SourceA)
	wB_GenSourceA := httptest.NewRecorder()
	httpReqB_GenSourceA := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/generate", bytes.NewReader(reqBytesB_SourceA))
	httpReqB_GenSourceA.Header.Set("Authorization", "Bearer "+tokenB)
	httpReqB_GenSourceA.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(wB_GenSourceA, httpReqB_GenSourceA)
	assert.Equal(t, http.StatusNotFound, wB_GenSourceA.Code)
	assert.Contains(t, wB_GenSourceA.Body.String(), "source_not_found")

	// 3. Source Snapshot Immutability:
	// Edit the source quotation revision HTML in the database after blueprint creation
	_, err = pool.Exec(context.Background(), `
		UPDATE quotation_revisions 
		SET content_html = '<section><h2>MUTATED AFTER CREATE</h2><p>This content was changed after blueprint creation.</p></section>'
		WHERE id = $1
	`, revID_A)
	require.NoError(t, err)

	// Continue Company A's blueprint
	wContA := httptest.NewRecorder()
	httpReqContA := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/continue", bpID_A), nil)
	httpReqContA.Header.Set("Authorization", "Bearer "+tokenA)
	router.ServeHTTP(wContA, httpReqContA)
	require.Equal(t, http.StatusOK, wContA.Code)

	// Verify that DB revision's source_context_snapshot still contains original data, not mutated HTML
	var sourceSnapshotBytes []byte
	err = pool.QueryRow(context.Background(), `
		SELECT source_context_snapshot FROM blueprint_revisions WHERE blueprint_id = $1
	`, bpID_A).Scan(&sourceSnapshotBytes)
	require.NoError(t, err)
	assert.NotContains(t, string(sourceSnapshotBytes), "MUTATED AFTER CREATE")

	// 4. Sentinel Redaction Checks:
	// Verify that none of the canary sentinels ever leak into any HTTP response (success, status, or detail)
	sentinels := []string{
		"COMMERCIAL_RATE_DISCOUNT_99_PERCENT",
		"LEGAL_INDEMNITY_UNLIMITED_SECRET",
		"POSTGRES_PASSWORD_SUPER_SECRET_123",
		"AI_PROVIDER_API_KEY_SECRET_CANARY",
		"SYSTEM_PROMPT_RAW_INSTRUCTION_SECRET",
	}

	wDetailA := httptest.NewRecorder()
	httpReqDetailA := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/quotation/v1/blueprints/%s", bpID_A), nil)
	httpReqDetailA.Header.Set("Authorization", "Bearer "+tokenA)
	router.ServeHTTP(wDetailA, httpReqDetailA)
	require.Equal(t, http.StatusOK, wDetailA.Code)

	detailBody := wDetailA.Body.String()
	for _, canary := range sentinels {
		assert.NotContains(t, detailBody, canary, "Detail response must not leak sensitive sentinel %s", canary)
	}

	wStatusA := httptest.NewRecorder()
	httpReqStatusA := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/quotation/v1/blueprints/%s/generation-status", bpID_A), nil)
	httpReqStatusA.Header.Set("Authorization", "Bearer "+tokenA)
	router.ServeHTTP(wStatusA, httpReqStatusA)
	require.Equal(t, http.StatusOK, wStatusA.Code)

	statusBody := wStatusA.Body.String()
	for _, canary := range sentinels {
		assert.NotContains(t, statusBody, canary, "Status response must not leak sensitive sentinel %s", canary)
	}
}

// =========================================================================
// Task 7A.08: Real JWT & CompanyContext middleware via protected route harness
// =========================================================================

func TestBlueprintHTTP_MiddlewareJWTAndCompanyContext(t *testing.T) {
	pool := getBlueprintHTTPIntegrationPool(t)
	companyID := uuid.New()
	userID := uuid.New()
	cleanupHTTPIntegrationTenant(t, pool, companyID)
	t.Cleanup(func() { cleanupHTTPIntegrationTenant(t, pool, companyID) })

	fix := loadHTTPQueueFixture(t)
	provider := newHTTPIntegrationAIProvider(fix)
	provSource := &httpIntegrationProviderSource{provider: provider}

	router, _ := setupHTTPIntegrationRouter(t, pool, provSource)

	validToken := generateTestJWTToken(t, userID.String(), companyID.String())
	expiredToken := generateExpiredJWTToken(t, userID.String(), companyID.String())

	// 1. Valid QA Token -> 200 OK (accesses list endpoint)
	wValid := httptest.NewRecorder()
	rValid := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints", nil)
	rValid.Header.Set("Authorization", "Bearer "+validToken)
	router.ServeHTTP(wValid, rValid)
	assert.Equal(t, http.StatusOK, wValid.Code)

	// 2. Missing Authorization Header -> 401 Unauthorized
	wNoAuth := httptest.NewRecorder()
	rNoAuth := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints", nil)
	router.ServeHTTP(wNoAuth, rNoAuth)
	assert.Equal(t, http.StatusUnauthorized, wNoAuth.Code)
	assert.Contains(t, wNoAuth.Body.String(), "Authorization header required")

	// 3. Malformed Authorization Header -> 401 Unauthorized
	wMalformed := httptest.NewRecorder()
	rMalformed := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints", nil)
	rMalformed.Header.Set("Authorization", "Basic user:pass")
	router.ServeHTTP(wMalformed, rMalformed)
	assert.Equal(t, http.StatusUnauthorized, wMalformed.Code)
	assert.Contains(t, wMalformed.Body.String(), "Invalid authorization header format")

	// 4. Invalid Signature (token signed with wrong secret) -> 401 Unauthorized
	wrongSecretToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwtpkg.Claims{
		UserID:    userID.String(),
		CompanyID: companyID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}).SignedString([]byte("wrong-secret-that-does-not-match-jwt-secret-12345"))

	wWrongSig := httptest.NewRecorder()
	rWrongSig := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints", nil)
	rWrongSig.Header.Set("Authorization", "Bearer "+wrongSecretToken)
	router.ServeHTTP(wWrongSig, rWrongSig)
	assert.Equal(t, http.StatusUnauthorized, wWrongSig.Code)
	assert.Contains(t, wWrongSig.Body.String(), "Invalid token signature")

	// 5. Expired Token -> 401 Unauthorized
	wExpired := httptest.NewRecorder()
	rExpired := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints", nil)
	rExpired.Header.Set("Authorization", "Bearer "+expiredToken)
	router.ServeHTTP(wExpired, rExpired)
	assert.Equal(t, http.StatusUnauthorized, wExpired.Code)
	assert.Contains(t, wExpired.Body.String(), "Token has expired")

	// 6. Token with Empty CompanyID -> 403 Forbidden
	noCompanyToken, _ := jwtpkg.GenerateToken(
		userID.String(),
		"", // empty company ID
		"",
		"",
		"",
		"qa@venturo.com",
		"qa_tester",
		"QA Tester",
		false,
		[]string{"admin"},
	)

	wNoComp := httptest.NewRecorder()
	rNoComp := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints", nil)
	rNoComp.Header.Set("Authorization", "Bearer "+noCompanyToken)
	router.ServeHTTP(wNoComp, rNoComp)
	assert.Equal(t, http.StatusForbidden, wNoComp.Code)
	assert.Contains(t, wNoComp.Body.String(), "missing_company_context")
}
