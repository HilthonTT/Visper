package dependency

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hilthontt/visper/api/infrastructure/jobs"
	"github.com/hilthontt/visper/api/infrastructure/metrics"
	"github.com/hilthontt/visper/api/infrastructure/metrics/exporters"
	"github.com/hilthontt/visper/api/infrastructure/profiler"
	"github.com/hilthontt/visper/api/infrastructure/storage"
	"go.uber.org/zap"
)

func (c *Container) initInfrastructure() error {
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

func (c *Container) initProfile() {
	profileDir := "/var/log/myapp/profiles"
	reportDir := "/var/log/myapp/reports"

	// Create report directory
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		c.Logger.Error("Failed to create report directory",
			zap.String("dir", reportDir),
			zap.Error(err))
		return
	}

	// Find all CPU profiles from the last 24 hours
	yesterday := time.Now().Add(-24 * time.Hour)

	err := filepath.Walk(profileDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			c.Logger.Warn("Error accessing path during walk",
				zap.String("path", path),
				zap.Error(err))
			return err
		}

		// Only process CPU profiles
		if !strings.HasPrefix(filepath.Base(path), "cpu-") {
			return nil
		}

		// Check if the file is recent enough
		if info.ModTime().Before(yesterday) {
			c.Logger.Debug("Skipping old profile",
				zap.String("path", path),
				zap.Time("modTime", info.ModTime()))
			return nil
		}

		c.Logger.Info("Processing CPU profile",
			zap.String("path", path),
			zap.Time("modTime", info.ModTime()))

		// Generate SVG for this profile
		svgPath := filepath.Join(reportDir, strings.TrimSuffix(filepath.Base(path), ".pprof")+".svg")
		cmd := exec.Command("go", "tool", "pprof", "-svg", path)
		svg, err := cmd.Output()
		if err != nil {
			c.Logger.Error("Failed to generate SVG",
				zap.String("path", path),
				zap.String("svgPath", svgPath),
				zap.Error(err))
			return nil
		}

		// Write SVG to file
		if err = os.WriteFile(svgPath, svg, 0644); err != nil {
			c.Logger.Error("Failed to write SVG file",
				zap.String("svgPath", svgPath),
				zap.Error(err))
		} else {
			c.Logger.Info("Generated SVG report", zap.String("path", svgPath))
		}

		// Also generate a text report
		txtPath := filepath.Join(reportDir, strings.TrimSuffix(filepath.Base(path), ".pprof")+".txt")
		cmd = exec.Command("go", "tool", "pprof", "-top", path)
		txt, err := cmd.Output()
		if err != nil {
			c.Logger.Error("Failed to generate text report",
				zap.String("path", path),
				zap.String("txtPath", txtPath),
				zap.Error(err))
			return nil
		}

		// Write text report to file
		if err = os.WriteFile(txtPath, txt, 0644); err != nil {
			c.Logger.Error("Failed to write text report",
				zap.String("txtPath", txtPath),
				zap.Error(err))
		} else {
			c.Logger.Info("Generated text report", zap.String("path", txtPath))
		}

		return nil
	})

	if err != nil {
		c.Logger.Error("Error walking profile directory",
			zap.String("dir", profileDir),
			zap.Error(err))
	} else {
		c.Logger.Info("Profile processing complete",
			zap.String("profileDir", profileDir),
			zap.String("reportDir", reportDir))
	}

	c.Profiler = profiler.NewAdaptiveProfiler(profileDir)
	c.Profiler.Start(c.ctx)
}
