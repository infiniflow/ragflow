package service

import (
	"testing"

	"ragflow/internal/entity"
)

// TestBootstrap_RedisUnavailable verifies that when the global Redis client
// is not initialized or unavailable, Bootstrap gracefully returns an error
// rather than panicking.
func TestBootstrap_RedisUnavailable(t *testing.T) {
	svc := NewCanvasReplicaService()

	// Given an empty DSL
	dsl := entity.JSONMap{}

	// When calling Bootstrap without initializing the cache package
	payload, err := svc.Bootstrap("canvas-1", "tenant-1", "user-1", dsl, "agent_canvas", "Test Title")

	// Then it should return an error regarding Redis availability
	if err == nil {
		t.Fatalf("expected error due to uninitialized redis client, got nil")
	}

	expectedErrMsg := "redis client not initialized or unavailable"
	if err.Error() != expectedErrMsg {
		t.Errorf("expected error message '%s', got '%v'", expectedErrMsg, err)
	}

	if payload != nil {
		t.Errorf("expected payload to be nil when redis fails, got %v", payload)
	}
}
