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

package service

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/entity"
	"ragflow/internal/server/config"
)

// realMetadataESEngine connects to the Elasticsearch the integration tier runs
// against; set ES_TEST=1 to run.
func realMetadataESEngine(t *testing.T) *elasticsearch.Engine {
	t.Helper()
	if common.GetEnv(common.EnvESTest) != "1" {
		t.Skip("Skipping ES integration test; set ES_TEST=1 to run")
	}
	if err := common.InitLogger("error", common.FileOutput{}, "metadata_value_space_integration_test"); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	cfg := config.ElasticsearchConfig{Hosts: "http://localhost:1200", Username: "elastic", Password: "infini_rag_flow"}
	if host := common.GetEnv(common.EnvESHost); host != "" {
		cfg.Hosts = host
	}
	if user := common.GetEnv(common.EnvESUsername); user != "" {
		cfg.Username = user
	}
	if password := common.GetEnv(common.EnvESPassword); password != "" {
		cfg.Password = password
	}
	eng, err := elasticsearch.NewEngine(t.Context(), cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return eng
}

// TestGetMetaValueSpaceByKBs_MatchesTheScanOnRealIndex writes metadata the way
// the metadata service does and checks that the aggregated value space is
// exactly the flattened scan's for the same rows. The values are chosen where
// doc values and _source disagree: an integer, a float stored as float32, a
// date-looking string, a string past ignore_above, a nested object, a number
// written as a string, blanks and booleans.
func TestGetMetaValueSpaceByKBs_MatchesTheScanOnRealIndex(t *testing.T) {
	eng := realMetadataESEngine(t)
	ctx := t.Context()
	tenantID := fmt.Sprintf("value_space_test_%d", time.Now().UnixNano())
	if err := eng.CreateMetadataStore(ctx, tenantID); err != nil {
		t.Fatalf("CreateMetadataStore: %v", err)
	}
	t.Cleanup(func() { _ = eng.DropMetadataStore(context.Background(), tenantID) })

	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	for _, kbID := range []string{"kb-1", "kb-2"} {
		if err := db.Create(&entity.Knowledgebase{ID: kbID, TenantID: tenantID, Name: kbID, CreatedBy: tenantID, EmbdID: "embd"}).Error; err != nil {
			t.Fatalf("insert kb: %v", err)
		}
	}

	long := strings.Repeat("x", 300)
	rows := []map[string]interface{}{
		{"id": "doc-1", "kb_id": "kb-1", "meta_fields": map[string]interface{}{
			"price": 1234567, "score": 0.1, "ts": "2026-07-23T10:00:00+02:00", "author": "Mario",
			"tags": []interface{}{"red", "blue"}, "summary": long, "draft": true, "blank": "",
		}},
		{"id": "doc-2", "kb_id": "kb-1", "meta_fields": map[string]interface{}{
			"price": "1234567", "score": 0.25, "author": []interface{}{"Mario", "Luigi"}, "nested": map[string]interface{}{"x": 1},
		}},
		{"id": "doc-3", "kb_id": "kb-1", "meta_fields": map[string]interface{}{"big": 9007199254740993, "tiny": 1e-7}},
		{"id": "doc-4", "kb_id": "kb-1", "meta_fields": map[string]interface{}{}},
		{"id": "doc-5", "kb_id": "kb-2", "meta_fields": map[string]interface{}{"author": "Peach"}},
	}
	// One row per request: rows of one bulk land on different shards in
	// parallel, and two of them mapping the same new key differently (price as
	// a number and as a string) race, so ES may reject one. InsertMetadata only
	// logs a rejected item, hence the count below.
	for _, row := range rows {
		if _, err := eng.InsertMetadata(ctx, []map[string]interface{}{row}, tenantID); err != nil {
			t.Fatalf("InsertMetadata(%s): %v", row["id"], err)
		}
	}
	stored, err := NewMetadataServiceForTest(nil, eng).SearchMetadataByKBs(ctx, []string{"kb-1", "kb-2"}, 100)
	if err != nil {
		t.Fatalf("SearchMetadataByKBs: %v", err)
	}
	if len(stored.MetadataRecords) != len(rows) {
		t.Fatalf("%d of %d rows were indexed", len(stored.MetadataRecords), len(rows))
	}

	svc := NewMetadataServiceForTest(nil, eng)
	for _, kbIDs := range [][]string{{"kb-1"}, {"kb-1", "kb-2"}} {
		scanned, err := svc.GetFlattedMetaByKBs(ctx, kbIDs)
		if err != nil {
			t.Fatalf("GetFlattedMetaByKBs(%v): %v", kbIDs, err)
		}
		want := scanned.ValueSpace()
		for key, values := range want {
			if len(values) == 0 {
				delete(want, key)
			}
		}

		space, err := svc.GetMetaValueSpaceByKBs(ctx, kbIDs)
		if err != nil {
			t.Fatalf("GetMetaValueSpaceByKBs(%v): %v", kbIDs, err)
		}
		if !reflect.DeepEqual(space, want) {
			t.Errorf("kb %v: aggregated value space\n%v\nwant the scan's\n%v", kbIDs, space, want)
		}
	}

	// The values doc values would have rendered differently.
	space, err := svc.GetMetaValueSpaceByKBs(ctx, []string{"kb-1"})
	if err != nil {
		t.Fatalf("GetMetaValueSpaceByKBs: %v", err)
	}
	for key, value := range map[string]string{
		"price":   "1234567",
		"score":   "0.1",
		"ts":      "2026-07-23T10:00:00+02:00",
		"summary": long,
		"big":     "9007199254740992",
		"tiny":    "0.0000001",
	} {
		found := false
		for _, v := range space[key] {
			found = found || v == value
		}
		if !found {
			t.Errorf("value space[%s] = %v, missing %q", key, space[key], value)
		}
	}
}
