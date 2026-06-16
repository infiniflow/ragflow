package core

import (
	"context"
	"fmt"
	"time"
)

// stopPhase tracks the stop commitment lifecycle.
type stopPhase uint8

const (
	stopOpen stopPhase = iota
	stopIdleWaiting
	stopCommitted
)

// preemptTurnPhase tracks the preempt turn lifecycle.
type preemptTurnPhase uint8

const (
	preemptTurnIdle preemptTurnPhase = iota
	preemptTurnPlanning
	preemptTurnActive
)

func (p preemptTurnPhase) String() string {
	switch p {
	case preemptTurnIdle:
		return "idle"
	case preemptTurnPlanning:
		return "planning"
	case preemptTurnActive:
		return "active"
	default:
		return "unknown"
	}
}

// preemptTurnSnapshot captures the turn state at Push time.
type preemptTurnSnapshot struct {
	hasTargetTurn bool
	turnID        uint64
	ctx           context.Context
	tc            any
}

// cancelRequestState holds cancel configuration with optional deadline.
type cancelRequestState struct {
	cfg             cancelConfig
	timeoutDeadline *time.Time
}

func parseCancelOptions(opts ...CancelOption) cancelConfig {
	cfg := cancelConfig{Mode: CancelImmediate}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

func newCancelRequestState(opts []CancelOption, now time.Time) cancelRequestState {
	cfg := parseCancelOptions(opts...)
	var deadline *time.Time
	if cfg.Timeout != nil && *cfg.Timeout > 0 && cfg.Mode != CancelImmediate {
		d := now.Add(*cfg.Timeout)
		deadline = &d
	}
	cfg.Timeout = nil

	return cancelRequestState{
		cfg:             cfg,
		timeoutDeadline: deadline,
	}
}

func (s *cancelRequestState) merge(opts []CancelOption, now time.Time) {
	if opts == nil {
		return
	}

	next := newCancelRequestState(opts, now)
	if s.cfg.Mode == CancelImmediate || next.cfg.Mode == CancelImmediate {
		s.cfg.Mode = CancelImmediate
		s.timeoutDeadline = nil
	} else {
		s.cfg.Mode |= next.cfg.Mode
		if next.timeoutDeadline != nil {
			if s.timeoutDeadline == nil || next.timeoutDeadline.Before(*s.timeoutDeadline) {
				deadline := *next.timeoutDeadline
				s.timeoutDeadline = &deadline
			}
		}
	}
	if next.cfg.Recursive {
		s.cfg.Recursive = true
	}
}

func (s cancelRequestState) cancelOptions(now time.Time) []CancelOption {
	cfg := s.cfg
	if cfg.Mode != CancelImmediate && s.timeoutDeadline != nil {
		remaining := s.timeoutDeadline.Sub(now)
		if remaining <= 0 {
			cfg.Mode = CancelImmediate
			cfg.Timeout = nil
		} else {
			cfg.Timeout = &remaining
		}
	}

	opts := []CancelOption{WithCancelMode(cfg.Mode)}
	if cfg.Recursive {
		opts = append(opts, WithRecursiveCancel())
	}
	if cfg.Timeout != nil {
		opts = append(opts, WithCancelTimeout(*cfg.Timeout))
	}
	return opts
}

// AgentLoopConfig is the configuration for creating a AgentLoop.
type AgentLoopConfig[T any] struct {
	GenInput func(ctx context.Context, loop *AgentLoop[T], items []T) (*GenInputResult[T], error)

	GenResume func(ctx context.Context, loop *AgentLoop[T], interruptedItems, unhandledItems, newItems []T) (*GenResumeResult[T], error)

	PrepareAgent func(ctx context.Context, loop *AgentLoop[T], consumed []T) (Agent, error)

	OnAgentEvents func(ctx context.Context, tc *TurnContext[T], events *AsyncIterator[*AgentEvent]) error

	Store CheckPointStore

	CheckpointID string
}

// GenInputResult contains the result of GenInput processing.
type GenInputResult[T any] struct {
	RunCtx    context.Context
	Input     *AgentInput
	RunOpts   []RunOption
	Consumed  []T
	Remaining []T
}

// GenResumeResult contains the result of GenResume processing.
type GenResumeResult[T any] struct {
	RunCtx        context.Context
	RunOpts       []RunOption
	ResumeParams  *ResumeParams
	Consumed      []T
	Remaining     []T
}

type turnRunSpec[T any] struct {
	runCtx       context.Context
	input        *AgentInput
	runOpts      []RunOption
	resumeParams *ResumeParams
	isResume     bool
	consumed     []T
	resumeBytes  []byte
}

type turnPlan[T any] struct {
	turnCtx   context.Context
	remaining []T
	spec      *turnRunSpec[T]
}

// AgentLoopState is returned when AgentLoop exits.
type AgentLoopState[T any] struct {
	ExitReason          error
	UnhandledItems      []T
	InterruptedItems    []T
	StopCause           string
	CheckpointAttempted bool
	CheckpointErr       error
	TakeLateItems       func() []T
}

// TurnContext provides per-turn context to the OnAgentEvents callback.
type TurnContext[T any] struct {
	Loop      *AgentLoop[T]
	Consumed  []T
	Preempted <-chan struct{}
	Stopped   <-chan struct{}
	StopCause func() string
}

type agentLoopCheckpoint[T any] struct {
	RunnerCheckpoint []byte
	HasRunnerState   bool
	UnhandledItems   []T
	CanceledItems    []T
}

type agentLoopPendingResume[T any] struct {
	interrupted []T
	unhandled   []T
	newItems    []T
	resumeBytes []byte
}

// SafePoint describes at which boundary the agent may be cancelled.
type SafePoint int

const (
	AfterChatModel SafePoint = 1 << iota
	AfterToolCalls
	AnySafePoint = AfterChatModel | AfterToolCalls
)

func (sp SafePoint) toCancelMode() CancelMode {
	var mode CancelMode
	if sp&AfterToolCalls != 0 {
		mode |= CancelAfterToolCalls
	}
	if sp&AfterChatModel != 0 {
		mode |= CancelAfterChatModel
	}
	return mode
}

type stopConfig struct {
	agentCancelOpts []CancelOption
	skipCheckpoint  bool
	stopCause       string
	idleFor         time.Duration
	timeout         *time.Duration
}

type pushConfig[T any] struct {
	preempt         bool
	preemptDelay    time.Duration
	agentCancelOpts []CancelOption
	pushStrategy    func(context.Context, *TurnContext[T]) []PushOption[T]
}

// StopOption is an option for Stop().
type StopOption func(*stopConfig)

// PushOption is an option for Push().
type PushOption[T any] func(*pushConfig[T])

// InterruptError signals a business interrupt during a turn.
type InterruptError struct {
	InterruptContexts []*InterruptCtx
}

func (e *InterruptError) Error() string {
	return fmt.Sprintf("agent interrupted: %d context(s)", len(e.InterruptContexts))
}

// stopDecision communicates the result of a stop request.
type stopDecision struct {
	commit   bool
	wakeIdle bool
}

type stopCancelRequest struct {
	cancel cancelRequestState
}

func newStopCancelRequest(opts []CancelOption, now time.Time) *stopCancelRequest {
	return &stopCancelRequest{cancel: newCancelRequestState(opts, now)}
}

func (r *stopCancelRequest) merge(opts []CancelOption, now time.Time) {
	if r == nil {
		return
	}
	r.cancel.merge(opts, now)
}

func (r *stopCancelRequest) cancelOptions(now time.Time) []CancelOption {
	if r == nil {
		return nil
	}
	return r.cancel.cancelOptions(now)
}

// preemptRequest holds pending preempt state.
type preemptRequest struct {
	cancel   cancelRequestState
	ackChans []chan struct{}
}

func newPreemptRequest(ack chan struct{}, opts []CancelOption, now time.Time) *preemptRequest {
	req := &preemptRequest{cancel: newCancelRequestState(opts, now)}
	if ack != nil {
		req.ackChans = append(req.ackChans, ack)
	}
	return req
}

func (r *preemptRequest) ack() {
	if r == nil {
		return
	}
	for _, ack := range r.ackChans {
		close(ack)
	}
	r.ackChans = nil
}

func (r *preemptRequest) merge(ack chan struct{}, opts []CancelOption, now time.Time) {
	if ack != nil {
		r.ackChans = append(r.ackChans, ack)
	}
	r.cancel.merge(opts, now)
}

func (r *preemptRequest) cancelOptions(now time.Time) []CancelOption {
	if r == nil {
		return nil
	}
	return r.cancel.cancelOptions(now)
}
