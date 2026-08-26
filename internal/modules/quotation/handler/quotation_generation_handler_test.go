//go:build integration

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		ClientID:                  fx.clientID,
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
			{"role":"programmer","seniority":"senior","employee_count":1,"period_months":3,
			 "scope_references":["Customer portal"],"reason":"Mengimplementasikan fitur portal"},
			{"role":"business_analyst","seniority":"intermediate","employee_count":1,"period_months":3,
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
// (duplikat getTestPool karena bed package).
func getHandlerPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := "postgresql://postgres:postgres@localhost:5432/quotation_db?sslmode=disable"
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

func newGenerationRouter(pool *pgxpool.Pool, provider service.AIProvider, owner uuid.UUID, opts ...service.GenerationOption) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	genRepo := repository.NewGenerationRepository(pool)
	source := &fixedSource{provider: provider, name: "gemini", model: "gemini-test"}
	genSvc := service.NewQuotationGenerationService(genRepo, source, opts...)
	genHandler := handler.NewQuotationGenerationHandler(genSvc, genRepo)

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
		q.POST("/:id/regenerate", genHandler.Regenerate)
		q.GET("/:id/generation-status", genHandler.GetGenerationStatus)
	}
	return router
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
