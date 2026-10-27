package parser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	models "ragflow/internal/entity/models"
)

// minerURunningServer answers every status call the way a MinerU server does
// while a task is still being parsed: HTTP 200 with no "results" block, which
// ShowTask reports as an error and the poll therefore treats as "not done".
func minerURunningServer(t *testing.T) *models.MinerULocalModel {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"running"}`))
	}))
	t.Cleanup(server.Close)
	return models.NewMinerLocalUModel(
		map[string]string{"default": server.URL},
		models.URLSuffix{Task: "tasks"},
	)
}

func TestPollMinerUTask_StopsOnContextCancel(t *testing.T) {
	withSSRFBypass(t)
	driver := minerURunningServer(t)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := pollMinerUTask(ctx, driver, "task-1", &models.APIConfig{}, 30*time.Second)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	// The poll must leave on cancellation instead of sleeping out the 30s
	// budget it was given.
	if elapsed > 5*time.Second {
		t.Fatalf("poll returned after %s, want it to stop at cancellation", elapsed)
	}
}

func TestPollMinerUTask_ReportsTimeoutWithLastError(t *testing.T) {
	withSSRFBypass(t)
	driver := minerURunningServer(t)

	_, err := pollMinerUTask(t.Context(), driver, "task-2", &models.APIConfig{}, 300*time.Millisecond)
	if err == nil {
		t.Fatal("poll: want timeout error, got nil")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want a timeout rather than cancellation", err)
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("error = %q, want it to name the timeout", err)
	}
	if !strings.Contains(err.Error(), "missing results field") {
		t.Fatalf("error = %q, want the last status error as the cause", err)
	}
	cause := errors.Unwrap(err)
	if cause == nil || !strings.Contains(cause.Error(), "missing results field") {
		t.Fatalf("cause = %v, want the last status error wrapped with %%w", cause)
	}
}

func TestPollMinerUTask_StopsBeforeStatusCallWhenAlreadyCancelled(t *testing.T) {
	withSSRFBypass(t)
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":{"doc":{"md_content":"# Done"}}}`))
	}))
	defer server.Close()
	driver := models.NewMinerLocalUModel(
		map[string]string{"default": server.URL},
		models.URLSuffix{Task: "tasks"},
	)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := pollMinerUTask(ctx, driver, "task-3", &models.APIConfig{}, 30*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("status requests = %d, want 0 for an already cancelled run", hits.Load())
	}
}

// A status call can outlive the poll budget on its own request timeout. Its
// late content must not be returned as an in-time success.
func TestPollMinerUTask_ReportsTimeoutWhenStatusReturnsLate(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(500 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":{"doc":{"md_content":"# Late"}}}`))
	}))
	defer server.Close()
	driver := models.NewMinerLocalUModel(
		map[string]string{"default": server.URL},
		models.URLSuffix{Task: "tasks"},
	)

	start := time.Now()
	_, err := pollMinerUTask(t.Context(), driver, "task-late", &models.APIConfig{}, 100*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("poll returned a late result after the budget elapsed")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("error = %v, want it to name the timeout", err)
	}
	if elapsed > time.Second {
		t.Fatalf("poll returned after %s, want it bounded by the 100ms budget", elapsed)
	}
}
