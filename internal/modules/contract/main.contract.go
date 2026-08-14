package contract

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/contract/handler"
	"venturo-skeleton-go/internal/modules/contract/repository"
	"venturo-skeleton-go/internal/modules/contract/service"
	quotation_repo "venturo-skeleton-go/internal/modules/quotation/repository"
)

type Module struct {
	Handler *handler.ContractHandler
}

func Initialize(db *pgxpool.Pool) *Module {
	contractRepo := repository.NewContractRepository(db)
	quotationRepo := quotation_repo.NewRepository(db)
	contractSvc := service.NewContractService(contractRepo, quotationRepo)
	contractHandler := handler.NewContractHandler(contractSvc)

	return &Module{
		Handler: contractHandler,
	}
}

func (m *Module) SetupRoutes(router *gin.Engine) {
	// Internal routes
	internal := router.Group("/contract/v1")
	internal.Use(middleware.JWTAuth())
	internal.Use(middleware.CompanyContext())
	{
		internal.POST("/contracts", m.Handler.Create)
		internal.GET("/contracts", m.Handler.List)
		internal.GET("/contracts/:id", m.Handler.Get)
		internal.PUT("/contracts/:id", m.Handler.Update)
		internal.POST("/contracts/:id/share", m.Handler.Share)
	}

	// Public routes
	public := router.Group("/p")
	{
		public.GET("/c/:token", m.Handler.GetPublic)
		public.POST("/c/:token/sign", m.Handler.SignPublic)
	}
}
