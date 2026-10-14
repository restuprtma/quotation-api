package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

func TestCreateRateCardRequest_AllowsEmptyItems(t *testing.T) {
	validate := validator.New()

	req := dto.CreateRateCardRequest{
		Code:          "RC-2026-TEST",
		Name:          "Test Rate Card",
		Market:        "national",
		ClientTier:    "commercial_national",
		Currency:      "IDR",
		EffectiveFrom: "2026-01-01T00:00:00Z",
		Items:         []dto.CreateRateCardItemRequest{}, // empty items allowed
	}

	err := validate.Struct(req)
	assert.NoError(t, err)
}

func TestRateCardSummaryResponse_Serialization_IncludesIsDefault(t *testing.T) {
	resp := dto.RateCardSummaryResponse{
		ID:         uuid.New(),
		Code:       "RC-2026-DEF",
		Name:       "Default Rate Card",
		Market:     "national",
		ClientTier: "commercial_national",
		Currency:   "IDR",
		IsActive:   true,
		IsDefault:  true,
		ItemCount:  10,
		CreatedAt:  time.Now(),
	}

	bytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var raw map[string]interface{}
	err = json.Unmarshal(bytes, &raw)
	require.NoError(t, err)

	assert.Equal(t, true, raw["is_default"])
}

func TestRateCardDetailResponse_Serialization_IncludesIsDefault(t *testing.T) {
	resp := dto.RateCardDetailResponse{
		ID:                    uuid.New(),
		CompanyID:             uuid.New(),
		Code:                  "RC-2026-DEF",
		Name:                  "Default Rate Card",
		Market:                "national",
		ClientTier:            "commercial_national",
		Currency:              "IDR",
		ShortTermSurchargePct: 40,
		EnterpriseMarkupPct:   35,
		IsActive:              true,
		IsDefault:             true,
		EffectiveFrom:         time.Now(),
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	bytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var raw map[string]interface{}
	err = json.Unmarshal(bytes, &raw)
	require.NoError(t, err)

	assert.Equal(t, true, raw["is_default"])
}
