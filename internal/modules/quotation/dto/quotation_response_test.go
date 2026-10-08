package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

func TestQuotationResponse_Serialization_IncludesTotalInvestmentAndPIC(t *testing.T) {
	totalInv := int64(150000000)
	pic := "Bapak Reza Mahardika"
	client := "PT Citra Niaga Perkasa"

	resp := dto.QuotationResponse{
		ID:               uuid.New(),
		CompanyID:        uuid.New(),
		ClientID:         uuid.New(),
		ClientName:       &client,
		PICName:          &pic,
		QuotationNumber:  "QUO-2026-0001",
		SolutionType:     "custom_software",
		CommercialScheme: "dedicated_squad",
		TotalInvestment:  &totalInv,
		Status:           "published",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	bytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var raw map[string]interface{}
	err = json.Unmarshal(bytes, &raw)
	require.NoError(t, err)

	assert.Equal(t, float64(150000000), raw["total_investment"])
	assert.Equal(t, "Bapak Reza Mahardika", raw["pic_name"])
	assert.Equal(t, "PT Citra Niaga Perkasa", raw["client_name"])
}

func TestQuotationResponse_ExposesPublicToken(t *testing.T) {
	testToken := uuid.New()
	resp := dto.QuotationResponse{
		ID:              uuid.New(),
		QuotationNumber: "QUO-001",
		Status:          "published",
		PublicToken:     testToken,
	}

	bytes, err := json.Marshal(resp)
	require.NoError(t, err)

	var rawMap map[string]interface{}
	err = json.Unmarshal(bytes, &rawMap)
	require.NoError(t, err)

	assert.Equal(t, testToken.String(), rawMap["public_token"])
}

