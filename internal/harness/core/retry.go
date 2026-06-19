package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"ragflow/internal/harness/core/schema"
	"ragflow/internal/harness/graph/types"
)

var (
	ErrExceedMaxRetries = errors.New("exceeds max retries")
)

// RetryExhaustedError is returned when all retry attempts are exhausted.
type RetryExhaustedError struct {
	LastErr      error
	TotalRetries int
}

func (e *RetryExhaustedError) Error() string {
	if e.LastErr != nil {
		return fmt.Sprintf("exceeds max retries: last error: %v", e.LastErr)
	}
	return "exceeds max retries"
}

func (e *RetryExhaustedError) Unwrap() error { return ErrExceedMaxRetries }

// WillRetryError is emitted when a retryable error occurs and a retry will be attempted.
type WillRetryError struct {
	ErrStr       string
	RetryAttempt int
	rejectReason any
	err          error
}

func (e *WillRetryError) Error() string { return e.ErrStr }
func (e *WillRetryError) Unwrap() error { return e.err }
func (e *WillRetryError) RejectReason() any { return e.rejectReason }

func init() {
	schema.RegisterType("agentcore_will_retry_error", func() any { return &WillRetryError{} })
}

// RetryContext contains context passed to ShouldRetry during a retry decision.
type TypedRetryContext[M MessageType] struct {
	RetryAttempt  int
	InputMessages []M
	OutputMessage M
	Err           error
}

type RetryContext = TypedRetryContext[*schema.Message]

// RetryDecision represents the decision made by ShouldRetry.
type TypedRetryDecision[M MessageType] struct {
	Retry                        bool
	RewriteError                 error
	ModifiedInputMessages        []M
	PersistModifiedInputMessages bool
	AdditionalOptions            []ModelOption
	Backoff                      time.Duration
	RejectReason                 any
}

type RetryDecision = TypedRetryDecision[*schema.Message]

// ModelRetryConfig configures retry behavior for the Model.
type TypedModelRetryConfig[M MessageType] struct {
	MaxRetries  int
	ShouldRetry func(ctx context.Context, retryCtx *TypedRetryContext[M]) *TypedRetryDecision[M]
	IsRetryAble func(ctx context.Context, err error) bool
	BackoffFunc func(ctx context.Context, attempt int) time.Duration
}

type ModelRetryConfig = TypedModelRetryConfig[*schema.Message]

func defaultIsRetryAble(_ context.Context, err error) bool { return err != nil }

func defaultBackoff(_ context.Context, attempt int) time.Duration {
	p := types.RetryPolicy{
		InitialInterval: 100 * time.Millisecond,
		BackoffFactor:   2.0,
		MaxInterval:     10 * time.Second,
		Jitter:          true,
	}
	return p.CalculateBackoff(attempt)
}

// typedRetryModelWrapper wraps a Model with retry logic.
type typedRetryModelWrapper[M MessageType] struct {
	inner  Model[M]
	config *TypedModelRetryConfig[M]
}

func newTypedRetryModelWrapper[M MessageType](inner Model[M], config *TypedModelRetryConfig[M]) *typedRetryModelWrapper[M] {
	return &typedRetryModelWrapper[M]{inner: inner, config: config}
}

func (r *typedRetryModelWrapper[M]) Generate(ctx context.Context, input []M, opts ...ModelOption) (M, error) {
	if r.config.ShouldRetry != nil {
		return r.generateWithShouldRetry(ctx, input, opts...)
	}
	return r.generateLegacy(ctx, input, opts...)
}

func (r *typedRetryModelWrapper[M]) generateLegacy(ctx context.Context, input []M, opts ...ModelOption) (zero M, _ error) {
	isRetryAble := r.config.IsRetryAble
	if isRetryAble == nil { isRetryAble = defaultIsRetryAble }
	backoff := r.config.BackoffFunc
	if backoff == nil { backoff = defaultBackoff }

	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		out, err := r.inner.Generate(ctx, input, opts...)
		if err == nil { return out, nil }
		if errors.Is(err, ErrStreamCanceled) { return zero, err }
		if !isRetryAble(ctx, err) { return zero, err }
		lastErr = err
		if attempt < r.config.MaxRetries {
			if err := contextAwareSleep(ctx, backoff(ctx, attempt+1)); err != nil { return zero, err }
		}
	}
	return zero, &RetryExhaustedError{LastErr: lastErr, TotalRetries: r.config.MaxRetries}
}

func (r *typedRetryModelWrapper[M]) generateWithShouldRetry(ctx context.Context, input []M, opts ...ModelOption) (M, error) {
	backoff := r.config.BackoffFunc
	if backoff == nil { backoff = defaultBackoff }
	execCtx := getReActExecCtx[M](ctx)
	currentInput := input
	currentOpts := opts
	var lastErr error
	var zero M

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		if execCtx != nil { execCtx.suppressEventSend = true }
		out, err := r.inner.Generate(ctx, currentInput, currentOpts...)
		if execCtx != nil { execCtx.suppressEventSend = false }

		if errors.Is(err, ErrStreamCanceled) { return zero, err }

		retryCtx := &TypedRetryContext[M]{
			RetryAttempt: attempt + 1, InputMessages: currentInput,
			OutputMessage: out, Err: err,
		}
		decision := r.config.ShouldRetry(ctx, retryCtx)
		if decision == nil { decision = &TypedRetryDecision[M]{} }

		if !decision.Retry {
			if decision.RewriteError != nil { return zero, decision.RewriteError }
			if err != nil { return zero, err }
			if execCtx != nil && execCtx.generator != nil && !isNilMessage(out) {
				execCtx.send(typedModelOutputEvent(out, nil))
			}
			return out, nil
		}

		lastErr = err
		if lastErr == nil { lastErr = fmt.Errorf("model output rejected by ShouldRetry at attempt %d", attempt+1) }
		if attempt >= r.config.MaxRetries { break }

		// Emit WillRetryError event before sleeping
		if execCtx != nil && execCtx.generator != nil {
			willRetry := &WillRetryError{ErrStr: lastErr.Error(), RetryAttempt: attempt + 1, rejectReason: decision.RejectReason, err: lastErr}
			execCtx.send(&TypedAgentEvent[M]{Err: any(willRetry).(error)})
		}
		applyRetryDecision(&currentInput, &currentOpts, decision)
		delay := decision.Backoff
		if delay == 0 { delay = backoff(ctx, attempt+1) }
		if err := contextAwareSleep(ctx, delay); err != nil { return zero, err }
	}
	return zero, &RetryExhaustedError{LastErr: lastErr, TotalRetries: r.config.MaxRetries}
}

func (r *typedRetryModelWrapper[M]) Stream(ctx context.Context, input []M, opts ...ModelOption) (*schema.StreamReader[M], error) {
	if r.config.ShouldRetry != nil {
		return r.streamWithShouldRetry(ctx, input, opts...)
	}
	return r.streamLegacy(ctx, input, opts...)
}

func (r *typedRetryModelWrapper[M]) streamLegacy(ctx context.Context, input []M, opts ...ModelOption) (*schema.StreamReader[M], error) {
	isRetryAble := r.config.IsRetryAble
	if isRetryAble == nil { isRetryAble = defaultIsRetryAble }
	backoff := r.config.BackoffFunc
	if backoff == nil { backoff = defaultBackoff }

	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		stream, err := r.inner.Stream(ctx, input, opts...)
		if err != nil {
			if errors.Is(err, ErrStreamCanceled) { return nil, err }
			if !isRetryAble(ctx, err) { return nil, err }
			lastErr = err
			if attempt < r.config.MaxRetries {
				if err := contextAwareSleep(ctx, backoff(ctx, attempt+1)); err != nil { return nil, err }
			}
			continue
		}
		// Verify the stream is healthy by reading one chunk
		chunk, streamErr := stream.Recv()
		if streamErr == nil {
				outStream := schema.NewStreamReader[M]()
			go func() {
				outStream.Send(chunk, nil)
				for {
					c, e := stream.Recv()
					if e == io.EOF { break }
					if e != nil { outStream.Send(c, e); return }
					select {
					case <-ctx.Done():
						outStream.Send(c, ctx.Err())
						return
					default:
					}
					outStream.Send(c, nil)
				}
				outStream.Close()
			}()
			return outStream, nil
		}
		stream.Close()
		if errors.Is(streamErr, ErrStreamCanceled) { return nil, streamErr }
		if !isRetryAble(ctx, streamErr) { return nil, streamErr }
		lastErr = streamErr
		if attempt < r.config.MaxRetries {
			if err := contextAwareSleep(ctx, backoff(ctx, attempt+1)); err != nil { return nil, err }
		}
	}
	return nil, &RetryExhaustedError{LastErr: lastErr, TotalRetries: r.config.MaxRetries}
}

func (r *typedRetryModelWrapper[M]) streamWithShouldRetry(ctx context.Context, input []M, opts ...ModelOption) (*schema.StreamReader[M], error) {
	backoff := r.config.BackoffFunc
	if backoff == nil { backoff = defaultBackoff }
	execCtx := getReActExecCtx[M](ctx)
	currentInput := input
	currentOpts := opts
	var lastErr error

	sig := &retrySignal{ch: make(chan streamRetryVerdict, 1)}
	if execCtx != nil {
		execCtx.retrySignal = sig
	}

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		stream, err := r.inner.Stream(ctx, currentInput, currentOpts...)
		if err != nil {
			if errors.Is(err, ErrStreamCanceled) { return nil, err }
			retryCtx := &TypedRetryContext[M]{
				RetryAttempt: attempt + 1, InputMessages: currentInput, Err: err,
			}
			decision := r.config.ShouldRetry(ctx, retryCtx)
			if decision == nil { decision = &TypedRetryDecision[M]{} }
			if !decision.Retry {
				if decision.RewriteError != nil { return nil, decision.RewriteError }
				return nil, err
			}
			lastErr = err
			if attempt < r.config.MaxRetries {
				if execCtx != nil && execCtx.generator != nil {
					execCtx.send(&TypedAgentEvent[M]{Err: &WillRetryError{ErrStr: lastErr.Error(), RetryAttempt: attempt + 1, rejectReason: decision.RejectReason, err: lastErr}})
				}
				applyRetryDecision(&currentInput, &currentOpts, decision)
				delay := decision.Backoff
				if delay == 0 { delay = backoff(ctx, attempt+1) }
				if err := contextAwareSleep(ctx, delay); err != nil { return nil, err }
			}
			continue
		}

		// Read first chunk for verification
		chunk, streamErr := stream.Recv()
		if streamErr != nil && streamErr != io.EOF {
			stream.Close()
			retryCtx := &TypedRetryContext[M]{
				RetryAttempt: attempt + 1, InputMessages: currentInput, Err: streamErr,
			}
			decision := r.config.ShouldRetry(ctx, retryCtx)
			if decision == nil { decision = &TypedRetryDecision[M]{} }
			if !decision.Retry {
				if decision.RewriteError != nil { return nil, decision.RewriteError }
				return nil, streamErr
			}
			lastErr = streamErr
			select { case sig.ch <- streamRetryVerdict{WillRetry: true, Err: streamErr, RejectReason: decision.RejectReason}: default: }
			if attempt < r.config.MaxRetries {
				if execCtx != nil && execCtx.generator != nil {
					execCtx.send(&TypedAgentEvent[M]{Err: &WillRetryError{ErrStr: lastErr.Error(), RetryAttempt: attempt + 1, rejectReason: decision.RejectReason, err: lastErr}})
				}
				applyRetryDecision(&currentInput, &currentOpts, decision)
				delay := decision.Backoff
				if delay == 0 { delay = backoff(ctx, attempt+1) }
				if err := contextAwareSleep(ctx, delay); err != nil { return nil, err }
			}
			continue
		}

		// Collect all chunks for output event, forward to caller
		var allChunks []M
		if streamErr != io.EOF {
			allChunks = append(allChunks, chunk)
		}
		callerCh := schema.NewStreamReader[M]()
		go func() {
			if len(allChunks) > 0 { callerCh.Send(allChunks[0], nil) }
			for {
				c, e := stream.Recv()
				if e == io.EOF { break }
				if e != nil { callerCh.Send(c, e); return }
				allChunks = append(allChunks, c)
				callerCh.Send(c, nil)
			}
			// Send output event with merged message
			if execCtx != nil && execCtx.generator != nil && len(allChunks) > 0 {
				if merged, err := mergeChunks(allChunks); err == nil {
					execCtx.send(typedModelOutputEvent(merged, nil))
				}
			}
			callerCh.Close()
		}()
		select { case sig.ch <- streamRetryVerdict{WillRetry: false}: default: }
		return callerCh, nil
	}
	return nil, &RetryExhaustedError{LastErr: lastErr, TotalRetries: r.config.MaxRetries}
}

func (r *typedRetryModelWrapper[M]) BindTools(tools []*schema.ToolInfo) error { return r.inner.BindTools(tools) }

// WithModelRetry wraps a Model with retry logic.
// When cfg.ShouldRetry is set but MaxRetries is 0, the loop runs exactly once
// (attempt 0), so ShouldRetry returning Retry:true will immediately exhaust
// with RetryExhaustedError{TotalRetries: 0}. Set MaxRetries >= 1 to allow
// ShouldRetry-driven retries to actually retry.
func WithModelRetry[M MessageType](inner Model[M], cfg *TypedModelRetryConfig[M]) Model[M] {
	if cfg == nil || (cfg.MaxRetries <= 0 && cfg.ShouldRetry == nil) { return inner }
	return newTypedRetryModelWrapper(inner, cfg)
}

func applyRetryDecision[M MessageType](input *[]M, opts *[]ModelOption, decision *TypedRetryDecision[M]) {
	if decision.ModifiedInputMessages != nil && decision.PersistModifiedInputMessages {
		*input = decision.ModifiedInputMessages
	} else if decision.ModifiedInputMessages != nil {
		// Apply for the next attempt but don't persist to the original input.
		// Caller must handle revert externally.
		tmp := make([]M, len(decision.ModifiedInputMessages))
		copy(tmp, decision.ModifiedInputMessages)
		*input = tmp
	}
	if decision.AdditionalOptions != nil {
		*opts = append(*opts, decision.AdditionalOptions...)
	}
}

func contextAwareSleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 { return nil }
	select {
	case <-ctx.Done(): return ctx.Err()
	case <-time.After(delay): return nil
	}
}

func mergeChunks[M MessageType](chunks []M) (M, error) {
	var zero M
	if len(chunks) == 0 { return zero, nil }
	switch c := any(chunks).(type) {
	case []*schema.Message:
		merged, err := schema.ConcatMessages(c)
		if err != nil { return zero, err }
		return any(merged).(M), nil
	case []*schema.AgenticMessage:
		merged, err := schema.ConcatAgenticMessages(c)
		if err != nil { return zero, err }
		return any(merged).(M), nil
	}
	return chunks[0], nil
}

// streamRetryVerdict is the internal retry signal for streaming retry.
type streamRetryVerdict struct {
	WillRetry    bool
	Err          error
	RejectReason any
}

type retrySignal struct {
	ch chan streamRetryVerdict
}

func (rs *retrySignal) consume() streamRetryVerdict {
	if rs == nil { return streamRetryVerdict{} }
	select {
	case v := <-rs.ch: return v
	default: return streamRetryVerdict{}
	}
}

// WithRetry attaches retry configuration to an option.
func WithRetry[M MessageType](cfg *TypedModelRetryConfig[M]) ModelOption {
	return &typedModelOption[M]{f: func(o *modelOptions[M]) { o.RetryConfig = cfg }}
}

