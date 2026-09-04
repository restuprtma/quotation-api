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
	Handler    *handler.Handler
	GenHandler *handler.QuotationGenerationHandler
	Service    *service.Service
}

// Initialize wires the quotation module. The AI provider is instantiated via
// the factory based on the active configuration.
func Initialize(db *pgxpool.Pool, cfg *config.Config) *Module {
	repo := repository.NewRepository(db)
	credentialRepo := repository.NewCredentialRepository(db)
	skillRepo := repository.NewSkillRepository(db)

	cipher, err := service.NewCredentialCipher(cfg.AI.CredentialEncryptionKey)
	if err != nil {
		panic("failed to initialize credential cipher: " + err.Error())
	}

	aiFactory := service.NewProviderFactory()

	rcRepo := repository.NewRateCardRepository(db)
	legalRepo := repository.NewLegalTemplateRepository(db)
	rcSvc := service.NewRateCardService(db, rcRepo)
	legalSvc := service.NewLegalTemplateService(legalRepo)
	skillSvc := service.NewSkillService(skillRepo)
	settingsSvc := service.NewSettingsService(db, skillRepo, rcRepo, legalRepo, skillSvc)

	svc := service.NewService(repo, credentialRepo, skillRepo, cipher, aiFactory, settingsSvc, rcSvc, legalSvc)
	hdlr := handler.NewHandler(svc)

	// AI-first generation stack (Fase 6): request-scoped credential
	// resolution stays inside DBCredentialProviderSource; nothing derived
	// from credentials is cached.
	genRepo := repository.NewGenerationRepository(db)
	providerSource := service.NewDBCredentialProviderSource(credentialRepo, aiFactory, cfg.AI.CredentialEncryptionKey)
	genSvc := service.NewQuotationGenerationService(genRepo, providerSource)

	return &Module{
		Handler:    hdlr,
		GenHandler: handler.NewQuotationGenerationHandler(genSvc, genRepo),
		Service:    svc,
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
		// AI-first detail (Fase 6 Session 5): menggantikan target stub yang
		// sebelumnya selalu "not implemented"; kontrak response bertambah,
		// tidak ada perilaku lama yang hilang.
		quotations.GET("/:id", m.GenHandler.GetByID)
		quotations.PUT("/:id", m.Handler.Update)

		quotations.POST("/generate", m.GenHandler.Generate)
		quotations.POST("/:id/regenerate", m.GenHandler.Regenerate)
		quotations.GET("/:id/generation-status", m.GenHandler.GetGenerationStatus)
		quotations.PUT("/:id/draft", m.GenHandler.SaveDraft)

		// V2 Revisions & Sharing
		quotations.POST("/:id/revisions", m.Handler.CreateRevision)
		quotations.GET("/:id/revisions", m.Handler.ListRevisions)
		quotations.GET("/:id/revisions/:revision_id", m.Handler.GetRevisionDetail)
		quotations.POST("/:id/share", m.Handler.ShareQuotation)
	}

	ai := router.Group("/ai")
	ai.Use(middleware.JWTAuth(), middleware.CompanyContext())
	{
		ai.GET("/settings", m.Handler.GetSettings)
		ai.PUT("/settings", m.Handler.UpdateSettings)
		ai.POST("/settings/test", m.Handler.TestSettingsConnection)

		ai.GET("/skills", m.Handler.ListSkills)
		ai.GET("/skills/:id", m.Handler.GetSkill)
		ai.PUT("/skills/:id", m.Handler.UpdateSkill)
		ai.POST("/skills/:id/reset", m.Handler.ResetSkillDefault)
	}

	rateCards := router.Group("/rate-cards")
	rateCards.Use(middleware.JWTAuth(), middleware.CompanyContext())
	{
		rateCards.GET("", m.Handler.ListRateCards)
		rateCards.GET("/:id", m.Handler.GetRateCard)
	}

	legalTemplates := router.Group("/legal-templates")
	legalTemplates.Use(middleware.JWTAuth(), middleware.CompanyContext())
	{
		legalTemplates.GET("", m.Handler.ListLegalTemplates)
		legalTemplates.GET("/active/:code", m.Handler.GetActiveLegalTemplate)
	}
}

// SetupPublicRoutes registers token-based quotation endpoints without auth.
func (m *Module) SetupPublicRoutes(router *gin.RouterGroup) {
	// Fase 1: All public endpoints that depend on legacy artifacts have been removed.
	// We will introduce the new allowlist-based endpoints in Fase 11.
}
