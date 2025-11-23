package tracing

import (
	"context"
	"fmt"

	"github.com/hilthontt/visper/internal/infrastructure/env"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	ServiceName    string
	Environment    string
	JaegerEndpoint string
}

func NewDefaultConfig(serviceName string) Config {
	return Config{
		ServiceName:    serviceName,
		Environment:    env.GetString("ENVIRONMENT", "development"),
		JaegerEndpoint: env.GetString("JAEGER_ENDPOINT", "http://jaeger:14268/api/traces"),
	}
}

type ShutdownFunc = func(context.Context) error

func InitTracer(cfg Config) (ShutdownFunc, error) {
	exp, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithEndpointURL(cfg.JaegerEndpoint),
		otlptracehttp.WithInsecure(), // Use WithTLSClientConfig() in production
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Resource (service identity)
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Tracer Provider with batching
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))),
	)

	otel.SetTracerProvider(tp)

	// Set global propagator (W3C Trace Context + Baggage)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

func GetTracer(name string) trace.Tracer {
	return otel.GetTracerProvider().Tracer(name)
}
