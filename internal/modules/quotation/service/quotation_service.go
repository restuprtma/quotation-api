package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/pkg/logger"
)

// Service errors surfaced to the handler for HTTP status mapping.
var (
	ErrNotFound         = errors.New("quotation not found")
	ErrInvalidClient    = errors.New("client not found or not active")
	ErrNoCompanyConte   = errors.New("company context is required")
	ErrPublicNotFound   = errors.New("public quotation not found")
	ErrAlreadySigned    = errors.New("quotation is already signed")
	ErrInvalidSignature = errors.New("invalid signature image")
	ErrConflict         = errors.New("conflict: cannot edit signed revision")
)

type Service struct {
	repo *repository.Repository
}

func NewService(repo *repository.Repository) *Service {
	return &Service{repo: repo}
}

// Create creates a new quotation (status draft) together with its first
// revision and optional price-line items. All inserts run in one transaction.
func (s *Service) Create(
	ctx context.Context,
	companyID, userID string,
	req *dto.CreateQuotationRequest,
) (*dto.QuotationResponse, error) {
	companyUUID, err := uuid.Parse(companyID)
	if err != nil {
		return nil, ErrNoCompanyConte
	}
	clientUUID, err := uuid.Parse(req.ClientID)
	if err != nil {
		return nil, ErrInvalidClient
	}

	exists, err := s.repo.ClientExists(ctx, s.repo.Pool(), req.ClientID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrInvalidClient
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	number, err := s.generateQuotationNumber(ctx, tx, companyID)
	if err != nil {
		return nil, err
	}

	var finalBudget int64 = req.Budget

	quot := &domain.Quotation{
		ID:              uuid.New(),
		CompanyID:       companyUUID,
		ClientID:        clientUUID,
		Subject:         req.Subject,
		QuotationNumber: number,
		ProjectType:     req.ProjectType,
		Status:          "draft",
		CurrentRevision: 1,
		CreatedBy:       uuidPtr(userID),
	}
	if err := s.repo.CreateQuotation(ctx, tx, quot); err != nil {
		return nil, err
	}

	rev := &domain.QuotationRevision{
		ID:             uuid.New(),
		CompanyID:      companyUUID,
		QuotationID:    quot.ID,
		RevisionNumber: 1,
		Budget:         finalBudget,
		CreatedBy:      uuidPtr(userID),
	}
	if err := s.repo.CreateRevision(ctx, tx, rev); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	resp := toQuotationResponse(quot)
	resp.Revision = toRevisionResponse(rev)

	logger.Info("Quotation created",
		logger.String("quotation_id", quot.ID.String()),
		logger.String("company_id", companyID),
	)
	return &resp, nil
}

// GetDetail returns a quotation with its active revision and price lines,
// scoped to the caller's company.
func (s *Service) GetDetail(ctx context.Context, companyID, id string) (*dto.QuotationResponse, error) {
	quot, err := s.repo.GetQuotationByID(ctx, id, companyID)
	if err != nil {
		return nil, mapRepoErr(err)
	}

	rev, err := s.repo.GetActiveRevision(ctx, quot.ID.String(), companyID, quot.CurrentRevision)
	if err != nil {
		return nil, mapRepoErr(err)
	}

	resp := toQuotationResponse(quot)
	resp.Revision = toRevisionResponse(rev)
	return &resp, nil
}

// Update edits the content of the active revision (description, scope, notes,
// blueprint, budget). A separate revision row is created on demand by the
// revisions endpoints — this endpoint mutates the current one.
func (s *Service) Update(
	ctx context.Context,
	companyID, id string,
	req *dto.UpdateQuotationRequest,
) (*dto.QuotationResponse, error) {
	quot, err := s.repo.GetQuotationByID(ctx, id, companyID)
	if err != nil {
		return nil, mapRepoErr(err)
	}

	rev, err := s.repo.GetActiveRevision(ctx, quot.ID.String(), companyID, quot.CurrentRevision)
	if err != nil {
		return nil, mapRepoErr(err)
	}

	if rev.SignedAt != nil {
		return nil, ErrAlreadySigned
	}

	var finalBudget *int64 = req.Budget

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rev.Budget = 0 // Will be COALESCED
	if finalBudget != nil {
		rev.Budget = *finalBudget
	}

	// Extract budget from update if provided
	if req.Budget != nil {
		rev.Budget = *req.Budget
	}

	// Persist the mapped UI values back into the DB fields
	rev.ProjectDescription = &req.ProjectDescription
	rev.ScopeOfWork = &req.ScopeOfWork
	rev.MaintenanceInfo = &req.MaintenanceInfo
	rev.HostingInfo = &req.HostingInfo
	rev.Notes = &req.Notes
	rev.QuotationDate = nil // TODO: parse req.QuotationDate
	if req.QuotationDate != nil {
		if pt, err := time.Parse("2006-01-02", *req.QuotationDate); err == nil {
			rev.QuotationDate = &pt
		}
	}
	rev.ValidUntil = nil
	if req.ValidUntil != nil {
		if pt, err := time.Parse("2006-01-02", *req.ValidUntil); err == nil {
			rev.ValidUntil = &pt
		}
	}
	rev.ClientSnapshot = req.ClientSnapshot

	updatedRev, err := s.repo.UpdateRevision(ctx, tx, rev)
	if err != nil {
		return nil, mapRepoErr(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	resp := toQuotationResponse(quot)
	resp.Revision = toRevisionResponse(updatedRev)

	logger.Info("Quotation updated",
		logger.String("quotation_id", id),
		logger.String("revision_id", updatedRev.ID.String()),
	)
	return &resp, nil
}

// CreateRevision duplicates the active revision, incrementing current_revision.
func (s *Service) CreateRevision(ctx context.Context, companyID, id string) (*dto.QuotationResponse, error) {
	quot, err := s.repo.GetQuotationByID(ctx, id, companyID)
	if err != nil {
		return nil, mapRepoErr(err)
	}

	if quot.Status == "signed" {
		return nil, ErrConflict
	}

	activeRev, err := s.repo.GetActiveRevision(ctx, quot.ID.String(), companyID, quot.CurrentRevision)
	if err != nil {
		return nil, mapRepoErr(err)
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Increment revision number
	newRevisionNumber := quot.CurrentRevision + 1
	if _, err := tx.Exec(ctx, `UPDATE quotations SET current_revision = $1, updated_at = NOW() WHERE id = $2`, newRevisionNumber, quot.ID); err != nil {
		return nil, err
	}
	quot.CurrentRevision = newRevisionNumber

	newRev := &domain.QuotationRevision{
		ID:                 uuid.New(),
		CompanyID:          activeRev.CompanyID,
		QuotationID:        activeRev.QuotationID,
		RevisionNumber:     newRevisionNumber,
		Budget:             activeRev.Budget,
		ProjectDescription: activeRev.ProjectDescription,
		ScopeOfWork:        activeRev.ScopeOfWork,
		MaintenanceInfo:    activeRev.MaintenanceInfo,
		HostingInfo:        activeRev.HostingInfo,
		Notes:              activeRev.Notes,
		QuotationDate:      activeRev.QuotationDate,
		ValidUntil:         activeRev.ValidUntil,
		ClientSnapshot:     activeRev.ClientSnapshot,
		CreatedBy:          activeRev.CreatedBy,
	}

	if err := s.repo.CreateRevision(ctx, tx, newRev); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	resp := toQuotationResponse(quot)
	resp.Revision = toRevisionResponse(newRev)

	logger.Info("Revision created",
		logger.String("quotation_id", id),
		logger.String("revision_id", newRev.ID.String()),
		logger.Int("revision_number", newRevisionNumber),
	)
	return &resp, nil
}

// ShareInternal marks the quotation as shared and creates public artifact links.
func (s *Service) ShareInternal(ctx context.Context, companyID, id string) (*dto.QuotationResponse, error) {
	quot, err := s.repo.GetQuotationByID(ctx, id, companyID)
	if err != nil {
		return nil, mapRepoErr(err)
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if quot.Status != "shared" && quot.Status != "signed" {
		if _, err := tx.Exec(ctx, `UPDATE quotations SET status = 'shared', updated_at = NOW() WHERE id = $1`, quot.ID); err != nil {
			return nil, err
		}
		quot.Status = "shared"
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Reload detail to return everything
	return s.GetDetail(ctx, companyID, id)
}

// ListRevisions returns all revisions for a quotation.
func (s *Service) ListRevisions(ctx context.Context, companyID, id string) ([]dto.QuotationRevisionResponse, error) {
	quot, err := s.repo.GetQuotationByID(ctx, id, companyID)
	if err != nil {
		return nil, mapRepoErr(err)
	}

	query := `
		SELECT id, company_id, quotation_id, revision_number, public_token, budget,
		       project_description, scope_of_work, maintenance_info, hosting_info,
		       notes, ai_raw_response, signature_image_url, signed_at,
		       signed_ip, created_by, created_at, updated_at,
		       quotation_date, valid_until, client_snapshot
		FROM quotation_revisions
		WHERE quotation_id = $1 AND company_id = $2
		ORDER BY revision_number DESC
	`
	rows, err := s.repo.Pool().Query(ctx, query, quot.ID, quot.CompanyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var revs []domain.QuotationRevision
	for rows.Next() {
		var r domain.QuotationRevision
		if err := rows.Scan(
			&r.ID, &r.CompanyID, &r.QuotationID, &r.RevisionNumber, &r.PublicToken, &r.Budget,
			&r.ProjectDescription, &r.ScopeOfWork, &r.MaintenanceInfo, &r.HostingInfo,
			&r.Notes, &r.AiRawResponse, &r.SignatureImageURL, &r.SignedAt,
			&r.SignedIP, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
			&r.QuotationDate, &r.ValidUntil, &r.ClientSnapshot,
		); err != nil {
			return nil, err
		}
		revs = append(revs, r)
	}

	var out []dto.QuotationRevisionResponse
	for i := range revs {
		out = append(out, *toRevisionResponse(&revs[i]))
	}
	return out, nil
}



// List returns a paginated list of quotations for the company, optionally
// filtered by free-text search and status.
func (s *Service) List(
	ctx context.Context,
	companyID, search, status string,
	page, limit int,
) (*dto.QuotationListResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	rows, total, err := s.repo.ListQuotations(ctx, companyID, search, status, limit, offset)
	if err != nil {
		return nil, err
	}

	items := make([]dto.QuotationResponse, len(rows))
	for i := range rows {
		items[i] = toQuotationResponse(&rows[i])
	}
	return &dto.QuotationListResponse{Items: items, Total: total}, nil
}



// generateQuotationNumber derives the next sequential number for a company,
// e.g. QUO-2026-0001. The count-based sequence is adequate for the current
// stage; a proper sequence table can replace it if collisions matter.
func (s *Service) generateQuotationNumber(ctx context.Context, q repository.Execer, companyID string) (string, error) {
	count, err := s.repo.CountQuotations(ctx, q, companyID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("QUO-%d-%04d", time.Now().UTC().Year(), count+1), nil
}

// mapRepoErr converts repository "not found" errors to service-level ones.
func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, repository.ErrRevisionNotFound):
		return ErrNotFound
	default:
		return err
	}
}

func mapPublicRepoErr(err error) error {
	if errors.Is(err, repository.ErrPublicNotFound) {
		return ErrPublicNotFound
	}
	return err
}

func uuidPtr(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	u, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &u
}

func toQuotationResponse(q *domain.Quotation) dto.QuotationResponse {
	return dto.QuotationResponse{
		ID:              q.ID,
		CompanyID:       q.CompanyID,
		ClientID:        q.ClientID,
		ClientName:      q.ClientName,
		Subject:         q.Subject,
		QuotationNumber: q.QuotationNumber,
		ProjectType:     q.ProjectType,
		Status:          q.Status,
		CurrentRevision: q.CurrentRevision,
		CreatedBy:       q.CreatedBy,
		CreatedAt:       q.CreatedAt,
		UpdatedAt:       q.UpdatedAt,
	}
}

func toRevisionResponse(rev *domain.QuotationRevision) *dto.QuotationRevisionResponse {
	var qDate, vUntil *string
	if rev.QuotationDate != nil {
		d := rev.QuotationDate.Format("2006-01-02")
		qDate = &d
	}
	if rev.ValidUntil != nil {
		d := rev.ValidUntil.Format("2006-01-02")
		vUntil = &d
	}

	return &dto.QuotationRevisionResponse{
		ID:                 rev.ID,
		RevisionNumber:     rev.RevisionNumber,
		PublicToken:        rev.PublicToken,
		Budget:             rev.Budget,
		ProjectDescription: rev.ProjectDescription,
		ScopeOfWork:        rev.ScopeOfWork,
		MaintenanceInfo:    rev.MaintenanceInfo,
		HostingInfo:        rev.HostingInfo,
		Notes:              rev.Notes,
		QuotationDate:      qDate,
		ValidUntil:         vUntil,
		ClientSnapshot:     rev.ClientSnapshot,
		SignatureImageURL:  rev.SignatureImageURL,
		SignedAt:           rev.SignedAt,
	}
}
