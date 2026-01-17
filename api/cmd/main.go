package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/hilthontt/visper/api/dependency"
	"github.com/hilthontt/visper/api/infrastructure/config"
	"go.uber.org/zap"
)

func main() {
	cfg := config.GetConfig()
	err := sentry.Init(sentry.ClientOptions{
		Dsn:            cfg.Sentry.Dsn,
		Debug:          cfg.Sentry.Debug,
		SendDefaultPII: cfg.Sentry.SendDefaultPII,
	})
	if err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}
	defer sentry.Flush(2 * time.Second)

	container, err := dependency.NewContainer()
	if err != nil {
		log.Fatal(fmt.Errorf("failed to initialize dependencies: %w", err))
	}
	defer container.Shutdown()

	router := container.SetupRouter()

	srv := &http.Server{
		Addr:           fmt.Sprintf(":%s", container.Config.Server.ExternalPort),
		Handler:        router,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	go func() {
		container.Logger.Info("Server starting",
			zap.String("port", container.Config.Server.ExternalPort),
			zap.String("mode", container.Config.Server.RunMode),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			container.Logger.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	container.Logger.Info("Server started successfully",
		zap.String("port", container.Config.Server.ExternalPort),
		zap.String("domain", container.Config.Server.Domain),
		zap.String("metrics_url", fmt.Sprintf("http://%s:%s/observability/metrics", container.Config.Server.Domain, container.Config.Server.ExternalPort)),
		zap.String("pprof_url", fmt.Sprintf("http://%s:%s/observability/debug/pprof/", container.Config.Server.Domain, container.Config.Server.ExternalPort)),
	)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	container.Logger.Info("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		container.Logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	container.Logger.Info("Server exited successfully")
}
