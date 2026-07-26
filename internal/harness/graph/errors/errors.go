// Package errors provides error types for Agent Harness Go.
package errors

import (
	"fmt"
	"runtime"
	"strings"
)

// ErrorCode represents specific error codes for Agent Harness.
type ErrorCode string

const (
	// ErrorCodeGraphRecursionLimit is raised when the graph exhausts the maximum number of steps.
	ErrorCodeGraphRecursionLimit ErrorCode = "GRAPH_RECURSION_LIMIT"
	// ErrorCodeInvalidConcurrentGraphUpdate is raised for invalid concurrent graph updates.
	ErrorCodeInvalidConcurrentGraphUpdate ErrorCode = "INVALID_CONCURRENT_GRAPH_UPDATE"
	// ErrorCodeInvalidGraphNodeReturnValue is raised for invalid node return values.
	ErrorCodeInvalidGraphNodeReturnValue ErrorCode = "INVALID_GRAPH_NODE_RETURN_VALUE"
	// ErrorCodeMultipleSubgraphs is raised when multiple subgraphs are detected.
	ErrorCodeMultipleSubgraphs ErrorCode = "MULTIPLE_SUBGRAPHS"
	// ErrorCodeInvalidChatHistory is raised for invalid chat history.
	ErrorCodeInvalidChatHistory ErrorCode = "INVALID_CHAT_HISTORY"
	// ErrorCodeCheckpointConflict is raised when there is a checkpoint version conflict.
	ErrorCodeCheckpointConflict ErrorCode = "CHECKPOINT_CONFLICT"
	// ErrorCodeInvalidState is raised when the state is invalid.
	ErrorCodeInvalidState ErrorCode = "INVALID_STATE"
	// ErrorCodeNodeNotFound is raised when a node is not found.
	ErrorCodeNodeNotFound ErrorCode = "NODE_NOT_FOUND"
	// ErrorCodeChannelNotFound is raised when a channel is not found.
	ErrorCodeChannelNotFound ErrorCode = "CHANNEL_NOT_FOUND"
	// ErrorCodeTimeout is raised when a timeout occurs.
	ErrorCodeTimeout ErrorCode = "TIMEOUT"
	// ErrorCodeCancellation is raised when the execution is cancelled.
	ErrorCodeCancellation ErrorCode = "CANCELLATION"
)

// CreateErrorMessage creates an error message with troubleshooting information.
// The URL points to the Harness-Go documentation (not the Python LangGraph docs).
func CreateErrorMessage(message string, errorCode ErrorCode) string {
	return fmt.Sprintf(
		"%s\nFor troubleshooting, visit: https://ragflow/internal/harness/docs/errors/%s",
		message,
		errorCode,
	)
}

// ErrorContext provides additional context about an error.
type ErrorContext struct {
	// ErrorCode is the specific error code.
	ErrorCode ErrorCode
	// Message is the error message.
	Message string
	// StackTrace is the stack trace at the point of error.
	StackTrace []string
	// Cause is the underlying cause of this error.
	Cause error
	// Metadata contains additional error metadata.
	Metadata map[string]interface{}
}

// NewErrorContext creates a new error context.
func NewErrorContext(code ErrorCode, message string, cause error) *ErrorContext {
	return &ErrorContext{
		ErrorCode:  code,
		Message:    message,
		StackTrace: captureStackTrace(2), // Skip captureStackTrace and NewErrorContext
		Cause:      cause,
		Metadata:   make(map[string]interface{}),
	}
}

// Error returns the error message with context.
func (ec *ErrorContext) Error() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("[%s] %s", ec.ErrorCode, ec.Message))

	if ec.Cause != nil {
		sb.WriteString(fmt.Sprintf("\nCaused by: %s", ec.Cause.Error()))
	}

	if len(ec.StackTrace) > 0 {
		sb.WriteString("\nStack trace:")
		for _, frame := range ec.StackTrace {
			sb.WriteString(fmt.Sprintf("\n  %s", frame))
		}
	}

	if len(ec.Metadata) > 0 {
		sb.WriteString("\nMetadata:")
		for k, v := range ec.Metadata {
			sb.WriteString(fmt.Sprintf("\n  %s: %v", k, v))
		}
	}

	return sb.String()
}

// Unwrap returns the underlying cause.
func (ec *ErrorContext) Unwrap() error {
	return ec.Cause
}

// AddMetadata adds metadata to the error context.
func (ec *ErrorContext) AddMetadata(key string, value interface{}) {
	if ec.Metadata == nil {
		ec.Metadata = make(map[string]interface{})
	}
	ec.Metadata[key] = value
}

// GetMetadata gets metadata from the error context.
func (ec *ErrorContext) GetMetadata(key string) (interface{}, bool) {
	if ec.Metadata == nil {
		return nil, false
	}
	val, ok := ec.Metadata[key]
	return val, ok
}

// captureStackTrace captures the current stack trace.
func captureStackTrace(skip int) []string {
	var stack []string
	pcs := make([]uintptr, 32)
	n := runtime.Callers(skip, pcs)
	if n == 0 {
		return stack
	}

	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		stack = append(stack, fmt.Sprintf("%s\n\t%s:%d", frame.Function, frame.File, frame.Line))
		if !more {
			break
		}
	}

	return stack
}

// WrapError wraps an error with additional context.
func WrapError(err error, code ErrorCode, message string) error {
	if err == nil {
		return nil
	}

	// If it's already an ErrorContext, just add to it
	if ec, ok := err.(*ErrorContext); ok {
		return &ErrorContext{
			ErrorCode:  code,
			Message:    message,
			StackTrace: captureStackTrace(2),
			Cause:      ec,
			Metadata:   make(map[string]interface{}),
		}
	}

	return NewErrorContext(code, message, err)
}

// GetErrorCode extracts the error code from an error.
func GetErrorCode(err error) ErrorCode {
	if err == nil {
		return ""
	}

	// Check for ErrorContext
	if ec, ok := err.(*ErrorContext); ok {
		return ec.ErrorCode
	}

	// Check for specific error types
	if IsGraphRecursionError(err) {
		return ErrorCodeGraphRecursionLimit
	}
	if IsGraphInterrupt(err) {
		return ErrorCodeCancellation
	}
	if IsParentCommand(err) {
		return ErrorCodeInvalidConcurrentGraphUpdate
	}

	return ""
}

// GetErrorStack extracts the stack trace from an error.
func GetErrorStack(err error) []string {
	if err == nil {
		return nil
	}

	if ec, ok := err.(*ErrorContext); ok {
		return ec.StackTrace
	}

	return nil
}

// FormatError formats an error for display.
func FormatError(err error) string {
	if err == nil {
		return ""
	}

	var sb strings.Builder

	current := err
	depth := 0
	for current != nil && depth < 10 { // Prevent infinite loops
		prefix := strings.Repeat("  ", depth)
		sb.WriteString(fmt.Sprintf("%s%s\n", prefix, current.Error()))

		// Check for wrapped error
		if unwrapped := fmt.Sprintf("%v", err); unwrapped != current.Error() {
			current = fmt.Errorf("%s", unwrapped)
		} else {
			current = nil
		}

		depth++
	}

	return sb.String()
}

// ChainError creates a chain of errors for better debugging.
func ChainError(base error, newErr error) error {
	if newErr == nil {
		return base
	}

	if base == nil {
		return newErr
	}

	return fmt.Errorf("%s: %w", newErr, base)
}

// EmptyChannelError is raised when a channel is empty (never updated yet).
type EmptyChannelError struct {
	Message string
}

func (e *EmptyChannelError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "channel is empty"
}

// IsEmptyChannelError checks if an error is an EmptyChannelError.
func IsEmptyChannelError(err error) bool {
	_, ok := err.(*EmptyChannelError)
	return ok
}

// GraphRecursionError is raised when the graph has exhausted the maximum number of steps.
type GraphRecursionError struct {
	Limit int
}

func (e *GraphRecursionError) Error() string {
	return fmt.Sprintf(
		"Graph recursion limit of %d reached. To increase the limit, "+
			"run your graph with a config specifying a higher recursion_limit.",
		e.Limit,
	)
}

// IsGraphRecursionError checks if an error is a GraphRecursionError.
func IsGraphRecursionError(err error) bool {
	_, ok := err.(*GraphRecursionError)
	return ok
}

// InvalidUpdateError is raised when attempting to update a channel with an invalid set of updates.
type InvalidUpdateError struct {
	Message string
}

func (e *InvalidUpdateError) Error() string {
	return fmt.Sprintf("Invalid update: %s", e.Message)
}

// IsInvalidUpdateError checks if an error is an InvalidUpdateError.
func IsInvalidUpdateError(err error) bool {
	_, ok := err.(*InvalidUpdateError)
	return ok
}

// GraphBubbleUp is the base type for exceptions that bubble up from subgraphs.
type GraphBubbleUp struct {
	Message string
	Cause   error
}

func (e *GraphBubbleUp) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "graph bubble up"
}

func (e *GraphBubbleUp) Unwrap() error {
	return e.Cause
}

// GraphInterrupt is raised when a subgraph is interrupted.
type GraphInterrupt struct {
	Interrupts []interface{}
}

func (e *GraphInterrupt) Error() string {
	return fmt.Sprintf("graph interrupted with %d interrupt(s)", len(e.Interrupts))
}

// IsGraphInterrupt checks if an error is a GraphInterrupt.
func IsGraphInterrupt(err error) bool {
	_, ok := err.(*GraphInterrupt)
	return ok
}

// ParentCommand is raised when a command should be sent to the parent graph.
type ParentCommand struct {
	Command interface{}
}

func (e *ParentCommand) Error() string {
	return "parent command"
}

// IsParentCommand checks if an error is a ParentCommand.
func IsParentCommand(err error) bool {
	_, ok := err.(*ParentCommand)
	return ok
}

// EmptyInputError is raised when graph receives an empty input.
type EmptyInputError struct {
	Message string
}

func (e *EmptyInputError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "empty input"
}

// IsEmptyInputError checks if an error is an EmptyInputError.
func IsEmptyInputError(err error) bool {
	_, ok := err.(*EmptyInputError)
	return ok
}

// TaskNotFound is raised when the executor is unable to find a task.
type TaskNotFound struct {
	TaskID string
}

func (e *TaskNotFound) Error() string {
	return fmt.Sprintf("task not found: %s", e.TaskID)
}

// IsTaskNotFound checks if an error is a TaskNotFound.
func IsTaskNotFound(err error) bool {
	_, ok := err.(*TaskNotFound)
	return ok
}

// InvalidNodeError is raised when a node is invalid.
type InvalidNodeError struct {
	NodeName string
	Message  string
}

func (e *InvalidNodeError) Error() string {
	return fmt.Sprintf("invalid node '%s': %s", e.NodeName, e.Message)
}

// InvalidEdgeError is raised when an edge is invalid.
type InvalidEdgeError struct {
	From    string
	To      string
	Message string
}

func (e *InvalidEdgeError) Error() string {
	return fmt.Sprintf("invalid edge from '%s' to '%s': %s", e.From, e.To, e.Message)
}

// ChannelNotFoundError is raised when a channel is not found.
type ChannelNotFoundError struct {
	ChannelName string
}

func (e *ChannelNotFoundError) Error() string {
	return fmt.Sprintf("channel not found: %s", e.ChannelName)
}

// NodeNotFoundError is raised when a node is not found.
type NodeNotFoundError struct {
	NodeName string
}

func (e *NodeNotFoundError) Error() string {
	return fmt.Sprintf("node not found: %s", e.NodeName)
}
