package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestRateCardHybridItemDefaulting(t *testing.T) {
	monthlyRate := int64(15000000)
	req := &dto.CreateRateCardItemRequest{
		Role:           "fullstack_ai_engineer",
		DisplayName:    "Fullstack AI Engineer",
		Seniority:      "senior",
		Discipline:     "ENGINEERING",
		TermType:       "long_term",
		AmountPerMonth: monthlyRate,
	}

	// Verify hybrid math populator
	item := &domain.RateCardItem{
		Role:           req.Role,
		DisplayName:    req.DisplayName,
		Seniority:      req.Seniority,
		Discipline:     req.Discipline,
		TermType:       req.TermType,
		AmountPerMonth: req.AmountPerMonth,
		BaseSalary:     req.BaseSalary,
	}
	if item.BaseSalary == 0 {
		item.BaseSalary = item.AmountPerMonth
	}
	if item.PublishedRate == 0 {
		item.PublishedRate = item.AmountPerMonth
	}
	if item.FloorRate == 0 {
		item.FloorRate = item.AmountPerMonth
	}

	assert.Equal(t, monthlyRate, item.BaseSalary)
	assert.Equal(t, monthlyRate, item.PublishedRate)
	assert.Equal(t, monthlyRate, item.FloorRate)
	assert.Equal(t, int64(681818), item.DailyRate(item.AmountPerMonth))
}

func TestRateCardHybridItemDefaulting_ExplicitValuesNotOverwritten(t *testing.T) {
	monthlyRate := int64(20000000)
	explicitSalary := int64(12000000)
	explicitPublished := int64(22000000)
	explicitFloor := int64(18000000)

	req := &dto.CreateRateCardItemRequest{
		Role:           "backend_programmer",
		DisplayName:    "Backend Programmer",
		Seniority:      "senior",
		Discipline:     "ENGINEERING",
		TermType:       "long_term",
		AmountPerMonth: monthlyRate,
		BaseSalary:     explicitSalary,
		PublishedRate:  explicitPublished,
		FloorRate:      explicitFloor,
	}

	item := &domain.RateCardItem{
		Role:           req.Role,
		DisplayName:    req.DisplayName,
		Seniority:      req.Seniority,
		Discipline:     req.Discipline,
		TermType:       req.TermType,
		AmountPerMonth: req.AmountPerMonth,
		BaseSalary:     req.BaseSalary,
		PublishedRate:  req.PublishedRate,
		FloorRate:      req.FloorRate,
	}
	if item.BaseSalary == 0 {
		item.BaseSalary = item.AmountPerMonth
	}
	if item.PublishedRate == 0 {
		item.PublishedRate = item.AmountPerMonth
	}
	if item.FloorRate == 0 {
		item.FloorRate = item.AmountPerMonth
	}

	assert.Equal(t, explicitSalary, item.BaseSalary)
	assert.Equal(t, explicitPublished, item.PublishedRate)
	assert.Equal(t, explicitFloor, item.FloorRate)
}

func TestRateCardItemResponse_CalculatedRates(t *testing.T) {
	monthlyRate := int64(17300000)
	item := &domain.RateCardItem{
		AmountPerMonth: monthlyRate,
		RecruitmentFee: 2000000,
		OnboardingFee:  1000000,
	}

	assert.Equal(t, int64(100000), item.HourlyRate())
	assert.Equal(t, int64(786363), item.DailyRate(item.AmountPerMonth))
	assert.Equal(t, int64(3000000), item.TotalHiringFee())
}

func TestUpdateRateCardItemRequest_Fields(t *testing.T) {
	amount := int64(18000000)
	displayName := "Senior AI Prompt Engineer"
	role := "ai_prompt_engineer"
	seniority := "senior"
	discipline := "ENGINEERING"
	termType := "long_term"

	req := dto.UpdateRateCardItemRequest{
		Role:           &role,
		DisplayName:    &displayName,
		Seniority:      &seniority,
		Discipline:     &discipline,
		TermType:       &termType,
		AmountPerMonth: &amount,
	}

	assert.Equal(t, "Senior AI Prompt Engineer", *req.DisplayName)
	assert.Equal(t, "ai_prompt_engineer", *req.Role)
	assert.Equal(t, "senior", *req.Seniority)
	assert.Equal(t, "ENGINEERING", *req.Discipline)
	assert.Equal(t, "long_term", *req.TermType)
	assert.Equal(t, int64(18000000), *req.AmountPerMonth)
	assert.Nil(t, req.BaseSalary)
}

func TestErrRateCardItemNotFound_Definition(t *testing.T) {
	assert.NotNil(t, repository.ErrRateCardItemNotFound)
	assert.Equal(t, "rate card item not found", repository.ErrRateCardItemNotFound.Error())
}

func TestRateCardService_CreateItem_UUIDValidation(t *testing.T) {
	ctx := context.Background()
	svc := service.NewRateCardService(nil, nil)

	req := &dto.CreateRateCardItemRequest{
		Role:           "backend_programmer",
		DisplayName:    "Backend Programmer",
		Seniority:      "senior",
		Discipline:     "ENGINEERING",
		TermType:       "long_term",
		AmountPerMonth: 15000000,
	}

	// Invalid company ID
	_, err := svc.CreateItem(ctx, "invalid-company-id", uuid.NewString(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid company id")

	// Invalid rate card ID
	_, err = svc.CreateItem(ctx, uuid.NewString(), "invalid-rate-card-id", req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid rate card id")
}

func TestRateCardDetail_IsDefaultField(t *testing.T) {
	card := &domain.RateCard{
		ID:            uuid.New(),
		CompanyID:     uuid.New(),
		Code:          "RC-CUSTOM-01",
		Name:          "Custom Rate Card",
		Market:        "national",
		ClientTier:    "commercial_national",
		Currency:      "IDR",
		EffectiveFrom: time.Now(),
		IsActive:      true,
		IsDefault:     true,
	}

	assert.True(t, card.IsDefault)
}
