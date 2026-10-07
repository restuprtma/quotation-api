package router_test

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/config"
	"venturo-skeleton-go/internal/router"
)

func TestRouteAudit_CanonicalRoutesPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// Initialize with test config and nil DB (isolated from live Redis / PostgreSQL)
	cfg := &config.Config{}
	cfg.AI.CredentialEncryptionKey = "12345678901234567890123456789012"
	cfg.Redis.Host = "localhost"
	cfg.Redis.Port = "6379"

	router.SetupQuotationRoutes(engine, nil, cfg)
	router.SetupContractRoutes(engine, nil)

	// Collect registered routes
	registered := make(map[string]bool)
	for _, r := range engine.Routes() {
		registered[r.Method+" "+r.Path] = true
	}

	// 1. Audit active routes allowlist
	expectedRoutes := []struct {
		method string
		path   string
	}{
		{"GET", "/quotation/v1/ai/settings"},
		{"PUT", "/quotation/v1/ai/settings"},
		{"GET", "/quotation/v1/ai/skills"},
		{"POST", "/quotation/v1/quotations/generate"},
		{"PUT", "/quotation/v1/quotations/:id/draft"},
		{"POST", "/quotation/v1/quotations/:id/publish"},
		{"GET", "/quotation/v1/quotations/:id/revisions"},
		{"POST", "/quotation/v1/quotations/:id/revisions"},
		{"POST", "/quotation/v1/blueprints/generate"},
		{"PUT", "/quotation/v1/blueprints/:id/sections/:section/draft"},
		{"POST", "/quotation/v1/blueprints/:id/publish"},
		{"POST", "/quotation/v1/blueprints/:id/share-links"},
		{"GET", "/quotation/v1/blueprints/:id/share-links"},
		{"DELETE", "/quotation/v1/blueprints/:id/share-links/:link_id"},
		{"GET", "/p/q/:public_token"},
		{"POST", "/p/q/:public_token/sign"},
		{"GET", "/p/blueprint/:token"},
	}

	for _, exp := range expectedRoutes {
		key := exp.method + " " + exp.path
		assert.True(t, registered[key], "expected canonical route %s to be registered", key)
	}

	// 2. Audit forbidden legacy routes blocklist
	forbiddenPaths := []string{
		"/quotation/v1/ai/generate",
		"/p/b/:token",
		"/p/d/:token",
		"/quotation/v1/pricing",
	}

	for routeKey := range registered {
		for _, forbidden := range forbiddenPaths {
			assert.False(t, strings.Contains(routeKey, forbidden),
				"route %s must not match forbidden legacy pattern %s", routeKey, forbidden)
		}

		// Verify legacy :id/share endpoint is purged from quotation module
		assert.False(t, strings.HasSuffix(routeKey, "/quotations/:id/share"),
			"legacy route %s must not be registered", routeKey)
	}
}

func TestRouteAudit_ContractCanonicalRoutesPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	router.SetupContractRoutes(engine, nil)

	registered := make(map[string]bool)
	for _, r := range engine.Routes() {
		registered[r.Method+" "+r.Path] = true
	}

	expectedContractRoutes := []string{
		"POST /contract/v1/contracts",
		"GET /contract/v1/contracts",
		"GET /contract/v1/contracts/:id",
		"PUT /contract/v1/contracts/:id",
		"POST /contract/v1/contracts/:id/share",
		"GET /p/c/:token",
		"POST /p/c/:token/sign",
	}

	for _, route := range expectedContractRoutes {
		assert.True(t, registered[route], "expected contract route %s to be registered", route)
	}
}
