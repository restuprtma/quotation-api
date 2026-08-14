package quotation

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/config"
	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/handler"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

type Module struct {
	Handler *handler.Handler
	Service *service.Service
}

// Initialize wires the quotation module. The AI provider is instantiated via
// the factory based on the active configuration.
func Initialize(db *pgxpool.Pool, cfg *config.Config) *Module {
	repo := repository.NewRepository(db)
	aiProvider := service.NewAIProvider(cfg.AI)
	svc := service.NewService(repo, aiProvider)
	hdlr := handler.NewHandler(svc)

	return &Module{
		Handler: hdlr,
		Service: svc,
	}
}

// SetupRoutes registers the quotation endpoints under the caller-supplied
// /quotation/v1 group. Internal endpoints require JWT + a valid company
// context; the AI endpoint only requires authentication.
func (m *Module) SetupRoutes(router *gin.RouterGroup) {
	quotations := router.Group("/quotations")
	quotations.Use(middleware.JWTAuth(), middleware.CompanyContext())
	{
		quotations.GET("", m.Handler.List)
		quotations.POST("", m.Handler.Create)
		quotations.GET("/:id", m.Handler.GetByID)
		quotations.PUT("/:id", m.Handler.Update)

		// V2 Revisions & Sharing
		quotations.POST("/:id/revisions", m.Handler.CreateRevision)
		quotations.GET("/:id/revisions", m.Handler.ListRevisions)
		quotations.GET("/:id/revisions/:revision_id", m.Handler.GetRevisionDetail)
		quotations.POST("/:id/share", m.Handler.ShareQuotation)
		quotations.GET("/:id/artifacts", m.Handler.GetArtifacts)
	}

	ai := router.Group("/ai")
	ai.Use(middleware.JWTAuth())
	{
		ai.POST("/generate", m.Handler.GenerateAI)
	}
}

// SetupPublicRoutes registers token-based quotation endpoints without auth.
func (m *Module) SetupPublicRoutes(router *gin.RouterGroup) {
	router.GET("/q/:artifact_token", m.Handler.GetPublicQuotation)
	router.POST("/q/:artifact_token/sign", m.Handler.SignPublicQuotation)
	router.GET("/b/:artifact_token", m.Handler.GetPublicBlueprint)
	router.GET("/d/:artifact_token", m.Handler.GetPublicDetail)
}
