package component

import (
	"math"
	"reflect"
	"testing"

	"ragflow/internal/agent/canvas"
)

func TestListOperationsMixedSortPermutations(t *testing.T) {
	inputs := [][]any{
		{10, 2, "11"}, {10, "11", 2}, {2, 10, "11"},
		{2, "11", 10}, {"11", 10, 2}, {"11", 2, 10},
	}
	for _, input := range inputs {
		for _, method := range []string{"asc", "desc"} {
			l := &ListOperationsComponent{param: listOperationsParam{SortMethod: method}}
			want := []any{2, 10, "11"}
			if method == "desc" {
				want = []any{"11", 10, 2}
			}
			if got := l.opSort(input); !reflect.DeepEqual(got, want) {
				t.Errorf("%s input %v: got %v, want %v", method, input, got, want)
			}
		}
	}
}

func TestListOperationsMixedSortComparatorLaws(t *testing.T) {
	values := []any{math.Inf(-1), -1, 0, int64(2), 10.0, math.Inf(1), math.NaN(), nil, false, true, "11", "2", []any{1.0}, map[string]any{"a": 1.0}}
	for _, a := range values {
		if lessScalar(a, a) {
			t.Fatalf("irreflexivity failed for %v", a)
		}
		for _, b := range values {
			if lessScalar(a, b) && lessScalar(b, a) {
				t.Fatalf("asymmetry failed: %v, %v", a, b)
			}
			for _, c := range values {
				if lessScalar(a, b) && lessScalar(b, c) && !lessScalar(a, c) {
					t.Fatalf("transitivity failed: %v, %v, %v", a, b, c)
				}
				abEqual := !lessScalar(a, b) && !lessScalar(b, a)
				bcEqual := !lessScalar(b, c) && !lessScalar(c, b)
				if abEqual && bcEqual && (lessScalar(a, c) || lessScalar(c, a)) {
					t.Fatalf("equivalence transitivity failed: %v, %v, %v", a, b, c)
				}
			}
		}
	}
}

func TestListOperationsMixedSortNaN(t *testing.T) {
	l := &ListOperationsComponent{param: listOperationsParam{SortMethod: "asc"}}
	for _, input := range [][]any{{math.NaN(), 2, math.Inf(1), math.Inf(-1)}, {2, math.NaN(), math.Inf(-1), math.Inf(1)}} {
		got := l.opSort(input)
		if got[0] != math.Inf(-1) || got[1] != 2 || got[2] != math.Inf(1) || !math.IsNaN(got[3].(float64)) {
			t.Fatalf("ascending NaN sort: %v", got)
		}
		l.param.SortMethod = "desc"
		got = l.opSort(input)
		if !math.IsNaN(got[0].(float64)) || got[1] != math.Inf(1) || got[2] != 2 || got[3] != math.Inf(-1) {
			t.Fatalf("descending NaN sort: %v", got)
		}
		l.param.SortMethod = "asc"
	}
}

func TestListOperationsMixedSortNonNumeric(t *testing.T) {
	l := &ListOperationsComponent{param: listOperationsParam{SortMethod: "asc"}}
	input := []any{true, "2", nil, 10, false, "11", 2}
	want := []any{2, 10, "11", "2", nil, false, true}
	if got := l.opSort(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestListOperationsMixedSortFieldsInvoke(t *testing.T) {
	rows := []any{
		map[string]any{"id": "text", "score": "11", "tie": 0},
		map[string]any{"id": "missing", "tie": 0},
		map[string]any{"id": "ten", "score": 10, "tie": 0},
		map[string]any{"id": "two-b", "score": 2, "tie": 2},
		map[string]any{"id": "two-a", "score": 2.0, "tie": 1},
		map[string]any{"id": "null", "score": nil, "tie": 0},
	}
	for _, method := range []string{"asc", "desc"} {
		c, err := NewListOperationsComponent(map[string]any{"query": "cpn_0@rows", "operations": "sort", "n": 1, "sort_by": "score,tie", "sort_method": method})
		if err != nil {
			t.Fatal(err)
		}
		state := canvas.NewCanvasState("mixed-sort", "mixed-sort")
		state.Outputs["cpn_0"] = map[string]any{"rows": rows}
		out, err := c.Invoke(canvas.WithState(t.Context(), state), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		gotRows := out["result"].([]any)
		got := make([]string, len(gotRows))
		for i, row := range gotRows {
			got[i] = row.(map[string]any)["id"].(string)
		}
		want := []string{"two-a", "two-b", "ten", "text", "missing", "null"}
		if method == "desc" {
			want = []string{"missing", "null", "text", "ten", "two-b", "two-a"}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s got %v, want %v", method, got, want)
		}
		if !reflect.DeepEqual(out["first"], gotRows[0]) || !reflect.DeepEqual(out["last"], gotRows[len(gotRows)-1]) {
			t.Fatal("first/last do not match sorted rows")
		}
	}
}

func TestListOperationsMixedSortDefaultMapKey(t *testing.T) {
	input := []any{map[string]any{"score": "11"}, map[string]any{"score": 10}, map[string]any{"score": 2}}
	want := []any{input[2], input[1], input[0]}
	l := &ListOperationsComponent{param: listOperationsParam{SortMethod: "asc"}}
	if got := l.opSort(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
