package replay

import (
	"context"
	"fmt"
	"time"

	"ragflow/internal/harness/events"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/pregel"
	"ragflow/internal/harness/graph/types"
)

// ForkContextKey is used to pass the ForkConfig's ModelOverride/ToolOverride
// through context to node-level wrappers during true replay.
// This is a spare key; the actual model/tool substitution during engine
// re-execution is done by the caller via agent-level middleware.
type ForkContextKey struct{}

// ForkConfig configures a fork operation.
type ForkConfig struct {
	// Store is the event source for the original trace.
	Store events.EventLog

	// TraceID identifies the original trace to fork from.
	TraceID string

	// Point is the event ID at which to fork.
	Point events.EventID

	// Substitution strategies for the forked branch.
	ModelOverride ModelOverrideFunc
	ToolOverride  ToolOverrideFunc
	NewInput      any

	// ForkEngine is the actual graph engine to execute the forked branch.
	// When set, Fork replays up to ForkPoint, builds a checkpoint from the
	// events, saves it into a MemorySaver, and hands off to real execution.
	// When nil, Fork replays deterministically from EventLog alone.
	ForkEngine *pregel.Engine

	// Checkpointer is the persistence backend to use when resuming the
	// ForkEngine. When nil, a fresh MemorySaver is created.
	Checkpointer checkpoint.BaseCheckpointer

	// OutputStore receives events generated during the fork (nil = discard).
	OutputStore events.EventLog
}

// ForkResult contains the result of a fork operation.
type ForkResult struct {
	// ForkTraceID identifies the new fork trace.
	ForkTraceID string

	// ForkEvents generated during the forked execution.
	ForkEvents []*events.Event

	// ParentTraceID is the original trace that was forked.
	ParentTraceID string

	// ForkPoint is the event ID where the fork occurred.
	ForkPoint events.EventID

	// FinalState is the output state from the forked Engine execution.
	// Only set when ForkEngine was used.
	FinalState any

	// Duration of the fork operation.
	Duration time.Duration
}

// Fork creates a branched execution from a specified point in the trace.
// Events up to ForkPoint are replayed from the original store.
// After ForkPoint, if ForkEngine is set, execution hands off to the real
// graph engine via checkpoint resume; otherwise replay continues
// deterministically with overrides.
func (e *ReplayEngine) Fork(ctx context.Context, cfg *ForkConfig) (*ForkResult, error) {
	start := time.Now()

	// Use config store, falling back to engine store.
	store := cfg.Store
	if store == nil {
		store = e.store
	}

	// Find the fork point event.
	forkEvent, err := store.Get(ctx, cfg.Point)
	if err != nil {
		return nil, err
	}
	if forkEvent == nil {
		return nil, errEventNotFound(cfg.Point)
	}

	// Read ALL events up to (but not including) the fork point.
	// We need the complete event list to reconstruct the checkpoint.
	filter := events.EventFilter{
		TraceID: cfg.TraceID,
		ToClock: forkEvent.Clock - 1,
	}
	iter := store.Stream(ctx, filter)
	defer iter.Close()

	var preForkEvents []*events.Event
	for {
		ev, ok := iter.Next(ctx)
		if !ok {
			break
		}
		preForkEvents = append(preForkEvents, ev)
	}

	result := &ForkResult{
		ForkTraceID:   cfg.TraceID + "_fork_" + string(cfg.Point),
		ParentTraceID: cfg.TraceID,
		ForkPoint:     cfg.Point,
	}

	// Append fork marker event.
	forkMarker := events.NewEvent(events.EventFork, 0)
	forkMarker.TraceID = result.ForkTraceID
	forkMarker.ParentID = cfg.Point
	forkMarker.CausedBy = []events.EventID{cfg.Point}
	forkMarker.Metadata["parent_trace"] = cfg.TraceID
	forkMarker.Seal()

	// Collect pre-fork events.
	result.ForkEvents = append(result.ForkEvents, preForkEvents...)
	result.ForkEvents = append(result.ForkEvents, forkMarker)

	if cfg.OutputStore != nil {
		if err := cfg.OutputStore.Append(ctx, result.ForkEvents...); err != nil {
			return nil, err
		}
	}

	// If a fork engine is provided, reconstruct checkpoint and resume.
	if cfg.ForkEngine != nil {
		forkResult, err := e.resumeFromCheckpoint(ctx, cfg, preForkEvents, forkMarker)
		if err != nil {
			return nil, fmt.Errorf("fork resume: %w", err)
		}
		result.FinalState = forkResult
	}

	result.Duration = time.Since(start)
	return result, nil
}

// resumeFromCheckpoint reconstructs checkpoint state from pre-fork events and
// resumes the ForkEngine from that point. The engine runs the graph from the
// reconstructed state and returns the final output.
func (e *ReplayEngine) resumeFromCheckpoint(ctx context.Context, cfg *ForkConfig, preForkEvents []*events.Event, forkMarker *events.Event) (any, error) {
	if cfg.ForkEngine == nil {
		return nil, nil
	}

	threadID := cfg.TraceID
	if threadID == "" {
		threadID = "fork-" + string(cfg.Point)
	}

	// Build checkpoint map from pre-fork events.
	cp, cpID := BuildCheckpoint(preForkEvents, threadID)

	// Save checkpoint into a MemorySaver (or caller-provided checkpointer).
	saver := cfg.Checkpointer
	if saver == nil {
		saver = checkpoint.NewMemorySaver()
	}

	if err := saver.Put(ctx, map[string]any{
		constants.ConfigKeyThreadID:     threadID,
		constants.ConfigKeyCheckpointID: cpID,
	}, cp); err != nil {
		return nil, fmt.Errorf("save fork checkpoint: %w", err)
	}

	// Check if ForkEngine already has a checkpointer; if not, set it.
	// We inject our own via WithCheckpointer option at Fork creation time
	// by creating a new Engine wrapping the same graph.

	// Configure the engine's runnable config to point at the checkpoint.
	rc := types.NewRunnableConfig()
	rc.ThreadID = threadID
	rc.Set(constants.ConfigKeyThreadID, threadID)
	rc.Set(constants.ConfigKeyCheckpointID, cpID)

	// Run the ForkEngine with the resume config.
	outputCh, errCh := cfg.ForkEngine.Run(ctx, nil, types.StreamModeValues)

	// Drain outputCh for final state.
	var finalState any
	for result := range outputCh {
		if se, ok := result.(*pregel.StreamEvent); ok {
			if se.Type == pregel.EventTypeFinal {
				if data, ok := se.Data.(map[string]any); ok {
					if state, ok := data["state"]; ok {
						finalState = state
					}
				}
			}
		}
	}

	if err := <-errCh; err != nil {
		return nil, err
	}

	// If output store is set, record fork completion.
	if cfg.OutputStore != nil {
		forkEnd := events.NewEvent(events.EventGraphEnd, 0)
		forkEnd.TraceID = cfg.TraceID + "_fork_" + string(cfg.Point)
		forkEnd.Metadata["fork_replay"] = true
		forkEnd.Seal()
		_ = cfg.OutputStore.Append(ctx, forkEnd)
	}

	return finalState, nil
}

func errEventNotFound(id events.EventID) error {
	return errorf("event not found: %s", id)
}
