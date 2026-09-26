// Package canvas — directMessageDownstream unit tests.
package canvas

import "testing"

// TestDirectMessageDownstream covers the mixed-downstream contract:
// deferral is only allowed when every direct downstream is a Message.
func TestDirectMessageDownstream(t *testing.T) {
	cases := []struct {
		name     string
		canvas   *Canvas
		cpnID    string
		deferred bool
	}{
		{
			name:     "nil canvas",
			canvas:   nil,
			cpnID:    "agent_0",
			deferred: false,
		},
		{
			name: "unknown component",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"agent_0": {
						Obj:        CanvasComponentObj{ComponentName: "Agent"},
						Downstream: []string{"message_0"},
					},
				},
			},
			cpnID:    "missing",
			deferred: false,
		},
		{
			name: "no downstream keeps eager",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"agent_0": {
						Obj:        CanvasComponentObj{ComponentName: "Agent"},
						Downstream: []string{},
					},
				},
			},
			cpnID:    "agent_0",
			deferred: false,
		},
		{
			name: "sole message downstream defers",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"agent_0": {
						Obj:        CanvasComponentObj{ComponentName: "Agent"},
						Downstream: []string{"message_0"},
					},
					"message_0": {
						Obj:        CanvasComponentObj{ComponentName: "Message"},
						Downstream: []string{},
					},
				},
			},
			cpnID:    "agent_0",
			deferred: true,
		},
		{
			name: "two message downstreams defer",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"agent_0": {
						Obj:        CanvasComponentObj{ComponentName: "Agent"},
						Downstream: []string{"message_0", "message_1"},
					},
					"message_0": {
						Obj:        CanvasComponentObj{ComponentName: "Message"},
						Downstream: []string{},
					},
					"message_1": {
						Obj:        CanvasComponentObj{ComponentName: "Message"},
						Downstream: []string{},
					},
				},
			},
			cpnID:    "agent_0",
			deferred: true,
		},
		{
			// Regression for the mixed-branch bug: an Agent feeding both an
			// Agent and a Message must stay eager, otherwise the downstream
			// Agent receives the opaque deferred stream instead of the answer.
			name: "mixed agent and message downstream stays eager",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"agent_0": {
						Obj:        CanvasComponentObj{ComponentName: "Agent"},
						Downstream: []string{"agent_1", "message_0"},
					},
					"agent_1": {
						Obj:        CanvasComponentObj{ComponentName: "Agent"},
						Downstream: []string{},
					},
					"message_0": {
						Obj:        CanvasComponentObj{ComponentName: "Message"},
						Downstream: []string{},
					},
				},
			},
			cpnID:    "agent_0",
			deferred: false,
		},
		{
			name: "message first then agent still stays eager",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"agent_0": {
						Obj:        CanvasComponentObj{ComponentName: "Agent"},
						Downstream: []string{"message_0", "agent_1"},
					},
					"agent_1": {
						Obj:        CanvasComponentObj{ComponentName: "Agent"},
						Downstream: []string{},
					},
					"message_0": {
						Obj:        CanvasComponentObj{ComponentName: "Message"},
						Downstream: []string{},
					},
				},
			},
			cpnID:    "agent_0",
			deferred: false,
		},
		{
			name: "non-message downstream only keeps eager",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"agent_0": {
						Obj:        CanvasComponentObj{ComponentName: "Agent"},
						Downstream: []string{"fillup_0"},
					},
					"fillup_0": {
						Obj:        CanvasComponentObj{ComponentName: "Fillup"},
						Downstream: []string{},
					},
				},
			},
			cpnID:    "agent_0",
			deferred: false,
		},
		{
			name: "dangling downstream reference stays eager",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"agent_0": {
						Obj:        CanvasComponentObj{ComponentName: "Agent"},
						Downstream: []string{"message_0", "ghost_0"},
					},
					"message_0": {
						Obj:        CanvasComponentObj{ComponentName: "Message"},
						Downstream: []string{},
					},
				},
			},
			cpnID:    "agent_0",
			deferred: false,
		},
		{
			name: "component name is case insensitive",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"agent_0": {
						Obj:        CanvasComponentObj{ComponentName: "Agent"},
						Downstream: []string{"message_0"},
					},
					"message_0": {
						Obj:        CanvasComponentObj{ComponentName: "message"},
						Downstream: []string{},
					},
				},
			},
			cpnID:    "agent_0",
			deferred: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := directMessageDownstream(tc.canvas, tc.cpnID); got != tc.deferred {
				t.Errorf("directMessageDownstream(%q) = %v, want %v", tc.cpnID, got, tc.deferred)
			}
		})
	}
}
