// Package otel sets up the OpenTelemetry TracerProvider for RAGFlow.
//
// Call Init once at server startup. When otel.enable is false the global
// provider stays a no-op and the returned shutdown is a cheap no-op too.
// All instrumentation code (gin middleware, otelhttp transport, harness spans)
// stays in the binary regardless: they hit the no-op provider and cost
// essentially nothing at runtime.
package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"ragflow/internal/server/config"
)

const serviceName = "ragflow"

// ShutdownFunc drains and flushes pending spans. Call it on graceful shutdown.
type ShutdownFunc func(context.Context) error

// Init configures the global OTel TracerProvider from cfg and returns a
// ShutdownFunc that flushes pending spans. When cfg.Enable is false, Init is a
// no-op and returns a cheap no-op shutdown.
func Init(cfg config.OpenTelemetryConfig) (ShutdownFunc, error) {
	if !cfg.Enable {
		return func(context.Context) error { return nil }, nil
	}

	res, err := sdkresource.New(context.Background(),
		sdkresource.WithAttributes(semconv.ServiceName(serviceName)),
		sdkresource.WithFromEnv(),
		sdkresource.WithProcess(),
	)
	if err != nil {
		res = sdkresource.Default()
	}

	var exporters []sdktrace.SpanExporter

	// OTLP/HTTP exporter — always added when enable: true.
	otlpExporter, err := newOTLPExporter(cfg)
	if err != nil {
		return nil, fmt.Errorf("create OTLP exporter: %w", err)
	}
	exporters = append(exporters, otlpExporter)

	// Optional stdout exporter for local debugging.
	if cfg.Stdout {
		stdoutExp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("create stdout exporter: %w", err)
		}
		exporters = append(exporters, stdoutExp)
	}

	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	}
	for _, exp := range exporters {
		opts = append(opts, sdktrace.WithBatcher(exp,
			sdktrace.WithBatchTimeout(5*time.Second),
		))
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	}, nil
}

func newOTLPExporter(cfg config.OpenTelemetryConfig) (sdktrace.SpanExporter, error) {
	endpoint := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint),
	}
	if !cfg.Secure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	return otlptracehttp.New(context.Background(), opts...)
}
