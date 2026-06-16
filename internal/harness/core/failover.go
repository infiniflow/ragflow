package core

import (
	"context"
	"fmt"

	"ragflow/internal/harness/core/schema"
)

// FailoverConfig configures model failover behavior.
type FailoverConfig[M MessageType] struct {
	// Models contains backup models tried in order after the primary.
	Models []Model[M]
	// ShouldFailover is called to decide whether to try the next model.
	ShouldFailover func(ctx context.Context, err error) bool
	// GetFailoverModel is called to dynamically select a failover model.
	GetFailoverModel func(ctx context.Context, err error) Model[M]
}

type FailoverConfigMsg = FailoverConfig[*schema.Message]

// failoverModel provides failover across multiple chat models.
type failoverModel[M MessageType] struct {
	models           []Model[M]
	shouldFailover   func(ctx context.Context, err error) bool
	getFailoverModel func(ctx context.Context, err error) Model[M]
}

func newFailoverModel[M MessageType](models []Model[M], cfg *FailoverConfig[M]) Model[M] {
	var sf func(ctx context.Context, err error) bool
	var gf func(ctx context.Context, err error) Model[M]
	if cfg != nil {
		sf = cfg.ShouldFailover
		gf = cfg.GetFailoverModel
	}
	return &failoverModel[M]{
		models:           models,
		shouldFailover:   sf,
		getFailoverModel: gf,
	}
}

func (m *failoverModel[M]) Generate(ctx context.Context, input []M, opts ...ModelOption) (M, error) {
	var lastErr error
	for i, model := range m.models {
		if i > 0 && m.shouldFailover != nil && !m.shouldFailover(ctx, lastErr) {
			var zero M
			return zero, fmt.Errorf("failover skipped: %w", lastErr)
		}
		r, err := model.Generate(ctx, input, opts...)
		if err == nil { return r, nil }
		lastErr = fmt.Errorf("model[%d]: %w", i, err)
	}
	var zero M
	return zero, fmt.Errorf("all %d models failed: %w", len(m.models), lastErr)
}

func (m *failoverModel[M]) Stream(ctx context.Context, input []M, opts ...ModelOption) (*schema.StreamReader[M], error) {
	var lastErr error
	for i, model := range m.models {
		if i > 0 && m.shouldFailover != nil && !m.shouldFailover(ctx, lastErr) {
			return nil, fmt.Errorf("failover skipped: %w", lastErr)
		}
		s, err := model.Stream(ctx, input, opts...)
		if err == nil { return s, nil }
		lastErr = fmt.Errorf("model[%d]: %w", i, err)
	}
	return nil, fmt.Errorf("all %d models failed to stream: %w", len(m.models), lastErr)
}

func (m *failoverModel[M]) BindTools(tools []*schema.ToolInfo) error {
	for _, model := range m.models {
		if err := model.BindTools(tools); err != nil { return err }
	}
	return nil
}

// WithModelFailover creates a failover-wrapped model.
func WithModelFailover[M MessageType](primary Model[M], secondaries ...Model[M]) Model[M] {
	all := append([]Model[M]{primary}, secondaries...)
	return newFailoverModel(all, nil)
}
