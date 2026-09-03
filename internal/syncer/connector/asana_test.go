package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func asanaTestConnector(t *testing.T, doJSON func(ctx context.Context, apiPath string, query url.Values, out any) error) *AsanaConnector {
	t.Helper()
	connector, err := NewAsanaConnector(map[string]any{
		"asana_workspace_id": "workspace_1",
		"asana_project_ids":  "",
		"asana_team_id":      "team_1",
		"batch_size":         2,
		"credentials": map[string]any{
			"asana_api_token_secret": "token",
		},
	})
	if err != nil {
		t.Fatalf("NewAsanaConnector failed: %v", err)
	}
	connector.doJSON = doJSON
	connector.download = func(ctx context.Context, rawURL string, maxSize int64) ([]byte, error) {
		return []byte("attachment body"), nil
	}
	return connector
}

func asanaSetEnvelope(out any, data any, nextPage string) {
	switch typed := out.(type) {
	case *asanaListEnvelope:
		typed.Data, _ = json.Marshal(data)
		if nextPage != "" {
			typed.NextPage = &asanaNextPage{Offset: nextPage}
		}
	case *asanaObjectEnvelope:
		typed.Data, _ = json.Marshal(data)
	default:
		panic(fmt.Sprintf("unexpected Asana response target %T", out))
	}
}

func asanaFixtureDoJSON(projects []asanaProject, tasks []asanaTask, stories []asanaStory, attachments []asanaAttachment) func(ctx context.Context, apiPath string, query url.Values, out any) error {
	return func(ctx context.Context, apiPath string, query url.Values, out any) error {
		switch apiPath {
		case "projects":
			asanaSetEnvelope(out, projects, "")
		case "tasks":
			asanaSetEnvelope(out, tasks, "")
		case "attachments":
			asanaSetEnvelope(out, attachments, "")
		default:
			if strings.HasPrefix(apiPath, "tasks/") && strings.HasSuffix(apiPath, "/stories") {
				asanaSetEnvelope(out, stories, "")
				return nil
			}
			return fmt.Errorf("unexpected Asana API path %q", apiPath)
		}
		return nil
	}
}

func asanaTestProject(gid, name, teamID string) asanaProject {
	return asanaProject{
		GID:  gid,
		Name: name,
		Team: &struct {
			GID string `json:"gid"`
		}{GID: teamID},
	}
}

func asanaTestTask(gid, name, modifiedAt string) asanaTask {
	return asanaTask{
		GID:          gid,
		Name:         name,
		Notes:        "Task notes",
		PermalinkURL: "https://app.asana.com/0/task/" + gid,
		CreatedAt:    "2026-01-01T00:00:00Z",
		ModifiedAt:   modifiedAt,
		CreatedBy:    asanaUser{Name: "Alice"},
	}
}

func TestAsanaConnectorConfigParsing(t *testing.T) {
	connector, err := NewAsanaConnector(map[string]any{
		"asana_workspace_id": "workspace_1",
		"asana_project_ids":  "project_1, project_2",
		"asana_team_id":      "team_1",
		"sync_batch_size":    7,
		"size_threshold":     12345,
		"credentials": map[string]any{
			"asana_api_token_secret": "token",
		},
	})
	if err != nil {
		t.Fatalf("NewAsanaConnector failed: %v", err)
	}
	if connector.workspaceID != "workspace_1" {
		t.Fatalf("workspace id = %q", connector.workspaceID)
	}
	if len(connector.projectIDs) != 2 || connector.projectIDs[0] != "project_1" || connector.projectIDs[1] != "project_2" {
		t.Fatalf("project ids = %#v", connector.projectIDs)
	}
	if connector.batchSize != 7 {
		t.Fatalf("batch size = %d, want 7", connector.batchSize)
	}
	if connector.sizeThreshold != 12345 {
		t.Fatalf("size threshold = %d, want 12345", connector.sizeThreshold)
	}
}

func TestAsanaConnectorValidate(t *testing.T) {
	connector := asanaTestConnector(t, func(ctx context.Context, apiPath string, query url.Values, out any) error {
		if apiPath != "workspaces/workspace_1" {
			return fmt.Errorf("unexpected validation path %q", apiPath)
		}
		asanaSetEnvelope(out, asanaWorkspace{GID: "workspace_1", Name: "Workspace"}, "")
		return nil
	})
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestAsanaConnectorValidateClassifiesUnauthorized(t *testing.T) {
	connector := asanaTestConnector(t, func(ctx context.Context, apiPath string, query url.Values, out any) error {
		return &asanaAPIError{Status: 401, Message: "invalid token"}
	})
	err := connector.Validate(context.Background())
	var missing *ConnectorMissingCredentialError
	if !errors.As(err, &missing) {
		t.Fatalf("Validate err = %T %v, want ConnectorMissingCredentialError", err, err)
	}
}

func TestAsanaConnectorValidateStatic(t *testing.T) {
	cases := []struct {
		name  string
		conn  *AsanaConnector
		valid bool
	}{
		{name: "missing workspace", conn: &AsanaConnector{token: "token", batchSize: 2}},
		{name: "missing token", conn: &AsanaConnector{workspaceID: "workspace_1", batchSize: 2}},
		{name: "invalid batch", conn: &AsanaConnector{workspaceID: "workspace_1", token: "token", batchSize: 0}},
		{name: "valid", conn: &AsanaConnector{workspaceID: "workspace_1", token: "token", batchSize: 2}, valid: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.conn.validateStatic()
			if tc.valid && err != nil {
				t.Fatalf("validateStatic err = %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatalf("validateStatic unexpectedly passed")
			}
		})
	}
}

func TestAsanaValidateConnectorSettingUsesRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workspaces/request_workspace" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"gid":"request_workspace","name":"Request Workspace"}}`))
	}))
	defer server.Close()

	receiver, err := NewAsanaConnector(map[string]any{
		"asana_workspace_id": "receiver_workspace",
		"credentials": map[string]any{
			"asana_api_token_secret": "receiver_token",
		},
	})
	if err != nil {
		t.Fatalf("NewAsanaConnector failed: %v", err)
	}
	receiver.apiBaseURL = server.URL
	receiver.httpClient = server.Client()
	request := map[string]any{
		"asana_workspace_id": "request_workspace",
		"credentials": map[string]any{
			"asana_api_token_secret": "request_token",
		},
	}
	if err := receiver.ValidateConnectorSetting(context.Background(), request); err != nil {
		t.Fatalf("ValidateConnectorSetting failed: %v", err)
	}
}

func TestAsanaSelectProjectsFilters(t *testing.T) {
	projects := []asanaProject{
		asanaTestProject("p1", "Good", "team_1"),
		{GID: "p2", Name: "Archived", Archived: true, Team: &struct {
			GID string `json:"gid"`
		}{GID: "team_1"}},
		{GID: "p3", Name: "No team"},
		{GID: "p4", Name: "Private", PrivacySetting: "private", Team: &struct {
			GID string `json:"gid"`
		}{GID: "team_other"}},
	}
	connector := asanaTestConnector(t, func(ctx context.Context, apiPath string, query url.Values, out any) error {
		if apiPath != "projects" {
			return fmt.Errorf("unexpected path %q", apiPath)
		}
		asanaSetEnvelope(out, projects, "")
		return nil
	})
	got, err := connector.selectProjects(context.Background())
	if err != nil {
		t.Fatalf("selectProjects failed: %v", err)
	}
	if len(got) != 1 || got[0].GID != "p1" {
		t.Fatalf("projects = %#v, want p1 only", got)
	}
}

func TestAsanaOpenSyncTaskCommentsAndAttachmentFetch(t *testing.T) {
	projects := []asanaProject{asanaTestProject("p1", "Project One", "team_1")}
	tasks := []asanaTask{asanaTestTask("t1", "Task One", "2026-01-02T00:00:00Z")}
	stories := []asanaStory{{
		GID:             "s1",
		ResourceSubtype: "comment_added",
		Text:            "Looks good",
		CreatedAt:       "2026-01-01T01:00:00Z",
		CreatedBy:       asanaUser{Name: "Bob"},
	}}
	attachments := []asanaAttachment{{
		GID:         "a1",
		Name:        "plan.pdf",
		DownloadURL: "https://example.com/plan.pdf",
		Size:        123,
	}}
	connector := asanaTestConnector(t, asanaFixtureDoJSON(projects, tasks, stories, attachments))
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-03T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("documents len = %d, want 2", len(batch.Documents))
	}
	taskDoc := batch.Documents[0]
	if taskDoc.SourceID != "asana:t1" || taskDoc.Extension != ".md" {
		t.Fatalf("task doc = %#v", taskDoc)
	}
	body := string(taskDoc.Blob)
	for _, want := range []string{"Task One", "Task notes", "## Comments", "Comment by Bob: Looks good"} {
		if !strings.Contains(body, want) {
			t.Fatalf("task body missing %q:\n%s", want, body)
		}
	}
	attachmentDoc := batch.Documents[1]
	if attachmentDoc.SourceID != "asana:t1:a1" || attachmentDoc.Extension != ".pdf" || attachmentDoc.FetchRef == nil {
		t.Fatalf("attachment doc = %#v", attachmentDoc)
	}
	fetcher, ok := session.(Fetcher)
	if !ok {
		t.Fatalf("session does not implement Fetcher")
	}
	blob, err := fetcher.Fetch(context.Background(), *attachmentDoc.FetchRef)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if string(blob) != "attachment body" {
		t.Fatalf("fetched blob = %q", blob)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("final NextBatch err = %v, want io.EOF", err)
	}
}

func TestAsanaOpenSyncSkipsOversizedAttachment(t *testing.T) {
	projects := []asanaProject{asanaTestProject("p1", "Project One", "team_1")}
	tasks := []asanaTask{asanaTestTask("t1", "Task One", "2026-01-02T00:00:00Z")}
	attachments := []asanaAttachment{{
		GID:         "a1",
		Name:        "large.bin",
		DownloadURL: "https://example.com/large.bin",
		Size:        1024,
	}}
	connector := asanaTestConnector(t, asanaFixtureDoJSON(projects, tasks, nil, attachments))
	connector.sizeThreshold = 100
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "asana:t1" {
		t.Fatalf("documents = %#v, want task only", batch.Documents)
	}
}

func TestAsanaOpenSyncAdvancesAcrossProjects(t *testing.T) {
	projects := []asanaProject{
		asanaTestProject("p1", "Project One", "team_1"),
		asanaTestProject("p2", "Project Two", "team_1"),
	}
	connector := asanaTestConnector(t, func(ctx context.Context, apiPath string, query url.Values, out any) error {
		switch apiPath {
		case "projects":
			asanaSetEnvelope(out, projects, "")
		case "tasks":
			if query.Get("project") == "p1" {
				if query.Get("offset") == "" {
					asanaSetEnvelope(out, []asanaTask{}, "empty_page")
				} else {
					asanaSetEnvelope(out, []asanaTask{}, "")
				}
			} else {
				asanaSetEnvelope(out, []asanaTask{asanaTestTask("t2", "Task Two", "2026-01-02T00:00:00Z")}, "")
			}
		case "attachments":
			asanaSetEnvelope(out, []asanaAttachment{}, "")
		default:
			if strings.HasPrefix(apiPath, "tasks/") && strings.HasSuffix(apiPath, "/stories") {
				asanaSetEnvelope(out, []asanaStory{}, "")
				return nil
			}
			return fmt.Errorf("unexpected path %q", apiPath)
		}
		return nil
	})
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "asana:t2" {
		t.Fatalf("documents = %#v, want t2 only", batch.Documents)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("final NextBatch = %v, want io.EOF", err)
	}
}

func TestAsanaIncludeDocumentWindowAndFingerprint(t *testing.T) {
	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	doc := SourceDocument{SourceID: "asana:t1", UpdatedAt: mustTime(t, "2026-01-03T00:00:00Z"), Fingerprint: "fp"}
	if !includeAsanaDocument(SyncRequest{WindowStart: &start, WindowEnd: end}, doc) {
		t.Fatalf("window doc should be included")
	}
	oldDoc := doc
	oldDoc.UpdatedAt = mustTime(t, "2026-01-01T00:00:00Z")
	if includeAsanaDocument(SyncRequest{WindowStart: &start, WindowEnd: end}, oldDoc) {
		t.Fatalf("old window doc should be excluded")
	}
	request := SyncRequest{Fingerprints: map[string]string{"asana:t1": "fp"}}
	if includeAsanaDocument(request, doc) {
		t.Fatalf("unchanged fingerprint doc should be excluded")
	}
	request.Fingerprints["asana:t1"] = "changed"
	if !includeAsanaDocument(request, doc) {
		t.Fatalf("changed fingerprint doc should be included")
	}
}

func TestAsanaOpenSyncResumeContinuesAfterAnchor(t *testing.T) {
	projects := []asanaProject{asanaTestProject("p1", "Project One", "team_1")}
	connector := asanaTestConnector(t, func(ctx context.Context, apiPath string, query url.Values, out any) error {
		switch apiPath {
		case "projects":
			asanaSetEnvelope(out, projects, "")
		case "tasks":
			if query.Get("offset") == "" {
				asanaSetEnvelope(out, []asanaTask{asanaTestTask("t1", "Task One", "2026-01-02T00:00:00Z")}, "page_2")
			} else {
				asanaSetEnvelope(out, []asanaTask{asanaTestTask("t2", "Task Two", "2026-01-02T01:00:00Z")}, "")
			}
		case "attachments":
			asanaSetEnvelope(out, []asanaAttachment{}, "")
		default:
			if strings.HasPrefix(apiPath, "tasks/") && strings.HasSuffix(apiPath, "/stories") {
				asanaSetEnvelope(out, []asanaStory{}, "")
				return nil
			}
			return fmt.Errorf("unexpected path %q", apiPath)
		}
		return nil
	})
	connector.batchSize = 1
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-03T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch failed: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "asana:t1" || first.Checkpoint == nil {
		t.Fatalf("first batch = %#v", first)
	}

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-03T00:00:00Z"), Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resumed OpenSync failed: %v", err)
	}
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "asana:t2" {
		t.Fatalf("resumed documents = %#v, want t2", second.Documents)
	}
	if _, err := resumed.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("resumed final NextBatch = %v, want io.EOF", err)
	}
}

func TestAsanaOpenSyncResumeRejectsInvalidCheckpoint(t *testing.T) {
	connector := asanaTestConnector(t, asanaFixtureDoJSON(nil, nil, nil, nil))
	cases := map[string]*SyncCheckpoint{
		"missing":   {},
		"malformed": {Cursor: "not-json"},
		"foreign":   {Cursor: `{"project_gid":"p1","source_id":"other:t1"}`},
		"no-anchor": {Cursor: `{"project_gid":"p1"}`},
	}
	for name, checkpoint := range cases {
		t.Run(name, func(t *testing.T) {
			session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: checkpoint})
			if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
				t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
			}
		})
	}
}

func TestAsanaOpenSyncResumeRejectsMissingAnchor(t *testing.T) {
	projects := []asanaProject{asanaTestProject("p1", "Project One", "team_1")}
	tasks := []asanaTask{asanaTestTask("t1", "Task One", "2026-01-02T00:00:00Z")}
	connector := asanaTestConnector(t, asanaFixtureDoJSON(projects, tasks, nil, nil))
	cursor, err := json.Marshal(asanaSyncCursor{ProjectGID: "p1", SourceID: "asana:missing"})
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{Cursor: string(cursor), SourceID: "asana:missing"}})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	if _, err := session.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestAsanaOpenPruneStreamsAndDeduplicates(t *testing.T) {
	projects := []asanaProject{
		asanaTestProject("p1", "Project One", "team_1"),
		asanaTestProject("p2", "Project Two", "team_1"),
	}
	tasks := []asanaTask{
		asanaTestTask("t1", "Task One", "2026-01-02T00:00:00Z"),
		asanaTestTask("t1", "Task One Duplicate", "2026-01-02T00:00:00Z"),
		asanaTestTask("t2", "Task Two", "2026-01-02T01:00:00Z"),
	}
	attachments := []asanaAttachment{{
		GID:         "a1",
		Name:        "plan.pdf",
		DownloadURL: "https://example.com/plan.pdf",
		Size:        10,
	}}
	connector := asanaTestConnector(t, func(ctx context.Context, apiPath string, query url.Values, out any) error {
		switch apiPath {
		case "projects":
			asanaSetEnvelope(out, projects, "")
		case "tasks":
			if query.Get("offset") == "" {
				asanaSetEnvelope(out, []asanaTask{tasks[0]}, "page_2")
			} else {
				asanaSetEnvelope(out, tasks[1:], "")
			}
		case "attachments":
			if query.Get("parent") == "t1" {
				asanaSetEnvelope(out, attachments, "")
			} else {
				asanaSetEnvelope(out, []asanaAttachment{}, "")
			}
		default:
			return fmt.Errorf("unexpected path %q", apiPath)
		}
		return nil
	})
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first prune NextBatch failed: %v", err)
	}
	if len(first.Documents) != 2 || first.Documents[0].SourceID != "asana:t1" || first.Documents[1].SourceID != "asana:t1:a1" {
		t.Fatalf("first prune docs = %#v", first.Documents)
	}
	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("second prune NextBatch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "asana:t2" {
		t.Fatalf("second prune docs = %#v", second.Documents)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("prune final NextBatch = %v, want io.EOF", err)
	}
}

func TestAsanaOpenPruneAdvancesAcrossEmptyTaskPages(t *testing.T) {
	projects := []asanaProject{
		asanaTestProject("p1", "Project One", "team_1"),
		asanaTestProject("p2", "Project Two", "team_1"),
	}
	connector := asanaTestConnector(t, func(ctx context.Context, apiPath string, query url.Values, out any) error {
		switch apiPath {
		case "projects":
			asanaSetEnvelope(out, projects, "")
		case "tasks":
			if query.Get("project") == "p1" {
				if query.Get("offset") == "" {
					asanaSetEnvelope(out, []asanaTask{}, "empty_page")
				} else {
					asanaSetEnvelope(out, []asanaTask{}, "")
				}
			} else {
				asanaSetEnvelope(out, []asanaTask{asanaTestTask("t2", "Task Two", "2026-01-02T00:00:00Z")}, "")
			}
		case "attachments":
			asanaSetEnvelope(out, []asanaAttachment{}, "")
		default:
			if strings.HasPrefix(apiPath, "tasks/") && strings.HasSuffix(apiPath, "/stories") {
				asanaSetEnvelope(out, []asanaStory{}, "")
				return nil
			}
			return fmt.Errorf("unexpected path %q", apiPath)
		}
		return nil
	})
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "asana:t2" {
		t.Fatalf("documents = %#v, want t2 only", batch.Documents)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("final NextBatch = %v, want io.EOF", err)
	}
}

func TestAsanaConnectorRegisteredBuiltIn(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	connector, err := registry.OpenFromConfig("asana", map[string]any{
		"asana_workspace_id": "workspace_1",
		"credentials": map[string]any{
			"asana_api_token_secret": "token",
		},
	})
	if err != nil {
		t.Fatalf("OpenFromConfig failed: %v", err)
	}
	if _, ok := connector.(*AsanaConnector); !ok {
		t.Fatalf("connector type = %T, want *AsanaConnector", connector)
	}
}

func TestAsanaFetchReferenceRejectsOversizedAttachment(t *testing.T) {
	connector, err := NewAsanaConnector(map[string]any{
		"asana_workspace_id": "workspace_1",
		"size_threshold":     10,
		"credentials": map[string]any{
			"asana_api_token_secret": "token",
		},
	})
	if err != nil {
		t.Fatalf("NewAsanaConnector failed: %v", err)
	}
	connector.download = func(ctx context.Context, rawURL string, maxSize int64) ([]byte, error) {
		t.Fatalf("download should not be called for oversized attachment")
		return nil, nil
	}
	refKey, _ := json.Marshal(asanaFetchReference{
		TaskGID:       "t1",
		AttachmentGID: "a1",
		Filename:      "large.bin",
		DownloadURL:   "https://example.com/large.bin",
		Size:          11,
	})
	if _, err := connector.Fetch(context.Background(), FetchReference{Key: string(refKey)}); err == nil {
		t.Fatalf("Fetch unexpectedly succeeded")
	}
}
