// Package types tests configuration functionality.
package types

import (
	"testing"
)

func TestDurabilityTypes(t *testing.T) {
	// Test that all durability modes are defined
	tests := []struct {
		name       string
		durability Durability
	}{
		{"Sync", DurabilitySync},
		{"Async", DurabilityAsync},
		{"Exit", DurabilityExit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.durability == "" {
				t.Errorf("Durability mode %s should not be empty", tt.name)
			}
		})
	}
}

func TestRunnableConfig_WithDurability(t *testing.T) {
	config := NewRunnableConfig()

	// Test default durability
	if config.Durability != DurabilitySync {
		t.Errorf("Default durability should be Sync, got %s", config.Durability)
	}

	// Test WithDurability
	config = config.WithDurability(DurabilityAsync)
	if config.Durability != DurabilityAsync {
		t.Errorf("Expected DurabilityAsync, got %s", config.Durability)
	}

	config = config.WithDurability(DurabilityExit)
	if config.Durability != DurabilityExit {
		t.Errorf("Expected DurabilityExit, got %s", config.Durability)
	}
}

func TestRunnableConfig_Merge(t *testing.T) {
	config1 := NewRunnableConfig().WithDurability(DurabilitySync)
	config2 := NewRunnableConfig().WithDurability(DurabilityAsync)

	config1.Merge(config2)

	if config1.Durability != DurabilityAsync {
		t.Errorf("Expected DurabilityAsync after merge, got %s", config1.Durability)
	}
}

func TestDurabilityString(t *testing.T) {
	tests := []struct {
		durability Durability
		expected   string
	}{
		{DurabilitySync, "sync"},
		{DurabilityAsync, "async"},
		{DurabilityExit, "exit"},
	}

	for _, tt := range tests {
		t.Run(string(tt.durability), func(t *testing.T) {
			if string(tt.durability) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.durability))
			}
		})
	}
}
