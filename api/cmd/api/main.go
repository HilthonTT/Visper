package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/hilthontt/visper/api/dependency"
	"go.uber.org/zap"
)

const shutdownTimeout = 10 * time.Second

func main() {
	// The application context lives for the whole process; it is cancelled on
	// SIGINT/SIGTERM. The shutdown deadline is only created once shutdown starts.
	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	container, err := dependency.NewContainer(appCtx)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to initialize dependencies: %w", err))
	}
	defer func() {
		if err := container.Shutdown(); err != nil {
			container.Logger.Error("Shutdown failed", zap.Error(err))
		}
	}()

	cfg := container.Config
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:            cfg.Sentry.Dsn,
		Debug:          cfg.Sentry.Debug,
		DataCollection: sentryDataCollection(cfg.Sentry.SendDefaultPII),
	}); err != nil {
		container.Logger.Error("sentry.Init failed", zap.Error(err))
	}
	defer sentry.Flush(2 * time.Second)

	var wg sync.WaitGroup

	// Start event consumer in background
	wg.Go(container.EventConsumer.Start)

	srv := &http.Server{
		Addr:           fmt.Sprintf(":%s", cfg.Server.ExternalPort),
		Handler:        container.SetupRouter(),
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	serverErr := make(chan error, 1)
	go func() {
		container.Logger.Info("Server starting",
			zap.String("port", cfg.Server.ExternalPort),
			zap.String("mode", cfg.Server.RunMode),
			zap.String("domain", cfg.Server.Domain),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-appCtx.Done():
		container.Logger.Info("Shutting down server...")
	case err := <-serverErr:
		if err != nil {
			container.Logger.Error("Server failed", zap.Error(err))
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		container.Logger.Error("Server forced to shutdown", zap.Error(err))
	}

	container.EventConsumer.Stop()
	wg.Wait()

	container.Logger.Info("Server exited")
}

// sentryDataCollection maps the sentry.sendDefaultPII config flag onto the
// DataCollection option that replaces the deprecated SendDefaultPII.
func sentryDataCollection(sendPII bool) *sentry.DataCollection {
	if sendPII {
		return &sentry.DataCollection{UserInfo: sentry.Set(true)}
	}

	denyList := &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionDenyList}
	return &sentry.DataCollection{
		UserInfo:   sentry.Set(false),
		Cookies:    &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
		HTTPBodies: []sentry.BodyType{},
		HTTPHeaders: &sentry.HeaderCollectionConfig{
			Request:  denyList,
			Response: denyList,
		},
		QueryParams: denyList,
	}
}
