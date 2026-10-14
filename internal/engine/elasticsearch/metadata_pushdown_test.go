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

package elasticsearch

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func TestMetadataPushdownSearchesAllTenantIndices(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatal(err)
	}
	status := string(entity.StatusValid)
	kbs := []entity.Knowledgebase{
		{ID: "kb-a", TenantID: "a", Status: &status},
		{ID: "kb-b", TenantID: "a", Status: &status},
		{ID: "kb-c", TenantID: "b", Status: &status},
	}
	if err := db.Create(&kbs).Error; err != nil {
		t.Fatal(err)
	}
	conditions := []map[string]interface{}{{"key": "author", "op": "=", "value": "Alice"}}
	for _, tc := range []struct {
		name         string
		missingIndex bool
		missingKB    bool
		total        int
	}{
		{name: "both tenants", total: 2},
		{name: "overflow falls back", total: metaPushdownMaxSize + 1},
		{name: "missing index falls back", missingIndex: true},
		{name: "missing dataset falls back", missingKB: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			searches := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Elastic-Product", "Elasticsearch")
				if r.Method == http.MethodHead {
					if tc.missingIndex && r.URL.Path == "/ragflow_doc_meta_b" {
						w.WriteHeader(http.StatusNotFound)
					}
					return
				}
				searches++
				if r.URL.Path != "/ragflow_doc_meta_a,ragflow_doc_meta_b/_search" {
					t.Errorf("search path = %s, want both tenant indices once", r.URL.Path)
				}
				if r.URL.Query().Get("allow_partial_search_results") != "false" {
					t.Error("partial tenant search results must trigger fallback")
				}
				fmt.Fprintf(w, `{"hits":{"total":{"value":%d,"relation":"eq"},"hits":[{"_id":"doc-a"},{"_id":"doc-b"}]}}`, tc.total)
			}))
			defer srv.Close()
			kbIDs := []string{"kb-a", "kb-b", "kb-c", "kb-a"}
			if tc.missingKB {
				kbIDs = append(kbIDs, "missing")
			}
			got := newTestEngine(t, srv.URL).FilterDocIdsByMetaPushdown(t.Context(), db, kbIDs, conditions, "and")
			if tc.missingIndex || tc.missingKB || tc.total > metaPushdownMaxSize {
				if got != nil {
					t.Fatalf("partial result = %v, want fallback", got)
				}
			} else if !reflect.DeepEqual(got, []string{"doc-a", "doc-b"}) {
				t.Fatalf("doc IDs = %v, want both tenants' documents", got)
			}
			if (tc.missingIndex || tc.missingKB) && searches != 0 {
				t.Fatal("searched despite incomplete tenant scope")
			}
		})
	}
}
