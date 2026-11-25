package runtime

import "context"

// AgentNodeObserver records an execution and supplies its context and completion callback.
// The returned context lets nested operations inherit the execution as their parent.
type AgentNodeObserver func(context.Context, string, string, string, uint32, any) (context.Context, func(any, error))

type agentNodeObserverKey struct{}

// WithAgentNodeObserver enables child execution observation for this Agent context.
func WithAgentNodeObserver(ctx context.Context, observer AgentNodeObserver) context.Context {
	return context.WithValue(ctx, agentNodeObserverKey{}, observer)
}

// ObserveAgentNode starts an observed execution when an Agent observer is installed.
func ObserveAgentNode(ctx context.Context, nodeID, name, nodeType string, attempt uint32, inputs any) (context.Context, func(any, error)) {
	observer, _ := ctx.Value(agentNodeObserverKey{}).(AgentNodeObserver)
	if observer == nil {
		return ctx, func(any, error) {}
	}
	return observer(ctx, nodeID, name, nodeType, attempt, inputs)
}

// HasAgentNodeObserver reports whether this context records Agent child executions.
func HasAgentNodeObserver(ctx context.Context) bool {
	observer, _ := ctx.Value(agentNodeObserverKey{}).(AgentNodeObserver)
	return observer != nil
}
