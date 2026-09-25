package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

type MockClientOrgRepo struct {
	mock.Mock
}

func (m *MockClientOrgRepo) Create(ctx context.Context, org *domain.ClientOrganization) error {
	args := m.Called(ctx, org)
	return args.Error(0)
}

func (m *MockClientOrgRepo) List(ctx context.Context, companyID uuid.UUID) ([]*domain.ClientOrganization, error) {
	args := m.Called(ctx, companyID)
	if args.Get(0) != nil {
		return args.Get(0).([]*domain.ClientOrganization), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockClientOrgRepo) FindByNormalizedName(ctx context.Context, companyID uuid.UUID, normalizedName string) (*domain.ClientOrganization, error) {
	args := m.Called(ctx, companyID, normalizedName)
	if args.Get(0) != nil {
		return args.Get(0).(*domain.ClientOrganization), args.Error(1)
	}
	return nil, args.Error(1)
}

func TestClientOrganizationService_Create(t *testing.T) {
	companyID := uuid.New()
	userID := uuid.New()

	t.Run("Success - valid name normalized", func(t *testing.T) {
		repo := new(MockClientOrgRepo)
		svc := service.NewClientOrganizationService(repo)

		repo.On("FindByNormalizedName", mock.Anything, companyID, "pt venturo teknologi indonesia").
			Return(nil, repository.ErrClientOrganizationNotFound)

		repo.On("Create", mock.Anything, mock.MatchedBy(func(org *domain.ClientOrganization) bool {
			return org.CompanyID == companyID &&
				org.Name == "PT Venturo Teknologi Indonesia" &&
				org.NormalizedName == "pt venturo teknologi indonesia" &&
				org.CreatedBy != nil && *org.CreatedBy == userID
		})).Return(nil)

		resp, err := svc.Create(context.Background(), companyID, &userID, dto.CreateClientOrganizationRequest{
			Name: "   PT   Venturo \t Teknologi   Indonesia   ",
		})

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "PT Venturo Teknologi Indonesia", resp.Name)
		assert.Equal(t, "pt venturo teknologi indonesia", resp.NormalizedName)
		assert.Equal(t, companyID, resp.CompanyID)
		assert.Equal(t, &userID, resp.CreatedBy)
		repo.AssertExpectations(t)
	})

	t.Run("Validation Error - empty name", func(t *testing.T) {
		repo := new(MockClientOrgRepo)
		svc := service.NewClientOrganizationService(repo)

		resp, err := svc.Create(context.Background(), companyID, &userID, dto.CreateClientOrganizationRequest{
			Name: "   \t \n  ",
		})

		assert.ErrorIs(t, err, service.ErrInvalidClientOrgName)
		assert.Nil(t, resp)
		repo.AssertNotCalled(t, "FindByNormalizedName")
		repo.AssertNotCalled(t, "Create")
	})

	t.Run("Validation Error - name exceeds 255 chars", func(t *testing.T) {
		repo := new(MockClientOrgRepo)
		svc := service.NewClientOrganizationService(repo)

		tooLong := strings.Repeat("a", 256)
		resp, err := svc.Create(context.Background(), companyID, &userID, dto.CreateClientOrganizationRequest{
			Name: tooLong,
		})

		assert.ErrorIs(t, err, service.ErrInvalidClientOrgName)
		assert.Nil(t, resp)
		repo.AssertNotCalled(t, "FindByNormalizedName")
		repo.AssertNotCalled(t, "Create")
	})

	t.Run("Duplicate Error - pre-check found existing", func(t *testing.T) {
		repo := new(MockClientOrgRepo)
		svc := service.NewClientOrganizationService(repo)

		existingOrg := &domain.ClientOrganization{
			ID:             uuid.New(),
			CompanyID:      companyID,
			Name:           "Acme Corp",
			NormalizedName: "acme corp",
			CreatedAt:      time.Now(),
		}
		repo.On("FindByNormalizedName", mock.Anything, companyID, "acme corp").
			Return(existingOrg, nil)

		resp, err := svc.Create(context.Background(), companyID, &userID, dto.CreateClientOrganizationRequest{
			Name: " ACME   CORP ",
		})

		assert.ErrorIs(t, err, service.ErrClientOrganizationDuplicate)
		assert.Nil(t, resp)
		repo.AssertExpectations(t)
		repo.AssertNotCalled(t, "Create")
	})

	t.Run("Duplicate Error - repo returns duplicate error", func(t *testing.T) {
		repo := new(MockClientOrgRepo)
		svc := service.NewClientOrganizationService(repo)

		repo.On("FindByNormalizedName", mock.Anything, companyID, "acme corp").
			Return(nil, repository.ErrClientOrganizationNotFound)

		repo.On("Create", mock.Anything, mock.Anything).
			Return(repository.ErrClientOrganizationDuplicate)

		resp, err := svc.Create(context.Background(), companyID, &userID, dto.CreateClientOrganizationRequest{
			Name: "Acme Corp",
		})

		assert.ErrorIs(t, err, service.ErrClientOrganizationDuplicate)
		assert.Nil(t, resp)
		repo.AssertExpectations(t)
	})
}

func TestClientOrganizationService_List(t *testing.T) {
	companyID := uuid.New()

	t.Run("Success - returns list of client organizations", func(t *testing.T) {
		repo := new(MockClientOrgRepo)
		svc := service.NewClientOrganizationService(repo)

		mockList := []*domain.ClientOrganization{
			{
				ID:             uuid.New(),
				CompanyID:      companyID,
				Name:           "Alpha Tech",
				NormalizedName: "alpha tech",
				CreatedAt:      time.Now(),
			},
			{
				ID:             uuid.New(),
				CompanyID:      companyID,
				Name:           "Beta Solutions",
				NormalizedName: "beta solutions",
				CreatedAt:      time.Now(),
			},
		}

		repo.On("List", mock.Anything, companyID).Return(mockList, nil)

		resp, err := svc.List(context.Background(), companyID)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Items, 2)
		assert.Equal(t, "Alpha Tech", resp.Items[0].Name)
		assert.Equal(t, "Beta Solutions", resp.Items[1].Name)
		repo.AssertExpectations(t)
	})

	t.Run("Success - returns empty list", func(t *testing.T) {
		repo := new(MockClientOrgRepo)
		svc := service.NewClientOrganizationService(repo)

		repo.On("List", mock.Anything, companyID).Return([]*domain.ClientOrganization{}, nil)

		resp, err := svc.List(context.Background(), companyID)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Empty(t, resp.Items)
		repo.AssertExpectations(t)
	})
}
