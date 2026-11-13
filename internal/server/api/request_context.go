package api

import "context"

// newRequestContext preserves process values while HTTP shutdown drains requests.
func newRequestContext(processCtx context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(context.WithoutCancel(processCtx))
}
