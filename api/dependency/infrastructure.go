package dependency

import (
	"context"
	"fmt"
	"time"

	"github.com/hilthontt/visper/api/infrastructure/jobs"
	"github.com/hilthontt/visper/api/infrastructure/metrics"
	"github.com/hilthontt/visper/api/infrastructure/metrics/exporters"
	"github.com/hilthontt/visper/api/infrastructure/storage"
	"go.uber.org/zap"
)

func (c *Container) initInfrastructure() error {
	tracerProvider, err := exporters.InitJaegerExporter(c.Config)
	if err != nil {
		c.Logger.Error("failed to initialize Jaeger exporter", zap.Error(err))
	} else {
		c.TracerProvider = tracerProvider
		c.Logger.Info("Jaeger exporter initialized successfully",
			zap.String("endpoint", c.Config.Jaeger.Endpoint),
			zap.String("service", c.Config.Jaeger.ServiceName),
		)

		go exporters.SendTelemetryTrace(c.Config)
	}

	meter := exporters.Prometheus(c.Config.Jaeger.ServiceName, c.Config.Jaeger.ServiceVersion)
	if meter == nil {
		return fmt.Errorf("failed to initialize Prometheus exporter")
	}

	c.MetricsManager = metrics.NewMetricsManager(meter, c.Logger)

	c.MetricsManager.NewGauge("app_go_routines", "Number of goroutines")
	c.MetricsManager.NewGauge("app_sys_memory_alloc", "Bytes allocated and in use")
	c.MetricsManager.NewGauge("app_sys_total_alloc", "Total bytes allocated")
	c.MetricsManager.NewGauge("app_go_numGC", "Number of completed GC cycles")
	c.MetricsManager.NewGauge("app_go_sys", "Total bytes of memory obtained from OS")

	// Register application metrics
	c.MetricsManager.NewCounter("http_requests_total", "Total number of HTTP requests")
	c.MetricsManager.NewHistogram("http_request_duration_seconds", "HTTP request duration in seconds",
		0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0)
	c.MetricsManager.NewUpDownCounter("active_websocket_connections", "Number of active WebSocket connections")
	c.MetricsManager.NewCounter("websocket_messages_sent", "Total number of WebSocket messages sent")
	c.MetricsManager.NewCounter("websocket_messages_received", "Total number of WebSocket messages received")

	c.Logger.Info("Metrics initialized successfully")

	storage, err := storage.NewLocalStorage()
	if err != nil {
		return err
	}
	c.Storage = storage

	return nil
}

func (c *Container) initBackgroundJobs(ctx context.Context) {
	c.FileCleanupJob = jobs.NewFileCleanupJob(c.FileUC, c.Logger, 6*time.Hour)

	go func() {
		time.Sleep(2 * time.Second) // Wait for all dependencies to initialize
		c.Logger.Info("Starting background jobs...")
		c.FileCleanupJob.Start(ctx)
	}()

	c.Logger.Info("Background jobs initialized and started successfully")
}
