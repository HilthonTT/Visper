package dependency

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hilthontt/visper/api/infrastructure/broker"
	"github.com/hilthontt/visper/api/infrastructure/events"
	"github.com/hilthontt/visper/api/infrastructure/jobs"
	"github.com/hilthontt/visper/api/infrastructure/metrics"
	"github.com/hilthontt/visper/api/infrastructure/metrics/exporters"
	"github.com/hilthontt/visper/api/infrastructure/persistence/database"
	"github.com/hilthontt/visper/api/infrastructure/persistence/migration"
	"github.com/hilthontt/visper/api/infrastructure/profiler"
	"github.com/hilthontt/visper/api/infrastructure/storage"
	"go.uber.org/zap"
)

const (
	brokerDataDir = "./data/broker"
	profileDir    = "./data/profiles"
	reportDir     = "./data/reports"
)

func (c *Container) initInfrastructure() error {
	if err := c.initDatabase(); err != nil {
		return err
	}

	tracerProvider, err := exporters.InitJaegerExporter(c.Config)
	if err != nil {
		c.Logger.Error("failed to initialize Jaeger exporter", zap.Error(err))
		// Use noop tracer provider as fallback
		c.Logger.Warn("Using noop tracer provider as fallback")
	} else {
		c.TracerProvider = tracerProvider
		c.Logger.Info("Jaeger exporter initialized successfully",
			zap.String("endpoint", c.Config.Jaeger.Endpoint),
			zap.String("service", c.Config.Jaeger.ServiceName),
		)

		go exporters.SendTelemetryTrace(tracerProvider, c.Config)
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

	localStorage, err := storage.NewLocalStorage()
	if err != nil {
		return err
	}
	c.Storage = localStorage

	return nil
}

func (c *Container) initBackgroundJobs() {
	c.FileCleanupJob = jobs.NewFileCleanupJob(c.FileUC, c.Logger, 6*time.Hour)

	go func() {
		// Give the server a moment to come up before the first cleanup run.
		select {
		case <-time.After(2 * time.Second):
		case <-c.ctx.Done():
			return
		}
		c.Logger.Info("Starting background jobs...")
		c.FileCleanupJob.Start(c.ctx)
	}()

	c.Logger.Info("Background jobs initialized")
}

func (c *Container) initProfile() {
	if err := os.MkdirAll(reportDir, 0o750); err != nil {
		c.Logger.Error("Failed to create report directory",
			zap.String("dir", reportDir),
			zap.Error(err))
		return
	}

	c.generateProfileReports()

	c.Profiler = profiler.NewAdaptiveProfiler(profileDir)
	c.Profiler.Start(c.ctx)
}

// generateProfileReports renders SVG/text reports for CPU profiles captured in
// the last 24 hours. It needs the Go toolchain and is skipped when it is not
// available (e.g. in the production container image).
func (c *Container) generateProfileReports() {
	goBin, err := exec.LookPath("go")
	if err != nil {
		c.Logger.Debug("go toolchain not found, skipping profile report generation")
		return
	}

	if _, err := os.Stat(profileDir); err != nil {
		// Nothing captured yet.
		return
	}

	yesterday := time.Now().Add(-24 * time.Hour)

	err = filepath.WalkDir(profileDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			c.Logger.Warn("Error accessing path during walk",
				zap.String("path", path),
				zap.Error(err))
			return err
		}

		// Only process CPU profiles
		if d.IsDir() || !strings.HasPrefix(d.Name(), "cpu-") {
			return nil
		}

		info, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			// Removed while walking.
			return nil
		}
		if err != nil {
			return err
		}

		if info.ModTime().Before(yesterday) {
			return nil
		}

		base := strings.TrimSuffix(d.Name(), ".pprof")
		for _, report := range []struct{ flag, ext string }{{"-svg", ".svg"}, {"-top", ".txt"}} {
			outPath := filepath.Join(reportDir, base+report.ext)
			// goBin comes from LookPath and path from walking our own profile
			// directory; the arguments are not user controlled.
			out, err := exec.Command(goBin, "tool", "pprof", report.flag, path).Output() //nolint:gosec // G204: trusted, non-user input
			if err != nil {
				c.Logger.Error("Failed to generate profile report",
					zap.String("path", path),
					zap.String("output", outPath),
					zap.Error(err))
				continue
			}

			if err := os.WriteFile(outPath, out, 0o600); err != nil {
				c.Logger.Error("Failed to write profile report",
					zap.String("output", outPath),
					zap.Error(err))
				continue
			}

			c.Logger.Info("Generated profile report", zap.String("path", outPath))
		}

		return nil
	})

	if err != nil {
		c.Logger.Error("Error walking profile directory",
			zap.String("dir", profileDir),
			zap.Error(err))
	}
}

func (c *Container) initBroker() error {
	brokerInstance, err := broker.NewBroker(brokerDataDir)
	if err != nil {
		return err
	}
	c.Broker = brokerInstance

	eventPublisher, err := events.NewEventPublisher(brokerInstance, "visper-events")
	if err != nil {
		return err
	}

	eventConsumer, err := events.NewEventConsumer(brokerInstance, "visper-consumer-group", "visper-events", c.AuditLogRepo)
	if err != nil {
		return err
	}

	c.EventConsumer = eventConsumer
	c.EventPublisher = eventPublisher

	return nil
}

func (c *Container) initDatabase() error {
	if err := database.InitDb(c.Config); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	if err := migration.Up1(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}
