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

package tablequery

import (
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const (
	testAllowedKB = "11111111-1111-1111-1111-111111111111"
	testOtherKB   = "22222222-2222-2222-2222-222222222222"
)

func testScope(table string, kbIDs ...string) Scope {
	if len(kbIDs) == 0 {
		kbIDs = []string{testAllowedKB}
	}
	return Scope{
		TenantID:  "tenant_a",
		TableName: table,
		KBIDs:     kbIDs,
		Fields: FieldsFromFieldMap(map[string]interface{}{
			"product_tks": "Product Name",
			"category":    "Category",
			"price_flt":   "Price (float)",
			"qty_int":     "Quantity (int)",
			"2024_sales":  "2024 Sales",
			"Unit Price":  "Unit Price",
		}),
		MaxLimit: DefaultMaxLimit,
	}
}

func TestParseAndValidate_BasicAndRepairedJSON(t *testing.T) {
	scope := testScope("ragflow_tenant_a")
	raw := "<think>reasoning</think>\n```json\n" +
		`{"select": ["category", "price_flt"], "where": {"field": "category", "op": "eq", "value": "phone",}, "order_by": [{"field": "price_flt", "desc": true}], "limit": 10}` +
		"\n```"

	vq, err := ParseAndValidate(raw, scope, true)
	if err != nil {
		t.Fatalf("ParseAndValidate failed: %v", err)
	}
	if vq.IsAggregate() {
		t.Fatal("expected non-aggregate query")
	}
	if !vq.IncludesCitations() {
		t.Fatal("expected citations enabled for non-aggregate query")
	}
	q := vq.Query()
	if len(q.Select) != 2 || q.Select[0] != "category" || q.Select[1] != "price_flt" {
		t.Fatalf("Select = %v", q.Select)
	}
	if q.Limit != 10 {
		t.Fatalf("Limit = %d, want 10", q.Limit)
	}
}

func TestParseAndValidate_UnsupportedFlag(t *testing.T) {
	scope := testScope("ragflow_tenant_a")
	_, err := ParseAndValidate(`{"unsupported": true}`, scope, true)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

func TestParseAndValidate_RejectsUnknownFieldsAndInjectionAttempts(t *testing.T) {
	scope := testScope("ragflow_tenant_a")

	badPayloads := []struct {
		name string
		raw  string
	}{
		{"unknown top-level key (table override)", `{"table": "ragflow_other", "select": ["category"]}`},
		{"unknown top-level key (kb_id override)", `{"kb_id": "` + testOtherKB + `", "select": ["category"]}`},
		{"unknown select column", `{"select": ["kb_id"]}`},
		{"sql injection in select field", `{"select": ["category FROM ragflow_other"]}`},
		{"unknown where field", `{"select": ["category"], "where": {"field": "kb_id", "op": "eq", "value": "` + testOtherKB + `"}}`},
		{"unsupported operator", `{"select": ["category"], "where": {"field": "category", "op": "sql", "value": "1=1"}}`},
		{"is_null with value", `{"select": ["category"], "where": {"field": "category", "op": "is_null", "value": true}}`},
		{"eq with null value", `{"select": ["category"], "where": {"field": "category", "op": "eq", "value": null}}`},
		{"empty in array", `{"select": ["category"], "where": {"field": "category", "op": "in", "value": []}}`},
		{"object value in filter", `{"select": ["category"], "where": {"field": "category", "op": "eq", "value": {"subquery": "SELECT 1"}}}`},
		{"reserved aggregate alias doc_id", `{"aggregates": [{"fn": "count", "as": "doc_id"}]}`},
		{"sql injection in aggregate alias", `{"aggregates": [{"fn": "count", "as": "x FROM other"}]}`},
		{"unsupported aggregate function", `{"aggregates": [{"fn": "query_to_xml", "field": "category", "as": "x"}]}`},
		{"sum without field", `{"aggregates": [{"fn": "sum", "as": "total"}]}`},
		{"having without aggregate or group_by", `{"select": ["category"], "having": {"field": "category", "op": "eq", "value": "a"}}`},
		{"empty query", `{}`},
		{"mixed filter node", `{"select": ["category"], "where": {"and": [{"field": "category", "op": "eq", "value": "a"}], "field": "category", "op": "eq", "value": "b"}}`},
		{"negative offset", `{"select": ["category"], "offset": -5}`},
	}

	for _, tc := range badPayloads {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseAndValidate(tc.raw, scope, true); err == nil {
				t.Fatalf("SECURITY: expected rejection for %s (%s)", tc.name, tc.raw)
			}
		})
	}
}

func TestValidate_EnforcesBoundsAndScopeInvariants(t *testing.T) {
	// Invalid KB UUID must fail closed.
	badKBScope := testScope("ragflow_tenant_a", "not-a-uuid")
	if _, err := Validate(badKBScope, Query{Select: []string{"category"}}, true); err == nil {
		t.Fatal("expected invalid kb_id UUID to be rejected")
	}

	// Invalid table name must fail closed.
	badTableScope := testScope("ragflow_a; DROP TABLE x")
	if _, err := Validate(badTableScope, Query{Select: []string{"category"}}, true); err == nil {
		t.Fatal("expected invalid table name to be rejected")
	}

	// Excessively deep filter tree must be rejected.
	deep := &FilterNode{Field: "category", Op: OpEq, Value: "x"}
	for i := 0; i <= maxFilterDepth+1; i++ {
		deep = &FilterNode{Not: deep}
	}
	if _, err := Validate(testScope("ragflow_tenant_a"), Query{Select: []string{"category"}, Where: deep}, true); err == nil {
		t.Fatal("expected overly deep filter tree to be rejected")
	}

	// Limit exceeding MaxLimit is clamped.
	scope := testScope("ragflow_tenant_a")
	scope.MaxLimit = 100
	vq, err := Validate(scope, Query{Select: []string{"category"}, Limit: 99999}, false)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if vq.Query().Limit != 100 {
		t.Fatalf("Limit = %d, want clamped 100", vq.Query().Limit)
	}
}

func TestCompile_ElasticsearchAndOpenSearch(t *testing.T) {
	scope := testScope("ragflow_tenant_a", testAllowedKB)
	vq, err := Validate(scope, Query{
		Select: []string{"product_tks", "2024_sales"},
		Where: &FilterNode{
			Or: []FilterNode{
				{Field: "product_tks", Op: OpContains, Value: "smart phone%"},
				{Field: "category", Op: OpEq, Value: "O'Brien's"},
			},
		},
		OrderBy: []OrderSpec{{Field: "2024_sales", Desc: true}},
		Limit:   25,
	}, true)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	opts := CompileOptions{
		TokenizeForMatch: func(s string) (string, error) {
			return "smart phone", nil
		},
	}

	for _, dialect := range []Dialect{DialectElasticsearch, DialectOpenSearch} {
		compiled, err := CompileWithOptions(vq, dialect, opts)
		if err != nil {
			t.Fatalf("Compile(%s): %v", dialect, err)
		}
		if len(compiled.Args) != 0 {
			t.Fatalf("%s expected 0 bind args, got %v", dialect, compiled.Args)
		}
		want := `SELECT doc_id, docnm_kwd, product_tks, "2024_sales" FROM ragflow_tenant_a ` +
			`WHERE ((MATCH(product_tks, 'smart phone', 'operator=OR;minimum_should_match=30%')) OR (category = 'O''Brien''s')) ` +
			`AND (kb_id = '` + testAllowedKB + `') ` +
			`ORDER BY "2024_sales" DESC LIMIT 25`
		if compiled.SQL != want {
			t.Fatalf("%s SQL mismatch:\n got: %s\nwant: %s", dialect, compiled.SQL, want)
		}
	}
}

func TestCompile_OceanBaseParameterizedJSON(t *testing.T) {
	scope := testScope("ragflow_tenant_a", testAllowedKB, testOtherKB)
	vq, err := Validate(scope, Query{
		Select: []string{"category"},
		Aggregates: []Aggregate{
			{Fn: AggSum, Field: "Unit Price", As: "total_price"},
		},
		Where: &FilterNode{
			And: []FilterNode{
				{Field: "price_flt", Op: OpGte, Value: 100.5},
				{Field: "category", Op: OpContains, Value: "50%_off"},
			},
		},
		GroupBy: []string{"category"},
		Having:  &FilterNode{Field: "total_price", Op: OpGt, Value: 500},
		OrderBy: []OrderSpec{{Field: "total_price", Desc: true}},
		Limit:   10,
	}, true)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if vq.IncludesCitations() {
		t.Fatal("aggregate query must not include citation columns directly")
	}

	compiled, err := Compile(vq, DialectOceanBase)
	if err != nil {
		t.Fatalf("Compile(oceanbase): %v", err)
	}

	wantSQL := "SELECT JSON_UNQUOTE(JSON_EXTRACT(`chunk_data`, '$.category')) AS `category`, " +
		"SUM(CAST(JSON_UNQUOTE(JSON_EXTRACT(`chunk_data`, '$.\"Unit Price\"')) AS DOUBLE)) AS `total_price` " +
		"FROM `ragflow_tenant_a` " +
		"WHERE ((CAST(JSON_UNQUOTE(JSON_EXTRACT(`chunk_data`, '$.price_flt')) AS DOUBLE) >= ?) " +
		"AND (JSON_UNQUOTE(JSON_EXTRACT(`chunk_data`, '$.category')) LIKE ?)) " +
		"AND (`kb_id` IN (?, ?)) " +
		"GROUP BY JSON_UNQUOTE(JSON_EXTRACT(`chunk_data`, '$.category')) " +
		"HAVING SUM(CAST(JSON_UNQUOTE(JSON_EXTRACT(`chunk_data`, '$.\"Unit Price\"')) AS DOUBLE)) > ? " +
		"ORDER BY `total_price` DESC LIMIT 10"
	if compiled.SQL != wantSQL {
		t.Fatalf("oceanbase SQL mismatch:\n got: %s\nwant: %s", compiled.SQL, wantSQL)
	}
	if len(compiled.Args) != 5 {
		t.Fatalf("args len = %d, want 5 (%v)", len(compiled.Args), compiled.Args)
	}
	if compiled.Args[0] != 100.5 || compiled.Args[1] != `%50\%\_off%` ||
		compiled.Args[2] != testAllowedKB || compiled.Args[3] != testOtherKB || compiled.Args[4] != float64(500) {
		t.Fatalf("unexpected args: %#v", compiled.Args)
	}
}

func TestCompile_InfinitySingleKBAndPreservesLikeWildcards(t *testing.T) {
	// Multi-KB must fail closed for Infinity.
	multiScope := testScope("ragflow_tenant_a", testAllowedKB, testOtherKB)
	multiVQ, err := Validate(multiScope, Query{Select: []string{"category"}}, true)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := Compile(multiVQ, DialectInfinity); err == nil {
		t.Fatal("SECURITY: Infinity must reject multi-KB queries")
	}

	// Single-KB locks the physical per-dataset table and preserves LIKE '%...%'.
	singleScope := testScope("ragflow_tenant_a_"+testAllowedKB, testAllowedKB)
	vq, err := Validate(singleScope, Query{
		Select: []string{"category"},
		Where: &FilterNode{
			And: []FilterNode{
				{Field: "category", Op: OpContains, Value: "Alice"},
				{Field: "price_flt", Op: OpIsNotNull},
			},
		},
		Limit: 5,
	}, true)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	compiled, err := Compile(vq, DialectInfinity)
	if err != nil {
		t.Fatalf("Compile(infinity): %v", err)
	}
	wantSQL := `SELECT doc_id, docnm, json_extract_string(chunk_data, '$.category') AS "category" ` +
		`FROM ragflow_tenant_a_` + testAllowedKB + ` ` +
		`WHERE (json_extract_string(chunk_data, '$.category') LIKE '%Alice%') ` +
		`AND (json_extract_isnull(chunk_data, '$.price_flt') = false) ` +
		`LIMIT 5`
	if compiled.SQL != wantSQL {
		t.Fatalf("infinity SQL mismatch:\n got: %s\nwant: %s", compiled.SQL, wantSQL)
	}
}

func TestCompile_PostgresFamilyParameterized(t *testing.T) {
	scope := testScope("ragflow_tenant_a", testAllowedKB)
	vq, err := Validate(scope, Query{
		Select: []string{"category", "price_flt"},
		Where: &FilterNode{
			Field: "category",
			Op:    OpIn,
			Value: []string{"books", "music"},
		},
		Limit: 20,
	}, true)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	for _, dialect := range []Dialect{DialectVastbase, DialectSereneDB} {
		compiled, err := Compile(vq, dialect)
		if err != nil {
			t.Fatalf("Compile(%s): %v", dialect, err)
		}
		wantSQL := `SELECT "doc_id", "docnm_kwd", "category", "price_flt" FROM "ragflow_tenant_a" ` +
			`WHERE ("category" IN ($1, $2)) AND ("kb_id" = $3) LIMIT 20`
		if compiled.SQL != wantSQL {
			t.Fatalf("%s SQL mismatch:\n got: %s\nwant: %s", dialect, compiled.SQL, wantSQL)
		}
		if len(compiled.Args) != 3 || compiled.Args[0] != "books" || compiled.Args[1] != "music" || compiled.Args[2] != testAllowedKB {
			t.Fatalf("%s args = %#v", dialect, compiled.Args)
		}
	}
}

func TestDeriveCitationQuery_PreservesScopeAndWhere(t *testing.T) {
	scope := testScope("ragflow_tenant_a", testAllowedKB, testOtherKB)

	// No WHERE => returns false.
	rowCount, err := BuildRowCountQuery(scope)
	if err != nil {
		t.Fatalf("BuildRowCountQuery: %v", err)
	}
	if _, ok := rowCount.DeriveCitationQuery(DefaultCitationLimit); ok {
		t.Fatal("expected DeriveCitationQuery to skip queries without WHERE")
	}

	// With WHERE => preserves predicate + authorized scope + multi-KB kb_id column.
	aggQ, err := Validate(scope, Query{
		Aggregates: []Aggregate{{Fn: AggCount, As: "cnt"}},
		Where:      &FilterNode{Field: "category", Op: OpEq, Value: "books"},
	}, true)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	citeQ, ok := aggQ.DeriveCitationQuery(DefaultCitationLimit)
	if !ok {
		t.Fatal("expected citation query to be derived")
	}
	compiled, err := Compile(citeQ, DialectElasticsearch)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	want := "SELECT doc_id, docnm_kwd, kb_id FROM ragflow_tenant_a " +
		"WHERE (category = 'books') AND (kb_id = '" + testAllowedKB + "' OR kb_id = '" + testOtherKB + "') LIMIT 20"
	if compiled.SQL != want {
		t.Fatalf("citation SQL mismatch:\n got: %s\nwant: %s", compiled.SQL, want)
	}
}

func TestCompiledSQL_ExecutesIsolatedAgainstSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(1)

	if err := db.Exec(`CREATE TABLE ragflow_tenant_a (
		doc_id TEXT,
		docnm_kwd TEXT,
		kb_id TEXT,
		category TEXT,
		price_flt REAL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO ragflow_tenant_a VALUES
		('d-allowed-1', 'allowed.xlsx', ?, 'phone', 500.0),
		('d-allowed-2', 'allowed.xlsx', ?, 'book', 20.0),
		('d-victim-1', 'secret.xlsx', ?, 'phone', 999.0),
		('d-victim-2', 'secret.xlsx', ?, 'book', 888.0)`,
		testAllowedKB, testAllowedKB, testOtherKB, testOtherKB).Error; err != nil {
		t.Fatal(err)
	}

	scope := testScope("ragflow_tenant_a", testAllowedKB)
	// Even with an OR predicate attempting to widen results, isolation must hold.
	vq, err := ParseAndValidate(`{
		"select": ["category", "price_flt"],
		"where": {
			"or": [
				{"field": "category", "op": "eq", "value": "phone"},
				{"field": "category", "op": "eq", "value": "book"}
			]
		},
		"order_by": [{"field": "price_flt", "desc": true}]
	}`, scope, true)
	if err != nil {
		t.Fatalf("ParseAndValidate: %v", err)
	}

	compiled, err := Compile(vq, DialectElasticsearch)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	var rows []map[string]interface{}
	if err := db.Raw(compiled.SQL).Scan(&rows).Error; err != nil {
		t.Fatalf("execute compiled SQL (%s): %v", compiled.SQL, err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (only allowed KB): %#v", len(rows), rows)
	}
	for _, r := range rows {
		docID, _ := r["doc_id"].(string)
		if !strings.HasPrefix(docID, "d-allowed-") {
			t.Fatalf("SECURITY: leaked row from unauthorized KB: %#v", r)
		}
	}

	// Verify adversarial literal value containing SQL injection payload is safely escaped.
	injVQ, err := ParseAndValidate(`{
		"select": ["category"],
		"where": {"field": "category", "op": "eq", "value": "' OR 1=1 --"}
	}`, scope, true)
	if err != nil {
		t.Fatalf("ParseAndValidate(inj): %v", err)
	}
	injCompiled, err := Compile(injVQ, DialectElasticsearch)
	if err != nil {
		t.Fatalf("Compile(inj): %v", err)
	}
	var injRows []map[string]interface{}
	if err := db.Raw(injCompiled.SQL).Scan(&injRows).Error; err != nil {
		t.Fatalf("execute injection SQL (%s): %v", injCompiled.SQL, err)
	}
	if len(injRows) != 0 {
		t.Fatalf("SECURITY: SQL injection literal returned rows: %#v", injRows)
	}
}
