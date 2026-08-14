package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	ai   AIProvider
}

func NewService(repo *repository.Repository, ai AIProvider) *Service {
	return &Service{repo: repo, ai: ai}
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
	var sectionsResp []domain.QuotationPricingSection
	var allItemsResp []domain.QuotationPricingItem

	if len(req.Sections) > 0 {
		calc := NewPricingCalculator()
		oneTime, recurring, err := calc.CalculateTotals(req.Sections)
		if err != nil {
			return nil, err
		}
		finalBudget = oneTime + recurring
	}

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

	// Legacy V1 Items
	items := make([]domain.QuotationItem, 0, len(req.Items))
	for i := range req.Items {
		itemReq := req.Items[i]
		item := domain.QuotationItem{
			ID:         uuid.New(),
			CompanyID:  companyUUID,
			RevisionID: rev.ID,
			ItemName:   itemReq.ItemName,
			Amount:     itemReq.Amount,
			SortOrder:  itemReq.SortOrder,
		}
		if itemReq.Description != "" {
			d := itemReq.Description
			item.Description = &d
		}
		if err := s.repo.CreateItem(ctx, tx, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	// V2 Sections and Items
	for _, secReq := range req.Sections {
		sec := domain.QuotationPricingSection{
			ID:          uuid.New(),
			CompanyID:   companyUUID,
			RevisionID:  rev.ID,
			SectionName: secReq.SectionName,
			SectionType: secReq.SectionType,
			SortOrder:   secReq.SortOrder,
		}
		if secReq.Description != nil {
			sec.Description = secReq.Description
		}
		if err := s.repo.CreatePricingSection(ctx, tx, &sec); err != nil {
			return nil, err
		}
		sectionsResp = append(sectionsResp, sec)

		for _, itemReq := range secReq.Items {
			it := domain.QuotationPricingItem{
				ID:            uuid.New(),
				CompanyID:     companyUUID,
				SectionID:     sec.ID,
				ItemName:      itemReq.ItemName,
				Features:      itemReq.Features,
				PricingMode:   itemReq.PricingMode,
				UnitPrice:     itemReq.UnitPrice,
				Quantity:      itemReq.Quantity,
				EmployeeCount: itemReq.EmployeeCount,
				Duration:      itemReq.Duration,
				BillingPeriod: itemReq.BillingPeriod,
				LineTotal:     itemReq.LineTotal,
				SortOrder:     itemReq.SortOrder,
			}
			if itemReq.Description != nil {
				it.Description = itemReq.Description
			}
			if itemReq.UnitLabel != nil {
				it.UnitLabel = itemReq.UnitLabel
			}
			if itemReq.DurationUnit != nil {
				it.DurationUnit = itemReq.DurationUnit
			}
			if itemReq.ManualOverrideReason != nil {
				it.ManualOverrideReason = itemReq.ManualOverrideReason
			}

			if err := s.repo.CreatePricingItem(ctx, tx, &it); err != nil {
				return nil, err
			}
			allItemsResp = append(allItemsResp, it)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	resp := toQuotationResponse(quot)
	resp.Revision = toRevisionResponse(rev, items, sectionsResp, allItemsResp)

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

	items, err := s.repo.GetItemsByRevisionID(ctx, rev.ID.String())
	if err != nil {
		return nil, err
	}

	sections, err := s.repo.GetPricingSectionsByRevisionID(ctx, s.repo.Pool(), rev.ID.String())
	if err != nil {
		return nil, err
	}

	var pricingItems []domain.QuotationPricingItem
	for _, sec := range sections {
		itms, err := s.repo.GetPricingItemsBySectionID(ctx, s.repo.Pool(), sec.ID.String())
		if err != nil {
			return nil, err
		}
		pricingItems = append(pricingItems, itms...)
	}

	resp := toQuotationResponse(quot)
	resp.Revision = toRevisionResponse(rev, items, sections, pricingItems)
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
	if len(req.Sections) > 0 {
		calc := NewPricingCalculator()
		oneTime, recurring, err := calc.CalculateTotals(req.Sections)
		if err != nil {
			return nil, err
		}
		sum := oneTime + recurring
		finalBudget = &sum
	}

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

	// Re-assemble AI structure to validate it.
	// We map the raw maps from the request into our typed struct using JSON marshalling.
	// If the user modified the AI draft, it MUST still pass the schema validation rules
	// before we save it.
	tempAI := &dto.AIGenerateResponse{
		SchemaVersion: "1.0",
	}

	if req.NarrativeData != nil {
		narRaw, _ := json.Marshal(req.NarrativeData)
		_ = json.Unmarshal(narRaw, &tempAI.Narrative)
	}
	if req.ProductDetailData != nil {
		prodRaw, _ := json.Marshal(req.ProductDetailData)
		_ = json.Unmarshal(prodRaw, &tempAI.ProductOrProjectDetail)
	}
	if req.BlueprintData != nil {
		blueRaw, _ := json.Marshal(req.BlueprintData)
		_ = json.Unmarshal(blueRaw, &tempAI.Blueprint)
	}

	// If narrative or product detail data is present, we consider it a V2 AI payload
	// and run the validator over it before saving.
	if req.NarrativeData != nil || req.BlueprintData != nil {
		if err := ValidateAIBundle(tempAI); err != nil {
			return nil, err // Returns validation error (safe to surface to user)
		}
	}

	// Persist the mapped UI values back into the DB fields
	rev.ProjectDescription = &req.ProjectDescription
	rev.ScopeOfWork = &req.ScopeOfWork
	rev.MaintenanceInfo = &req.MaintenanceInfo
	rev.HostingInfo = &req.HostingInfo
	rev.Notes = &req.Notes
	rev.BlueprintData = req.BlueprintData
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
	rev.NarrativeData = req.NarrativeData
	rev.ProductDetailData = req.ProductDetailData

	updatedRev, err := s.repo.UpdateRevision(ctx, tx, rev)
	if err != nil {
		return nil, mapRepoErr(err)
	}

	// Update Sections/Items (Delete and Insert)
	if err := s.repo.DeletePricingSectionsByRevisionID(ctx, tx, updatedRev.ID.String()); err != nil {
		return nil, err
	}

	var sectionsResp []domain.QuotationPricingSection
	var allItemsResp []domain.QuotationPricingItem

	companyUUID := quot.CompanyID

	for _, secReq := range req.Sections {
		sec := domain.QuotationPricingSection{
			ID:          uuid.New(),
			CompanyID:   companyUUID,
			RevisionID:  updatedRev.ID,
			SectionName: secReq.SectionName,
			SectionType: secReq.SectionType,
			SortOrder:   secReq.SortOrder,
		}
		if secReq.Description != nil {
			sec.Description = secReq.Description
		}
		if err := s.repo.CreatePricingSection(ctx, tx, &sec); err != nil {
			return nil, err
		}
		sectionsResp = append(sectionsResp, sec)

		for _, itemReq := range secReq.Items {
			it := domain.QuotationPricingItem{
				ID:            uuid.New(),
				CompanyID:     companyUUID,
				SectionID:     sec.ID,
				ItemName:      itemReq.ItemName,
				Features:      itemReq.Features,
				PricingMode:   itemReq.PricingMode,
				UnitPrice:     itemReq.UnitPrice,
				Quantity:      itemReq.Quantity,
				EmployeeCount: itemReq.EmployeeCount,
				Duration:      itemReq.Duration,
				BillingPeriod: itemReq.BillingPeriod,
				LineTotal:     itemReq.LineTotal,
				SortOrder:     itemReq.SortOrder,
			}
			if itemReq.Description != nil {
				it.Description = itemReq.Description
			}
			if itemReq.UnitLabel != nil {
				it.UnitLabel = itemReq.UnitLabel
			}
			if itemReq.DurationUnit != nil {
				it.DurationUnit = itemReq.DurationUnit
			}
			if itemReq.ManualOverrideReason != nil {
				it.ManualOverrideReason = itemReq.ManualOverrideReason
			}

			if err := s.repo.CreatePricingItem(ctx, tx, &it); err != nil {
				return nil, err
			}
			allItemsResp = append(allItemsResp, it)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	items, err := s.repo.GetItemsByRevisionID(ctx, updatedRev.ID.String())
	if err != nil {
		return nil, err
	}

	resp := toQuotationResponse(quot)
	resp.Revision = toRevisionResponse(updatedRev, items, sectionsResp, allItemsResp)

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
		BlueprintData:      activeRev.BlueprintData,
		QuotationDate:      activeRev.QuotationDate,
		ValidUntil:         activeRev.ValidUntil,
		ClientSnapshot:     activeRev.ClientSnapshot,
		NarrativeData:      activeRev.NarrativeData,
		ProductDetailData:  activeRev.ProductDetailData,
		CreatedBy:          activeRev.CreatedBy,
	}

	if err := s.repo.CreateRevision(ctx, tx, newRev); err != nil {
		return nil, err
	}

	// Copy V1 Items
	oldItems, err := s.repo.GetItemsByRevisionID(ctx, activeRev.ID.String())
	if err != nil {
		return nil, err
	}
	var newItems []domain.QuotationItem
	for _, it := range oldItems {
		newIt := domain.QuotationItem{
			ID:          uuid.New(),
			CompanyID:   it.CompanyID,
			RevisionID:  newRev.ID,
			ItemName:    it.ItemName,
			Description: it.Description,
			Amount:      it.Amount,
			SortOrder:   it.SortOrder,
		}
		if err := s.repo.CreateItem(ctx, tx, &newIt); err != nil {
			return nil, err
		}
		newItems = append(newItems, newIt)
	}

	// Copy V2 Sections & Items
	oldSections, err := s.repo.GetPricingSectionsByRevisionID(ctx, tx, activeRev.ID.String())
	if err != nil {
		return nil, err
	}
	var newSectionsResp []domain.QuotationPricingSection
	var newAllItemsResp []domain.QuotationPricingItem

	for _, sec := range oldSections {
		newSec := domain.QuotationPricingSection{
			ID:          uuid.New(),
			CompanyID:   sec.CompanyID,
			RevisionID:  newRev.ID,
			SectionName: sec.SectionName,
			SectionType: sec.SectionType,
			Description: sec.Description,
			SortOrder:   sec.SortOrder,
		}
		if err := s.repo.CreatePricingSection(ctx, tx, &newSec); err != nil {
			return nil, err
		}
		newSectionsResp = append(newSectionsResp, newSec)

		oldSecItems, err := s.repo.GetPricingItemsBySectionID(ctx, tx, sec.ID.String())
		if err != nil {
			return nil, err
		}
		for _, oItem := range oldSecItems {
			nItem := domain.QuotationPricingItem{
				ID:                   uuid.New(),
				CompanyID:            oItem.CompanyID,
				SectionID:            newSec.ID,
				ItemName:             oItem.ItemName,
				Description:          oItem.Description,
				Features:             oItem.Features,
				PricingMode:          oItem.PricingMode,
				UnitLabel:            oItem.UnitLabel,
				UnitPrice:            oItem.UnitPrice,
				Quantity:             oItem.Quantity,
				EmployeeCount:        oItem.EmployeeCount,
				Duration:             oItem.Duration,
				DurationUnit:         oItem.DurationUnit,
				BillingPeriod:        oItem.BillingPeriod,
				LineTotal:            oItem.LineTotal,
				ManualOverrideReason: oItem.ManualOverrideReason,
				SortOrder:            oItem.SortOrder,
			}
			if err := s.repo.CreatePricingItem(ctx, tx, &nItem); err != nil {
				return nil, err
			}
			newAllItemsResp = append(newAllItemsResp, nItem)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	resp := toQuotationResponse(quot)
	resp.Revision = toRevisionResponse(newRev, newItems, newSectionsResp, newAllItemsResp)

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

	activeRev, err := s.repo.GetActiveRevision(ctx, quot.ID.String(), companyID, quot.CurrentRevision)
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

	// Revoke old artifacts for this quotation
	if err := s.repo.RevokeArtifactsByQuotationID(ctx, tx, quot.ID.String()); err != nil {
		return nil, err
	}

	// Create 3 new artifacts
	now := time.Now()
	types := []string{"quotation", "blueprint", "detail"}
	for _, t := range types {
		artifact := domain.QuotationPublicArtifact{
			ID:           uuid.New(),
			CompanyID:    activeRev.CompanyID,
			RevisionID:   activeRev.ID,
			ArtifactType: t,
			PublicToken:  uuid.New(),
			PublishedAt:  &now,
		}
		if err := s.repo.CreatePublicArtifact(ctx, tx, &artifact); err != nil {
			return nil, err
		}
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

	// For now, we can just return a list without Sections to keep it simple, or we can fetch them.
	// We'll leave it as a TODO or return just active revisions.
	// But Phase 3 implies we need ListRevisions Endpoint, so let's write a simple repo method.
	query := `
		SELECT id, company_id, quotation_id, revision_number, public_token, budget,
		       project_description, scope_of_work, maintenance_info, hosting_info,
		       notes, blueprint_data, ai_raw_response, signature_image_url, signed_at,
		       signed_ip, created_by, created_at, updated_at,
		       quotation_date, valid_until, client_snapshot, narrative_data, product_detail_data
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
			&r.Notes, &r.BlueprintData, &r.AiRawResponse, &r.SignatureImageURL, &r.SignedAt,
			&r.SignedIP, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
			&r.QuotationDate, &r.ValidUntil, &r.ClientSnapshot, &r.NarrativeData, &r.ProductDetailData,
		); err != nil {
			return nil, err
		}
		revs = append(revs, r)
	}

	var out []dto.QuotationRevisionResponse
	for i := range revs {
		// Just simple mapping without sections/items for summary view, or we can fetch them.
		items, _ := s.repo.GetItemsByRevisionID(ctx, revs[i].ID.String())
		sections, _ := s.repo.GetPricingSectionsByRevisionID(ctx, s.repo.Pool(), revs[i].ID.String())
		var pricingItems []domain.QuotationPricingItem
		for _, sec := range sections {
			itms, _ := s.repo.GetPricingItemsBySectionID(ctx, s.repo.Pool(), sec.ID.String())
			pricingItems = append(pricingItems, itms...)
		}
		out = append(out, *toRevisionResponse(&revs[i], items, sections, pricingItems))
	}
	return out, nil
}

// GetArtifacts returns the public artifacts for the active revision.
func (s *Service) GetArtifacts(ctx context.Context, companyID, id string) ([]domain.QuotationPublicArtifact, error) {
	quot, err := s.repo.GetQuotationByID(ctx, id, companyID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	activeRev, err := s.repo.GetActiveRevision(ctx, quot.ID.String(), companyID, quot.CurrentRevision)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return s.repo.GetPublicArtifactsByRevisionID(ctx, s.repo.Pool(), activeRev.ID.String())
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

// GenerateAIDraft delegates to the configured AI provider. Currently backed
// by the MockAIProvider so the frontend can be built without an LLM key.
func (s *Service) GenerateAIDraft(ctx context.Context, req *dto.AIGenerateRequest) (*dto.AIGenerateResponse, error) {
	return s.ai.GenerateDraft(ctx, req)
}

// GetPublicQuotation returns the safe public representation for a quotation token.
func (s *Service) GetPublicQuotation(ctx context.Context, publicToken string) (*dto.PublicQuotationResponse, error) {
	if _, err := uuid.Parse(publicToken); err != nil {
		return nil, ErrPublicNotFound
	}
	quot, rev, items, err := s.repo.GetPublicQuotation(ctx, publicToken, "quotation")
	if err != nil {
		return nil, mapPublicRepoErr(err)
	}
	return toPublicResponse(quot, rev, items), nil
}

// GetPublicBlueprint returns the safe public representation for a blueprint token.
func (s *Service) GetPublicBlueprint(ctx context.Context, publicToken string) (*dto.PublicBlueprintResponse, error) {
	if _, err := uuid.Parse(publicToken); err != nil {
		return nil, ErrPublicNotFound
	}
	quot, rev, _, err := s.repo.GetPublicQuotation(ctx, publicToken, "blueprint")
	if err != nil {
		return nil, mapPublicRepoErr(err)
	}
	
	resp := &dto.PublicBlueprintResponse{
		QuotationNumber: quot.QuotationNumber,
		ClientName:      quot.ClientName,
		Subject:         quot.Subject,
	}
	if rev.BlueprintData != nil {
		resp.BlueprintData = rev.BlueprintData
	}
	return resp, nil
}

// GetPublicDetail returns the safe public representation for a detail token.
func (s *Service) GetPublicDetail(ctx context.Context, publicToken string) (*dto.PublicDetailResponse, error) {
	if _, err := uuid.Parse(publicToken); err != nil {
		return nil, ErrPublicNotFound
	}
	quot, rev, _, err := s.repo.GetPublicQuotation(ctx, publicToken, "detail")
	if err != nil {
		return nil, mapPublicRepoErr(err)
	}
	
	resp := &dto.PublicDetailResponse{
		QuotationNumber: quot.QuotationNumber,
		ClientName:      quot.ClientName,
		Subject:         quot.Subject,
	}
	if rev.NarrativeData != nil {
		resp.NarrativeData = rev.NarrativeData
	}
	if rev.ProductDetailData != nil {
		resp.ProductDetailData = rev.ProductDetailData
	}
	return resp, nil
}

// SignPublic validates and stores a one-time client signature.
func (s *Service) SignPublic(ctx context.Context, publicToken, signatureData, signedIP string) (*dto.PublicQuotationResponse, error) {
	if _, err := uuid.Parse(publicToken); err != nil {
		return nil, ErrPublicNotFound
	}
	if err := validateSignatureData(signatureData); err != nil {
		return nil, err
	}
	if err := s.repo.SignPublicRevision(ctx, publicToken, signatureData, signedIP); err != nil {
		switch {
		case errors.Is(err, repository.ErrPublicNotFound):
			return nil, ErrPublicNotFound
		case errors.Is(err, repository.ErrAlreadySigned):
			return nil, ErrAlreadySigned
		default:
			return nil, err
		}
	}
	return s.GetPublicQuotation(ctx, publicToken)
}

func validateSignatureData(signatureData string) error {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(signatureData, prefix) {
		return ErrInvalidSignature
	}
	
	// Max encoded size to prevent huge allocations during decode (approx 3MB for 2MB decoded)
	if len(signatureData) > 3*1024*1024 {
		return ErrInvalidSignature
	}
	
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(signatureData, prefix))
	if err != nil || len(decoded) == 0 || len(decoded) > 2*1024*1024 {
		return ErrInvalidSignature
	}
	return nil
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

func toRevisionResponse(
	rev *domain.QuotationRevision,
	items []domain.QuotationItem,
	sections []domain.QuotationPricingSection,
	pricingItems []domain.QuotationPricingItem,
) *dto.QuotationRevisionResponse {
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
		BlueprintData:      rev.BlueprintData,
		QuotationDate:      qDate,
		ValidUntil:         vUntil,
		ClientSnapshot:     rev.ClientSnapshot,
		NarrativeData:      rev.NarrativeData,
		ProductDetailData:  rev.ProductDetailData,
		SignatureImageURL:  rev.SignatureImageURL,
		SignedAt:           rev.SignedAt,
		Items:              toItemsResponse(items),
		Sections:           toSectionsResponse(sections, pricingItems),
	}
}

func toSectionsResponse(sections []domain.QuotationPricingSection, items []domain.QuotationPricingItem) []dto.PricingSectionResponse {
	if len(sections) == 0 {
		return nil
	}
	itemMap := make(map[uuid.UUID][]dto.PricingItemResponse)
	for _, it := range items {
		itemMap[it.SectionID] = append(itemMap[it.SectionID], dto.PricingItemResponse{
			ID:                   it.ID,
			ItemName:             it.ItemName,
			Description:          it.Description,
			Features:             it.Features,
			PricingMode:          it.PricingMode,
			UnitLabel:            it.UnitLabel,
			UnitPrice:            it.UnitPrice,
			Quantity:             it.Quantity,
			EmployeeCount:        it.EmployeeCount,
			Duration:             it.Duration,
			DurationUnit:         it.DurationUnit,
			BillingPeriod:        it.BillingPeriod,
			LineTotal:            it.LineTotal,
			ManualOverrideReason: it.ManualOverrideReason,
			SortOrder:            it.SortOrder,
		})
	}

	var out []dto.PricingSectionResponse
	for _, sec := range sections {
		out = append(out, dto.PricingSectionResponse{
			ID:          sec.ID,
			SectionName: sec.SectionName,
			SectionType: sec.SectionType,
			Description: sec.Description,
			SortOrder:   sec.SortOrder,
			Items:       itemMap[sec.ID],
		})
	}
	return out
}

func toItemsResponse(items []domain.QuotationItem) []dto.QuotationItemResponse {
	out := make([]dto.QuotationItemResponse, len(items))
	for i := range items {
		out[i] = dto.QuotationItemResponse{
			ID:          items[i].ID,
			ItemName:    items[i].ItemName,
			Description: items[i].Description,
			Amount:      items[i].Amount,
			SortOrder:   items[i].SortOrder,
		}
	}
	return out
}

func toPublicResponse(q *domain.Quotation, rev *domain.QuotationRevision, items []domain.QuotationItem) *dto.PublicQuotationResponse {
	publicItems := make([]dto.PublicQuotationItemResponse, len(items))
	for i := range items {
		publicItems[i] = dto.PublicQuotationItemResponse{
			ItemName: items[i].ItemName, Description: items[i].Description,
			Amount: items[i].Amount, SortOrder: items[i].SortOrder,
		}
	}

	resp := &dto.PublicQuotationResponse{
		QuotationNumber: q.QuotationNumber,
		ClientName:      q.ClientName,
		ProjectType:     q.ProjectType,
		Status:          q.Status,
		CreatedAt:       q.CreatedAt,
		Revision: &dto.PublicQuotationRevisionResponse{
			RevisionNumber:     rev.RevisionNumber,
			PublicToken:        rev.PublicToken.String(),
			Budget:             rev.Budget,
			ProjectDescription: rev.ProjectDescription,
			ScopeOfWork:        rev.ScopeOfWork,
			MaintenanceInfo:    rev.MaintenanceInfo,
			HostingInfo:        rev.HostingInfo,
			Notes:              rev.Notes,
			SignatureImageURL:  rev.SignatureImageURL,
			SignedAt:           rev.SignedAt,
			Items:              publicItems,
		},
	}
	if rev.NarrativeData != nil {
		resp.Revision.NarrativeData = rev.NarrativeData
	}
	return resp
}
