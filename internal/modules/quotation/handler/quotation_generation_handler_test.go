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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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

// ---------------------------------------------------------------------------
// Adaptive AI mock — satu-satunya komponen yang di-mock di suite ini.
// Klasifiksi call dari ISI PROMPT sehingga aman dipakai oleh banyak goroutine
// konkuren yang interleave allocation/narrative secara nondeterministik.
// ---------------------------------------------------------------------------

type adaptiveAIProvider struct {
	mu             sync.Mutex
	narrativeDelay time.Duration
	narrativeErr   error
	allocErr       error
	callCount      int
}

func (p *adaptiveAIProvider) Generate(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
	p.mu.Lock()
	prompt := req.Prompt
	p.callCount++
	isNarrative := strings.Contains(prompt, `"output_contract"`)
	var (
		delay     time.Duration
		forcedErr error
	)
	if isNarrative {
		delay, forcedErr = p.narrativeDelay, p.narrativeErr
	} else {
		forcedErr = p.allocErr
	}
	p.mu.Unlock()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return service.GenerateResult{}, ctx.Err()
		}
	}
	if forcedErr != nil {
		return service.GenerateResult{}, forcedErr
	}
	if isNarrative {
		return service.GenerateResult{Text: httpNarrativeHTML()}, nil
	}
	return service.GenerateResult{Text: httpAllocationJSON()}, nil
}

func (p *adaptiveAIProvider) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callCount
}

type fixedSource struct {
	provider service.AIProvider
	name     string
	model    string
}

func (f *fixedSource) ProviderForCompany(ctx context.Context, companyID uuid.UUID) (service.AIProvider, string, string, error) {
	return f.provider, f.name, f.model, nil
}

// ---------------------------------------------------------------------------
// Fixtures (salinan minimal dari service_test — bed package, tidak bisa share)
// ---------------------------------------------------------------------------

const httpLegalCode = "QUOTATION_TERMS"

var httpTestUser = uuid.New()

type httpFixture struct {
	pool       *pgxpool.Pool
	companyID  uuid.UUID
	skillID    uuid.UUID
	rateCardID uuid.UUID
	clientID   uuid.UUID
	key        uuid.UUID
	budget     int64
}

func cleanupHTTPFixture(t *testing.T, pool *pgxpool.Pool, companyID uuid.UUID) {
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		pool.Exec(ctx, `UPDATE quotations SET active_published_revision_id = NULL WHERE company_id = $1`, companyID)
		pool.Exec(ctx, `DELETE FROM quotation_revisions WHERE company_id = $1`, companyID)
		pool.Exec(ctx, `DELETE FROM quotations WHERE company_id = $1`, companyID)
		pool.Exec(ctx, `DELETE FROM rate_card_items WHERE company_id = $1`, companyID)
		pool.Exec(ctx, `DELETE FROM rate_cards WHERE company_id = $1`, companyID)
		pool.Exec(ctx, `DELETE FROM legal_templates WHERE company_id = $1`, companyID)
		pool.Exec(ctx, `DELETE FROM ai_skills WHERE company_id = $1`, companyID)
	})
}

func seedHTTPFixtures(t *testing.T, pool *pgxpool.Pool) httpFixture {
	t.Helper()
	fx := httpFixture{
		pool:       pool,
		companyID:  uuid.New(),
		skillID:    uuid.New(),
		rateCardID: uuid.New(),
		clientID:   uuid.New(),
		key:        uuid.New(),
		budget:     150_000_000,
	}
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO rate_cards (id, company_id, code, name, market, currency, effective_from, is_active)
		VALUES ($1, $2, 'RC-S5', 'Session5 Card', 'national', 'IDR', NOW(), true)
	`, fx.rateCardID, fx.companyID)
	require.NoError(t, err)

	for _, item := range []struct {
		role, seniority, term string
		amount                int64
	}{
		{"programmer", "senior", "long_term", 12_000_000},
		{"business_analyst", "intermediate", "long_term", 9_000_000},
	} {
		_, err = pool.Exec(ctx, `
			INSERT INTO rate_card_items (company_id, rate_card_id, role, seniority, term_type, amount_per_month)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, fx.companyID, fx.rateCardID, item.role, item.seniority, item.term, item.amount)
		require.NoError(t, err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO legal_templates (id, company_id, code, version, content_html, is_active)
		VALUES ($1, $2, $3, 1, '<p>Syarat dan ketentuan penawaran resmi berlaku 30 hari.</p>', true)
	`, uuid.New(), fx.companyID, httpLegalCode)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO ai_skills (id, company_id, name, command, content_instruction, is_active)
		VALUES ($1, $2, 'Quotation S5', 'quotation_s5', 'Susun alokasi dedicated squad sesuai scope.', true)
	`, fx.skillID, fx.companyID)
	require.NoError(t, err)

	cleanupHTTPFixture(t, pool, fx.companyID)
	return fx
}

func (fx httpFixture) requestBody() []byte {
	body, err := json.Marshal(dto.GenerateQuotationRequest{
		IdempotencyKey:            strPtr(fx.key.String()),
		SkillID:                   fx.skillID,
		ClientID:                  &fx.clientID,
		Subject:                   "Pengembangan Customer Portal",
		SolutionType:              "custom_software",
		CommercialScheme:          "dedicated_squad",
		RateCardID:                fx.rateCardID,
		RateCardCode:              "RC-S5",
		ProjectDurationMonths:     3,
		BudgetMaximum:             fx.budget,
		Summary:                   "Portal layanan pelanggan internal",
		MandatoryScope:            []string{"Customer portal", "Admin panel"},
		OptionalScope:             []string{"Notification center"},
		AllowedStandardActivities: []string{"code_review", "deployment_support"},
		PaymentTerms: dto.PaymentTermsInput{
			Mode: "milestone",
			Items: []dto.PaymentTermItem{
				{MilestoneName: "DP", Percentage: 50},
				{MilestoneName: "Retensi", Percentage: 50},
			},
		},
		AdditionalNotes: "Gunakan Bahasa Indonesia formal",
	})
	if err != nil {
		panic(err)
	}
	return body
}

func strPtr(s string) *string { return &s }

func httpAllocationJSON() string {
	return `{
		"resources": [
			{"role":"programmer","seniority":"senior","employee_count":1,"period_quarters":12,
			 "scope_references":["Customer portal"],"reason":"Mengimplementasikan fitur portal"},
			{"role":"business_analyst","seniority":"intermediate","employee_count":1,"period_quarters":12,
			 "scope_references":["Admin panel"],"reason":"Mendefinisikan kebutuhan panel admin"}
		],
		"selected_optional_scope": [],
		"standard_activities": []
	}`
}

func httpNarrativeHTML() string {
	return `<h2>Ringkasan Penawaran</h2><p>Kami menyampaikan penawaran dedicated squad untuk mendukung kebutuhan Anda.</p><ul><li>Tim berpengalaman</li></ul>`
}

// ---------------------------------------------------------------------------
// Router yang meniru registrasi produksi main.quotation.go
// ---------------------------------------------------------------------------

// getHandlerPool menyediakan pool integrasi untuk package handler_test
// (duplikat getTestPool karena bed package) dengan resolusi env yang sama.
func getHandlerPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		host := envOr("DB_HOST", "localhost")
		port := envOr("DB_PORT", "5432")
		user := envOr("DB_USER", "postgres")
		pass := envOr("DB_PASSWORD", "postgres")
		name := envOr("DB_NAME", "quotation_db")
		dsn = fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s",
			user, pass, host, port, name, envOr("DB_SSLMODE", "disable"))
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newGenerationRouter(pool *pgxpool.Pool, provider service.AIProvider, owner uuid.UUID, opts ...service.GenerationOption) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	genRepo := repository.NewGenerationRepository(pool)
	source := &fixedSource{provider: provider, name: "gemini", model: "gemini-test"}
	genSvc := service.NewQuotationGenerationService(genRepo, source, opts...)
	previewCalculator := service.NewDedicatedSquadCalculator(repository.NewRateCardRepository(pool))
	genHandler := handler.NewQuotationGenerationHandler(genSvc, genRepo, previewCalculator)

	router.Use(func(c *gin.Context) {
		company := owner
		if raw := c.GetHeader("X-Test-Company"); raw != "" {
			if id, err := uuid.Parse(raw); err == nil {
				company = id
			}
		}
		c.Set(middleware.UserContextKey, &jwtpkg.Claims{
			UserID:    httpTestUser.String(),
			CompanyID: company.String(),
		})
		c.Next()
	})

	q := router.Group("/quotation/v1/quotations")
	{
		q.GET("/:id", genHandler.GetByID)
		q.POST("/generate", genHandler.Generate)
		q.POST("/preview", genHandler.Preview)
		q.POST("/:id/regenerate", genHandler.Regenerate)
		q.GET("/:id/generation-status", genHandler.GetGenerationStatus)
		q.PUT("/:id/draft", genHandler.SaveDraft)
	}
	return router
}

func TestPreview_HTTP_CalculatesWithoutAIOrPersistence(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)
	ctx := context.Background()
	cardID := uuid.New()
	_, err := pool.Exec(ctx, `
		INSERT INTO rate_cards (id, company_id, code, name, market, client_tier, currency, effective_from, is_active)
		VALUES ($1,$2,'VENTURO-STD-2026','Preview Card','national','commercial_national','IDR',NOW(),true)
	`, cardID, fx.companyID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO rate_card_items (company_id, rate_card_id, role, display_name, seniority, discipline, term_type,
			amount_per_month, recruitment_fee, onboarding_fee)
		VALUES ($1,$2,'backend_programmer','Backend Programmer','senior','ENGINEERING','short_term',50000000,1000000,2000000)
	`, fx.companyID, cardID)
	require.NoError(t, err)

	provider := &adaptiveAIProvider{}
	router := newGenerationRouter(pool, provider, fx.companyID)
	anchor := time.Date(2026, time.September, 4, 0, 0, 0, 0, time.UTC)
	payload := dto.GenerateQuotationRequest{
		RateCardID: cardID,
		DedicatedSquad: &domain.DedicatedSquadInput{
			Model: domain.ModelSOWSmall, Requirement: "Bangun portal pelanggan",
			DevelopmentSquad: []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 4}},
			PaymentCount:     2, PaymentAnchorDate: &anchor, Timeline: domain.TimelineInput{Mode: domain.TimelineRelative},
		},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	var beforeQuotations, beforeRevisions int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM quotations WHERE company_id=$1`, fx.companyID).Scan(&beforeQuotations))
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM quotation_revisions WHERE company_id=$1`, fx.companyID).Scan(&beforeRevisions))
	w := doJSON(router, http.MethodPost, "/quotation/v1/quotations/preview", nil, body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 0, provider.calls())

	var responseBody struct {
		Data dto.DedicatedSquadPreviewResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &responseBody))
	assert.Equal(t, int64(50_000_000), responseBody.Data.Calculation.TotalInvestment)
	assert.NotEmpty(t, responseBody.Data.VisibleSections)
	assert.False(t, responseBody.Data.IncludeK6)
	assert.False(t, responseBody.Data.IncludePentest)

	var afterQuotations, afterRevisions int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM quotations WHERE company_id=$1`, fx.companyID).Scan(&afterQuotations))
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM quotation_revisions WHERE company_id=$1`, fx.companyID).Scan(&afterRevisions))
	assert.Equal(t, beforeQuotations, afterQuotations)
	assert.Equal(t, beforeRevisions, afterRevisions)

	payload.DedicatedSquad.DevelopmentSquad[0].Role = "missing_role"
	missingBody, _ := json.Marshal(payload)
	wMissing := doJSON(router, http.MethodPost, "/quotation/v1/quotations/preview", nil, missingBody)
	assert.Equal(t, http.StatusUnprocessableEntity, wMissing.Code)
	assert.Equal(t, service.ErrCodeRateItemMissing, detailCode(t, wMissing))

	payload.DedicatedSquad.DevelopmentSquad[0].Role = "backend_programmer"
	payload.DedicatedSquad.Model = domain.ModelSOWLarge
	invalidBody, _ := json.Marshal(payload)
	wInvalid := doJSON(router, http.MethodPost, "/quotation/v1/quotations/preview", nil, invalidBody)
	assert.Equal(t, http.StatusUnprocessableEntity, wInvalid.Code)
	assert.Equal(t, service.ErrCodeAllocationInvalid, detailCode(t, wInvalid))
}

func TestPreview_HTTP_RequiresCompanyContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := handler.NewQuotationGenerationHandler(nil, nil)
	router.POST("/quotation/v1/quotations/preview", h.Preview)

	w := doJSON(router, http.MethodPost, "/quotation/v1/quotations/preview", nil, []byte(`{"dedicated_squad":{}}`))
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func doJSON(router *gin.Engine, method, path string, companyOverride *uuid.UUID, body []byte) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if companyOverride != nil {
		req.Header.Set("X-Test-Company", companyOverride.String())
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

type apiEnvelope struct {
	Data    json.RawMessage     `json:"data"`
	Message string              `json:"message"`
	Errors  map[string][]string `json:"errors"`
}

func detailCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var env apiEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	if len(env.Errors["detail"]) == 0 {
		return ""
	}
	return env.Errors["detail"][0]
}

func quotationIDFrom(t *testing.T, w *httptest.ResponseRecorder) uuid.UUID {
	t.Helper()
	var env struct {
		Data struct {
			QuotationID uuid.UUID `json:"quotation_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	return env.Data.QuotationID
}

func revisionRowByQuotation(t *testing.T, pool *pgxpool.Pool, quotationID uuid.UUID) (status, html string, version, regenCnt int) {
	t.Helper()
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT generation_status, COALESCE(content_html,''), content_version, regenerate_count
		FROM quotation_revisions WHERE quotation_id = $1
	`, quotationID).Scan(&status, &html, &version, &regenCnt))
	return
}

func generateViaHTTP(t *testing.T, router *gin.Engine, fx httpFixture) uuid.UUID {
	t.Helper()
	w := doJSON(router, http.MethodPost, "/quotation/v1/quotations/generate", nil, fx.requestBody())
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	qid := quotationIDFrom(t, w)
	require.NotEqual(t, uuid.Nil, qid)
	return qid
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestHTTP_Generate_HappyPath_Returns201WithQuotationID(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)
	mock := &adaptiveAIProvider{}
	router := newGenerationRouter(pool, mock, fx.companyID)

	w := doJSON(router, http.MethodPost, "/quotation/v1/quotations/generate", nil, fx.requestBody())
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	qid := quotationIDFrom(t, w)
	require.NotEqual(t, uuid.Nil, qid)

	status, html, _, _ := revisionRowByQuotation(t, pool, qid)
	assert.Equal(t, "completed", status)
	assert.Contains(t, html, "Rp 63.000.000")
	assert.LessOrEqual(t, 2, mock.calls(), "pipeline asli harus memanggil provider (allocation+narrative)")
}

func TestHTTP_Regenerate_HappyPath_Returns200WithIncrementedVersion(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)
	mock := &adaptiveAIProvider{}
	router := newGenerationRouter(pool, mock, fx.companyID)
	qid := generateViaHTTP(t, router, fx)

	w := doJSON(router, http.MethodPost, "/quotation/v1/quotations/"+qid.String()+"/regenerate", nil, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var env struct {
		Data struct {
			ContentVersion   int    `json:"content_version"`
			GenerationStatus string `json:"generation_status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, 2, env.Data.ContentVersion)
	assert.Equal(t, "completed", env.Data.GenerationStatus)

	_, _, version, regenCnt := revisionRowByQuotation(t, pool, qid)
	assert.Equal(t, 2, version)
	assert.Equal(t, 1, regenCnt)
}

func TestHTTP_Regenerate_Timeout_OldHTMLIntactVersionNotIncremented(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)

	// Router fase generate: tanpa delay.
	genMock := &adaptiveAIProvider{}
	genRouter := newGenerationRouter(pool, genMock, fx.companyID)
	qid := generateViaHTTP(t, genRouter, fx)
	beforeStatus, beforeHTML, beforeVersion, _ := revisionRowByQuotation(t, pool, qid)
	require.Equal(t, "completed", beforeStatus)

	// Router fase regenerate: deadline ketat + narrative lambat => timeout.
	regenMock := &adaptiveAIProvider{narrativeDelay: 500 * time.Millisecond}
	regenRouter := newGenerationRouter(pool, regenMock, fx.companyID, service.WithAttemptTimeout(80*time.Millisecond))

	w := doJSON(regenRouter, http.MethodPost, "/quotation/v1/quotations/"+qid.String()+"/regenerate", nil, nil)
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Equal(t, "provider_timeout", detailCode(t, w))

	afterStatus, afterHTML, afterVersion, regenCnt := revisionRowByQuotation(t, pool, qid)
	assert.Equal(t, beforeHTML, afterHTML, "content_html lama WAJIB tetap utuh byte-per-byte")
	assert.Equal(t, beforeVersion, afterVersion, "gagal regenerate TIDAK boleh increment content_version")
	assert.Equal(t, "completed", afterStatus)
	assert.Equal(t, 1, regenCnt, "quota terpakai walau AI gagal")
}

func TestHTTP_TenantIsolation_ForeignCompanyGets404(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)
	mock := &adaptiveAIProvider{}
	router := newGenerationRouter(pool, mock, fx.companyID)
	qid := generateViaHTTP(t, router, fx)

	foreign := uuid.New()

	// Pemilik sah menerima detail lengkap.
	wOwner := doJSON(router, http.MethodGet, "/quotation/v1/quotations/"+qid.String(), nil, nil)
	require.Equal(t, http.StatusOK, wOwner.Code, wOwner.Body.String())

	wStatus := doJSON(router, http.MethodGet, "/quotation/v1/quotations/"+qid.String()+"/generation-status", &foreign, nil)
	assert.Equal(t, http.StatusNotFound, wStatus.Code)

	wRegen := doJSON(router, http.MethodPost, "/quotation/v1/quotations/"+qid.String()+"/regenerate", &foreign, nil)
	assert.Equal(t, http.StatusNotFound, wRegen.Code)

	wLegacy := doJSON(router, http.MethodGet, "/quotation/v1/quotations/"+qid.String(), &foreign, nil)
	assert.Equal(t, http.StatusNotFound, wLegacy.Code)

	// Unknown ID milik tenant sendiri juga 404 (bukan 403/500).
	unknown := doJSON(router, http.MethodGet, "/quotation/v1/quotations/"+uuid.New().String()+"/generation-status", nil, nil)
	assert.Equal(t, http.StatusNotFound, unknown.Code)

	// Counter tidak boleh terkonsumsi oleh percobaan ilegal.
	_, _, _, regenCnt := revisionRowByQuotation(t, pool, qid)
	assert.Equal(t, 0, regenCnt)
}

func TestHTTP_ErrorCodes_RedactedNoInternalDetails(t *testing.T) {
	pool := getHandlerPool(t)
	leaks := []string{
		"150000000", "50000000", // angka budget fixture
		"Customer portal", "Admin panel", // isi prompt
		"goroutine", ".go:", // stack trace
		"connection reset",
	}

	t.Run("budget_unfeasible", func(t *testing.T) {
		fx := seedHTTPFixtures(t, pool)
		fx.budget = 50_000_000 // di bawah alokasi minimum viable
		router := newGenerationRouter(pool, &adaptiveAIProvider{}, fx.companyID)

		w := doJSON(router, http.MethodPost, "/quotation/v1/quotations/generate", nil, fx.requestBody())
		require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
		assert.Equal(t, "budget_unfeasible", detailCode(t, w))
		body := w.Body.String()
		assert.Contains(t, body, "budget_unfeasible")
		for _, leak := range leaks {
			assert.NotContains(t, body, leak, "response tidak boleh membocorkan internal detail")
		}
	})

	t.Run("settings_incomplete", func(t *testing.T) {
		fx := seedHTTPFixtures(t, pool)
		fx.skillID = uuid.New() // skill tidak ada => settings_incomplete
		router := newGenerationRouter(pool, &adaptiveAIProvider{}, fx.companyID)

		w := doJSON(router, http.MethodPost, "/quotation/v1/quotations/generate", nil, fx.requestBody())
		require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
		assert.Equal(t, "settings_incomplete", detailCode(t, w))
	})

	t.Run("provider_timeout", func(t *testing.T) {
		fx := seedHTTPFixtures(t, pool)
		mock := &adaptiveAIProvider{narrativeDelay: 400 * time.Millisecond}
		router := newGenerationRouter(pool, mock, fx.companyID, service.WithAttemptTimeout(60*time.Millisecond))

		w := doJSON(router, http.MethodPost, "/quotation/v1/quotations/generate", nil, fx.requestBody())
		require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
		assert.Equal(t, "provider_timeout", detailCode(t, w))
		body := w.Body.String()
		for _, leak := range leaks {
			assert.NotContains(t, body, leak)
		}
	})
}

func TestHTTP_Regenerate_SixthWithinHour_RateLimited(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)
	router := newGenerationRouter(pool, &adaptiveAIProvider{}, fx.companyID)
	qid := generateViaHTTP(t, router, fx)

	_, err := pool.Exec(context.Background(), `
		UPDATE quotation_revisions
		SET regenerate_count = $2, last_regenerate_at = NOW() - INTERVAL '10 minutes'
		WHERE quotation_id = $1
	`, qid, repository.MaxRegeneratesPerHour)
	require.NoError(t, err)

	w := doJSON(router, http.MethodPost, "/quotation/v1/quotations/"+qid.String()+"/regenerate", nil, nil)
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	assert.Equal(t, "regenerate_rate_limited", detailCode(t, w))

	_, _, version, regenCnt := revisionRowByQuotation(t, pool, qid)
	assert.Equal(t, repository.MaxRegeneratesPerHour, regenCnt, "penolakan tidak boleh mengubah counter")
	assert.Equal(t, 1, version)
}

func TestHTTP_Regenerate_ConcurrentRace_CounterAccurate(t *testing.T) {
	pool := getHandlerPool(t)

	for round := 0; round < 2; round++ {
		fx := seedHTTPFixtures(t, pool)
		router := newGenerationRouter(pool, &adaptiveAIProvider{}, fx.companyID)
		qid := generateViaHTTP(t, router, fx)

		const goroutines = 8
		type outcome struct {
			status int
			code   string
			body   string
		}
		results := make([]outcome, goroutines)
		var wg sync.WaitGroup
		start := make(chan struct{})

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-start // semua goroutine dilepas bersamaan
				w := doJSON(router, http.MethodPost, "/quotation/v1/quotations/"+qid.String()+"/regenerate", nil, nil)
				results[idx] = outcome{status: w.Code, body: w.Body.String()}
			}(i)
		}
		close(start)
		wg.Wait()

		var ok200, limited429 int
		for _, res := range results {
			switch {
			case res.status == http.StatusOK:
				ok200++
			case res.status == http.StatusTooManyRequests && strings.Contains(res.body, "regenerate_rate_limited"):
				limited429++
			default:
				t.Fatalf("unexpected outcome %+v", res)
			}
		}
		assert.Equal(t, repository.MaxRegeneratesPerHour, ok200, "round %d: tepat lima regenerate lolos", round)
		assert.Equal(t, goroutines-repository.MaxRegeneratesPerHour, limited429, "round %d: sisanya ditolak", round)

		_, _, _, regenCnt := revisionRowByQuotation(t, pool, qid)
		assert.Equal(t, repository.MaxRegeneratesPerHour, regenCnt, "round %d: counter akhir akurat", round)
	}
}

func TestHTTP_GetGenerationStatus_ExposesStateWithoutBudgetLeak(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)
	router := newGenerationRouter(pool, &adaptiveAIProvider{}, fx.companyID)
	qid := generateViaHTTP(t, router, fx)

	w := doJSON(router, http.MethodGet, "/quotation/v1/quotations/"+qid.String()+"/generation-status", nil, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	body := w.Body.String()
	assert.Contains(t, body, `"content_version":1`)
	assert.Contains(t, body, `"generation_status":"completed"`)
	assert.Contains(t, body, "Rp 63.000.000")
	for _, forbidden := range []string{"budget_maximum", "allocation_snapshot", "rate_card_snapshot", "legal_template_snapshot", "skill_snapshot"} {
		assert.NotContains(t, body, forbidden)
	}
}

func TestHTTP_GetByID_ExposesLegalSnapshotAndRevisionID(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)
	router := newGenerationRouter(pool, &adaptiveAIProvider{}, fx.companyID)
	qid := generateViaHTTP(t, router, fx)

	w := doJSON(router, http.MethodGet, "/quotation/v1/quotations/"+qid.String(), nil, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	body := w.Body.String()
	assert.Contains(t, body, `"quotation_id":`+fmt.Sprintf("%q", qid.String()))
	assert.Contains(t, body, `"revision_id":`)
	assert.Contains(t, body, `"legal_template_snapshot":`)
	assert.Contains(t, body, `"legal_content_modified":false`)
	assert.Contains(t, body, `"commercial_content_modified":false`)
	assert.Contains(t, body, `"content_version":1`)

	// Ensure sensitive internal figures/snapshots are NOT leaked in detail
	for _, forbidden := range []string{"budget_maximum", "allocation_snapshot", "rate_card_snapshot", "skill_snapshot"} {
		assert.NotContains(t, body, forbidden)
	}
}

// ---------------------------------------------------------------------------
// Task: wiring verification — request HTTP nyata harus mengalir sampai
// pipeline asli (handler → service → repo/renderer/sanitizer/DOM validator);
// HANYA AI provider yang di-mock.
// ---------------------------------------------------------------------------

func TestWiring_HTTPGenerateAndRegenerate_HandlerToRealPipeline(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)
	mock := &adaptiveAIProvider{}
	router := newGenerationRouter(pool, mock, fx.companyID)

	// 1) POST /generate lewat HTTP nyata → response JSON nyata.
	wGen := doJSON(router, http.MethodPost, "/quotation/v1/quotations/generate", nil, fx.requestBody())
	require.Equal(t, http.StatusCreated, wGen.Code, wGen.Body.String())
	qid := quotationIDFrom(t, wGen)
	require.NotEqual(t, uuid.Nil, qid)

	// 2) Artefak yang mendasari response harus berasal dari pipeline ASLI:
	//    re-validasi content_html tersimpan dengan DOMValidator produksi dan
	//    expectations yang dibangun dari allocation_snapshot tersimpan.
	var html, allocSnap string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_html, allocation_snapshot::text
		FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&html, &allocSnap))

	var enriched dto.EnrichedAllocation
	require.NoError(t, json.Unmarshal([]byte(allocSnap), &enriched))
	require.Equal(t, int64(63_000_000), enriched.TotalInvestment,
		"snapshot berasal dari RateAllocator asli terhadap rate card fixture")

	expectations := service.IntegrityExpectations{
		Currency:        enriched.Currency,
		TotalInvestment: enriched.TotalInvestment,
		LegalHTML:       "<p>Syarat dan ketentuan penawaran resmi berlaku 30 hari.</p>",
		BudgetMaximum:   &fx.budget,
	}
	for _, res := range enriched.Resources {
		expectations.Lines = append(expectations.Lines, service.ExpectedLine{
			Role: res.Role, Seniority: res.Seniority, LineTotal: res.LineTotal,
		})
	}
	expectations.PaymentItems = []service.ExpectedPayment{{Percentage: 50}, {Percentage: 50}}

	require.NoError(t, service.NewDOMValidator().ValidateIntegrity(html, expectations),
		"content_html di balik response HTTP harus lolos DOM validator asli")

	// 3) POST /:id/regenerate lewat HTTP → versi baru juga hasil pipeline asli.
	wRegen := doJSON(router, http.MethodPost, "/quotation/v1/quotations/"+qid.String()+"/regenerate", nil, nil)
	require.Equal(t, http.StatusOK, wRegen.Code, wRegen.Body.String())

	var env struct {
		Data struct {
			ContentVersion int    `json:"content_version"`
			ContentHTML    string `json:"content_html"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wRegen.Body.Bytes(), &env))
	require.Equal(t, 2, env.Data.ContentVersion)

	var htmlV2 string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_html FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&htmlV2))
	assert.Equal(t, htmlV2, env.Data.ContentHTML,
		"response regenerate harus konsisten dengan artefak tersimpan")
	require.NoError(t, service.NewDOMValidator().ValidateIntegrity(htmlV2, expectations),
		"content v2 harus tetap lolos validasi integritas")
}

func TestSaveDraft_HTTP_Success_And_StaleSaveCheckpoint(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)

	ai := &adaptiveAIProvider{}
	router := newGenerationRouter(pool, ai, fx.companyID)

	// 1) Generate quotation awal (content_version = 1)
	wGen := doJSON(router, http.MethodPost, "/quotation/v1/quotations/generate", nil, fx.requestBody())
	require.Equal(t, http.StatusCreated, wGen.Code, wGen.Body.String())
	var genResp struct {
		Data struct {
			QuotationID uuid.UUID `json:"quotation_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wGen.Body.Bytes(), &genResp))
	qid := genResp.Data.QuotationID

	// Ambil content awal
	var initHTML string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_html FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&initHTML))

	// 2) PUT draft dengan content_version = 1 -> Sukses 200 OK, versi naik ke 2
	updatedLegalHTML := strings.Replace(
		initHTML,
		"Syarat dan ketentuan penawaran resmi berlaku 30 hari.",
		"Syarat dan ketentuan khusus telah dinegosiasikan 60 hari.",
		1,
	)
	saveBody1, err := json.Marshal(dto.SaveDraftRequest{
		ContentHTML:    updatedLegalHTML,
		ContentVersion: 1,
	})
	require.NoError(t, err)

	wSave1 := doJSON(router, http.MethodPut, "/quotation/v1/quotations/"+qid.String()+"/draft", nil, saveBody1)
	require.Equal(t, http.StatusOK, wSave1.Code, wSave1.Body.String())

	var saveResp1 struct {
		Message string                `json:"message"`
		Data    dto.SaveDraftResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wSave1.Body.Bytes(), &saveResp1))
	assert.Equal(t, "Draft saved successfully", saveResp1.Message)
	assert.Equal(t, 2, saveResp1.Data.ContentVersion)
	assert.True(t, saveResp1.Data.LegalContentModified)
	assert.False(t, saveResp1.Data.CommercialContentModified)
	assert.Equal(t, updatedLegalHTML, saveResp1.Data.ContentHTML)

	// 3) Stale-Save Checkpoint: PUT kedua berurutan dengan content_version = 1 (stale)
	staleHTML := "<p>Stale edit yang seharusnya ditolak</p>"
	saveBodyStale, err := json.Marshal(dto.SaveDraftRequest{
		ContentHTML:    staleHTML,
		ContentVersion: 1,
	})
	require.NoError(t, err)

	wSaveStale := doJSON(router, http.MethodPut, "/quotation/v1/quotations/"+qid.String()+"/draft", nil, saveBodyStale)
	assert.Equal(t, http.StatusConflict, wSaveStale.Code, "Stale save must receive 409 Conflict")
	assert.Equal(t, service.ErrCodeRevisionConflict, detailCode(t, wSaveStale))

	// 4) Buktikan HTML di database TIDAK tertimpa oleh stale save dan versi tetap 2
	var currentHTML string
	var currentVersion int
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_html, content_version FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&currentHTML, &currentVersion))
	assert.Equal(t, 2, currentVersion)
	assert.Equal(t, updatedLegalHTML, currentHTML)
	assert.NotEqual(t, staleHTML, currentHTML)
}

func TestSaveDraft_HTTP_UnsafeHTML_Returns422(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)

	ai := &adaptiveAIProvider{}
	router := newGenerationRouter(pool, ai, fx.companyID)

	wGen := doJSON(router, http.MethodPost, "/quotation/v1/quotations/generate", nil, fx.requestBody())
	require.Equal(t, http.StatusCreated, wGen.Code)
	var genResp struct {
		Data struct {
			QuotationID uuid.UUID `json:"quotation_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wGen.Body.Bytes(), &genResp))
	qid := genResp.Data.QuotationID

	// Kirim HTML dengan kata terlarang "budget"
	unsafeBody, _ := json.Marshal(dto.SaveDraftRequest{
		ContentHTML:    "<p>Informasi budget internal kami</p>",
		ContentVersion: 1,
	})

	wSave := doJSON(router, http.MethodPut, "/quotation/v1/quotations/"+qid.String()+"/draft", nil, unsafeBody)
	assert.Equal(t, http.StatusUnprocessableEntity, wSave.Code)
	assert.Equal(t, service.ErrCodeUnsafeHTML, detailCode(t, wSave))
}

func TestSaveDraft_HTTP_ReturnsCanonicalSanitizedHTML(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)
	router := newGenerationRouter(pool, &adaptiveAIProvider{}, fx.companyID)

	wGen := doJSON(router, http.MethodPost, "/quotation/v1/quotations/generate", nil, fx.requestBody())
	require.Equal(t, http.StatusCreated, wGen.Code, wGen.Body.String())
	var genResp struct {
		Data struct {
			QuotationID uuid.UUID `json:"quotation_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wGen.Body.Bytes(), &genResp))

	var initialHTML string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_html FROM quotation_revisions WHERE quotation_id = $1
	`, genResp.Data.QuotationID).Scan(&initialHTML))
	body, err := json.Marshal(dto.SaveDraftRequest{
		ContentHTML:    `<script>alert("secret")</script>` + initialHTML,
		ContentVersion: 1,
	})
	require.NoError(t, err)

	wSave := doJSON(router, http.MethodPut, "/quotation/v1/quotations/"+genResp.Data.QuotationID.String()+"/draft", nil, body)
	require.Equal(t, http.StatusOK, wSave.Code, wSave.Body.String())
	var saveResp struct {
		Data dto.SaveDraftResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wSave.Body.Bytes(), &saveResp))
	assert.NotContains(t, saveResp.Data.ContentHTML, "<script")
	assert.NotContains(t, saveResp.Data.ContentHTML, "alert(")
	assert.Equal(t, initialHTML, saveResp.Data.ContentHTML)
}

func TestSaveDraft_HTTP_InvalidPayload_Returns400(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)

	router := newGenerationRouter(pool, &adaptiveAIProvider{}, fx.companyID)

	// Bad UUID in path
	wBadUUID := doJSON(router, http.MethodPut, "/quotation/v1/quotations/not-a-uuid/draft", nil, []byte(`{}`))
	assert.Equal(t, http.StatusBadRequest, wBadUUID.Code)

	// Bad JSON body
	wBadBody := doJSON(router, http.MethodPut, "/quotation/v1/quotations/"+uuid.New().String()+"/draft", nil, []byte(`invalid json`))
	assert.Equal(t, http.StatusBadRequest, wBadBody.Code)

	// Non-positive version in JSON body
	wZeroVersion := doJSON(router, http.MethodPut, "/quotation/v1/quotations/"+uuid.New().String()+"/draft", nil, []byte(`{"content_version": 0, "content_html": "<p>test</p>"}`))
	assert.Equal(t, http.StatusBadRequest, wZeroVersion.Code)
}

func TestSaveDraft_HTTP_NotFound_Returns404(t *testing.T) {
	pool := getHandlerPool(t)
	fx := seedHTTPFixtures(t, pool)

	router := newGenerationRouter(pool, &adaptiveAIProvider{}, fx.companyID)

	validBody, _ := json.Marshal(dto.SaveDraftRequest{
		ContentHTML:    "<p>Hello</p>",
		ContentVersion: 1,
	})

	// Non-existent quotation
	wNotFound := doJSON(router, http.MethodPut, "/quotation/v1/quotations/"+uuid.New().String()+"/draft", nil, validBody)
	assert.Equal(t, http.StatusNotFound, wNotFound.Code)

	// Cross-tenant: quotation milik company lain
	wGen := doJSON(router, http.MethodPost, "/quotation/v1/quotations/generate", nil, fx.requestBody())
	require.Equal(t, http.StatusCreated, wGen.Code)
	var genResp struct {
		Data struct {
			QuotationID uuid.UUID `json:"quotation_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wGen.Body.Bytes(), &genResp))

	otherCompany := uuid.New()
	wCrossTenant := doJSON(router, http.MethodPut, "/quotation/v1/quotations/"+genResp.Data.QuotationID.String()+"/draft", &otherCompany, validBody)
	assert.Equal(t, http.StatusNotFound, wCrossTenant.Code, "Cross-tenant access must return 404 without leaking existence")
}
