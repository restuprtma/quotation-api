//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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

const pipelineLegalCode = "quotation_standard"

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
	fixBudget      = int64(150_000_000)
	fixTinyBudget  = int64(50_000_000)
	fixValidTotal  = int64(63_000_000) // 12jt*1*3 + 9jt*1*3
	fixOverTotal   = int64(171_000_000) // 12jt*4*3 + 9jt*1*3
	fixOverExcess  = int64(21_000_000)
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
		ClientID:                  fx.clientID,
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
			{"role":"programmer","seniority":"senior","employee_count":1,"period_months":3,
			 "scope_references":["Customer portal"],"reason":"Mengimplementasikan fitur portal"},
			{"role":"business_analyst","seniority":"intermediate","employee_count":1,"period_months":3,
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
			{"role":"programmer","seniority":"senior","employee_count":4,"period_months":3,
			 "scope_references":["Customer portal"],"reason":"Percepatan implementasi portal"},
			{"role":"business_analyst","seniority":"intermediate","employee_count":1,"period_months":3,
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

	qid, err := svc.Generate(context.Background(), fx.companyID, nil, fx.request())
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
	assert.Len(t, ge.Error(), len("budget_unfeasible")) // redaksi: pesan == kode saja

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
