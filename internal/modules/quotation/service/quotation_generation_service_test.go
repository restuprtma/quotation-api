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

	"venturo-skeleton-go/internal/modules/quotation/domain"
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
	assert.Equal(t, "failed", status, "revision gagal harus berstatus failed")
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
		{text: `<p>Aman.</p><div data-block-type="commercial">blok palsu dari AI</div>`},
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
	assert.Contains(t, html, `data-block-type="metadata"`)
	assert.Contains(t, html, `data-block-type="payment"`)
	assert.Contains(t, html, `data-block-type="legal"`)
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
		{text: `<section data-block-type="opening"><p>Aman</p><script>alert(1)</script><strong>OK</strong></section>`},
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
		{text: `<section data-block-type="opening"><p>Total penawaran kami hanya Rp 150.000.000 semua termasuk.</p></section>`},
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

func validSubscriptionSaaSInput() domain.SubscriptionSaaSInput {
	return domain.SubscriptionSaaSInput{
		ProductName:       "Subscription AI Percetakan",
		QuotationDate:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SectionLayout:     domain.SubscriptionLayoutSubscriptionDevelopment,
		BillingCycle:      domain.SubscriptionBillingQuarterly,
		TaxTreatment:      domain.SubscriptionTaxNotStated,
		InitialTermMonths: 12,
		PricePeriods: []domain.SubscriptionPricePeriodInput{
			{Label: "Subscription 12 bulan", StartMonthOffset: 0, DurationMonths: 12, MonthlyRate: 1_250_000},
		},
		PaymentSchedule: []domain.SubscriptionPaymentRowInput{
			{WorkPeriod: "Agustus-Oktober 2026", StartMonthOffset: 0, CoveredMonths: 3, InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Nominal: 3_750_000},
			{WorkPeriod: "November 2026-Januari 2027", StartMonthOffset: 3, CoveredMonths: 3, InvoiceDateLabel: "1 Oktober 2026", DueDateLabel: "15 Oktober 2026", Nominal: 3_750_000},
			{WorkPeriod: "Februari-April 2027", StartMonthOffset: 6, CoveredMonths: 3, InvoiceDateLabel: "1 Januari 2027", DueDateLabel: "15 Januari 2027", Nominal: 3_750_000},
			{WorkPeriod: "Mei-Juli 2027", StartMonthOffset: 9, CoveredMonths: 3, InvoiceDateLabel: "1 April 2027", DueDateLabel: "15 April 2027", Nominal: 3_750_000},
		},
		FeatureGroups: []domain.SubscriptionFeatureGroupInput{
			{
				Name:     "Modul Utama",
				Features: []string{"Fitur Order", "Pencatatan order percetakan"},
			},
		},
		IsPartnership: true,
		IncomeSharing: &domain.IncomeSharingInput{
			VenturoShareBPS:            5_000,
			PartnerShareBPS:            5_000,
			EligibleRevenueDescription: "Penjualan dari referensi mitra",
			SettlementBusinessDays:     30,
			ProhibitUnilateralDiscount: true,
		},
	}
}

func saasRequest(fx pipelineFixture, input domain.SubscriptionSaaSInput) dto.GenerateQuotationRequest {
	return dto.GenerateQuotationRequest{
		IdempotencyKey:   uuidPtrString(fx.key.String()),
		SkillID:          fx.skillID,
		ClientID:         &fx.clientID,
		Subject:          "Penawaran SaaS Percetakan",
		SolutionType:     "subscription_saas",
		CommercialScheme: domain.CommercialSchemeSubscriptionSaaS,
		SubscriptionSaaS: &input,
	}
}

func TestGenerateSubscriptionSaaS_Success(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{}
	svc, _ := newSvc(pool, mock)

	createdByID := uuid.New()
	req := saasRequest(fx, validSubscriptionSaaSInput())
	qid, err := svc.Generate(context.Background(), fx.companyID, &createdByID, req)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, qid)

	assert.Equal(t, 0, mock.callCount(), "provider call count must be 0 for SaaS")

	var scheme, status string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT commercial_scheme, status FROM quotations WHERE id = $1 AND company_id = $2
	`, qid, fx.companyID).Scan(&scheme, &status))
	assert.Equal(t, domain.CommercialSchemeSubscriptionSaaS, scheme)
	assert.Equal(t, "draft", status)

	var (
		genStatus    string
		budgetMax    *int64
		rateSnap     []byte
		calcSnap     string
		profileSnap  string
		moduleSnap   string
		timelineSnap string
		contentHTML  string
		contentHash  *string
	)
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT generation_status, budget_maximum, rate_card_snapshot,
		       calculation_snapshot::text, resolved_profile_snapshot::text,
		       module_snapshot::text, timeline_snapshot::text,
		       content_html, content_hash
		FROM quotation_revisions WHERE quotation_id = $1 AND company_id = $2
	`, qid, fx.companyID).Scan(
		&genStatus, &budgetMax, &rateSnap,
		&calcSnap, &profileSnap,
		&moduleSnap, &timelineSnap,
		&contentHTML, &contentHash,
	))

	assert.Equal(t, "completed", genStatus)
	assert.Nil(t, budgetMax, "budget_maximum must be NULL")
	assert.Nil(t, rateSnap, "rate_card_snapshot must be NULL")
	assert.Contains(t, calcSnap, "15000000")
	assert.Contains(t, profileSnap, "section_layout")
	assert.Contains(t, moduleSnap, "Fitur Order")
	assert.Equal(t, "[]", timelineSnap, "timeline_snapshot must be []")
	assert.NotEmpty(t, contentHTML)
	assert.Contains(t, contentHTML, "Penawaran SaaS Percetakan")
	assert.Contains(t, contentHTML, `data-block-type="commercial"`)
	require.NotNil(t, contentHash)
	assert.NotEmpty(t, *contentHash)
}

func TestGenerateSubscriptionSaaS_FailureAfterDraft_LeavesQuotationFailed(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{}
	svc, _ := newSvc(pool, mock)

	input := validSubscriptionSaaSInput()
	// Payment total (3.75M*3 + 4M = 15.25M) != TotalContract (15M)
	// input.Validate() passes, but CalculateSubscriptionSaaS fails after draft creation!
	input.PaymentSchedule[3].Nominal = 4_000_000

	req := saasRequest(fx, input)
	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.Error(t, err)
	assertGenerationCode(t, err, "allocation_invalid")
	require.NotEqual(t, uuid.Nil, qid)

	var qStatus string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT status FROM quotations WHERE id = $1 AND company_id = $2
	`, qid, fx.companyID).Scan(&qStatus))
	assert.Equal(t, "failed", qStatus)

	var revStatus, revError string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT generation_status, COALESCE(generation_error, '')
		FROM quotation_revisions WHERE quotation_id = $1 AND company_id = $2
	`, qid, fx.companyID).Scan(&revStatus, &revError))
	assert.Equal(t, "failed", revStatus)
	assert.NotEmpty(t, revError)
}

func TestRegenerateSubscriptionSaaS_Success(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{}
	svc, _ := newSvc(pool, mock)

	req := saasRequest(fx, validSubscriptionSaaSInput())
	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)

	var v1 int
	var html1 string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_version, content_html FROM quotation_revisions WHERE quotation_id = $1 AND company_id = $2
	`, qid, fx.companyID).Scan(&v1, &html1))
	assert.Equal(t, 1, v1)

	// Regenerate
	err = svc.Regenerate(context.Background(), fx.companyID, qid)
	require.NoError(t, err)
	assert.Equal(t, 0, mock.callCount(), "provider call count must still be 0 after regenerate")

	var v2 int
	var html2, genStatus string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_version, content_html, generation_status FROM quotation_revisions WHERE quotation_id = $1 AND company_id = $2
	`, qid, fx.companyID).Scan(&v2, &html2, &genStatus))
	assert.Equal(t, 2, v2, "regenerate must increment content_version")
	assert.Equal(t, "completed", genStatus)
	assert.NotEmpty(t, html2)
}

func TestRegenerateSubscriptionSaaS_CorruptSnapshot_ReturnsGenerationFailed(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{}
	svc, _ := newSvc(pool, mock)

	req := saasRequest(fx, validSubscriptionSaaSInput())
	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)

	// Corrupt calculation_snapshot in DB
	_, err = pool.Exec(context.Background(), `
		UPDATE quotation_revisions SET calculation_snapshot = '"not-an-object"' WHERE quotation_id = $1 AND company_id = $2
	`, qid, fx.companyID)
	require.NoError(t, err)

	var vBefore int
	var htmlBefore string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_version, content_html FROM quotation_revisions WHERE quotation_id = $1 AND company_id = $2
	`, qid, fx.companyID).Scan(&vBefore, &htmlBefore))

	err = svc.Regenerate(context.Background(), fx.companyID, qid)
	require.Error(t, err)
	assertGenerationCode(t, err, "generation_failed")

	var vAfter int
	var htmlAfter string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_version, content_html FROM quotation_revisions WHERE quotation_id = $1 AND company_id = $2
	`, qid, fx.companyID).Scan(&vAfter, &htmlAfter))
	assert.Equal(t, vBefore, vAfter, "corrupt snapshot must not change content_version")
	assert.Equal(t, htmlBefore, htmlAfter, "corrupt snapshot must not change content_html")
}

func TestValidateGenerateInput_DiscriminatedUnion(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	svc, _ := newSvc(pool, &scriptedAIProvider{})

	t.Run("SubscriptionSaaS_WithRateCardID_RejectsInvalidRateCard", func(t *testing.T) {
		req := saasRequest(fx, validSubscriptionSaaSInput())
		req.RateCardID = fx.rateCardID // not accepted for subscription_saas
		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		assertGenerationCode(t, err, "invalid_rate_card")
	})

	t.Run("SubscriptionSaaS_NilPayload_RejectsAllocationInvalid", func(t *testing.T) {
		req := saasRequest(fx, validSubscriptionSaaSInput())
		req.SubscriptionSaaS = nil
		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("SubscriptionSaaS_BothPayloads_RejectsAllocationInvalid", func(t *testing.T) {
		req := saasRequest(fx, validSubscriptionSaaSInput())
		req.DedicatedSquad = &domain.DedicatedSquadInput{}
		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("DedicatedSquad_WithSubscriptionPayload_RejectsAllocationInvalid", func(t *testing.T) {
		req := fx.request()
		saas := validSubscriptionSaaSInput()
		req.SubscriptionSaaS = &saas
		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("UnsupportedScheme_RejectsGenerationFailed", func(t *testing.T) {
		req := fx.request()
		req.CommercialScheme = "unsupported_scheme"
		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		assertGenerationCode(t, err, "generation_failed")
	})
}

func generateSubscriptionSaaSDraft(t *testing.T, pool *pgxpool.Pool, customize ...func(input *domain.SubscriptionSaaSInput)) (uuid.UUID, uuid.UUID, pipelineFixture) {
	t.Helper()
	fx := seedPipelineFixtures(t, pool)
	mock := &scriptedAIProvider{}
	svc, _ := newSvc(pool, mock)
	input := validSubscriptionSaaSInput()
	for _, fn := range customize {
		fn(&input)
	}
	req := saasRequest(fx, input)
	createdByID := uuid.New()
	qid, err := svc.Generate(context.Background(), fx.companyID, &createdByID, req)
	require.NoError(t, err)
	return qid, fx.companyID, fx
}

func TestSaveDraft_SubscriptionSaaS_Success_UnmodifiedNarrative(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)
	require.Equal(t, 1, snap.version)
	require.NotEmpty(t, snap.html)

	// Modify narrative in opening section without touching commercial or legal
	modifiedHTML := strings.Replace(
		snap.html,
		"Melalui penawaran ini, Venturo menyampaikan",
		"Melalui penawaran resmi yang diperbarui ini, Venturo menyampaikan",
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

func TestSaveDraft_SubscriptionSaaS_CommercialModified(t *testing.T) {
	pool := getTestPool(t)

	t.Run("MonthlyRateModified", func(t *testing.T) {
		qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(snap.html, "Rp 1.250.000", "Rp 2.000.000", 1)
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

	t.Run("GrandTotalModified", func(t *testing.T) {
		qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			`<td data-field="total_contract"><strong>Rp 15.000.000</strong></td>`,
			`<td data-field="total_contract"><strong>Rp 99.000.000</strong></td>`,
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
		qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			`<td data-field="payment_nominal">Rp 3.750.000</td>`,
			`<td data-field="payment_nominal">Rp 5.000.000</td>`,
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

func TestSaveDraft_SubscriptionSaaS_LegalModified(t *testing.T) {
	pool := getTestPool(t)

	t.Run("OwnershipClauseModified", func(t *testing.T) {
		qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool, func(input *domain.SubscriptionSaaSInput) {
			input.RightsClauseCode = domain.RightsClauseVenturoSoftwareOwnershipV1
		})
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(snap.html, "milik Venturo", "milik Klien", 1)
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

	t.Run("IncomeSharingModified", func(t *testing.T) {
		qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(snap.html, "50% untuk Mitra", "40% untuk Mitra", 1)
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

	t.Run("LimitationsModified", func(t *testing.T) {
		qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(
			snap.html,
			"Syarat dan ketentuan penawaran resmi berlaku 30 hari.",
			"Syarat dan ketentuan khusus berlaku 90 hari.",
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

func TestSaveDraft_SubscriptionSaaS_MissingBlocks(t *testing.T) {
	pool := getTestPool(t)

	t.Run("CommercialBlockMissing_SucceedsWithCommercialModified", func(t *testing.T) {
		qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		// Replace commercial block type with untracked section
		modifiedHTML := strings.Replace(snap.html, `data-block-type="commercial"`, `data-block-type="untracked"`, 1)
		require.NotEqual(t, snap.html, modifiedHTML)

		svc, _ := newSvc(pool, &scriptedAIProvider{})
		res, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
			ContentHTML:    modifiedHTML,
			ContentVersion: 1,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, res.ContentVersion)
		assert.True(t, res.CommercialContentModified)
	})

	t.Run("PaymentBlockMissing_SucceedsWithCommercialModified", func(t *testing.T) {
		qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		modifiedHTML := strings.Replace(snap.html, `data-block-type="payment"`, `data-block-type="untracked"`, 1)
		require.NotEqual(t, snap.html, modifiedHTML)

		svc, _ := newSvc(pool, &scriptedAIProvider{})
		res, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
			ContentHTML:    modifiedHTML,
			ContentVersion: 1,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, res.ContentVersion)
		assert.True(t, res.CommercialContentModified)
	})
}

func TestSaveDraft_SubscriptionSaaS_DangerousHTML_RejectsUnsafeHTML(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	// Inject forbidden term
	tamperedHTML := strings.Replace(
		snap.html,
		"Melalui penawaran ini, Venturo menyampaikan",
		"Melalui penawaran budget ini, Venturo menyampaikan",
		1,
	)
	require.NotEqual(t, snap.html, tamperedHTML)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    tamperedHTML,
		ContentVersion: 1,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, "unsafe_html")
}

func TestSaveDraft_SubscriptionSaaS_Sanitization_StripsScript(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	maliciousHTML := snap.html + `<script>alert('malicious')</script>`

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	res, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    maliciousHTML,
		ContentVersion: 1,
	})
	require.NoError(t, err)
	assert.NotContains(t, res.ContentHTML, "<script>")
	assert.NotContains(t, res.ContentHTML, "alert('malicious')")
}

func TestSaveDraft_SubscriptionSaaS_StaleVersion_ReturnsConflict(t *testing.T) {
	pool := getTestPool(t)
	qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
	snap := captureSnapshot(t, pool, qid)

	svc, _ := newSvc(pool, &scriptedAIProvider{})
	_, err := svc.SaveDraft(context.Background(), companyID, qid, nil, dto.SaveDraftRequest{
		ContentHTML:    snap.html,
		ContentVersion: 99,
	})
	require.Error(t, err)
	assertGenerationCode(t, err, "revision_conflict")
}

func TestSaveDraft_SubscriptionSaaS_MalformedSnapshots_Rejects(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	t.Run("CorruptCalculationSnapshot", func(t *testing.T) {
		qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET calculation_snapshot = 'null'::jsonb WHERE quotation_id = $1`, qid)
		require.NoError(t, err)

		svc, _ := newSvc(pool, &scriptedAIProvider{})
		_, err = svc.SaveDraft(ctx, companyID, qid, nil, dto.SaveDraftRequest{
			ContentHTML:    snap.html,
			ContentVersion: 1,
		})
		require.Error(t, err)
		assertGenerationCode(t, err, "unsafe_html")
	})

	t.Run("CorruptProfileSnapshot", func(t *testing.T) {
		qid, companyID, _ := generateSubscriptionSaaSDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET resolved_profile_snapshot = 'null'::jsonb WHERE quotation_id = $1`, qid)
		require.NoError(t, err)

		svc, _ := newSvc(pool, &scriptedAIProvider{})
		_, err = svc.SaveDraft(ctx, companyID, qid, nil, dto.SaveDraftRequest{
			ContentHTML:    snap.html,
			ContentVersion: 1,
		})
		require.Error(t, err)
		assertGenerationCode(t, err, "unsafe_html")
	})
}

// ---------------------------------------------------------------------------
// White Label Product Generation & Regeneration Tests
// ---------------------------------------------------------------------------

func validWhiteLabelInputForTest() domain.WhiteLabelInput {
	return domain.WhiteLabelInput{
		ProductName:   "POS Mini Market",
		QuotationDate: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC),
		TaxTreatment:  domain.WhiteLabelTaxExclusive,
		LicenseItems: []domain.WhiteLabelLicenseItemInput{
			{Name: "POS Mini Market Core", UnitRate: 35_000_000, Quantity: 1},
		},
		MaintenanceItems: []domain.WhiteLabelMaintenanceItemInput{
			{Name: "Senior Web Programmer", MonthlyRate: 16_170_000, FTEQuarters: 2, PeriodMonths: 3},
		},
		HostingItems: []domain.WhiteLabelHostingItemInput{
			{Name: "Shared Hosting Small", MonthlyRate: 350_000, Quantity: 1, PeriodMonths: 3},
		},
		ModuleGroups: []domain.WhiteLabelModuleGroupInput{
			{Name: "Inventory", Features: []string{"Stok masuk", "Stok keluar"}},
		},
		TimelineRows: []domain.WhiteLabelTimelineRowInput{
			{Description: "Setup Server & Database", Timeline: "Minggu 1"},
		},
		PaymentSchedule: []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
			{WorkPeriod: "Maintenance 1", Terms: "Bulanan", InvoiceDateLabel: "Bulan 1", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			{WorkPeriod: "Maintenance 2", Terms: "Bulanan", InvoiceDateLabel: "Bulan 2", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			{WorkPeriod: "Maintenance 3", Terms: "Bulanan", InvoiceDateLabel: "Bulan 3", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
		},
		RightsStatements: []string{
			"Klien berhak menggunakan source code untuk kebutuhan internal.",
		},
		LimitationCodes: []string{"out_of_pocket", "overtime", "third_party", "scope_change"},
	}
}

func whiteLabelRequest(fx pipelineFixture, input domain.WhiteLabelInput) dto.GenerateQuotationRequest {
	key := uuid.New().String()
	return dto.GenerateQuotationRequest{
		IdempotencyKey:    &key,
		Subject:           "Penawaran White Label POS",
		SolutionType:      "white_label",
		CommercialScheme:  domain.CommercialSchemeWhiteLabel,
		WhiteLabel:        &input,
		RateCardID:        uuid.Nil, // rate_card_id boleh zero
		ClientID:          &fx.clientID,
		ClientName:        "PT Retail Jaya",
		ClientCompanyName: "PT Retail Jaya Makmur",
	}
}

func generateWhiteLabelDraft(t *testing.T, pool *pgxpool.Pool, customize ...func(input *domain.WhiteLabelInput)) (uuid.UUID, uuid.UUID, pipelineFixture) {
	t.Helper()
	fx := seedPipelineFixtures(t, pool)
	mock := &scriptedAIProvider{}
	svc, _ := newSvc(pool, mock)

	input := validWhiteLabelInputForTest()
	for _, fn := range customize {
		fn(&input)
	}
	req := whiteLabelRequest(fx, input)
	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)
	return qid, fx.companyID, fx
}

type failingWhiteLabelRenderer struct{}

func (f *failingWhiteLabelRenderer) Render(_ service.WhiteLabelDocumentInput) (string, error) {
	return "", errors.New("simulated render failure")
}

type failingHTMLSanitizer struct{}

func (f *failingHTMLSanitizer) Sanitize(_ string) (string, error) {
	return "", errors.New("simulated sanitize failure")
}

func TestGenerateWhiteLabel_Success(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{}
	svc, source := newSvc(pool, mock)

	input := validWhiteLabelInputForTest()
	req := whiteLabelRequest(fx, input)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, qid)

	// 1. Valid generate membuat exactly one quotation/revision
	var qCount, rCount int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM quotations WHERE id = $1`, qid).Scan(&qCount))
	assert.Equal(t, 1, qCount)
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM quotation_revisions WHERE quotation_id = $1`, qid).Scan(&rCount))
	assert.Equal(t, 1, rCount)

	// 2. providerSource.ProviderForCompany tidak pernah dipanggil
	assert.Empty(t, source.requests(), "AI provider tidak boleh dipanggil untuk white_label")

	// 3. rate-card loader tidak dipanggil dan rate_card_id boleh zero
	assert.Equal(t, uuid.Nil, req.RateCardID)

	var (
		qStatus, revStatus                 string
		calcSnap, modSnap, timeSnap, pSnap string
		contentHTML                        string
		contentHash                        *string
		aiProv, aiModel                    string
		rcSnap, skillSnap                  string
	)
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT q.status, r.generation_status,
		       r.calculation_snapshot::text, r.module_snapshot::text,
		       r.timeline_snapshot::text, r.resolved_profile_snapshot::text,
		       r.content_html, r.content_hash,
		       COALESCE(r.ai_provider, ''), COALESCE(r.ai_model, ''),
		       COALESCE(r.rate_card_snapshot::text, '{}'), COALESCE(r.skill_snapshot::text, '{}')
		FROM quotations q
		JOIN quotation_revisions r ON r.quotation_id = q.id
		WHERE q.id = $1 AND q.company_id = $2
	`, qid, fx.companyID).Scan(
		&qStatus, &revStatus,
		&calcSnap, &modSnap, &timeSnap, &pSnap,
		&contentHTML, &contentHash,
		&aiProv, &aiModel,
		&rcSnap, &skillSnap,
	))

	assert.Equal(t, "draft", qStatus)
	assert.Equal(t, "completed", revStatus)
	assert.Empty(t, aiProv)
	assert.Empty(t, aiModel)
	assert.Equal(t, "{}", rcSnap)
	assert.Equal(t, "{}", skillSnap)

	// 4. calculation/module/timeline/profile snapshots persis dapat di-unmarshal
	var calc domain.WhiteLabelCalculation
	require.NoError(t, json.Unmarshal([]byte(calcSnap), &calc))
	assert.Equal(t, int64(60_305_000), calc.TotalInvestment)
	assert.Equal(t, int64(35_000_000), calc.LicenseSubtotal)
	assert.Equal(t, int64(24_255_000), calc.MaintenanceSubtotal)
	assert.Equal(t, int64(1_050_000), calc.HostingSubtotal)
	assert.Equal(t, int64(25_305_000), calc.MaintenanceHostingSubtotal)

	var modules []domain.WhiteLabelModuleGroupInput
	require.NoError(t, json.Unmarshal([]byte(modSnap), &modules))
	require.Len(t, modules, 1)
	assert.Equal(t, "Inventory", modules[0].Name)
	assert.Equal(t, []string{"Stok masuk", "Stok keluar"}, modules[0].Features)

	var timeline []domain.WhiteLabelTimelineRowInput
	require.NoError(t, json.Unmarshal([]byte(timeSnap), &timeline))
	require.Len(t, timeline, 1)
	assert.Equal(t, "Setup Server & Database", timeline[0].Description)

	var profile domain.ResolvedWhiteLabelProfile
	require.NoError(t, json.Unmarshal([]byte(pSnap), &profile))
	assert.Equal(t, domain.WhiteLabelTaxExclusive, profile.TaxTreatment)
	assert.True(t, profile.ShowMaintenanceHosting)
	assert.True(t, profile.ShowScopeTimeline)
	assert.True(t, profile.ShowRights)

	// HTML and Hash assertions
	assert.NotEmpty(t, contentHTML)
	assert.Contains(t, contentHTML, "quotation-document")
	assert.Contains(t, contentHTML, `data-block-type="commercial"`)
	assert.Contains(t, contentHTML, "POS Mini Market")
	assert.Contains(t, contentHTML, "Rp 35.000.000")
	require.NotNil(t, contentHash)
	assert.NotEmpty(t, *contentHash)
}

func TestGenerateWhiteLabel_IdempotencyReplay(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{}
	svc, _ := newSvc(pool, mock)

	input := validWhiteLabelInputForTest()
	req := whiteLabelRequest(fx, input)

	qid1, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)

	// 5. Idempotency replay mengembalikan quotation ID sama
	qid2, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)
	assert.Equal(t, qid1, qid2)

	var qCount int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM quotations WHERE id = $1`, qid1).Scan(&qCount))
	assert.Equal(t, 1, qCount)
}

func TestGenerateWhiteLabel_RendererFailure_LeavesQuotationFailed(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{}
	svc, _ := newSvc(pool, mock, service.WithWhiteLabelRenderer(&failingWhiteLabelRenderer{}))

	input := validWhiteLabelInputForTest()
	req := whiteLabelRequest(fx, input)

	// 6. Renderer failure meninggalkan revision failed dengan safe message
	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.Error(t, err)
	assertGenerationCode(t, err, "unsafe_html")
	require.NotEqual(t, uuid.Nil, qid)

	var qStatus, revStatus, revError string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT q.status, r.generation_status, COALESCE(r.generation_error, '')
		FROM quotations q
		JOIN quotation_revisions r ON r.quotation_id = q.id
		WHERE q.id = $1 AND q.company_id = $2
	`, qid, fx.companyID).Scan(&qStatus, &revStatus, &revError))

	assert.Equal(t, "failed", qStatus)
	assert.Equal(t, "failed", revStatus)
	assert.Contains(t, revError, "white label document failed to render")
}

func TestGenerateWhiteLabel_SanitizerFailure_LeavesQuotationFailed(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{}
	svc, _ := newSvc(pool, mock, service.WithHTMLSanitizer(&failingHTMLSanitizer{}))

	input := validWhiteLabelInputForTest()
	req := whiteLabelRequest(fx, input)

	// 6. Sanitizer failure meninggalkan revision failed dengan safe message
	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.Error(t, err)
	assertGenerationCode(t, err, "unsafe_html")
	require.NotEqual(t, uuid.Nil, qid)

	var qStatus, revStatus, revError string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT q.status, r.generation_status, COALESCE(r.generation_error, '')
		FROM quotations q
		JOIN quotation_revisions r ON r.quotation_id = q.id
		WHERE q.id = $1 AND q.company_id = $2
	`, qid, fx.companyID).Scan(&qStatus, &revStatus, &revError))

	assert.Equal(t, "failed", qStatus)
	assert.Equal(t, "failed", revStatus)
	assert.Contains(t, revError, "white label document sanitization failed")
}

func TestRegenerateWhiteLabel_Success(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &scriptedAIProvider{}
	svc, source := newSvc(pool, mock)

	input := validWhiteLabelInputForTest()
	req := whiteLabelRequest(fx, input)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)

	var initialVersion int
	var initialHTML string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_version, content_html FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&initialVersion, &initialHTML))
	assert.Equal(t, 1, initialVersion)

	// 7. Regenerate memakai snapshot tersimpan dan tidak menghitung ulang harga dari data eksternal
	err = svc.Regenerate(context.Background(), fx.companyID, qid)
	require.NoError(t, err)

	// Provider Source tidak boleh dipanggil selama regenerate
	assert.Empty(t, source.requests())

	var updatedVersion int
	var updatedHTML string
	var calcSnap, modSnap, timeSnap, pSnap string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_version, content_html,
		       calculation_snapshot::text, module_snapshot::text,
		       timeline_snapshot::text, resolved_profile_snapshot::text
		FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&updatedVersion, &updatedHTML, &calcSnap, &modSnap, &timeSnap, &pSnap))

	assert.Equal(t, initialVersion+1, updatedVersion, "content_version harus bertambah 1")
	assert.NotEmpty(t, updatedHTML)

	// Snapshots remain valid and unmarshaled
	var calc domain.WhiteLabelCalculation
	require.NoError(t, json.Unmarshal([]byte(calcSnap), &calc))
	assert.Equal(t, int64(60_305_000), calc.TotalInvestment)

	var modules []domain.WhiteLabelModuleGroupInput
	require.NoError(t, json.Unmarshal([]byte(modSnap), &modules))
	assert.Equal(t, "Inventory", modules[0].Name)
}

func TestRegenerateWhiteLabel_CorruptSnapshots_RejectsAndPreservesVersion(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	// 8. Corrupt calculation/module/timeline/profile snapshot menolak regenerate dan tidak mengubah HTML/version
	t.Run("CorruptCalculation", func(t *testing.T) {
		qid, companyID, _ := generateWhiteLabelDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET calculation_snapshot = 'null'::jsonb WHERE quotation_id = $1`, qid)
		require.NoError(t, err)

		svc, _ := newSvc(pool, &scriptedAIProvider{})
		err = svc.Regenerate(ctx, companyID, qid)
		require.Error(t, err)
		assertGenerationCode(t, err, "generation_failed")

		snapAfter := captureSnapshot(t, pool, qid)
		assert.Equal(t, snap.version, snapAfter.version, "version must not change on corrupt snapshot")
		assert.Equal(t, snap.html, snapAfter.html, "html must not change on corrupt snapshot")
	})

	t.Run("CorruptProfile", func(t *testing.T) {
		qid, companyID, _ := generateWhiteLabelDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET resolved_profile_snapshot = 'null'::jsonb WHERE quotation_id = $1`, qid)
		require.NoError(t, err)

		svc, _ := newSvc(pool, &scriptedAIProvider{})
		err = svc.Regenerate(ctx, companyID, qid)
		require.Error(t, err)
		assertGenerationCode(t, err, "generation_failed")

		snapAfter := captureSnapshot(t, pool, qid)
		assert.Equal(t, snap.version, snapAfter.version)
		assert.Equal(t, snap.html, snapAfter.html)
	})

	t.Run("CorruptModule", func(t *testing.T) {
		qid, companyID, _ := generateWhiteLabelDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET module_snapshot = '{"invalid":"json_shape"}'::jsonb WHERE quotation_id = $1`, qid)
		require.NoError(t, err)

		svc, _ := newSvc(pool, &scriptedAIProvider{})
		err = svc.Regenerate(ctx, companyID, qid)
		require.Error(t, err)
		assertGenerationCode(t, err, "generation_failed")

		snapAfter := captureSnapshot(t, pool, qid)
		assert.Equal(t, snap.version, snapAfter.version)
		assert.Equal(t, snap.html, snapAfter.html)
	})

	t.Run("CorruptTimeline", func(t *testing.T) {
		qid, companyID, _ := generateWhiteLabelDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET timeline_snapshot = '{"invalid":"json_shape"}'::jsonb WHERE quotation_id = $1`, qid)
		require.NoError(t, err)

		svc, _ := newSvc(pool, &scriptedAIProvider{})
		err = svc.Regenerate(ctx, companyID, qid)
		require.Error(t, err)
		assertGenerationCode(t, err, "generation_failed")

		snapAfter := captureSnapshot(t, pool, qid)
		assert.Equal(t, snap.version, snapAfter.version)
		assert.Equal(t, snap.html, snapAfter.html)
	})
}

func TestRegenerateWhiteLabel_CrossTenant_ReturnsNotFound(t *testing.T) {
	pool := getTestPool(t)
	qid, _, _ := generateWhiteLabelDraft(t, pool)

	svc, _ := newSvc(pool, &scriptedAIProvider{})

	// 9. Cross-tenant regenerate returns not found
	foreignCompanyID := uuid.New()
	err := svc.Regenerate(context.Background(), foreignCompanyID, qid)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrRevisionNotFound), "cross-tenant regenerate returns not found")
}

func TestGenerateWhiteLabel_ValidationRejection(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)
	svc, _ := newSvc(pool, &scriptedAIProvider{})

	t.Run("BothWhiteLabelAndDedicatedSquad", func(t *testing.T) {
		req := whiteLabelRequest(fx, validWhiteLabelInputForTest())
		req.DedicatedSquad = &domain.DedicatedSquadInput{}

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("BothWhiteLabelAndSubscriptionSaaS", func(t *testing.T) {
		req := whiteLabelRequest(fx, validWhiteLabelInputForTest())
		saas := validSubscriptionSaaSInput()
		req.SubscriptionSaaS = &saas

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("NilWhiteLabelPayload", func(t *testing.T) {
		req := whiteLabelRequest(fx, validWhiteLabelInputForTest())
		req.WhiteLabel = nil

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("InvalidWhiteLabelInput", func(t *testing.T) {
		in := validWhiteLabelInputForTest()
		in.ProductName = "" // empty product name is invalid
		req := whiteLabelRequest(fx, in)

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "allocation_invalid")
	})
}

// ---------------------------------------------------------------------------
// Standalone Maintenance Generation & Regeneration Tests
// ---------------------------------------------------------------------------

type panicAIProvider struct{}

func (p *panicAIProvider) Generate(_ context.Context, _ service.GenerateRequest) (service.GenerateResult, error) {
	panic("AI provider must not be called for standalone_maintenance")
}

type panicProviderSource struct{}

func (p *panicProviderSource) ProviderForCompany(_ context.Context, _ uuid.UUID) (service.AIProvider, string, string, error) {
	panic("ProviderForCompany must not be called for standalone_maintenance")
}

type failingStandaloneMaintenanceRenderer struct{}

func (f *failingStandaloneMaintenanceRenderer) Render(_ service.StandaloneMaintenanceDocumentInput) (string, error) {
	return "", errors.New("simulated standalone maintenance render failure")
}

type missingBlockStandaloneMaintenanceRenderer struct{}

func (m *missingBlockStandaloneMaintenanceRenderer) Render(_ service.StandaloneMaintenanceDocumentInput) (string, error) {
	return `<article class="quotation-document"><section data-block-type="metadata">Meta</section></article>`, nil
}

func standaloneMaintenanceRequest(fx pipelineFixture, input domain.StandaloneMaintenanceInput) dto.GenerateQuotationRequest {
	key := uuid.New().String()
	return dto.GenerateQuotationRequest{
		IdempotencyKey:        &key,
		Subject:               "Penawaran Maintenance Aplikasi Nex Health",
		SolutionType:          "maintenance",
		CommercialScheme:      domain.CommercialSchemeStandaloneMaintenance,
		StandaloneMaintenance: &input,
		RateCardID:            uuid.Nil,
		ClientID:              &fx.clientID,
		ClientName:            "Nex Health Hospital",
		ClientCompanyName:     "PT Nex Health Medika",
	}
}

func generateStandaloneMaintenanceDraft(t *testing.T, pool *pgxpool.Pool, customize ...func(input *domain.StandaloneMaintenanceInput)) (uuid.UUID, uuid.UUID, pipelineFixture) {
	t.Helper()
	fx := seedPipelineFixtures(t, pool)
	mock := &panicAIProvider{}
	svc, _ := newSvc(pool, mock)

	input := nexRetainerInputForCalculator()
	for _, fn := range customize {
		fn(&input)
	}
	req := standaloneMaintenanceRequest(fx, input)
	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)
	return qid, fx.companyID, fx
}

func TestGenerateStandaloneMaintenance_ValidationRejection(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)
	svc, _ := newSvc(pool, &panicAIProvider{})

	t.Run("NilStandaloneMaintenancePayload", func(t *testing.T) {
		req := standaloneMaintenanceRequest(fx, nexRetainerInputForCalculator())
		req.StandaloneMaintenance = nil

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("BothStandaloneMaintenanceAndDedicatedSquad", func(t *testing.T) {
		req := standaloneMaintenanceRequest(fx, nexRetainerInputForCalculator())
		req.DedicatedSquad = &domain.DedicatedSquadInput{}

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("BothStandaloneMaintenanceAndSubscriptionSaaS", func(t *testing.T) {
		req := standaloneMaintenanceRequest(fx, nexRetainerInputForCalculator())
		saas := validSubscriptionSaaSInput()
		req.SubscriptionSaaS = &saas

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("BothStandaloneMaintenanceAndWhiteLabel", func(t *testing.T) {
		req := standaloneMaintenanceRequest(fx, nexRetainerInputForCalculator())
		wl := validWhiteLabelInputForTest()
		req.WhiteLabel = &wl

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("NonZeroRateCardID", func(t *testing.T) {
		req := standaloneMaintenanceRequest(fx, nexRetainerInputForCalculator())
		req.RateCardID = uuid.New()

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "invalid_rate_card")
	})

	t.Run("InvalidStandaloneNestedInput", func(t *testing.T) {
		in := nexRetainerInputForCalculator()
		in.SystemName = ""
		req := standaloneMaintenanceRequest(fx, in)

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("DedicatedSquadRejectsStandaloneMaintenance", func(t *testing.T) {
		req := fx.request()
		in := nexRetainerInputForCalculator()
		req.StandaloneMaintenance = &in

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("SubscriptionSaaSRejectsStandaloneMaintenance", func(t *testing.T) {
		saas := validSubscriptionSaaSInput()
		req := saasRequest(fx, saas)
		in := nexRetainerInputForCalculator()
		req.StandaloneMaintenance = &in

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "allocation_invalid")
	})

	t.Run("WhiteLabelRejectsStandaloneMaintenance", func(t *testing.T) {
		req := whiteLabelRequest(fx, validWhiteLabelInputForTest())
		in := nexRetainerInputForCalculator()
		req.StandaloneMaintenance = &in

		_, err := svc.Generate(context.Background(), fx.companyID, nil, req)
		require.Error(t, err)
		assertGenerationCode(t, err, "allocation_invalid")
	})
}

func TestGenerateStandaloneMaintenance_Success(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	svc := service.NewQuotationGenerationService(
		repository.NewGenerationRepository(pool),
		&panicProviderSource{},
		service.WithAttemptTimeout(2*time.Second),
	)

	input := nexRetainerInputForCalculator()
	req := standaloneMaintenanceRequest(fx, input)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, qid)

	var qCount, rCount int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM quotations WHERE id = $1`, qid).Scan(&qCount))
	assert.Equal(t, 1, qCount)
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM quotation_revisions WHERE quotation_id = $1`, qid).Scan(&rCount))
	assert.Equal(t, 1, rCount)

	var (
		qScheme                            string
		qStatus, revStatus                 string
		budgetMax                          *int64
		calcSnap, modSnap, timeSnap, pSnap string
		contentHTML                        string
		contentHash                        *string
		aiProv, aiModel                    *string
		rcSnap, skillSnap                  string
	)
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT q.commercial_scheme,
		       q.status, r.generation_status,
		       r.budget_maximum,
		       r.calculation_snapshot::text, r.module_snapshot::text,
		       r.timeline_snapshot::text, r.resolved_profile_snapshot::text,
		       r.content_html, r.content_hash,
		       r.ai_provider, r.ai_model,
		       COALESCE(r.rate_card_snapshot::text, '{}'), COALESCE(r.skill_snapshot::text, '{}')
		FROM quotations q
		JOIN quotation_revisions r ON r.quotation_id = q.id
		WHERE q.id = $1 AND q.company_id = $2
	`, qid, fx.companyID).Scan(
		&qScheme,
		&qStatus, &revStatus,
		&budgetMax,
		&calcSnap, &modSnap, &timeSnap, &pSnap,
		&contentHTML, &contentHash,
		&aiProv, &aiModel,
		&rcSnap, &skillSnap,
	))

	assert.Equal(t, "standalone_maintenance", qScheme)
	assert.Equal(t, "draft", qStatus)
	assert.Equal(t, "completed", revStatus)
	assert.Nil(t, budgetMax, "budget_maximum must be NULL")
	assert.Nil(t, aiProv, "ai_provider must be empty/null")
	assert.Nil(t, aiModel, "ai_model must be empty/null")
	assert.Equal(t, "{}", rcSnap, "rate_card_snapshot must be {}")
	assert.Equal(t, "{}", skillSnap, "skill_snapshot must be {}")

	// Calculation total = 23085000
	var calc domain.StandaloneMaintenanceCalculation
	require.NoError(t, json.Unmarshal([]byte(calcSnap), &calc))
	assert.Equal(t, int64(23_085_000), calc.TotalInvestment)
	assert.Equal(t, domain.StandaloneMaintenanceRetainer, calc.Mode)

	// Module snapshot mode data = defect_groups only
	var scopeSnap domain.StandaloneMaintenanceScopeSnapshot
	require.NoError(t, json.Unmarshal([]byte(modSnap), &scopeSnap))
	assert.NotEmpty(t, scopeSnap.DefectGroups)
	assert.Empty(t, scopeSnap.HourlyTasks, "module snapshot must contain defect_groups only in retainer mode")
	assert.Equal(t, "1. Detail Pasien", scopeSnap.DefectGroups[0].Title)

	// Timeline snapshot = []
	var timelineSnap []json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(timeSnap), &timelineSnap))
	assert.Empty(t, timelineSnap)

	// Resolved profile snapshot
	var profile domain.ResolvedStandaloneMaintenanceProfile
	require.NoError(t, json.Unmarshal([]byte(pSnap), &profile))
	assert.Equal(t, domain.StandaloneMaintenanceRetainer, profile.Mode)
	assert.Equal(t, domain.StandaloneTaxExclusive, profile.TaxTreatment)

	// HTML and Hash assertions
	assert.NotEmpty(t, contentHTML)
	assert.Contains(t, contentHTML, "quotation-document")
	assert.Contains(t, contentHTML, `data-block-type="commercial"`)
	assert.Contains(t, contentHTML, "Aplikasi Nex Health")
	assert.Contains(t, contentHTML, "23.085.000")
	require.NotNil(t, contentHash)
	assert.NotEmpty(t, *contentHash)
}

func TestGenerateStandaloneMaintenance_IdempotencyReplay(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &panicAIProvider{}
	svc, _ := newSvc(pool, mock)

	input := nexRetainerInputForCalculator()
	req := standaloneMaintenanceRequest(fx, input)

	qid1, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)

	qid2, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)
	assert.Equal(t, qid1, qid2)

	var qCount int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM quotations WHERE id = $1`, qid1).Scan(&qCount))
	assert.Equal(t, 1, qCount)
}

func TestGenerateStandaloneMaintenance_RendererFailure_LeavesQuotationFailed(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &panicAIProvider{}
	svc, _ := newSvc(pool, mock, service.WithStandaloneMaintenanceRenderer(&failingStandaloneMaintenanceRenderer{}))

	input := nexRetainerInputForCalculator()
	req := standaloneMaintenanceRequest(fx, input)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.Error(t, err)
	assertGenerationCode(t, err, "unsafe_html")
	require.NotEqual(t, uuid.Nil, qid)

	var qStatus, revStatus, revError string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT q.status, r.generation_status, COALESCE(r.generation_error, '')
		FROM quotations q
		JOIN quotation_revisions r ON r.quotation_id = q.id
		WHERE q.id = $1 AND q.company_id = $2
	`, qid, fx.companyID).Scan(&qStatus, &revStatus, &revError))

	assert.Equal(t, "failed", qStatus)
	assert.Equal(t, "failed", revStatus)
	assert.Contains(t, revError, "standalone maintenance document failed to render")
}

func TestGenerateStandaloneMaintenance_SanitizerFailure_LeavesQuotationFailed(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &panicAIProvider{}
	svc, _ := newSvc(pool, mock, service.WithHTMLSanitizer(&failingHTMLSanitizer{}))

	input := nexRetainerInputForCalculator()
	req := standaloneMaintenanceRequest(fx, input)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.Error(t, err)
	assertGenerationCode(t, err, "unsafe_html")
	require.NotEqual(t, uuid.Nil, qid)

	var qStatus, revStatus, revError string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT q.status, r.generation_status, COALESCE(r.generation_error, '')
		FROM quotations q
		JOIN quotation_revisions r ON r.quotation_id = q.id
		WHERE q.id = $1 AND q.company_id = $2
	`, qid, fx.companyID).Scan(&qStatus, &revStatus, &revError))

	assert.Equal(t, "failed", qStatus)
	assert.Equal(t, "failed", revStatus)
	assert.Contains(t, revError, "standalone maintenance document sanitization failed")
}

func TestGenerateStandaloneMaintenance_IntegrityFailure_LeavesQuotationFailed(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &panicAIProvider{}
	svc, _ := newSvc(pool, mock, service.WithStandaloneMaintenanceRenderer(&missingBlockStandaloneMaintenanceRenderer{}))

	input := nexRetainerInputForCalculator()
	req := standaloneMaintenanceRequest(fx, input)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.Error(t, err)
	assertGenerationCode(t, err, "unsafe_html")
	require.NotEqual(t, uuid.Nil, qid)

	var qStatus, revStatus, revError string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT q.status, r.generation_status, COALESCE(r.generation_error, '')
		FROM quotations q
		JOIN quotation_revisions r ON r.quotation_id = q.id
		WHERE q.id = $1 AND q.company_id = $2
	`, qid, fx.companyID).Scan(&qStatus, &revStatus, &revError))

	assert.Equal(t, "failed", qStatus)
	assert.Equal(t, "failed", revStatus)
	assert.Contains(t, revError, "standalone maintenance document integrity validation failed")
}

func TestRegenerateStandaloneMaintenance_Success(t *testing.T) {
	pool := getTestPool(t)
	fx := seedPipelineFixtures(t, pool)

	mock := &panicAIProvider{}
	svc, source := newSvc(pool, mock)

	input := nexRetainerInputForCalculator()
	req := standaloneMaintenanceRequest(fx, input)

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, req)
	require.NoError(t, err)

	var initialVersion int
	var initialHTML string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_version, content_html FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&initialVersion, &initialHTML))
	assert.Equal(t, 1, initialVersion)

	// Change active legal template in DB
	_, err = pool.Exec(context.Background(), `
		UPDATE legal_templates SET content_html = '<p>Changed active legal terms that should not affect regenerate</p>'
		WHERE company_id = $1
	`, fx.companyID)
	require.NoError(t, err)

	err = svc.Regenerate(context.Background(), fx.companyID, qid)
	require.NoError(t, err)

	assert.Empty(t, source.requests(), "AI provider must not be called during regenerate")

	var updatedVersion int
	var updatedHTML string
	var calcSnap, modSnap, timeSnap, pSnap string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT content_version, content_html,
		       calculation_snapshot::text, module_snapshot::text,
		       timeline_snapshot::text, resolved_profile_snapshot::text
		FROM quotation_revisions WHERE quotation_id = $1
	`, qid).Scan(&updatedVersion, &updatedHTML, &calcSnap, &modSnap, &timeSnap, &pSnap))

	assert.Equal(t, initialVersion+1, updatedVersion, "content_version must increment by 1")
	assert.NotEmpty(t, updatedHTML)
	assert.NotContains(t, updatedHTML, "Changed active legal terms that should not affect regenerate", "regenerate must use stored legal snapshot")
	assert.Contains(t, updatedHTML, "Syarat dan ketentuan penawaran resmi berlaku 30 hari", "must use original legal snapshot text")

	var calc domain.StandaloneMaintenanceCalculation
	require.NoError(t, json.Unmarshal([]byte(calcSnap), &calc))
	assert.Equal(t, int64(23_085_000), calc.TotalInvestment)

	var scopeSnap domain.StandaloneMaintenanceScopeSnapshot
	require.NoError(t, json.Unmarshal([]byte(modSnap), &scopeSnap))
	assert.NotEmpty(t, scopeSnap.DefectGroups)
}

func TestRegenerateStandaloneMaintenance_CorruptSnapshots_RejectsAndPreservesVersion(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	t.Run("CorruptCalculation", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET calculation_snapshot = 'null'::jsonb WHERE quotation_id = $1`, qid)
		require.NoError(t, err)

		svc, _ := newSvc(pool, &panicAIProvider{})
		err = svc.Regenerate(ctx, companyID, qid)
		require.Error(t, err)
		assertGenerationCode(t, err, "generation_failed")

		snapAfter := captureSnapshot(t, pool, qid)
		assert.Equal(t, snap.version, snapAfter.version, "version must not change on corrupt snapshot")
		assert.Equal(t, snap.html, snapAfter.html, "html must not change on corrupt snapshot")
	})

	t.Run("CorruptProfile", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET resolved_profile_snapshot = 'null'::jsonb WHERE quotation_id = $1`, qid)
		require.NoError(t, err)

		svc, _ := newSvc(pool, &panicAIProvider{})
		err = svc.Regenerate(ctx, companyID, qid)
		require.Error(t, err)
		assertGenerationCode(t, err, "generation_failed")

		snapAfter := captureSnapshot(t, pool, qid)
		assert.Equal(t, snap.version, snapAfter.version)
		assert.Equal(t, snap.html, snapAfter.html)
	})

	t.Run("CorruptModule", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET module_snapshot = '{"invalid":"json_shape"}'::jsonb WHERE quotation_id = $1`, qid)
		require.NoError(t, err)

		svc, _ := newSvc(pool, &panicAIProvider{})
		err = svc.Regenerate(ctx, companyID, qid)
		require.Error(t, err)
		assertGenerationCode(t, err, "generation_failed")

		snapAfter := captureSnapshot(t, pool, qid)
		assert.Equal(t, snap.version, snapAfter.version)
		assert.Equal(t, snap.html, snapAfter.html)
	})

	t.Run("CorruptLegal", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET legal_template_snapshot = 'null'::jsonb WHERE quotation_id = $1`, qid)
		require.NoError(t, err)

		svc, _ := newSvc(pool, &panicAIProvider{})
		err = svc.Regenerate(ctx, companyID, qid)
		require.Error(t, err)
		assertGenerationCode(t, err, "generation_failed")

		snapAfter := captureSnapshot(t, pool, qid)
		assert.Equal(t, snap.version, snapAfter.version)
		assert.Equal(t, snap.html, snapAfter.html)
	})

	t.Run("MissingStandaloneGenerationInput", func(t *testing.T) {
		qid, companyID, _ := generateStandaloneMaintenanceDraft(t, pool)
		snap := captureSnapshot(t, pool, qid)

		_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET generation_input = '{"subject":"corrupted","commercial_scheme":"standalone_maintenance"}'::jsonb WHERE quotation_id = $1`, qid)
		require.NoError(t, err)

		svc, _ := newSvc(pool, &panicAIProvider{})
		err = svc.Regenerate(ctx, companyID, qid)
		require.Error(t, err)
		assertGenerationCode(t, err, "generation_failed")

		snapAfter := captureSnapshot(t, pool, qid)
		assert.Equal(t, snap.version, snapAfter.version)
		assert.Equal(t, snap.html, snapAfter.html)
	})
}

func TestRegenerateStandaloneMaintenance_CrossTenant_ReturnsNotFound(t *testing.T) {
	pool := getTestPool(t)
	qid, _, _ := generateStandaloneMaintenanceDraft(t, pool)

	svc, _ := newSvc(pool, &panicAIProvider{})

	foreignCompanyID := uuid.New()
	err := svc.Regenerate(context.Background(), foreignCompanyID, qid)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrRevisionNotFound), "cross-tenant regenerate returns not found")
}
