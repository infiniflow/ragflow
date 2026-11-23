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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func jiraCompletenessServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	withConnectorLoopbackTestHook(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/project/RAG") {
			jiraWriteJSON(t, w, map[string]any{"key": "RAG"})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func jiraCompletenessBatch(t *testing.T, c *JiraConnector) (SyncBatch, error) {
	t.Helper()
	session, err := c.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-04T00:00:00Z")})
	if err != nil {
		return SyncBatch{}, err
	}
	defer session.Close()
	return session.NextBatch(context.Background())
}

func TestJiraServerCappedPagination(t *testing.T) {
	var server *httptest.Server
	server = jiraCompletenessServer(t, func(w http.ResponseWriter, r *http.Request) {
		start, _ := strconv.Atoi(r.URL.Query().Get("startAt"))
		issue := jiraFixtureIssue(server.URL, jiraFixtureUpdatedAt)
		issue["id"], issue["key"] = fmt.Sprint(10000+start), fmt.Sprintf("RAG-%d", start+1)
		jiraWriteJSON(t, w, map[string]any{"startAt": start, "maxResults": 1, "total": 3, "issues": []any{issue}})
	})
	c := mustJiraResumeConnector(t, server.URL, map[string]any{"batch_size": 50, "credentials": map[string]any{"rest_api_version": "2", "jira_api_token": "test-token"}})
	batch, err := jiraCompletenessBatch(t, c)
	if err != nil || len(batch.Documents) != 3 {
		t.Fatalf("capped pages documents=%d err=%v", len(batch.Documents), err)
	}
	session, err := c.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	pruned, err := session.NextBatch(context.Background())
	if err != nil || len(pruned.Documents) != 3 {
		t.Fatalf("prune pages documents=%d err=%v", len(pruned.Documents), err)
	}
}

func TestJiraServerRejectsIncompletePages(t *testing.T) {
	for _, tc := range []struct {
		name   string
		offset int
	}{{"empty-before-total", 0}, {"wrong-offset", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			server := jiraCompletenessServer(t, func(w http.ResponseWriter, r *http.Request) {
				jiraWriteJSON(t, w, map[string]any{"startAt": tc.offset, "total": 2, "issues": []any{}})
			})
			c := mustJiraResumeConnector(t, server.URL, map[string]any{"credentials": map[string]any{"rest_api_version": "2", "jira_api_token": "test-token"}})
			batch, err := jiraCompletenessBatch(t, c)
			if err == nil || batch.Checkpoint != nil {
				t.Fatalf("incomplete page committed: %+v, %v", batch, err)
			}
		})
	}
}

func TestJiraCloudRejectsIncompleteBulkResponses(t *testing.T) {
	for _, mode := range []string{"missing", "issue-errors", "duplicate", "unexpected", "empty-non-final", "missing-next-token", "repeated-token"} {
		t.Run(mode, func(t *testing.T) {
			var server *httptest.Server
			server = jiraCompletenessServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/rest/api/3/search/jql":
					page := map[string]any{"issues": []any{map[string]any{"id": "10000"}, map[string]any{"id": "10001"}}}
					if mode == "empty-non-final" {
						page["issues"] = []any{}
						page["nextPageToken"] = "next"
					}
					if mode == "missing-next-token" {
						page["isLast"] = false
					}
					if mode == "repeated-token" {
						page["nextPageToken"] = "same"
					}
					jiraWriteJSON(t, w, page)
				case "/rest/api/3/issue/bulkfetch":
					issue := jiraFixtureIssue(server.URL, jiraFixtureUpdatedAt)
					other := jiraFixtureIssue(server.URL, jiraFixtureUpdatedAt)
					other["id"], other["key"] = "10001", "RAG-8"
					page := map[string]any{"issues": []any{issue, other}}
					if mode == "missing" {
						page["issues"] = []any{issue}
					}
					if mode == "issue-errors" {
						page["issueErrors"] = []any{map[string]any{"issueIdOrKey": "10001"}}
					}
					if mode == "duplicate" {
						page["issues"] = []any{issue, issue}
					}
					if mode == "unexpected" {
						other["id"] = "10099"
					}
					jiraWriteJSON(t, w, page)
				default:
					http.NotFound(w, r)
				}
			})
			c := mustJiraResumeConnector(t, server.URL)
			batch, err := jiraCompletenessBatch(t, c)
			if err == nil || batch.Checkpoint != nil {
				t.Fatalf("mode %s committed an incomplete page: err=%v", mode, err)
			}
		})
	}
}

func TestJiraCommentPagination(t *testing.T) {
	for _, mode := range []string{"complete", "disabled", "blacklist", "duplicate", "changed-total", "empty", "nonzero-inline-offset"} {
		t.Run(mode, func(t *testing.T) {
			var requests atomic.Int32
			comment := func(index int) map[string]any {
				return map[string]any{"id": fmt.Sprint(index), "body": fmt.Sprintf("Comment-%d", index), "author": map[string]any{"emailAddress": "skip@example.com"}}
			}
			var server *httptest.Server
			server = jiraCompletenessServer(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/search") {
					issue := jiraFixtureIssue(server.URL, jiraFixtureUpdatedAt)
					block := map[string]any{"total": 3, "startAt": 0, "comments": []any{comment(0)}}
					if mode == "nonzero-inline-offset" {
						block["startAt"] = 1
						block["comments"] = []any{comment(1)}
					}
					issue["fields"].(map[string]any)["comment"] = block
					jiraWriteJSON(t, w, map[string]any{"issues": []any{issue}, "total": 1, "startAt": 0})
					return
				}
				requests.Add(1)
				start, _ := strconv.Atoi(r.URL.Query().Get("startAt"))
				block := map[string]any{"total": 3, "startAt": start, "comments": []any{comment(start)}}
				if mode == "duplicate" {
					block["comments"] = []any{comment(0)}
				}
				if mode == "changed-total" {
					block["total"] = 4
				}
				if mode == "empty" {
					block["comments"] = []any{}
				}
				jiraWriteJSON(t, w, block)
			})
			opts := map[string]any{"credentials": map[string]any{"rest_api_version": "2", "jira_api_token": "test-token"}}
			if mode == "disabled" {
				opts["include_comments"] = false
			}
			if mode == "blacklist" {
				opts["comment_email_blacklist"] = []string{"skip@example.com"}
			}
			batch, err := jiraCompletenessBatch(t, mustJiraResumeConnector(t, server.URL, opts))
			if mode == "duplicate" || mode == "changed-total" || mode == "empty" {
				if err == nil || batch.Checkpoint != nil {
					t.Fatalf("incomplete comments accepted: %v", err)
				}
				return
			}
			if err != nil || len(batch.Documents) != 1 {
				t.Fatalf("comments batch: %v", err)
			}
			body := string(batch.Documents[0].Blob)
			if mode == "disabled" {
				if requests.Load() != 0 || strings.Contains(body, "Comment-") {
					t.Error("disabled comments fetched or emitted")
				}
			} else if mode == "blacklist" {
				if strings.Contains(body, "Comment-") || requests.Load() != 2 {
					t.Error("comment blacklist was not applied after fetching")
				}
			} else {
				for i := 0; i < 3; i++ {
					if strings.Count(body, fmt.Sprintf("Comment-%d", i)) != 1 {
						t.Errorf("comment %d missing or repeated", i)
					}
				}
			}
		})
	}
}
