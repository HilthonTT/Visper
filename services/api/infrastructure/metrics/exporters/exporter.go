package exporters

import (
	"github.com/prometheus/otlptranslator"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	metricSdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func Prometheus(appName, appVersion string) metric.Meter {
	exporter, err := prometheus.New(
		prometheus.WithoutTargetInfo(),
		prometheus.WithTranslationStrategy(otlptranslator.NoTranslation))
	if err != nil {
		return nil
	}

	meter := metricSdk.NewMeterProvider(
		metricSdk.WithReader(exporter),
		metricSdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(appName),
		))).Meter(appName, metric.WithInstrumentationVersion(appVersion))

	return meter
}
