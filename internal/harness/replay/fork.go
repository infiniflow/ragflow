package replay

import (
	"context"

	"ragflow/internal/harness/events"
)

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
	// When set, Fork will replay up to ForkPoint, then hand off to real execution.
	// When nil, Fork replays deterministically from EventLog alone.
	ForkEngine interface{}

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
}

// Fork creates a branched execution from a specified point in the trace.
// Events up to ForkPoint are replayed from the original store.
// After ForkPoint, if ForkEngine is set, execution hands off to the real
// graph engine; otherwise replay continues deterministically with overrides.
func (e *ReplayEngine) Fork(ctx context.Context, cfg *ForkConfig) (*ForkResult, error) {
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

	// Replay events up to (but not including) the fork point.
	replayResult, err := e.Replay(ctx, &ReplayConfig{
		Store:         store,
		TraceID:       cfg.TraceID,
		End:           forkEvent.Clock - 1,
		ModelOverride: cfg.ModelOverride,
		ToolOverride:  cfg.ToolOverride,
	})
	if err != nil {
		return nil, err
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

	// Collect events.
	if cfg.OutputStore != nil {
		// Write pre-fork events.
		if len(replayResult.Events) > 0 {
			if err := cfg.OutputStore.Append(ctx, replayResult.Events...); err != nil {
				return nil, err
			}
		}
		if err := cfg.OutputStore.Append(ctx, forkMarker); err != nil {
			return nil, err
		}
	}

	result.ForkEvents = append(replayResult.Events, forkMarker)

	// If a fork engine is provided, hand off to real graph execution.
	if cfg.ForkEngine != nil {
		// TODO: When ForkEngine is set, continue execution via the real engine.
		// This requires the engine to accept a checkpoint/state captured at the
		// fork point. Integration with Engine.Resume will be added in a future
		// iteration.
	}

	return result, nil
}

func errEventNotFound(id events.EventID) error {
	return errorf("event not found: %s", id)
}
