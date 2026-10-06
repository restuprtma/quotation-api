package quotation

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/config"
	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/handler"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	quotationWorker "venturo-skeleton-go/internal/modules/quotation/worker"
)

type Module struct {
	Handler                   *handler.Handler
	LifecycleHandler          *handler.LifecycleHandler
	GenHandler                *handler.QuotationGenerationHandler
	PromptHandler             *handler.PromptQuotationHandler
	MeetingExtractionHandler  *handler.MeetingExtractionHandler
	PromptRevisionHandler     *handler.PromptRevisionHandler
	BlueprintHandler          *handler.BlueprintGenerationHandler
	BlueprintShareLinkHandler *handler.BlueprintShareLinkHandler
	ClientOrganizationHandler *handler.ClientOrganizationHandler
	PublicHandler             *handler.PublicQuotationHandler
	Service                   *service.Service
	ExtractionExecutor        quotationWorker.Executor
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
	meetingExtractionRepo := repository.NewMeetingExtractionRepository(db)
	providerSource := service.NewDBCredentialProviderSource(credentialRepo, aiFactory, cfg.AI.CredentialEncryptionKey)
	startDecider := service.NewMeetingExtractionStartDecider(providerSource, string(cfg.MeetingExtraction.Mode), cfg.MeetingExtraction.ShadowSamplingPercent)
	meetingExtractionAPI := handler.NewRepositoryMeetingExtractionAPI(meetingExtractionRepo, genRepo, startDecider)
	var extractionExecutor quotationWorker.Executor
	if cfg.MeetingExtraction.Mode == config.MeetingExtractionModePipeline || cfg.MeetingExtraction.Mode == config.MeetingExtractionModeShadow {
		extractionStore, storeErr := service.NewPostgreSQLMeetingExtractionOrchestratorStore(meetingExtractionRepo, service.MeetingExtractionStoreOptions{DailyProviderCallsMax: 8, DailyQuotationCallsMax: 8})
		if storeErr != nil {
			panic("meeting extraction startup dependency missing: " + storeErr.Error())
		}
		p5, p5Err := service.NewCatalogBackedMeetingExtractionP5(genRepo, service.NewPromptServiceAdapterRegistry(nil, nil, nil, nil))
		if p5Err != nil {
			panic("meeting extraction startup dependency missing: " + p5Err.Error())
		}
		productionExecutor, executorErr := service.NewMeetingExtractionExecutor(service.MeetingExtractionExecutorOptions{
			Revisions: repository.NewRevisionRepository(db), Providers: providerSource, Catalog: service.RevisionRateCardSnapshotLoader{},
			Store: extractionStore, P5: p5, PipelineVersion: "meeting-extraction-v1", PromptVersion: "meeting-extraction-v1", SchemaVersion: "meeting-extraction-v1",
		})
		if executorErr != nil {
			panic("meeting extraction startup dependency missing: " + executorErr.Error())
		}
		extractionExecutor = productionExecutor
	}
	genSvc := service.NewQuotationGenerationService(genRepo, providerSource)
	previewCalculator := service.NewDedicatedSquadCalculator(rcRepo)
	promptCatalog := service.NewPromptCatalogService(rcRepo)
	promptExtractor := service.NewMeetingExtractionService(providerSource)
	promptDraftSvc := service.NewPromptDraftService(
		genRepo,
		promptCatalog,
		promptExtractor,
		previewCalculator,
		service.NewDedicatedSquadHTMLRenderer(),
		service.NewHTMLSanitizer(),
	)
	promptRevisionSvc := service.NewPromptRevisionService(genRepo, service.NewHTMLSanitizer(), nil)
	promptNarrativeSvc := service.NewPromptNarrativeRevisionService(genRepo, service.NewHTMLSanitizer())
	promptIntentSvc := service.NewPromptRevisionIntentService(providerSource)
	promptSectionRevisionSvc := service.NewPromptSectionRevisionService(genRepo, promptIntentSvc, promptNarrativeSvc, promptRevisionSvc)

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
	bpDraftSvc := service.NewBlueprintDraftService(bpGenRepo, bpValidator, bpCrossValidator, providerSource)
	bpHandler := handler.NewBlueprintGenerationHandler(bpGenSvc, bpReadSvc, bpPipeline, bpDraftSvc)
	lifecycleHandler := handler.NewLifecycleHandler(svc, bpDraftSvc, bpReadSvc)

	clientOrgRepo := repository.NewClientOrganizationRepository(db)
	clientOrgSvc := service.NewClientOrganizationService(clientOrgRepo)
	clientOrgHandler := handler.NewClientOrganizationHandler(clientOrgSvc)

	shareLinkRepo := repository.NewShareLinkRepository(db)
	shareLinkSvc := service.NewBlueprintShareLinkService(shareLinkRepo, bpReadRepo)
	shareLinkHandler := handler.NewBlueprintShareLinkHandler(shareLinkSvc, "")

	return &Module{
		Handler:                   hdlr,
		LifecycleHandler:          lifecycleHandler,
		GenHandler:                handler.NewQuotationGenerationHandler(genSvc, genRepo, previewCalculator),
		PromptHandler:             handler.NewPromptQuotationHandler(promptDraftSvc),
		MeetingExtractionHandler:  handler.NewMeetingExtractionHandler(meetingExtractionAPI),
		PromptRevisionHandler:     handler.NewPromptRevisionHandler(promptRevisionSvc, promptSectionRevisionSvc),
		BlueprintHandler:          bpHandler,
		BlueprintShareLinkHandler: shareLinkHandler,
		ClientOrganizationHandler: clientOrgHandler,
		PublicHandler:             handler.NewPublicQuotationHandler(svc),
		Service:                   svc,
		ExtractionExecutor:        extractionExecutor,
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
		// Register the additive prompt-first route before the /:id wildcard.
		quotations.POST("/from-meeting", m.PromptHandler.CreateFromMeeting)
		quotations.POST("/:id/meeting-extraction", m.MeetingExtractionHandler.Start)
		quotations.GET("/:id/meeting-extraction/:run_id", m.MeetingExtractionHandler.Status)
		quotations.POST("/:id/meeting-extraction/:run_id/retry", m.MeetingExtractionHandler.Retry)
		quotations.POST("/:id/process-meeting", m.PromptHandler.ProcessMeeting)
		quotations.PATCH("/:id/prompt-state", m.PromptRevisionHandler.PatchPromptState)
		quotations.POST("/:id/revise-section", m.PromptRevisionHandler.ReviseSection)
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

		// Lifecycle & Revisions
		if m.LifecycleHandler != nil {
			quotations.POST("/:id/publish", m.LifecycleHandler.PublishQuotation)
			quotations.POST("/:id/revisions", m.LifecycleHandler.CreateQuotationRevision)
			quotations.GET("/:id/revisions", m.LifecycleHandler.ListQuotationRevisions)
			quotations.GET("/:id/revisions/:revision_id", m.LifecycleHandler.GetQuotationRevisionDetail)
		} else if m.Handler != nil {
			quotations.POST("/:id/revisions", m.Handler.CreateRevision)
			quotations.GET("/:id/revisions", m.Handler.ListRevisions)
			quotations.GET("/:id/revisions/:revision_id", m.Handler.GetRevisionDetail)
		}
		if m.Handler != nil {
			quotations.POST("/:id/share", m.Handler.ShareQuotation)
		}
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
		if m.BlueprintHandler != nil {
			blueprints.POST("/generate", m.BlueprintHandler.Generate)
			blueprints.GET("/:id/generation-status", m.BlueprintHandler.GetStatus)
			blueprints.GET("/:id", m.BlueprintHandler.GetDetail)
			blueprints.POST("/:id/continue", m.BlueprintHandler.Continue)
			blueprints.POST("/:id/sections/:section/retry", m.BlueprintHandler.RetrySection)
			blueprints.PUT("/:id/sections/:section/manual", m.BlueprintHandler.ManualCompleteSection)
			blueprints.PUT("/:id/sections/:section/draft", m.BlueprintHandler.SaveDraftSection)
			blueprints.GET("", m.BlueprintHandler.List)
		}

		if m.LifecycleHandler != nil {
			blueprints.POST("/:id/publish", m.LifecycleHandler.PublishBlueprint)
			blueprints.POST("/:id/revisions", m.LifecycleHandler.CreateBlueprintRevision)
			blueprints.GET("/:id/revisions", m.LifecycleHandler.ListBlueprintRevisions)
			blueprints.GET("/:id/revisions/:revision_id", m.LifecycleHandler.GetBlueprintRevisionDetail)
			blueprints.DELETE("/:id/draft", m.LifecycleHandler.DeleteBlueprintDraft)
		}

		if m.BlueprintShareLinkHandler != nil {
			blueprints.POST("/:id/share-links", m.BlueprintShareLinkHandler.Create)
			blueprints.GET("/:id/share-links", m.BlueprintShareLinkHandler.List)
			blueprints.DELETE("/:id/share-links/:link_id", m.BlueprintShareLinkHandler.Revoke)
		}
	}

	clientOrganizations := router.Group("/client-organizations")
	clientOrganizations.Use(middleware.JWTAuth(), middleware.CompanyContext())
	{
		clientOrganizations.GET("", m.ClientOrganizationHandler.List)
		clientOrganizations.POST("", m.ClientOrganizationHandler.Create)
	}
}

// SetupPublicRoutes registers token-based quotation endpoints without auth.
func (m *Module) SetupPublicRoutes(router *gin.RouterGroup) {
	if m.PublicHandler == nil {
		return
	}
	q := router.Group("/q")
	{
		q.GET("/:public_token", m.PublicHandler.GetPublicQuotation)
		q.POST("/:public_token/sign", m.PublicHandler.SignPublicQuotation)
	}
}
