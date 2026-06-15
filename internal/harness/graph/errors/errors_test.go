// Package errors tests error handling functionality.
package errors

import (
	"testing"
)

func TestErrorCodeConstants(t *testing.T) {
	// Test that all error codes are defined
	tests := []struct {
		name      string
		code      ErrorCode
		expected  string
	}{
		{"GraphRecursionLimit", ErrorCodeGraphRecursionLimit, "GRAPH_RECURSION_LIMIT"},
		{"InvalidConcurrentGraphUpdate", ErrorCodeInvalidConcurrentGraphUpdate, "INVALID_CONCURRENT_GRAPH_UPDATE"},
		{"InvalidGraphNodeReturnValue", ErrorCodeInvalidGraphNodeReturnValue, "INVALID_GRAPH_NODE_RETURN_VALUE"},
		{"MultipleSubgraphs", ErrorCodeMultipleSubgraphs, "MULTIPLE_SUBGRAPHS"},
		{"InvalidChatHistory", ErrorCodeInvalidChatHistory, "INVALID_CHAT_HISTORY"},
		{"CheckpointConflict", ErrorCodeCheckpointConflict, "CHECKPOINT_CONFLICT"},
		{"InvalidState", ErrorCodeInvalidState, "INVALID_STATE"},
		{"NodeNotFound", ErrorCodeNodeNotFound, "NODE_NOT_FOUND"},
		{"ChannelNotFound", ErrorCodeChannelNotFound, "CHANNEL_NOT_FOUND"},
		{"Timeout", ErrorCodeTimeout, "TIMEOUT"},
		{"Cancellation", ErrorCodeCancellation, "CANCELLATION"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.code) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.code))
			}
		})
	}
}

func TestCreateErrorMessage(t *testing.T) {
	message := "Test error message"
	code := ErrorCodeGraphRecursionLimit

	result := CreateErrorMessage(message, code)

	expected := "Test error message\nFor troubleshooting, visit: https://ragflow/internal/harness/docs/errors/GRAPH_RECURSION_LIMIT"
	if result != expected {
		t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, result)
	}
}

func TestNewErrorContext(t *testing.T) {
	err := NewErrorContext(ErrorCodeInvalidState, "Invalid state", nil)

	if err.ErrorCode != ErrorCodeInvalidState {
		t.Errorf("Expected ErrorCodeInvalidState, got %s", err.ErrorCode)
	}

	if err.Message != "Invalid state" {
		t.Errorf("Expected 'Invalid state', got '%s'", err.Message)
	}

	if err.Cause != nil {
		t.Error("Cause should be nil")
	}

	if len(err.StackTrace) == 0 {
		t.Error("StackTrace should not be empty")
	}
}

func TestErrorContext_Error(t *testing.T) {
	baseErr := &GraphBubbleUp{
		Message: "Base error",
	}

	err := NewErrorContext(ErrorCodeInvalidState, "Invalid state", baseErr)

	errorStr := err.Error()

	// Check that error code is included
	if len(errorStr) == 0 {
		t.Error("Error string should not be empty")
	}

	// Check that message is included
	if !contains(errorStr, "Invalid state") {
		t.Error("Error string should contain 'Invalid state'")
	}

	// Check that cause is included
	if !contains(errorStr, "Base error") {
		t.Error("Error string should contain cause")
	}
}

func TestErrorContext_Metadata(t *testing.T) {
	err := NewErrorContext(ErrorCodeInvalidState, "Invalid state", nil)

	err.AddMetadata("key1", "value1")
	err.AddMetadata("key2", 42)

	val, ok := err.GetMetadata("key1")
	if !ok {
		t.Error("Expected key1 to exist")
	}
	if val != "value1" {
		t.Errorf("Expected 'value1', got '%v'", val)
	}

	val, ok = err.GetMetadata("key2")
	if !ok {
		t.Error("Expected key2 to exist")
	}
	if val != 42 {
		t.Errorf("Expected 42, got '%v'", val)
	}

	_, ok = err.GetMetadata("key3")
	if ok {
		t.Error("key3 should not exist")
	}
}

func TestWrapError(t *testing.T) {
	baseErr := &GraphBubbleUp{Message: "Base error"}

	wrapped := WrapError(baseErr, ErrorCodeInvalidState, "Invalid state")

	if wrapped == nil {
		t.Error("Wrapped error should not be nil")
	}

	// Wrap nil should return nil
	nilWrapped := WrapError(nil, ErrorCodeInvalidState, "Invalid state")
	if nilWrapped != nil {
		t.Error("Wrapping nil should return nil")
	}
}

func TestGetErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCode
	}{
		{"GraphRecursionError", &GraphRecursionError{Limit: 100}, ErrorCodeGraphRecursionLimit},
		{"GraphInterrupt", &GraphInterrupt{Interrupts: []interface{}{"test"}}, ErrorCodeCancellation},
		{"ParentCommand", &ParentCommand{Command: "test"}, ErrorCodeInvalidConcurrentGraphUpdate},
		{"ErrorContext", NewErrorContext(ErrorCodeInvalidState, "test", nil), ErrorCodeInvalidState},
		{"Nil", nil, ""},
		{"GenericError", &GraphBubbleUp{Message: "test"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetErrorCode(tt.err)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetErrorStack(t *testing.T) {
	err := NewErrorContext(ErrorCodeInvalidState, "test", nil)

	stack := GetErrorStack(err)

	if stack == nil {
		t.Error("Stack should not be nil for ErrorContext")
	}

	if len(stack) == 0 {
		t.Error("Stack should not be empty")
	}

	// Test with non-ErrorContext
	nilStack := GetErrorStack(&GraphBubbleUp{Message: "test"})
	if nilStack != nil {
		t.Error("Stack should be nil for non-ErrorContext")
	}
}

func TestChainError(t *testing.T) {
	baseErr := &GraphBubbleUp{Message: "Base error"}
	newErr := &GraphBubbleUp{Message: "New error"}

	chained := ChainError(baseErr, newErr)

	if chained == nil {
		t.Error("Chained error should not be nil")
	}

	errorStr := chained.Error()
	if !contains(errorStr, "New error") {
		t.Error("Chained error should contain new error")
	}
	if !contains(errorStr, "Base error") {
		t.Error("Chained error should contain base error")
	}

	// Test with nil base
	nilChained := ChainError(nil, newErr)
	if nilChained != newErr {
		t.Error("Chaining nil base should return new error")
	}

	// Test with nil new
	nilChained2 := ChainError(baseErr, nil)
	if nilChained2 != baseErr {
		t.Error("Chaining nil new should return base error")
	}
}

func TestFormatError(t *testing.T) {
	baseErr := &GraphBubbleUp{Message: "Base error"}
	err := NewErrorContext(ErrorCodeInvalidState, "Invalid state", baseErr)

	formatted := FormatError(err)

	if len(formatted) == 0 {
		t.Error("Formatted error should not be empty")
	}

	if !contains(formatted, "Invalid state") {
		t.Error("Formatted error should contain message")
	}

	// Test with nil
	nilFormatted := FormatError(nil)
	if nilFormatted != "" {
		t.Error("Formatting nil should return empty string")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
