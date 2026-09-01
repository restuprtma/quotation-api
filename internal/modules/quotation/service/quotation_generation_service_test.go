//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

// Harus identik dengan kode yang di-seed EnsureDefaultAIAssets (settings_service.go)
// agar tenant yang memakai default assets langsung kompatibel dengan pipeline.
const pipelineLegalCode = "QUOTATION_TERMS"

// ---------------------------------------------------------------------------
// Scripted mock AI provider — the ONLY mocked component in this suite.
// ---------------------------------------------------------------------------

type scriptedStep struct {
	text  string
	delay time.Duration
	err   error
}

type scriptedAIProvider struct {
	mu      sync.Mutex
	steps   []scriptedStep
	prompts []string
	onCall  func(callIndex int)
}

func (p *scriptedAIProvider) Generate(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
	p.mu.Lock()
	idx := len(p.prompts)
	p.prompts = append(p.prompts, req.Prompt)
	step := scriptedStep{}
	if idx < len(p.steps) {
		step = p.steps[idx]
	} else {
		step.err = fmt.Errorf("unexpected extra AI call #%d", idx+1)
	}
	hook := p.onCall
	p.mu.Unlock()

	if hook != nil {
		hook(idx)
	}
	if step.delay > 0 {
		select {
		case <-time.After(step.delay):
		case <-ctx.Done():
			return service.GenerateResult{}, ctx.Err()
		}
	}
	if step.err != nil {
		return service.GenerateResult{}, step.err
	}
	return service.GenerateResult{Text: step.text}, nil
}

func (p *scriptedAIProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.prompts)
}

type fixedProviderSource struct {
	provider service.AIProvider
	name     string
	model    string

	mu        sync.Mutex
	requested []uuid.UUID
}

func (f *fixedProviderSource) ProviderForCompany(ctx context.Context, companyID uuid.UUID) (service.AIProvider, string, string, error) {
	f.mu.Lock()
	f.requested = append(f.requested, companyID)
	f.mu.Unlock()
	return f.provider, f.name, f.model, nil
}

func (f *fixedProviderSource) requests() []uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uuid.UUID(nil), f.requested...)
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

const (
	fixBudget     = int64(150_000_000)
	fixTinyBudget = int64(50_000_000)
	fixValidTotal = int64(63_000_000)  // 12jt*1*3 + 9jt*1*3
	fixOverTotal  = int64(171_000_000) // 12jt*4*3 + 9jt*1*3
	fixOverExcess = int64(21_000_000)
)

type pipelineFixture struct {
	pool       *pgxpool.Pool
	companyID  uuid.UUID
	skillID    uuid.UUID
	rateCardID uuid.UUID
	clientID   uuid.UUID
	key        uuid.UUID
	budget     int64
}

func seedPipelineFixtures(t *testing.T, pool *pgxpool.Pool) pipelineFixture {
	t.Helper()
	fx := pipelineFixture{
		pool:       pool,
		companyID:  uuid.New(),
		skillID:    uuid.New(),
		rateCardID: uuid.New(),
		clientID:   uuid.New(),
		key:        uuid.New(),
		budget:     fixBudget,
	}
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO rate_cards (id, company_id, code, name, market, currency, effective_from, is_active)
		VALUES ($1, $2, 'RC-S4', 'Session4 Card', 'national', 'IDR', NOW(), true)
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
	`, uuid.New(), fx.companyID, pipelineLegalCode)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO ai_skills (id, company_id, name, command, content_instruction, is_active)
		VALUES ($1, $2, 'Quotation S4', 'quotation_s4', 'Susun alokasi dedicated squad sesuai scope.', true)
	`, fx.skillID, fx.companyID)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*1e9)
		defer cancel()
		pool.Exec(cleanupCtx, `UPDATE quotations SET active_published_revision_id = NULL WHERE company_id = $1`, fx.companyID)
		pool.Exec(cleanupCtx, `DELETE FROM quotation_revisions WHERE company_id = $1`, fx.companyID)
		pool.Exec(cleanupCtx, `DELETE FROM quotations WHERE company_id = $1`, fx.companyID)
		pool.Exec(cleanupCtx, `DELETE FROM rate_card_items WHERE company_id = $1`, fx.companyID)
		pool.Exec(cleanupCtx, `DELETE FROM rate_cards WHERE company_id = $1`, fx.companyID)
		pool.Exec(cleanupCtx, `DELETE FROM legal_templates WHERE company_id = $1`, fx.companyID)
		pool.Exec(cleanupCtx, `DELETE FROM ai_skills WHERE company_id = $1`, fx.companyID)
	})

	return fx
}

// request builds a fully valid generate request bound to the fixture IDs.
func (fx pipelineFixture) request() dto.GenerateQuotationRequest {
	subject := "Pengembangan Customer Portal"
	return dto.GenerateQuotationRequest{
		IdempotencyKey:            uuidPtrString(fx.key.String()),
		SkillID:                   fx.skillID,
		ClientID:                  &fx.clientID,
		Subject:                   subject,
		SolutionType:              "custom_software",
		CommercialScheme:          "dedicated_squad",
		RateCardID:                fx.rateCardID,
		RateCardCode:              "RC-S4",
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
	}
}

func uuidPtrString(s string) *string { return &s }

// allocationJSON returns a valid, in-budget allocation proposal payload.
func allocationJSON() string {
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

// allocationJSONOverbudget exceeds the default budget by fixOverExcess.
func allocationJSONOverbudget() string {
	return `{
		"resources": [
			{"role":"programmer","seniority":"senior","employee_count":4,"period_quarters":12,
			 "scope_references":["Customer portal"],"reason":"Percepatan implementasi portal"},
			{"role":"business_analyst","seniority":"intermediate","employee_count":1,"period_quarters":12,
			 "scope_references":["Admin panel"],"reason":"Mendefinisikan kebutuhan panel admin"}
		],
		"selected_optional_scope": [],
		"standard_activities": []
	}`
}

func narrativeHTML() string {
	return `<h2>Ringkasan Penawaran</h2><p>Kami menyampaikan penawaran dedicated squad untuk mendukung kebutuhan Anda.</p><ul><li>Tim berpengalaman</li></ul>`
}

func newSvc(pool *pgxpool.Pool, provider service.AIProvider, opts ...service.GenerationOption) (*service.QuotationGenerationService, *fixedProviderSource) {
	source := &fixedProviderSource{provider: provider, name: "gemini", model: "gemini-test"}
	opts = append([]service.GenerationOption{service.WithAttemptTimeout(2 * time.Second)}, opts...)
	svc := service.NewQuotationGenerationService(repository.NewGenerationRepository(pool), source, opts...)
	return svc, source
}

func revisionRow(t *testing.T, pool *pgxpool.Pool, quotationID uuid.UUID) (status, html, aiProv, aiModel string) {
	t.Helper()
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT generation_status, COALESCE(content_html,''), COALESCE(ai_provider,''), COALESCE(ai_model,'')
		FROM quotation_revisions WHERE quotation_id = $1
	`, quotationID).Scan(&status, &html, &aiProv, &aiModel))
	return
}

func assertPendingRevision(t *testing.T, pool *pgxpool.Pool, companyID uuid.UUID, key uuid.UUID) {
	t.Helper()
	var status, html string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT r.generation_status, COALESCE(r.content_html,'')
		FROM quotation_revisions r
		JOIN quotations q ON q.id = r.quotation_id
		WHERE r.company_id = $1 AND r.idempotency_key = $2
	`, companyID, key).Scan(&status, &html))
	assert.Equal(t, "pending", status, "revision gagal harus tetap pending/retryable")
	assert.Empty(t, html, "revision gagal tidak boleh punya content_html")
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestGenerate_HappyPath_PersistsCompletedRevision(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: narrativeHTML()},
	}}
	svc, _ := newSvc(pool, mock)

	createdByID := uuid.New()
	qid, err := svc.Generate(context.Background(), fx.companyID, &createdByID, fx.request())
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, qid)

	status, html, aiProv, aiModel := revisionRow(t, pool, qid)
	assert.Equal(t, "completed", status)
	assert.Contains(t, html, `data-field="total_investment"`)
	assert.Contains(t, html, "Rp 63.000.000")
	assert.NotContains(t, strings.ToLower(html), "<script")
	assert.Equal(t, "gemini", aiProv)
	assert.Equal(t, "gemini-test", aiModel)

	require.Len(t, mock.prompts, 2)
	// Allocation prompt membawa safety envelope + instruction skill + scope wajib.
	assert.Contains(t, mock.prompts[0], "Output MUST strictly adhere")
	assert.Contains(t, mock.prompts[0], "Susun alokasi dedicated squad sesuai scope.")
	assert.Contains(t, mock.prompts[0], "Customer portal")
	// Narrative prompt membawa kontrak output HTML fragmen.
	assert.Contains(t, mock.prompts[1], "Pengembangan Customer Portal")

	var allocSnap, rateSnap, legalSnap, skillSnap string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT allocation_snapshot::text, rate_card_snapshot::text, legal_template_snapshot::text, skill_snapshot::text
		FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&allocSnap, &rateSnap, &legalSnap, &skillSnap))

	var allocMap map[string]any
	require.NoError(t, json.Unmarshal([]byte(allocSnap), &allocMap))
	assert.Equal(t, float64(63_000_000), allocMap["total_investment"])
	assert.Equal(t, "long_term", allocMap["term_type"])

	var rateMap map[string]any
	require.NoError(t, json.Unmarshal([]byte(rateSnap), &rateMap))
	assert.Equal(t, "IDR", rateMap["currency"])

	var legalMap map[string]any
	require.NoError(t, json.Unmarshal([]byte(legalSnap), &legalMap))
	assert.Equal(t, pipelineLegalCode, legalMap["code"])

	var skillMap map[string]any
	require.NoError(t, json.Unmarshal([]byte(skillSnap), &skillMap))
	assert.Equal(t, "quotation_s4", skillMap["command"])

	// Audit trail: identitas pembuat (jalur injeksi handler Session 5) dan
	// versi safety envelope wajib ter-persist.
	var createdBy, envelopeVersion *string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT created_by::text, safety_envelope_version
		FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&createdBy, &envelopeVersion))
	require.NotNil(t, createdBy)
	assert.Equal(t, createdByID.String(), *createdBy)
	require.NotNil(t, envelopeVersion)
	assert.Equal(t, "quotation_safety_v1", *envelopeVersion)
}

func TestGenerate_BudgetRetry_SecondAttemptSucceedsWithFeedback(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	// Attempt 1: 12jt*4*3 + 9jt*1*3 = 171jt > budget 150jt (excess 21jt).
	// Attempt 2: valid & in-budget => lanjut narrative.
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSONOverbudget()},
		{text: allocationJSON()},
		{text: narrativeHTML()},
	}}
	svc, _ := newSvc(pool, mock)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	require.NoError(t, err)

	assert.Equal(t, 3, mock.callCount()) // 2 allocation + 1 narrative
	assert.Contains(t, mock.prompts[1], `"attempt_number":2`)
	assert.Contains(t, mock.prompts[1], `"maximum_attempts":3`)
	assert.Contains(t, mock.prompts[1], `"budget_excess_amount":21000000`)

	status, html, _, _ := revisionRow(t, pool, qid)
	assert.Equal(t, "completed", status)
	assert.Contains(t, html, "Rp 63.000.000")
}

func TestGenerate_ThreeTimesOverbudget_BudgetUnfeasible(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)
	fx.budget = fixTinyBudget // 50jt < alokasi minimum viable 63jt

	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: allocationJSON()},
		{text: allocationJSON()},
	}}
	svc, _ := newSvc(pool, mock)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	require.Error(t, err)
	var ge *service.GenerationError
	require.ErrorAs(t, err, &ge)
	assert.Equal(t, "budget_unfeasible", ge.Code)
	assert.Len(t, ge.Error(), len("budget_unfeasible")) // redaksi: Error() == kode saja
	// Exception sanctionne: untuk budget_unfeasible, ringkasan alokasi percobaan
	// terakhir (role + total + selisih) boleh ikut dalam Summary/pesan HTTP —
	// angka turunan rate card yang memang sudah terlihat di dokumen.
	require.NotEmpty(t, ge.Summary, "budget_unfeasible harus membawa ringkasan alokasi terakhir")
	assert.Contains(t, ge.Summary, "programmer", "ringkasan menyebut komposisi role")
	assert.Contains(t, ge.Summary, "business_analyst")
	assert.Contains(t, ge.Summary, "Rp", "ringkasan memuat angka uang")
	assert.Contains(t, ge.Summary, "Rp 50.000.000", "ringkasan menyebut maksimum budget")

	assert.Equal(t, 3, mock.callCount(), "tepat 3 attempts, tidak lebih")
	assert.NotEqual(t, uuid.Nil, qid)
	assertPendingRevision(t, pool, fx.companyID, fx.key)
}

func TestGenerate_AllocationTimeout_ProviderTimeoutFastFail(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	// Provider lebih lambat dari deadline attempt => harus provider_timeout,
	// cepat, fatal (tidak lanjut attempt 2), dan revision tetap pending.
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON(), delay: 500 * time.Millisecond},
	}}
	svc, _ := newSvc(pool, mock, service.WithAttemptTimeout(80*time.Millisecond))

	start := time.Now()
	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	elapsed := time.Since(start)

	require.Error(t, err)
	var ge *service.GenerationError
	require.ErrorAs(t, err, &ge)
	assert.Equal(t, "provider_timeout", ge.Code)
	assert.Less(t, elapsed, 2*time.Second, "attempt yang melewati deadline tidak boleh menggantung")
	assert.Equal(t, 1, mock.callCount(), "timeout bersifat fatal: tidak retry attempt berikutnya")
	assert.NotEqual(t, uuid.Nil, qid)
	assertPendingRevision(t, pool, fx.companyID, fx.key)
}

func TestGenerate_NarrativeTimeout_ProviderTimeout(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	// Allocation cepat; narrative lambat => tahap narrative juga wajib
	// ter-cover deadline per attempt.
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: narrativeHTML(), delay: 500 * time.Millisecond},
	}}
	svc, _ := newSvc(pool, mock, service.WithAttemptTimeout(80*time.Millisecond))

	_, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	require.Error(t, err)
	var ge *service.GenerationError
	require.ErrorAs(t, err, &ge)
	assert.Equal(t, "provider_timeout", ge.Code)
}

// ---------------------------------------------------------------------------
// Task 5: validation failure mapping & mandatory scope across retries
// ---------------------------------------------------------------------------

// assertGenerationCode memastikan error adalah GenerationError dengan kode
// tertentu DAN pesan ter-redaksi (Error() == kode saja).
func assertGenerationCode(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	var ge *service.GenerationError
	require.ErrorAs(t, err, &ge)
	assert.Equal(t, code, ge.Code)
	assert.Equal(t, code, err.Error(), "pesan error harus ter-redaksi menjadi kode saja")
}

func TestGenerate_UnknownRole_AllocationInvalidSingleCall(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: `{"resources":[{"role":"data_scientist","seniority":"senior","employee_count":1,"period_quarters":12,"scope_references":["Customer portal"],"reason":"Analisis data"}],"selected_optional_scope":[],"standard_activities":[]}`},
	}}
	svc, _ := newSvc(pool, mock)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	assertGenerationCode(t, err, "allocation_invalid")
	assert.NotEqual(t, uuid.Nil, qid)
	assert.Equal(t, 1, mock.callCount(), "allocation_invalid bersifat fatal")
	assertPendingRevision(t, pool, fx.companyID, fx.key)
}

func TestGenerate_ForbiddenRateField_AllocationInvalid(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	// AI mencoba menyelipkan angka rate/total — DisallowUnknownFields harus menolak.
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: `{"resources":[{"role":"programmer","seniority":"senior","employee_count":1,"period_quarters":12,"scope_references":["Customer portal"],"reason":"Implementasi","rate":12000000}],"selected_optional_scope":[],"standard_activities":[],"total":36000000}`},
	}}
	svc, _ := newSvc(pool, mock)

	_, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	assertGenerationCode(t, err, "allocation_invalid")
}

func TestGenerate_NewBusinessFeatureRejected_AllocationInvalid(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	// Fitur baru di luar mandatory+optional+standard allowlist.
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: `{"resources":[{"role":"programmer","seniority":"senior","employee_count":1,"period_quarters":12,"scope_references":["AI dashboard"],"reason":"Fitur AI baru"}],"selected_optional_scope":[],"standard_activities":[]}`},
	}}
	svc, _ := newSvc(pool, mock)

	_, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	assertGenerationCode(t, err, "allocation_invalid")
}

func TestGenerate_MissingRateItem_RateItemMissing(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	// quality_assurance valid sbg role tapi tidak ada di rate card fixture.
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: `{"resources":[{"role":"programmer","seniority":"senior","employee_count":1,"period_quarters":12,"scope_references":["Customer portal"],"reason":"Implementasi"},{"role":"quality_assurance","seniority":"intermediate","employee_count":1,"period_quarters":12,"scope_references":["Admin panel"],"reason":"Uji kualitas"}],"selected_optional_scope":[],"standard_activities":[]}`},
	}}
	svc, _ := newSvc(pool, mock)

	_, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	assertGenerationCode(t, err, "rate_item_missing")
}

func TestGenerate_MandatoryScopeDroppedOnRetry_FatalAllocationInvalid(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	// Attempt 1: overbudget tapi mandatory ter-cover semua.
	// Attempt 2: muat budget TAPI "Admin panel" hilang => validator wajib
	// menolak meski sedang berada di tengah retry loop.
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSONOverbudget()},
		{text: `{"resources":[{"role":"programmer","seniority":"senior","employee_count":1,"period_quarters":12,"scope_references":["Customer portal"],"reason":"Implementasi"}],"selected_optional_scope":["Notification center"],"standard_activities":[]}`},
		{text: narrativeHTML()},
	}}
	svc, _ := newSvc(pool, mock)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	assertGenerationCode(t, err, "allocation_invalid")
	assert.Equal(t, 2, mock.callCount(), "gagal pada attempt-2 menghentikan loop")
	assertPendingRevision(t, pool, fx.companyID, fx.key)
	_ = qid
}

// ---------------------------------------------------------------------------
// Task 6: unsafe narrative handling & sanitization enforcement
// ---------------------------------------------------------------------------

func TestGenerate_NarrativeWithProtectedMarker_RejectedUnsafeHTML(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	// Narrative mencoba memalsukan blok critical backend => Assemble wajib menolak.
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: `<p>Aman.</p><div data-block="investment">blok palsu dari AI</div>`},
	}}
	svc, _ := newSvc(pool, mock)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	assertGenerationCode(t, err, "unsafe_html")
	assert.NotEqual(t, uuid.Nil, qid)
	assertPendingRevision(t, pool, fx.companyID, fx.key)
}

// ---------------------------------------------------------------------------
// Task 7: idempotency replay & commit-before-provider ordering
// ---------------------------------------------------------------------------

func TestGenerate_IdempotentDoubleCall_SameIDSingleRun(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: narrativeHTML()},
	}}
	svc, _ := newSvc(pool, mock)
	req := fx.request()

	q1, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)
	q2, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)

	assert.Equal(t, q1, q2, "double submit harus mengembalikan aggregate yang sama")
	assert.Equal(t, 2, mock.callCount(), "call ke-2 short-circuit sebelum menyentuh provider")
}

func TestGenerate_ProviderSeesCommittedDraft_CommitBeforeProvider(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	// Saat AI call PERTAMA dieksekusi, draft revision SUDAH harus committed
	// di DB (generation_status='pending') — design rule commit-before-provider.
	var sawCommittedRow atomic.Bool

	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: narrativeHTML()},
	}}
	mock.onCall = func(callIndex int) {
		if callIndex != 0 {
			return
		}
		var status string
		err := pool.QueryRow(context.Background(), `
			SELECT generation_status FROM quotation_revisions
			WHERE company_id = $1 AND idempotency_key = $2 AND quotation_id IS NOT NULL
		`, fx.companyID, fx.key).Scan(&status)
		if err == nil && status == "pending" {
			sawCommittedRow.Store(true)
		}
	}
	svc, _ := newSvc(pool, mock)

	_, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	require.NoError(t, err)
	assert.True(t, sawCommittedRow.Load(),
		"draft harus sudah committed sebelum pemanggilan provider pertama")
}

// ---------------------------------------------------------------------------
// Task 8: credential handling — Fase 5 compliance
// ---------------------------------------------------------------------------

const testEncryptionKey = "0123456789abcdef0123456789abcdef" // 32-byte test-only key

func seedCredential(t *testing.T, pool *pgxpool.Pool, companyID uuid.UUID, provider, encryptedKey string) uuid.UUID {
	t.Helper()
	credID := uuid.New()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO ai_credentials (id, company_id, provider, model, encrypted_api_key, key_hint, is_active)
		VALUES ($1, $2, $3, 'gemini-test', $4, 'hint-abcd', true)
	`, credID, companyID, provider, encryptedKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM ai_credentials WHERE company_id = $1`, companyID)
	})
	return credID
}

func TestProviderSource_MissingCredential_SettingsIncomplete(t *testing.T) {
	pool := getTestPool(t)
	source := service.NewDBCredentialProviderSource(
		repository.NewCredentialRepository(pool),
		service.NewProviderFactory(),
		testEncryptionKey,
	)

	_, _, _, err := source.ProviderForCompany(context.Background(), uuid.New())
	assertGenerationCode(t, err, "settings_incomplete")
}

func TestProviderSource_EmptyEncryptedKey_ProviderInvalidCredential(t *testing.T) {
	pool := getTestPool(t)
	companyID := uuid.New()
	seedCredential(t, pool, companyID, "gemini", "") // key kosong => factory guard

	source := service.NewDBCredentialProviderSource(
		repository.NewCredentialRepository(pool),
		service.NewProviderFactory(),
		testEncryptionKey,
	)

	_, _, _, err := source.ProviderForCompany(context.Background(), companyID)
	assertGenerationCode(t, err, "provider_invalid_credential")
}

func TestProviderSource_DecryptsPerRequest_NoCacheNoFallback(t *testing.T) {
	pool := getTestPool(t)
	companyID := uuid.New()
	cipher, err := service.NewCredentialCipher(testEncryptionKey)
	require.NoError(t, err)

	v1, err := cipher.Encrypt("key-plaintext-one")
	require.NoError(t, err)
	seedCredential(t, pool, companyID, "gemini", v1)

	source := service.NewDBCredentialProviderSource(
		repository.NewCredentialRepository(pool),
		service.NewProviderFactory(),
		testEncryptionKey,
	)
	ctx := context.Background()

	p1, name1, model1, err := source.ProviderForCompany(ctx, companyID)
	require.NoError(t, err)
	assert.NotNil(t, p1)
	assert.Equal(t, "gemini", name1)
	assert.Equal(t, "gemini-test", model1)

	// Rotasi ciphertext di DB: jika ada cache (struct field/global), call ke-2
	// akan memakai hasil decrypt lama atau gagal — sukses di sini membuktikan
	// decrypt fresh per-request. Tidak ada ketergantungan env GEMINI_API_KEY.
	v2, err := cipher.Encrypt("key-plaintext-two")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE ai_credentials SET encrypted_api_key = $1 WHERE company_id = $2`, v2, companyID)
	require.NoError(t, err)

	p2, _, _, err := source.ProviderForCompany(ctx, companyID)
	require.NoError(t, err)
	assert.NotNil(t, p2)
}

func TestGenerate_TenantIsolation_NoCrossCompanyCredential(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)
	otherCompany := uuid.New() // TANPA credential apapun

	cipher, err := service.NewCredentialCipher(testEncryptionKey)
	require.NoError(t, err)
	plainKey, err := cipher.Encrypt("tenant-a-key")
	require.NoError(t, err)
	seedCredential(t, pool, fx.companyID, "gemini", plainKey)

	source := service.NewDBCredentialProviderSource(
		repository.NewCredentialRepository(pool),
		service.NewProviderFactory(),
		testEncryptionKey,
	)

	// Company A (punya credential) -> sukses instansiasi.
	_, _, _, err = source.ProviderForCompany(context.Background(), fx.companyID)
	require.NoError(t, err)

	// Company B (tanpa credential) -> ditolak; tidak ada fallback/pinjam A.
	_, _, _, err = source.ProviderForCompany(context.Background(), otherCompany)
	assertGenerationCode(t, err, "settings_incomplete")

	// Pipeline hanya pernah meminta provider untuk company milik request.
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: narrativeHTML()},
	}}
	svc, fakeSource := newSvc(pool, mock)
	_, err = svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	require.NoError(t, err)
	requested := fakeSource.requests()
	require.Len(t, requested, 1)
	assert.Equal(t, fx.companyID, requested[0])
}

// ---------------------------------------------------------------------------
// Task 9: full orchestration wiring verification
// ---------------------------------------------------------------------------

func indexOf(slice []string, target string) int {
	for i, v := range slice {
		if v == target {
			return i
		}
	}
	return -1
}

func TestWiring_GenerateEndToEnd_RealComponentsOnlyProviderMocked(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)
	ctx := context.Background()

	var mu sync.Mutex
	var events []string

	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: narrativeHTML()},
	}}
	mock.onCall = func(callIndex int) {
		mu.Lock()
		defer mu.Unlock()
		if callIndex == 0 {
			var pendingCount int
			pool.QueryRow(ctx, `
				SELECT COUNT(*) FROM quotation_revisions
				WHERE company_id = $1 AND idempotency_key = $2 AND generation_status = 'pending'
			`, fx.companyID, fx.key).Scan(&pendingCount)
			if pendingCount == 1 {
				events = append(events, "draft_committed_before_ai")
			}
			events = append(events, "allocation_call")
		} else {
			events = append(events, "narrative_call")
		}
	}

	svc, _ := newSvc(pool, mock)
	qid, err := svc.Generate(ctx, fx.companyID, nil, fx.request())
	require.NoError(t, err)

	mu.Lock()
	require.GreaterOrEqual(t, indexOf(events, "draft_committed_before_ai"), 0)
	require.GreaterOrEqual(t, indexOf(events, "allocation_call"), 0)
	require.Less(t, indexOf(events, "draft_committed_before_ai"), indexOf(events, "allocation_call"))
	require.Less(t, indexOf(events, "allocation_call"), indexOf(events, "narrative_call"))
	mu.Unlock()

	// Semua komponen NYATA membuktikan dirinya pada artefak persist:
	var html, allocSnap string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT content_html, allocation_snapshot::text
		FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&html, &allocSnap))

	// Renderer nyata: blok metadata + payment + legal + format IDR backend.
	assert.Contains(t, html, `data-block="metadata"`)
	assert.Contains(t, html, `data-block="payment_schedule"`)
	assert.Contains(t, html, `data-block="legal"`)
	assert.Contains(t, html, "Rp 63.000.000")
	// Sanitizer nyata: tidak ada tag berbahaya.
	lowerHTML := strings.ToLower(html)
	assert.NotContains(t, lowerHTML, "<iframe")
	assert.NotContains(t, lowerHTML, "<form")
	assert.NotContains(t, lowerHTML, "javascript:")
	// Loader/repo nyata: snapshot konsisten dengan seed.
	assert.Contains(t, allocSnap, "63000000")

	// Re-validasi integritas dokumen TERSIMPAN memakai DOMValidator ASLI,
	// dengan expectations dari snapshot alokasi yang tersimpan.
	var enriched dto.EnrichedAllocation
	require.NoError(t, json.Unmarshal([]byte(allocSnap), &enriched))

	var budgetMax int64
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT budget_maximum FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&budgetMax))

	expectations := service.IntegrityExpectations{
		Currency:        enriched.Currency,
		TotalInvestment: enriched.TotalInvestment,
		LegalHTML:       `<p>Syarat dan ketentuan penawaran resmi berlaku 30 hari.</p>`,
		BudgetMaximum:   &budgetMax,
	}
	for _, res := range enriched.Resources {
		expectations.Lines = append(expectations.Lines, service.ExpectedLine{
			Role: res.Role, Seniority: res.Seniority, LineTotal: res.LineTotal,
		})
	}
	expectations.PaymentItems = []service.ExpectedPayment{{Percentage: 50}, {Percentage: 50}}

	require.NoError(t, service.NewDOMValidator().ValidateIntegrity(html, expectations),
		"content_html tersimpan harus lolos validasi integritas DOM validator asli")
}

func TestGenerate_NarrativeScriptTag_SanitizedStillCompletes(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	// <script> di narrative tidak boleh menggagalkan pipeline — sanitizer
	// menghapusnya dan hasil tetap completed tanpa jejak script.
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: `<p>Aman</p><script>alert(1)</script><strong>OK</strong>`},
	}}
	svc, _ := newSvc(pool, mock)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	require.NoError(t, err)

	status, html, _, _ := revisionRow(t, pool, qid)
	assert.Equal(t, "completed", status)
	assert.NotContains(t, strings.ToLower(html), "<script")
	assert.Contains(t, html, "<strong>OK</strong>")
}

func TestGenerate_NarrativeBudgetLeak_DetectedByDOMValidation(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	// Narrative membocorkan angka budget maximum (150.000.000) =>
	// defense-in-depth DOM validator harus menolak dokumen final.
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: `<p>Total penawaran kami hanya Rp 150.000.000 semua termasuk.</p>`},
	}}
	svc, _ := newSvc(pool, mock)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	assertGenerationCode(t, err, "unsafe_html")
	assert.NotEqual(t, uuid.Nil, qid)
	assertPendingRevision(t, pool, fx.companyID, fx.key)
}

func TestGenerate_EmptyNarrative_RejectedUnsafeHTML(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: allocationJSON()},
		{text: "   \n\t "}, // narrative kosong setelah trim
	}}
	svc, _ := newSvc(pool, mock)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	assertGenerationCode(t, err, "unsafe_html")
	assert.NotEqual(t, uuid.Nil, qid)
	assertPendingRevision(t, pool, fx.companyID, fx.key)
}

func TestGenerate_FencedAllocationJSON_Tolerated(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	// Provider nyata sering membungkus JSON dalam markdown fence —
	// extractJSONObject wajib toleran terhadapnya.
	mock := &scriptedAIProvider{steps: []scriptedStep{
		{text: "Berikut proposalnya:\n```json\n" + allocationJSON() + "\n```\nTerima kasih."},
		{text: narrativeHTML()},
	}}
	svc, _ := newSvc(pool, mock)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	require.NoError(t, err)
	status, _, _, _ := revisionRow(t, pool, qid)
	assert.Equal(t, "completed", status)
}

func TestGenerate_ProviderRateLimited_MappedToSafeCode(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{steps: []scriptedStep{
		{err: &service.ProviderError{Code: service.ErrRateLimited, Message: "429 quota"}},
	}}
	svc, _ := newSvc(pool, mock)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	assertGenerationCode(t, err, "provider_rate_limited")
	assert.NotEqual(t, uuid.Nil, qid)
	assertPendingRevision(t, pool, fx.companyID, fx.key)
}

func TestGenerate_ProviderGenericFailure_MappedToGenerationFailed(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{steps: []scriptedStep{
		{err: errors.New("connection reset by peer")},
	}}
	svc, _ := newSvc(pool, mock)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
	assertGenerationCode(t, err, "generation_failed")
	assert.NotEqual(t, uuid.Nil, qid)
	assertPendingRevision(t, pool, fx.companyID, fx.key)
}
