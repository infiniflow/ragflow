package elasticsearch

import (
	"reflect"
	"testing"

	"ragflow/internal/engine/types"
)

// TestParseOrderByExprSkipsLogicalID guards that the logical "id" column is NOT
// translated into an Elasticsearch _id sort. Ordering on _id requires
// indices.id_field_data, which is disabled by default on the bundled
// Elasticsearch 8.x deployment; emitting a _id sort would make every id-ordered
// query (knowledge_compile merge reads, dataset structure graph) fail at runtime.
// The caller that needs deep pagination must supply a real stored, sortable field
// instead of relying on the logical id.
func TestParseOrderByExprSkipsLogicalID(t *testing.T) {
	cases := []struct {
		name string
		expr *types.OrderByExpr
		want []map[string]interface{}
	}{
		{
			name: "asc id is skipped (no _id sort)",
			expr: (&types.OrderByExpr{}).Asc("id"),
			want: nil,
		},
		{
			name: "desc id is skipped (no _id sort)",
			expr: (&types.OrderByExpr{}).Desc("id"),
			want: nil,
		},
		{
			name: "id mixed with other fields keeps the others, drops id",
			expr: (&types.OrderByExpr{}).Asc("weight_flt").Asc("id"),
			want: []map[string]interface{}{
				{"weight_flt": map[string]interface{}{"order": "asc", "unmapped_type": "float"}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseOrderByExpr(tc.expr)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseOrderByExpr(%v) = %#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

// TestParseOrderByExprIDNotDroppedToIDMetadata is the explicit regression guard:
// parseOrderByExpr must never emit a {"_id": ...} sort for the logical id column.
func TestParseOrderByExprIDNotDroppedToIDMetadata(t *testing.T) {
	got := parseOrderByExpr((&types.OrderByExpr{}).Asc("id"))
	for _, sort := range got {
		if _, ok := sort["_id"]; ok {
			t.Fatalf("parseOrderByExpr(id) emitted a _id sort %#v; this breaks on default ES 8.x", sort)
		}
	}
}
