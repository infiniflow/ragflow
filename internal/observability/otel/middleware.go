package otel

import (
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/gin-gonic/gin"
)

const instrumentationName = "ragflow/internal/observability/otel"

// ServerMiddleware returns a gin middleware that starts a server span for each
// request. The span is named after the matched gin route template (e.g.
// "GET /api/v1/chats/:id") so high-cardinality path parameters do not create
// unbounded series in the trace backend.
//
// An inbound W3C traceparent header is extracted and the new span is made a
// child of the remote span. A malformed traceparent is silently ignored by the
// OTel propagator and results in a fresh root span.
//
// When the global TracerProvider is the no-op default (otel.enable: false),
// every span is a no-op and the middleware adds negligible overhead.
func ServerMiddleware() gin.HandlerFunc {
	tracer := otel.Tracer(instrumentationName)
	prop := otel.GetTextMapPropagator()

	return func(c *gin.Context) {
		// Extract remote trace context from inbound headers.
		ctx := prop.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))

		// Name the span after the route template once it is known. Before
		// c.Next() the full template is not yet set, so we start with the
		// method + raw path and rename after the handler runs.
		spanName := c.Request.Method + " " + c.Request.URL.Path

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(c.Request.Method),
				semconv.HTTPRouteKey.String(c.FullPath()),
				attribute.String("http.url", c.Request.URL.RequestURI()),
				attribute.String("http.host", c.Request.Host),
				attribute.String("net.peer.addr", c.ClientIP()),
			),
		)
		defer span.End()

		// Propagate the trace context so downstream calls (HTTP clients,
		// harness spans) can attach to this server span.
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		// After the handler the route template is populated. Update the span
		// name to the low-cardinality template form.
		if route := c.FullPath(); route != "" {
			span.SetName(fmt.Sprintf("%s %s", c.Request.Method, route))
			span.SetAttributes(semconv.HTTPRouteKey.String(route))
		}

		status := c.Writer.Status()
		span.SetAttributes(semconv.HTTPResponseStatusCode(status))
		if status >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
		} else {
			span.SetStatus(codes.Ok, "")
		}
	}
}
