package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/observability"

	"github.com/gin-gonic/gin"
)

func TestSystemHandlerMetricsExposesPrometheusCounters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	observability.ResetIngestionLogMetricsForTest()
	t.Cleanup(observability.ResetIngestionLogMetricsForTest)
	observability.RecordIngestionLogFoldSuccess(4)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	h := NewSystemHandler(nil)
	h.Metrics(ctx)

	if recorder.Code != 200 {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain; version=0.0.4") {
		t.Fatalf("content type = %q, want Prometheus text", got)
	}
	if !strings.Contains(recorder.Body.String(), "ingestion_log_folds_total{result=\"success\"} 1") {
		t.Fatalf("metrics body missing fold counter:\n%s", recorder.Body.String())
	}
}
