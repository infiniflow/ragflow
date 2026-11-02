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

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestParseLocalHTMLWithInlineImage(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "image.html")
	if err := os.WriteFile(filename, []byte(`<p>hello</p><img alt="image" src="data:image/png;base64,aGVsbG8=">`), 0600); err != nil {
		t.Fatal(err)
	}
	c := &CLI{Config: &CommandLineConfig{CLIMode: APIMode}}
	response, err := c.APIParseLocalFileCommand(1, &Command{Params: map[string]interface{}{"filename": filename}})
	if err != nil {
		t.Fatal(err)
	}
	if result, ok := response.(*SimpleResponse); !ok || result.Code != 0 {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestDevUpdateChunkCommandUsesChunkResource(t *testing.T) {
	updated := make(chan struct{}, 1)
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/datasets":
			if r.Method != http.MethodGet {
				t.Errorf("dataset lookup method = %s, want GET", r.Method)
			}
			_, _ = fmt.Fprint(w, `{"code":0,"data":[{"id":"dataset-1","name":"test"}]}`)
		case "/api/v1/datasets/dataset-1/documents/doc-1/chunks/chunk-1":
			if r.Method != http.MethodPatch {
				t.Errorf("update method = %s, want PATCH", r.Method)
			}
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode update: %v", err)
			}
			if payload["content"] != "updated" || payload["available"] != false {
				t.Errorf("unexpected update payload: %#v", payload)
			}
			updated <- struct{}{}
			_, _ = fmt.Fprint(w, `{"code":0,"message":"success"}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	client.useAPIKey = true
	c := &CLI{
		Config: &CommandLineConfig{
			CLIMode:         APIMode,
			APIClientConfig: APIModeConfig{CurrentAPIServer: "test"},
		},
		APIServerClientMap: map[string]*HTTPClient{"test": client},
	}
	result, err := c.DevUpdateChunkCommand(1, &Command{Params: map[string]interface{}{
		"dataset_name": "test",
		"doc_id":       "doc-1",
		"chunk_id":     "chunk-1",
		"json_body":    `{"content":"updated","available":false}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-updated:
	default:
		t.Fatal("chunk update was not sent")
	}
	if response, ok := result.(*SimpleResponse); !ok || response.Code != 0 {
		t.Fatalf("unexpected response: %#v", result)
	}
}

// TestAPIListIngestionTasksSendsDatasetIDFilter pins the fix for the wrong
// type assertion: the parser stores dataset_id as a plain string, but the old
// code asserted .(*string), which always failed and sent dataset_id: null —
// so the server returned tasks across ALL datasets instead of the requested
// one. The id must now reach the request body.
func TestAPIListIngestionTasksSendsDatasetIDFilter(t *testing.T) {
	got := make(chan map[string]interface{}, 1)
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/datasets/ingestion/tasks" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode body: %v", err)
			http.NotFound(w, r)
			return
		}
		got <- payload
		_, _ = fmt.Fprint(w, `{"code":0,"message":"success","data":[]}`)
	}))
	client.useAPIKey = true
	c := &CLI{
		Config: &CommandLineConfig{
			CLIMode:         APIMode,
			APIClientConfig: APIModeConfig{CurrentAPIServer: "test"},
		},
		APIServerClientMap: map[string]*HTTPClient{"test": client},
	}
	if _, err := c.APIListIngestionTasks(1, &Command{Params: map[string]interface{}{"dataset_id": "ds-1"}}); err != nil {
		t.Fatalf("APIListIngestionTasks: %v", err)
	}
	select {
	case payload := <-got:
		if payload["dataset_id"] != "ds-1" {
			t.Fatalf("dataset_id = %#v, want \"ds-1\" (filter must reach the body)", payload["dataset_id"])
		}
	default:
		t.Fatal("no request was sent")
	}
}

// TestAPIListIngestionTasksOmitsDatasetIDWhenAbsent confirms the fix keeps
// the "no filter" semantics when the caller does not supply a dataset_id:
// the field must be omitted/null (the server filters only when it is
// non-nil), not an empty string that would trigger a permission check.
func TestAPIListIngestionTasksOmitsDatasetIDWhenAbsent(t *testing.T) {
	got := make(chan map[string]interface{}, 1)
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		got <- payload
		_, _ = fmt.Fprint(w, `{"code":0,"message":"success","data":[]}`)
	}))
	client.useAPIKey = true
	c := &CLI{
		Config: &CommandLineConfig{
			CLIMode:         APIMode,
			APIClientConfig: APIModeConfig{CurrentAPIServer: "test"},
		},
		APIServerClientMap: map[string]*HTTPClient{"test": client},
	}
	if _, err := c.APIListIngestionTasks(1, &Command{Params: map[string]interface{}{}}); err != nil {
		t.Fatalf("APIListIngestionTasks: %v", err)
	}
	select {
	case payload := <-got:
		if v, present := payload["dataset_id"]; present && v != nil {
			t.Fatalf("dataset_id = %#v, want omitted or null when no filter given", v)
		}
	default:
		t.Fatal("no request was sent")
	}
}
