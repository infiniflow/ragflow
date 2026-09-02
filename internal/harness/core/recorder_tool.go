package core

import (
	"context"
	"encoding/json"
	"time"

	"ragflow/internal/harness/core/schema"
	"ragflow/internal/harness/events"
)

// NewEventRecorderToolMiddleware creates a ToolInvokeMiddleware that records
// every tool invocation to the EventRecorder found in the context.
//
// Usage:
//
//	cfg := &ReActConfig[*schema.Message]{
//	    ToolsConfig: &ToolsNodeConfig{
//	        ToolInvokeMiddlewares: []ToolInvokeMiddleware{
//	            NewEventRecorderToolMiddleware(),
//	        },
//	    },
//	}
//	ctx = events.ContextWithRecorder(ctx, recorder)
func NewEventRecorderToolMiddleware() ToolInvokeMiddleware {
	return func(next InvokeTool) InvokeTool {
		return func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
			rec := events.RecorderFromContext(ctx)

			start := time.Now()
			result, err := next(ctx, ictx)
			durMs := time.Since(start).Milliseconds()

			if rec == nil {
				return result, err
			}

			// Extract tool arguments as map (parse from JSON string).
			var args map[string]any
			if ictx.Arguments != nil && ictx.Arguments.Arguments != "" {
				json.Unmarshal([]byte(ictx.Arguments.Arguments), &args)
			}

			errStr := ""
			retryCount := 0
			if ictx.RetryConfig != nil {
				retryCount = ictx.RetryConfig.MaxAttempts
			}
			if err != nil {
				errStr = err.Error()
			} else if result != nil && result.Error != "" {
				errStr = result.Error
			}

			rec.RecordToolCall(ctx, ictx.Name, args, result, durMs, retryCount, errStr)
			return result, err
		}
	}
}
