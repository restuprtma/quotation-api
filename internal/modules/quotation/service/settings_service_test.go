package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

// Mock DBPool
type MockDBPool struct {
	mock.Mock
}

func (m *MockDBPool) Begin(ctx context.Context) (pgx.Tx, error) {
	args := m.Called(ctx)
	if args.Get(0) != nil {
		return args.Get(0).(pgx.Tx), args.Error(1)
	}
	return nil, args.Error(1)
}

// Mock Tx
type MockTx struct {
	mock.Mock
	pgx.Tx
}

func (m *MockTx) Commit(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockTx) Rollback(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Mock Repositories
type MockRateCardRepo struct {
	mock.Mock
}

func (m *MockRateCardRepo) GetActiveRateCard(ctx context.Context, companyID, market string) (*domain.RateCard, error) {
	args := m.Called(ctx, companyID, market)
	var rc *domain.RateCard
	if args.Get(0) != nil {
		rc = args.Get(0).(*domain.RateCard)
	}
	return rc, args.Error(1)
}

func (m *MockRateCardRepo) Create(ctx context.Context, q repository.Execer, rc *domain.RateCard) error {
	args := m.Called(ctx, q, rc)
	return args.Error(0)
}

type MockLegalTemplateRepo struct {
	mock.Mock
}

func (m *MockLegalTemplateRepo) GetActiveTemplate(ctx context.Context, companyID, code string) (*domain.LegalTemplate, error) {
	args := m.Called(ctx, companyID, code)
	var tmpl *domain.LegalTemplate
	if args.Get(0) != nil {
		tmpl = args.Get(0).(*domain.LegalTemplate)
	}
	return tmpl, args.Error(1)
}

func (m *MockLegalTemplateRepo) Create(ctx context.Context, q repository.Execer, tmpl *domain.LegalTemplate) error {
	args := m.Called(ctx, q, tmpl)
	return args.Error(0)
}

func TestSettingsService_EnsureDefaultAIAssets_Idempotent(t *testing.T) {
	companyID := uuid.New().String()

	dbPool := new(MockDBPool)
	tx := new(MockTx)
	skillRepo := new(MockSkillRepo)
	rcRepo := new(MockRateCardRepo)
	legalRepo := new(MockLegalTemplateRepo)
	skillSvc := service.NewSkillService(skillRepo)

	dbPool.On("Begin", mock.Anything).Return(tx, nil)
	tx.On("Rollback", mock.Anything).Return(nil)
	tx.On("Commit", mock.Anything).Return(nil)

	// Idempotency test: all exist
	skillRepo.On("GetActiveSkillByCommand", mock.Anything, companyID, "/quotation").Return(&domain.Skill{}, nil)
	skillRepo.On("GetActiveSkillByCommand", mock.Anything, companyID, "/blueprint").Return(&domain.Skill{}, nil)
	rcRepo.On("GetActiveRateCard", mock.Anything, companyID, "DEFAULT").Return(&domain.RateCard{}, nil)
	legalRepo.On("GetActiveTemplate", mock.Anything, companyID, "QUOTATION_TERMS").Return(&domain.LegalTemplate{}, nil)

	settingsSvc := service.NewSettingsService(dbPool, skillRepo, rcRepo, legalRepo, skillSvc)
	err := settingsSvc.EnsureDefaultAIAssets(context.Background(), companyID)

	assert.NoError(t, err)
	// Assert create was never called since they exist
	skillRepo.AssertNotCalled(t, "Create")
	rcRepo.AssertNotCalled(t, "Create")
	legalRepo.AssertNotCalled(t, "Create")
}

func TestSettingsService_EnsureDefaultAIAssets_CreateAll(t *testing.T) {
	companyID := uuid.New().String()

	dbPool := new(MockDBPool)
	tx := new(MockTx)
	skillRepo := new(MockSkillRepo)
	rcRepo := new(MockRateCardRepo)
	legalRepo := new(MockLegalTemplateRepo)
	skillSvc := service.NewSkillService(skillRepo)

	dbPool.On("Begin", mock.Anything).Return(tx, nil)
	tx.On("Rollback", mock.Anything).Return(nil)
	tx.On("Commit", mock.Anything).Return(nil)

	// Idempotency test: none exist
	skillRepo.On("GetActiveSkillByCommand", mock.Anything, companyID, "/quotation").Return(nil, repository.ErrSkillNotFound)
	skillRepo.On("Create", mock.Anything, tx, mock.AnythingOfType("*domain.Skill")).Return(nil).Times(2)

	skillRepo.On("GetActiveSkillByCommand", mock.Anything, companyID, "/blueprint").Return(nil, repository.ErrSkillNotFound)

	rcRepo.On("GetActiveRateCard", mock.Anything, companyID, "DEFAULT").Return(nil, repository.ErrRateCardNotFound)
	rcRepo.On("Create", mock.Anything, tx, mock.AnythingOfType("*domain.RateCard")).Return(nil)

	legalRepo.On("GetActiveTemplate", mock.Anything, companyID, "QUOTATION_TERMS").Return(nil, repository.ErrLegalTemplateNotFound)
	legalRepo.On("Create", mock.Anything, tx, mock.AnythingOfType("*domain.LegalTemplate")).Return(nil)

	settingsSvc := service.NewSettingsService(dbPool, skillRepo, rcRepo, legalRepo, skillSvc)
	err := settingsSvc.EnsureDefaultAIAssets(context.Background(), companyID)

	assert.NoError(t, err)
	skillRepo.AssertExpectations(t)
	rcRepo.AssertExpectations(t)
	legalRepo.AssertExpectations(t)
	tx.AssertExpectations(t)
}
