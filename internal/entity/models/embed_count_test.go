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

package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/common"
)

func TestEmbedRejectsUnexpectedResponseCount(t *testing.T) {
	for _, tc := range []struct {
		name      string
		inputs    int
		outputs   int
		wantError bool
	}{
		{"empty_query", 1, 0, true},
		{"empty_batch", 2, 0, true},
		{"short_batch", 2, 1, true},
		{"extra_batch", 2, 3, true},
		{"single_success", 1, 1, false},
		{"batch_success", 2, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/embeddings" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				data := make([]map[string]any, tc.outputs)
				for i := range data {
					data[i] = map[string]any{"index": i, "embedding": []float64{float64(i + 1), float64(i + 2)}}
				}
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
					t.Errorf("encode response: %v", err)
				}
			}))
			defer server.Close()

			driver := newOpenAIForTest(server.URL)
			// Exercise the real driver's response decoding with the local mock's client.
			driver.baseModel.httpClient = server.Client()
			model := newEmbedModel(t, driver, "", 8192)
			modelName := "test-embedding"
			model.ModelName = &modelName
			texts := make([]string, tc.inputs)
			for i := range texts {
				texts[i] = fmt.Sprintf("input %d", i)
			}
			got, err := model.Embed(t.Context(), EmbedRequest{Texts: texts}, nil, nil)
			if tc.wantError {
				want := fmt.Sprintf("unexpected embedding count: got %d, want %d", tc.outputs, tc.inputs)
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %v, want %q", err, want)
				}
				if got != nil {
					t.Fatalf("invalid response returned partial results: %v", got)
				}
			} else if err != nil || len(got) != len(texts) {
				t.Fatalf("complete response: embeddings = %d, error = %v", len(got), err)
			}
			if got := calls.Load(); got != 1 {
				t.Fatalf("provider calls = %d, want 1; count mismatches must not trigger over-limit retries", got)
			}
		})
	}
}

type emptyIsolatedEmbeddingDriver struct{ recordingEmbedDriver }

func (d *emptyIsolatedEmbeddingDriver) Embed(ctx context.Context, modelName *string, req EmbedRequest, apiConfig *APIConfig, config *EmbeddingConfig, usage *common.ModelUsage) ([]EmbeddingData, error) {
	_, err := d.recordingEmbedDriver.Embed(ctx, modelName, req, apiConfig, config, usage)
	return nil, err
}

func TestEmbedRejectsEmptyIsolatedResponse(t *testing.T) {
	driver := &emptyIsolatedEmbeddingDriver{recordingEmbedDriver: recordingEmbedDriver{rejectBatch: true}}
	model := newEmbedModel(t, driver, "", 8192)
	got, err := model.Embed(t.Context(), EmbedRequest{Texts: []string{"first", "second"}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "embedding input 0: driver returned 0 embeddings for one text") {
		t.Fatalf("error = %v, want the isolated input count error", err)
	}
	if got != nil {
		t.Fatalf("invalid isolated response returned partial results: %v", got)
	}
}
