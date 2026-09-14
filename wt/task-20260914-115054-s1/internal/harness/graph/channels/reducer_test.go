package channels

import (
	"reflect"
	"testing"

	"ragflow/internal/harness/graph/types"
)

// TestReducerChannel_IntraStep verifies reducer combines multiple values in a single Update.
func TestReducerChannel_IntraStep(t *testing.T) {
	base := NewLastValue("")
	base.SetKey("base")
	reducer := types.ReducerFunc(func(a, b interface{}) interface{} {
		ai, _ := a.(int)
		bi, _ := b.(int)
		return ai + bi
	})
	ch := NewReducerChannel(base, reducer)
	ch.SetKey("reduced")

	// Intra-step: combine 5 + 3 = 8
	updated, err := ch.Update([]interface{}{5, 3})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !updated {
		t.Error("expected updated")
	}

	val, err := ch.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != 8 {
		t.Errorf("expected 8 (5+3), got %v", val)
	}

	// Another step: single value combined with current via reducer.
	updated, err = ch.Update([]interface{}{2})
	if err != nil {
		t.Fatalf("Update 2: %v", err)
	}
	val, _ = ch.Get()
	if val != 10 {
		t.Errorf("expected 10 (8+2 via reducer), got %v", val)
	}
}

// TestReducerChannel_Append verifies AppendReducer appends one item to a slice.
func TestReducerChannel_Append(t *testing.T) {
	base := NewLastValue("")
	base.SetKey("base")
	ch := NewReducerChannel(base, AppendReducer)
	ch.SetKey("append_ch")

	// Intra-step: append "b" to ["a"] → ["a", "b"]
	ch.Update([]interface{}{[]interface{}{"a"}, "b"})

	val, _ := ch.Get()
	list, ok := val.([]interface{})
	if !ok {
		t.Fatalf("expected list, got %T", val)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 items, got %d", len(list))
	}
	if list[0] != "a" || list[1] != "b" {
		t.Errorf("expected [a, b], got %v", list)
	}
}

// TestReducerChannel_Merge verifies MergeReducer merges maps in one Update.
func TestReducerChannel_Merge(t *testing.T) {
	base := NewLastValue("")
	base.SetKey("base")
	ch := NewReducerChannel(base, MergeReducer)
	ch.SetKey("merge_ch")

	// Intra-step: merge maps
	ch.Update([]interface{}{map[string]interface{}{"a": 1}, map[string]interface{}{"b": 2}})

	val, _ := ch.Get()
	m, ok := val.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", val)
	}
	if m["a"] != 1 || m["b"] != 2 {
		t.Errorf("expected merged map {a:1, b:2}, got %v", m)
	}
}

// TestReducerChannel_SingleValue verifies single value passes through.
func TestReducerChannel_SingleValue(t *testing.T) {
	base := NewLastValue("")
	base.SetKey("base")
	ch := NewReducerChannel(base, AppendReducer)
	ch.SetKey("single")
	ch.Update([]interface{}{42})

	val, _ := ch.Get()
	if val != 42 {
		t.Errorf("expected 42, got %v", val)
	}
}

// TestCreateReducerChannel verifies CreateReducerChannel factory.
func TestCreateReducerChannel(t *testing.T) {
	ch, err := CreateReducerChannel("counter", reflect.TypeOf(0), nil)
	if err != nil {
		t.Fatalf("CreateReducerChannel: %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
}
