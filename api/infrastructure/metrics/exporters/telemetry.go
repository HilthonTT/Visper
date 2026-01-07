package exporters

import (
	"context"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/api/infrastructure/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultJaegerEndpoint = "http://localhost:14268/api/traces"
	defaultAppName        = "gofr-app"
	tracerName            = "gofr-telemetry"
)

var tracer trace.Tracer

func InitJaegerExporter(config *config.Config) (*sdktrace.TracerProvider, error) {
	if config.Jaeger.ServiceName == "" {
		config.Jaeger.ServiceName = defaultAppName
	}
	if config.Jaeger.ServiceVersion == "" {
		config.Jaeger.ServiceVersion = "unknown"
	}
	if config.Jaeger.Endpoint == "" {
		config.Jaeger.Endpoint = defaultJaegerEndpoint
	}

	exp, err := jaeger.New(
		jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(config.Jaeger.Endpoint)),
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.Jaeger.ServiceName),
			semconv.ServiceVersion(config.Jaeger.ServiceVersion),
			attribute.String("go.version", runtime.Version()),
			attribute.String("os", runtime.GOOS),
			attribute.String("arch", runtime.GOARCH),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	tracer = tp.Tracer(tracerName)

	return tp, nil
}

func SendTelemetryTrace(config *config.Config) {
	tp, err := InitJaegerExporter(config)
	if err != nil {
		return
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}()

	ctx := context.Background()
	now := time.Now().UTC()

	ctx, span := tracer.Start(ctx, "visper.startup",
		trace.WithTimestamp(now),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	span.SetAttributes(
		attribute.String("event.id", uuid.NewString()),
		attribute.String("event.source", "gofr-framework"),
		attribute.String("service.name", config.Jaeger.ServiceName),
		attribute.String("service.version", config.Jaeger.ServiceVersion),
		attribute.String("go.version", runtime.Version()),
		attribute.String("os", runtime.GOOS),
		attribute.String("architecture", runtime.GOARCH),
		attribute.String("startup.time", now.Format(time.RFC3339)),
		attribute.Int("raw.data.size", 0),
	)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = tp.ForceFlush(ctx)
}
