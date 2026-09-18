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
	Handler          *handler.Handler
	GenHandler       *handler.QuotationGenerationHandler
	BlueprintHandler *handler.BlueprintGenerationHandler
	Service          *service.Service
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
	previewCalculator := service.NewDedicatedSquadCalculator(rcRepo)

	// Blueprint pipeline stack (Fase 8)
	bpGenRepo := repository.NewBlueprintGenerationRepository(db)
	bpReadRepo := repository.NewBlueprintReadRepository(db)
	bpInputRepo := repository.NewBlueprintGenerationInputRepository(db)
	bpContextBuilder := service.NewBlueprintSourceContextBuilder()
	bpGenSvc := service.NewBlueprintGenerationService(bpInputRepo, skillRepo, bpContextBuilder, bpGenRepo)
	bpReadSvc := service.NewBlueprintReadService(bpGenRepo, bpReadRepo)
	bpPrompts := service.NewBlueprintPromptBuilder()
	bpValidator := service.NewBlueprintSectionValidator()
	bpCrossValidator := service.NewProductionBlueprintCrossValidator()
	bpPipeline := service.NewBlueprintPipeline(bpGenRepo, providerSource, bpPrompts, bpValidator, bpCrossValidator)
	bpHandler := handler.NewBlueprintGenerationHandler(bpGenSvc, bpReadSvc, bpPipeline)

	return &Module{
		Handler:          hdlr,
		GenHandler:       handler.NewQuotationGenerationHandler(genSvc, genRepo, previewCalculator),
		BlueprintHandler: bpHandler,
		Service:          svc,
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
		quotations.POST("/preview/subscription-saas", m.GenHandler.PreviewSubscriptionSaaS)
		quotations.POST("/preview/white-label", m.GenHandler.PreviewWhiteLabel)
		quotations.POST("/preview/standalone-maintenance", m.GenHandler.PreviewStandaloneMaintenance)
		quotations.POST("/preview/custom-development", m.GenHandler.PreviewCustomDevelopment)
		quotations.POST("/preview/kiosk-queue-system", m.GenHandler.PreviewKioskQueueSystem)
		quotations.POST("/preview/talent-outsourcing", m.GenHandler.PreviewTalentOutsourcing)
		quotations.POST("/preview/cloud-infrastructure", m.GenHandler.PreviewCloudInfrastructure)
		// AI-first detail (Fase 6 Session 5): menggantikan target stub yang
		// sebelumnya selalu "not implemented"; kontrak response bertambah,
		// tidak ada perilaku lama yang hilang.
		quotations.GET("/:id", m.GenHandler.GetByID)
		quotations.PUT("/:id", m.Handler.Update)

		quotations.POST("/generate", m.GenHandler.Generate)
		quotations.POST("/preview", m.GenHandler.Preview)
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
		rateCards.GET("/:id/details", m.Handler.GetRateCardDetail)
		rateCards.POST("", m.Handler.CreateRateCard)
		rateCards.DELETE("/:id", m.Handler.DeleteRateCard)
	}

	legalTemplates := router.Group("/legal-templates")
	legalTemplates.Use(middleware.JWTAuth(), middleware.CompanyContext())
	{
		legalTemplates.GET("", m.Handler.ListLegalTemplates)
		legalTemplates.GET("/active/:code", m.Handler.GetActiveLegalTemplate)
	}

	blueprints := router.Group("/blueprints")
	blueprints.Use(middleware.JWTAuth(), middleware.CompanyContext())
	{
		blueprints.POST("/generate", m.BlueprintHandler.Generate)
		blueprints.GET("/:id/generation-status", m.BlueprintHandler.GetStatus)
		blueprints.GET("/:id", m.BlueprintHandler.GetDetail)
		blueprints.POST("/:id/continue", m.BlueprintHandler.Continue)
		blueprints.POST("/:id/sections/:section/retry", m.BlueprintHandler.RetrySection)
		blueprints.PUT("/:id/sections/:section/manual", m.BlueprintHandler.ManualCompleteSection)
		blueprints.GET("", m.BlueprintHandler.List)
	}
}

// SetupPublicRoutes registers token-based quotation endpoints without auth.
func (m *Module) SetupPublicRoutes(router *gin.RouterGroup) {
	// Fase 1: All public endpoints that depend on legacy artifacts have been removed.
	// We will introduce the new allowlist-based endpoints in Fase 11.
}
