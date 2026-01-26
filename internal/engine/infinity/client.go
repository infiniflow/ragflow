package infinity

import (
	"context"
	"fmt"

	"ragflow/internal/config"
)

// Engine Infinity engine implementation
// Note: Infinity Go SDK is not yet available. This is a placeholder implementation.
type infinityEngine struct {
	config *config.InfinityConfig
}

// NewEngine creates an Infinity engine
// Note: This is a placeholder implementation waiting for official Infinity Go SDK
func NewEngine(cfg interface{}) (*infinityEngine, error) {
	infConfig, ok := cfg.(*config.InfinityConfig)
	if !ok {
		return nil, fmt.Errorf("invalid infinity config type, expected *config.InfinityConfig")
	}

	engine := &infinityEngine{
		config: infConfig,
	}

	return engine, nil
}

// Type returns the engine type
func (e *infinityEngine) Type() string {
	return "infinity"
}

// Ping health check
func (e *infinityEngine) Ping(ctx context.Context) error {
	return fmt.Errorf("infinity engine not implemented: waiting for official Go SDK")
}

// Close closes the connection
func (e *infinityEngine) Close() error {
	return nil
}
