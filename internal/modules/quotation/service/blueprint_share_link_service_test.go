package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

type MockShareLinkRepo struct {
	mock.Mock
}

func (m *MockShareLinkRepo) Create(ctx context.Context, link *domain.ShareLink) error {
	args := m.Called(ctx, link)
	return args.Error(0)
}

func (m *MockShareLinkRepo) Revoke(ctx context.Context, companyID, blueprintID, linkID uuid.UUID) error {
	args := m.Called(ctx, companyID, blueprintID, linkID)
	return args.Error(0)
}

func (m *MockShareLinkRepo) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*domain.ShareLink, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ShareLink), args.Error(1)
}

func (m *MockShareLinkRepo) TouchLastAccessed(ctx context.Context, linkID uuid.UUID) error {
	args := m.Called(ctx, linkID)
	return args.Error(0)
}

func (m *MockShareLinkRepo) ListByBlueprint(ctx context.Context, companyID, blueprintID uuid.UUID) ([]repository.ShareLinkWithRevision, error) {
	args := m.Called(ctx, companyID, blueprintID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]repository.ShareLinkWithRevision), args.Error(1)
}

type MockBlueprintRevisionLookup struct {
	mock.Mock
}

func (m *MockBlueprintRevisionLookup) GetRevisionForSharing(ctx context.Context, companyID, blueprintID, revisionID uuid.UUID) (*domain.BlueprintRevision, error) {
	args := m.Called(ctx, companyID, blueprintID, revisionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.BlueprintRevision), args.Error(1)
}

func TestBlueprintShareLinkService_Create_RejectsDraftRevision(t *testing.T) {
	repo := new(MockShareLinkRepo)
	bpLookup := new(MockBlueprintRevisionLookup)
	svc := service.NewBlueprintShareLinkService(repo, bpLookup)

	companyID := uuid.New()
	blueprintID := uuid.New()
	draftRevID := uuid.New()

	bpLookup.On("GetRevisionForSharing", mock.Anything, companyID, blueprintID, draftRevID).Return(&domain.BlueprintRevision{
		ID:                draftRevID,
		CompanyID:         companyID,
		BlueprintID:       blueprintID,
		RevisionNumber:    1,
		PublicationStatus: "draft",
		PublishedAt:       nil,
	}, nil)

	req := dto.CreateBlueprintShareLinkRequest{
		BlueprintRevisionID: draftRevID,
		Label:               "Draft Test",
	}

	res, err := svc.CreateShareLink(context.Background(), companyID, blueprintID, nil, req, "https://app.tuai.id")
	assert.Nil(t, res)
	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrCannotShareDraftRevision))
}

func TestBlueprintShareLinkService_Create_SuccessForPublishedOrSuperseded(t *testing.T) {
	repo := new(MockShareLinkRepo)
	bpLookup := new(MockBlueprintRevisionLookup)
	svc := service.NewBlueprintShareLinkService(repo, bpLookup)

	companyID := uuid.New()
	blueprintID := uuid.New()
	pubRevID := uuid.New()
	now := time.Now()

	bpLookup.On("GetRevisionForSharing", mock.Anything, companyID, blueprintID, pubRevID).Return(&domain.BlueprintRevision{
		ID:                pubRevID,
		CompanyID:         companyID,
		BlueprintID:       blueprintID,
		RevisionNumber:    2,
		PublicationStatus: "published",
		PublishedAt:       &now,
	}, nil)

	repo.On("Create", mock.Anything, mock.MatchedBy(func(l *domain.ShareLink) bool {
		return l.CompanyID == companyID && l.BlueprintID == blueprintID && l.BlueprintRevisionID == pubRevID && len(l.TokenHash) == 64
	})).Return(nil)

	req := dto.CreateBlueprintShareLinkRequest{
		BlueprintRevisionID: pubRevID,
		Label:               "Programmer Review",
	}

	res, err := svc.CreateShareLink(context.Background(), companyID, blueprintID, nil, req, "https://app.tuai.id")
	require.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "Programmer Review", res.Label)
	assert.Equal(t, 2, res.RevisionNumber)
	assert.Contains(t, res.URL, "https://app.tuai.id/p/blueprint/")
}

func TestBlueprintShareLinkService_ListAndRevoke(t *testing.T) {
	repo := new(MockShareLinkRepo)
	bpLookup := new(MockBlueprintRevisionLookup)
	svc := service.NewBlueprintShareLinkService(repo, bpLookup)

	companyID := uuid.New()
	blueprintID := uuid.New()
	linkID := uuid.New()

	repo.On("ListByBlueprint", mock.Anything, companyID, blueprintID).Return([]repository.ShareLinkWithRevision{
		{
			ShareLink: domain.ShareLink{
				ID:          linkID,
				CompanyID:   companyID,
				BlueprintID: blueprintID,
				Label:       "Active Link",
				CreatedAt:   time.Now(),
			},
			RevisionNumber: 2,
		},
	}, nil)

	repo.On("Revoke", mock.Anything, companyID, blueprintID, linkID).Return(nil)

	items, err := svc.ListShareLinks(context.Background(), companyID, blueprintID)
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, linkID, items[0].ID)
	assert.Equal(t, 2, items[0].RevisionNumber)
	assert.False(t, items[0].IsRevoked)

	err = svc.RevokeShareLink(context.Background(), companyID, blueprintID, linkID)
	require.NoError(t, err)
}
