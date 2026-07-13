//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestPing_Span verifies that Ping creates an OTel span named
// "SystemHandler.Ping" with the expected HTTP attributes.
func TestPing_Span(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })

	h := NewSystemHandler(nil, tp.Tracer("ragflow"))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request, _ = http.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/system/ping", nil)

	h.Ping(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "pong" {
		t.Errorf("body: got %q, want %q", body, "pong")
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("span count: got %d, want 1", len(spans))
	}
	span := spans[0]
	if want := "SystemHandler.Ping"; span.Name() != want {
		t.Errorf("span name: got %q, want %q", span.Name(), want)
	}

	attrs := make(map[string]string)
	for _, kv := range span.Attributes() {
		attrs[string(kv.Key)] = kv.Value.AsString()
	}
	if got, want := attrs["http.method"], "GET"; got != want {
		t.Errorf("http.method: got %q, want %q", got, want)
	}
	if got, want := attrs["http.path"], "/api/v1/system/ping"; got != want {
		t.Errorf("http.path: got %q, want %q", got, want)
	}
}
