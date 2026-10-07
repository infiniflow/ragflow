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

package vastbase

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestMain initializes the shared logger: Search logs every request through
// common.LogSearchRequest, whose level probe dereferences a nil AtomicLevel
// until InitLogger runs.
func TestMain(m *testing.M) {
	_ = common.InitLogger("info", common.FileOutput{}, "vastbase_test")
	m.Run()
}

// chunkColumnRows is the standard chunk table column list shared by the
// search golden tests.
func chunkColumnRows() []string {
	return []string{"id", "kb_id", "content_ltks", "pagerank_fea", "q_3_vec"}
}

// expectChunkColumns stubs the listTableColumns round trip with the standard
// chunk column set.
func expectChunkColumns(mock sqlmock.Sqlmock, table string) {
	expectCount(mock, tableCountQuery(), table)
	rows := sqlmock.NewRows([]string{"column_name", "data_type", "column_default", "is_nullable"})
	for _, column := range chunkColumnRows() {
		rows.AddRow(column, "text", nil, "YES")
	}
	mock.ExpectQuery(regexp.QuoteMeta(listColumnsQuery)).WithArgs(table).WillReturnRows(rows)
}

// TestFullTextCTESingleColumnGolden pins the single-column bm25 CTE SQL.
func TestFullTextCTESingleColumnGolden(t *testing.T) {
	engine := &Engine{dbCompatibility: compatB}
	text := &types.MatchTextExpr{
		Fields:       []string{"content_ltks"},
		MatchingText: "hello",
		TopN:         100,
		ExtraOptions: map[string]interface{}{"minimum_should_match": 0.5},
	}
	args := &sqlArgs{}
	got, err := engine.fullTextCTE("t1", "chunk", []string{"id", "content_ltks"}, `"kb_id" = $1`,
		[]interface{}{"kb-1"}, text, 100, args)
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "id", "content_ltks", bm25_score AS _score FROM ` +
		`(SELECT "id", "content_ltks", bm25_score() AS bm25_score FROM "t1" ` +
		`WHERE ("kb_id" = $1) AND ("content_ltks" @~@ $2) ORDER BY bm25_score DESC LIMIT 100) AS ranked`
	if got != want {
		t.Fatalf("single-column CTE =\n%s", got)
	}
	operand := "hello @<PARAM:MINIMUM_SHOULD_MATCH=50% PARAM:BOOST=1>@"
	if !reflect.DeepEqual(args.values, []interface{}{"kb-1", operand}) {
		t.Fatalf("args = %#v", args.values)
	}
}

// TestFullTextCTEMultiColumnUnionGolden pins the UNION ALL form the CTE takes
// when several columns carry full-text indexes.
func TestFullTextCTEMultiColumnUnionGolden(t *testing.T) {
	engine := &Engine{dbCompatibility: compatB}
	text := &types.MatchTextExpr{
		Fields:       []string{"title_tks^2", "content_ltks"},
		MatchingText: "boosted~3 query^2",
		TopN:         50,
	}
	args := &sqlArgs{}
	got, err := engine.fullTextCTE("t1", "chunk", []string{"id"}, `"kb_id" = $1`,
		[]interface{}{"kb-1"}, text, 50, args)
	if err != nil {
		t.Fatal(err)
	}
	branch := func(field, weight string, filterPart, placeholder string) string {
		return `(SELECT "id", bm25_score() AS bm25_score FROM "t1" ` +
			`WHERE (` + filterPart + `) AND (` + field + ` @~@ ` + placeholder + `) ORDER BY bm25_score DESC LIMIT 100)`
	}
	// Each branch re-binds the filter, so placeholders advance per branch; the
	// dedup keys on the kind's row identifier.
	want := `SELECT "id", "SCORE" AS _score FROM (SELECT DISTINCT ON ("id") "id", bm25_score AS "SCORE" FROM (` +
		branch(`"title_tks"`, `2`, `"kb_id" = $1`, `$2`) + " UNION ALL " +
		branch(`"content_ltks"`, `1`, `"kb_id" = $3`, `$4`) +
		`) AS unioned ORDER BY "id", "SCORE" DESC) AS deduped ORDER BY _score DESC LIMIT 50`
	if got != want {
		t.Fatalf("multi-column CTE =\n%s\nwant\n%s", got, want)
	}
	// Boost suffixes move out of the query text into PARAM:BOOST.
	if args.values[1] != "boosted query @<PARAM:MINIMUM_SHOULD_MATCH=0% PARAM:BOOST=2>@" {
		t.Fatalf("operand 1 = %#v", args.values[1])
	}
	if args.values[3] != "boosted query @<PARAM:MINIMUM_SHOULD_MATCH=0% PARAM:BOOST=1>@" {
		t.Fatalf("operand 2 = %#v", args.values[3])
	}
}

// TestFullTextCTEPGModeSingleColumnGolden pins the PG-mode single-column CTE:
// predicates restate the indexed to_tsvector('cn_tokenizer', ...) expression
// against plainto_tsquery, terms OR together below 100% minimum_should_match,
// and ts_rank supplies the score.
func TestFullTextCTEPGModeSingleColumnGolden(t *testing.T) {
	engine := &Engine{dbCompatibility: compatPG}
	text := &types.MatchTextExpr{
		Fields:       []string{"content_ltks"},
		MatchingText: "hello world",
		TopN:         100,
		ExtraOptions: map[string]interface{}{"minimum_should_match": 0.5},
	}
	args := &sqlArgs{}
	got, err := engine.fullTextCTE("t1", "chunk", []string{"id", "content_ltks"}, `"kb_id" = $1`,
		[]interface{}{"kb-1"}, text, 100, args)
	if err != nil {
		t.Fatal(err)
	}
	rank := `GREATEST(ts_rank(to_tsvector('cn_tokenizer', "content_ltks"), plainto_tsquery('cn_tokenizer', $2)), ` +
		`ts_rank(to_tsvector('cn_tokenizer', "content_ltks"), plainto_tsquery('cn_tokenizer', $3)))`
	match := `to_tsvector('cn_tokenizer', "content_ltks") @@ plainto_tsquery('cn_tokenizer', $2) OR ` +
		`to_tsvector('cn_tokenizer', "content_ltks") @@ plainto_tsquery('cn_tokenizer', $3)`
	want := `SELECT "id", "content_ltks", (` + rank + ` * 1) AS _score FROM "t1" ` +
		`WHERE ("kb_id" = $1) AND (` + match + `) ORDER BY _score DESC LIMIT 100`
	if got != want {
		t.Fatalf("PG single-column CTE =\n%s\nwant\n%s", got, want)
	}
	if !reflect.DeepEqual(args.values, []interface{}{"kb-1", "hello", "world"}) {
		t.Fatalf("args = %#v", args.values)
	}
}

// TestFullTextCTEPGModeMultiColumnSkillGolden pins the PG-mode UNION form and
// the skill table's row identifier: branches deduplicate on skill_id, and a
// 100% minimum_should_match AND-joins the term predicates.
func TestFullTextCTEPGModeMultiColumnSkillGolden(t *testing.T) {
	engine := &Engine{dbCompatibility: compatPG}
	text := &types.MatchTextExpr{
		Fields:       []string{"name_tks^2", "content_tks"},
		MatchingText: "deploy",
		TopN:         50,
		ExtraOptions: map[string]interface{}{"minimum_should_match": "100%"},
	}
	args := &sqlArgs{}
	got, err := engine.fullTextCTE("skill_t1", "skill", []string{"skill_id"}, `"space_id" = $1`,
		[]interface{}{"sp-1"}, text, 50, args)
	if err != nil {
		t.Fatal(err)
	}
	branch := func(field, weight, filterPart, placeholder string) string {
		return `(SELECT "skill_id", (GREATEST(ts_rank(to_tsvector('cn_tokenizer', ` + field +
			`), plainto_tsquery('cn_tokenizer', ` + placeholder + `))) * ` + weight + `) AS "SCORE" FROM "skill_t1" ` +
			`WHERE (` + filterPart + `) AND (to_tsvector('cn_tokenizer', ` + field + `) @@ plainto_tsquery('cn_tokenizer', ` +
			placeholder + `)) ORDER BY "SCORE" DESC LIMIT 100)`
	}
	want := `SELECT "skill_id", "SCORE" AS _score FROM (SELECT DISTINCT ON ("skill_id") "skill_id", "SCORE" FROM (` +
		branch(`"name_tks"`, `2`, `"space_id" = $1`, `$2`) + " UNION ALL " +
		branch(`"content_tks"`, `1`, `"space_id" = $3`, `$4`) +
		`) AS unioned ORDER BY "skill_id", "SCORE" DESC) AS deduped ORDER BY _score DESC LIMIT 50`
	if got != want {
		t.Fatalf("PG skill multi-column CTE =\n%s\nwant\n%s", got, want)
	}
	if !reflect.DeepEqual(args.values, []interface{}{"sp-1", "deploy", "sp-1", "deploy"}) {
		t.Fatalf("args = %#v", args.values)
	}
}

// TestMinimumShouldMatchCoversAllTerms checks when the PG-mode predicate
// AND-joins terms instead of OR-joining them.
func TestMinimumShouldMatchCoversAllTerms(t *testing.T) {
	cases := []struct {
		minimumShouldMatch string
		terms              int
		want               bool
	}{
		{"0%", 2, false},
		{"50%", 2, false},
		{"100%", 2, true},
		{"200%", 1, true},
		{"75%", 0, false},
		{"2", 2, true},
		{"1", 2, false},
		{"3<-25%", 3, false},
		{"nonsense", 2, false},
	}
	for _, test := range cases {
		if got := minimumShouldMatchCoversAllTerms(test.minimumShouldMatch, test.terms); got != test.want {
			t.Fatalf("minimumShouldMatchCoversAllTerms(%q, %d) = %v", test.minimumShouldMatch, test.terms, got)
		}
	}
}

// TestVectorCTEGolden pins the dense-vector CTE SQL, including the bare
// ORDER BY that keeps the graph index eligible.
func TestVectorCTEGolden(t *testing.T) {
	dense := &types.MatchDenseExpr{
		VectorColumnName: "q_3_vec",
		EmbeddingData:    []float64{0.1, 0.2, 0.3},
		TopN:             10,
		ExtraOptions:     map[string]interface{}{"similarity": 0.2},
	}
	build := func(engine *Engine) string {
		args := &sqlArgs{}
		cte, err := engine.vectorCTE("t1", []string{"id"}, `"kb_id" = $1`,
			[]interface{}{"kb-1"}, dense, 10, 0.2, "[0.1,0.2,0.3]", args)
		if err != nil {
			t.Fatal(err)
		}
		return cte
	}
	pg := build(&Engine{dbCompatibility: compatPG})
	wantPG := `SELECT * FROM (SELECT "id", (1 - ("q_3_vec" <=> $2)) AS _score FROM "t1" ` +
		`WHERE "kb_id" = $1 ORDER BY "q_3_vec" <=> $3 LIMIT 10) AS knn WHERE _score >= $4`
	if pg != wantPG {
		t.Fatalf("PG vector CTE =\n%s", pg)
	}
	b := build(&Engine{dbCompatibility: compatB})
	if !regexp.MustCompile(`"q_3_vec" <\+> \$2`).MatchString(b) || !regexp.MustCompile(`"q_3_vec" <\+> \$3`).MatchString(b) {
		t.Fatalf("B-mode operator missing:\n%s", b)
	}
	// The plan contract: threshold never inside the KNN scan, ORDER BY is the
	// bare column so graph_index stays eligible.
	if !regexp.MustCompile(`\) AS knn WHERE _score >= \$4$`).MatchString(b) {
		t.Fatalf("threshold placement changed:\n%s", b)
	}
}

// TestSearchFusionQueryGolden pins the fused full-text-plus-vector query SQL
// for PG mode: the ts_rank CTE, the vector KNN scan, and the identifier join.
func TestSearchFusionQueryGolden(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	expectChunkColumns(mock, "t1")
	expectChunkColumns(mock, "t1") // searchFusion re-lists columns for the "*" expansion

	projection := `COALESCE(a."content_ltks", b."content_ltks") AS "content_ltks", ` +
		`COALESCE(a."id", b."id") AS "id", COALESCE(a."kb_id", b."kb_id") AS "kb_id", ` +
		`COALESCE(a."pagerank_fea", b."pagerank_fea") AS "pagerank_fea", ` +
		`COALESCE(a."q_3_vec", b."q_3_vec") AS "q_3_vec"`
	fields := `"content_ltks", "id", "kb_id", "pagerank_fea", "q_3_vec"`
	fulltextScore := `(GREATEST(ts_rank(to_tsvector('cn_tokenizer', "content_ltks"), plainto_tsquery('cn_tokenizer', $2))) * 1)`
	fulltextMatch := `to_tsvector('cn_tokenizer', "content_ltks") @@ plainto_tsquery('cn_tokenizer', $2)`
	fulltextCTE := `SELECT ` + fields + `, ` + fulltextScore + ` AS _score FROM "t1" ` +
		`WHERE ("kb_id" IN ($1)) AND (` + fulltextMatch + `) ORDER BY _score DESC LIMIT 10`
	vectorCTE := `SELECT * FROM (SELECT ` + fields + `, (1 - ("q_3_vec" <=> $4)) AS _score FROM "t1" ` +
		`WHERE "kb_id" IN ($3) ORDER BY "q_3_vec" <=> $5 LIMIT 10) AS knn WHERE _score >= $6`
	fused := `(COALESCE(a._score, 0) / COALESCE(NULLIF((SELECT MAX(_score) FROM filter_fulltext), 0), 1) * 0.7 + COALESCE(b._score, 0) * 0.3)`
	query := `WITH filter_fulltext AS (` + fulltextCTE + `), filter_vector AS (` + vectorCTE + `) ` +
		`SELECT ` + projection + `, ` + fused + ` AS _score ` +
		`FROM filter_fulltext a FULL OUTER JOIN filter_vector b ON a."id" = b."id" ORDER BY _score DESC LIMIT 10 OFFSET 0`

	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs("kb-1", "hello",
			"kb-1", "[0.1,0.2,0.3]", "[0.1,0.2,0.3]", 0.2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content_ltks", "kb_id", "pagerank_fea", "q_3_vec", "_score"}).
			AddRow("c1", "hello world", "kb-1", 0.5, "[0.1,0.2,0.3]", 0.9))

	result, err := engine.Search(context.Background(), &types.SearchRequest{
		IndexNames:   []string{"t1"},
		KbIDs:        []string{"kb-1"},
		SelectFields: []string{"*"},
		Limit:        10,
		MatchExprs: []interface{}{
			&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "hello", TopN: 10},
			&types.MatchDenseExpr{VectorColumnName: "q_3_vec", EmbeddingData: []float64{0.1, 0.2, 0.3},
				TopN: 10, ExtraOptions: map[string]interface{}{"similarity": 0.2}},
			&types.FusionExpr{Method: "weighted_sum", TopN: 10,
				FusionParams: map[string]interface{}{"weights": "0.7,0.3"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0]["id"] != "c1" {
		t.Fatalf("chunks = %#v", result.Chunks)
	}
	if score, _ := numberToFloat(result.Chunks[0]["_score"]); score != 0.9 {
		t.Fatalf("fused score = %#v", result.Chunks[0]["_score"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestSearchSkillFusionUsesSkillIdentifier drives a skill-table hybrid
// search end to end: the fused join and the projected row key must use
// skill_id, not id.
func TestSearchSkillFusionUsesSkillIdentifier(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	skillColumns := []string{"skill_id", "name", "name_tks", "content_tks", "q_2_vec"}
	listSkillColumns := func() {
		expectCount(mock, tableCountQuery(), "skill_t1")
		rows := sqlmock.NewRows([]string{"column_name", "data_type", "column_default", "is_nullable"})
		for _, column := range skillColumns {
			rows.AddRow(column, "text", nil, "YES")
		}
		mock.ExpectQuery(regexp.QuoteMeta(listColumnsQuery)).WithArgs("skill_t1").WillReturnRows(rows)
	}
	listSkillColumns()
	listSkillColumns() // searchFusion re-lists columns for the "*" expansion

	fields := `"content_tks", "name", "name_tks", "q_2_vec", "skill_id"`
	fulltextScore := `(GREATEST(ts_rank(to_tsvector('cn_tokenizer', "name_tks"), plainto_tsquery('cn_tokenizer', $1))) * 1)`
	fulltextCTE := `SELECT ` + fields + `, ` + fulltextScore + ` AS _score FROM "skill_t1" ` +
		`WHERE (1=1) AND (to_tsvector('cn_tokenizer', "name_tks") @@ plainto_tsquery('cn_tokenizer', $1)) ` +
		`ORDER BY _score DESC LIMIT 10`
	vectorCTE := `SELECT * FROM (SELECT ` + fields + `, (1 - ("q_2_vec" <=> $2)) AS _score FROM "skill_t1" ` +
		`WHERE 1=1 ORDER BY "q_2_vec" <=> $3 LIMIT 10) AS knn WHERE _score >= $4`
	projection := `COALESCE(a."content_tks", b."content_tks") AS "content_tks", ` +
		`COALESCE(a."name", b."name") AS "name", COALESCE(a."name_tks", b."name_tks") AS "name_tks", ` +
		`COALESCE(a."q_2_vec", b."q_2_vec") AS "q_2_vec", COALESCE(a."skill_id", b."skill_id") AS "skill_id"`
	fused := `(COALESCE(a._score, 0) / COALESCE(NULLIF((SELECT MAX(_score) FROM filter_fulltext), 0), 1) * 0.5 + COALESCE(b._score, 0) * 0.5)`
	query := `WITH filter_fulltext AS (` + fulltextCTE + `), filter_vector AS (` + vectorCTE + `) ` +
		`SELECT ` + projection + `, ` + fused + ` AS _score ` +
		`FROM filter_fulltext a FULL OUTER JOIN filter_vector b ON a."skill_id" = b."skill_id" ` +
		`ORDER BY _score DESC LIMIT 10 OFFSET 0`

	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs("deploy", "[0.5,0.25]", "[0.5,0.25]", 0.2).
		WillReturnRows(sqlmock.NewRows([]string{"skill_id", "name", "_score"}).AddRow("s1", "deploy skill", 0.8))

	result, err := engine.Search(context.Background(), &types.SearchRequest{
		IndexNames:   []string{"skill_t1"},
		SelectFields: []string{"*"},
		Limit:        10,
		MatchExprs: []interface{}{
			&types.MatchTextExpr{Fields: []string{"name_tks"}, MatchingText: "deploy", TopN: 10},
			&types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{0.5, 0.25},
				TopN: 10, ExtraOptions: map[string]interface{}{"similarity": 0.2}},
			&types.FusionExpr{Method: "weighted_sum", TopN: 10,
				FusionParams: map[string]interface{}{"weights": "0.5,0.5"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0]["skill_id"] != "s1" {
		t.Fatalf("chunks = %#v", result.Chunks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestSearchFusionWeightDegeneration checks that all-or-nothing fusion
// weights collapse the hybrid path into the single remaining path.
func TestSearchFusionWeightDegeneration(t *testing.T) {
	newReq := func(weights string) *types.SearchRequest {
		return &types.SearchRequest{
			IndexNames:   []string{"t1"},
			KbIDs:        []string{"kb-1"},
			SelectFields: []string{"id"},
			Limit:        10,
			MatchExprs: []interface{}{
				&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "hello", TopN: 10},
				&types.MatchDenseExpr{VectorColumnName: "q_3_vec", EmbeddingData: []float64{0.1, 0.2, 0.3}, TopN: 10},
				&types.FusionExpr{Method: "weighted_sum", TopN: 10,
					FusionParams: map[string]interface{}{"weights": weights}},
			},
		}
	}

	// Vector weight 1: the text leg is dropped, only the KNN scan runs.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)
	expectChunkColumns(mock, "t1")
	mock.ExpectQuery(`^SELECT \* FROM \(SELECT \* FROM \(SELECT .+ <=> \$2.+ <=> \$3 .+\) AS knn WHERE _score >= \$4\) AS candidates ORDER BY _score DESC LIMIT 10 OFFSET 0$`).
		WithArgs("kb-1", "[0.1,0.2,0.3]", "[0.1,0.2,0.3]", 0.0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "pagerank_fea", "_score"}).AddRow("c1", 0, 0.5))
	if _, err := engine.Search(context.Background(), newReq("0,1")); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	// Vector weight 0: the dense leg is dropped, only the full-text scan runs
	// (PG mode: the plainto_tsquery predicate).
	db2, mock2, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	engine2 := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db2)
	expectChunkColumns(mock2, "t1")
	mock2.ExpectQuery(regexp.QuoteMeta(`@@ plainto_tsquery('cn_tokenizer', $2)`)).
		WithArgs("kb-1", "hello").
		WillReturnRows(sqlmock.NewRows([]string{"id", "pagerank_fea", "_score"}).AddRow("c1", 0, 1.5))
	if _, err := engine2.Search(context.Background(), newReq("1,0")); err != nil {
		t.Fatal(err)
	}
	if err := mock2.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestSearchMemoryFullTextUsesTSQuery pins the memory keyword path, which
// ranks through to_tsquery instead of the bm25 operator.
func TestSearchMemoryFullTextUsesTSQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	expectCount(mock, tableCountQuery(), "memory_t1")
	rows := sqlmock.NewRows([]string{"column_name", "data_type", "column_default", "is_nullable"})
	for _, column := range []string{"id", "memory_id", "content_ltks", "status_int", "forget_at"} {
		rows.AddRow(column, "text", nil, "YES")
	}
	mock.ExpectQuery(regexp.QuoteMeta(listColumnsQuery)).WithArgs("memory_t1").WillReturnRows(rows)

	// The score expression is built after the match predicate, so the
	// ts_rank placeholder ($3) trails the match placeholder ($2); both run
	// over the fine-grained tokenized_content_ltks column.
	query := `SELECT "content_ltks", "id", ts_rank(to_tsvector('simple', COALESCE(tokenized_content_ltks, '')), to_tsquery('simple', $3)) AS _score ` +
		`FROM "memory_t1" WHERE "memory_id" IN ($1) AND ("forget_at" IS NULL OR "forget_at" = '') ` +
		`AND to_tsvector('simple', COALESCE(tokenized_content_ltks, '')) @@ to_tsquery('simple', $2) ` +
		`ORDER BY _score DESC LIMIT 30 OFFSET 0`
	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs("mem-1", "hello & world", "hello & world").
		WillReturnRows(sqlmock.NewRows([]string{"content_ltks", "id", "_score"}).AddRow("hello world", "m1_1", 0.1))

	result, err := engine.Search(context.Background(), &types.SearchRequest{
		IndexNames:   []string{"memory_t1"},
		KbIDs:        []string{"mem-1"},
		SelectFields: []string{"content"},
		MatchExprs:   []interface{}{&types.MatchTextExpr{MatchingText: "hello world"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0]["content"] != "hello world" {
		t.Fatalf("chunks = %#v", result.Chunks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestSearchMemoryFusionCandidatesGolden pins the memory fusion candidate
// query that scores vectors over CTE rows.
func TestSearchMemoryFusionCandidatesGolden(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	expectCount(mock, tableCountQuery(), "memory_t1")
	rows := sqlmock.NewRows([]string{"column_name", "data_type", "column_default", "is_nullable"})
	for _, column := range []string{"id", "memory_id", "content_ltks", "forget_at", "q_2_vec"} {
		rows.AddRow(column, "text", nil, "YES")
	}
	mock.ExpectQuery(regexp.QuoteMeta(listColumnsQuery)).WithArgs("memory_t1").WillReturnRows(rows)

	score := `(ts_rank(to_tsvector('simple', COALESCE(tokenized_content_ltks, '')), to_tsquery('simple', $3)) * 0.7 + (1 - ("q_2_vec" <=> $4)) * 0.3)`
	inner := `SELECT "content_ltks", "id", "q_2_vec", ` + score + ` AS _score FROM "memory_t1" ` +
		`WHERE "memory_id" IN ($1) AND ("forget_at" IS NULL OR "forget_at" = '') ` +
		`AND to_tsvector('simple', COALESCE(tokenized_content_ltks, '')) @@ to_tsquery('simple', $2) LIMIT 20`
	query := `SELECT * FROM (` + inner + `) AS candidates WHERE (1 - ("q_2_vec" <=> $5)) >= $6 ORDER BY _score DESC LIMIT 10 OFFSET 0`
	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs("mem-1", "hello", "hello", "[0.5,0.25]", "[0.5,0.25]", 0.2).
		WillReturnRows(sqlmock.NewRows([]string{"content_ltks", "id", "q_2_vec", "_score"}).
			AddRow("hello", "m1_1", "[0.5,0.25]", 0.6))

	result, err := engine.Search(context.Background(), &types.SearchRequest{
		IndexNames:   []string{"memory_t1"},
		KbIDs:        []string{"mem-1"},
		SelectFields: []string{"content"},
		Limit:        10,
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "hello", TopN: 10},
			&types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{0.5, 0.25},
				TopN: 10, ExtraOptions: map[string]interface{}{"similarity": 0.2}},
			&types.FusionExpr{Method: "weighted_sum", TopN: 10,
				FusionParams: map[string]interface{}{"weights": "0.7,0.3"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0]["content_embed"] == nil {
		t.Fatalf("chunks = %#v", result.Chunks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestSearchFilterOnlyCountsThenPages checks the two-statement shape of the
// filter-only path: one COUNT for the total, one paged select.
func TestSearchFilterOnlyCountsThenPages(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	// Search lists the table's columns before dispatching; filter-only then
	// runs the COUNT gate and lists columns once more for ORDER BY validation.
	expectCount(mock, tableCountQuery(), "t1")
	columns := sqlmock.NewRows([]string{"column_name", "data_type", "column_default", "is_nullable"})
	for _, column := range []string{"id", "kb_id", "doc_id", "page_num_int"} {
		columns.AddRow(column, "text", nil, "YES")
	}
	mock.ExpectQuery(regexp.QuoteMeta(listColumnsQuery)).WithArgs("t1").WillReturnRows(columns)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT("id") FROM "t1" WHERE "doc_id" = $1 AND "kb_id" IN ($2)`)).
		WithArgs("doc-1", "kb-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	expectCount(mock, tableCountQuery(), "t1")
	orderColumns := sqlmock.NewRows([]string{"column_name", "data_type", "column_default", "is_nullable"})
	for _, column := range []string{"id", "kb_id", "doc_id", "page_num_int"} {
		orderColumns.AddRow(column, "integer[]", nil, "YES")
	}
	mock.ExpectQuery(regexp.QuoteMeta(listColumnsQuery)).WithArgs("t1").WillReturnRows(orderColumns)
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT "id" FROM "t1" WHERE "doc_id" = $1 AND "kb_id" IN ($2) `+
			`ORDER BY (SELECT AVG(element) FROM unnest("page_num_int") AS element) DESC LIMIT 30 OFFSET 0`)).
		WithArgs("doc-1", "kb-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("c1").AddRow("c2"))

	result, err := engine.Search(context.Background(), &types.SearchRequest{
		IndexNames: []string{"t1"},
		KbIDs:      []string{"kb-1"},
		Filter:     map[string]interface{}{"doc_id": "doc-1"},
		OrderBy:    &types.OrderByExpr{Fields: []types.OrderByField{{Field: "page_num_int", Type: types.SortDesc}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 3 || len(result.Chunks) != 2 {
		t.Fatalf("total = %d chunks = %#v", result.Total, result.Chunks)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestSearchSkipsMissingTables checks that a search naming a table the
// schema never created skips it instead of failing.
func TestSearchSkipsMissingTables(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	mock.ExpectQuery(regexp.QuoteMeta(tableCountQuery())).WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	result, err := engine.Search(context.Background(), &types.SearchRequest{
		IndexNames:   []string{"t1"},
		KbIDs:        []string{"kb-1"},
		SelectFields: []string{"id"},
		MatchExprs:   []interface{}{&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Chunks) != 0 || result.Total != 0 {
		t.Fatalf("result = %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestMemoryTSQuery checks the memory tsquery builder's term joining and
// escaping.
func TestMemoryTSQuery(t *testing.T) {
	cases := []struct {
		text *types.MatchTextExpr
		want string
	}{
		{&types.MatchTextExpr{MatchingText: "hello world"}, "hello & world"},
		{&types.MatchTextExpr{MatchingText: "boost~3"}, "boost"},
		{
			&types.MatchTextExpr{
				MatchingText: "ignored",
				ExtraOptions: map[string]interface{}{"original_query": `user's (input) & more`},
			},
			"users & input & more",
		},
		{&types.MatchTextExpr{MatchingText: "  "}, ""},
	}
	for _, test := range cases {
		if got := memoryTSQuery(test.text); got != test.want {
			t.Fatalf("memoryTSQuery(%#v) = %q, want %q", test.text.MatchingText, got, test.want)
		}
	}
}

// TestParseFullTextFields checks "field^weight" specification parsing,
// including entries without a weight.
func TestParseFullTextFields(t *testing.T) {
	fields, weights := parseFullTextFields(&types.MatchTextExpr{
		Fields: []string{"title_tks^10", "content_ltks", "question_tks^0.5"},
	})
	if !reflect.DeepEqual(fields, []string{"title_tks", "content_ltks", "question_tks"}) {
		t.Fatalf("fields = %#v", fields)
	}
	if !reflect.DeepEqual(weights, []string{"10", "1", "0.5"}) {
		t.Fatalf("weights = %#v", weights)
	}
}

// TestFusionVectorWeight checks fusion weight parsing and its even-split
// fallbacks.
func TestFusionVectorWeight(t *testing.T) {
	cases := []struct {
		fusion *types.FusionExpr
		want   float64
	}{
		{nil, 0.5},
		{&types.FusionExpr{}, 0.5},
		{&types.FusionExpr{FusionParams: map[string]interface{}{"weights": "0.7,0.3"}}, 0.3},
		{&types.FusionExpr{FusionParams: map[string]interface{}{"weights": "0.7"}}, 0.5},
		{&types.FusionExpr{FusionParams: map[string]interface{}{"weights": "0.7,x"}}, 0.5},
		{&types.FusionExpr{FusionParams: map[string]interface{}{"weights": " 0.5 , 0.9 "}}, 0.9},
	}
	for _, test := range cases {
		if got := fusionVectorWeight(test.fusion); got != test.want {
			t.Fatalf("fusionVectorWeight(%#v) = %v, want %v", test.fusion, got, test.want)
		}
	}
}

// TestFormatMinimumShouldMatch checks the percent rendering of the
// minimum_should_match option across its accepted types.
func TestFormatMinimumShouldMatch(t *testing.T) {
	cases := []struct {
		options map[string]interface{}
		want    string
	}{
		{nil, "0%"},
		{map[string]interface{}{}, "0%"},
		{map[string]interface{}{"minimum_should_match": 0.5}, "50%"},
		{map[string]interface{}{"minimum_should_match": float32(0.25)}, "25%"},
		// A bare count is formatted through the percent helper, matching the
		// oceanbase connector's behavior for the same ExtraOptions value.
		{map[string]interface{}{"minimum_should_match": 2}, "200%"},
		{map[string]interface{}{"minimum_should_match": "75%"}, "75%"},
		{map[string]interface{}{"minimum_should_match": ""}, "0%"},
	}
	for _, test := range cases {
		got := formatMinimumShouldMatch(&types.MatchTextExpr{ExtraOptions: test.options})
		if got != test.want {
			t.Fatalf("formatMinimumShouldMatch(%#v) = %q, want %q", test.options, got, test.want)
		}
	}
}

// TestBuildSearchOutput checks how requested fields, kinds, and hidden sort
// fields turn into the SQL select list.
func TestBuildSearchOutput(t *testing.T) {
	columns := testColumns()
	output := buildSearchOutput([]string{"content_ltks", "_score", "ghost_field"}, "chunk", columns, true, nil)
	if !reflect.DeepEqual(output, []string{"content_ltks", "id", "pagerank_fea"}) {
		t.Fatalf("chunk scored output = %#v", output)
	}
	plain := buildSearchOutput([]string{"content_ltks"}, "chunk", columns, false, nil)
	if !reflect.DeepEqual(plain, []string{"content_ltks", "id"}) {
		t.Fatalf("chunk unscored output = %#v", plain)
	}
	if star := buildSearchOutput([]string{"*"}, "chunk", columns, true, nil); !reflect.DeepEqual(star, []string{"*"}) {
		t.Fatalf("star output = %#v", star)
	}
	memory := make(map[string]columnMeta)
	for _, column := range memoryColumns {
		memory[column.name] = columnMeta{name: column.name, dataType: column.typeSQL}
	}
	memory["q_2_vec"] = columnMeta{name: "q_2_vec", dataType: "text"}
	memoryOutput := buildSearchOutput([]string{"content", "status", "content_embed"}, "memory", memory, true, nil)
	if !reflect.DeepEqual(memoryOutput, []string{"content_ltks", "status_int", "q_2_vec", "id"}) {
		t.Fatalf("memory output = %#v", memoryOutput)
	}
	// Skill rows key on skill_id, not id.
	skill := map[string]columnMeta{"skill_id": {}, "name": {}, "content_tks": {}}
	skillOutput := buildSearchOutput([]string{"name"}, "skill", skill, true, nil)
	if !reflect.DeepEqual(skillOutput, []string{"name", "skill_id"}) {
		t.Fatalf("skill output = %#v", skillOutput)
	}
}

// TestScoredChunkSortFoldsPagerank checks that merged ranking folds
// pagerank_fea into the sort key.
func TestScoredChunkSortFoldsPagerank(t *testing.T) {
	chunks := []map[string]interface{}{
		{"id": "low", "_score": 0.8, "pagerank_fea": 0},
		{"id": "high", "_score": 0.5, "pagerank_fea": 1.0},
		{"id": "mid", "_score": 0.7, "pagerank_fea": 0},
	}
	sortScoredChunks(chunks)
	if chunks[0]["id"] != "high" || chunks[1]["id"] != "low" || chunks[2]["id"] != "mid" {
		t.Fatalf("order = %#v", chunks)
	}
}

// Multi-table searches page only once, on the merged set: each table's SQL
// must cover the whole candidate range (LIMIT offset+limit OFFSET 0), not
// page with req's OFFSET — otherwise mergeSearchChunks skips offset rows a
// second time and loses page-2 rows that ranked inside a single table.
func TestSearchMultiTableWindowsOnlyAfterMerge(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	for _, table := range []string{"t1", "t2"} {
		expectChunkColumns(mock, table)
		mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`SELECT COUNT("id") FROM "%s" WHERE 1=1`, table))).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
		// searchFilterOnly re-lists columns before building ORDER BY.
		expectChunkColumns(mock, table)
		prefix := "a"
		if table == "t2" {
			prefix = "b"
		}
		rows := sqlmock.NewRows([]string{"id"})
		for i := 1; i <= 5; i++ {
			rows.AddRow(fmt.Sprintf("%s%d", prefix, i))
		}
		mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(
			`SELECT "id" FROM "%s" WHERE 1=1 ORDER BY "id" ASC LIMIT 5 OFFSET 0`, table))).
			WillReturnRows(rows)
	}

	result, err := engine.Search(context.Background(), &types.SearchRequest{
		IndexNames:   []string{"t1", "t2"},
		Offset:       2,
		Limit:        3,
		SelectFields: []string{"id"},
		OrderBy:      &types.OrderByExpr{Fields: []types.OrderByField{{Field: "id", Type: types.SortAsc}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Global order a1..a5, b1..b5; the page window [2:5) keeps a3..a5 —
	// exactly the rows the double-offset path dropped.
	got := make([]string, 0, len(result.Chunks))
	for _, chunk := range result.Chunks {
		got = append(got, chunk["id"].(string))
	}
	if !reflect.DeepEqual(got, []string{"a3", "a4", "a5"}) {
		t.Fatalf("merged page ids = %v", got)
	}
	if result.Total != 10 {
		t.Fatalf("total = %d", result.Total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
