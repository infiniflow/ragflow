package runnable

import (
	"context"
	"fmt"
	"reflect"
)

// Runnable is the base interface for all runnable components.
// A Runnable represents a unit of computation that can be invoked.
type Runnable[Input, Output any] interface {
	// Invoke executes the runnable synchronously.
	Invoke(ctx context.Context, input Input) (Output, error)

	// Batch executes the runnable on multiple inputs.
	Batch(ctx context.Context, inputs []Input) ([]Output, []error)

	// Stream returns a stream of outputs.
	Stream(ctx context.Context, input Input) <-chan Output

	// GetSchema returns the input/output schema.
	GetSchema() *RunnableSchema
}

// RunnableSchema describes the schema of a runnable.
type RunnableSchema struct {
	InputType  string
	OutputType string
	Name       string
	Description string
}

// RunnableFunc wraps a function as a Runnable.
type RunnableFunc[Input, Output any] struct {
	fn       func(context.Context, Input) (Output, error)
	schema   *RunnableSchema
	batchFn  func(context.Context, []Input) ([]Output, []error)
	streamFn func(context.Context, Input) <-chan Output
}

// NewRunnableFunc creates a new Runnable from a function.
func NewRunnableFunc[Input, Output any](
	fn func(context.Context, Input) (Output, error),
	opts ...RunnableOption[Input, Output],
) Runnable[Input, Output] {
	r := &RunnableFunc[Input, Output]{
		fn: fn,
		schema: &RunnableSchema{
			InputType:  fmt.Sprintf("%T", *new(Input)),
			OutputType: fmt.Sprintf("%T", *new(Output)),
		},
	}

	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Invoke executes the runnable.
func (r *RunnableFunc[Input, Output]) Invoke(ctx context.Context, input Input) (Output, error) {
	return r.fn(ctx, input)
}

// Batch executes the runnable on multiple inputs.
func (r *RunnableFunc[Input, Output]) Batch(ctx context.Context, inputs []Input) ([]Output, []error) {
	if r.batchFn != nil {
		return r.batchFn(ctx, inputs)
	}

	// Default: execute sequentially
	outputs := make([]Output, len(inputs))
	errs := make([]error, len(inputs))
	for i, input := range inputs {
		outputs[i], errs[i] = r.Invoke(ctx, input)
	}
	return outputs, errs
}

// Stream returns a stream of outputs.
func (r *RunnableFunc[Input, Output]) Stream(ctx context.Context, input Input) <-chan Output {
	if r.streamFn != nil {
		return r.streamFn(ctx, input)
	}

	// Default: single output
	ch := make(chan Output, 1)
	go func() {
		defer close(ch)
		output, err := r.Invoke(ctx, input)
		if err == nil {
			ch <- output
		}
	}()
	return ch
}

// GetSchema returns the schema.
func (r *RunnableFunc[Input, Output]) GetSchema() *RunnableSchema {
	return r.schema
}

// RunnableOption configures a Runnable.
type RunnableOption[Input, Output any] func(*RunnableFunc[Input, Output])

// WithName sets the name of the runnable.
func WithName[Input, Output any](name string) RunnableOption[Input, Output] {
	return func(r *RunnableFunc[Input, Output]) {
		r.schema.Name = name
	}
}

// WithDescription sets the description of the runnable.
func WithDescription[Input, Output any](desc string) RunnableOption[Input, Output] {
	return func(r *RunnableFunc[Input, Output]) {
		r.schema.Description = desc
	}
}

// WithBatchFn sets the batch function.
func WithBatchFn[Input, Output any](
	fn func(context.Context, []Input) ([]Output, []error),
) RunnableOption[Input, Output] {
	return func(r *RunnableFunc[Input, Output]) {
		r.batchFn = fn
	}
}

// WithStreamFn sets the stream function.
func WithStreamFn[Input, Output any](
	fn func(context.Context, Input) <-chan Output,
) RunnableOption[Input, Output] {
	return func(r *RunnableFunc[Input, Output]) {
		r.streamFn = fn
	}
}

// RunnableSeq chains multiple runnables together in sequence.
// The output of one runnable is the input to the next.
type RunnableSeq struct {
	runnables []Runnable[any, any]
	schema    *RunnableSchema
}

// NewRunnableSeq creates a new sequence of runnables.
func NewRunnableSeq(runnables ...Runnable[any, any]) (*RunnableSeq, error) {
	if len(runnables) == 0 {
		return nil, &RunnableError{Message: "at least one runnable is required"}
	}

	// Verify type compatibility (simplified check)
	for i := 1; i < len(runnables); i++ {
		prevSchema := runnables[i-1].GetSchema()
		currSchema := runnables[i].GetSchema()
		if prevSchema.OutputType != currSchema.InputType {
			return nil, &RunnableError{
				Message: fmt.Sprintf("type mismatch: %s -> %s", prevSchema.OutputType, currSchema.InputType),
			}
		}
	}

	return &RunnableSeq{
		runnables: runnables,
		schema: &RunnableSchema{
			InputType:  runnables[0].GetSchema().InputType,
			OutputType: runnables[len(runnables)-1].GetSchema().OutputType,
			Name:       "sequence",
		},
	}, nil
}

// Invoke executes all runnables in sequence.
func (s *RunnableSeq) Invoke(ctx context.Context, input interface{}) (interface{}, error) {
	var current interface{} = input
	var err error

	for _, r := range s.runnables {
		current, err = r.Invoke(ctx, current)
		if err != nil {
			return nil, err
		}
	}

	return current, nil
}

// Batch executes the sequence on multiple inputs.
func (s *RunnableSeq) Batch(ctx context.Context, inputs []interface{}) ([]interface{}, []error) {
	outputs := make([]interface{}, len(inputs))
	errs := make([]error, len(inputs))

	for i, input := range inputs {
		outputs[i], errs[i] = s.Invoke(ctx, input)
	}

	return outputs, errs
}

// Stream returns a stream of outputs.
// On error, sends a StreamError value instead of silently dropping.
// Callers can type-assert: if se, ok := val.(StreamError); ok { /* handle err */ }
func (s *RunnableSeq) Stream(ctx context.Context, input interface{}) <-chan interface{} {
	ch := make(chan interface{}, 1)
	go func() {
		defer close(ch)
		output, err := s.Invoke(ctx, input)
		if err != nil {
			ch <- StreamError{Err: err}
			return
		}
		ch <- output
	}()
	return ch
}

// GetSchema returns the schema.
func (s *RunnableSeq) GetSchema() *RunnableSchema {
	return s.schema
}

// RunnableParallel executes multiple runnables in parallel.
type RunnableParallel struct {
	runnables map[string]Runnable[any, any]
	schema    *RunnableSchema
}

// NewRunnableParallel creates a new parallel runnable.
func NewRunnableParallel(runnables map[string]Runnable[any, any]) *RunnableParallel {
	return &RunnableParallel{
		runnables: runnables,
		schema: &RunnableSchema{
			Name:       "parallel",
			InputType:  "map",
			OutputType: "map",
		},
	}
}

// Invoke executes all runnables in parallel with the same input.
func (p *RunnableParallel) Invoke(ctx context.Context, input interface{}) (interface{}, error) {
	type result struct {
		name   string
		value  interface{}
		err    error
	}

	resultCh := make(chan result, len(p.runnables))
	for name, r := range p.runnables {
		go func(n string, rn Runnable[any, any]) {
			value, err := rn.Invoke(ctx, input)
			resultCh <- result{name: n, value: value, err: err}
		}(name, r)
	}

	outputs := make(map[string]interface{})
	for range p.runnables {
		res := <-resultCh
		if res.err != nil {
			return nil, res.err
		}
		outputs[res.name] = res.value
	}

	return outputs, nil
}

// Batch executes the parallel runnable.
func (p *RunnableParallel) Batch(ctx context.Context, inputs []interface{}) ([]interface{}, []error) {
	outputs := make([]interface{}, len(inputs))
	errs := make([]error, len(inputs))

	for i, input := range inputs {
		outputs[i], errs[i] = p.Invoke(ctx, input)
	}

	return outputs, errs
}

// Stream returns a stream of outputs.
// On error, sends a StreamError value instead of silently dropping.
func (p *RunnableParallel) Stream(ctx context.Context, input interface{}) <-chan interface{} {
	ch := make(chan interface{}, 1)
	go func() {
		defer close(ch)
		output, err := p.Invoke(ctx, input)
		if err != nil {
			ch <- StreamError{Err: err}
			return
		}
		ch <- output
	}()
	return ch
}

// GetSchema returns the schema.
func (p *RunnableParallel) GetSchema() *RunnableSchema {
	return p.schema
}

// RunnableMap transforms the input before passing to the underlying runnable.
type RunnableMap struct {
	inputFn  func(context.Context, interface{}) (interface{}, error)
	outputFn func(context.Context, interface{}) (interface{}, error)
	base     Runnable[any, any]
	schema   *RunnableSchema
}

// NewRunnableMap creates a new runnable with input/output transformation.
func NewRunnableMap(
	base Runnable[any, any],
	inputFn func(context.Context, interface{}) (interface{}, error),
	outputFn func(context.Context, interface{}) (interface{}, error),
) Runnable[any, any] {
	return &RunnableMap{
		base:     base,
		inputFn:  inputFn,
		outputFn: outputFn,
		schema: &RunnableSchema{
			Name: "map",
		},
	}
}

// Invoke executes the runnable with transformations.
func (m *RunnableMap) Invoke(ctx context.Context, input interface{}) (interface{}, error) {
	if m.inputFn != nil {
		transformed, err := m.inputFn(ctx, input)
		if err != nil {
			return nil, err
		}
		input = transformed
	}

	output, err := m.base.Invoke(ctx, input)
	if err != nil {
		return nil, err
	}

	if m.outputFn != nil {
		transformed, err := m.outputFn(ctx, output)
		if err != nil {
			return nil, err
		}
		output = transformed
	}

	return output, nil
}

// Batch executes the mapped runnable.
func (m *RunnableMap) Batch(ctx context.Context, inputs []interface{}) ([]interface{}, []error) {
	outputs := make([]interface{}, len(inputs))
	errs := make([]error, len(inputs))

	for i, input := range inputs {
		outputs[i], errs[i] = m.Invoke(ctx, input)
	}

	return outputs, errs
}

// Stream returns a stream of outputs.
// On error, sends a StreamError value instead of silently dropping.
func (m *RunnableMap) Stream(ctx context.Context, input interface{}) <-chan interface{} {
	ch := make(chan interface{}, 1)
	go func() {
		defer close(ch)
		output, err := m.Invoke(ctx, input)
		if err != nil {
			ch <- StreamError{Err: err}
			return
		}
		ch <- output
	}()
	return ch
}

// GetSchema returns the schema.
func (m *RunnableMap) GetSchema() *RunnableSchema {
	return m.schema
}

// CoerceToRunnable converts various types to a Runnable.
// Supported types: Runnable, func(context.Context, T) (U, error), func(T) U
func CoerceToRunnable(value interface{}) (Runnable[any, any], error) {
	switch v := value.(type) {
	case Runnable[any, any]:
		return v, nil
	default:
		// Check if it's a function
		valType := reflect.TypeOf(value)
		if valType == nil {
			return nil, &RunnableError{
				Message: "cannot coerce nil to Runnable",
			}
		}
		
		if valType.Kind() == reflect.Func {
			// Try to wrap it as a RunnableFunc
			return coerceFuncToRunnable(value, valType)
		}
		
		return nil, &RunnableError{
			Message: fmt.Sprintf("cannot coerce %T to Runnable", value),
		}
	}
}

// coerceFuncToRunnable coerces a function to a Runnable.
func coerceFuncToRunnable(fn interface{}, fnType reflect.Type) (Runnable[any, any], error) {
	// Check function signature
	numIn := fnType.NumIn()
	numOut := fnType.NumOut()
	
	// Supported signatures:
	// 1. func(context.Context, T) (U, error)
	// 2. func(T) U
	// 3. func(T) (U, error)
	// 4. func() U
	// 5. func() (U, error)
	
	// We'll create a wrapper that adapts the function to Runnable[any, any]
	wrapper := func(ctx context.Context, input interface{}) (interface{}, error) {
		// Prepare arguments
		args := make([]reflect.Value, 0, numIn)
		
		argIndex := 0
		// Check if first argument is context.Context
		if numIn > 0 && fnType.In(0).AssignableTo(reflect.TypeOf((*context.Context)(nil)).Elem()) {
			args = append(args, reflect.ValueOf(ctx))
			argIndex++
		}
		
		// Add input argument if needed
		if argIndex < numIn {
			inputVal := reflect.ValueOf(input)
			paramType := fnType.In(argIndex)
			
			// Try to convert input to expected type
			if input != nil && inputVal.Type().AssignableTo(paramType) {
				args = append(args, inputVal)
			} else if input != nil && inputVal.Type().ConvertibleTo(paramType) {
				args = append(args, inputVal.Convert(paramType))
			} else {
				// Use zero value
				args = append(args, reflect.Zero(paramType))
			}
			argIndex++
		}
		
		// Fill remaining parameters with zero values
		for ; argIndex < numIn; argIndex++ {
			args = append(args, reflect.Zero(fnType.In(argIndex)))
		}
		
		// Call function
		results := reflect.ValueOf(fn).Call(args)
		
		// Process results
		if numOut == 0 {
			return nil, nil
		} else if numOut == 1 {
			// Single return value
			result := results[0].Interface()
			// Check if it's an error
			if err, ok := result.(error); ok {
				return nil, err
			}
			return result, nil
		} else if numOut == 2 {
			// Two return values: result, error
			result := results[0].Interface()
			errVal := results[1].Interface()
			if errVal != nil {
				if err, ok := errVal.(error); ok {
					return result, err
				}
				return nil, fmt.Errorf("expected error, got %T", errVal)
			}
			return result, nil
		}
		
		return nil, &RunnableError{
			Message: fmt.Sprintf("unsupported number of return values: %d", numOut),
		}
	}
	
	return NewRunnableFunc(wrapper), nil
}

// StreamError wraps an error for stream channels.
// Stream implementations send this instead of silently dropping errors.
// Callers type-assert: if se, ok := val.(runnable.StreamError); ok { handle(se.Err) }.
type StreamError struct {
	Err error
}

func (e StreamError) Error() string {
	if e.Err == nil {
		return "<nil error>"
	}
	return e.Err.Error()
}

// RunnableError represents a runnable-related error.
type RunnableError struct {
	Message string
	Code    string
}

func (e *RunnableError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

// InvokeCompat provides a compatibility layer for invoke with different input types.
func InvokeCompat(ctx context.Context, r Runnable[any, any], input interface{}) (interface{}, error) {
	return r.Invoke(ctx, input)
}

// RunnableBuilder provides a fluent interface for building runnables.
type RunnableBuilder struct {
	runnable Runnable[any, any]
}

// NewRunnableBuilder creates a new runnable builder.
func NewRunnableBuilder(runnable Runnable[any, any]) *RunnableBuilder {
	return &RunnableBuilder{runnable: runnable}
}

// Then chains another runnable after this one.
func (b *RunnableBuilder) Then(next Runnable[any, any]) (*RunnableBuilder, error) {
	seq, err := NewRunnableSeq(b.runnable, next)
	if err != nil {
		return nil, err
	}
	return &RunnableBuilder{runnable: seq}, nil
}

// Map applies input/output transformations.
func (b *RunnableBuilder) Map(
	inputFn func(context.Context, interface{}) (interface{}, error),
	outputFn func(context.Context, interface{}) (interface{}, error),
) *RunnableBuilder {
	b.runnable = NewRunnableMap(b.runnable, inputFn, outputFn)
	return b
}

// Build returns the final runnable.
func (b *RunnableBuilder) Build() Runnable[any, any] {
	return b.runnable
}

// Pipe chains runnables in a more ergonomic way.
func Pipe(runnables ...Runnable[any, any]) (Runnable[any, any], error) {
	return NewRunnableSeq(runnables...)
}

// MapValue transforms the input value.
func MapValue(
	fn func(interface{}) interface{},
) func(context.Context, interface{}) (interface{}, error) {
	return func(_ context.Context, v interface{}) (interface{}, error) {
		return fn(v), nil
	}
}
