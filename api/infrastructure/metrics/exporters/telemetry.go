package exporters

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hilthontt/visper/api/infrastructure/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	// defaultOTLPEndpoint is Jaeger's OTLP/HTTP receiver.
	defaultOTLPEndpoint = "http://localhost:4318"
	otlpTracesPath      = "/v1/traces"

	// Legacy Jaeger collector (Thrift over HTTP) endpoint shape, still accepted
	// in the config and mapped to the OTLP receiver of the same host.
	legacyJaegerPath = "/api/traces"
	legacyJaegerPort = "14268"
	otlpHTTPPort     = "4318"

	defaultAppName = "visper-api"
	tracerName     = "github.com/hilthontt/visper/api/telemetry"
)

// OTLPTracesURL turns the configured tracing endpoint into a full OTLP/HTTP
// traces URL. It accepts:
//   - "" (defaults to http://localhost:4318/v1/traces),
//   - a base URL such as "http://jaeger:4318" or "jaeger:4318" (the
//     /v1/traces path is appended),
//   - a full URL with a path, used as-is,
//   - the legacy Jaeger collector URL "http://host:14268/api/traces", which is
//     rewritten to "http://host:4318/v1/traces" for backwards compatibility.
func OTLPTracesURL(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = defaultOTLPEndpoint
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid tracing endpoint %q: %w", endpoint, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid tracing endpoint %q: missing host", endpoint)
	}

	switch strings.TrimRight(u.Path, "/") {
	case legacyJaegerPath:
		if u.Port() == legacyJaegerPort {
			u.Host = net.JoinHostPort(u.Hostname(), otlpHTTPPort)
		}
		u.Path = otlpTracesPath
	case "":
		u.Path = otlpTracesPath
	}

	return u.String(), nil
}

// InitJaegerExporter creates a tracer provider exporting spans over OTLP/HTTP
// (Jaeger accepts OTLP natively) and installs it as the global provider.
func InitJaegerExporter(config *config.Config) (*sdktrace.TracerProvider, error) {
	if config.Jaeger.ServiceName == "" {
		config.Jaeger.ServiceName = defaultAppName
	}
	if config.Jaeger.ServiceVersion == "" {
		config.Jaeger.ServiceVersion = "unknown"
	}

	endpointURL, err := OTLPTracesURL(config.Jaeger.Endpoint)
	if err != nil {
		return nil, err
	}

	exp, err := otlptracehttp.New(context.Background(), otlptracehttp.WithEndpointURL(endpointURL))
	if err != nil {
		return nil, err
	}

	// Schemaless so it merges with resource.Default() whatever semconv schema
	// version the SDK uses (mismatched schema URLs make Merge fail).
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
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

	return tp, nil
}

// SendTelemetryTrace emits a single startup span through the given tracer
// provider. The provider is owned by the caller and is not shut down here.
func SendTelemetryTrace(tp *sdktrace.TracerProvider, config *config.Config) {
	ctx := context.Background()
	now := time.Now().UTC()

	ctx, span := tp.Tracer(tracerName).Start(ctx, "visper.startup",
		trace.WithTimestamp(now),
		trace.WithSpanKind(trace.SpanKindInternal),
	)

	span.SetAttributes(
		attribute.String("event.id", uuid.NewString()),
		attribute.String("service.name", config.Jaeger.ServiceName),
		attribute.String("service.version", config.Jaeger.ServiceVersion),
		attribute.String("go.version", runtime.Version()),
		attribute.String("os", runtime.GOOS),
		attribute.String("architecture", runtime.GOARCH),
		attribute.String("startup.time", now.Format(time.RFC3339)),
	)
	span.End()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = tp.ForceFlush(ctx)
}
