package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

type MockSkillRepo struct {
	mock.Mock
}

func (m *MockSkillRepo) GetActiveSkillByCommand(ctx context.Context, companyID, command string) (*domain.Skill, error) {
	args := m.Called(ctx, companyID, command)
	var skill *domain.Skill
	if args.Get(0) != nil {
		skill = args.Get(0).(*domain.Skill)
	}
	return skill, args.Error(1)
}

func (m *MockSkillRepo) Create(ctx context.Context, q repository.Execer, skill *domain.Skill) error {
	args := m.Called(ctx, q, skill)
	return args.Error(0)
}

func (m *MockSkillRepo) Update(ctx context.Context, q repository.Execer, skill *domain.Skill) error {
	args := m.Called(ctx, q, skill)
	return args.Error(0)
}

func TestSkillService_Create(t *testing.T) {
	companyID := uuid.New()

	tests := []struct {
		name          string
		inputSkill    *domain.Skill
		mockSetup     func(*MockSkillRepo, *domain.Skill)
		expectedError string
		expectedCmd   string
	}{
		{
			name: "Success - new skill",
			inputSkill: &domain.Skill{
				CompanyID: companyID,
				Command:   "TestCommand",
			},
			mockSetup: func(m *MockSkillRepo, skill *domain.Skill) {
				m.On("GetActiveSkillByCommand", mock.Anything, companyID.String(), "/testcommand").Return(nil, repository.ErrSkillNotFound)
				m.On("Create", mock.Anything, mock.Anything, skill).Return(nil)
			},
			expectedCmd: "/testcommand",
		},
		{
			name: "Success - already has slash",
			inputSkill: &domain.Skill{
				CompanyID: companyID,
				Command:   "/TestCommand",
			},
			mockSetup: func(m *MockSkillRepo, skill *domain.Skill) {
				m.On("GetActiveSkillByCommand", mock.Anything, companyID.String(), "/testcommand").Return(nil, repository.ErrSkillNotFound)
				m.On("Create", mock.Anything, mock.Anything, skill).Return(nil)
			},
			expectedCmd: "/testcommand",
		},
		{
			name: "Error - duplicate command",
			inputSkill: &domain.Skill{
				CompanyID: companyID,
				Command:   "testcommand",
			},
			mockSetup: func(m *MockSkillRepo, skill *domain.Skill) {
				m.On("GetActiveSkillByCommand", mock.Anything, companyID.String(), "/testcommand").Return(&domain.Skill{ID: uuid.New()}, nil)
			},
			expectedError: "active skill with this command already exists",
			expectedCmd:   "/testcommand",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockSkillRepo)
			tt.mockSetup(repo, tt.inputSkill)
			svc := service.NewSkillService(repo)

			err := svc.Create(context.Background(), nil, tt.inputSkill)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedCmd, tt.inputSkill.Command)
			repo.AssertExpectations(t)
		})
	}
}
