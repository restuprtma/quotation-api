package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
	jwtpkg "venturo-skeleton-go/pkg/jwt"
)

type clientOrgServiceStub struct {
	createdOrg  *dto.ClientOrganizationResponse
	createErr   error
	listResp    *dto.ClientOrganizationListResponse
	listErr     error
	calledComp  uuid.UUID
	calledUser  *uuid.UUID
	calledReq   dto.CreateClientOrganizationRequest
	createCalls int
	listCalls   int
}

func (s *clientOrgServiceStub) Create(_ context.Context, companyID uuid.UUID, createdBy *uuid.UUID, req dto.CreateClientOrganizationRequest) (*dto.ClientOrganizationResponse, error) {
	s.createCalls++
	s.calledComp = companyID
	s.calledUser = createdBy
	s.calledReq = req
	if s.createErr != nil {
		return nil, s.createErr
	}
	if s.createdOrg != nil {
		return s.createdOrg, nil
	}
	return &dto.ClientOrganizationResponse{
		ID:             uuid.New(),
		CompanyID:      companyID,
		Name:           req.Name,
		NormalizedName: strings.ToLower(req.Name),
		CreatedBy:      createdBy,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}, nil
}

func (s *clientOrgServiceStub) List(_ context.Context, companyID uuid.UUID) (*dto.ClientOrganizationListResponse, error) {
	s.listCalls++
	s.calledComp = companyID
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResp != nil {
		return s.listResp, nil
	}
	return &dto.ClientOrganizationListResponse{Items: []dto.ClientOrganizationResponse{}}, nil
}

func setTestClaims(c *gin.Context, companyID, userID string) {
	c.Set("user", &jwtpkg.Claims{
		CompanyID: companyID,
		UserID:    userID,
	})
}

func TestClientOrganizationHandler_Create(t *testing.T) {
	gin.SetMode(gin.TestMode)
	companyID := uuid.New()
	userID := uuid.New()

	t.Run("Missing company context returns 403 Forbidden", func(t *testing.T) {
		stub := &clientOrgServiceStub{}
		h := NewClientOrganizationHandler(stub)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/quotation/v1/client-organizations", strings.NewReader(`{"name":"Acme Corp"}`))
		c.Request.Header.Set("Content-Type", "application/json")

		h.Create(c)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Equal(t, 0, stub.createCalls)
	})

	t.Run("Missing name returns 400 Bad Request", func(t *testing.T) {
		stub := &clientOrgServiceStub{}
		h := NewClientOrganizationHandler(stub)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/quotation/v1/client-organizations", strings.NewReader(`{"name":""}`))
		c.Request.Header.Set("Content-Type", "application/json")
		setTestClaims(c, companyID.String(), userID.String())

		h.Create(c)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, 0, stub.createCalls)
	})

	t.Run("Duplicate name returns 409 Conflict", func(t *testing.T) {
		stub := &clientOrgServiceStub{
			createErr: service.ErrClientOrganizationDuplicate,
		}
		h := NewClientOrganizationHandler(stub)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/quotation/v1/client-organizations", strings.NewReader(`{"name":"Acme Corp"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		setTestClaims(c, companyID.String(), userID.String())

		h.Create(c)

		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.Equal(t, 1, stub.createCalls)
		assert.Contains(t, rec.Body.String(), "client_organization_exists")
	})

	t.Run("Invalid name returns 400 Bad Request", func(t *testing.T) {
		stub := &clientOrgServiceStub{
			createErr: service.ErrInvalidClientOrgName,
		}
		h := NewClientOrganizationHandler(stub)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/quotation/v1/client-organizations", strings.NewReader(`{"name":"test"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		setTestClaims(c, companyID.String(), userID.String())

		h.Create(c)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid_name")
	})

	t.Run("Valid request returns 201 Created", func(t *testing.T) {
		stub := &clientOrgServiceStub{}
		h := NewClientOrganizationHandler(stub)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/quotation/v1/client-organizations", strings.NewReader(`{"name":"Acme Corp"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		setTestClaims(c, companyID.String(), userID.String())

		h.Create(c)

		assert.Equal(t, http.StatusCreated, rec.Code)
		assert.Equal(t, 1, stub.createCalls)
		assert.Equal(t, companyID, stub.calledComp)
		assert.Equal(t, &userID, stub.calledUser)
		assert.Equal(t, "Acme Corp", stub.calledReq.Name)
	})
}

func TestClientOrganizationHandler_List(t *testing.T) {
	gin.SetMode(gin.TestMode)
	companyID := uuid.New()
	userID := uuid.New()

	t.Run("Missing company context returns 403 Forbidden", func(t *testing.T) {
		stub := &clientOrgServiceStub{}
		h := NewClientOrganizationHandler(stub)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/quotation/v1/client-organizations", nil)

		h.List(c)

		assert.Equal(t, http.StatusForbidden, rec.Code)
		assert.Equal(t, 0, stub.listCalls)
	})

	t.Run("Valid request returns 200 OK with items", func(t *testing.T) {
		stub := &clientOrgServiceStub{
			listResp: &dto.ClientOrganizationListResponse{
				Items: []dto.ClientOrganizationResponse{
					{
						ID:             uuid.New(),
						CompanyID:      companyID,
						Name:           "Acme Corp",
						NormalizedName: "acme corp",
						CreatedAt:      time.Now(),
						UpdatedAt:      time.Now(),
					},
				},
			},
		}
		h := NewClientOrganizationHandler(stub)

		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/quotation/v1/client-organizations", nil)
		setTestClaims(c, companyID.String(), userID.String())

		h.List(c)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, 1, stub.listCalls)
		assert.Equal(t, companyID, stub.calledComp)
		assert.Contains(t, rec.Body.String(), "Acme Corp")
	})
}
