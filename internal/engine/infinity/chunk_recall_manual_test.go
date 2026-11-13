//go:build manual

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

package infinity

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"

	sdk "github.com/infiniflow/infinity-go-sdk"
)

// TestDenseSearchRecall exercises issue #22161 against a real Infinity server:
// RAGFLOW_INFINITY_URI=localhost:23817 bash build.sh --test-manual ./internal/engine/infinity -run '^TestDenseSearchRecall$' -timeout 30m
// Self-query hit rate measures whether a stored vector finds its own row. It is
// not recall@10 against an exhaustive nearest-neighbor reference. HNSW graph
// construction can vary between server versions and runs despite the fixed data.
func TestDenseSearchRecall(t *testing.T) {
	uri := os.Getenv("RAGFLOW_INFINITY_URI")
	if uri == "" {
		t.Skip("set RAGFLOW_INFINITY_URI to run the real-service dense-search regression")
	}
	if err := common.InitLogger("error", common.FileOutput{}, "infinity_dense_recall_test"); err != nil {
		t.Fatal(err)
	}
	eng, err := NewEngine(t.Context(), config.InfinityConfig{URI: uri, DBName: "ragflow_ef_test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := eng.Close(); err != nil {
			t.Errorf("close engine: %v", err)
		}
	})

	const (
		rowCount    = 60000
		dimension   = 128
		batchSize   = 1000
		queryCount  = 200
		resultCount = 10
	)
	rng := rand.New(rand.NewSource(2026))
	vectors := make([][]float64, rowCount)
	vectorNorms := make([]float64, rowCount)
	for i := range vectors {
		vectors[i] = make([]float64, dimension)
		for j := range vectors[i] {
			vectors[i][j] = math.Round(rng.Float64()*1e6) / 1e6
			value := float64(float32(vectors[i][j]))
			vectorNorms[i] += value * value
		}
		vectorNorms[i] = math.Sqrt(vectorNorms[i])
	}
	queryRows := rng.Perm(rowCount)[:queryCount]
	boundaryRows := []int{0, 1, queryRows[0], queryRows[1]}
	// Establish the threshold expectation independently of either search path:
	// at float32 storage precision, only the query's own row can reach 0.99.
	for _, row := range boundaryRows {
		maxOtherCosine := 0.0
		for other := range vectors {
			if other == row {
				continue
			}
			dot := 0.0
			for j, value := range vectors[row] {
				dot += float64(float32(value)) * float64(float32(vectors[other][j]))
			}
			maxOtherCosine = max(maxOtherCosine, dot/(vectorNorms[row]*vectorNorms[other]))
		}
		if maxOtherCosine >= 0.99 {
			t.Fatalf("threshold fixture row=%d has another vector with cosine=%g, want <0.99", row, maxOtherCosine)
		}
		t.Logf("threshold fixture row=%d maximum other-row cosine=%g", row, maxOtherCosine)
	}

	for _, index := range []struct {
		name   string
		params map[string]string
	}{
		{"issue", map[string]string{"M": "16", "ef_construction": "200", "metric": "cosine"}},
		{"ragflow", map[string]string{"M": "16", "ef_construction": "50", "metric": "cosine", "encode": "lvq"}},
	} {
		t.Run(index.name, func(t *testing.T) {
			tableName := fmt.Sprintf("dense_recall_%s_%d", index.name, time.Now().UnixNano())
			func() {
				db, release, err := eng.client.checkoutDatabase(t.Context(), "dense_recall_setup")
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				table, err := db.CreateTable(tableName, sdk.TableSchema{
					&sdk.ColumnDefinition{Name: "id", DataType: "varchar"},
					&sdk.ColumnDefinition{Name: "v", DataType: "vector,128,float"},
					&sdk.ColumnDefinition{Name: "available_int", DataType: "integer"},
					&sdk.ColumnDefinition{Name: "content", DataType: "varchar"},
					// Search projects these fields even when SelectFields only asks for id.
					&sdk.ColumnDefinition{Name: common.PAGERANK_FLD, DataType: "integer"},
					&sdk.ColumnDefinition{Name: common.TAG_FLD, DataType: "varchar"},
				}, sdk.ConflictTypeError)
				if err != nil {
					t.Fatalf("create table: %v", err)
				}
				t.Cleanup(func() {
					// t.Context is canceled before cleanups; acquire a live connection
					// and retain it until DropTable completes.
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					db, release, err := eng.client.checkoutDatabase(ctx, "dense_recall_cleanup")
					if err != nil {
						t.Errorf("checkout cleanup database for %s: %v", tableName, err)
						return
					}
					defer release()
					if _, err := db.DropTable(tableName, sdk.ConflictTypeIgnore); err != nil {
						t.Errorf("drop table %s: %v", tableName, err)
					}
				})
				// Build before inserting to exercise the in-memory HNSW path from
				// the report, rather than building an index over persisted rows.
				if _, err := table.CreateIndex("v_hnsw", sdk.NewIndexInfo("v", sdk.IndexTypeHnsw, index.params), sdk.ConflictTypeError, ""); err != nil {
					t.Fatalf("create HNSW index: %v", err)
				}
				// The logical content_ltks search field maps to this storage column
				// and named index in convertMatchingField.
				if _, err := table.CreateIndex("ft_content_rag_coarse", sdk.NewIndexInfo("content", sdk.IndexTypeFullText, map[string]string{"ANALYZER": "standard"}), sdk.ConflictTypeError, ""); err != nil {
					t.Fatalf("create full-text index: %v", err)
				}
				started := time.Now()
				for start := 0; start < rowCount; start += batchSize {
					rows := make([]map[string]interface{}, 0, batchSize)
					for i := start; i < min(start+batchSize, rowCount); i++ {
						available := 1
						if i%4 == 0 {
							available = 0
						}
						content := "dense search regression"
						if i%3 == 0 {
							content = "unrelated"
						}
						rows = append(rows, map[string]interface{}{
							"id": strconv.Itoa(i), "v": vectors[i], "available_int": available,
							"content": content, common.PAGERANK_FLD: 0, common.TAG_FLD: "{}",
						})
					}
					if _, err := table.Insert(rows); err != nil {
						t.Fatalf("insert batch starting at %d: %v", start, err)
					}
				}
				t.Logf("inserted %d vectors, dimension=%d, HNSW=%v, elapsed=%s", rowCount, dimension, index.params, time.Since(started))
			}()

			searchSDK := func(row, topN, limit int, ef, filter, threshold string) ([]string, time.Duration) {
				t.Helper()
				db, release, err := eng.client.checkoutDatabase(t.Context(), "dense_recall_sdk")
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				table, err := db.GetTable(tableName)
				if err != nil {
					t.Fatal(err)
				}
				options := map[string]string{"threshold": threshold, "filter": filter}
				if ef != "" {
					options["ef"] = ef
				}
				started := time.Now()
				result, err := table.Output([]string{"id"}).MatchDense("v", vectors[row], "float", "cosine", topN, options).Limit(limit).ToResult()
				elapsed := time.Since(started)
				if err != nil {
					t.Fatalf("SDK row=%d topN=%d limit=%d ef=%q filter=%q threshold=%q: %v", row, topN, limit, ef, filter, threshold, err)
				}
				qr, ok := result.(*sdk.QueryResult)
				if !ok {
					t.Fatalf("SDK returned %T, want *sdk.QueryResult", result)
				}
				ids := make([]string, len(qr.Data["id"]))
				for i, value := range qr.Data["id"] {
					id, ok := value.(string)
					if !ok {
						t.Fatalf("SDK id=%#v, want string", value)
					}
					ids[i] = id
				}
				return ids, elapsed
			}
			searchEngine := func(row, topN, limit int, filter map[string]interface{}, threshold string) ([]string, time.Duration) {
				t.Helper()
				started := time.Now()
				result, err := eng.Search(t.Context(), &types.SearchRequest{
					IndexNames: []string{tableName}, SelectFields: []string{"id"}, IncludeUnavailable: true,
					Limit: limit, Filter: filter,
					MatchExprs: []interface{}{&types.MatchDenseExpr{
						VectorColumnName: "v", EmbeddingData: vectors[row], EmbeddingDataType: "float", DistanceType: "cosine",
						TopN: topN, ExtraOptions: map[string]interface{}{"threshold": threshold},
					}},
				})
				elapsed := time.Since(started)
				if err != nil {
					t.Fatalf("Engine.Search row=%d topN=%d limit=%d filter=%v threshold=%q: %v", row, topN, limit, filter, threshold, err)
				}
				ids := make([]string, len(result.Chunks))
				for i, chunk := range result.Chunks {
					id, ok := chunk["id"].(string)
					if !ok {
						t.Fatalf("engine id=%#v, want string", chunk["id"])
					}
					ids[i] = id
				}
				return ids, elapsed
			}
			assertIDs := func(label string, row int, got, want []string) {
				t.Helper()
				// Compare membership; equal cosine scores need not preserve order
				// across the SDK projection and Search's score sorting.
				got, want = slices.Clone(got), slices.Clone(want)
				slices.Sort(got)
				slices.Sort(want)
				if !slices.Equal(got, want) {
					t.Errorf("%s row=%d IDs=%v, want %v", label, row, got, want)
				}
			}

			var hits [4]int
			var elapsed [4]time.Duration
			for _, row := range queryRows {
				defaults, d0 := searchSDK(row, resultCount, resultCount, "", "", "0.0")
				ef10, d1 := searchSDK(row, resultCount, resultCount, "10", "", "0.0")
				ef200, d2 := searchSDK(row, resultCount, resultCount, "200", "", "0.0")
				actual, d3 := searchEngine(row, resultCount, resultCount, nil, "0.0")
				assertIDs("SDK default vs ef=10", row, defaults, ef10)
				assertIDs("Engine.Search vs SDK ef=200", row, actual, ef200)
				for i, ids := range [][]string{defaults, ef10, ef200, actual} {
					if len(ids) != resultCount {
						t.Errorf("query variant=%d row=%d returned %d IDs, want %d", i, row, len(ids), resultCount)
					}
					if slices.Contains(ids, strconv.Itoa(row)) {
						hits[i]++
					}
				}
				for i, duration := range []time.Duration{d0, d1, d2, d3} {
					elapsed[i] += duration
				}
			}
			for i, label := range []string{"SDK default", "SDK ef=10", "SDK ef=200", "Engine.Search"} {
				t.Logf("%s self-query hits=%d/%d (%.1f%%), query elapsed=%s, mean=%s", label, hits[i], queryCount, float64(hits[i])*100/queryCount, elapsed[i], elapsed[i]/queryCount)
			}
			if hits[2] <= hits[0] {
				t.Errorf("ef=200 self-query hits=%d must exceed default=%d to reproduce the reported HNSW issue; graph construction is variable", hits[2], hits[0])
			}

			for _, row := range boundaryRows {
				for _, test := range []struct {
					name      string
					topN      int
					limit     int
					sdkTopN   int
					ef        string
					filter    string
					threshold string
					wantCount int
				}{
					{"smaller_limit", 10, 3, 10, "200", "", "0.0", 3},
					{"topN_limits_results", 10, 25, 10, "200", "", "0.0", 10},
					{"above_ef_floor", 201, 10, 201, "201", "", "0.0", 10},
					{"topN_from_limit", 0, 201, 201, "201", "", "0.0", 201},
					{"large_topN", 400, 400, 400, "400", "", "0.0", 400},
					{"large_topN_small_limit", 400, 10, 400, "400", "", "0.0", 10},
					{"available_filter", 10, 10, 10, "200", "available_int=1", "0.0", 10},
					{"threshold", 10, 10, 10, "200", "", "0.99", -1},
					{"filter_and_threshold", 10, 10, 10, "200", "available_int=1", "0.99", -1},
				} {
					var filter map[string]interface{}
					if test.filter != "" {
						filter = map[string]interface{}{"available_int": 1}
					}
					want, _ := searchSDK(row, test.sdkTopN, test.limit, test.ef, test.filter, test.threshold)
					got, _ := searchEngine(row, test.topN, test.limit, filter, test.threshold)
					assertIDs(test.name, row, got, want)
					if test.wantCount >= 0 && len(got) != test.wantCount {
						t.Errorf("%s row=%d count=%d, want %d", test.name, row, len(got), test.wantCount)
					}
					if test.threshold == "0.99" {
						for _, id := range got {
							if id != strconv.Itoa(row) {
								t.Errorf("%s row=%d returned ID %q below the independently checked cosine threshold", test.name, row, id)
							}
						}
					}
					if test.filter != "" {
						for _, id := range got {
							value, err := strconv.Atoi(id)
							if err != nil || value%4 == 0 {
								t.Errorf("%s row=%d returned unavailable or invalid ID %q", test.name, row, id)
							}
						}
					}
				}
			}

			for _, row := range queryRows[:3] {
				for _, availableOnly := range []bool{false, true} {
					var filter map[string]interface{}
					textOptions := map[string]string{"minimum_should_match": "0%"}
					denseFilter := "filter_fulltext('content@ft_content_rag_coarse', 'dense')"
					if availableOnly {
						filter = map[string]interface{}{"available_int": 1}
						textOptions["filter"] = "available_int=1"
						denseFilter = "available_int=1 AND filter_fulltext('content@ft_content_rag_coarse', 'dense')"
					}
					// Build the reference query directly against the SDK's real
					// storage/index names, separately from Engine.Search requests.
					want := func() []string {
						db, release, err := eng.client.checkoutDatabase(t.Context(), "dense_recall_hybrid_sdk")
						if err != nil {
							t.Fatal(err)
						}
						defer release()
						table, err := db.GetTable(tableName)
						if err != nil {
							t.Fatal(err)
						}
						result, err := table.Output([]string{"id", "score()"}).
							MatchText("content@ft_content_rag_coarse", "dense", 10, textOptions).
							MatchDense("v", vectors[row], "float", "cosine", 10, map[string]string{
								"ef": "200", "threshold": "0.0", "filter": denseFilter,
							}).
							Fusion("weighted_sum", 10, map[string]interface{}{"weights": "0.5,0.5", "normalize": "atan"}).
							Limit(10).ToResult()
						if err != nil {
							t.Fatalf("SDK hybrid row=%d availableOnly=%t: %v", row, availableOnly, err)
						}
						qr, ok := result.(*sdk.QueryResult)
						if !ok {
							t.Fatalf("SDK hybrid returned %T, want *sdk.QueryResult", result)
						}
						ids := make([]string, len(qr.Data["id"]))
						for i, value := range qr.Data["id"] {
							id, ok := value.(string)
							if !ok {
								t.Fatalf("SDK hybrid id=%#v, want string", value)
							}
							ids[i] = id
						}
						return ids
					}()
					result, err := eng.Search(t.Context(), &types.SearchRequest{
						IndexNames: []string{tableName}, SelectFields: []string{"id"}, IncludeUnavailable: true,
						Limit: 10, Filter: filter,
						MatchExprs: []interface{}{
							&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "dense", TopN: 10},
							&types.MatchDenseExpr{VectorColumnName: "v", EmbeddingData: vectors[row], EmbeddingDataType: "float", DistanceType: "cosine", TopN: 10},
							&types.FusionExpr{Method: "weighted_sum", TopN: 10, FusionParams: map[string]interface{}{"weights": "0.5,0.5", "normalize": "atan"}},
						},
					})
					if err != nil {
						t.Fatalf("Engine.Search hybrid row=%d availableOnly=%t: %v", row, availableOnly, err)
					}
					got := make([]string, len(result.Chunks))
					for i, chunk := range result.Chunks {
						id, ok := chunk["id"].(string)
						if !ok {
							t.Fatalf("engine hybrid id=%#v, want string", chunk["id"])
						}
						got[i] = id
						value, err := strconv.Atoi(id)
						if err != nil || value%3 == 0 || (availableOnly && value%4 == 0) {
							t.Errorf("hybrid row=%d availableOnly=%t returned excluded ID %q", row, availableOnly, id)
						}
					}
					assertIDs(fmt.Sprintf("hybrid availableOnly=%t", availableOnly), row, got, want)
					if len(got) != 10 {
						t.Errorf("hybrid row=%d availableOnly=%t returned %d IDs, want 10", row, availableOnly, len(got))
					}
				}
			}
		})
	}
}
