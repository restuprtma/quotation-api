package handler

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
	jwtpkg "venturo-skeleton-go/pkg/jwt"
)

func cloudInfrastructurePreviewRouter(companyID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Omit-Company") == "" {
			c.Set(middleware.UserContextKey, &jwtpkg.Claims{CompanyID: companyID.String()})
		}
		c.Next()
	})
	h := NewQuotationGenerationHandler(nil, nil)
	router.POST("/quotation/v1/quotations/preview/cloud-infrastructure", h.PreviewCloudInfrastructure)
	return router
}

func validCloudInfrastructurePreviewRequest() dto.GenerateQuotationRequest {
	return dto.GenerateQuotationRequest{
		CommercialScheme: domain.CommercialSchemeCloudInfrastructure,
		CloudInfrastructure: &domain.CloudInfrastructureInput{
			DocumentNumber: "CTH/Authen-Cloud/0626/001",
			ServiceName:    "Venturo Cloud",
			QuotationDate:  time.Date(2026, time.June, 24, 0, 0, 0, 0, time.UTC),
			TaxTreatment:   domain.CloudTaxExclusive,
			PackageItems: []domain.CloudPackageItemInput{
				{Category: domain.CloudPackageVPS, Name: "Virtual Machine Small - Kube Master", VCPU: 4, RAMGB: 4, StorageGB: 32, UnitRate: 900_000},
				{Category: domain.CloudPackageVPS, Name: "Virtual Machine Small - Kube Master", VCPU: 4, RAMGB: 4, StorageGB: 32, UnitRate: 900_000},
			},
			ScopeRows: []domain.CloudScopeRowInput{{
				Sections: []domain.CloudScopeSectionInput{{Title: "Performance / Load Test", Activities: []string{"Load test"}}},
				Timeline: "2 Minggu",
			}},
			PaymentSchedule: []domain.CloudPaymentRowInput{{
				WorkPeriod: "Bulan 1", Terms: "Pelunasan Bulanan", InvoiceDateLabel: "30 Jun 2026", DueDateLabel: "14 Jul 2026", Component: domain.CloudPaymentMonthly, Nominal: 1_800_000,
			}},
			PaymentDueDays:  14,
			LimitationCodes: []domain.CloudLimitationCode{domain.CloudLimitOutOfPocket},
		},
	}
}

func performCloudInfrastructurePreview(t *testing.T, router *gin.Engine, payload dto.GenerateQuotationRequest) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/preview/cloud-infrastructure", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestCloudInfrastructurePreview(t *testing.T) {
	router := cloudInfrastructurePreviewRouter(uuid.New())

	t.Run("returns authoritative Authen preview", func(t *testing.T) {
		w := performCloudInfrastructurePreview(t, router, validCloudInfrastructurePreviewRequest())
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var body struct {
			Data dto.CloudInfrastructurePreviewResponse `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, int64(1_800_000), body.Data.Calculation.MonthlySubtotal)
		assert.Len(t, body.Data.ResolvedProfile.Sections, 4)
	})

	for _, tt := range []struct {
		name   string
		mutate func(*dto.GenerateQuotationRequest)
		status int
	}{
		{"rejects missing cloud input", func(r *dto.GenerateQuotationRequest) { r.CloudInfrastructure = nil }, http.StatusBadRequest},
		{"rejects cross scheme pointer", func(r *dto.GenerateQuotationRequest) { r.WhiteLabel = &domain.WhiteLabelInput{} }, http.StatusBadRequest},
		{"rejects rate card", func(r *dto.GenerateQuotationRequest) { r.RateCardID = uuid.New() }, http.StatusUnprocessableEntity},
		{"rejects legacy fields", func(r *dto.GenerateQuotationRequest) { r.BudgetMaximum = 1 }, http.StatusUnprocessableEntity},
		{"rejects payment mismatch", func(r *dto.GenerateQuotationRequest) { r.CloudInfrastructure.PaymentSchedule[0].Nominal-- }, http.StatusUnprocessableEntity},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := validCloudInfrastructurePreviewRequest()
			tt.mutate(&payload)
			w := performCloudInfrastructurePreview(t, router, payload)
			assert.Equal(t, tt.status, w.Code, w.Body.String())
		})
	}

	t.Run("rejects ref nominal during strict decode", func(t *testing.T) {
		body := []byte(`{"commercial_scheme":"cloud_infrastructure","cloud_infrastructure":{"quotation_date":"2026-06-24","payment_schedule":[{"nominal":"#REF!"}]}}`)
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/preview/cloud-infrastructure", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
