package core

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"reflect"
	"slices"
)

// AgentCallbackInput is the input to the agent callback OnStart.
type AgentCallbackInput struct {
	Input      *AgentInput
	ResumeInfo *ResumeInfo
}

// AgentCallbackOutput is the output from the agent callback OnEnd.
type AgentCallbackOutput struct {
	Events *AsyncIterator[*AgentEvent]
}

type TypedAgentCallbackInput[M MessageType] struct {
	Input      *TypedAgentInput[M]
	ResumeInfo *ResumeInfo
}

type TypedAgentCallbackOutput[M MessageType] struct {
	Events *AsyncIterator[*TypedAgentEvent[M]]
}

// callbackHandler holds registered callback functions.
type callbackHandler struct {
	onStart     func(ctx context.Context, input *AgentCallbackInput)
	onEnd       func(ctx context.Context, output *AgentCallbackOutput)
	onError     func(ctx context.Context, err error)
	onInterrupt func(ctx context.Context, info *InterruptInfo)
}

type callbackKey struct{}

func getCallbacks(ctx context.Context) []callbackHandler {
	if v := ctx.Value(callbackKey{}); v != nil {
		return v.([]callbackHandler)
	}
	return nil
}

// propagateCallbacks copies callbacks from parent context to nested run options.
func propagateCallbacks(ctx context.Context, opts []RunOption) []RunOption {
	cbs := getCallbacks(ctx)
	if len(cbs) == 0 {
		return opts
	}
	cbOpts := make([]RunOption, 0, len(cbs))
	for _, cb := range cbs {
		handler := cb
		wrapped := callbackHandler{onStart: handler.onStart, onEnd: handler.onEnd, onError: handler.onError, onInterrupt: handler.onInterrupt}
		cbOpts = append(cbOpts, WrapImplSpecificOptFn(func(o *runOptions) {
			o.callbacks = append(o.callbacks, wrapped)
		}))
	}
	return append(cbOpts, opts...)
}

func withCallbacks(ctx context.Context, cbs []callbackHandler) context.Context {
	if len(cbs) == 0 {
		return ctx
	}
	return context.WithValue(ctx, callbackKey{}, cbs)
}

func initAgentCallbacks(ctx context.Context, name, agentType string, opts ...RunOption) context.Context {
	o := getCommonOptions(nil, opts...)
	if len(o.callbacks) == 0 {
		return ctx
	}
	cbs := make([]callbackHandler, 0, len(o.callbacks))
	for _, cb := range o.callbacks {
		switch c := cb.(type) {
		case callbackHandler:
			cbs = append(cbs, c)
		}
	}
	return withCallbacks(ctx, cbs)
}

func initAgenticCallbacks(ctx context.Context, name, agentType string, opts ...RunOption) context.Context {
	return initAgentCallbacks(ctx, name, agentType, opts...)
}

func filterOptions(name string, opts []RunOption) []RunOption {
	// Remove callbacks not matching the given agent name from agentNames list
	o := getCommonOptions(nil, opts...)
	if len(o.agentNames) == 0 {
		return opts
	}

	var filtered []RunOption
	for _, opt := range opts {
		// Filter out AgentNames options that don't match
		if fn, ok := opt.(runOptFn); ok {
			tmp := &runOptions{}
			fn(tmp)
			if tmp.agentNames != nil {
				if !slices.Contains(tmp.agentNames, name) {
					continue
				}
			}
		}
		filtered = append(filtered, opt)
	}
	return filtered
}

func filterCancelOption(opts []RunOption) []RunOption {
	// Remove cancel context options from sub-agent options
	// to avoid duplicate cancel handling
	var filtered []RunOption
	for _, opt := range opts {
		if fn, ok := opt.(runOptFn); ok {
			tmp := &runOptions{}
			fn(tmp)
			if tmp.cancelCtx != nil {
				continue
			}
		}
		filtered = append(filtered, opt)
	}
	if len(filtered) == len(opts) {
		return opts
	}
	return filtered
}

func filterCallbackHandlersForNestedAgents(name string, opts []RunOption) []RunOption {
	// Remove callback handlers that are scoped to specific agents
	o := getCommonOptions(nil, opts...)
	if len(o.agentNames) == 0 {
		return opts
	}

	var filtered []RunOption
	for _, opt := range opts {
		if fn, ok := opt.(runOptFn); ok {
			tmp := &runOptions{}
			fn(tmp)
			if tmp.agentNames != nil {
				if !slices.Contains(tmp.agentNames, name) {
					continue
				}
			}
		}
		filtered = append(filtered, opt)
	}
	return filtered
}

func getAgentType(a Agent) string {
	if t, ok := a.(interface{ GetType() string }); ok {
		return t.GetType()
	}
	return "ReActAgent"
}

// ---- Run-local value helpers ----

func SetRunLocalValue(ctx context.Context, key string, val any) error {
	// P2: Gob encodability check - catch unregistered types early at Set time
	if err := checkGobEncodability(key, val); err != nil {
		return err
	}

	rc := getRunCtx(ctx)
	if rc == nil || rc.Session == nil {
		return errNotInAgentExec
	}
	rc.Session.Values[key] = val
	return nil
}

func GetRunLocalValue(ctx context.Context, key string) (any, bool, error) {
	rc := getRunCtx(ctx)
	if rc == nil || rc.Session == nil {
		return nil, false, errNotInAgentExec
	}
	v, ok := rc.Session.Values[key]
	return v, ok, nil
}

func DeleteRunLocalValue(ctx context.Context, key string) error {
	rc := getRunCtx(ctx)
	if rc == nil || rc.Session == nil {
		return errNotInAgentExec
	}
	delete(rc.Session.Values, key)
	return nil
}

func SendEvent(ctx context.Context, event *AgentEvent) error {
	ec := getChatModelExecCtx(ctx)
	if ec == nil || ec.generator == nil {
		return errNotInAgentExec
	}
	ec.send(event)
	return nil
}

func TypedSendEvent[M MessageType](ctx context.Context, event *TypedAgentEvent[M]) error {
	ec := getReActExecCtx[M](ctx)
	if ec == nil || ec.generator == nil {
		return errNotInAgentExec
	}
	ec.send(event)
	return nil
}

type AgentExecError struct{ Message string }

func (e *AgentExecError) Error() string { return e.Message }

var errNotInAgentExec = &AgentExecError{Message: "must be called within ReActAgent Run/Resume"}

// checkGobEncodability probes whether the value can be gob-encoded as part of
// a map[string]any, which is exactly how session values are serialized during
// checkpoint. This catches unregistered types early at Set time, rather than
// letting them fail at checkpoint/resume time with a confusing error.
func checkGobEncodability(key string, value any) error {
	probe := map[string]any{key: value}
	if err := gob.NewEncoder(io.Discard).Encode(probe); err != nil {
		typeName := reflect.TypeOf(value).String()
		return &AgentExecError{Message: fmt.Sprintf(
			"SetRunLocalValue: the value (type %s) for key %q is not gob-serializable, "+
				"which means it will fail when the agent checkpoint is saved or resumed.\n\n"+
				"To fix this, register the type in an init() function in your package:\n\n"+
				"  func init() {\n"+
				"      schema.RegisterName[%s](\"a_unique_name_for_this_type\")\n"+
				"  }\n\n"+
				"This is required because agent state (including values set via SetRunLocalValue) is "+
				"persisted using gob encoding for interrupt/resume support. All concrete types stored "+
				"in interface-typed fields (like map[string]any) must be registered with gob.\n\n"+
				"If this value does not need to survive interrupt/resume, store it on the context instead, "+
				"for example via context.WithValue, so you don't need gob registration.\n\n"+
				"Underlying error: %v", typeName, key, typeName, err)}
	}
	return nil
}
