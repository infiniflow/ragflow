package channels

import (
	"testing"

	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/errors"
	"ragflow/internal/harness/graph/types"
)

// TestBinaryOperatorAggregate_IntAdd verifies IntAdd reducer with BinaryOperatorAggregate.
func TestBinaryOperatorAggregate_IntAdd(t *testing.T) {
	ch := NewBinaryOperatorAggregate(int(0), IntAdd)
	ch.SetKey("counter")

	if !ch.IsAvailable() {
		t.Error("expected IsAvailable to be true (has initial zero value)")
	}

	updated, err := ch.Update([]interface{}{5})
	if err != nil {
		t.Fatalf("Update 5: %v", err)
	}
	if !updated {
		t.Error("expected updated")
	}

	val, err := ch.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != 5 {
		t.Errorf("expected 5, got %v", val)
	}

	// Multiple values in one step: 5 + 3 + 2 = 10
	updated, err = ch.Update([]interface{}{3, 2})
	if err != nil {
		t.Fatalf("Update 3,2: %v", err)
	}
	if !updated {
		t.Error("expected updated")
	}
	val, err = ch.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != 10 {
		t.Errorf("expected 10, got %v", val)
	}
}

// TestBinaryOperatorAggregate_StringConcat verifies StringConcat reducer.
func TestBinaryOperatorAggregate_StringConcat(t *testing.T) {
	ch := NewBinaryOperatorAggregate("", StringConcat)
	ch.SetKey("text")

	updated, err := ch.Update([]interface{}{"hello"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	val, err := ch.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "hello" {
		t.Errorf("expected 'hello', got %v", val)
	}

	// Second write
	updated, err = ch.Update([]interface{}{" world"})
	if err != nil {
		t.Fatalf("Update 2: %v", err)
	}
	_ = updated
	val, _ = ch.Get()
	if val != "hello world" {
		t.Errorf("expected 'hello world', got %v", val)
	}
}

// TestBinaryOperatorAggregate_ListAppend verifies ListAppend reducer.
func TestBinaryOperatorAggregate_ListAppend(t *testing.T) {
	ch := NewBinaryOperatorAggregate([]interface{}{}, ListAppend)
	ch.SetKey("list")

	updated, err := ch.Update([]interface{}{[]interface{}{"a", "b"}})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !updated {
		t.Error("expected updated")
	}

	updated, err = ch.Update([]interface{}{[]interface{}{"c"}})
	if err != nil {
		t.Fatalf("Update 2: %v", err)
	}

	val, _ := ch.Get()
	list, ok := val.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", val)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 items, got %d", len(list))
	}
}

// TestBinaryOperatorAggregate_Negative verifies empty update handling.
func TestBinaryOperatorAggregate_Negative(t *testing.T) {
	ch := NewBinaryOperatorAggregate(int(0), IntAdd)
	ch.SetKey("neg")
	ch.Update([]interface{}{7})

	updated, err := ch.Update([]interface{}{})
	if err != nil {
		t.Fatalf("empty update: %v", err)
	}
	if updated {
		t.Error("expected not updated for empty input")
	}

	val, _ := ch.Get()
	if val != 7 {
		t.Errorf("expected value unchanged (7), got %v", val)
	}
}

// TestBinaryOperatorAggregate_Checkpoint verifies checkpoint creation.
func TestBinaryOperatorAggregate_Checkpoint(t *testing.T) {
	ch := NewBinaryOperatorAggregate(int(0), IntAdd)
	ch.SetKey("sum")
	ch.Update([]interface{}{10})

	cp := ch.Checkpoint()
	if cp == nil {
		t.Fatal("expected non-nil checkpoint")
	}
}

// TestIntAddDirect verifies the IntAdd binary operator function directly.
func TestIntAddDirect(t *testing.T) {
	result := IntAdd(5, 3)
	if result != 8 {
		t.Errorf("expected 8, got %v", result)
	}
}

// TestStringConcatDirect verifies the StringConcat binary operator function directly.
func TestStringConcatDirect(t *testing.T) {
	result := StringConcat("hello ", "world")
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %v", result)
	}
}

// TestListAppendDirect verifies the ListAppend binary operator function directly.
func TestListAppendDirect(t *testing.T) {
	a := []interface{}{1, 2}
	b := []interface{}{3, 4}
	result := ListAppend(a, b)
	list, ok := result.([]interface{})
	if !ok || len(list) != 4 {
		t.Errorf("expected 4 items, got %v", result)
	}
}

func TestBinaryOperatorAggregate_OverwriteTypes(t *testing.T) {
	cases := []struct {
		name    string
		updates []interface{}
		want    int
	}{
		{
			name:    "pointer_overwrite",
			updates: []interface{}{types.NewOverwrite(99)},
			want:    99,
		},
		{
			name:    "value_overwrite",
			updates: []interface{}{types.Overwrite{Value: 7}},
			want:    7,
		},
		{
			name:    "map_overwrite",
			updates: []interface{}{map[string]interface{}{constants.Overwrite: 55}},
			want:    55,
		},
		{
			name:    "overwrite_before_normal",
			updates: []interface{}{types.NewOverwrite(12), 5},
			want:    12,
		},
		{
			name:    "normal_before_overwrite",
			updates: []interface{}{5, types.NewOverwrite(13)},
			want:    13,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch := NewBinaryOperatorAggregate(int(0), IntAdd)
			if _, err := ch.Update([]interface{}{10}); err != nil {
				t.Fatalf("seed Update: %v", err)
			}
			updated, err := ch.Update(tc.updates)
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			if !updated {
				t.Fatal("expected updated=true")
			}
			got, err := ch.Get()
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got != tc.want {
				t.Errorf("Get() = %#v (%T), want %d", got, got, tc.want)
			}
		})
	}
}

func TestBinaryOperatorAggregate_OverwriteWhenEmpty(t *testing.T) {
	t.Run("single_pointer_overwrite", func(t *testing.T) {
		ch := NewBinaryOperatorAggregate(nil, IntAdd)
		if updated, err := ch.Update([]interface{}{types.NewOverwrite(42)}); err != nil || !updated {
			t.Fatalf("Update = (%v, %v), want (true, nil)", updated, err)
		}
		got, err := ch.Get()
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got != 42 {
			t.Errorf("Get() = %#v (%T), want 42", got, got)
		}
	})

	t.Run("single_map_overwrite", func(t *testing.T) {
		ch := NewBinaryOperatorAggregate(nil, IntAdd)
		if updated, err := ch.Update([]interface{}{map[string]interface{}{constants.Overwrite: 42}}); err != nil || !updated {
			t.Fatalf("Update = (%v, %v), want (true, nil)", updated, err)
		}
		got, err := ch.Get()
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got != 42 {
			t.Errorf("Get() = %#v (%T), want 42", got, got)
		}
	})

	t.Run("overwrite_followed_by_normal_value", func(t *testing.T) {
		ch := NewBinaryOperatorAggregate(nil, IntAdd)
		if updated, err := ch.Update([]interface{}{types.NewOverwrite(42), 5}); err != nil || !updated {
			t.Fatalf("Update = (%v, %v), want (true, nil)", updated, err)
		}
		got, err := ch.Get()
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got != 42 {
			t.Errorf("Get() = %#v (%T), want 42", got, got)
		}
	})

	t.Run("duplicate_overwrite_rejected_and_keeps_empty", func(t *testing.T) {
		ch := NewBinaryOperatorAggregate(nil, IntAdd)
		updated, err := ch.Update([]interface{}{
			map[string]interface{}{constants.Overwrite: 42},
			map[string]interface{}{constants.Overwrite: 99},
		})
		if !errors.IsInvalidUpdateError(err) {
			t.Fatalf("Update err = %v, want InvalidUpdateError", err)
		}
		if updated {
			t.Error("expected updated=false on duplicate Overwrite")
		}
		if _, err := ch.Get(); !errors.IsEmptyChannelError(err) {
			t.Errorf("Get() err = %v, want EmptyChannelError (channel must stay empty)", err)
		}
	})
}

func TestBinaryOperatorAggregate_DuplicateOverwriteLeavesStateUnchanged(t *testing.T) {
	cases := []struct {
		name    string
		updates []interface{}
	}{
		{
			name:    "pointer_overwrites_after_normal",
			updates: []interface{}{5, types.NewOverwrite(42), types.NewOverwrite(99)},
		},
		{
			name: "map_overwrites",
			updates: []interface{}{
				map[string]interface{}{constants.Overwrite: 42},
				map[string]interface{}{constants.Overwrite: 99},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch := NewBinaryOperatorAggregate(int(0), IntAdd)
			if _, err := ch.Update([]interface{}{10}); err != nil {
				t.Fatalf("seed Update: %v", err)
			}
			updated, err := ch.Update(tc.updates)
			if !errors.IsInvalidUpdateError(err) {
				t.Fatalf("Update err = %v, want InvalidUpdateError", err)
			}
			if updated {
				t.Error("expected updated=false on duplicate Overwrite")
			}
			got, err := ch.Get()
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got != 10 {
				t.Errorf("Get() = %#v after rejected Update, want unchanged 10", got)
			}
		})
	}
}
