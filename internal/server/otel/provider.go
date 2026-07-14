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
// The handler is designed to be a no-op when tracing is not configured, so
// production code can wire it up unconditionally without paying any cost
// in deployments that do not run an OTel collector.
package otel

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Default values applied when ProviderConfig fields are left zero.
const (
	defaultServiceName    = "ragflow"
	defaultServiceVersion = "0.0.0"
	defaultServiceNS      = "ragflow"
	defaultExportTimeout  = 30 * time.Second
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
	// "http://otel-collector:4318"). When empty, the returned provider
	// has no exporter and effectively no-ops.
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
// Two failure modes are special-cased and never return an error:
//
//   - config.OTLPEndpoint == "": returns a provider with no exporter. Useful
//     for unit tests and for deployments that do not yet run a collector.
//   - config.SampleRatio == 0: returns a provider configured with
//     [trace.NeverSample] and no exporter, so even a single manual span
//     is dropped.
func NewTracerProvider(ctx context.Context, config ProviderConfig) (*sdktrace.TracerProvider, error) {
	config = getTraceProviderConfig(config)

	// Short-circuit: no endpoint or no sampling requested → no-op provider.
	// We deliberately still return a non-nil *sdktrace.TracerProvider so
	// the handler does not need to special-case nil.
	if config.OTLPEndpoint == "" || config.SampleRatio == 0 {
		return sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.NeverSample()),
		), nil
	}

	res, err := buildResource(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("otel: build resource: %w", err)
	}

	exporter, err := buildExporter(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("otel: build exporter: %w", err)
	}

	bsp := sdktrace.NewBatchSpanProcessor(exporter,
		sdktrace.WithExportTimeout(defaultExportTimeout),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.SampleRatio)),
	)

	// Register as the global tracer provider so that any code that calls
	// otel.Tracer("...") also routes through this provider.
	otel.SetTracerProvider(tp)

	return tp, nil
}

func getTraceProviderConfig(config ProviderConfig) ProviderConfig {
	if config.ServiceName == "" {
		config.ServiceName = defaultServiceName
	}
	if config.ServiceVersion == "" {
		config.ServiceVersion = defaultServiceVersion
	}
	if config.SampleRatio < 0 {
		config.SampleRatio = 0
	}
	return config
}

// buildResource composes the OTel resource (process identity) attached to
// every span emitted by the provider. The resource uses semconv v1.26.0
// attribute keys.
func buildResource(ctx context.Context, config ProviderConfig) (*resource.Resource, error) {
	schemaURL := semconv.SchemaURL

	// service.namespace is set to "ragflow" regardless of config so that the
	// Go runtime and the Python RAGFlow share a single namespace in any
	// shared OTel backend (see plan §2.10.8).
	attrs := resource.NewWithAttributes(
		schemaURL,
		semconv.ServiceName(config.ServiceName),
		semconv.ServiceVersion(config.ServiceVersion),
		semconv.ServiceNamespace(defaultServiceNS),
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
