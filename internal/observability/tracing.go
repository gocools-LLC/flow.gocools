package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func InitTracing(ctx context.Context, serviceName string, logger *slog.Logger) (func(context.Context) error, error) {
	endpoint := strings.TrimSpace(os.Getenv("FLOW_OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		logger.Info("otel tracing disabled; FLOW_OTEL_EXPORTER_OTLP_ENDPOINT is not set")
		return func(context.Context) error { return nil }, nil
	}

	options := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint),
	}
	if os.Getenv("FLOW_OTEL_INSECURE") == "true" {
		options = append(options, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, options...)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	logger.Info("otel tracing enabled", "endpoint", endpoint, "service_name", serviceName)
	return tracerProvider.Shutdown, nil
}
