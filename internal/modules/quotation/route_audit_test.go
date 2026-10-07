package quotation_test

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation"
	"venturo-skeleton-go/internal/modules/quotation/handler"
)

func TestRouteAudit_CanonicalRoutesPresentAndLegacyPurged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Initialize module with handlers
	mod := &quotation.Module{
		Handler:                   &handler.Handler{},
		LifecycleHandler:          &handler.LifecycleHandler{},
		GenHandler:                &handler.QuotationGenerationHandler{},
		PromptHandler:             &handler.PromptQuotationHandler{},
		MeetingExtractionHandler:  &handler.MeetingExtractionHandler{},
		PromptRevisionHandler:     &handler.PromptRevisionHandler{},
		BlueprintHandler:          &handler.BlueprintGenerationHandler{},
		BlueprintShareLinkHandler: &handler.BlueprintShareLinkHandler{},
		ClientOrganizationHandler: &handler.ClientOrganizationHandler{},
		PublicHandler:             &handler.PublicQuotationHandler{},
		PublicBlueprintHandler:    &handler.PublicBlueprintHandler{},
	}

	v1 := router.Group("/quotation/v1")
	mod.SetupRoutes(v1)

	p := router.Group("/p")
	mod.SetupPublicRoutes(p)

	// Collect registered routes
	registered := make(map[string]bool)
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		registered[key] = true
	}

	// 1. Verify Expected Canonical Routes
	expectedRoutes := []string{
		"GET /quotation/v1/ai/settings",
		"PUT /quotation/v1/ai/settings",
		"GET /quotation/v1/ai/skills",
		"POST /quotation/v1/quotations/generate",
		"PUT /quotation/v1/quotations/:id/draft",
		"POST /quotation/v1/quotations/:id/publish",
		"GET /quotation/v1/quotations/:id/revisions",
		"POST /quotation/v1/quotations/:id/revisions",
		"POST /quotation/v1/blueprints/generate",
		"POST /quotation/v1/blueprints/:id/publish",
		"GET /quotation/v1/blueprints/:id/revisions",
		"POST /quotation/v1/blueprints/:id/share-links",
		"GET /quotation/v1/blueprints/:id/share-links",
		"DELETE /quotation/v1/blueprints/:id/share-links/:link_id",
		"GET /p/q/:public_token",
		"POST /p/q/:public_token/sign",
		"GET /p/blueprint/:token",
	}

	for _, expected := range expectedRoutes {
		assert.True(t, registered[expected], "expected canonical route %s to be registered", expected)
	}

	// 2. Verify Legacy Forbidden Routes are Completely Absent
	forbiddenPrefixes := []string{
		"/quotation/v1/ai/generate",
		"/p/b/",
		"/p/b/:",
		"/p/d/",
		"/p/d/:",
		"/quotation/v1/pricing",
	}

	for routeKey := range registered {
		// Verify no forbidden legacy prefixes
		for _, forbidden := range forbiddenPrefixes {
			assert.False(t, strings.Contains(routeKey, forbidden),
				"route %s must not match forbidden legacy pattern %s", routeKey, forbidden)
		}

		// Verify legacy :id/share endpoint is purged
		assert.False(t, strings.HasSuffix(routeKey, "/quotations/:id/share"),
			"legacy route %s must not be registered", routeKey)
	}
}
