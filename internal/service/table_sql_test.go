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
	"errors"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

// A published column, addressed the way the index stores it.
var (
	testRegionKey = "c_" + strings.Repeat("a1", 32)
	testSalesKey  = "c_" + strings.Repeat("b2", 32)
)

func testTableFieldMap() map[string]interface{} {
	return map[string]interface{}{
		testRegionKey: "地区",
		testSalesKey:  "销售额",
	}
}

func testRegionPath() string { return "'$." + testRegionKey + "'" }

func newTestTableQuery(t *testing.T, docEngine *sqlFakeEngine, docIDs []string) *tableSQL {
	t.Helper()
	query, err := newTableSQL(docEngine, &entity.Chat{TenantID: "tenant1"},
		[]*entity.Knowledgebase{{ID: "0f1e2d3c4b5a69788796a5b4c3d2e1f0"}}, docIDs, testTableFieldMap())
	if err != nil || query == nil {
		t.Fatalf("newTableSQL: query=%v err=%v", query, err)
	}
	return query
}

func newTestInfinityQuery(t *testing.T, docIDs []string) *tableSQL {
	t.Helper()
	if docIDs == nil {
		docIDs = []string{"doc-one"}
	}
	chat := &entity.Chat{TenantID: "tenant1"}
	kbs := []*entity.Knowledgebase{{ID: "0f1e2d3c4b5a69788796a5b4c3d2e1f0"}}
	query, err := newTableSQL(&sqlFakeEngine{engineType: "infinity"}, chat, kbs, docIDs, testTableFieldMap())
	if err != nil {
		t.Fatalf("newTableSQL: %v", err)
	}
	if query == nil {
		t.Fatal("newTableSQL returned no range")
	}
	return query
}

func newTestOceanBaseQuery(t *testing.T, docIDs []string) *tableSQL {
	t.Helper()
	if docIDs == nil {
		docIDs = []string{"doc-one"}
	}
	chat := &entity.Chat{TenantID: "tenant1"}
	kbs := []*entity.Knowledgebase{{ID: "0f1e2d3c4b5a69788796a5b4c3d2e1f0"}}
	query, err := newTableSQL(&sqlFakeEngine{engineType: "oceanbase"}, chat, kbs, docIDs, testTableFieldMap())
	if err != nil {
		t.Fatalf("newTableSQL: %v", err)
	}
	if query == nil {
		t.Fatal("newTableSQL returned no range")
	}
	return query
}

func TestTableSQLRewriteCarriesTheWholeRange(t *testing.T) {
	query := newTestOceanBaseQuery(t, []string{"doc-one", "doc-two"})
	statement, err := query.policy.check("select doc_id, json_extract_string(chunk_data, " +
		testRegionPath() + ") from ragflow_tenant1 where weight_int = 1 or rank_flt = 2")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	want := "select doc_id, json_extract_string ( chunk_data, '$." + testRegionKey + "' ) " +
		"from ragflow_tenant1 " +
		"where kb_id = '0f1e2d3c4b5a69788796a5b4c3d2e1f0' " +
		"and doc_id in ('doc-one', 'doc-two') " +
		"and available_int = 1 and table_row_int = 1 " +
		"and (weight_int = 1 or rank_flt = 2)"
	if !strings.EqualFold(statement.text, want) {
		t.Errorf("statement =\n  %q\nwant\n  %q", statement.text, want)
	}
	if statement.aggregating {
		t.Error("no aggregate here")
	}
	if statement.where != "weight_int = 1 or rank_flt = 2" {
		t.Errorf("where = %q", statement.where)
	}
}

func TestTableSQLInfinityTableNameCarriesTheKnowledgeBase(t *testing.T) {
	query := newTestInfinityQuery(t, nil)
	if !strings.Contains(query.policy.tableName, "0f1e2d3c4b5a69788796a5b4c3d2e1f0") {
		t.Fatalf("table = %q", query.policy.tableName)
	}
	statement, err := query.policy.check("select doc_id from " + query.policy.tableName)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if strings.Contains(statement.text, "kb_id") {
		t.Errorf("Infinity reads one table per knowledge base, got %q", statement.text)
	}
	if !strings.Contains(statement.text, "doc_id = 'doc-one'") {
		t.Errorf("publishing document is not enforced: %q", statement.text)
	}
	if !strings.Contains(statement.text, "table_row_int = 1") {
		t.Errorf("rows are not separated from other chunks: %q", statement.text)
	}
}

func TestTableSQLRefusesAnythingOutsideOneTable(t *testing.T) {
	query := newTestInfinityQuery(t, nil)
	table := query.policy.tableName
	cases := map[string]string{
		"other table":      "select doc_id from ragflow_someone_else",
		"second table":     "select doc_id from " + table + ", other_t",
		"alias":            "select doc_id from " + table + " t",
		"join":             "select doc_id from " + table + " join u on u.id = " + table + ".id",
		"subquery":         "select doc_id from " + table + " where doc_id in (select id from u)",
		"union":            "select doc_id from " + table + " union select doc_id from u",
		"cte":              "with x as (select 1) select doc_id from " + table,
		"update":           "update " + table + " set available_int = 0",
		"write out":        "select doc_id into outfile '/tmp/x' from " + table,
		"two statements":   "select doc_id from " + table + "; drop table " + table,
		"comment":          "select doc_id from " + table + " -- where x = 1",
		"unknown column":   "select secret_column from " + table,
		"other table col":  "select u.doc_id from " + table,
		"invented field":   "select json_extract_string(chunk_data, '$.salary') from " + table,
		"other table json": "select json_extract_string(other_data, '$." + testRegionKey + "') from " + table,
		"nested path":      "select json_extract_string(chunk_data, '$." + testRegionKey + ".x') from " + table,
		"unknown func":     "select load_file('/etc/passwd') from " + table,
		"unquoted path":    "select json_extract_string(chunk_data, " + testRegionKey + ") from " + table,
	}
	for name, sqlText := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := query.policy.check(sqlText); err == nil {
				t.Errorf("check(%q) succeeded, want a refusal", sqlText)
			}
		})
	}
}

func TestTableSQLQuotesAndLiteralsSurvive(t *testing.T) {
	query := newTestInfinityQuery(t, nil)
	statement, err := query.policy.check("select doc_id, docnm from " + query.policy.tableName +
		" where json_extract_string(chunk_data, " + testRegionPath() + ") like '%Al%'")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(statement.text, "'%Al%'") {
		t.Errorf("a LIKE pattern lost its wildcards: %q", statement.text)
	}
	if !strings.Contains(statement.text, "'$."+testRegionKey+"'") {
		t.Errorf("the column path was not kept: %q", statement.text)
	}
}

func TestTableSQLValueCannotEndItsOwnLiteral(t *testing.T) {
	query := newTestInfinityQuery(t, nil)
	// A value that looks like the end of a literal plus a new condition is
	// still one value: the scanner reads it as a string and the statement is
	// rewritten with the caller's condition parenthesised.
	statement, err := query.policy.check("select doc_id from " + query.policy.tableName +
		" where docnm = 'a'' or 1=1 --'")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(statement.text, "(docnm = 'a'' or 1=1 --')") {
		t.Errorf("literal was rewritten: %q", statement.text)
	}
}

func TestTableSQLAggregatingFlag(t *testing.T) {
	query := newTestInfinityQuery(t, nil)
	statement, err := query.policy.check("select count(*) from " + query.policy.tableName + " group by doc_id")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !statement.aggregating {
		t.Error("count(*) must be read as an aggregate")
	}
	if !strings.Contains(statement.text, "group by doc_id") {
		t.Errorf("group by was dropped: %q", statement.text)
	}
}

func TestTableSQLQuotedAliasKeepsItsEngineStyle(t *testing.T) {
	ocean := newTestOceanBaseQuery(t, nil)
	statement, err := ocean.policy.check("select json_extract_string(chunk_data, " + testRegionPath() +
		") as \"地区\" from ragflow_tenant1")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(statement.text, "`地区`") {
		t.Errorf("OceanBase reads quoted names with backticks: %q", statement.text)
	}
}

func TestTableSQLRefusesTooManyDocuments(t *testing.T) {
	docIDs := make([]string, 0, 3000)
	for i := 0; i < 3000; i++ {
		docIDs = append(docIDs, "document-with-a-long-identifier-for-this-row-0000")
	}
	query := newTestInfinityQuery(t, docIDs)
	if _, err := query.policy.check("select doc_id from " + query.policy.tableName); err == nil {
		t.Fatal("a statement over the budget was accepted")
	} else if !strings.Contains(err.Error(), "budget") {
		t.Errorf("err = %v, want the budget", err)
	}
}

func TestTableSQLRefusesADocumentIDThatIsNotOne(t *testing.T) {
	query := newTestInfinityQuery(t, []string{"drop table x"})
	if _, err := query.policy.check("select doc_id from " + query.policy.tableName); err == nil {
		t.Error("a document id that is not an identifier was accepted")
	}
}

func TestTableSQLRangeIsBuiltOrRefused(t *testing.T) {
	chat := &entity.Chat{TenantID: "tenant1"}
	kb := &entity.Knowledgebase{ID: "0f1e2d3c4b5a69788796a5b4c3d2e1f0"}

	t.Run("no field map", func(t *testing.T) {
		query, err := newTableSQL(&sqlFakeEngine{engineType: "infinity"}, chat, []*entity.Knowledgebase{kb}, nil, nil)
		if err != nil || query != nil {
			t.Errorf("query = %v, err = %v, want no range and no error", query, err)
		}
	})

	t.Run("several knowledge bases", func(t *testing.T) {
		query, err := newTableSQL(&sqlFakeEngine{engineType: "infinity"}, chat,
			[]*entity.Knowledgebase{kb, {ID: "1f1e2d3c4b5a69788796a5b4c3d2e1f0"}}, nil, testTableFieldMap())
		if err != nil || query != nil {
			t.Errorf("query = %v, err = %v, want no range and no error", query, err)
		}
	})

	t.Run("unsupported engine", func(t *testing.T) {
		query, err := newTableSQL(&sqlFakeEngine{engineType: "elasticsearch"}, chat, []*entity.Knowledgebase{kb}, nil, testTableFieldMap())
		if err != nil || query != nil {
			t.Errorf("query = %v, err = %v, want no range and no error", query, err)
		}
	})

	t.Run("no document publishes columns", func(t *testing.T) {
		query, err := newTableSQL(&sqlFakeEngine{engineType: "infinity"}, chat, []*entity.Knowledgebase{kb}, []string{}, testTableFieldMap())
		if query != nil {
			t.Errorf("query = %v", query)
		}
		if err == nil {
			t.Fatal("an empty document range was accepted")
		}
	})
	t.Run("missing publishing document set", func(t *testing.T) {
		query, err := newTableSQL(&sqlFakeEngine{engineType: "infinity"}, chat, []*entity.Knowledgebase{kb}, nil, testTableFieldMap())
		if query != nil || err == nil {
			t.Fatalf("query=%v err=%v: missing publishers must not allow every indexed document", query, err)
		}
	})

	t.Run("infinity table without the knowledge base", func(t *testing.T) {
		// A knowledge base id that is not a UUID makes ragflowTableName fall
		// back to the tenant's shared table, which is not a range.
		query, err := newTableSQL(&sqlFakeEngine{engineType: "infinity"}, chat,
			[]*entity.Knowledgebase{{ID: "not-a-uuid-that-is-loose"}}, []string{"doc-one"}, testTableFieldMap())
		if query != nil {
			t.Errorf("query = %v, want no range over a shared table", query)
		}
		if err == nil || !strings.Contains(err.Error(), "does not name knowledge base") {
			t.Errorf("err = %v", err)
		}
	})
}

func TestTableSQLRunRechecksWithEachStatement(t *testing.T) {
	var seen []string
	engine := &sqlFakeEngine{
		engineType: "infinity",
		runSQL: func(ctx context.Context, table, sqlText string, kbIDs []string) ([]map[string]interface{}, error) {
			seen = append(seen, sqlText)
			if strings.Contains(sqlText, "secret_column") {
				return nil, errors.New("unknown column")
			}
			return []map[string]interface{}{{"doc_id": "d1"}}, nil
		},
	}
	chat := &entity.Chat{TenantID: "tenant1"}
	kbs := []*entity.Knowledgebase{{ID: "0f1e2d3c4b5a69788796a5b4c3d2e1f0"}}
	query, err := newTableSQL(engine, chat, kbs, []string{"doc-one"}, testTableFieldMap())
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := query.run(t.Context(), "select secret_column from "+query.policy.tableName); err == nil {
		t.Fatal("an unchecked column reached the engine")
	}
	if len(seen) != 0 {
		t.Fatalf("refused statements reached the engine: %v", seen)
	}

	rows, statement, err := query.run(t.Context(), "select doc_id from "+query.policy.tableName+" where weight_int = 1")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(rows) != 1 || statement == nil {
		t.Fatalf("rows = %v, statement = %v", rows, statement)
	}
	if !strings.Contains(seen[0], "doc_id = 'doc-one'") || !strings.Contains(seen[0], "(weight_int = 1)") {
		t.Errorf("the engine was handed %q", seen[0])
	}
}

func TestRestrictToRequestedDocs(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		publishing, requested, want []string
	}{
		{"all publishers", []string{"d1", "d2"}, nil, []string{"d1", "d2"}},
		{"explicit empty", []string{"d1", "d2"}, []string{}, []string{}},
		{"intersection", []string{"d1", "d2"}, []string{"d2", "outside"}, []string{"d2"}},
		{"no overlap", []string{"d1"}, []string{"outside"}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := restrictToRequestedDocs(tc.publishing, tc.requested); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}
