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

package opengauss

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"
)

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func TestOpenGaussStorageRoundTrip(t *testing.T) {
	host := os.Getenv("RAGFLOW_TEST_OPENGAUSS_HOST")
	if host == "" {
		t.Skip("RAGFLOW_TEST_OPENGAUSS_HOST is not set")
	}
	port, err := strconv.Atoi(envOr("RAGFLOW_TEST_OPENGAUSS_PORT", "5432"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(config.OpenGaussConfig{
		Host: host, Port: port, User: os.Getenv("RAGFLOW_TEST_OPENGAUSS_USER"),
		Password: os.Getenv("RAGFLOW_TEST_OPENGAUSS_PASSWORD"),
		Database: envOr("RAGFLOW_TEST_OPENGAUSS_DATABASE", "postgres"),
		Schema:   envOr("RAGFLOW_TEST_OPENGAUSS_SCHEMA", "public"),
		SSLMode:  envOr("RAGFLOW_TEST_OPENGAUSS_SSL_MODE", "disable"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	testStorageRoundTrip(t, engine)
	t.Run("report limits and authorized rows", func(t *testing.T) {
		ctx := t.Context()
		rows, err := engine.RunSQL(ctx, "SELECT\n i AS \"limit\", 'a;b' AS label FROM generate_series(1,1030) i", "")
		if err != nil || len(rows) != 1024 {
			t.Fatalf("default outer limit: %d, %v", len(rows), err)
		}
		table := "ragflow_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		cleanupTestStore(t, func(ctx context.Context) error { return engine.DropChunkStore(ctx, table, "") })
		for _, kb := range []string{"east", "west"} {
			if _, err := engine.InsertChunks(ctx, []map[string]any{{"id": "row"}}, table, kb, ""); err != nil {
				t.Fatal(err)
			}
		}
		query, ok := ScopeChatSQL("select kb_id, count(*) as n from "+table+" group by kb_id", map[string][]string{table: {"east"}})
		if !ok {
			t.Fatal("authorized report rejected")
		}
		rows, err = engine.RunSQL(ctx, query, "")
		if err != nil || len(rows) != 1 || rows[0]["kb_id"] != "east" || rows[0]["n"] != int64(1) {
			t.Fatalf("authorized count: %v, %v", rows, err)
		}
	})
	t.Run("multi-table sort with default projection", func(t *testing.T) {
		ctx := t.Context()
		prefix := "og_order_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		tables := []string{prefix + "_a", prefix + "_b"}
		for i, table := range tables {
			cleanupTestStore(t, func(ctx context.Context) error { return engine.DropChunkStore(ctx, table, "") })
			row := map[string]any{"id": strconv.Itoa(i), "doc_id": []string{"z", "a"}[i], "name": "dynamic"}
			if i == 1 {
				row["page_num_int"] = []int{2}
				row["edition"] = "revised"
			}
			if _, err := engine.InsertChunks(ctx, []map[string]any{row}, table, "kb", ""); err != nil {
				t.Fatal(err)
			}
		}
		for _, tc := range []struct {
			order *types.OrderByExpr
			id    string
		}{
			{new(types.OrderByExpr).Asc("doc_id"), "1"}, {new(types.OrderByExpr).Asc("page_num_int"), "1"}, {new(types.OrderByExpr).Desc("page_num_int"), "0"},
			{new(types.OrderByExpr).Asc("edition"), "1"}, {new(types.OrderByExpr).Desc("edition"), "0"},
		} {
			res, err := engine.Search(ctx, &types.SearchRequest{IndexNames: tables, KbIDs: []string{"kb"}, OrderBy: tc.order, Limit: 1})
			if err != nil || len(res.Chunks) != 1 || len(res.Chunks[0]) != 1 || res.Chunks[0]["id"] != tc.id {
				t.Fatalf("ordered page: %v, %v", res, err)
			}
		}
		for _, names := range [][]string{tables[:1], tables} {
			_, err := engine.Search(ctx, &types.SearchRequest{IndexNames: names, OrderBy: new(types.OrderByExpr).Asc("not_a_column")})
			if err == nil {
				t.Fatal("accepted sort field missing from every table")
			}
		}
		for i, names := range [][]string{{"ä", "z", "2"}, {"ö", "10"}} {
			for _, name := range names {
				if _, err := engine.InsertChunks(ctx, []map[string]any{{"id": name, "name": name}}, tables[i], "collation", ""); err != nil {
					t.Fatal(err)
				}
			}
		}
		for offset, want := range []string{"10", "2", "z", "ä", "ö"} {
			page, err := engine.Search(ctx, &types.SearchRequest{IndexNames: tables, KbIDs: []string{"collation"}, OrderBy: new(types.OrderByExpr).Asc("name"), Offset: offset, Limit: 1})
			if err != nil || len(page.Chunks) != 1 || page.Chunks[0]["id"] != want {
				t.Fatalf("collated page %d: %v, %v; want %s", offset, page, err, want)
			}
		}
	})
	t.Run("filter page under concurrent updates", func(t *testing.T) {
		ctx := t.Context()
		table := "og_snapshot_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		cleanupTestStore(t, func(ctx context.Context) error { return engine.DropChunkStore(ctx, table, "") })
		if _, err := engine.InsertChunks(ctx, []map[string]any{{"id": "only", "available_int": 1}}, table, "kb", ""); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			for range 80 {
				if _, err := engine.db.ExecContext(ctx, "UPDATE "+quoteIdent(table)+" SET available_int=1-available_int"); err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}()
		defer func() {
			if err := <-done; err != nil {
				t.Error(err)
			}
		}()
		req := &types.SearchRequest{IndexNames: []string{table}, KbIDs: []string{"kb"}, Filter: map[string]any{"available_int": 1}, Limit: 1}
		for range 30 {
			page, err := engine.Search(ctx, req)
			if err != nil || page.Total != int64(len(page.Chunks)) {
				t.Fatalf("inconsistent first page: %v, %v", page, err)
			}
		}
	})
	t.Run("skill identifiers and vector columns", func(t *testing.T) {
		ctx := t.Context()
		table := "skill_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		cleanupTestStore(t, func(ctx context.Context) error { return engine.DropChunkStore(ctx, table, "") })
		_, err := engine.BulkIndex(ctx, table, []any{map[string]any{"skill_id": "guide/setup", "q_2_vec": []float64{1, 0}}, map[string]any{"skill_id": "guide_setup", "q_3_vec": []float64{0, 1, 0}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := engine.DeleteDocument(ctx, table, "guide_setup"); err != nil {
			t.Fatal(err)
		}
		if row, err := engine.GetChunk(ctx, table, "guide/setup", nil); err != nil || row == nil {
			t.Fatalf("exact skill deletion lost sibling: %v", err)
		}
		if row, err := engine.GetChunk(ctx, table, "guide_setup", nil); err != nil || row != nil {
			t.Fatalf("deleted skill lookup: %v, %v", row, err)
		}
	})
	t.Run("metadata key edits", func(t *testing.T) {
		ctx := t.Context()
		tenant := strconv.FormatInt(time.Now().UnixNano(), 10)
		cleanupTestStore(t, func(ctx context.Context) error { return engine.DropMetadataStore(ctx, tenant) })
		_, err := engine.InsertMetadata(ctx, []map[string]any{{"id": "r", "kb_id": "kb", "meta_fields": `{"a":1,"b":2,"keep":9007199254740993}`}}, tenant)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 2)
		for _, key := range []string{"a", "b"} {
			go func() { done <- engine.DeleteMetadataKeys(ctx, "r", "kb", []string{key}, tenant) }()
		}
		for range 2 {
			if err := <-done; err != nil {
				t.Error(err)
			}
		}
		var raw string
		if err := engine.db.QueryRowContext(ctx, "SELECT meta_fields FROM "+quoteIdent(metadataTableName(tenant))+" WHERE id='r'").Scan(&raw); err != nil || raw != `{"keep":9007199254740993}` {
			t.Fatalf("metadata after key edits: %s, %v", raw, err)
		}
	})
	t.Run("calendar and literal metadata filters", func(t *testing.T) {
		ctx := t.Context()
		table := "og_dates_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		cleanupTestStore(t, func(ctx context.Context) error { return engine.DropChunkStore(ctx, table, "") })
		_, err := engine.InsertChunks(ctx, []map[string]any{
			{"id": "good", "metadata": map[string]any{"day": "2024-02-29", "name": "a%_z"}},
			{"id": "invalid", "metadata": map[string]any{"day": "2025-02-29", "name": "abc"}},
		}, table, "kb", "")
		if err != nil {
			t.Fatal(err)
		}
		for _, condition := range []map[string]any{{"name": "day", "comparison_operator": "after", "value": "2024-02-28"}, {"name": "name", "comparison_operator": "start with", "value": "a%_"}} {
			result, err := engine.Search(ctx, &types.SearchRequest{IndexNames: []string{table}, KbIDs: []string{"kb"}, Filter: map[string]any{"metadata_filtering_conditions": map[string]any{"conditions": []any{condition}}}})
			if err != nil || len(result.Chunks) != 1 || result.Chunks[0]["id"] != "good" {
				t.Fatalf("metadata filter: %v, %v", result, err)
			}
		}
	})
	t.Run("dataset isolation and paging", func(t *testing.T) {
		ctx := t.Context()
		table := "og_scope_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		cleanupTestStore(t, func(ctx context.Context) error { return engine.DropChunkStore(ctx, table, "") })
		rows, err := engine.RunSQL(ctx, "SELECT current_schema() AS schema", "")
		if err != nil || len(rows) != 1 || rows[0]["schema"] != envOr("RAGFLOW_TEST_OPENGAUSS_SCHEMA", "public") {
			t.Fatalf("schema: %v %v", rows, err)
		}
		for _, kb := range []string{"b", "a"} {
			rows := []map[string]interface{}{
				{"id": "same", "doc_id": "doc", "content_ltks": "common token", "q_2_vec": []float64{1, 0}, "source_id": []string{"source"}},
				{"id": "second", "doc_id": "doc", "content_ltks": "common token", "q_2_vec": []float64{0, 1}},
			}
			if _, err := engine.InsertChunks(ctx, rows, table, kb, ""); err != nil {
				t.Fatal(err)
			}
		}
		// All four rows have the same doc_id and NULL page_num_int; IDs repeat across datasets.
		for _, order := range []*types.OrderByExpr{nil, new(types.OrderByExpr).Desc("page_num_int").Asc("doc_id")} {
			req := &types.SearchRequest{IndexNames: []string{table}, SelectFields: []string{"id", "kb_id"}, OrderBy: order, Limit: 1}
			for offset, want := range []string{"same/a", "same/b", "second/a", "second/b"} {
				req.Offset = offset
				page, err := engine.Search(ctx, req)
				if err != nil || page.Total != 4 || len(page.Chunks) != 1 {
					t.Fatalf("filter page %d: %v, %v", offset, page, err)
				}
				row := page.Chunks[0]
				if got := fmt.Sprintf("%s/%s", row["id"], row["kb_id"]); got != want {
					t.Fatalf("filter page %d: got %s, want %s", offset, got, want)
				}
			}
		}
		if _, err := engine.InsertChunks(ctx, []map[string]interface{}{{"id": "same", "doc_id": "doc", "content_ltks": "changed", "q_3_vec": []float64{1, 0, 0}}}, table, "a", ""); err != nil {
			t.Fatal(err)
		}
		old, err := engine.GetChunk(ctx, table, "same", []string{"b"})
		if err != nil || old == nil || old.(map[string]interface{})["content_ltks"] != "common token" {
			t.Fatalf("sibling chunk: %v %v", old, err)
		}
		req := &types.SearchRequest{IndexNames: []string{table}, KbIDs: []string{"b"}, SelectFields: []string{"id"}, Limit: 1, Offset: 2, MatchExprs: []interface{}{&types.MatchTextExpr{MatchingText: "common"}}}
		page, err := engine.Search(ctx, req)
		if err != nil || page.Total != 2 || len(page.Chunks) != 0 {
			t.Fatalf("BM25 empty page: %v %v", page, err)
		}
		req.Limit = 1
		req.Offset = 1
		dense := &types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{1, 0}, TopN: 10}
		for _, matches := range [][]interface{}{{dense}, {&types.MatchTextExpr{MatchingText: "common", TopN: 10}, dense, &types.FusionExpr{Method: "weighted_sum", TopN: 10}}} {
			req.MatchExprs = matches
			page, err := engine.Search(ctx, req)
			if err != nil || page.Total != 2 || len(page.Chunks) != 1 {
				t.Fatalf("ranked page: %v %v", page, err)
			}
		}
		req.Offset = 0
		req.Limit = 10
		req.KbIDs = []string{"a"}
		req.MatchExprs = []interface{}{&types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{1, 0}, TopN: 10}}
		hits, err := engine.Search(ctx, req)
		if err != nil || len(hits.Chunks) != 1 || hits.Chunks[0]["id"] != "second" {
			t.Fatalf("stale vector: %v %v", hits, err)
		}
		for _, edit := range []map[string]any{{"kb_id": "a"}, {"remove": "kb_id"}} {
			if err := engine.UpdateChunks(ctx, map[string]any{"id": "same"}, edit, table, "b"); err == nil {
				t.Fatal("dataset field was editable")
			}
		}
		edit := map[string]interface{}{"remove": map[string]interface{}{"source_id": "source"}, "add": map[string]interface{}{"source_id": "new"}}
		for i := 0; i < 2; i++ {
			if err := engine.UpdateChunks(ctx, map[string]interface{}{"id": "same"}, edit, table, "b"); err != nil {
				t.Fatal(err)
			}
		}
		row, err := engine.GetChunk(ctx, table, "same", []string{"b"})
		if err != nil || !reflect.DeepEqual(row.(map[string]interface{})["source_id"], []string{"new"}) {
			t.Fatalf("keyword edit: %v %v", row, err)
		}
		for _, field := range []string{"doc_id", "doc_ids", "id"} {
			n, err := engine.DeleteChunks(ctx, map[string]interface{}{field: []string{}}, table, "b")
			if err != nil || n != 0 {
				t.Fatalf("empty %s delete: n=%d err=%v", field, n, err)
			}
		}
		n, err := engine.DeleteChunks(ctx, map[string]interface{}{"exists": "source_id"}, table, "b")
		if err != nil || n != 1 {
			t.Fatalf("source_id exists delete: n=%d err=%v", n, err)
		}
	})
	t.Run("text fields, replacement and candidate windows", func(t *testing.T) {
		ctx := t.Context()
		table := "og_candidates_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		cleanupTestStore(t, func(ctx context.Context) error { return engine.DropChunkStore(ctx, table, "") })
		insert := func(docs ...map[string]interface{}) {
			t.Helper()
			if _, err := engine.InsertChunks(ctx, docs, table, "kb", ""); err != nil {
				t.Fatal(err)
			}
		}
		req := &types.SearchRequest{IndexNames: []string{table}, KbIDs: []string{"kb"}, SelectFields: []string{"id", "source_chunk_ids", "tag_feas"}, Limit: 10}
		search := func(total int64, ids ...string) {
			t.Helper()
			res, err := engine.Search(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			got := engine.GetChunkIDs(res.Chunks)
			if res.Total != total || !slices.Equal(got, ids) {
				t.Fatalf("total %d, IDs %v; want %d, %v", res.Total, got, total, ids)
			}
		}
		title := map[string]interface{}{"id": "title", "title_tks": "quartz", "content_ltks": "other", "source_chunk_ids": []string{"src", "longsrc"}, "source_doc_ids": []string{"doc"}, "tag_feas": map[string]float64{"red": .5}}
		body := map[string]interface{}{"id": "body", "content_ltks": "quartz"}
		insert(title, body)
		req.Filter = map[string]interface{}{"source_chunk_ids": "src"}
		search(1, "title")
		row, err := engine.GetChunk(ctx, table, "title", []string{"kb"})
		if err != nil {
			t.Fatal(err)
		}
		stored := row.(map[string]interface{})
		if !reflect.DeepEqual(stored["source_chunk_ids"], []string{"src", "longsrc"}) || !reflect.DeepEqual(stored["tag_feas"], map[string]interface{}{"red": .5}) {
			t.Fatalf("typed fields: %v", stored)
		}
		req.Filter = nil
		text := &types.MatchTextExpr{Fields: []string{"title_tks^10", "content_ltks"}, MatchingText: "quartz", TopN: 1}
		dense := &types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{1, 0}, TopN: 10}
		fusion := &types.FusionExpr{Method: "weighted_sum", TopN: 1}
		// Simulate a table created with only the original content index.
		if _, err := engine.db.ExecContext(ctx, "DROP INDEX "+quoteIdent(regularIndexName(table, "title_tks_bm25"))); err != nil {
			t.Fatal(err)
		}
		if ready, err := engine.ChunkStoreExists(ctx, table, "kb"); err != nil || ready {
			t.Fatalf("incomplete index set ready = %v, %v", ready, err)
		}
		req.MatchExprs = []interface{}{dense}
		search(0)
		req.MatchExprs = []interface{}{text, dense, fusion}
		search(1, "title")
		req.MatchExprs = []interface{}{text}
		search(1, "title")
		fusion.TopN = 3
		fusion.FusionParams = map[string]any{"weights": "1,0"}
		req.MatchExprs = []any{text, dense, fusion}
		search(1, "title")
		req.Offset = 1
		search(1)
		fusion.TopN, fusion.FusionParams = 1, nil
		req.Offset = 0
		body["q_2_vec"] = []float64{1, 0}
		body["pagerank_fea"] = 2
		insert(body)
		req.MatchExprs = []interface{}{text, dense, fusion}
		req.Limit = 1
		search(1, "body")
		req.Offset = 1
		search(1)
		delete(title, "source_chunk_ids")
		delete(title, "tag_feas")
		insert(title)
		req.MatchExprs, req.Offset = nil, 0
		req.Filter = map[string]interface{}{"source_chunk_ids": "src"}
		search(0)
		req.Limit = 10
		req.OrderBy = (&types.OrderByExpr{}).Asc("id")
		req.Filter = map[string]any{"and": []any{}}
		search(2, "body", "title")
		req.Filter = map[string]any{"doc_id": []string{}}
		search(0)
		req.Filter = nil

		// Production recall uses a small positive lexical weight and a full dense weight.
		// Both strong lexical matches must reach the caller's term/vector reranker.
		delete(body, "pagerank_fea")
		body["q_2_vec"] = []float64{0.1, 0.995}
		insert(body)
		insert(map[string]any{"id": "neighbor", "q_2_vec": []float64{1, 0}, "content_ltks": "unrelated"})
		dense.ExtraOptions = map[string]any{"similarity": 0.2}
		text.TopN, fusion.TopN, req.Limit = 10, 10, 10
		fusion.FusionParams = map[string]any{"weights": "0.001,1"}
		req.MatchExprs = []any{text, dense, fusion}
		res, err := engine.Search(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		ids := engine.GetChunkIDs(res.Chunks)
		slices.Sort(ids)
		if !slices.Equal(ids, []string{"body", "neighbor", "title"}) {
			t.Fatalf("hybrid candidates = %v", ids)
		}

		fusion.FusionParams = map[string]any{"weights": "0,1"}
		// A text-only candidate with this pagerank would beat the dense hit if admitted.
		title["pagerank_fea"] = 3
		insert(title)
		fusion.TopN, req.Offset = 1, 0
		search(1, "neighbor")
		req.Offset = 1
		search(1)
		fusion.FusionParams = map[string]any{"weights": "0.001,1"}
		req.MatchExprs = []any{dense}
		dense.ExtraOptions = nil
		dense.TopN, req.Offset, req.Limit = 0, 1, 1
		search(2, "body")
		dense.TopN = 1
		search(1)

		// Both legs prefer "first", but the final rank puts "next" first.
		insert(map[string]any{"id": "first", "doc_id": "paging", "q_2_vec": []float64{1, 0}, "title_tks": "quartz"})
		insert(map[string]any{"id": "next", "doc_id": "paging", "q_2_vec": []float64{0.5, 0.866}, "content_ltks": "quartz", "pagerank_fea": 2})
		req.Filter = map[string]any{"doc_id": "paging"}
		text.TopN, dense.TopN, fusion.TopN = 0, 0, 0
		for _, matches := range [][]any{{dense}, {text, dense, fusion}} {
			req.MatchExprs, req.Offset = matches, 0
			search(2, "next")
			req.Offset = 1
			search(2, "first")
		}
		text.TopN, dense.TopN = 1, 1
		req.Offset = 0
		search(1, "first")
		req.Offset = 1
		search(1)
		for _, kb := range []string{"z", "a"} {
			if _, err := engine.InsertChunks(ctx, []map[string]any{
				{"id": "z", "q_2_vec": []float64{1, 0}}, {"id": "a", "q_2_vec": []float64{1, 0}},
			}, table, kb, ""); err != nil {
				t.Fatal(err)
			}
		}
		req.KbIDs, req.Filter, req.SelectFields = []string{"z", "a"}, nil, []string{"id", "kb_id"}
		req.Offset = 0
		for _, matches := range [][]any{{dense}, {text, dense, fusion}} {
			req.MatchExprs = matches
			page, err := engine.Search(ctx, req)
			if err != nil || page.Total != 1 || len(page.Chunks) != 1 || page.Chunks[0]["id"] != "a" || page.Chunks[0]["kb_id"] != "a" {
				t.Fatalf("tied candidate cutoff: %v, %v", page, err)
			}
		}
	})
	t.Run("keyword boundaries", func(t *testing.T) {
		ctx := t.Context()
		table := "og_keywords_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		cleanupTestStore(t, func(ctx context.Context) error { return engine.DropChunkStore(ctx, table, "") })
		values := map[string][]string{
			"left": {"C#", "Go"}, "right": {"C", "#Go"}, "plain": {"C", "Go"},
			"long":    {strings.Repeat("#", 254)},
			"escaped": {`###`, `x,"\y}`, "百分%_号"},
		}
		for id, words := range values {
			if _, err := engine.InsertChunks(ctx, []map[string]any{{"id": id, "tag_kwd": words}}, table, "kb", ""); err != nil {
				t.Fatal(err)
			}
		}
		check := func(id string, want []string) {
			t.Helper()
			row, err := engine.GetChunk(ctx, table, id, []string{"kb"})
			if err != nil || row == nil {
				t.Fatalf("get %s: %v", id, err)
			}
			if got := row.(map[string]any)["tag_kwd"]; !reflect.DeepEqual(got, want) {
				t.Fatalf("%s got %#v, want %#v", id, got, want)
			}
		}
		for id, words := range values {
			check(id, words)
		}
		for term, id := range map[string]string{"C#": "left", "#Go": "right", "###": "escaped", "百分%_号": "escaped"} {
			res, err := engine.Search(ctx, &types.SearchRequest{IndexNames: []string{table}, KbIDs: []string{"kb"}, Filter: map[string]any{"tag_kwd": term}})
			if err != nil || !slices.Equal(engine.GetChunkIDs(res.Chunks), []string{id}) {
				t.Fatalf("filter %q: %v, %v", term, res, err)
			}
		}
		update := func(id string, v map[string]any) {
			t.Helper()
			if err := engine.UpdateChunks(ctx, map[string]any{"id": id}, v, table, "kb"); err != nil {
				t.Fatal(err)
			}
		}
		for _, id := range []string{"plain", "left"} {
			update(id, map[string]any{"tag_kwd": []string{"C", "Go"}})
			edit := map[string]any{"remove": map[string]any{"tag_kwd": "C"}, "add": map[string]any{"tag_kwd": "C#"}}
			update(id, edit)
			update(id, edit)
			check(id, []string{"Go", "C#"})
			update(id, map[string]any{"remove": map[string]any{"tag_kwd": "C#"}})
			check(id, []string{"Go"})
		}
		long := strings.Repeat("k", 254)
		update("left", map[string]any{"tag_kwd": []string{long}})
		update("left", map[string]any{"add": map[string]any{"tag_kwd": long}})
		check("left", []string{long})
		update("left", map[string]any{"tag_kwd": "C###Go"})
		check("left", []string{"C###Go"})
		update("left", map[string]any{"remove": map[string]any{"tag_kwd": "C###Go"}})
		res, err := engine.Search(ctx, &types.SearchRequest{IndexNames: []string{table}, KbIDs: []string{"kb"}, Filter: map[string]any{"id": "left", "exists": "tag_kwd"}})
		if err != nil || len(res.Chunks) != 0 {
			t.Fatalf("exists after removing the last keyword: %v, %v", res, err)
		}
	})
}

func cleanupTestStore(t *testing.T, drop func(context.Context) error) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := drop(ctx); err != nil {
			t.Errorf("drop test store: %v", err)
		}
	})
}

func testStorageRoundTrip(t *testing.T, engine *Engine) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	if err := engine.Ping(ctx); err != nil {
		t.Fatalf("ping %s: %v", engine.GetType(), err)
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	table := "og_it_" + suffix
	dataset := "kb-" + suffix
	cleanupTestStore(t, func(ctx context.Context) error { return engine.DropChunkStore(ctx, table, "") })

	// --- Chunk store: DDL, dynamic columns, storage-format round trip ---
	if err := engine.CreateChunkStore(ctx, table, dataset, 3, "naive", ""); err != nil {
		t.Fatal(err)
	}
	exists, err := engine.ChunkStoreExists(ctx, table, dataset)
	if err != nil || !exists {
		t.Fatalf("chunk store exists = %v err = %v", exists, err)
	}
	if _, err := engine.InsertChunks(ctx, []map[string]interface{}{
		{
			"id": "c1", "kb_id": dataset, "doc_id": "doc-1",
			"content_with_weight": "hello opengauss world", "content_ltks": "hello opengauss world",
			"important_kwd": []interface{}{"hello", "opengauss"},
			"metadata":      map[string]interface{}{"_group_id": "g1"},
			"position_int":  []interface{}{[]interface{}{int64(1), 2, 3, 4, 5}},
			"page_num_int":  []interface{}{int64(1)},
			"custom_field":  "dynamic column value",
			"q_3_vec":       []interface{}{0.1, 0.2, 0.3},
		},
		{
			"id": "c2", "kb_id": dataset, "doc_id": "doc-1",
			"content_with_weight": "second chunk", "content_ltks": "second chunk",
			"available_int": 1, "q_3_vec": []interface{}{0.9, 0.8, 0.7},
		},
	}, table, dataset, ""); err != nil {
		t.Fatal(err)
	}
	row, err := engine.GetChunk(ctx, table, "c1", []string{dataset})
	if err != nil {
		t.Fatal(err)
	}
	chunk := row.(map[string]interface{})
	if !reflect.DeepEqual(chunk["important_kwd"], []string{"hello", "opengauss"}) {
		t.Fatalf("keyword round trip = %#v", chunk["important_kwd"])
	}
	positions, ok := chunk["position_int"].([][]int64)
	if !ok || len(positions) != 1 || len(positions[0]) != 5 || positions[0][0] != 1 {
		t.Fatalf("position_int regroup = %#v", chunk["position_int"])
	}
	if vector, ok := chunk["q_3_vec"].([]float64); !ok || len(vector) != 3 {
		t.Fatalf("vector round trip = %#v", chunk["q_3_vec"])
	}
	if chunk["custom_field"] != "dynamic column value" {
		t.Fatalf("dynamic column round trip = %#v", chunk["custom_field"])
	}
	if groupID, _ := chunk["metadata"].(map[string]interface{}); groupID["_group_id"] != "g1" {
		t.Fatalf("metadata JSON round trip = %#v", chunk["metadata"])
	}

	// --- Four retrieval paths ---
	textResult, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{table}, KbIDs: []string{dataset}, Limit: 10,
		SelectFields: []string{"id", "content_ltks"},
		MatchExprs: []interface{}{&types.MatchTextExpr{
			Fields: []string{"content_ltks"}, MatchingText: "opengauss", TopN: 10,
		}},
	})
	if err != nil {
		t.Fatalf("fulltext search: %v", err)
	}
	if len(textResult.Chunks) != 1 || textResult.Chunks[0]["id"] != "c1" {
		t.Fatalf("fulltext hits = %#v", textResult.Chunks)
	}

	vectorResult, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{table}, KbIDs: []string{dataset}, Limit: 10,
		SelectFields: []string{"id"},
		MatchExprs: []interface{}{&types.MatchDenseExpr{
			VectorColumnName: "q_3_vec", EmbeddingData: []float64{0.1, 0.2, 0.3},
			EmbeddingDataType: "float", TopN: 10,
		}},
	})
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(vectorResult.Chunks) != 2 || vectorResult.Chunks[0]["id"] != "c1" {
		t.Fatalf("knn order = %#v", vectorResult.Chunks)
	}

	fusionResult, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{table}, KbIDs: []string{dataset}, Limit: 10,
		SelectFields: []string{"id"},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "opengauss", TopN: 10},
			&types.MatchDenseExpr{VectorColumnName: "q_3_vec", EmbeddingData: []float64{0.1, 0.2, 0.3},
				EmbeddingDataType: "float", TopN: 10},
			&types.FusionExpr{Method: "weighted_sum", TopN: 10,
				FusionParams: map[string]interface{}{"weights": "0.5,0.5"}},
		},
	})
	if err != nil {
		t.Fatalf("fusion search: %v", err)
	}
	if len(fusionResult.Chunks) != 2 || fusionResult.Chunks[0]["id"] != "c1" {
		t.Fatalf("fusion order = %#v", fusionResult.Chunks)
	}

	filterResult, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{table}, KbIDs: []string{dataset}, Limit: 10,
		SelectFields: []string{"id"}, Filter: map[string]interface{}{"doc_id": "doc-1"},
	})
	if err != nil {
		t.Fatalf("filter search: %v", err)
	}
	if filterResult.Total != 2 || len(filterResult.Chunks) != 2 {
		t.Fatalf("filter total = %d chunks = %#v", filterResult.Total, filterResult.Chunks)
	}

	// --- Update + pagerank + delete ---
	if err := engine.UpdateChunks(ctx, map[string]interface{}{"id": "c2"},
		map[string]interface{}{"important_kwd": []interface{}{"tagged"}}, table, dataset); err != nil {
		t.Fatal(err)
	}
	if err := engine.AdjustChunkPagerank(ctx, table, "c1", dataset, 1.5, 0, 10); err != nil {
		t.Fatal(err)
	}
	row, err = engine.GetChunk(ctx, table, "c1", []string{dataset})
	if err != nil {
		t.Fatal(err)
	}
	if pagerank, _ := common.ToFloat(row.(map[string]interface{})["pagerank_fea"]); pagerank != 1.5 {
		t.Fatalf("pagerank after adjust = %#v", row.(map[string]interface{})["pagerank_fea"])
	}
	deleted, err := engine.DeleteChunks(ctx, map[string]interface{}{"id": "c2"}, table, dataset)
	if err != nil || deleted != 1 {
		t.Fatalf("delete = %d err = %v", deleted, err)
	}

	// --- Memory store: tsquery fulltext + fusion ---
	memoryTable := "memory_og_it_" + suffix
	memoryID := "mem-" + suffix
	cleanupTestStore(t, func(ctx context.Context) error { return engine.DropChunkStore(ctx, memoryTable, "") })
	if err := engine.CreateChunkStore(ctx, memoryTable, memoryID, 0, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.InsertChunks(ctx, []map[string]interface{}{
		{"id": memoryID + "_1", "content": "remember the opengauss meeting", "status": true,
			"content_embed": []interface{}{0.25, 0.5}},
	}, memoryTable, memoryID, ""); err != nil {
		t.Fatal(err)
	}
	memoryText, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{memoryTable}, KbIDs: []string{memoryID}, Limit: 10,
		SelectFields: []string{"content", "status"},
		MatchExprs:   []interface{}{&types.MatchTextExpr{MatchingText: "opengauss meeting"}},
	})
	if err != nil {
		t.Fatalf("memory fulltext: %v", err)
	}
	if len(memoryText.Chunks) != 1 || memoryText.Chunks[0]["content"] != "remember the opengauss meeting" {
		t.Fatalf("memory fulltext hits = %#v", memoryText.Chunks)
	}
	memoryFusion, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{memoryTable}, KbIDs: []string{memoryID}, Limit: 10,
		SelectFields: []string{"content"},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "opengauss", TopN: 10},
			&types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{0.25, 0.5},
				EmbeddingDataType: "float", TopN: 10},
			&types.FusionExpr{Method: "weighted_sum", TopN: 10,
				FusionParams: map[string]interface{}{"weights": "0.5,0.5"}},
		},
	})
	if err != nil {
		t.Fatalf("memory fusion: %v", err)
	}
	if len(memoryFusion.Chunks) != 1 {
		t.Fatalf("memory fusion hits = %#v", memoryFusion.Chunks)
	}

	for _, edit := range []map[string]any{{"memory_id": "other"}, {"remove": "memory_id"}} {
		if err := engine.UpdateChunks(ctx, map[string]any{"id": memoryID + "_1"}, edit, memoryTable, memoryID); err == nil {
			t.Fatal("memory ownership was editable")
		}
	}
	if err := engine.UpdateChunks(ctx, map[string]any{"id": memoryID + "_1"}, map[string]any{"remove": "content"}, memoryTable, memoryID); err != nil {
		t.Fatal(err)
	}
	var contentLen, tokensLen int
	if err := engine.db.QueryRowContext(ctx, "SELECT coalesce(length(content_ltks),0), coalesce(length(tokenized_content_ltks),0) FROM "+quoteIdent(memoryTable)+" WHERE id=$1 AND memory_id=$2", memoryID+"_1", memoryID).Scan(&contentLen, &tokensLen); err != nil || contentLen != 0 || tokensLen != 0 {
		t.Fatalf("reset memory row: content length %d, token length %d, error %v", contentLen, tokensLen, err)
	}

	// --- Metadata store CRUD ---
	tenant := "it" + suffix
	cleanupTestStore(t, func(ctx context.Context) error { return engine.DropMetadataStore(ctx, tenant) })
	if err := engine.CreateMetadataStore(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	index := regularIndexName(metadataTableName(tenant), "kb_id")
	if _, err := engine.db.ExecContext(ctx, "DROP INDEX "+quoteIdent(index)); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.InsertMetadata(ctx, []map[string]interface{}{
		{"id": "md-1", "kb_id": dataset, "doc_id": "doc-1",
			"meta_fields": map[string]interface{}{"year": 2026, "author": "it"}},
	}, tenant); err != nil {
		t.Fatal(err)
	}
	if ok, err := engine.indexExists(ctx, metadataTableName(tenant), index); err != nil || !ok {
		t.Fatalf("metadata index missing after retry: %v", err)
	}
	metaResult, err := engine.SearchMetadata(ctx, &types.SearchMetadataRequest{
		TenantID: tenant, Limit: 10, Filter: map[string]interface{}{"kb_id": dataset},
	})
	if err != nil {
		t.Fatalf("metadata search: %v", err)
	}
	if metaResult.Total != 1 || len(metaResult.MetadataRecords) != 1 {
		t.Fatalf("metadata search result = %#v", metaResult)
	}
	if err := engine.UpdateMetadata(ctx, "md-1", dataset,
		map[string]interface{}{"year": 2026, "author": "updated"}, tenant); err != nil {
		t.Fatal(err)
	}
	metaResult, err = engine.SearchMetadata(ctx, &types.SearchMetadataRequest{
		TenantID: tenant, Limit: 10, Filter: map[string]interface{}{"doc_id": "doc-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := metaResult.MetadataRecords[0]["meta_fields"].(map[string]interface{})
	if fields["author"] != "updated" {
		t.Fatalf("metadata update = %#v", metaResult.MetadataRecords[0])
	}
	if _, err := engine.DeleteMetadata(ctx, map[string]interface{}{"id": "md-1"}, tenant); err != nil {
		t.Fatal(err)
	}

	// --- Drop deletes dataset rows but keeps the shared table ---
	remaining, err := engine.InsertChunks(ctx, []map[string]interface{}{
		{"id": "c3", "kb_id": dataset, "content_ltks": "third"},
	}, table, dataset, "")
	if err != nil {
		t.Fatal(err)
	}
	_ = remaining
	if _, err := engine.InsertChunks(ctx, []map[string]any{{"id": "sibling"}}, table, "other", ""); err != nil {
		t.Fatal(err)
	}
	if err := engine.DropChunkStore(ctx, table, dataset); err != nil {
		t.Fatal(err)
	}
	filterResult, err = engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{table}, KbIDs: []string{dataset}, Limit: 10,
		SelectFields: []string{"id"},
		MatchExprs:   []interface{}{&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "third"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ready, err := engine.ChunkStoreExists(ctx, table, "other"); err != nil || !ready {
		t.Fatalf("shared store after drop: %v, %v", ready, err)
	}
	if row, err := engine.GetChunk(ctx, table, "sibling", []string{"other"}); err != nil || row == nil {
		t.Fatalf("sibling after drop: %v, %v", row, err)
	}
	if len(filterResult.Chunks) != 0 {
		t.Fatalf("rows survived dataset drop: %#v", filterResult.Chunks)
	}
}
