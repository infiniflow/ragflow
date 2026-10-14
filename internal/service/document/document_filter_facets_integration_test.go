//go:build integration

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

package document

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/server/config"
	"ragflow/internal/service"
)

// TestGetDocumentMetadataFilter_AggregationMatchesTheScanOnRealIndex writes
// metadata the way the metadata service does and checks, on a real
// Elasticsearch, that the aggregated file-list filter is exactly what the scan
// of the same rows produces: the same labels -- which a selection is resolved
// against -- and the same counts. The values are the ones doc values render
// differently from _source: an integer, a float stored as float32, a
// date-looking string with an offset, a string past ignore_above, a number
// written as a string, blanks and booleans. Set ES_TEST=1 to run.
func TestGetDocumentMetadataFilter_AggregationMatchesTheScanOnRealIndex(t *testing.T) {
	if common.GetEnv(common.EnvESTest) != "1" {
		t.Skip("Skipping ES integration test; set ES_TEST=1 to run")
	}
	if err := common.InitLogger("error", common.FileOutput{}, "document_filter_facets_integration_test"); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	ctx := t.Context()
	cfg := config.ElasticsearchConfig{Hosts: "http://localhost:1200", Username: "elastic", Password: "infini_rag_flow"}
	if host := common.GetEnv(common.EnvESHost); host != "" {
		cfg.Hosts = host
	}
	eng, err := elasticsearch.NewEngine(ctx, cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	tenantID := fmt.Sprintf("facets_test_%d", time.Now().UnixNano())
	if err := eng.CreateMetadataStore(ctx, tenantID); err != nil {
		t.Fatalf("CreateMetadataStore: %v", err)
	}
	t.Cleanup(func() { _ = eng.DropMetadataStore(context.Background(), tenantID) })

	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.Create(&entity.Knowledgebase{ID: "kb-1", TenantID: tenantID, Name: "kb-1", CreatedBy: tenantID, EmbdID: "embd"}).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}

	rows := []map[string]interface{}{
		{"id": "doc-1", "kb_id": "kb-1", "meta_fields": map[string]interface{}{
			"price": 1234567, "score": 0.1, "ts": "2026-07-23T10:00:00+02:00", "author": "Mario",
			"tags": []interface{}{"red", "blue"}, "summary": strings.Repeat("x", 300), "draft": true, "blank": "",
		}},
		{"id": "doc-2", "kb_id": "kb-1", "meta_fields": map[string]interface{}{
			"price": "1234567", "score": 0.25, "author": []interface{}{"Mario", "Luigi"},
		}},
		{"id": "doc-3", "kb_id": "kb-1", "meta_fields": map[string]interface{}{"big": 9007199254740993, "tiny": 1e-7}},
		{"id": "doc-4", "kb_id": "kb-1", "meta_fields": map[string]interface{}{}},
		// Not in the listed documents: an orphaned row.
		{"id": "orphan", "kb_id": "kb-1", "meta_fields": map[string]interface{}{"author": "Zed"}},
	}
	// One row per request: rows of one bulk land on different shards in
	// parallel and race to map the same new key, so ES may reject one.
	for _, row := range rows {
		if _, err := eng.InsertMetadata(ctx, []map[string]interface{}{row}, tenantID); err != nil {
			t.Fatalf("InsertMetadata(%s): %v", row["id"], err)
		}
	}

	s := &DocumentService{metadataSvc: service.NewMetadataServiceForTest(nil, eng)}
	docIDs := []string{"doc-1", "doc-2", "doc-3", "doc-4", "doc-5"}

	scanned := func() map[string]interface{} {
		orig := metadataFacets
		metadataFacets = func(context.Context, *service.MetadataService, string, []string) ([]types.MetaValueCount, int64, bool, error) {
			return nil, 0, false, nil
		}
		defer func() { metadataFacets = orig }()
		got, err := s.getDocumentMetadataFilter(ctx, "kb-1", docIDs)
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		return got
	}()
	if counts, _ := scanned["author"].(map[string]int64); counts["Mario"] != 2 || scanned["ts"] == nil {
		t.Fatalf("the scan does not see every row (%v); the comparison would prove nothing", scanned)
	}

	if _, _, ok, err := s.metadataSvc.GetMetadataFacets(ctx, "kb-1", docIDs); !ok || err != nil {
		t.Fatalf("the aggregation path was not taken: ok=%v err=%v", ok, err)
	}
	aggregated, err := s.getDocumentMetadataFilter(ctx, "kb-1", docIDs)
	if err != nil {
		t.Fatalf("aggregation: %v", err)
	}
	if !reflect.DeepEqual(aggregated, scanned) {
		t.Errorf("aggregated filter\n%v\nwant the scan's\n%v", aggregated, scanned)
	}
}
