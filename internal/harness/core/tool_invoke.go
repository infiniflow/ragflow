package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"ragflow/internal/harness/core/schema"
)

// ToolInvocationContext captures the full context of a single tool invocation.
// It replaces the separate endpoint function signatures in middleware chains
// with a single unified object, making it easier to implement cross-cutting
// concerns like timeout, retry, and approval.
type ToolInvocationContext struct {
	// Name is the tool name being called (e.g., "get_weather").
	Name string
	// CallID is the unique identifier for this invocation from the LLM.
	CallID string
	// Arguments is the structured tool argument.
	Arguments *schema.ToolArgument
	// Result holds the tool result after successful execution (may be set by middleware).
	Result *schema.ToolResult
	// Timeout is the per-invocation timeout. Zero means no timeout.
	Timeout time.Duration
	// RetryConfig configures retry for this invocation. Nil means no retry.
	RetryConfig *ToolRetryConfig
	// Fallback is an optional fallback tool function to call if the primary fails.
	Fallback func(ctx context.Context, args *schema.ToolArgument) (*schema.ToolResult, error)

	// internal
	err     error
	skipped bool
	mu      sync.Mutex
}

// ToolRetryConfig configures retry behavior for a single tool invocation.
type ToolRetryConfig struct {
	MaxAttempts int
	Backoff     time.Duration
	IsRetryable func(err error) bool
}

// InvokeTool is the standard tool invocation function signature using the unified context.
type InvokeTool func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error)

// ToolInvokeMiddleware wraps a tool invocation with cross-cutting behavior.
// It receives the next handler in the chain and the invocation context.
type ToolInvokeMiddleware func(next InvokeTool) InvokeTool

// ---- ToolWrapper: timeout + retry + fallback ----

// NewTimeoutToolMiddleware creates a ToolInvokeMiddleware that enforces a timeout.
// If the tool invocation exceeds the duration, the context is cancelled.
func NewTimeoutToolMiddleware(timeout time.Duration) ToolInvokeMiddleware {
	return func(next InvokeTool) InvokeTool {
		return func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
			d := timeout
			if ictx.Timeout > 0 {
				d = ictx.Timeout
			}
			if d <= 0 {
				return next(ctx, ictx)
			}
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()
			return next(ctx, ictx)
		}
	}
}

// NewRetryToolMiddleware creates a ToolInvokeMiddleware that retries on failure.
func NewRetryToolMiddleware(cfg *ToolRetryConfig) ToolInvokeMiddleware {
	return func(next InvokeTool) InvokeTool {
		return func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
			rc := cfg
			if ictx.RetryConfig != nil {
				rc = ictx.RetryConfig
			}
			if rc == nil || rc.MaxAttempts <= 0 {
				return next(ctx, ictx)
			}
			backoff := rc.Backoff
			if backoff <= 0 {
				backoff = 100 * time.Millisecond
			}
			var lastErr error
			for attempt := 0; attempt <= rc.MaxAttempts; attempt++ {
				result, err := next(ctx, ictx)
				if err == nil {
					return result, nil
				}
				lastErr = err
				if rc.IsRetryable != nil && !rc.IsRetryable(err) {
					return nil, err
				}
				if attempt < rc.MaxAttempts {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(backoff):
					}
					backoff *= 2
				}
			}
			return nil, fmt.Errorf("tool retry exhausted after %d attempts: %w", rc.MaxAttempts, lastErr)
		}
	}
}

// NewFallbackToolMiddleware creates a ToolInvokeMiddleware that falls back to a secondary function.
func NewFallbackToolMiddleware(fallback func(ctx context.Context, args *schema.ToolArgument) (*schema.ToolResult, error)) ToolInvokeMiddleware {
	return func(next InvokeTool) InvokeTool {
		return func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
			result, err := next(ctx, ictx)
			if err == nil {
				return result, nil
			}
			fb := fallback
			if ictx.Fallback != nil {
				fb = ictx.Fallback
			}
			if fb == nil {
				return nil, err
			}
			return fb(ctx, ictx.Arguments)
		}
	}
}

// ---- Tool wrapper chain builder ----

// ToolWrapperChain builds a composed tool invocation handler from middleware and a final tool function.
func ToolWrapperChain(toolFn InvokeTool, middlewares ...ToolInvokeMiddleware) InvokeTool {
	chained := toolFn
	for i := len(middlewares) - 1; i >= 0; i-- {
		chained = middlewares[i](chained)
	}
	return chained
}

// ---- Approval mechanism ----

// ApprovalRequest is returned when a tool requires human approval before execution.
type ApprovalRequest struct {
	ToolName    string
	CallID      string
	Arguments   *schema.ToolArgument
	Description string
	// ApproveChan receives the approval decision. Send true to approve, false to reject.
	ApproveChan chan bool
}

// ApprovalMiddleware creates a ToolInvokeMiddleware that requests human approval before
// tool invocation. If approval is denied or times out, the tool is skipped.
// The getApproval callback is called for every tool invocation to produce an approval request.
func ApprovalMiddleware(getApproval func(ctx context.Context, ictx *ToolInvocationContext) (*ApprovalRequest, error)) ToolInvokeMiddleware {
	return func(next InvokeTool) InvokeTool {
		return func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
			req, err := getApproval(ctx, ictx)
			if err != nil {
				return nil, fmt.Errorf("approval setup error: %w", err)
			}
			if req == nil {
				return next(ctx, ictx)
			}

			select {
			case approved := <-req.ApproveChan:
				if !approved {
					return &schema.ToolResult{
						Name:    ictx.Name,
						Content: fmt.Sprintf("Tool '%s' execution rejected by user", ictx.Name),
						Error:   "rejected",
					}, nil
				}
				return next(ctx, ictx)
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
}

// AutoApprovalMiddleware creates an approval middleware that auto-approves all tools.
// Useful for testing or when no human-in-the-loop is needed.
func AutoApprovalMiddleware() ToolInvokeMiddleware {
	return ApprovalMiddleware(func(ctx context.Context, ictx *ToolInvocationContext) (*ApprovalRequest, error) {
		return nil, nil // nil = auto-approve
	})
}

// ---- Wrapping existing Tool into ToolInvokeMiddleware chain ----

// ToolToInvokeFn converts a standard Tool into an InvokeTool function.
func ToolToInvokeFn(tool Tool) InvokeTool {
	return func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		result, err := tool.Invoke(ctx, ictx.Arguments.Arguments)
		if err != nil {
			// Preserve tool interrupts so ToolsNode can handle them.
			if _, ok := IsToolInterrupt(err); ok {
				return nil, err
			}
			return &schema.ToolResult{Name: ictx.Name, Error: err.Error(), ToolCallID: ictx.CallID}, nil
		}
		return &schema.ToolResult{Name: ictx.Name, Content: result, ToolCallID: ictx.CallID}, nil
	}
}

// EnhancedToolToInvokeFn converts an EnhancedTool into an InvokeTool function.
func EnhancedToolToInvokeFn(tool EnhancedTool) InvokeTool {
	return func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		return tool.EnhancedInvoke(ctx, ictx.Arguments)
	}
}

// ---- Built-in middlewares: event sending and cancel monitoring ----

// NewEventSenderToolMiddleware creates a ToolInvokeMiddleware that emits tool
// result events to the agent's event stream after tool execution.
func NewEventSenderToolMiddleware[M MessageType]() ToolInvokeMiddleware {
	return func(next InvokeTool) InvokeTool {
		return func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
			result, err := next(ctx, ictx)
			if err != nil {
				return nil, err
			}
			ec := getReActExecCtx[M](ctx)
			if ec != nil && ec.generator != nil && result != nil {
				content := result.Content
				if content == "" {
					content = result.Error
				}
				var msg M
				var zero M
				switch any(zero).(type) {
				case *schema.AgenticMessage:
					msg = any(&schema.AgenticMessage{
						Role:    schema.AgenticRoleUser,
						Content: content,
						ContentBlocks: []schema.ContentBlock{
							{Type: "tool_result", ToolResult: &schema.ToolResult{
								ToolCallID: ictx.CallID, Content: content,
							}},
						},
					}).(M)
				default:
					msg = any(schema.ToolMessage(content, ictx.CallID)).(M)
				}
				ev := typedEventFromMessage(msg, nil, schema.RoleTool, ictx.Name)
				ec.send(ev)
			}
			return result, nil
		}
	}
}

// NewCancelToolMiddleware creates a ToolInvokeMiddleware that checks the cancel
// context before tool execution. If immediate cancel is requested, it returns
// ErrStreamCanceled immediately.
func NewCancelToolMiddleware() ToolInvokeMiddleware {
	return func(next InvokeTool) InvokeTool {
		return func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
			cc := getCancelContext(ctx)
			if cc != nil && cc.isImmediate() {
				return nil, ErrStreamCanceled
			}
			return next(ctx, ictx)
		}
	}
}

// ---- Rate limiting ----

// rateLimiter implements a simple per-tool token bucket.
type rateLimiter struct {
	mu     sync.Mutex
	tokens map[string]*tokenBucket
}

type tokenBucket struct {
	capacity int
	tokens   float64
	rate     float64 // tokens per nanosecond
	last     time.Time
}

func (rl *rateLimiter) allow(name string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.tokens[name]
	if !ok {
		return true // first use, always allow
	}
	now := time.Now()
	elapsed := now.Sub(b.last)
	b.tokens += elapsed.Seconds() * b.rate
	if b.tokens > float64(b.capacity) {
		b.tokens = float64(b.capacity)
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func (rl *rateLimiter) init(name string, rate_ float64, burst int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.tokens[name] = &tokenBucket{
		capacity: burst,
		tokens:   float64(burst),
		rate:     rate_,
		last:     time.Now(),
	}
}

// NewRateLimitToolMiddleware creates a ToolInvokeMiddleware that limits the
// invocation rate per tool name using a per-token token bucket.
// rate is the number of invocations per second, burst is the maximum burst size.
//
// Example: NewRateLimitToolMiddleware(10, 5) allows up to 10 req/s with burst of 5.
func NewRateLimitToolMiddleware(rate float64, burst int) ToolInvokeMiddleware {
	rl := &rateLimiter{tokens: make(map[string]*tokenBucket)}
	return func(next InvokeTool) InvokeTool {
		return func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
			rl.initOnce(ictx.Name, rate, burst)
			if !rl.allow(ictx.Name) {
				return nil, fmt.Errorf("rate limit exceeded for tool '%s'", ictx.Name)
			}
			return next(ctx, ictx)
		}
	}
}

func (rl *rateLimiter) initOnce(name string, rate float64, burst int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if _, ok := rl.tokens[name]; ok {
		return
	}
	rl.tokens[name] = &tokenBucket{
		capacity: burst,
		tokens:   float64(burst),
		rate:     rate,
		last:     time.Now(),
	}
}
