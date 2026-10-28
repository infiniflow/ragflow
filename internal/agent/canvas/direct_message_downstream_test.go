// Package canvas — directMessageDownstream unit tests.
package canvas

import (
	"testing"

	"ragflow/internal/agent/runtime"
)

// TestDirectMessageDownstream covers the mixed-downstream contract:
// deferral is only allowed when every direct downstream is a terminal Message.
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
			name: "intermediate message stays eager",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"agent_0": {
						Obj:        CanvasComponentObj{ComponentName: "Agent"},
						Downstream: []string{"message_0"},
					},
					"message_0": {
						Obj:        CanvasComponentObj{ComponentName: "Message"},
						Downstream: []string{"llm_0"},
					},
					"llm_0": {
						Obj: CanvasComponentObj{ComponentName: "LLM"},
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
		{
			name: "LLM component with sole message downstream defers",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"llm_0": {
						Obj:        CanvasComponentObj{ComponentName: "LLM"},
						Downstream: []string{"message_0"},
					},
					"message_0": {
						Obj:        CanvasComponentObj{ComponentName: "Message"},
						Downstream: []string{},
					},
				},
			},
			cpnID:    "llm_0",
			deferred: true,
		},
		{
			name: "Generate component with sole message downstream defers",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"gen_0": {
						Obj:        CanvasComponentObj{ComponentName: "Generate"},
						Downstream: []string{"message_0"},
					},
					"message_0": {
						Obj:        CanvasComponentObj{ComponentName: "Message"},
						Downstream: []string{},
					},
				},
			},
			cpnID:    "gen_0",
			deferred: true,
		},
		{
			name: "LLM component followed by non-message does not defer",
			canvas: &Canvas{
				Components: map[string]CanvasComponent{
					"llm_0": {
						Obj:        CanvasComponentObj{ComponentName: "LLM"},
						Downstream: []string{"sql_0"},
					},
					"sql_0": {
						Obj:        CanvasComponentObj{ComponentName: "ExeSQL"},
						Downstream: []string{},
					},
				},
			},
			cpnID:    "llm_0",
			deferred: false,
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

func TestResolveComponentExecutionOptions(t *testing.T) {
	makeCanvas := func(name string, downstream []string, msgParams map[string]any) *Canvas {
		c := &Canvas{
			Components: map[string]CanvasComponent{
				"root": {Obj: CanvasComponentObj{ComponentName: name}, Downstream: downstream},
			},
		}
		for _, d := range downstream {
			c.Components[d] = CanvasComponent{
				Obj: CanvasComponentObj{ComponentName: "Message", Params: msgParams},
			}
		}
		return c
	}

	cases := []struct {
		name     string
		canvas   *Canvas
		cpnName  string
		wantOpts runtime.ComponentExecutionOptions
	}{
		{"LLM consuming via text", makeCanvas("LLM", []string{"m0"}, map[string]any{"text": "Ans: {root@content}"}), "LLM", runtime.ComponentExecutionOptions{DeferLLMToMessage: true}},
		{"LLM with json_output disables deferral", &Canvas{Components: map[string]CanvasComponent{
			"root": {Obj: CanvasComponentObj{ComponentName: "LLM", Params: map[string]any{"json_output": true}}, Downstream: []string{"m0"}},
			"m0":   {Obj: CanvasComponentObj{ComponentName: "Message", Params: map[string]any{"text": "{root@content}"}}},
		}}, "LLM", runtime.ComponentExecutionOptions{}},
		{"LLM with output_structure disables deferral", &Canvas{Components: map[string]CanvasComponent{
			"root": {Obj: CanvasComponentObj{ComponentName: "LLM", Params: map[string]any{"output_structure": map[string]any{"key": "val"}}}, Downstream: []string{"m0"}},
			"m0":   {Obj: CanvasComponentObj{ComponentName: "Message", Params: map[string]any{"text": "{root@content}"}}},
		}}, "LLM", runtime.ComponentExecutionOptions{}},
		{"Generate consuming via content", makeCanvas("Generate", []string{"m0"}, map[string]any{"content": []any{"{{root@content}}"}}), "Generate", runtime.ComponentExecutionOptions{DeferLLMToMessage: true}},
		{"LLM constant template", makeCanvas("LLM", []string{"m0"}, map[string]any{"text": "All done!"}), "LLM", runtime.ComponentExecutionOptions{}},
		{"LLM referencing other node", makeCanvas("LLM", []string{"m0"}, map[string]any{"text": "{sql_0@result}"}), "LLM", runtime.ComponentExecutionOptions{}},
		{"LLM multiple messages", makeCanvas("LLM", []string{"m0", "m1"}, map[string]any{"text": "{root@content}"}), "LLM", runtime.ComponentExecutionOptions{}},
		{"Agent multiple messages", makeCanvas("Agent", []string{"m0", "m1"}, nil), "Agent", runtime.ComponentExecutionOptions{DeferAgentToMessage: true}},
		{"Agent without message downstream", &Canvas{Components: map[string]CanvasComponent{
			"root":  {Obj: CanvasComponentObj{ComponentName: "Agent"}, Downstream: []string{"sql_0"}},
			"sql_0": {Obj: CanvasComponentObj{ComponentName: "ExeSQL"}},
		}}, "Agent", runtime.ComponentExecutionOptions{SuppressAgentMessageEvents: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveComponentExecutionOptions(tc.canvas, "root", tc.cpnName)
			if got != tc.wantOpts {
				t.Errorf("resolveComponentExecutionOptions() = %+v, want %+v", got, tc.wantOpts)
			}
		})
	}
}
