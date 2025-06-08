package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ippclub/dora-osg/internal/config"
	"github.com/ippclub/dora-osg/internal/handler"
	"github.com/ippclub/dora-osg/internal/logger"
	"github.com/ippclub/dora-osg/internal/service"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.InitLogger(cfg)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync()

	// Initialize sync service
	syncService, err := service.NewSyncService(cfg, log)
	if err != nil {
		log.Fatal("failed to create sync service", zap.Error(err))
	}

	// Initialize API handler
	api, err := handler.NewAPI(cfg, log, syncService)
	if err != nil {
		log.Fatal("failed to create API handler", zap.Error(err))
	}
	defer api.Close()

	// Create router
	r := chi.NewRouter()
	api.RegisterRoutes(r)

	// Create server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	// Start server in a goroutine
	go func() {
		log.Info("starting server", zap.Int("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("failed to start server", zap.Error(err))
		}
	}()

	// Start periodic sync
	go func() {
		ticker := time.NewTicker(cfg.Sync.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := syncService.SyncAll(); err != nil {
					log.Error("periodic sync failed", zap.Error(err))
				} else {
					log.Info("periodic sync completed successfully")
				}
			}
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Graceful shutdown
	log.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("server forced to shutdown", zap.Error(err))
	}

	log.Info("server exited properly")
}