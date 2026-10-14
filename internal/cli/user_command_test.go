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
	"testing"
)

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
