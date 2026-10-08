package knowledge_compile

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ragflow/internal/service/nav"
)

func TestProcessClaimRetriesDeduperFactoryFailure(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		message string
	}{
		{"factory error", errors.New("model configuration unavailable"), "model configuration unavailable"},
		{"nil deduper", nil, "deduper factory returned nil"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			previousNav := nav.GetNavService()
			nav.SetNavService(&recordingNavService{})
			t.Cleanup(func() { nav.SetNavService(previousNav) })
			scheduler := NewFakeScheduler()
			recovered := false
			consumer := NewConsumer(scheduler,
				WithReader(&fakeReader{}),
				WithWriter(&fakeWriter{}),
				withWikiContributionStore(&memoryWikiContributionStore{items: map[string]wikiDocumentContribution{}}),
				WithDeduperFactory(func(_ context.Context, tenantID string) (Deduper, error) {
					if tenantID == "broken" && !recovered {
						return nil, tt.err
					}
					return NewNoopDeduper(), nil
				}),
			)
			ctx := t.Context()
			if err := scheduler.Publish(ctx, "broken", "kb-broken", "doc-1", string(EventTypeCompleted), []string{"tree"}, nil); err != nil {
				t.Fatalf("publish: %v", err)
			}
			claim, ok, err := scheduler.Claim(ctx, "kb-broken")
			if err != nil || !ok {
				t.Fatalf("claim: ok=%v err=%v", ok, err)
			}
			consumer.processClaim(ctx, claim)

			scheduler.mu.Lock()
			row := scheduler.rows["kb-broken"]
			inflight, errorMsg := len(row.inflight), row.errorMsg
			scheduler.mu.Unlock()
			if inflight != 1 || !strings.Contains(errorMsg, tt.message) {
				t.Fatalf("failed batch: inflight=%d error=%q, want retained batch with %q", inflight, errorMsg, tt.message)
			}

			// A model failure in one user's space must not stop other datasets.
			if err := scheduler.Publish(ctx, "healthy", "kb-healthy", "doc-2", string(EventTypeCompleted), []string{"tree"}, nil); err != nil {
				t.Fatalf("publish healthy dataset: %v", err)
			}
			healthy, ok, err := scheduler.Claim(ctx, "kb-healthy")
			if err != nil || !ok {
				t.Fatalf("claim healthy dataset: ok=%v err=%v", ok, err)
			}
			consumer.processClaim(ctx, healthy)
			if scheduler.rows["kb-healthy"].state != DatasetStateCompleted {
				t.Fatal("healthy dataset did not complete after another dataset's failure")
			}

			// Once configuration is repaired, reclaim and complete the failed batch.
			recovered = true
			scheduler.mu.Lock()
			expired := time.Now().Add(-time.Second)
			row.expires = &expired
			scheduler.mu.Unlock()
			retry, ok, err := scheduler.TryClaim(ctx)
			if err != nil || !ok || retry.DatasetID != "kb-broken" {
				t.Fatalf("reclaim failed dataset: claim=%+v ok=%v err=%v", retry, ok, err)
			}
			consumer.processClaim(ctx, retry)
			if row.state != DatasetStateCompleted || len(row.inflight) != 0 {
				t.Fatalf("retried batch did not complete: state=%q inflight=%d", row.state, len(row.inflight))
			}
		})
	}
}
