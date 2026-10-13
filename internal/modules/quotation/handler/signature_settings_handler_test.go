package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/handler"
)

type mockSignatureSettingsService struct {
	getFunc    func(ctx context.Context, companyID string) (*dto.SignatureSettingsResponse, error)
	updateFunc func(ctx context.Context, companyID string, req dto.UpdateSignatureSettingsRequest) (*dto.SignatureSettingsResponse, error)
}

func (m *mockSignatureSettingsService) GetSignatureSettings(ctx context.Context, companyID string) (*dto.SignatureSettingsResponse, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, companyID)
	}
	return nil, nil
}

func (m *mockSignatureSettingsService) UpdateSignatureSettings(ctx context.Context, companyID string, req dto.UpdateSignatureSettingsRequest) (*dto.SignatureSettingsResponse, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, companyID, req)
	}
	return nil, nil
}

func TestSignatureSettingsHandler_Get_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("company_id", "11111111-1111-1111-1111-111111111111")
		c.Next()
	})

	h := handler.NewSignatureSettingsHandler(&mockSignatureSettingsService{
		getFunc: func(ctx context.Context, companyID string) (*dto.SignatureSettingsResponse, error) {
			assert.Equal(t, "11111111-1111-1111-1111-111111111111", companyID)
			return &dto.SignatureSettingsResponse{
				DirectorName:  "Yulianto Frandi",
				DirectorTitle: "Direksi Venturo Pro",
				IsConfigured:  true,
			}, nil
		},
	})

	r.GET("/quotation/v1/settings/signature", h.GetSignatureSettings)

	req, _ := http.NewRequest(http.MethodGet, "/quotation/v1/settings/signature", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var res map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &res)
	assert.NoError(t, err)
	assert.Equal(t, "Signature settings retrieved", res["message"])
	data := res["data"].(map[string]interface{})
	assert.Equal(t, "Yulianto Frandi", data["director_name"])
	assert.Equal(t, true, data["is_configured"])
}

func TestSignatureSettingsHandler_Get_MissingCompanyContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	h := handler.NewSignatureSettingsHandler(&mockSignatureSettingsService{})
	r.GET("/quotation/v1/settings/signature", h.GetSignatureSettings)

	req, _ := http.NewRequest(http.MethodGet, "/quotation/v1/settings/signature", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSignatureSettingsHandler_Get_ServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("company_id", "11111111-1111-1111-1111-111111111111")
		c.Next()
	})

	h := handler.NewSignatureSettingsHandler(&mockSignatureSettingsService{
		getFunc: func(ctx context.Context, companyID string) (*dto.SignatureSettingsResponse, error) {
			return nil, errors.New("db connection failure")
		},
	})
	r.GET("/quotation/v1/settings/signature", h.GetSignatureSettings)

	req, _ := http.NewRequest(http.MethodGet, "/quotation/v1/settings/signature", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSignatureSettingsHandler_Update_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("company_id", "11111111-1111-1111-1111-111111111111")
		c.Next()
	})

	h := handler.NewSignatureSettingsHandler(&mockSignatureSettingsService{
		updateFunc: func(ctx context.Context, companyID string, req dto.UpdateSignatureSettingsRequest) (*dto.SignatureSettingsResponse, error) {
			assert.Equal(t, "11111111-1111-1111-1111-111111111111", companyID)
			assert.Equal(t, "Yulianto Frandi", req.DirectorName)
			return &dto.SignatureSettingsResponse{
				DirectorName:          req.DirectorName,
				DirectorTitle:         req.DirectorTitle,
				DirectorSignatureData: req.DirectorSignatureData,
				CompanyStampData:      req.CompanyStampData,
				IsConfigured:          true,
			}, nil
		},
	})
	r.PUT("/quotation/v1/settings/signature", h.UpdateSignatureSettings)

	payload := dto.UpdateSignatureSettingsRequest{
		DirectorName:          "Yulianto Frandi",
		DirectorTitle:         "Direksi Venturo Pro",
		DirectorSignatureData: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
		CompanyStampData:      "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPut, "/quotation/v1/settings/signature", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var res map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &res)
	assert.NoError(t, err)
	assert.Equal(t, "Signature settings updated", res["message"])
	data := res["data"].(map[string]interface{})
	assert.Equal(t, "Yulianto Frandi", data["director_name"])
	assert.Equal(t, true, data["is_configured"])
}

func TestSignatureSettingsHandler_Update_MissingCompanyContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	h := handler.NewSignatureSettingsHandler(&mockSignatureSettingsService{})
	r.PUT("/quotation/v1/settings/signature", h.UpdateSignatureSettings)

	payload := dto.UpdateSignatureSettingsRequest{
		DirectorName: "Yulianto Frandi",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPut, "/quotation/v1/settings/signature", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSignatureSettingsHandler_Update_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("company_id", "11111111-1111-1111-1111-111111111111")
		c.Next()
	})

	h := handler.NewSignatureSettingsHandler(&mockSignatureSettingsService{})
	r.PUT("/quotation/v1/settings/signature", h.UpdateSignatureSettings)

	req, _ := http.NewRequest(http.MethodPut, "/quotation/v1/settings/signature", bytes.NewReader([]byte("{invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSignatureSettingsHandler_Update_ServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("company_id", "11111111-1111-1111-1111-111111111111")
		c.Next()
	})

	h := handler.NewSignatureSettingsHandler(&mockSignatureSettingsService{
		updateFunc: func(ctx context.Context, companyID string, req dto.UpdateSignatureSettingsRequest) (*dto.SignatureSettingsResponse, error) {
			return nil, errors.New("image must be a valid PNG data URL")
		},
	})
	r.PUT("/quotation/v1/settings/signature", h.UpdateSignatureSettings)

	payload := dto.UpdateSignatureSettingsRequest{
		DirectorSignatureData: "invalid-data",
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPut, "/quotation/v1/settings/signature", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
