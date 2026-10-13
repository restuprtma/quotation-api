package handler_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestPublicQuotationHandler_Sign(t *testing.T) {
	token := uuid.New().String()
	validPngBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00}
	validDataUrl := "data:image/png;base64," + base64.StdEncoding.EncodeToString(validPngBytes)

	t.Run("successful first sign returns 200", func(t *testing.T) {
		signer := "Klien Setia"
		now := time.Now().UTC()
		mockSvc := &mockPublicQuotationService{
			signPublicQuotationFunc: func(ctx context.Context, publicToken string, req dto.SignPublicQuotationRequest, ip, userAgent string) (*dto.PublicQuotationResponse, error) {
				return &dto.PublicQuotationResponse{
					QuotationNumber: "Q-001",
					RevisionNumber:  1,
					Signed:          true,
					SignerName:      &signer,
					SignedAt:        &now,
				}, nil
			},
		}
		r := setupPublicTestRouter(mockSvc)
		w := httptest.NewRecorder()

		body, _ := json.Marshal(dto.SignPublicQuotationRequest{
			SignerName:    "Klien Setia",
			SignerEmail:   "klien@example.com",
			SignatureData: validDataUrl,
			Consent:       true,
		})
		req, _ := http.NewRequest(http.MethodPost, "/p/q/"+token+"/sign", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("repeated sign returns 409 conflict", func(t *testing.T) {
		mockSvc := &mockPublicQuotationService{
			signPublicQuotationFunc: func(ctx context.Context, publicToken string, req dto.SignPublicQuotationRequest, ip, userAgent string) (*dto.PublicQuotationResponse, error) {
				return nil, service.ErrQuotationAlreadySigned
			},
		}
		r := setupPublicTestRouter(mockSvc)
		w := httptest.NewRecorder()

		body, _ := json.Marshal(dto.SignPublicQuotationRequest{
			SignerName:    "Klien Setia",
			SignerEmail:   "klien@example.com",
			SignatureData: validDataUrl,
			Consent:       true,
		})
		req, _ := http.NewRequest(http.MethodPost, "/p/q/"+token+"/sign", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("streamlined sign without signer name and email returns 200", func(t *testing.T) {
		signer := "PT Client Corp"
		now := time.Now().UTC()
		var receivedReq dto.SignPublicQuotationRequest
		mockSvc := &mockPublicQuotationService{
			signPublicQuotationFunc: func(ctx context.Context, publicToken string, req dto.SignPublicQuotationRequest, ip, userAgent string) (*dto.PublicQuotationResponse, error) {
				receivedReq = req
				return &dto.PublicQuotationResponse{
					QuotationNumber: "Q-001",
					RevisionNumber:  1,
					Signed:          true,
					SignerName:      &signer,
					SignedAt:        &now,
				}, nil
			},
		}
		r := setupPublicTestRouter(mockSvc)
		w := httptest.NewRecorder()

		body, _ := json.Marshal(dto.SignPublicQuotationRequest{
			SignatureData: validDataUrl,
			Consent:       true,
		})
		req, _ := http.NewRequest(http.MethodPost, "/p/q/"+token+"/sign", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, receivedReq.SignerName)
		assert.Empty(t, receivedReq.SignerEmail)
		assert.Equal(t, validDataUrl, receivedReq.SignatureData)
		assert.True(t, receivedReq.Consent)
	})
}
