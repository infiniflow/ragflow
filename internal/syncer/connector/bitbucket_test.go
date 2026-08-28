package connector

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/utility"
)

func TestNewBitbucketConnectorParsesConfig(t *testing.T) {
	connector, err := NewBitbucketConnector(map[string]any{
		"workspace":        " acme ",
		"repository_slugs": "repo-a, repo-b,",
		"projects":         "P1, P2",
		"batch_size":       7,
		"credentials": map[string]any{
			"bitbucket_account_email": "sync@example.com",
			"bitbucket_api_token":     "token",
		},
	})
	if err != nil {
		t.Fatalf("NewBitbucketConnector failed: %v", err)
	}
	if connector.workspace != "acme" {
		t.Fatalf("workspace = %q, want acme", connector.workspace)
	}
	if len(connector.repositorySlugs) != 2 || connector.repositorySlugs[0] != "repo-a" || connector.repositorySlugs[1] != "repo-b" {
		t.Fatalf("repositorySlugs = %v", connector.repositorySlugs)
	}
	if len(connector.projects) != 2 || connector.projects[0] != "P1" || connector.projects[1] != "P2" {
		t.Fatalf("projects = %v", connector.projects)
	}
	if connector.email != "sync@example.com" || connector.apiToken != "token" {
		t.Fatalf("credentials = %q %q", connector.email, connector.apiToken)
	}
	if connector.batchSize != 7 {
		t.Fatalf("batch_size = %d, want 7", connector.batchSize)
	}
	if connector.baseURL != bitbucketBaseURL {
		t.Fatalf("baseURL = %q, want %q", connector.baseURL, bitbucketBaseURL)
	}
}

func TestBitbucketListReposUsesConfiguredSlugsBeforeProjects(t *testing.T) {
	connector := &BitbucketConnector{
		workspace:       "acme",
		repositorySlugs: []string{"repo-b", "repo-a", "repo-a"},
		projects:        []string{"P1"},
		baseURL:         "https://api.bitbucket.test",
		doJSON: func(ctx context.Context, apiURL string, out any) (http.Header, error) {
			t.Fatalf("repository slug config should not call the workspace API")
			return nil, nil
		},
	}
	repos, err := connector.listRepos(context.Background())
	if err != nil {
		t.Fatalf("listRepos failed: %v", err)
	}
	if len(repos) != 2 || repos[0] != "repo-a" || repos[1] != "repo-b" {
		t.Fatalf("repos = %v, want [repo-a repo-b]", repos)
	}
}

func TestBitbucketListReposFiltersByProjectKey(t *testing.T) {
	connector := &BitbucketConnector{
		workspace: "acme",
		projects:  []string{"P1", "P2"},
		baseURL:   "https://api.bitbucket.test",
		doJSON: func(ctx context.Context, apiURL string, out any) (http.Header, error) {
			parsed, err := url.Parse(apiURL)
			if err != nil {
				t.Fatalf("parse api url: %v", err)
			}
			if parsed.Path != "/repositories/acme" {
				t.Fatalf("path = %q", parsed.Path)
			}
			project := parsed.Query().Get("q")
			if project != `project.key="P1"` && project != `project.key="P2"` {
				t.Fatalf("project query = %q", project)
			}
			slug := "repo-a"
			if strings.Contains(project, "P2") {
				slug = "repo-b"
			}
			body, _ := json.Marshal(bitbucketRepositoryPage{
				Values: []bitbucketRepository{{Slug: slug}},
			})
			return nil, json.Unmarshal(body, out)
		},
	}
	repos, err := connector.listRepos(context.Background())
	if err != nil {
		t.Fatalf("listRepos failed: %v", err)
	}
	if len(repos) != 2 || repos[0] != "repo-a" || repos[1] != "repo-b" {
		t.Fatalf("repos = %v, want [repo-a repo-b]", repos)
	}
}

func TestBitbucketPaginationRejectsForeignHost(t *testing.T) {
	connector := &BitbucketConnector{
		workspace: "acme",
		baseURL:   "https://api.bitbucket.test",
	}

	// listWorkspaceRepos refuses to follow a server-supplied next URL on a
	// different host than the configured Bitbucket API.
	connector.doJSON = func(ctx context.Context, apiURL string, out any) (http.Header, error) {
		body, _ := json.Marshal(bitbucketRepositoryPage{
			Values: []bitbucketRepository{{Slug: "repo-a"}},
			Next:   "https://evil.example.com/repositories/acme",
		})
		return nil, json.Unmarshal(body, out)
	}
	if _, err := connector.listWorkspaceRepos(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "api.bitbucket.test") {
		t.Fatalf("listWorkspaceRepos error = %v, want host mismatch", err)
	}

	// A same-host URL on a different scheme is still rejected.
	connector.doJSON = func(ctx context.Context, apiURL string, out any) (http.Header, error) {
		body, _ := json.Marshal(bitbucketRepositoryPage{
			Values: []bitbucketRepository{{Slug: "repo-a"}},
			Next:   "http://api.bitbucket.test/repositories/acme",
		})
		return nil, json.Unmarshal(body, out)
	}
	if _, err := connector.listWorkspaceRepos(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("listWorkspaceRepos error = %v, want scheme mismatch", err)
	}

	// A same-host URL on a different port is still rejected.
	connector.doJSON = func(ctx context.Context, apiURL string, out any) (http.Header, error) {
		body, _ := json.Marshal(bitbucketRepositoryPage{
			Values: []bitbucketRepository{{Slug: "repo-a"}},
			Next:   "https://api.bitbucket.test:8443/repositories/acme",
		})
		return nil, json.Unmarshal(body, out)
	}
	if _, err := connector.listWorkspaceRepos(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "port") {
		t.Fatalf("listWorkspaceRepos error = %v, want port mismatch", err)
	}

	// The pull-request page helpers stop before requesting a foreign host.
	called := false
	connector.doJSON = func(ctx context.Context, apiURL string, out any) (http.Header, error) {
		called = true
		return http.Header{}, nil
	}
	if _, err := connector.listPullRequestPage(context.Background(), "repo-a", "https://evil.example.com/repositories/acme/repo-a/pullrequests", nil, time.Time{}); err == nil {
		t.Fatalf("listPullRequestPage should reject a foreign host")
	}
	if called {
		t.Fatalf("listPullRequestPage requested a foreign host")
	}

	called = false
	if _, err := connector.listSlimPullRequestPage(context.Background(), "repo-a", "https://evil.example.com/repositories/acme/repo-a/pullrequests"); err == nil {
		t.Fatalf("listSlimPullRequestPage should reject a foreign host")
	}
	if called {
		t.Fatalf("listSlimPullRequestPage requested a foreign host")
	}
}

func TestBitbucketPRListQueryBuildsIncrementalWindow(t *testing.T) {
	start := mustTime(t, "2026-01-02T12:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	query := bitbucketPRListQuery(&start, end)
	q := query.Get("q")
	for _, want := range []string{
		`(state = "OPEN" OR state = "MERGED" OR state = "DECLINED")`,
		"updated_on >",
		"updated_on <=",
	} {
		if !strings.Contains(q, want) {
			t.Fatalf("q = %q, want contains %q", q, want)
		}
	}
	if query.Get("sort") != "updated_on" {
		t.Fatalf("sort = %q", query.Get("sort"))
	}
}

func TestBitbucketPullRequestToSourceDocument(t *testing.T) {
	pr := bitbucketPullRequest{
		ID:          7,
		Title:       "Add: Bitbucket PR",
		Description: "Sync PR body",
		State:       "MERGED",
		Draft:       true,
		Author:      &bitbucketUser{DisplayName: "Alice", Nickname: "alice"},
		Reviewers: []bitbucketUser{
			{DisplayName: "Zoe"},
			{DisplayName: "Bob"},
		},
		Participants: []bitbucketParticipant{
			{User: bitbucketUser{DisplayName: "Zoe"}, Approved: true},
			{User: bitbucketUser{DisplayName: "Bill"}, Approved: false},
		},
		CommentCount: 3,
		TaskCount:    2,
		CreatedOn:    "2026-01-01T10:00:00+00:00",
		UpdatedOn:    "2026-01-03T00:00:00+00:00",
		Source: bitbucketBranchRef{Branch: struct {
			Name string `json:"name"`
		}{Name: "feature"}},
		Destination: bitbucketBranchRef{Branch: struct {
			Name string `json:"name"`
		}{Name: "main"}},
		Links: bitbucketPRLinks{HTML: struct {
			Href string `json:"href"`
		}{Href: "https://bitbucket.org/acme/repo/pull-requests/7"}},
	}
	first := pr.toSourceDocument("acme", "repo")
	second := pr.toSourceDocument("acme", "repo")
	if first.SourceID != "bitbucket:acme:repo:pr:7" {
		t.Fatalf("source id = %q", first.SourceID)
	}
	if first.SemanticIdentifier != "#7: Add Bitbucket PR.md" {
		t.Fatalf("semantic identifier = %q", first.SemanticIdentifier)
	}
	if first.Extension != ".md" {
		t.Fatalf("extension = %q", first.Extension)
	}
	if first.Fingerprint == "" || first.Fingerprint != second.Fingerprint {
		t.Fatalf("fingerprint unstable: %q %q", first.Fingerprint, second.Fingerprint)
	}
	if !strings.Contains(string(first.Blob), "Pull Request Information") ||
		!strings.Contains(string(first.Blob), "Description:\nSync PR body") {
		t.Fatalf("blob missing PR content: %s", first.Blob)
	}
	reviewers, _ := first.Metadata["reviewers"].([]string)
	if len(reviewers) != 2 || reviewers[0] != "Bob" || reviewers[1] != "Zoe" {
		t.Fatalf("reviewers = %v", reviewers)
	}

	changed := pr
	changed.Title = "Add Bitbucket PR v2"
	if got := changed.toSourceDocument("acme", "repo").Fingerprint; got == first.Fingerprint {
		t.Fatalf("fingerprint did not change after title update")
	}
}

func TestBitbucketConnectorOpenSyncUsesWindowAndFingerprint(t *testing.T) {
	connector := mustBitbucketConnector(t, 1, "repo-a", "repo-b")
	connector.doJSON = bitbucketFixtureDoJSON(t)

	start := mustTime(t, "2026-01-02T12:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(batch.Documents))
	}
	doc := batch.Documents[0]
	if doc.SourceID != "bitbucket:acme:repo-a:pr:1" {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.Fingerprint == "" {
		t.Fatalf("fingerprint is empty")
	}
	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch second failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "bitbucket:acme:repo-b:pr:3" {
		t.Fatalf("second documents = %+v, want PR 3", second.Documents)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestBitbucketConnectorOpenSyncResumesAfterCheckpoint(t *testing.T) {
	connector := mustBitbucketConnector(t, 1, "repo-a", "repo-b")
	connector.doJSON = bitbucketFixtureDoJSON(t)

	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch first failed: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "bitbucket:acme:repo-a:pr:1" {
		t.Fatalf("first documents = %+v, want PR 1", first.Documents)
	}
	if first.Checkpoint == nil || first.Checkpoint.SourceID != "bitbucket:acme:repo-a:pr:1" {
		t.Fatalf("first checkpoint = %+v, want PR 1", first.Checkpoint)
	}

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: end, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resume NextBatch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "bitbucket:acme:repo-a:pr:2" {
		t.Fatalf("resume documents = %+v, want PR 2", second.Documents)
	}
	if second.Checkpoint == nil || second.Checkpoint.SourceID != "bitbucket:acme:repo-a:pr:2" {
		t.Fatalf("resume checkpoint = %+v, want PR 2", second.Checkpoint)
	}
	third, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resume NextBatch third failed: %v", err)
	}
	if len(third.Documents) != 1 || third.Documents[0].SourceID != "bitbucket:acme:repo-b:pr:3" {
		t.Fatalf("resume documents = %+v, want PR 3", third.Documents)
	}
}

func TestBitbucketConnectorOpenSyncResumeRejectsMissingSourceAnchor(t *testing.T) {
	connector := mustBitbucketConnector(t, 1, "repo-a", "repo-b")
	connector.doJSON = bitbucketFixtureDoJSON(t)

	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch first failed: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "bitbucket:acme:repo-a:pr:1" {
		t.Fatalf("first documents = %+v, want PR 1", first.Documents)
	}
	resumeCheckpoint := cloneBitbucketCheckpointWithoutSourceID(t, first.Checkpoint)

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: end, Resume: resumeCheckpoint})
	if resumed != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", resumed, err)
	}
}

func TestBitbucketConnectorOpenSyncResumesAcrossPagedRepo(t *testing.T) {
	connector := mustBitbucketConnector(t, 1, "repo-a")
	connector.doJSON = bitbucketPagedFixtureDoJSON(t)

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch first failed: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "bitbucket:acme:repo-a:pr:1" {
		t.Fatalf("first documents = %+v, want PR 1", first.Documents)
	}
	if first.Checkpoint == nil {
		t.Fatalf("first checkpoint is nil")
	}

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resume NextBatch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "bitbucket:acme:repo-a:pr:2" {
		t.Fatalf("resume documents = %+v, want PR 2", second.Documents)
	}
	if _, err = resumed.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("resume EOF = %v", err)
	}
}

func TestBitbucketConnectorOpenPrune(t *testing.T) {
	connector := mustBitbucketConnector(t, 10, "repo-a", "repo-b")
	connector.doJSON = bitbucketFixtureDoJSON(t)

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
		"bitbucket:acme:repo-a:pr:1",
		"bitbucket:acme:repo-a:pr:2",
		"bitbucket:acme:repo-b:pr:3",
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

func TestBitbucketConnectorValidateConnectorSetting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"message":"API Token provided has no Bitbucket scopes."}`, want: "Invalid or expired Bitbucket credentials (HTTP 401)."},
		{name: "forbidden", status: http.StatusForbidden, body: `{"message":"nope"}`, want: "HTTP 403"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			connector := &BitbucketConnector{
				workspace: "acme",
				email:     "sync@example.com",
				apiToken:  "token",
				batchSize: 10,
				baseURL:   "https://api.bitbucket.test",
				doJSON: func(ctx context.Context, apiURL string, out any) (http.Header, error) {
					return nil, &bitbucketHTTPError{Status: tc.status, Body: tc.body}
				},
			}
			err := connector.ValidateConnectorSetting(context.Background(), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestBitbucketConnectorValidateMissingConfig(t *testing.T) {
	cases := []struct {
		name   string
		config map[string]any
		want   string
	}{
		{"missing workspace", map[string]any{
			"credentials": map[string]any{"bitbucket_api_token": "token"},
		}, "workspace"},
		{"missing email", map[string]any{
			"workspace":   "acme",
			"credentials": map[string]any{"bitbucket_api_token": "token"},
		}, "bitbucket_account_email"},
		{"missing token", map[string]any{
			"workspace":   "acme",
			"credentials": map[string]any{"bitbucket_account_email": "sync@example.com"},
		}, "bitbucket_api_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connector, err := NewBitbucketConnector(tc.config)
			if err != nil {
				t.Fatalf("NewBitbucketConnector failed: %v", err)
			}
			if err := connector.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestBitbucketGetJSONRetriesOnTransientStatuses(t *testing.T) {
	originalAllowAny := utility.AllowAnyHostForTest
	originalTries := bitbucketRetryTries
	originalDelay := bitbucketRetryBaseDelay
	originalBackoff := bitbucketRetryBackoff
	originalMaxDelay := bitbucketRetryMaxDelay
	utility.AllowAnyHostForTest = true
	bitbucketRetryTries = 3
	bitbucketRetryBaseDelay = time.Millisecond
	bitbucketRetryBackoff = 1
	bitbucketRetryMaxDelay = 10 * time.Millisecond
	t.Cleanup(func() {
		utility.AllowAnyHostForTest = originalAllowAny
		bitbucketRetryTries = originalTries
		bitbucketRetryBaseDelay = originalDelay
		bitbucketRetryBackoff = originalBackoff
		bitbucketRetryMaxDelay = originalMaxDelay
	})

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		case 2:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"pagelen":1}`))
		}
	}))
	defer server.Close()

	connector := &BitbucketConnector{
		workspace: "acme",
		email:     "sync@example.com",
		apiToken:  "token",
		baseURL:   server.URL,
	}
	var page map[string]any
	if _, err := connector.getJSON(context.Background(), server.URL+"/repositories/acme", &page); err != nil {
		t.Fatalf("getJSON failed: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
}

func TestRegisterBuiltInsOpensBitbucket(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	connector, err := registry.OpenFromConfig("bitbucket", map[string]any{
		"workspace": "acme",
		"credentials": map[string]any{
			"bitbucket_account_email": "sync@example.com",
			"bitbucket_api_token":     "token",
		},
	})
	if err != nil {
		t.Fatalf("OpenFromConfig failed: %v", err)
	}
	if _, ok := connector.(*BitbucketConnector); !ok {
		t.Fatalf("connector type = %T, want *BitbucketConnector", connector)
	}
}

func mustBitbucketConnector(t *testing.T, batchSize int, repos ...string) *BitbucketConnector {
	t.Helper()
	connector, err := NewBitbucketConnector(map[string]any{
		"workspace":        "acme",
		"repository_slugs": strings.Join(repos, ","),
		"batch_size":       batchSize,
		"credentials": map[string]any{
			"bitbucket_account_email": "sync@example.com",
			"bitbucket_api_token":     "token",
		},
	})
	if err != nil {
		t.Fatalf("NewBitbucketConnector failed: %v", err)
	}
	connector.baseURL = "https://api.bitbucket.test"
	return connector
}

// bitbucketFixtureDoJSON returns fixture Bitbucket JSON transport.
func bitbucketFixtureDoJSON(t *testing.T) func(ctx context.Context, apiURL string, out any) (http.Header, error) {
	t.Helper()
	return func(ctx context.Context, apiURL string, out any) (http.Header, error) {
		parsed, err := url.Parse(apiURL)
		if err != nil {
			t.Fatalf("parse api url: %v", err)
		}
		path := parsed.Path
		var body string
		switch {
		case path == "/repositories/acme/repo-a/pullrequests":
			body = `{
				"values": [
					{
						"id": 1,
						"title": "Add Bitbucket sync",
						"description": "Sync PR body",
						"state": "OPEN",
						"author": {"display_name": "Alice"},
						"reviewers": [],
						"participants": [],
						"comment_count": 1,
						"task_count": 0,
						"created_on": "2026-01-01T00:00:00+00:00",
						"updated_on": "2026-01-03T00:00:00+00:00",
						"source": {"branch": {"name": "feature"}},
						"destination": {"branch": {"name": "main"}},
						"links": {"html": {"href": "https://bitbucket.org/acme/repo-a/pull-requests/1"}}
					},
					{
						"id": 2,
						"title": "Old PR",
						"description": "outside window",
						"state": "MERGED",
						"author": {"display_name": "Bob"},
						"reviewers": [],
						"participants": [],
						"comment_count": 0,
						"task_count": 0,
						"created_on": "2026-01-01T00:00:00+00:00",
						"updated_on": "2026-01-02T00:00:00+00:00",
						"source": {"branch": {"name": "old"}},
						"destination": {"branch": {"name": "main"}},
						"links": {"html": {"href": "https://bitbucket.org/acme/repo-a/pull-requests/2"}}
					}
				]
			}`
		case path == "/repositories/acme/repo-b/pullrequests":
			body = `{
				"values": [
					{
						"id": 3,
						"title": "Second repo PR",
						"description": "Repo B body",
						"state": "OPEN",
						"author": {"display_name": "Bob"},
						"reviewers": [],
						"participants": [],
						"comment_count": 0,
						"task_count": 0,
						"created_on": "2026-01-03T00:00:00+00:00",
						"updated_on": "2026-01-03T12:00:00+00:00",
						"source": {"branch": {"name": "feature-b"}},
						"destination": {"branch": {"name": "main"}},
						"links": {"html": {"href": "https://bitbucket.org/acme/repo-b/pull-requests/3"}}
					}
				]
			}`
		default:
			t.Fatalf("unexpected api path %s", path)
		}
		if err := json.Unmarshal([]byte(body), out); err != nil {
			t.Fatalf("decode fixture: %v", err)
		}
		return http.Header{}, nil
	}
}

func bitbucketPagedFixtureDoJSON(t *testing.T) func(ctx context.Context, apiURL string, out any) (http.Header, error) {
	t.Helper()
	return func(ctx context.Context, apiURL string, out any) (http.Header, error) {
		parsed, err := url.Parse(apiURL)
		if err != nil {
			t.Fatalf("parse api url: %v", err)
		}
		if parsed.Path != "/repositories/acme/repo-a/pullrequests" {
			t.Fatalf("unexpected api path %s", parsed.Path)
		}
		var body string
		if parsed.Query().Get("cursor") == "2" {
			body = `{
				"values": [
					{
						"id": 2,
						"title": "Second page PR",
						"state": "OPEN",
						"author": {"display_name": "Bob"},
						"updated_on": "2026-01-04T00:00:00+00:00"
					}
				]
			}`
		} else {
			body = `{
				"next": "https://api.bitbucket.test/repositories/acme/repo-a/pullrequests?cursor=2",
				"values": [
					{
						"id": 1,
						"title": "First page PR",
						"state": "OPEN",
						"author": {"display_name": "Alice"},
						"updated_on": "2026-01-03T00:00:00+00:00"
					}
				]
			}`
		}
		if err := json.Unmarshal([]byte(body), out); err != nil {
			t.Fatalf("decode fixture: %v", err)
		}
		return http.Header{}, nil
	}
}

func cloneBitbucketCheckpointWithoutSourceID(t *testing.T, checkpoint *SyncCheckpoint) *SyncCheckpoint {
	t.Helper()
	if checkpoint == nil {
		t.Fatalf("checkpoint is nil")
	}
	var cursor bitbucketSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		t.Fatalf("decode checkpoint cursor: %v", err)
	}
	cursor.SourceID = ""
	data, err := json.Marshal(cursor)
	if err != nil {
		t.Fatalf("encode checkpoint cursor: %v", err)
	}
	clone := *checkpoint
	clone.Cursor = string(data)
	clone.SourceID = ""
	return &clone
}
