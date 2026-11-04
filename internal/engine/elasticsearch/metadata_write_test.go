// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package elasticsearch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/common"
)

func TestInsertMetadataChecksBulkItems(t *testing.T) {
	if err := common.InitLogger("error", common.FileOutput{}, "metadata_write_test"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		body string
		want []string
	}{
		{
			name: "success",
			body: `{"errors":false,"items":[{"index":{"_id":"doc-1","status":201}},{"index":{"_id":"doc-2","status":200}}]}`,
		},
		{
			name: "rejected",
			body: `{"errors":true,"items":[{"index":{"_id":"doc-1","status":429,"error":{"type":"es_rejected_execution_exception","reason":"queue full"}}},{"index":{"_id":"doc-2","status":201}}]}`,
			want: []string{"doc-1", "HTTP 429", "queue full"},
		},
		{
			name: "partial mapping failure",
			body: `{"errors":true,"items":[{"index":{"_id":"doc-1","status":201}},{"index":{"_id":"doc-2","status":400,"error":{"type":"mapper_parsing_exception","reason":"wrong type"}}}]}`,
			want: []string{"doc-2", "HTTP 400", "wrong type"},
		},
		{
			name: "failure despite false summary",
			body: `{"errors":false,"items":[{"index":{"_id":"doc-1","status":201}},{"index":{"status":503}}]}`,
			want: []string{"doc-2", "HTTP 503"},
		},
		{
			name: "error despite successful status",
			body: `{"errors":true,"items":[{"index":{"_id":"doc-1","status":201,"error":{"reason":"failed"}}},{"index":{"_id":"doc-2","status":201}}]}`,
			want: []string{"doc-1", "failed"},
		},
		{
			name: "unexplained errors",
			body: `{"errors":true,"items":[{"index":{"status":201}},{"index":{"status":201}}]}`,
			want: []string{"reported errors"},
		},
		{
			name: "incomplete acknowledgement",
			body: `{"errors":false,"items":[{"index":{"_id":"doc-1","status":201}}]}`,
			want: []string{"expected 2 items, got 1"},
		},
		{
			name: "missing summary",
			body: `{"items":[{"index":{"status":201}},{"index":{"status":201}}]}`,
			want: []string{"invalid metadata bulk response"},
		},
		{
			name: "malformed JSON",
			body: `not JSON`,
			want: []string{"failed to parse bulk response"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, captured := newCapturingServer(t, http.StatusOK, tc.body)
			result, err := newTestEngine(t, srv.URL).InsertMetadata(t.Context(), []map[string]interface{}{
				{"id": "doc-1", "kb_id": "kb-1", "meta_fields": map[string]interface{}{"category": "one"}},
				{"id": "doc-2", "kb_id": "kb-1", "meta_fields": map[string]interface{}{"category": "two"}},
			}, "tenant-1")
			if len(tc.want) == 0 {
				if err != nil || len(result) != 0 {
					t.Fatalf("InsertMetadata = %v, %v", result, err)
				}
			} else {
				if err == nil {
					t.Fatal("failed metadata writes reported success")
				}
				for _, want := range tc.want {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q missing %q", err, want)
					}
				}
			}
			captured.mu.Lock()
			defer captured.mu.Unlock()
			if captured.path != "/_bulk" || len(strings.Split(strings.TrimSpace(captured.body), "\n")) != 4 {
				t.Fatalf("unexpected bulk request: path=%s body=%s", captured.path, captured.body)
			}
		})
	}
}

func TestUpdateMetadataChecksCompletion(t *testing.T) {
	if err := common.InitLogger("error", common.FileOutput{}, "metadata_write_test"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"success", `{"total":1,"updated":1,"timed_out":false,"version_conflicts":0,"failures":[]}`, ""},
		{"timeout", `{"total":0,"timed_out":true,"failures":[]}`, "timeout"},
		{"conflict", `{"total":0,"version_conflicts":1,"failures":[]}`, "version_conflicts=1"},
		{"failure", `{"total":0,"failures":[{"id":"doc-1","status":429,"cause":{"reason":"queue full"}}]}`, "queue full"},
		{"missing total", `{}`, "missing or invalid total"},
		{"malformed JSON", `not JSON`, "failed to parse update response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, captured := newCapturingServer(t, http.StatusOK, tc.body)
			err := newTestEngine(t, srv.URL).UpdateMetadata(t.Context(), "doc-1", "kb-1", map[string]interface{}{"category": "one"}, "tenant-1")
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("UpdateMetadata error = %v, want %q", err, tc.want)
			}
			captured.mu.Lock()
			defer captured.mu.Unlock()
			if !strings.HasSuffix(captured.path, "/_update_by_query") {
				t.Fatalf("incomplete update incorrectly triggered insert: %s", captured.path)
			}
		})
	}
}

func TestUpdateMetadataFallbackPropagatesFailureAndAllowsRetry(t *testing.T) {
	if err := common.InitLogger("error", common.FileOutput{}, "metadata_write_test"); err != nil {
		t.Fatal(err)
	}
	var updates, inserts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		switch {
		case strings.HasSuffix(r.URL.Path, "/_update_by_query"):
			updates.Add(1)
			fmt.Fprint(w, `{"total":0,"updated":0,"timed_out":false,"version_conflicts":0,"failures":[]}`)
		case r.URL.Path == "/_bulk":
			attempt := inserts.Add(1)
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			lines := strings.Split(strings.TrimSpace(string(body)), "\n")
			if len(lines) != 2 {
				t.Errorf("unexpected insert body: %s", body)
				return
			}
			var doc struct {
				ID         string            `json:"id"`
				KBID       string            `json:"kb_id"`
				MetaFields map[string]string `json:"meta_fields"`
			}
			if err := json.Unmarshal([]byte(lines[1]), &doc); err != nil || doc.ID != "doc-1" || doc.KBID != "kb-1" || doc.MetaFields["category"] != "one" {
				t.Errorf("incorrect fallback document: %s, err=%v", lines[1], err)
			}
			if attempt == 1 {
				fmt.Fprint(w, `{"errors":true,"items":[{"index":{"_id":"doc-1","status":429,"error":{"reason":"queue full"}}}]}`)
			} else {
				fmt.Fprint(w, `{"errors":false,"items":[{"index":{"_id":"doc-1","status":201}}]}`)
			}
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	engine := newTestEngine(t, srv.URL)
	meta := map[string]interface{}{"category": "one"}
	if err := engine.UpdateMetadata(t.Context(), "doc-1", "kb-1", meta, "tenant-1"); err == nil || !strings.Contains(err.Error(), "failed to insert metadata") || !strings.Contains(err.Error(), "HTTP 429") {
		t.Fatalf("fallback error = %v", err)
	}
	if err := engine.UpdateMetadata(t.Context(), "doc-1", "kb-1", meta, "tenant-1"); err != nil {
		t.Fatalf("retry error = %v", err)
	}
	if updates.Load() != 2 || inserts.Load() != 2 {
		t.Fatalf("updates=%d inserts=%d, want two complete attempts", updates.Load(), inserts.Load())
	}
}
