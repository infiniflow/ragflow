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

//go:build integration

package elasticsearch

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

// TestSingleChunkUpdateElasticsearch executes the production update script and
// reads persisted documents from Elasticsearch. Requires ES_TEST=1.
func TestSingleChunkUpdateElasticsearch(t *testing.T) {
	if common.GetEnv(common.EnvESTest) != "1" {
		t.Skip("set ES_TEST=1 to run Elasticsearch integration tests")
	}

	t.Run("matching scope", func(t *testing.T) {
		engine, control, index, recorder := newScopeUpdateFixture(t)
		content := "Line 1's\nLine 2\r\n"
		updates := map[string]interface{}{
			"id": "must-not-replace-id", "available_int": 0,
			"content_with_weight": content, "q_2_vec": []float64{0.25, 0.75},
			"tag_feas": map[string]int{"high": 9, "low": 2}, "remove": "obsolete",
		}
		condition := map[string]interface{}{"id": "chunk-1", "doc_id": "doc-1"}
		for attempt := 0; attempt < 2; attempt++ {
			if err := engine.UpdateChunks(t.Context(), condition, updates, index, "kb-1"); err != nil {
				t.Fatalf("UpdateChunks: %v; response: %s", err, recorder.lastResponse)
			}
		}
		if !reflect.DeepEqual(recorder.results, []string{"updated", "updated"}) {
			t.Fatalf("actual Elasticsearch results = %v", recorder.results)
		}
		stored := readScopeUpdateDocument(t, control, index)
		for field, want := range map[string]interface{}{
			"id": "chunk-1", "kb_id": "kb-1", "doc_id": "doc-1",
			"available_int": float64(0), "content_with_weight": content,
			"q_2_vec":  []interface{}{0.25, 0.75},
			"tag_feas": map[string]interface{}{"high": float64(9), "low": float64(2)},
		} {
			if !reflect.DeepEqual(stored.Source[field], want) {
				t.Errorf("stored %s = %#v, want %#v", field, stored.Source[field], want)
			}
		}
		if _, found := stored.Source["obsolete"]; found {
			t.Error("removed field is still stored")
		}
	})

	for _, tc := range []struct {
		name    string
		value   interface{}
		present bool
	}{
		{"scalar removal", "old", true},
		{"numeric removal", float64(7), true},
		{"null removal", nil, true},
		{"missing removal", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, control, index, recorder := newScopeUpdateFixture(t)
			source := scopeUpdateDocument()
			if tc.present {
				source["labels"] = tc.value
			} else {
				delete(source, "labels")
			}
			putScopeUpdateDocument(t, control, index, source)
			if err := engine.UpdateChunks(t.Context(), map[string]interface{}{"id": "chunk-1"},
				map[string]interface{}{"available_int": 0, "remove": map[string]interface{}{"labels": "old"}},
				index, "kb-1"); err != nil {
				t.Fatalf("UpdateChunks: %v; response: %s", err, recorder.lastResponse)
			}
			stored := readScopeUpdateDocument(t, control, index)
			value, present := stored.Source["labels"]
			if present != tc.present || !reflect.DeepEqual(value, tc.value) || stored.Source["available_int"] != float64(0) {
				t.Fatalf("non-list removal changed the field or lost the other update: %v", stored.Source)
			}
		})
	}

	t.Run("list removal keeps remaining duplicates", func(t *testing.T) {
		engine, control, index, recorder := newScopeUpdateFixture(t)
		if err := engine.UpdateChunks(t.Context(), map[string]interface{}{"id": "chunk-1"},
			map[string]interface{}{"remove": map[string]interface{}{"labels": "old"}}, index, "kb-1"); err != nil {
			t.Fatalf("UpdateChunks: %v; response: %s", err, recorder.lastResponse)
		}
		stored := readScopeUpdateDocument(t, control, index)
		if !reflect.DeepEqual(stored.Source["labels"], []interface{}{"keep", "old"}) {
			t.Fatalf("list removal changed more than the first matching value: %v", stored.Source)
		}
	})

	t.Run("matching list-valued scope", func(t *testing.T) {
		engine, control, index, recorder := newScopeUpdateFixture(t)
		source := scopeUpdateDocument()
		for _, field := range []string{"id", "kb_id", "doc_id"} {
			source[field] = []string{source[field].(string), "alias"}
		}
		putScopeUpdateDocument(t, control, index, source)
		if err := engine.UpdateChunks(t.Context(), map[string]interface{}{"id": "chunk-1", "doc_id": "doc-1"},
			map[string]interface{}{"available_int": 0}, index, "kb-1"); err != nil {
			t.Fatalf("UpdateChunks: %v; response: %s", err, recorder.lastResponse)
		}
		if !reflect.DeepEqual(recorder.results, []string{"updated"}) ||
			readScopeUpdateDocument(t, control, index).Source["available_int"] != float64(0) {
			t.Fatal("matching list-valued scope did not update")
		}
	})

	for _, field := range []string{"id", "kb_id", "doc_id"} {
		for _, asList := range []bool{false, true} {
			t.Run(fmt.Sprintf("scope changed %s list=%t", field, asList), func(t *testing.T) {
				engine, control, index, recorder := newScopeUpdateFixture(t)
				var before scopeStoredDocument
				recorder.beforeUpdate = func() {
					source := scopeUpdateDocument()
					if asList {
						source[field] = []string{"other"}
					} else {
						source[field] = "other"
					}
					putScopeUpdateDocument(t, control, index, source)
					before = readScopeUpdateDocument(t, control, index)
				}
				err := engine.UpdateChunks(t.Context(), map[string]interface{}{"id": "chunk-1", "doc_id": "doc-1"},
					map[string]interface{}{"available_int": 0, "tag_feas": map[string]int{"new": 2},
						"remove": map[string]interface{}{"labels": "old"}}, index, "kb-1")
				if !errors.Is(err, types.ErrDocumentNotFound) {
					t.Fatalf("changed scope: error=%v; response: %s", err, recorder.lastResponse)
				}
				if !reflect.DeepEqual(recorder.results, []string{"noop"}) {
					t.Fatalf("actual Elasticsearch results = %v, want noop", recorder.results)
				}
				if after := readScopeUpdateDocument(t, control, index); !reflect.DeepEqual(after, before) {
					t.Fatalf("rejected update changed source/version: before=%v after=%v", before, after)
				}
			})
		}

		t.Run("lookup mismatch "+field, func(t *testing.T) {
			engine, control, index, recorder := newScopeUpdateFixture(t)
			before := readScopeUpdateDocument(t, control, index)
			condition := map[string]interface{}{"id": "chunk-1", "doc_id": "doc-1"}
			datasetID := "kb-1"
			if field == "kb_id" {
				datasetID = "other"
			} else {
				condition[field] = "other"
			}
			err := engine.UpdateChunks(t.Context(), condition, map[string]interface{}{"available_int": 0}, index, datasetID)
			if !errors.Is(err, types.ErrDocumentNotFound) || len(recorder.results) != 0 {
				t.Fatalf("lookup mismatch: error=%v update results=%v", err, recorder.results)
			}
			if after := readScopeUpdateDocument(t, control, index); !reflect.DeepEqual(after, before) {
				t.Fatalf("lookup mismatch changed source/version: before=%v after=%v", before, after)
			}
		})
	}
}

// scopeUpdateRecorder forwards every request to Elasticsearch, recording actual
// update responses and optionally changing a document just before its update.
type scopeUpdateRecorder struct {
	base         esapi.Transport
	beforeUpdate func()
	results      []string
	lastResponse string
}

// Perform preserves real Elasticsearch execution, including compilation/errors.
func (r *scopeUpdateRecorder) Perform(req *http.Request) (*http.Response, error) {
	if !strings.Contains(req.URL.Path, "/_update/") {
		return r.base.Perform(req)
	}
	if before := r.beforeUpdate; before != nil {
		r.beforeUpdate = nil
		before()
	}
	res, err := r.base.Perform(req)
	if err != nil {
		return res, err
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, err
	}
	res.Body = io.NopCloser(bytes.NewReader(body))
	r.lastResponse = string(body)
	var result struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err == nil {
		r.results = append(r.results, result.Result)
	}
	return res, nil
}

// newScopeUpdateFixture owns one random index and never changes shared templates.
func newScopeUpdateFixture(t *testing.T) (*Engine, *elasticsearch.Client, string, *scopeUpdateRecorder) {
	t.Helper()
	cfg := getESTestConfig()
	newClient := func() *elasticsearch.Client {
		client, err := elasticsearch.NewClient(elasticsearch.Config{
			Addresses: []string{cfg.Hosts}, Username: cfg.Username, Password: cfg.Password,
		})
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	client, control := newClient(), newClient()
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	index := "chunk_scope_test_" + hex.EncodeToString(suffix[:])
	mapping := `{"settings":{"number_of_shards":1,"number_of_replicas":0},"mappings":{"properties":{
		"id":{"type":"keyword"},"kb_id":{"type":"keyword"},"doc_id":{"type":"keyword"},
		"labels":{"type":"keyword"},"available_int":{"type":"integer"},
		"tag_feas":{"type":"object","enabled":false},
		"q_2_vec":{"type":"dense_vector","dims":2,"index":false}}}}`
	res, err := control.Indices.Create(index, control.Indices.Create.WithContext(t.Context()),
		control.Indices.Create.WithBody(strings.NewReader(mapping)))
	readScopeUpdateResponse(t, res, err, nil)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		res, err := control.Indices.Delete([]string{index}, control.Indices.Delete.WithContext(ctx))
		readScopeUpdateResponse(t, res, err, nil)
	})
	putScopeUpdateDocument(t, control, index, scopeUpdateDocument())
	recorder := &scopeUpdateRecorder{base: client.Transport}
	client.Transport = recorder
	return &Engine{client: client}, control, index, recorder
}

// scopeUpdateDocument supplies a minimal indexed chunk, including replacement
// features and duplicate list values whose removal semantics must be preserved.
func scopeUpdateDocument() map[string]interface{} {
	return map[string]interface{}{
		"id": "chunk-1", "kb_id": "kb-1", "doc_id": "doc-1", "available_int": 1,
		"content_with_weight": "before", "q_2_vec": []float64{1, 0},
		"tag_feas": map[string]int{"stale": 1}, "obsolete": "remove-me",
		"labels": []string{"old", "keep", "old"},
	}
}

// putScopeUpdateDocument writes only the fixture's addressable document.
func putScopeUpdateDocument(t *testing.T, client *elasticsearch.Client, index string, source map[string]interface{}) {
	t.Helper()
	body, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Index(index, bytes.NewReader(body), client.Index.WithDocumentID("stored-row"),
		client.Index.WithContext(t.Context()), client.Index.WithRefresh("wait_for"))
	readScopeUpdateResponse(t, res, err, nil)
}

// scopeStoredDocument keeps both source and version to detect unintended writes.
type scopeStoredDocument struct {
	Source  map[string]interface{} `json:"_source"`
	Version int                    `json:"_version"`
}

// readScopeUpdateDocument reads the real persisted document without search lag.
func readScopeUpdateDocument(t *testing.T, client *elasticsearch.Client, index string) scopeStoredDocument {
	t.Helper()
	res, err := client.Get(index, "stored-row", client.Get.WithContext(t.Context()))
	var result scopeStoredDocument
	readScopeUpdateResponse(t, res, err, &result)
	return result
}

// readScopeUpdateResponse closes every response and exposes real backend errors.
func readScopeUpdateResponse(t *testing.T, res *esapi.Response, err error, out interface{}) {
	t.Helper()
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError() {
		t.Fatalf("Elasticsearch %s: %s", res.Status(), body)
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			t.Fatal(err)
		}
	}
}
