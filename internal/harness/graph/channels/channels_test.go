package channels

import (
	"testing"

	"ragflow/internal/harness/graph/errors"
)

func TestLastValue(t *testing.T) {
	ch := NewLastValue("")
	ch.SetKey("test")

	// Empty channel should return error
	_, err := ch.Get()
	if err == nil {
		t.Error("Expected error for empty channel")
	}
	if !errors.IsEmptyChannelError(err) {
		t.Errorf("Expected EmptyChannelError, got %T", err)
	}

	// Update with single value
	updated, err := ch.Update([]interface{}{"hello"})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !updated {
		t.Error("Expected updated to be true")
	}

	val, err := ch.Get()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != "hello" {
		t.Errorf("Expected 'hello', got %v", val)
	}

	// Update with multiple values - should error (LastValue only accepts one value per step)
	_, err = ch.Update([]interface{}{"a", "b", "c"})
	if err == nil {
		t.Error("Expected error for multiple values in LastValue")
	}

	// IsAvailable should return true
	if !ch.IsAvailable() {
		t.Error("Expected IsAvailable to be true")
	}

	// Copy should have same value
	copyCh := ch.Copy()
	copyVal, _ := copyCh.Get()
	if copyVal != "hello" {
		t.Errorf("Expected copied value 'hello', got %v", copyVal)
	}
}

func TestTopic(t *testing.T) {
	// Topic without accumulation
	ch := NewTopic("", false)
	ch.SetKey("messages")

	// Add values - note: each Update flattens and adds
	ch.Update([]interface{}{"a"})
	ch.Update([]interface{}{"b"})
	ch.Update([]interface{}{"c"})

	val, _ := ch.Get()
	vals := val.([]interface{})
	if len(vals) != 1 || vals[0] != "c" {
		t.Errorf("Expected 1 value ['c'], got %v", vals)
	}

	// Topic with accumulation
	ch2 := NewTopic(0, true)
	ch2.Update([]interface{}{1, 2})
	ch2.Update([]interface{}{3})
	val, _ = ch2.Get()
	ints := val.([]interface{})
	if len(ints) != 3 {
		t.Errorf("Expected 3 values with accumulation, got %d", len(ints))
	}
}

func TestBinaryOperatorAggregate(t *testing.T) {
	// Sum operator - use float64 for JSON compatibility
	sumOp := func(a, b interface{}) interface{} {
		af, _ := a.(float64)
		bf, _ := b.(float64)
		return af + bf
	}

	ch := NewBinaryOperatorAggregate(float64(0), sumOp)
	ch.SetKey("total")

	// Update with values
	ch.Update([]interface{}{float64(10), float64(20), float64(30)})

	val, _ := ch.Get()
	if val != float64(60) {
		t.Errorf("Expected sum 60, got %v", val)
	}

	// Copy should have same value
	copyCh := ch.Copy()
	copyVal, _ := copyCh.Get()
	if copyVal != float64(60) {
		t.Errorf("Expected copied value 60, got %v", copyVal)
	}

	// List append operator
	listOp := func(a, b interface{}) interface{} {
		list := a.([]interface{})
		if newList, ok := b.([]interface{}); ok {
			return append(list, newList...)
		}
		return append(list, b)
	}

	listCh := NewBinaryOperatorAggregate([]interface{}{}, listOp)
	listCh.Update([]interface{}{[]interface{}{"a", "b"}})
	listCh.Update([]interface{}{[]interface{}{"c"}})

	listVal, _ := listCh.Get()
	list := listVal.([]interface{})
	if len(list) != 3 {
		t.Errorf("Expected 3 items, got %d", len(list))
	}
}

func TestEphemeralValue(t *testing.T) {
	// Ephemeral with guard
	ch := NewEphemeralValue("", true)
	ch.SetKey("temp")

	// Empty with guard should error
	_, err := ch.Get()
	if !errors.IsEmptyChannelError(err) {
		t.Error("Expected EmptyChannelError for guarded ephemeral")
	}

	// Set and get
	ch.Update([]interface{}{"value"})
	val, _ := ch.Get()
	if val != "value" {
		t.Errorf("Expected 'value', got %v", val)
	}

	// Second get should error (value is cleared)
	_, err = ch.Get()
	if !errors.IsEmptyChannelError(err) {
		t.Error("Expected EmptyChannelError after read")
	}

	// Ephemeral without guard
	ch2 := NewEphemeralValue("", false)
	val2, _ := ch2.Get()
	if val2 != nil {
		t.Errorf("Expected nil, got %v", val2)
	}
}

func TestUntrackedValue(t *testing.T) {
	ch := NewUntrackedValue("")
	ch.SetKey("untracked")

	ch.Update([]interface{}{"test"})
	val, _ := ch.Get()
	if val != "test" {
		t.Errorf("Expected 'test', got %v", val)
	}

	// Checkpoint should return Missing
	cp := ch.Checkpoint()
	if !IsMissing(cp) {
		t.Error("Expected Missing from checkpoint of untracked value")
	}

	// FromCheckpoint should return empty channel
	restored := ch.FromCheckpoint(cp)
	_, err := restored.Get()
	if !errors.IsEmptyChannelError(err) {
		t.Error("Expected empty channel after restoring from checkpoint")
	}
}

func TestAnyValue(t *testing.T) {
	ch := NewAnyValue(nil)
	ch.SetKey("any")

	// Can receive multiple values, keeps last
	ch.Update([]interface{}{"first", 42, true})
	val, _ := ch.Get()
	if val != true {
		t.Errorf("Expected true (last value), got %v", val)
	}
}

func TestNamedBarrierValue(t *testing.T) {
	ch := NewNamedBarrierValue(nil, []string{"node_a", "node_b", "node_c"})
	ch.SetKey("barrier")

	// Initially not available
	if ch.IsAvailable() {
		t.Error("Barrier should not be available initially")
	}

	// Add one node
	_, err := ch.Update([]interface{}{"node_a"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if ch.IsAvailable() {
		t.Error("Barrier should not be available after one node")
	}

	// Add remaining nodes
	_, err = ch.Update([]interface{}{"node_b", "node_c"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !ch.IsAvailable() {
		t.Error("Barrier should be available after all nodes")
	}

	// Get should return nil when available
	val, err := ch.Get()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if val != nil {
		t.Errorf("Expected nil, got %v", val)
	}

	// Consume should reset
	consumed := ch.Consume()
	if !consumed {
		t.Error("Consume should return true")
	}
	if ch.IsAvailable() {
		t.Error("Barrier should be reset after consume")
	}
}

func TestNamedBarrierValueAfterFinish(t *testing.T) {
	ch := NewNamedBarrierValueAfterFinish(nil, []string{"a", "b"})
	ch.SetKey("barrier_finish")

	// Initially not available (finished=false).
	if ch.IsAvailable() {
		t.Error("barrier should not be available initially (finished=false)")
	}

	// Add nodes but not yet finished.
	ch.Update([]interface{}{"a"})
	ch.Update([]interface{}{"b"})
	if ch.IsAvailable() {
		t.Error("barrier should not be available before Finish is called")
	}

	// Finish should trigger availability.
	finished := ch.Finish()
	if !finished {
		t.Error("Finish should return true")
	}
	if !ch.IsAvailable() {
		t.Error("barrier should be available after Finish + all names seen")
	}
}

func TestBaseChannelGetVersion(t *testing.T) {
	ch := NewLastValue("")
	if v := ch.GetVersion(); v != 0 {
		t.Errorf("new channel version should be 0, got %d", v)
	}
	ch.SetVersion(42)
	if v := ch.GetVersion(); v != 42 {
		t.Errorf("expected version 42, got %d", v)
	}
	// Concurrent read/write should not race (atomic).
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ { ch.SetVersion(i) }
		close(done)
	}()
	for i := 0; i < 100; i++ { _ = ch.GetVersion() }
	<-done
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry()

	// Register channels
	ch1 := NewLastValue("")
	ch1.SetKey("ch1")
	reg.Register("ch1", ch1)

	ch2 := NewLastValue(0)
	ch2.SetKey("ch2")
	reg.Register("ch2", ch2)

	if reg.Len() != 2 {
		t.Errorf("Expected 2 channels, got %d", reg.Len())
	}

	// Update channels
	writes := map[string][]interface{}{
		"ch1": {"hello"},
		"ch2": {42},
	}
	err := reg.UpdateChannels(writes)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Get values
	values, err := reg.GetValues()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if values["ch1"] != "hello" {
		t.Errorf("Expected 'hello', got %v", values["ch1"])
	}
	if values["ch2"] != 42 {
		t.Errorf("Expected 42, got %v", values["ch2"])
	}

	// Create and restore checkpoint
	checkpoint := reg.CreateCheckpoint()
	if len(checkpoint) != 2 {
		t.Errorf("Expected 2 entries in checkpoint, got %d", len(checkpoint))
	}

	// Modify channels
	reg.UpdateChannels(map[string][]interface{}{
		"ch1": {"modified"},
	})

	// Restore checkpoint
	err = reg.RestoreFromCheckpoint(checkpoint)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	values, _ = reg.GetValues()
	if values["ch1"] != "hello" {
		t.Errorf("Expected restored value 'hello', got %v", values["ch1"])
	}
}
