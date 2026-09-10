package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/handler"
	jwtpkg "venturo-skeleton-go/pkg/jwt"
)

func talentOutsourcingPreviewRouter(companyID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Omit-Company") == "" {
			c.Set(middleware.UserContextKey, &jwtpkg.Claims{CompanyID: companyID.String()})
		}
		c.Next()
	})
	h := handler.NewQuotationGenerationHandler(nil, nil)
	router.POST("/quotation/v1/quotations/preview/talent-outsourcing", h.PreviewTalentOutsourcing)
	return router
}

func validTalentOutsourcingPreviewRequest(mode domain.TalentEngagementMode) dto.GenerateQuotationRequest {
	input := domain.TalentOutsourcingInput{
		EngagementMode: mode,
		DocumentNumber: "LANDA/TALENT/0626/001",
		ServiceName:    "Talent Outsourcing",
		QuotationDate:  time.Date(2026, time.June, 10, 0, 0, 0, 0, time.UTC),
		ScopeBlocks: []domain.TalentScopeBlockInput{{
			Title: "Pelaksanaan", Description: "Pelaksanaan layanan outsourcing.", Timeline: "Juni 2026",
		}},
		PaymentDueDays: 14,
		LimitationCodes: []domain.TalentLimitationCode{
			domain.TalentLimitOutOfPocket,
		},
	}
	if mode == domain.TalentModeManpowerSupply {
		input.TaxTreatment = domain.TalentTaxBPJSDPP12
		input.CostGroups = []domain.TalentCostGroupInput{{
			Category: domain.TalentCategoryPersonnel, Title: "Biaya Langsung Personil",
			Items: []domain.TalentCostItemInput{{
				Name: "Project Manager", Quantity: 1, QuantityUnit: "Orang", PeriodCount: 32,
				PeriodUnit: domain.TalentPeriodWorkday, UnitRate: 25_000_000,
			}},
		}}
		input.PaymentSchedule = []domain.TalentPaymentRowInput{{
			WorkPeriod: "Juni", Terms: "Termin 1", InvoiceDateLabel: "1 Juli", DueDateLabel: "15 Juli", Nominal: 44_400_000,
		}}
	} else {
		input.TaxTreatment = domain.TalentTaxExclusive
		input.PreparationItems = []domain.TalentPreparationItemInput{{Name: "Persiapan", Headcount: 1, QuotedLineTotal: 12_060_000}}
		input.SquadItems = []domain.TalentSquadItemInput{{Role: "Programmer", MonthlyRate: 100_000_000, Headcount: 1, PeriodMonths: 4}}
		input.PaymentSchedule = []domain.TalentPaymentRowInput{{
			WorkPeriod: "Juni-September", Terms: "Termin 1", InvoiceDateLabel: "1 Juli", DueDateLabel: "15 Juli", Nominal: 412_060_000,
		}}
	}
	return dto.GenerateQuotationRequest{
		CommercialScheme:  domain.CommercialSchemeTalentOutsourcing,
		TalentOutsourcing: &input,
	}
}

func performTalentOutsourcingPreview(t *testing.T, router *gin.Engine, payload dto.GenerateQuotationRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/preview/talent-outsourcing", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestPreviewTalentOutsourcingSuccess(t *testing.T) {
	router := talentOutsourcingPreviewRouter(uuid.New())
	tests := []struct {
		name       string
		mode       domain.TalentEngagementMode
		grandTotal int64
	}{
		{"BPJS manpower", domain.TalentModeManpowerSupply, 44_400_000},
		{"SYS managed squad", domain.TalentModeManagedSquad, 412_060_000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := performTalentOutsourcingPreview(t, router, validTalentOutsourcingPreviewRequest(tt.mode))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var body struct {
				Data dto.TalentOutsourcingPreviewResponse `json:"data"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tt.grandTotal, body.Data.Calculation.GrandTotal)
			assert.Equal(t, tt.mode, body.Data.ResolvedProfile.EngagementMode)
			assert.Equal(t, tt.mode == domain.TalentModeManagedSquad, body.Data.ResolvedProfile.ShowPreparation)
		})
	}
}

func TestPreviewTalentOutsourcingRejectsInvalidTopLevelContract(t *testing.T) {
	router := talentOutsourcingPreviewRouter(uuid.New())
	tests := []struct {
		name   string
		mutate func(*dto.GenerateQuotationRequest)
		status int
	}{
		{"wrong scheme", func(r *dto.GenerateQuotationRequest) { r.CommercialScheme = domain.CommercialSchemeCustomDevelopment }, http.StatusBadRequest},
		{"missing talent input", func(r *dto.GenerateQuotationRequest) { r.TalentOutsourcing = nil }, http.StatusBadRequest},
		{"also custom pointer", func(r *dto.GenerateQuotationRequest) { r.CustomDevelopment = &domain.CustomDevelopmentInput{} }, http.StatusBadRequest},
		{"also kiosk pointer", func(r *dto.GenerateQuotationRequest) { r.KioskQueueSystem = &domain.KioskQueueSystemInput{} }, http.StatusBadRequest},
		{"rate card", func(r *dto.GenerateQuotationRequest) { r.RateCardID = uuid.New() }, http.StatusBadRequest},
		{"invalid nested", func(r *dto.GenerateQuotationRequest) { r.TalentOutsourcing.PaymentSchedule[0].Nominal-- }, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := validTalentOutsourcingPreviewRequest(domain.TalentModeManpowerSupply)
			tt.mutate(&payload)
			w := performTalentOutsourcingPreview(t, router, payload)
			assert.Equal(t, tt.status, w.Code)
			assert.NotContains(t, w.Body.String(), "Talent Outsourcing", "invalid response must not echo client input")
		})
	}
}

func TestPreviewTalentOutsourcingRequiresCompanyContext(t *testing.T) {
	router := talentOutsourcingPreviewRouter(uuid.New())
	payload := validTalentOutsourcingPreviewRequest(domain.TalentModeManpowerSupply)
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/preview/talent-outsourcing", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Omit-Company", "true")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
