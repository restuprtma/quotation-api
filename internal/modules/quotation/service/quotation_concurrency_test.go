package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

// ---------------------------------------------------------------------
// Synchronized Mock Infrastructure Replicating PostgreSQL Row Locks
// ---------------------------------------------------------------------

type concurrencyMockTx struct {
	pgx.Tx
	unlockFn   func()
	unlockOnce sync.Once
}

func (m *concurrencyMockTx) Commit(ctx context.Context) error {
	m.unlockOnce.Do(func() {
		if m.unlockFn != nil {
			m.unlockFn()
		}
	})
	return nil
}

func (m *concurrencyMockTx) Rollback(ctx context.Context) error {
	m.unlockOnce.Do(func() {
		if m.unlockFn != nil {
			m.unlockFn()
		}
	})
	return nil
}

type concurrencyQuotationRepo struct {
	mu        sync.Mutex
	locks     map[string]chan struct{}
	quotation *domain.Quotation
	revisions []*domain.QuotationRevision
}

var _ service.LifecycleRepository = (*concurrencyQuotationRepo)(nil)

func newConcurrencyQuotationRepo(quot *domain.Quotation, revs []*domain.QuotationRevision) *concurrencyQuotationRepo {
	r := &concurrencyQuotationRepo{
		locks:     make(map[string]chan struct{}),
		quotation: quot,
		revisions: make([]*domain.QuotationRevision, 0, len(revs)),
	}
	for _, rev := range revs {
		r.revisions = append(r.revisions, copyQuotationRev(rev))
	}
	return r
}

func (r *concurrencyQuotationRepo) getLockCh(id string) chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch, ok := r.locks[id]
	if !ok {
		ch = make(chan struct{}, 1)
		r.locks[id] = ch
	}
	return ch
}

func (r *concurrencyQuotationRepo) Begin(ctx context.Context) (pgx.Tx, error) {
	return &concurrencyMockTx{}, nil
}

func (r *concurrencyQuotationRepo) LockQuotationMaster(ctx context.Context, tx pgx.Tx, quotationID string) error {
	ch := r.getLockCh(quotationID)
	select {
	case ch <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	if stx, ok := tx.(*concurrencyMockTx); ok {
		stx.unlockFn = func() {
			select {
			case <-ch:
			default:
			}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.quotation == nil || r.quotation.ID.String() != quotationID {
		return repository.ErrNotFound
	}
	return nil
}

func (r *concurrencyQuotationRepo) GetQuotationByID(ctx context.Context, id, companyID string) (*domain.Quotation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.quotation == nil || r.quotation.ID.String() != id || r.quotation.CompanyID.String() != companyID {
		return nil, repository.ErrNotFound
	}
	cp := *r.quotation
	if r.quotation.ActivePublishedRevisionID != nil {
		idVal := *r.quotation.ActivePublishedRevisionID
		cp.ActivePublishedRevisionID = &idVal
	}
	return &cp, nil
}

func (r *concurrencyQuotationRepo) GetDraftRevision(ctx context.Context, tx pgx.Tx, quotationID, companyID string) (*domain.QuotationRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rev := range r.revisions {
		if rev.QuotationID.String() == quotationID && rev.CompanyID.String() == companyID && rev.PublicationStatus == repository.PublicationStatusDraft {
			return copyQuotationRev(rev), nil
		}
	}
	return nil, repository.ErrRevisionNotFound
}

func (r *concurrencyQuotationRepo) MarkPublishedRevisionSuperseded(ctx context.Context, tx pgx.Tx, quotationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rev := range r.revisions {
		if rev.QuotationID.String() == quotationID && rev.PublicationStatus == repository.PublicationStatusPublished {
			rev.PublicationStatus = repository.PublicationStatusSuperseded
		}
	}
	return nil
}

func (r *concurrencyQuotationRepo) PublishDraftQuotation(ctx context.Context, tx pgx.Tx, quotationID, draftID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var draftRev *domain.QuotationRevision
	for _, rev := range r.revisions {
		if rev.ID.String() == draftID && rev.PublicationStatus == repository.PublicationStatusDraft {
			draftRev = rev
			break
		}
	}
	if draftRev == nil {
		return repository.ErrRevisionNotFound
	}
	draftRev.PublicationStatus = repository.PublicationStatusPublished
	now := time.Now().UTC()
	draftRev.PublishedAt = &now

	if r.quotation != nil && r.quotation.ID.String() == quotationID {
		r.quotation.Status = repository.PublicationStatusPublished
		pID := draftRev.ID
		r.quotation.ActivePublishedRevisionID = &pID
	}
	return nil
}

func (r *concurrencyQuotationRepo) GetActivePublishedRevision(ctx context.Context, tx pgx.Tx, quotationID, companyID string) (*domain.QuotationRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rev := range r.revisions {
		if rev.QuotationID.String() == quotationID && rev.CompanyID.String() == companyID && rev.PublicationStatus == repository.PublicationStatusPublished {
			return copyQuotationRev(rev), nil
		}
	}
	return nil, repository.ErrRevisionNotFound
}

func (r *concurrencyQuotationRepo) GetMaxRevisionNumber(ctx context.Context, tx pgx.Tx, quotationID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	maxRev := 0
	for _, rev := range r.revisions {
		if rev.QuotationID.String() == quotationID && rev.RevisionNumber > maxRev {
			maxRev = rev.RevisionNumber
		}
	}
	return maxRev, nil
}

func (r *concurrencyQuotationRepo) CreateRevisionTx(ctx context.Context, tx pgx.Tx, rev *domain.QuotationRevision) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.revisions = append(r.revisions, copyQuotationRev(rev))
	return nil
}

func (r *concurrencyQuotationRepo) UpdateQuotationCurrentRevision(ctx context.Context, tx pgx.Tx, quotationID string, currentRevision int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.quotation == nil || r.quotation.ID.String() != quotationID {
		return repository.ErrNotFound
	}
	r.quotation.CurrentRevision = currentRevision
	return nil
}

func copyQuotationRev(rev *domain.QuotationRevision) *domain.QuotationRevision {
	if rev == nil {
		return nil
	}
	cp := *rev
	if rev.ContentHTML != nil {
		val := *rev.ContentHTML
		cp.ContentHTML = &val
	}
	if rev.ContentVersion != nil {
		val := *rev.ContentVersion
		cp.ContentVersion = &val
	}
	if rev.ContentHash != nil {
		val := *rev.ContentHash
		cp.ContentHash = &val
	}
	if rev.AiProvider != nil {
		val := *rev.AiProvider
		cp.AiProvider = &val
	}
	if rev.AiModel != nil {
		val := *rev.AiModel
		cp.AiModel = &val
	}
	if rev.SafetyEnvelopeVersion != nil {
		val := *rev.SafetyEnvelopeVersion
		cp.SafetyEnvelopeVersion = &val
	}
	if rev.BudgetMaximum != nil {
		val := *rev.BudgetMaximum
		cp.BudgetMaximum = &val
	}
	return &cp
}

type concurrencyBlueprintRepo struct {
	mu        sync.Mutex
	locks     map[string]chan struct{}
	blueprint *domain.Blueprint
	revisions []*domain.BlueprintRevision
}

var _ repository.BlueprintLifecycleRepository = (*concurrencyBlueprintRepo)(nil)

func newConcurrencyBlueprintRepo(bp *domain.Blueprint, revs []*domain.BlueprintRevision) *concurrencyBlueprintRepo {
	r := &concurrencyBlueprintRepo{
		locks:     make(map[string]chan struct{}),
		blueprint: bp,
		revisions: make([]*domain.BlueprintRevision, 0, len(revs)),
	}
	for _, rev := range revs {
		r.revisions = append(r.revisions, copyBlueprintRev(rev))
	}
	return r
}

func (r *concurrencyBlueprintRepo) getLockCh(id string) chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch, ok := r.locks[id]
	if !ok {
		ch = make(chan struct{}, 1)
		r.locks[id] = ch
	}
	return ch
}

func (r *concurrencyBlueprintRepo) Begin(ctx context.Context) (pgx.Tx, error) {
	return &concurrencyMockTx{}, nil
}

func (r *concurrencyBlueprintRepo) LockBlueprintMaster(ctx context.Context, tx pgx.Tx, blueprintID string) error {
	ch := r.getLockCh(blueprintID)
	select {
	case ch <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	if stx, ok := tx.(*concurrencyMockTx); ok {
		stx.unlockFn = func() {
			select {
			case <-ch:
			default:
			}
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.blueprint == nil || r.blueprint.ID.String() != blueprintID {
		return repository.ErrBlueprintMasterNotFound
	}
	return nil
}

func (r *concurrencyBlueprintRepo) DeleteDraft(ctx context.Context, companyID, blueprintID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	filtered := make([]*domain.BlueprintRevision, 0, len(r.revisions))
	deleted := false
	for _, rev := range r.revisions {
		if rev.BlueprintID.String() == blueprintID && rev.CompanyID.String() == companyID && rev.PublicationStatus == "draft" {
			deleted = true
			continue
		}
		filtered = append(filtered, rev)
	}
	r.revisions = filtered
	if !deleted {
		return repository.ErrBlueprintDraftNotFound
	}
	return nil
}

func (r *concurrencyBlueprintRepo) PublishDraftBlueprint(ctx context.Context, tx pgx.Tx, blueprintID, draftID string, isComplete bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var draftRev *domain.BlueprintRevision
	for _, rev := range r.revisions {
		if rev.ID.String() == draftID && rev.PublicationStatus == "draft" {
			draftRev = rev
			break
		}
	}
	if draftRev == nil {
		return repository.ErrBlueprintRevisionNotFound
	}
	draftRev.PublicationStatus = "published"
	if isComplete {
		draftRev.CompletenessStatus = "complete"
	} else {
		draftRev.CompletenessStatus = "partial"
	}
	now := time.Now().UTC()
	draftRev.PublishedAt = &now

	if r.blueprint != nil && r.blueprint.ID.String() == blueprintID {
		r.blueprint.Status = "published"
		pID := draftRev.ID
		r.blueprint.ActivePublishedRevisionID = &pID
	}
	return nil
}

func (r *concurrencyBlueprintRepo) MarkPublishedRevisionSuperseded(ctx context.Context, tx pgx.Tx, blueprintID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rev := range r.revisions {
		if rev.BlueprintID.String() == blueprintID && rev.PublicationStatus == "published" {
			rev.PublicationStatus = "superseded"
		}
	}
	return nil
}

func (r *concurrencyBlueprintRepo) GetMaxRevisionNumber(ctx context.Context, tx pgx.Tx, blueprintID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	maxRev := 0
	for _, rev := range r.revisions {
		if rev.BlueprintID.String() == blueprintID && rev.RevisionNumber > maxRev {
			maxRev = rev.RevisionNumber
		}
	}
	return maxRev, nil
}

func (r *concurrencyBlueprintRepo) GetDraftRevision(ctx context.Context, tx pgx.Tx, blueprintID, companyID string) (*domain.BlueprintRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rev := range r.revisions {
		if rev.BlueprintID.String() == blueprintID && rev.CompanyID.String() == companyID && rev.PublicationStatus == "draft" {
			return copyBlueprintRev(rev), nil
		}
	}
	return nil, repository.ErrBlueprintDraftNotFound
}

func (r *concurrencyBlueprintRepo) GetActivePublishedRevision(ctx context.Context, tx pgx.Tx, blueprintID, companyID string) (*domain.BlueprintRevision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rev := range r.revisions {
		if rev.BlueprintID.String() == blueprintID && rev.CompanyID.String() == companyID && rev.PublicationStatus == "published" {
			return copyBlueprintRev(rev), nil
		}
	}
	return nil, repository.ErrBlueprintRevisionNotFound
}

func (r *concurrencyBlueprintRepo) CreateRevisionTx(ctx context.Context, tx pgx.Tx, rev *domain.BlueprintRevision) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.revisions = append(r.revisions, copyBlueprintRev(rev))
	return nil
}

func (r *concurrencyBlueprintRepo) UpdateBlueprintCurrentRevision(ctx context.Context, tx pgx.Tx, blueprintID string, currentRevision int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.blueprint == nil || r.blueprint.ID.String() != blueprintID {
		return repository.ErrBlueprintMasterNotFound
	}
	r.blueprint.CurrentRevision = currentRevision
	return nil
}

func copyBlueprintRev(rev *domain.BlueprintRevision) *domain.BlueprintRevision {
	if rev == nil {
		return nil
	}
	cp := *rev
	if rev.SafetyEnvelopeVersion != nil {
		val := *rev.SafetyEnvelopeVersion
		cp.SafetyEnvelopeVersion = &val
	}
	if rev.AiProvider != nil {
		val := *rev.AiProvider
		cp.AiProvider = &val
	}
	if rev.AiModel != nil {
		val := *rev.AiModel
		cp.AiModel = &val
	}
	return &cp
}

func concurrencyCompleteBlueprintJSON() json.RawMessage {
	return json.RawMessage(`{
		"schema_version": "1.0",
		"title": "Complete System Blueprint",
		"summary": "Full blueprint summary",
		"journey": {
			"phases": [{"id": "p1", "label": "Onboarding", "order": 1}],
			"actors": [{"id": "a1", "label": "Customer", "order": 1}],
			"activities": [{"id": "act1", "phase_id": "p1", "actor_id": "a1", "label": "Register", "touchpoints": ["web"]}]
		},
		"architecture": {
			"groups": [{"id": "g1", "label": "App Tier", "order": 1}],
			"nodes": [{"id": "n1", "group_id": "g1", "type": "service", "label": "API", "layout": {"x": 10, "y": 10}}],
			"edges": []
		},
		"state_machine": {
			"entity": "Account",
			"initial_state": "s1",
			"states": [{"id": "s1", "label": "Active", "type": "normal", "layout": {"x": 10, "y": 10}}],
			"transitions": []
		},
		"workflows": {
			"steps": [{"id": "step1", "title": "Submit Form", "role": "User"}]
		},
		"wbs": {
			"items": [{"id": "w1", "name": "Sprint 1", "category": "development", "estimate_hours": 40}]
		}
	}`)
}

func concurrencyGetTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("skipping postgresql concurrency test: TEST_DATABASE_URL not set")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping postgresql concurrency test: unable to connect: %v", err)
		return nil
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping postgresql concurrency test: database ping failed: %v", err)
		return nil
	}

	t.Cleanup(pool.Close)
	return pool
}

// ---------------------------------------------------------------------
// Concurrency Tests: Quotation Publish & Create Revision
// ---------------------------------------------------------------------

func TestConcurrentQuotationPublish(t *testing.T) {
	t.Run("SynchronizedMock", func(t *testing.T) {
		ctx := context.Background()
		companyID := uuid.New()
		quotationID := uuid.New()
		draftRevID := uuid.New()

		contentHTML := "<p>Final Quotation Revision 1</p>"
		contentVer := "1"
		draftRev := &domain.QuotationRevision{
			ID:                    draftRevID,
			CompanyID:             companyID,
			QuotationID:           quotationID,
			RevisionNumber:        1,
			PublicationStatus:     repository.PublicationStatusDraft,
			GenerationStatus:      "completed",
			ContentVersion:        &contentVer,
			ContentHTML:           &contentHTML,
			LegalContentModified:  false,
			CommercialContentModified: false,
		}

		quotMaster := &domain.Quotation{
			ID:              quotationID,
			CompanyID:       companyID,
			Status:          "draft",
			CurrentRevision: 1,
		}

		repo := newConcurrencyQuotationRepo(quotMaster, []*domain.QuotationRevision{draftRev})
		svc := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).WithLifecycleRepository(repo)

		const concurrency = 15
		startCh := make(chan struct{})
		var wg sync.WaitGroup
		var mu sync.Mutex
		var successCount int
		var failCount int
		var errorsList []error

		req := dto.PublishQuotationRequest{
			RevisionID:     draftRevID.String(),
			ContentVersion: 1,
		}

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startCh

				err := svc.PublishQuotation(ctx, companyID.String(), quotationID.String(), req)
				mu.Lock()
				defer mu.Unlock()
				if err == nil {
					successCount++
				} else {
					failCount++
					errorsList = append(errorsList, err)
				}
			}()
		}

		close(startCh)
		wg.Wait()

		// Row-lock concurrency invariants:
		// Exactly 1 publish call succeeds; the other 14 are rejected because the draft is already published.
		assert.Equal(t, 1, successCount, "exactly 1 goroutine must succeed in publishing the draft")
		assert.Equal(t, concurrency-1, failCount, "all other concurrent calls must fail")
		assert.Len(t, errorsList, concurrency-1)

		for _, err := range errorsList {
			assert.True(t, errors.Is(err, service.ErrNotFound) || errors.Is(err, repository.ErrRevisionNotFound),
				"expected conflict/not found error, got: %v", err)
		}

		// Verify quotation master row status & pointer invariant
		quot, err := repo.GetQuotationByID(ctx, quotationID.String(), companyID.String())
		require.NoError(t, err)
		assert.Equal(t, repository.PublicationStatusPublished, quot.Status)
		require.NotNil(t, quot.ActivePublishedRevisionID)
		assert.Equal(t, draftRevID, *quot.ActivePublishedRevisionID)

		// Verify draft revision publication status transitioned to published
		pubRev, err := repo.GetActivePublishedRevision(ctx, nil, quotationID.String(), companyID.String())
		require.NoError(t, err)
		assert.Equal(t, draftRevID, pubRev.ID)
		assert.Equal(t, repository.PublicationStatusPublished, pubRev.PublicationStatus)
		assert.NotNil(t, pubRev.PublishedAt)
	})

	t.Run("PostgreSQL", func(t *testing.T) {
		pool := concurrencyGetTestPool(t)
		if pool == nil {
			return
		}

		ctx := context.Background()
		companyID := uuid.New()
		clientID := uuid.New()
		quotationID := uuid.New()
		draftRevID := uuid.New()
		quotNumber := "QUO-" + uuid.New().String()[:8]

		_, err := pool.Exec(ctx, `
			INSERT INTO quotation_client_organizations (id, company_id, name, normalized_name)
			VALUES ($1, $2, 'Concurrent Client', 'concurrent client')
		`, clientID, companyID)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			INSERT INTO quotations (id, company_id, client_id, quotation_number, solution_type, commercial_scheme, status, current_revision)
			VALUES ($1, $2, $3, $4, 'custom_software', 'dedicated_squad', 'draft', 1)
		`, quotationID, companyID, clientID, quotNumber)
		require.NoError(t, err)

		contentHTML := "<p>Real DB Concurrent Quotation Draft</p>"
		contentVer := "1"
		_, err = pool.Exec(ctx, `
			INSERT INTO quotation_revisions (
				id, company_id, quotation_id, revision_number, publication_status, generation_status,
				content_version, content_html, generation_input, budget_maximum
			) VALUES ($1, $2, $3, 1, 'draft', 'completed', $4, $5, '{}'::jsonb, 150000)
		`, draftRevID, companyID, quotationID, contentVer, contentHTML)
		require.NoError(t, err)

		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM quotation_revisions WHERE quotation_id = $1`, quotationID)
			_, _ = pool.Exec(ctx, `DELETE FROM quotations WHERE id = $1`, quotationID)
			_, _ = pool.Exec(ctx, `DELETE FROM quotation_client_organizations WHERE id = $1`, clientID)
		})

		repo := repository.NewRepository(pool)
		svc := service.NewService(repo, nil, nil, nil, nil, nil, nil, nil)

		const concurrency = 15
		startCh := make(chan struct{})
		var wg sync.WaitGroup
		var mu sync.Mutex
		var successCount int
		var failCount int

		req := dto.PublishQuotationRequest{
			RevisionID:     draftRevID.String(),
			ContentVersion: 1,
		}

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startCh

				err := svc.PublishQuotation(ctx, companyID.String(), quotationID.String(), req)
				mu.Lock()
				defer mu.Unlock()
				if err == nil {
					successCount++
				} else {
					failCount++
				}
			}()
		}

		close(startCh)
		wg.Wait()

		assert.Equal(t, 1, successCount, "exactly 1 goroutine must succeed in real DB publish")
		assert.Equal(t, concurrency-1, failCount)

		var status string
		var activePubID *uuid.UUID
		err = pool.QueryRow(ctx, `SELECT status, active_published_revision_id FROM quotations WHERE id = $1`, quotationID).
			Scan(&status, &activePubID)
		require.NoError(t, err)
		assert.Equal(t, "published", status)
		require.NotNil(t, activePubID)
		assert.Equal(t, draftRevID, *activePubID)
	})
}

func TestConcurrentQuotationCreateRevision(t *testing.T) {
	t.Run("SynchronizedMock", func(t *testing.T) {
		ctx := context.Background()
		companyID := uuid.New()
		quotationID := uuid.New()
		publishedRevID := uuid.New()

		contentHTML := "<h1>Active Published Quotation Content</h1>"
		contentVer := "1"
		budget := int64(200_000_000)

		pubRev := &domain.QuotationRevision{
			ID:                    publishedRevID,
			CompanyID:             companyID,
			QuotationID:           quotationID,
			RevisionNumber:        1,
			PublicationStatus:     repository.PublicationStatusPublished,
			GenerationStatus:      "completed",
			GenerationInput:       json.RawMessage(`{"scope":"full"}`),
			BudgetMaximum:         &budget,
			AllocationSnapshot:    json.RawMessage(`{"roles":[]}`),
			RateCardSnapshot:      json.RawMessage(`{"currency":"IDR"}`),
			LegalTemplateSnapshot: json.RawMessage(`{"clauses":[]}`),
			ContentHTML:           &contentHTML,
			ContentVersion:        &contentVer,
			PromptState:           json.RawMessage(`{}`),
			PromptSectionHashes:   json.RawMessage(`{}`),
		}

		quotMaster := &domain.Quotation{
			ID:                        quotationID,
			CompanyID:                 companyID,
			Status:                    "published",
			CurrentRevision:           1,
			ActivePublishedRevisionID: &publishedRevID,
		}

		repo := newConcurrencyQuotationRepo(quotMaster, []*domain.QuotationRevision{pubRev})
		svc := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).WithLifecycleRepository(repo)

		const concurrency = 15
		startCh := make(chan struct{})
		var wg sync.WaitGroup
		var mu sync.Mutex
		var successCount int
		var failCount int
		var errorsList []error

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startCh

				_, err := svc.CreateQuotationRevision(ctx, companyID.String(), quotationID.String())
				mu.Lock()
				defer mu.Unlock()
				if err == nil {
					successCount++
				} else {
					failCount++
					errorsList = append(errorsList, err)
				}
			}()
		}

		close(startCh)
		wg.Wait()

		// Single active draft invariant:
		// Exactly 1 succeeds in creating revision 2; all 14 remaining calls fail with ErrActiveDraftAlreadyExists.
		assert.Equal(t, 1, successCount, "exactly 1 goroutine must succeed in creating the draft revision")
		assert.Equal(t, concurrency-1, failCount, "all other concurrent calls must fail")
		assert.Len(t, errorsList, concurrency-1)

		for _, err := range errorsList {
			assert.ErrorIs(t, err, service.ErrActiveDraftAlreadyExists, "expected ErrActiveDraftAlreadyExists")
		}

		// Master current revision must be incremented to 2
		quot, err := repo.GetQuotationByID(ctx, quotationID.String(), companyID.String())
		require.NoError(t, err)
		assert.Equal(t, 2, quot.CurrentRevision)

		// Only 1 draft revision must exist with revision number 2
		draftRev, err := repo.GetDraftRevision(ctx, nil, quotationID.String(), companyID.String())
		require.NoError(t, err)
		require.NotNil(t, draftRev)
		assert.Equal(t, 2, draftRev.RevisionNumber)
		assert.Equal(t, repository.PublicationStatusDraft, draftRev.PublicationStatus)

		// Max revision number must be 2
		maxRev, err := repo.GetMaxRevisionNumber(ctx, nil, quotationID.String())
		require.NoError(t, err)
		assert.Equal(t, 2, maxRev)
	})

	t.Run("PostgreSQL", func(t *testing.T) {
		pool := concurrencyGetTestPool(t)
		if pool == nil {
			return
		}

		ctx := context.Background()
		companyID := uuid.New()
		clientID := uuid.New()
		quotationID := uuid.New()
		publishedRevID := uuid.New()
		quotNumber := "QUO-" + uuid.New().String()[:8]

		_, err := pool.Exec(ctx, `
			INSERT INTO quotation_client_organizations (id, company_id, name, normalized_name)
			VALUES ($1, $2, 'Concurrent Client 2', 'concurrent client 2')
		`, clientID, companyID)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			INSERT INTO quotations (id, company_id, client_id, quotation_number, solution_type, commercial_scheme, status, current_revision, active_published_revision_id)
			VALUES ($1, $2, $3, $4, 'custom_software', 'dedicated_squad', 'published', 1, $5)
		`, quotationID, companyID, clientID, quotNumber, publishedRevID)
		require.NoError(t, err)

		contentHTML := "<p>Real DB Published Revision 1</p>"
		contentVer := "1"
		_, err = pool.Exec(ctx, `
			INSERT INTO quotation_revisions (
				id, company_id, quotation_id, revision_number, publication_status, generation_status,
				content_version, content_html, generation_input, budget_maximum
			) VALUES ($1, $2, $3, 1, 'published', 'completed', $4, $5, '{}'::jsonb, 100000)
		`, publishedRevID, companyID, quotationID, contentVer, contentHTML)
		require.NoError(t, err)

		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM quotation_revisions WHERE quotation_id = $1`, quotationID)
			_, _ = pool.Exec(ctx, `DELETE FROM quotations WHERE id = $1`, quotationID)
			_, _ = pool.Exec(ctx, `DELETE FROM quotation_client_organizations WHERE id = $1`, clientID)
		})

		repo := repository.NewRepository(pool)
		svc := service.NewService(repo, nil, nil, nil, nil, nil, nil, nil)

		const concurrency = 15
		startCh := make(chan struct{})
		var wg sync.WaitGroup
		var mu sync.Mutex
		var successCount int
		var failCount int

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startCh

				_, err := svc.CreateQuotationRevision(ctx, companyID.String(), quotationID.String())
				mu.Lock()
				defer mu.Unlock()
				if err == nil {
					successCount++
				} else {
					failCount++
				}
			}()
		}

		close(startCh)
		wg.Wait()

		assert.Equal(t, 1, successCount)
		assert.Equal(t, concurrency-1, failCount)

		var currentRev int
		err = pool.QueryRow(ctx, `SELECT current_revision FROM quotations WHERE id = $1`, quotationID).Scan(&currentRev)
		require.NoError(t, err)
		assert.Equal(t, 2, currentRev)
	})
}

// ---------------------------------------------------------------------
// Concurrency Tests: Blueprint Publish & Create Revision
// ---------------------------------------------------------------------

func TestConcurrentBlueprintPublish(t *testing.T) {
	t.Run("SynchronizedMock", func(t *testing.T) {
		ctx := context.Background()
		companyID := uuid.New()
		blueprintID := uuid.New()
		draftRevID := uuid.New()

		draftRev := &domain.BlueprintRevision{
			ID:                    draftRevID,
			CompanyID:             companyID,
			BlueprintID:           blueprintID,
			RevisionNumber:        1,
			PublicationStatus:     "draft",
			CompletenessStatus:    "complete",
			GenerationStatus:      "completed",
			ContentJSON:           concurrencyCompleteBlueprintJSON(),
			CrossValidationResult: json.RawMessage(`{"valid":true,"issues":[]}`),
			ValidationResult:     json.RawMessage(`{"journey":{"valid":true,"issues":[]}}`),
		}

		bpMaster := &domain.Blueprint{
			ID:              blueprintID,
			CompanyID:       companyID,
			Status:          "draft",
			CurrentRevision: 1,
		}

		repo := newConcurrencyBlueprintRepo(bpMaster, []*domain.BlueprintRevision{draftRev})
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		const concurrency = 15
		startCh := make(chan struct{})
		var wg sync.WaitGroup
		var mu sync.Mutex
		var successCount int
		var failCount int
		var errorsList []error

		req := dto.PublishBlueprintRequest{
			RevisionID: draftRevID.String(),
		}

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startCh

				err := svc.PublishBlueprint(ctx, companyID.String(), blueprintID.String(), req)
				mu.Lock()
				defer mu.Unlock()
				if err == nil {
					successCount++
				} else {
					failCount++
					errorsList = append(errorsList, err)
				}
			}()
		}

		close(startCh)
		wg.Wait()

		// Row-lock concurrency invariant:
		// Exactly 1 publish succeeds; the other 14 fail because draft is already published.
		assert.Equal(t, 1, successCount, "exactly 1 goroutine must succeed in publishing the blueprint")
		assert.Equal(t, concurrency-1, failCount, "all other concurrent calls must fail")
		assert.Len(t, errorsList, concurrency-1)

		for _, err := range errorsList {
			assert.ErrorIs(t, err, service.ErrBlueprintNotFound, "expected ErrBlueprintNotFound for published blueprint")
		}

		// Master status and active pointer check
		assert.Equal(t, "published", repo.blueprint.Status)
		require.NotNil(t, repo.blueprint.ActivePublishedRevisionID)
		assert.Equal(t, draftRevID, *repo.blueprint.ActivePublishedRevisionID)

		// Revision status check
		pubRev, err := repo.GetActivePublishedRevision(ctx, nil, blueprintID.String(), companyID.String())
		require.NoError(t, err)
		assert.Equal(t, draftRevID, pubRev.ID)
		assert.Equal(t, "published", pubRev.PublicationStatus)
		assert.Equal(t, "complete", pubRev.CompletenessStatus)
		assert.NotNil(t, pubRev.PublishedAt)
	})

	t.Run("PostgreSQL", func(t *testing.T) {
		pool := concurrencyGetTestPool(t)
		if pool == nil {
			return
		}

		ctx := context.Background()
		companyID := uuid.New()
		clientID := uuid.New()
		quotationID := uuid.New()
		blueprintID := uuid.New()
		draftRevID := uuid.New()
		quotNumber := "QUO-" + uuid.New().String()[:8]

		_, err := pool.Exec(ctx, `
			INSERT INTO quotation_client_organizations (id, company_id, name, normalized_name)
			VALUES ($1, $2, 'BP Client', 'bp client')
		`, clientID, companyID)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			INSERT INTO quotations (id, company_id, client_id, quotation_number, solution_type, commercial_scheme, status, current_revision)
			VALUES ($1, $2, $3, $4, 'custom_software', 'dedicated_squad', 'published', 1)
		`, quotationID, companyID, clientID, quotNumber)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			INSERT INTO blueprints (id, company_id, quotation_id, title, status, current_revision)
			VALUES ($1, $2, $3, 'Concurrent Blueprint', 'draft', 1)
		`, blueprintID, companyID, quotationID)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			INSERT INTO blueprint_revisions (
				id, company_id, blueprint_id, revision_number, publication_status, completeness_status,
				generation_status, content_json, cross_validation_result, validation_result
			) VALUES ($1, $2, $3, 1, 'draft', 'complete', 'completed', $4, '{}'::jsonb, '{}'::jsonb)
		`, draftRevID, companyID, blueprintID, string(concurrencyCompleteBlueprintJSON()))
		require.NoError(t, err)

		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM blueprint_revisions WHERE blueprint_id = $1`, blueprintID)
			_, _ = pool.Exec(ctx, `DELETE FROM blueprints WHERE id = $1`, blueprintID)
			_, _ = pool.Exec(ctx, `DELETE FROM quotations WHERE id = $1`, quotationID)
			_, _ = pool.Exec(ctx, `DELETE FROM quotation_client_organizations WHERE id = $1`, clientID)
		})

		bpRepo := repository.NewBlueprintLifecycleRepository(pool)
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(bpRepo)

		const concurrency = 15
		startCh := make(chan struct{})
		var wg sync.WaitGroup
		var mu sync.Mutex
		var successCount int
		var failCount int

		req := dto.PublishBlueprintRequest{
			RevisionID: draftRevID.String(),
		}

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startCh

				err := svc.PublishBlueprint(ctx, companyID.String(), blueprintID.String(), req)
				mu.Lock()
				defer mu.Unlock()
				if err == nil {
					successCount++
				} else {
					failCount++
				}
			}()
		}

		close(startCh)
		wg.Wait()

		assert.Equal(t, 1, successCount)
		assert.Equal(t, concurrency-1, failCount)

		var status string
		var activePubID *uuid.UUID
		err = pool.QueryRow(ctx, `SELECT status, active_published_revision_id FROM blueprints WHERE id = $1`, blueprintID).
			Scan(&status, &activePubID)
		require.NoError(t, err)
		assert.Equal(t, "published", status)
		require.NotNil(t, activePubID)
		assert.Equal(t, draftRevID, *activePubID)
	})
}

func TestConcurrentBlueprintCreateRevision(t *testing.T) {
	t.Run("SynchronizedMock", func(t *testing.T) {
		ctx := context.Background()
		companyID := uuid.New()
		blueprintID := uuid.New()
		publishedRevID := uuid.New()

		pubRev := &domain.BlueprintRevision{
			ID:                    publishedRevID,
			CompanyID:             companyID,
			BlueprintID:           blueprintID,
			RevisionNumber:        1,
			PublicationStatus:     "published",
			CompletenessStatus:    "complete",
			GenerationStatus:      "completed",
			ContentJSON:           concurrencyCompleteBlueprintJSON(),
			CrossValidationResult: json.RawMessage(`{"valid":true,"issues":[]}`),
			ValidationResult:     json.RawMessage(`{"journey":{"valid":true,"issues":[]}}`),
		}

		bpMaster := &domain.Blueprint{
			ID:                        blueprintID,
			CompanyID:                 companyID,
			Status:                    "published",
			CurrentRevision:           1,
			ActivePublishedRevisionID: &publishedRevID,
		}

		repo := newConcurrencyBlueprintRepo(bpMaster, []*domain.BlueprintRevision{pubRev})
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		const concurrency = 15
		startCh := make(chan struct{})
		var wg sync.WaitGroup
		var mu sync.Mutex
		var successCount int
		var failCount int
		var errorsList []error

		req := dto.CreateBlueprintRevisionRequest{
			Mode: "clone",
		}

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startCh

				_, err := svc.CreateBlueprintRevision(ctx, companyID.String(), blueprintID.String(), req)
				mu.Lock()
				defer mu.Unlock()
				if err == nil {
					successCount++
				} else {
					failCount++
					errorsList = append(errorsList, err)
				}
			}()
		}

		close(startCh)
		wg.Wait()

		// Single active draft invariant:
		// Exactly 1 succeeds in creating revision 2; all 14 remaining calls fail with ErrBlueprintActiveDraftAlreadyExists.
		assert.Equal(t, 1, successCount, "exactly 1 goroutine must succeed in creating blueprint revision")
		assert.Equal(t, concurrency-1, failCount, "all other concurrent calls must fail")
		assert.Len(t, errorsList, concurrency-1)

		for _, err := range errorsList {
			assert.ErrorIs(t, err, service.ErrBlueprintActiveDraftAlreadyExists, "expected ErrBlueprintActiveDraftAlreadyExists")
		}

		// Master current revision must be incremented to 2
		assert.Equal(t, 2, repo.blueprint.CurrentRevision)

		// Only 1 draft revision must exist with revision number 2
		draftRev, err := repo.GetDraftRevision(ctx, nil, blueprintID.String(), companyID.String())
		require.NoError(t, err)
		require.NotNil(t, draftRev)
		assert.Equal(t, 2, draftRev.RevisionNumber)
		assert.Equal(t, "draft", draftRev.PublicationStatus)

		// Max revision number must be 2
		maxRev, err := repo.GetMaxRevisionNumber(ctx, nil, blueprintID.String())
		require.NoError(t, err)
		assert.Equal(t, 2, maxRev)
	})

	t.Run("PostgreSQL", func(t *testing.T) {
		pool := concurrencyGetTestPool(t)
		if pool == nil {
			return
		}

		ctx := context.Background()
		companyID := uuid.New()
		clientID := uuid.New()
		quotationID := uuid.New()
		blueprintID := uuid.New()
		publishedRevID := uuid.New()
		quotNumber := "QUO-" + uuid.New().String()[:8]

		_, err := pool.Exec(ctx, `
			INSERT INTO quotation_client_organizations (id, company_id, name, normalized_name)
			VALUES ($1, $2, 'BP Client 2', 'bp client 2')
		`, clientID, companyID)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			INSERT INTO quotations (id, company_id, client_id, quotation_number, solution_type, commercial_scheme, status, current_revision)
			VALUES ($1, $2, $3, $4, 'custom_software', 'dedicated_squad', 'published', 1)
		`, quotationID, companyID, clientID, quotNumber)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			INSERT INTO blueprints (id, company_id, quotation_id, title, status, current_revision, active_published_revision_id)
			VALUES ($1, $2, $3, 'Concurrent Blueprint 2', 'published', 1, $4)
		`, blueprintID, companyID, quotationID, publishedRevID)
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			INSERT INTO blueprint_revisions (
				id, company_id, blueprint_id, revision_number, publication_status, completeness_status,
				generation_status, content_json
			) VALUES ($1, $2, $3, 1, 'published', 'complete', 'completed', $4)
		`, publishedRevID, companyID, blueprintID, string(concurrencyCompleteBlueprintJSON()))
		require.NoError(t, err)

		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM blueprint_revisions WHERE blueprint_id = $1`, blueprintID)
			_, _ = pool.Exec(ctx, `DELETE FROM blueprints WHERE id = $1`, blueprintID)
			_, _ = pool.Exec(ctx, `DELETE FROM quotations WHERE id = $1`, quotationID)
			_, _ = pool.Exec(ctx, `DELETE FROM quotation_client_organizations WHERE id = $1`, clientID)
		})

		bpRepo := repository.NewBlueprintLifecycleRepository(pool)
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(bpRepo)

		const concurrency = 15
		startCh := make(chan struct{})
		var wg sync.WaitGroup
		var mu sync.Mutex
		var successCount int
		var failCount int

		req := dto.CreateBlueprintRevisionRequest{
			Mode: "clone",
		}

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-startCh

				_, err := svc.CreateBlueprintRevision(ctx, companyID.String(), blueprintID.String(), req)
				mu.Lock()
				defer mu.Unlock()
				if err == nil {
					successCount++
				} else {
					failCount++
				}
			}()
		}

		close(startCh)
		wg.Wait()

		assert.Equal(t, 1, successCount)
		assert.Equal(t, concurrency-1, failCount)

		var currentRev int
		err = pool.QueryRow(ctx, `SELECT current_revision FROM blueprints WHERE id = $1`, blueprintID).Scan(&currentRev)
		require.NoError(t, err)
		assert.Equal(t, 2, currentRev)
	})
}

func TestConcurrentPublish(t *testing.T) {
	t.Run("Quotation", TestConcurrentQuotationPublish)
	t.Run("Blueprint", TestConcurrentBlueprintPublish)
}
