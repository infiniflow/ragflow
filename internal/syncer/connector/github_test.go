package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

// TestGitHubConnectorOpenPrune verifies PRUNE returns Python-compatible html_url IDs.
func TestGitHubConnectorOpenPrune(t *testing.T) {
	connector, err := NewGitHubConnector(map[string]any{
		"repository_owner":      "openai",
		"repository_name":       "ragflow",
		"include_pull_requests": true,
		"include_issues":        true,
		"batch_size":            10,
		"credentials":           map[string]any{"github_access_token": "token"},
	})
	if err != nil {
		t.Fatalf("NewGitHubConnector failed: %v", err)
	}
	connector.baseURL = "https://api.github.test"
	connector.doJSON = githubFixtureDoJSON(t)

	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	got := []string{}
	for _, doc := range batch.Documents {
		got = append(got, doc.SourceID)
	}
	want := []string{
		"https://github.com/openai/ragflow/pull/7",
		"https://github.com/openai/ragflow/issues/3",
	}
	if len(got) != len(want) {
		t.Fatalf("ids len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// githubFixtureDoJSON returns a fixture GitHub JSON transport.
func githubFixtureDoJSON(t *testing.T) func(ctx context.Context, apiURL string, out any) (http.Header, error) {
	t.Helper()
	return func(ctx context.Context, apiURL string, out any) (http.Header, error) {
		parsed, err := url.Parse(apiURL)
		if err != nil {
			t.Fatalf("parse api url: %v", err)
		}
		fixtures := map[string]string{
			"/repos/openai/ragflow": `{"full_name":"openai/ragflow"}`,
			"/repos/openai/ragflow/pulls": `[{
				"html_url":"https://github.com/openai/ragflow/pull/7",
				"number":7,
				"title":"Add syncer",
				"body":"PR body",
				"state":"open",
				"updated_at":"2026-01-03T00:00:00Z",
				"labels":[{"name":"sync"}],
				"user":{"login":"alice"}
			}]`,
			"/repos/openai/ragflow/issues": `[{
				"html_url":"https://github.com/openai/ragflow/issues/3",
				"number":3,
				"title":"Prune bug",
				"body":"Issue body",
				"state":"open",
				"updated_at":"2026-01-02T00:00:00Z",
				"labels":[{"name":"bug"}],
				"user":{"login":"bob"}
			},{
				"html_url":"https://github.com/openai/ragflow/pull/7",
				"number":7,
				"title":"PR shadow",
				"body":"skip me",
				"state":"open",
				"updated_at":"2026-01-03T00:00:00Z",
				"pull_request":{}
			}]`,
		}
		body, ok := fixtures[parsed.Path]
		if !ok {
			t.Fatalf("unexpected api path %s", parsed.Path)
		}
		if err = json.Unmarshal([]byte(body), out); err != nil {
			t.Fatalf("decode fixture: %v", err)
		}
		return http.Header{}, nil
	}
}
