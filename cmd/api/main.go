package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"venturo-skeleton-go/internal/config"
	"venturo-skeleton-go/internal/database"
	"venturo-skeleton-go/internal/router"
	"venturo-skeleton-go/pkg/jwt"
	"venturo-skeleton-go/pkg/logger"

	"github.com/gin-gonic/gin"
)

func init() {
	// Force UTC timezone for the entire application
	// This ensures all time.Now() calls return UTC time
	time.Local = time.UTC
}

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize logger
	if err := logger.Initialize(cfg.Server.Env); err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	logger.Info("Starting Tuai API")

	// Validate JWT secret configuration
	if err := jwt.ValidateSecret(cfg.Server.Env); err != nil {
		logger.Fatal("JWT secret validation failed: " + err.Error())
	}
	logger.Info("JWT secret validation passed")

	// Initialize database
	db, err := database.New(cfg.Database.GetDSN())
	if err != nil {
		logger.Fatal("Failed to connect to database")
	}
	defer db.Close()

	logger.Info("Database connected successfully")

	// Initialize Gin router
	ginRouter := gin.New() // Use gin.New() instead of gin.Default() since we use custom middleware

	// Setup routes (all modules initialized inside router)
	managedWorker := router.Setup(ginRouter, db.Pool, cfg)

	processCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if err := managedWorker.Start(processCtx); err != nil {
		logger.Fatal("Failed to start meeting extraction worker", logger.Err(err))
	}

	// Start server
	serverAddr := ":" + cfg.Server.Port
	logger.Info("Starting server on " + serverAddr + " (Environment: " + cfg.Server.Env + ")")

	server := &http.Server{Addr: serverAddr, Handler: ginRouter}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-processCtx.Done():
		logger.Info("Shutdown signal received")
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server stopped", logger.Err(err))
		}
		stopSignals()
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP graceful shutdown failed", logger.Err(err))
	}
	managedWorker.Wait()
}
