package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	ErrCannotShareDraftRevision   = errors.New("cannot share draft blueprint revision; only published or superseded revisions can be shared")
	ErrInvalidShareLinkLabel      = errors.New("label cannot be empty or exceed 255 characters")
	ErrShareLinkBlueprintNotFound = errors.New("blueprint or revision not found")
)

type ShareLinkDataRepository interface {
	Create(ctx context.Context, link *domain.ShareLink) error
	Revoke(ctx context.Context, companyID, blueprintID, linkID uuid.UUID) error
	GetActiveByTokenHash(ctx context.Context, tokenHash string) (*domain.ShareLink, error)
	TouchLastAccessed(ctx context.Context, linkID uuid.UUID) error
	ListByBlueprint(ctx context.Context, companyID, blueprintID uuid.UUID) ([]repository.ShareLinkWithRevision, error)
}

type BlueprintRevisionSharingLookup interface {
	GetRevisionForSharing(ctx context.Context, companyID, blueprintID, revisionID uuid.UUID) (*domain.BlueprintRevision, error)
}

type BlueprintShareLinkService struct {
	repo     ShareLinkDataRepository
	bpLookup BlueprintRevisionSharingLookup
}

func NewBlueprintShareLinkService(repo ShareLinkDataRepository, bpLookup BlueprintRevisionSharingLookup) *BlueprintShareLinkService {
	return &BlueprintShareLinkService{
		repo:     repo,
		bpLookup: bpLookup,
	}
}

func (s *BlueprintShareLinkService) CreateShareLink(
	ctx context.Context,
	companyID, blueprintID uuid.UUID,
	userID *uuid.UUID,
	req dto.CreateBlueprintShareLinkRequest,
	baseURL string,
) (*dto.CreateBlueprintShareLinkResponse, error) {
	label := strings.TrimSpace(req.Label)
	if label == "" || len(label) > 255 {
		return nil, ErrInvalidShareLinkLabel
	}

	rev, err := s.bpLookup.GetRevisionForSharing(ctx, companyID, blueprintID, req.BlueprintRevisionID)
	if err != nil {
		return nil, ErrShareLinkBlueprintNotFound
	}

	// Drafts CANNOT be shared! Only published or superseded revisions that were once published
	if rev.PublicationStatus == "draft" || rev.PublishedAt == nil {
		return nil, ErrCannotShareDraftRevision
	}

	rawToken, tokenHash, err := GenerateShareToken()
	if err != nil {
		return nil, err
	}

	linkID := uuid.New()
	now := time.Now().UTC()
	link := &domain.ShareLink{
		ID:                  linkID,
		CompanyID:           companyID,
		BlueprintID:         blueprintID,
		BlueprintRevisionID: rev.ID,
		TokenHash:           tokenHash,
		Label:               label,
		CreatedBy:           userID,
		CreatedAt:           now,
	}

	if err := s.repo.Create(ctx, link); err != nil {
		return nil, err
	}

	cleanBaseURL := strings.TrimRight(baseURL, "/")
	fullURL := cleanBaseURL + "/p/blueprint/" + rawToken

	return &dto.CreateBlueprintShareLinkResponse{
		ID:             linkID,
		Label:          label,
		RevisionNumber: rev.RevisionNumber,
		URL:            fullURL,
		CreatedAt:      now,
	}, nil
}

func (s *BlueprintShareLinkService) ListShareLinks(ctx context.Context, companyID, blueprintID uuid.UUID) ([]dto.BlueprintShareLinkItemResponse, error) {
	records, err := s.repo.ListByBlueprint(ctx, companyID, blueprintID)
	if err != nil {
		return nil, err
	}

	items := make([]dto.BlueprintShareLinkItemResponse, len(records))
	for i, r := range records {
		items[i] = dto.BlueprintShareLinkItemResponse{
			ID:                  r.ID,
			Label:               r.Label,
			RevisionNumber:      r.RevisionNumber,
			BlueprintRevisionID: r.BlueprintRevisionID,
			CreatedAt:           r.CreatedAt,
			RevokedAt:           r.RevokedAt,
			LastAccessedAt:      r.LastAccessedAt,
			IsRevoked:           r.IsRevoked(),
		}
	}
	return items, nil
}

func (s *BlueprintShareLinkService) RevokeShareLink(ctx context.Context, companyID, blueprintID, linkID uuid.UUID) error {
	return s.repo.Revoke(ctx, companyID, blueprintID, linkID)
}
