//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newTestAzureDevOpsConnector points a connector at a stub server.
func newTestAzureDevOpsConnector(t *testing.T, serverURL string, overrides map[string]any) *AzureDevOpsConnector {
	t.Helper()

	config := map[string]any{
		"organization": "contoso",
		"credentials":  map[string]any{"azure_devops_pat": "token"},
	}
	for key, value := range overrides {
		config[key] = value
	}

	connector, err := NewAzureDevOpsConnector(config)
	if err != nil {
		t.Fatalf("NewAzureDevOpsConnector returned error: %v", err)
	}
	// httptest serves over http, which the connector refuses for real
	// configuration, so the stub endpoint is injected directly.
	connector.baseURL = serverURL
	return connector
}

func TestNewAzureDevOpsConnectorParsesConfig(t *testing.T) {
	connector, err := NewAzureDevOpsConnector(map[string]any{
		"organization":  "contoso",
		"index_mode":    azureDevOpsIndexModeRepositories,
		"projects":      " alpha , beta ",
		"repositories":  "iddaa/Bayi-Portal, sportsbook",
		"content_types": azureDevOpsContentCode,
		"credentials":   map[string]any{"azure_devops_pat": " token "},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if connector.baseURL != "https://dev.azure.com/contoso" {
		t.Fatalf("unexpected base URL: %s", connector.baseURL)
	}
	if len(connector.projects) != 2 || connector.projects[0] != "alpha" {
		t.Fatalf("unexpected projects: %#v", connector.projects)
	}
	if len(connector.repositories) != 2 || connector.repositories[1] != "sportsbook" {
		t.Fatalf("unexpected repositories: %#v", connector.repositories)
	}
	if connector.pat != "token" {
		t.Fatalf("credentials were not trimmed: %q", connector.pat)
	}
	if connector.indexesPullRequests() {
		t.Fatal("content_types=code must not index pull requests")
	}
}

func TestAzureDevOpsOrganizationURLSupportsSelfHostedCollection(t *testing.T) {
	if got := azureDevOpsOrganizationURL("contoso"); got != "https://dev.azure.com/contoso" {
		t.Fatalf("unexpected hosted URL: %s", got)
	}
	if got := azureDevOpsOrganizationURL("https://tfs.contoso.com/DefaultCollection/"); got != "https://tfs.contoso.com/DefaultCollection" {
		t.Fatalf("unexpected self-hosted URL: %s", got)
	}
}

func TestAzureDevOpsDefaultsIndexModeAndContentTypes(t *testing.T) {
	connector, _ := NewAzureDevOpsConnector(map[string]any{"organization": "contoso"})
	if connector.indexMode != azureDevOpsIndexModeOrganization {
		t.Fatalf("unexpected index mode: %s", connector.indexMode)
	}
	if !connector.indexesCode() || !connector.indexesPullRequests() {
		t.Fatal("default content types must cover code and pull requests")
	}
}

// Azure DevOps answers an unauthorized personal access token with HTTP 203 and
// an HTML sign-in page instead of 401.
func TestAzureDevOpsSignInPageIsReportedAsAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNonAuthoritativeInfo)
		_, _ = io.WriteString(w, "<html><body>Sign In</body></html>")
	}))
	defer server.Close()

	connector := newTestAzureDevOpsConnector(t, server.URL, nil)
	err := connector.ValidateConnectorSetting(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "Invalid or expired") {
		t.Fatalf("expected an auth failure, got %v", err)
	}
}

func TestAzureDevOpsForbiddenReportsMissingScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	connector := newTestAzureDevOpsConnector(t, server.URL, nil)
	err := connector.ValidateConnectorSetting(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "Code (Read)") {
		t.Fatalf("expected a scope error, got %v", err)
	}
}

func TestAzureDevOpsValidateRejectsMissingCredentials(t *testing.T) {
	connector, _ := NewAzureDevOpsConnector(map[string]any{"organization": "contoso"})
	if err := connector.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "azure_devops_pat") {
		t.Fatalf("expected a credential error, got %v", err)
	}
}

func TestAzureDevOpsValidateRejectsProjectModeWithoutProjects(t *testing.T) {
	connector, _ := NewAzureDevOpsConnector(map[string]any{
		"organization": "contoso",
		"index_mode":   azureDevOpsIndexModeProjects,
		"credentials":  map[string]any{"azure_devops_pat": "token"},
	})
	if err := connector.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "project") {
		t.Fatalf("expected a project error, got %v", err)
	}
}

func TestAzureDevOpsServerErrorIsRetriedThenSucceeds(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{}})
	}))
	defer server.Close()

	previousDelay := azureDevOpsRetryBaseDelay
	azureDevOpsRetryBaseDelay = time.Millisecond
	defer func() { azureDevOpsRetryBaseDelay = previousDelay }()

	connector := newTestAzureDevOpsConnector(t, server.URL, nil)
	if _, err := connector.listRepositories(context.Background()); err != nil {
		t.Fatalf("retry did not recover: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected exactly one retry, got %d attempts", attempts)
	}
}

func TestAzureDevOpsClientErrorIsNotRetried(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	connector := newTestAzureDevOpsConnector(t, server.URL, nil)
	_, err := connector.listRepositories(context.Background())

	var httpErr *azureDevOpsHTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != http.StatusBadRequest {
		t.Fatalf("expected a 400 http error, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("client errors must not be retried, got %d attempts", attempts)
	}
}

func TestAzureDevOpsListRepositoriesSkipsDisabledAndSorts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
			map[string]any{"name": "zeta", "defaultBranch": "refs/heads/master", "project": map[string]any{"name": "iddaa"}},
			map[string]any{"name": "alpha", "defaultBranch": "refs/heads/develop", "project": map[string]any{"name": "iddaa"}},
			map[string]any{"name": "retired", "isDisabled": true, "project": map[string]any{"name": "iddaa"}},
		}})
	}))
	defer server.Close()

	connector := newTestAzureDevOpsConnector(t, server.URL, nil)
	repos, err := connector.listRepositories(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repos) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(repos))
	}
	if repos[0].Name != "alpha" || repos[0].Branch != "develop" {
		t.Fatalf("unexpected first repository: %#v", repos[0])
	}
	if repos[1].Key() != "iddaa/zeta" {
		t.Fatalf("unexpected repository key: %s", repos[1].Key())
	}
}

func TestAzureDevOpsRepositoryFilterAcceptsQualifiedNames(t *testing.T) {
	connector, _ := NewAzureDevOpsConnector(map[string]any{
		"organization": "contoso",
		"index_mode":   azureDevOpsIndexModeRepositories,
		"repositories": "iddaa/Bayi-Portal,sportsbook",
		"credentials":  map[string]any{"azure_devops_pat": "token"},
	})

	if !connector.matchesRepositoryFilter("iddaa", "Bayi-Portal") {
		t.Fatal("qualified name must match")
	}
	if !connector.matchesRepositoryFilter("other", "sportsbook") {
		t.Fatal("bare name must match in any project")
	}
	if connector.matchesRepositoryFilter("iddaa", "unrelated") {
		t.Fatal("unlisted repository must not match")
	}
}

func TestShouldSkipAzureDevOpsPath(t *testing.T) {
	cases := map[string]bool{
		"/src/Payments/RefundService.cs":     false,
		"/Dockerfile":                        false,
		"/LICENSE":                           false,
		"/.gitattributes":                    true,
		"/.gitignore":                        true,
		"/node_modules/lib/index.js":         true,
		"/Bayi-Portal.Api/bin/Debug/app.dll": true,
		"/docs/logo.png":                     true,
	}
	for itemPath, expected := range cases {
		if got := shouldSkipAzureDevOpsPath(itemPath); got != expected {
			t.Fatalf("shouldSkipAzureDevOpsPath(%q) = %v, want %v", itemPath, got, expected)
		}
	}
}

func TestAzureDevOpsDocumentExtensionFallsBackToText(t *testing.T) {
	if got := azureDevOpsDocumentExtension("Dockerfile"); got != ".txt" {
		t.Fatalf("expected .txt fallback, got %q", got)
	}
	if got := azureDevOpsDocumentExtension("src/App.CS"); got != ".cs" {
		t.Fatalf("expected lowercase extension, got %q", got)
	}
}

func TestAzureDevOpsCodeDocumentCarriesCommitAndWebURL(t *testing.T) {
	connector, _ := NewAzureDevOpsConnector(map[string]any{
		"organization": "contoso",
		"credentials":  map[string]any{"azure_devops_pat": "token"},
	})
	repo := azureDevOpsRepository{Project: "iddaa", Name: "Bayi-Portal", Branch: "master"}
	changed := time.Date(2026, 1, 13, 21, 53, 6, 0, time.UTC)

	item := azureDevOpsItem{Path: "/src/Payments/RefundService.cs", GitObjectType: "blob"}
	item.LatestProcessedChange = &azureDevOpsChange{CommitID: "abc123"}
	item.LatestProcessedChange.Committer.Name = "Ada Lovelace"
	item.LatestProcessedChange.Committer.Date = changed

	document := connector.buildAzureDevOpsCodeDocument(repo, item, []byte("public class RefundService {}"))

	if document.SourceID != "azure_devops:contoso:iddaa:Bayi-Portal:file:src/Payments/RefundService.cs" {
		t.Fatalf("unexpected source id: %s", document.SourceID)
	}
	if document.Extension != ".cs" || document.SemanticIdentifier != "RefundService.cs" {
		t.Fatalf("unexpected document identity: %#v", document)
	}
	if !document.UpdatedAt.Equal(changed) {
		t.Fatalf("unexpected updated at: %s", document.UpdatedAt)
	}
	if document.Fingerprint != "abc123" {
		t.Fatalf("commit id must act as the fingerprint, got %q", document.Fingerprint)
	}
	if webURL, _ := document.Metadata["web_url"].(string); !strings.Contains(webURL, "path=/src/Payments/RefundService.cs") {
		t.Fatalf("unexpected web url: %s", webURL)
	}
}

func TestAzureDevOpsPullRequestDocumentSummarisesReview(t *testing.T) {
	connector, _ := NewAzureDevOpsConnector(map[string]any{
		"organization": "contoso",
		"credentials":  map[string]any{"azure_devops_pat": "token"},
	})
	repo := azureDevOpsRepository{Project: "iddaa", Name: "Bayi-Portal", Branch: "master"}
	created := time.Date(2025, 11, 18, 6, 20, 7, 0, time.UTC)
	closed := time.Date(2025, 11, 20, 6, 20, 7, 0, time.UTC)

	pullRequest := azureDevOpsPullRequest{
		PullRequestID: 4225,
		Title:         "BYS-1517 - PDF olusturma",
		Description:   "Backend tarafina tasindi.",
		Status:        "completed",
		SourceRefName: "refs/heads/feature/BYS-1517",
		TargetRefName: "refs/heads/master",
		CreationDate:  &created,
		ClosedDate:    &closed,
	}
	pullRequest.CreatedBy.DisplayName = "Ada Lovelace"
	pullRequest.Reviewers = append(pullRequest.Reviewers, struct {
		DisplayName string `json:"displayName"`
	}{DisplayName: "Grace Hopper"})

	document := connector.buildAzureDevOpsPullRequestDocument(repo, pullRequest)

	if document.SourceID != "azure_devops:contoso:iddaa:Bayi-Portal:pr:4225" {
		t.Fatalf("unexpected source id: %s", document.SourceID)
	}
	if !document.UpdatedAt.Equal(closed) {
		t.Fatalf("closed date must win over creation date, got %s", document.UpdatedAt)
	}
	body := string(document.Blob)
	if !strings.Contains(body, "Grace Hopper") || !strings.Contains(body, "Backend tarafina tasindi.") {
		t.Fatalf("pull request body is missing review metadata: %s", body)
	}
	if branch, _ := document.Metadata["source_branch"].(string); branch != "feature/BYS-1517" {
		t.Fatalf("unexpected source branch: %s", branch)
	}
}

func TestIncludeAzureDevOpsItemSkipsUnchangedFingerprint(t *testing.T) {
	item := azureDevOpsItem{Path: "/src/App.cs"}
	item.LatestProcessedChange = &azureDevOpsChange{CommitID: "abc123"}

	request := SyncRequest{Fingerprints: map[string]string{"source-id": "abc123"}}
	if includeAzureDevOpsItem(request, "source-id", item) {
		t.Fatal("an unchanged commit id must not be re-synced")
	}

	request.Fingerprints["source-id"] = "def456"
	if !includeAzureDevOpsItem(request, "source-id", item) {
		t.Fatal("a changed commit id must be re-synced")
	}

	if !includeAzureDevOpsItem(SyncRequest{FromBeginning: true}, "source-id", item) {
		t.Fatal("a full resync must include every item")
	}
}

func TestAzureDevOpsOpenSyncWalksFilesThenPullRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pullrequests"):
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{}})
		case strings.Contains(r.URL.Path, "/items") && r.URL.Query().Get("includeContent") == "true":
			_, _ = io.WriteString(w, "public class App {}")
		case strings.Contains(r.URL.Path, "/items"):
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"path": "/src/App.cs", "gitObjectType": "blob"},
				map[string]any{"path": "/.gitignore", "gitObjectType": "blob"},
			}})
		default:
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"name": "repo-a", "defaultBranch": "refs/heads/master", "project": map[string]any{"name": "iddaa"}},
			}})
		}
	}))
	defer server.Close()

	// A batch size of one stops the session at the stage boundary, which is
	// where the resume cursor has to be meaningful.
	connector := newTestAzureDevOpsConnector(t, server.URL, map[string]any{"batch_size": 1})
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: time.Now().UTC()})
	if err != nil {
		t.Fatalf("OpenSync returned error: %v", err)
	}
	defer func() { _ = session.Close() }()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch returned error: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("expected the noise file to be skipped, got %d documents", len(batch.Documents))
	}
	if batch.Documents[0].SourceID != "azure_devops:contoso:iddaa:repo-a:file:src/App.cs" {
		t.Fatalf("unexpected source id: %s", batch.Documents[0].SourceID)
	}
	if batch.Checkpoint == nil || batch.Checkpoint.Cursor == "" {
		t.Fatal("a batch must carry a resume cursor")
	}

	var cursor azureDevOpsSyncCursor
	if err := json.Unmarshal([]byte(batch.Checkpoint.Cursor), &cursor); err != nil {
		t.Fatalf("cursor is not valid JSON: %v", err)
	}
	if cursor.Stage != azureDevOpsStagePullRequests || cursor.RepoKey != "iddaa/repo-a" {
		t.Fatalf("unexpected cursor: %#v", cursor)
	}

	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF after the last repository, got %v", err)
	}
}

func TestAzureDevOpsOpenSyncResumesFromCursor(t *testing.T) {
	listed := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pullrequests"):
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{}})
		case strings.Contains(r.URL.Path, "/items") && r.URL.Query().Get("includeContent") == "true":
			_, _ = io.WriteString(w, "content")
		case strings.Contains(r.URL.Path, "/items"):
			listed++
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"path": "/a.cs", "gitObjectType": "blob"},
				map[string]any{"path": "/b.cs", "gitObjectType": "blob"},
			}})
		default:
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"name": "repo-a", "defaultBranch": "refs/heads/master", "project": map[string]any{"name": "iddaa"}},
			}})
		}
	}))
	defer server.Close()

	cursor, _ := json.Marshal(azureDevOpsSyncCursor{
		RepoKey:    "iddaa/repo-a",
		Stage:      azureDevOpsStageCode,
		FileOffset: 1,
		SourceID:   "azure_devops:contoso:iddaa:repo-a:file:a.cs",
	})
	connector := newTestAzureDevOpsConnector(t, server.URL, nil)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		WindowEnd:     time.Now().UTC(),
		Resume:        &SyncCheckpoint{Cursor: string(cursor), SourceID: "azure_devops:contoso:iddaa:repo-a:file:a.cs"},
	})
	if err != nil {
		t.Fatalf("OpenSync returned error: %v", err)
	}
	defer func() { _ = session.Close() }()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch returned error: %v", err)
	}
	if len(batch.Documents) != 1 || !strings.HasSuffix(batch.Documents[0].SourceID, "file:b.cs") {
		t.Fatalf("resume must continue after the committed file, got %#v", batch.Documents)
	}
	if listed == 0 {
		t.Fatal("the file listing should have been requested once on resume")
	}
}

func TestAzureDevOpsOpenSyncRejectsMissingSourceAnchor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pullrequests"):
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{}})
		case strings.Contains(r.URL.Path, "/items"):
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{}})
		default:
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"name": "repo-a", "defaultBranch": "refs/heads/master", "project": map[string]any{"name": "iddaa"}},
			}})
		}
	}))
	defer server.Close()

	cursor, _ := json.Marshal(azureDevOpsSyncCursor{RepoKey: "iddaa/repo-a", Stage: azureDevOpsStageCode, FileOffset: 1})
	connector := newTestAzureDevOpsConnector(t, server.URL, nil)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{Cursor: string(cursor)},
	})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestAzureDevOpsOpenPruneEmitsCodeAndPullRequestIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pullrequests"):
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"pullRequestId": 7, "title": "fix", "status": "completed"},
			}})
		case strings.Contains(r.URL.Path, "/items"):
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"path": "/src/App.cs", "gitObjectType": "blob"},
			}})
		default:
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"name": "repo-a", "defaultBranch": "refs/heads/master", "project": map[string]any{"name": "iddaa"}},
			}})
		}
	}))
	defer server.Close()

	connector := newTestAzureDevOpsConnector(t, server.URL, nil)
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune returned error: %v", err)
	}
	defer func() { _ = session.Close() }()

	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch returned error: %v", err)
	}
	if len(first.Documents) != 1 || !strings.HasSuffix(first.Documents[0].SourceID, "file:src/App.cs") {
		t.Fatalf("unexpected code snapshot: %#v", first.Documents)
	}

	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch returned error: %v", err)
	}
	if len(second.Documents) != 1 || !strings.HasSuffix(second.Documents[0].SourceID, "pr:7") {
		t.Fatalf("unexpected pull request snapshot: %#v", second.Documents)
	}

	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF at the end of the snapshot, got %v", err)
	}
}

func writeAzureDevOpsJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("failed to encode stub payload: %v", err)
	}
}

func TestAzureDevOpsShortPullRequestDescriptionNeedsNoDetailFetch(t *testing.T) {
	if azureDevOpsPullRequestMayBeTruncated(azureDevOpsPullRequest{Description: "kisa aciklama"}) {
		t.Fatal("a short description must not trigger a detail fetch")
	}
	if azureDevOpsPullRequestMayBeTruncated(azureDevOpsPullRequest{}) {
		t.Fatal("an empty description must not trigger a detail fetch")
	}
}

// The pull request list endpoint truncates descriptions at 400 characters.
func TestAzureDevOpsLongPullRequestDescriptionIsRefetchedInFull(t *testing.T) {
	truncated := strings.Repeat("x", azureDevOpsPRDescriptionLimit)
	full := truncated + " ...and the rest of the description"
	detailRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pullrequests/77"):
			detailRequests++
			writeAzureDevOpsJSON(t, w, map[string]any{
				"pullRequestId": 77, "title": "long", "description": full, "status": "completed",
			})
		case strings.Contains(r.URL.Path, "/pullrequests"):
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"pullRequestId": 77, "title": "long", "description": truncated, "status": "completed"},
			}})
		case strings.Contains(r.URL.Path, "/items"):
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{}})
		default:
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"name": "repo-a", "defaultBranch": "refs/heads/master", "project": map[string]any{"name": "iddaa"}},
			}})
		}
	}))
	defer server.Close()

	connector := newTestAzureDevOpsConnector(t, server.URL, map[string]any{"content_types": azureDevOpsContentPullRequests})
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: time.Now().UTC()})
	if err != nil {
		t.Fatalf("OpenSync returned error: %v", err)
	}
	defer func() { _ = session.Close() }()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch returned error: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("expected one pull request document, got %d", len(batch.Documents))
	}
	if !strings.Contains(string(batch.Documents[0].Blob), full) {
		t.Fatal("the document must carry the untruncated description")
	}
	if detailRequests != 1 {
		t.Fatalf("expected exactly one detail fetch, got %d", detailRequests)
	}
}

func TestAzureDevOpsRejectsCleartextCollectionURL(t *testing.T) {
	connector, err := NewAzureDevOpsConnector(map[string]any{
		"organization": "http://tfs.contoso.com/DefaultCollection",
		"credentials":  map[string]any{"azure_devops_pat": "token"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := connector.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("cleartext collection URLs must be rejected, got %v", err)
	}
}

func TestAzureDevOpsRejectsUnknownSelectorValues(t *testing.T) {
	for field, value := range map[string]string{"index_mode": "everything", "content_types": "everything"} {
		connector, _ := NewAzureDevOpsConnector(map[string]any{
			"organization": "contoso",
			field:          value,
			"credentials":  map[string]any{"azure_devops_pat": "token"},
		})
		if err := connector.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("%s=%q must be rejected, got %v", field, value, err)
		}
	}
}

func TestAzureDevOpsActivePullRequestIsAlwaysReindexed(t *testing.T) {
	windowStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	created := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	request := SyncRequest{WindowStart: &windowStart, WindowEnd: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)}

	active := azureDevOpsPullRequest{PullRequestID: 1, Status: "active", CreationDate: &created}
	if !includeAzureDevOpsPullRequest(request, "id", active) {
		t.Fatal("an old but still active pull request must be re-indexed; its description can change at any time")
	}

	closed := azureDevOpsPullRequest{PullRequestID: 2, Status: "completed", CreationDate: &created, ClosedDate: &created}
	if includeAzureDevOpsPullRequest(request, "id", closed) {
		t.Fatal("a pull request closed before the window must be skipped")
	}
}

// countingBody reports how many bytes the connector actually pulled off the
// wire, so a regression that reads everything and checks the size afterwards
// cannot pass.
type countingBody struct {
	remaining int64
	read      *int64
}

func (b *countingBody) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > b.remaining {
		n = b.remaining
	}
	for i := int64(0); i < n; i++ {
		p[i] = 'x'
	}
	b.remaining -= n
	*b.read += n
	return int(n), nil
}

func (b *countingBody) Close() error { return nil }

type stubTransport struct {
	respond func(*http.Request) *http.Response
}

func (t stubTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return t.respond(request), nil
}

// A single oversized repository file must never be allocated in full.
func TestAzureDevOpsOversizedFileStopsAtTheLimit(t *testing.T) {
	var read int64

	connector, err := NewAzureDevOpsConnector(map[string]any{
		"organization": "contoso",
		"credentials":  map[string]any{"azure_devops_pat": "token"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	connector.httpClient = &http.Client{Transport: stubTransport{respond: func(*http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       &countingBody{remaining: azureDevOpsMaxFileBytes * 4, read: &read},
		}
	}}}

	repo := azureDevOpsRepository{Project: "iddaa", Name: "repo-a", Branch: "master"}
	content, err := connector.fetchFile(context.Background(), repo, "/huge.bin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != nil {
		t.Fatalf("an oversized file must be skipped, got %d bytes", len(content))
	}
	if read != azureDevOpsMaxFileBytes+1 {
		t.Fatalf("the download must stop one byte past the limit, read %d bytes", read)
	}
}

func TestAzureDevOpsFileWithinLimitIsReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "public class App {}")
	}))
	defer server.Close()

	connector := newTestAzureDevOpsConnector(t, server.URL, nil)
	repo := azureDevOpsRepository{Project: "iddaa", Name: "repo-a", Branch: "master"}

	content, err := connector.fetchFile(context.Background(), repo, "/src/App.cs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != "public class App {}" {
		t.Fatalf("unexpected content: %q", string(content))
	}
}

func TestAzureDevOpsRetryAfterIsClamped(t *testing.T) {
	response := &http.Response{Header: http.Header{"Retry-After": []string{"86400"}}}
	if got := azureDevOpsRetryAfter(response, time.Second); got != azureDevOpsRetryMaxDelay {
		t.Fatalf("a large Retry-After must be clamped, got %s", got)
	}

	response.Header.Set("Retry-After", "5")
	if got := azureDevOpsRetryAfter(response, time.Second); got != 5*time.Second {
		t.Fatalf("a small Retry-After must be honoured, got %s", got)
	}

	response.Header.Set("Retry-After", "not-a-number")
	if got := azureDevOpsRetryAfter(response, 2*time.Second); got != 2*time.Second {
		t.Fatalf("an unparsable Retry-After must fall back, got %s", got)
	}

	// Multiplying this by time.Second overflows int64; the negative result would
	// slip past the cap and make the retry fire immediately.
	response.Header.Set("Retry-After", strconv.Itoa(math.MaxInt64/int(time.Millisecond)))
	if got := azureDevOpsRetryAfter(response, time.Second); got != azureDevOpsRetryMaxDelay {
		t.Fatalf("an overflowing Retry-After must be clamped, got %s", got)
	}
}

// The remote listing shifts between runs, so an offset alone points at the
// wrong item. The anchor has to decide where the walk continues.
func TestAzureDevOpsResumeFollowsTheAnchorWhenTheListingShifts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pullrequests"):
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{}})
		case strings.Contains(r.URL.Path, "/items") && r.URL.Query().Get("includeContent") == "true":
			_, _ = io.WriteString(w, "content")
		case strings.Contains(r.URL.Path, "/items"):
			// "added.cs" did not exist when the checkpoint was written, so every
			// index after it has moved by one.
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"path": "/added.cs", "gitObjectType": "blob"},
				map[string]any{"path": "/a.cs", "gitObjectType": "blob"},
				map[string]any{"path": "/b.cs", "gitObjectType": "blob"},
			}})
		default:
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"name": "repo-a", "defaultBranch": "refs/heads/master", "project": map[string]any{"name": "iddaa"}},
			}})
		}
	}))
	defer server.Close()

	// Written when the listing was [a.cs, b.cs] and a.cs had been committed.
	cursor, _ := json.Marshal(azureDevOpsSyncCursor{
		RepoKey:    "iddaa/repo-a",
		Stage:      azureDevOpsStageCode,
		FileOffset: 1,
		SourceID:   "azure_devops:contoso:iddaa:repo-a:file:a.cs",
	})

	connector := newTestAzureDevOpsConnector(t, server.URL, map[string]any{"batch_size": 1})
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		WindowEnd:     time.Now().UTC(),
		Resume:        &SyncCheckpoint{Cursor: string(cursor)},
	})
	if err != nil {
		t.Fatalf("OpenSync returned error: %v", err)
	}
	defer func() { _ = session.Close() }()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch returned error: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("expected one document, got %d", len(batch.Documents))
	}
	// Offset 1 now points at a.cs, which was already committed. Following the
	// anchor continues at b.cs instead.
	if got := batch.Documents[0].SourceID; got != "azure_devops:contoso:iddaa:repo-a:file:b.cs" {
		t.Fatalf("resume must continue after the anchor, got %s", got)
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != batch.Documents[0].SourceID {
		t.Fatalf("the checkpoint must carry the last emitted source id, got %#v", batch.Checkpoint)
	}
}

// Pull request pages shift as new pull requests are opened, so $skip alone is
// not a safe resume position either.
func TestAzureDevOpsPullRequestResumeFollowsTheAnchor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/pullrequests"):
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"pullRequestId": 3, "title": "newest", "status": "active"},
				map[string]any{"pullRequestId": 2, "title": "committed", "status": "active"},
				map[string]any{"pullRequestId": 1, "title": "pending", "status": "active"},
			}})
		case strings.Contains(r.URL.Path, "/items"):
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{}})
		default:
			writeAzureDevOpsJSON(t, w, map[string]any{"value": []any{
				map[string]any{"name": "repo-a", "defaultBranch": "refs/heads/master", "project": map[string]any{"name": "iddaa"}},
			}})
		}
	}))
	defer server.Close()

	cursor, _ := json.Marshal(azureDevOpsSyncCursor{
		RepoKey:  "iddaa/repo-a",
		Stage:    azureDevOpsStagePullRequests,
		SourceID: "azure_devops:contoso:iddaa:repo-a:pr:2",
	})

	connector := newTestAzureDevOpsConnector(t, server.URL, map[string]any{"content_types": azureDevOpsContentPullRequests})
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		WindowEnd:     time.Now().UTC(),
		Resume:        &SyncCheckpoint{Cursor: string(cursor)},
	})
	if err != nil {
		t.Fatalf("OpenSync returned error: %v", err)
	}
	defer func() { _ = session.Close() }()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch returned error: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("only the pull request after the anchor should be emitted, got %d", len(batch.Documents))
	}
	if got := batch.Documents[0].SourceID; got != "azure_devops:contoso:iddaa:repo-a:pr:1" {
		t.Fatalf("unexpected document: %s", got)
	}
}
