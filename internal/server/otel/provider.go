/*
 * Copyright 2026 The RAGFlow Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package otel provides OpenTelemetry-based observability for the RAGFlow
// agent canvas runtime.
//
// The package exposes a TracerProvider factory and a callbacks.Handler
// implementation that maps eino graph-node lifecycle events to OTel spans.
// When no OTLP endpoint is configured the provider falls back to a stdout
// exporter, writing pretty-printed JSON spans to stderr so local debugging
// works without a collector.
package tracer

import (
	"context"
	"fmt"
	"os"
	"ragflow/internal/common"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	stdouttrace "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// Default values applied when ProviderConfig fields are left zero.
const (
	defaultExportTimeout = 30 * time.Second
)

// ProviderConfig configures the OTel TracerProvider built by
// [NewTracerProvider].
type ProviderConfig struct {
	// ServiceName populates the "service.name" resource attribute. Defaults
	// to "ragflow" when empty.
	ServiceName string
	// ServiceVersion populates the "service.version" resource attribute.
	// Defaults to "0.0.0" when empty.
	ServiceVersion string
	// OTLPEndpoint is the OTLP/HTTP collector endpoint (e.g.
	// "http://otel-collector:4318"). When empty, the provider falls back to
	// a stdout exporter instead of no-oping.
	OTLPEndpoint string
	// Insecure disables TLS for the OTLP exporter. Defaults to true.
	Insecure bool
	// SampleRatio is the probability an in-process trace is sampled,
	// in the [0, 1] range. 0 disables the provider (no exporter, no
	// sampler wiring). Defaults to 1.0 (sample everything).
	SampleRatio float64
}

// NewTracerProvider builds a [sdktrace.TracerProvider] honoring config.
//
// One failure mode is special-cased and never returns an error:
//
//   - config.SampleRatio == 0: returns a provider configured with
//     [trace.NeverSample] and no exporter, so even a single manual span
//     is dropped.
//
// When config.OTLPEndpoint is empty the provider falls back to a stdout
// exporter so local debugging works without an OTel collector.
func NewTracerProvider(ctx context.Context, serviceName string, host string, port int, secure, stdout bool, sampleRatio float64) (*sdktrace.TracerProvider, error) {

	serviceVersion := common.GetRAGFlowVersion()
	traceConfig := newTraceProviderConfig(
		serviceName,
		serviceVersion,
		host,
		port,
		secure,
		sampleRatio,
	)

	// Short-circuit: no sampling requested → no-op provider.
	if !stdout || common.AlmostEqual64(traceConfig.SampleRatio, 0) {
		return sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.NeverSample()),
		), nil
	}

	oTelResource, err := buildResource(traceConfig)
	if err != nil {
		return nil, fmt.Errorf("OTEL: build resource: %w", err)
	}

	// Fallback chain:
	//   1. OTLP endpoint configured → OTLP/HTTP exporter.
	//   2. No endpoint → stdout exporter for local debugging.
	var oTelExporter sdktrace.SpanExporter
	if stdout {
		oTelExporter, err = stdouttrace.New(
			stdouttrace.WithWriter(os.Stdout),
			stdouttrace.WithPrettyPrint(),
		)
		if err != nil {
			return nil, fmt.Errorf("OTEL: build stdout exporter: %w", err)
		}
	} else {
		oTelExporter, err = buildExporter(ctx, traceConfig)
		if err != nil {
			return nil, fmt.Errorf("OTEL: build exporter: %w", err)
		}
	}

	batchSpanProcessor := sdktrace.NewBatchSpanProcessor(oTelExporter,
		sdktrace.WithExportTimeout(defaultExportTimeout),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(oTelResource),
		sdktrace.WithSpanProcessor(batchSpanProcessor),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(traceConfig.SampleRatio)),
	)

	// Register as the global tracer provider so that any code that calls
	// otel.Tracer("...") also routes through this provider.
	otel.SetTracerProvider(tp)

	return tp, nil
}

func newTraceProviderConfig(serviceName, serviceVersion, host string, port int, secure bool, sampleRatio float64) ProviderConfig {
	var url string
	if host == "" {
		url = ""
	} else {
		url = fmt.Sprintf("%s:%d", host, port)
	}
	return ProviderConfig{
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
		OTLPEndpoint:   url,
		Insecure:       !secure,
		SampleRatio:    sampleRatio,
	}
}

// buildResource composes the OTel resource (process identity) attached to
// every span emitted by the provider. The resource uses semconv v1.26.0
// attribute keys.
func buildResource(config ProviderConfig) (*resource.Resource, error) {
	schemaURL := semconv.SchemaURL

	// service.namespace is set to "ragflow" regardless of config so that the
	// Go runtime and the Python RAGFlow share a single namespace in any
	// shared OTel backend (see plan §2.10.8).
	attrs := resource.NewWithAttributes(
		schemaURL,
		semconv.ServiceName(config.ServiceName),
		semconv.ServiceVersion(config.ServiceVersion),
		semconv.ServiceNamespace("ragflow"),
	)
	detected, err := resource.Merge(
		resource.Default(),
		attrs,
	)
	if err != nil {
		return nil, err
	}
	return detected, nil
}

// buildExporter constructs an OTLP/HTTP span exporter pointed at the
// configured collector endpoint. Insecure defaults to true; callers that
// need TLS should set config.Insecure=false.
func buildExporter(ctx context.Context, config ProviderConfig) (*otlptrace.Exporter, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(config.OTLPEndpoint),
		otlptracehttp.WithTimeout(defaultExportTimeout),
	}
	if config.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	return otlptracehttp.New(ctx, opts...)
}
