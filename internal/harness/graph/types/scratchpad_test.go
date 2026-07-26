package types

import (
	"testing"
	"time"
)

func TestScratchpad_BasicOperations(t *testing.T) {
	s := NewPregelScratchpad()

	// Test Set and Get
	s.Set("key1", "value1")
	value, ok := s.Get("key1")
	if !ok {
		t.Error("Get should return true for existing key")
	}
	if value != "value1" {
		t.Errorf("Expected 'value1', got '%v'", value)
	}

	// Test Has
	if !s.Has("key1") {
		t.Error("Has should return true for existing key")
	}
	if s.Has("nonexistent") {
		t.Error("Has should return false for nonexistent key")
	}

	// Test Delete
	s.Delete("key1")
	if s.Has("key1") {
		t.Error("Key should be deleted")
	}
}

func TestScratchpad_GetAll(t *testing.T) {
	s := NewPregelScratchpad()

	s.Set("key1", "value1")
	s.Set("key2", "value2")
	s.Set("key3", "value3")

	all := s.GetAll()
	if len(all) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(all))
	}

	// Verify it's a copy
	s.Set("key4", "value4")
	if len(all) != 3 {
		t.Error("GetAll should return a copy")
	}
}

func TestScratchpad_Clear(t *testing.T) {
	s := NewPregelScratchpad()

	s.Set("key1", "value1")
	s.Set("key2", "value2")
	s.Clear()

	if s.Has("key1") || s.Has("key2") {
		t.Error("All keys should be cleared")
	}

	// Counters should also be cleared
	s.IncrementCounter("c1")
	s.Clear()
	if s.GetCounter("c1") != 0 {
		t.Error("Counters should be cleared")
	}
}

func TestScratchpad_Counters(t *testing.T) {
	s := NewPregelScratchpad()

	// Test IncrementCounter
	val := s.IncrementCounter("counter1")
	if val != 1 {
		t.Errorf("Expected counter value 1, got %d", val)
	}

	val = s.IncrementCounter("counter1")
	if val != 2 {
		t.Errorf("Expected counter value 2, got %d", val)
	}

	// Test DecrementCounter
	val = s.DecrementCounter("counter1")
	if val != 1 {
		t.Errorf("Expected counter value 1, got %d", val)
	}

	// Test GetCounter
	if s.GetCounter("counter1") != 1 {
		t.Error("GetCounter returned wrong value")
	}

	// Test SetCounter
	s.SetCounter("counter2", 100)
	if s.GetCounter("counter2") != 100 {
		t.Error("SetCounter failed")
	}

	// Test ResetCounter
	s.ResetCounter("counter1")
	if s.GetCounter("counter1") != 0 {
		t.Error("ResetCounter failed")
	}

	// Test ListCounters - clear first to avoid counting previous counters
	s2 := NewPregelScratchpad()
	s2.SetCounter("c1", 1)
	s2.SetCounter("c2", 2)
	counters := s2.ListCounters()
	if len(counters) != 2 {
		t.Errorf("Expected 2 counters, got %d", len(counters))
	}
}

func TestScratchpad_Metadata(t *testing.T) {
	s := NewPregelScratchpad()

	// Test SetMetadata and GetMetadata
	s.SetMetadata("meta1", "value1")
	value, ok := s.GetMetadata("meta1")
	if !ok {
		t.Error("GetMetadata should return true")
	}
	if value != "value1" {
		t.Errorf("Expected 'value1', got '%v'", value)
	}

	// Test GetAllMetadata
	s.SetMetadata("meta2", "value2")
	all := s.GetAllMetadata()
	if len(all) != 2 {
		t.Errorf("Expected 2 metadata entries, got %d", len(all))
	}
}

func TestScratchpad_Stack(t *testing.T) {
	stack := NewStack()

	// Test Push and Pop
	stack.Push(1)
	stack.Push(2)
	stack.Push(3)

	value, ok := stack.Pop()
	if !ok {
		t.Error("Pop should return true")
	}
	if value != 3 {
		t.Errorf("Expected 3, got %v", value)
	}

	// Test Peek
	value, ok = stack.Peek()
	if !ok {
		t.Error("Peek should return true")
	}
	if value != 2 {
		t.Errorf("Expected 2, got %v", value)
	}

	// Peek should not remove
	value, ok = stack.Pop()
	if value != 2 {
		t.Errorf("Expected 2, got %v", value)
	}

	// Test Size
	if stack.Size() != 1 {
		t.Errorf("Expected size 1, got %d", stack.Size())
	}

	// Test IsEmpty
	if stack.IsEmpty() {
		t.Error("Stack should not be empty")
	}

	stack.Pop()
	if !stack.IsEmpty() {
		t.Error("Stack should be empty")
	}
}

func TestScratchpad_StackClear(t *testing.T) {
	stack := NewStack()

	stack.Push(1)
	stack.Push(2)
	stack.Push(3)
	stack.Clear()

	if !stack.IsEmpty() {
		t.Error("Stack should be empty after Clear")
	}
}

func TestScratchpad_StackToSlice(t *testing.T) {
	stack := NewStack()

	stack.Push(1)
	stack.Push(2)
	stack.Push(3)

	slice := stack.ToSlice()
	if len(slice) != 3 {
		t.Errorf("Expected slice of length 3, got %d", len(slice))
	}

	// Should be a copy
	stack.Push(4)
	if len(slice) != 3 {
		t.Error("ToSlice should return a copy")
	}
}

func TestScratchpadStack(t *testing.T) {
	s := NewPregelScratchpad()
	ss := NewScratchpadStack(s, "mystack")

	// Test Push
	ss.Push(1)
	ss.Push(2)
	ss.Push(3)

	// Test Size
	if ss.Size() != 3 {
		t.Errorf("Expected size 3, got %d", ss.Size())
	}

	// Test Pop
	value, ok := ss.Pop()
	if !ok {
		t.Error("Pop should return true")
	}
	if value != 3 {
		t.Errorf("Expected 3, got %v", value)
	}

	// Test Peek
	value, ok = ss.Peek()
	if !ok {
		t.Error("Peek should return true")
	}
	if value != 2 {
		t.Errorf("Expected 2, got %v", value)
	}

	// Test IsEmpty
	if ss.IsEmpty() {
		t.Error("Stack should not be empty")
	}

	ss.Pop()
	ss.Pop()
	if !ss.IsEmpty() {
		t.Error("Stack should be empty")
	}
}

func TestScratchpadStats(t *testing.T) {
	s := NewPregelScratchpad()

	s.Set("key1", "value1")
	s.Set("key2", "value2")
	s.IncrementCounter("c1")
	s.IncrementCounter("c2")
	s.IncrementCounter("c3")
	s.SetMetadata("meta1", "value1")

	stats := s.Stats()
	if stats.DataSize != 2 {
		t.Errorf("Expected DataSize 2, got %d", stats.DataSize)
	}
	if stats.CountersCount != 3 {
		t.Errorf("Expected CountersCount 3, got %d", stats.CountersCount)
	}
	if stats.MetadataSize != 1 {
		t.Errorf("Expected MetadataSize 1, got %d", stats.MetadataSize)
	}
	if stats.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestScratchpad_LastAccess(t *testing.T) {
	s := NewPregelScratchpad()

	firstAccess := s.Stats().LastAccess
	time.Sleep(10 * time.Millisecond)

	s.Set("key1", "value1")
	secondAccess := s.Stats().LastAccess

	if !secondAccess.After(firstAccess) {
		t.Error("LastAccess should be updated")
	}
}
