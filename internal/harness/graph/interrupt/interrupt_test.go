// Package interrupt provides tests for interrupt functionality.
package interrupt

import (
	"context"
	"testing"
)

func TestInterrupt_Basic(t *testing.T) {
	// Test basic interrupt with no resume values
	ctx := WithInterruptContext(context.Background())

	// First call should interrupt (returns error)
	_, err := Interrupt(ctx, "Please provide input")

	if err == nil {
		t.Error("Expected interrupt error, got nil")
	}

	if !IsInterrupt(err) {
		t.Error("Error should be a GraphInterrupt")
	}

	// Check interrupt value
	val, ok := GetInterruptValue(err)
	if !ok {
		t.Error("Should be able to get interrupt value")
	}
	if val == nil {
		t.Error("Interrupt value should not be nil")
	}
}

func TestInterrupt_WithResume(t *testing.T) {
	ctx := WithInterruptContext(context.Background())

	// Add a resume value
	AppendResumeValue(ctx, "user_input")

	// Now interrupt should return the resume value
	value, err := Interrupt(ctx, "Please provide input")

	if err != nil {
		t.Errorf("Should not error with resume value: %v", err)
	}

	if value != "user_input" {
		t.Errorf("Expected 'user_input', got %v", value)
	}
}

func TestInterrupt_Multiple(t *testing.T) {
	ctx := WithInterruptContext(context.Background())

	// Add multiple resume values
	AppendResumeValue(ctx, "first")
	AppendResumeValue(ctx, "second")
	AppendResumeValue(ctx, "third")

	// First interrupt returns first value
	val1, err := Interrupt(ctx, "input1")
	if err != nil {
		t.Errorf("Should not error: %v", err)
	}
	if val1 != "first" {
		t.Errorf("Expected 'first', got %v", val1)
	}

	// Second interrupt returns second value
	val2, err := Interrupt(ctx, "input2")
	if err != nil {
		t.Errorf("Should not error: %v", err)
	}
	if val2 != "second" {
		t.Errorf("Expected 'second', got %v", val2)
	}

	// Third interrupt returns third value
	val3, err := Interrupt(ctx, "input3")
	if err != nil {
		t.Errorf("Should not error: %v", err)
	}
	if val3 != "third" {
		t.Errorf("Expected 'third', got %v", val3)
	}

	// Fourth interrupt should error (no more resume values)
	_, err = Interrupt(ctx, "input4")
	if err == nil {
		t.Error("Should error when no more resume values")
	}
}

func TestGetInterruptIndex(t *testing.T) {
	ctx := WithInterruptContext(context.Background())

	// Initial index should be 0
	idx := GetInterruptIndex(ctx)
	if idx != 0 {
		t.Errorf("Expected initial index 0, got %d", idx)
	}

	// Add resume value and use it
	AppendResumeValue(ctx, "test")
	Interrupt(ctx, "input")

	// Index should be 1
	idx = GetInterruptIndex(ctx)
	if idx != 1 {
		t.Errorf("Expected index 1, got %d", idx)
	}
}

func TestReset(t *testing.T) {
	ctx := WithInterruptContext(context.Background())

	// Add resume values and use them
	AppendResumeValue(ctx, "value")
	Interrupt(ctx, "input")

	// Reset
	Reset(ctx)

	// Index should be 0
	idx := GetInterruptIndex(ctx)
	if idx != 0 {
		t.Errorf("Expected index 0 after reset, got %d", idx)
	}

	// Resume values should be cleared
	values := GetResumeValues(ctx)
	if len(values) != 0 {
		t.Errorf("Expected 0 resume values after reset, got %d", len(values))
	}
}

func TestIsInterruptContext(t *testing.T) {
	// Check if context has interrupt
	ctx := WithInterruptContext(context.Background())
	if !IsInterruptContext(ctx) {
		t.Error("Context should be an interrupt context")
	}

	// Regular context should not be interrupt context
	regularCtx := context.Background()
	if IsInterruptContext(regularCtx) {
		t.Error("Regular context should not be an interrupt context")
	}
}

func TestSetResumeValues(t *testing.T) {
	ctx := WithInterruptContext(context.Background())

	// Set multiple resume values at once
	SetResumeValues(ctx, []interface{}{"a", "b", "c"})

	// Check values
	values := GetResumeValues(ctx)
	if len(values) != 3 {
		t.Errorf("Expected 3 resume values, got %d", len(values))
	}
}

func TestGlobalContext(t *testing.T) {
	// Reset global context
	Reset(context.Background())

	// Test with nil context (should fall back to global)
	AppendResumeValue(nil, "global_value")

	values := GetResumeValues(nil)
	if len(values) != 1 || values[0] != "global_value" {
		t.Error("Should use global context when ctx is nil")
	}
}
