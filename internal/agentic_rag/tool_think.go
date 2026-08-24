//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package agentic_rag

import (
	"context"
	"encoding/json"
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// thinkToolName is a stateless placeholder that records a reasoning step
// without side effects — the memory/persistence aspect is intentionally not
// kept in this port.
const thinkToolName = "think"

const thinkToolDescription = `A detailed tool for dynamic and reflective problem-solving through thoughts.

This tool helps analyze problems through a flexible thinking process that can adapt and evolve.
Each thought can build on, question, or revise previous insights as understanding deepens.

## When to Use This Tool

- Breaking down complex problems into steps
- Planning and design with room for revision
- Analysis that might need course correction
- Problems where the full scope might not be clear initially
- Problems that require a multi-step solution
- Tasks that need to maintain context over multiple steps
- Situations where irrelevant information needs to be filtered out

## Key Features

- You can adjust total_thoughts up or down as you progress
- You can question or revise previous thoughts
- You can add more thoughts even after reaching what seemed like the end
- You can express uncertainty and explore alternative approaches
- Not every thought needs to build linearly - you can branch or backtrack
- When your thinking is complete, deliver your answer by writing it as your plain reply and stopping (no further tool calls). NEVER include the final answer directly in a thought.

## Parameters Explained

- **thought**: Your current thinking step. Write in natural, user-friendly language. NEVER mention tool names in your thinking process. Focus on WHAT you're trying to find and WHY, not HOW (which tools you'll use).
- **next_thought_needed**: True if you need more thinking, even if at what seemed like the end
- **thought_number**: Current number in sequence (can go beyond initial total if needed)
- **total_thoughts**: Current estimate of thoughts needed (can be adjusted up/down)
- **is_revision**: A boolean indicating if this thought revises previous thinking
- **revises_thought**: If is_revision is true, which thought number is being reconsidered
- **branch_from_thought**: If branching, which thought number is the branching point
- **branch_id**: Identifier for the current branch (if any)
- **needs_more_thoughts**: If reaching end but realizing more thoughts needed`

// thinkArgs is the JSON the model sends into InvokableRun.
type thinkArgs struct {
	Thought           string `json:"thought"`
	NextThoughtNeeded bool   `json:"next_thought_needed"`
	ThoughtNumber     int    `json:"thought_number"`
	TotalThoughts     int    `json:"total_thoughts"`
	IsRevision        bool   `json:"is_revision,omitempty"`
	RevisesThought    *int   `json:"revises_thought,omitempty"`
	BranchFromThought *int   `json:"branch_from_thought,omitempty"`
	BranchID          string `json:"branch_id,omitempty"`
	NeedsMoreThoughts bool   `json:"needs_more_thoughts,omitempty"`
}

// ThinkTool is a stateless reflective-thinking tool. It validates the reasoning
// step and echoes back a completion marker; it does not persist history.
type ThinkTool struct{}

// NewThinkTool returns a ThinkTool implementing eino's tool.InvokableTool.
func NewThinkTool() *ThinkTool {
	return &ThinkTool{}
}

// Info returns the tool's metadata for the chat model.
func (t *ThinkTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: thinkToolName,
		Desc: thinkToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"thought": {
				Type: schema.String, Required: true,
				Desc: "Your current thinking step. Write in natural, user-friendly language. NEVER mention tool names. Focus on WHAT you're trying to find and WHY, not HOW.",
			},
			"next_thought_needed": {
				Type: schema.Boolean, Required: true,
				Desc: "Whether another thought step is needed.",
			},
			"thought_number": {
				Type: schema.Number, Required: true,
				Desc: "Current thought number (>= 1).",
			},
			"total_thoughts": {
				Type: schema.Number, Required: true,
				Desc: "Estimated total thoughts needed (>= 1).",
			},
			"is_revision": {
				Type: schema.Boolean,
				Desc: "Whether this revises previous thinking.",
			},
			"revises_thought": {
				Type: schema.Number,
				Desc: "Which thought number is being reconsidered.",
			},
			"branch_from_thought": {
				Type: schema.Number,
				Desc: "Branching point thought number.",
			},
			"branch_id": {
				Type: schema.String,
				Desc: "Branch identifier.",
			},
			"needs_more_thoughts": {
				Type: schema.Boolean,
				Desc: "If more thoughts are needed.",
			},
		}),
	}, nil
}

// InvokableRun validates the reasoning step and returns a completion marker.
// It is stateless: no history is retained across calls.
func (t *ThinkTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	var args thinkArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("think: parse arguments: %w", err)
	}
	if args.Thought == "" {
		return "", fmt.Errorf("think: thought must be a non-empty string")
	}
	if args.ThoughtNumber < 1 {
		return "", fmt.Errorf("think: thought_number must be >= 1")
	}
	if args.TotalThoughts < 1 {
		return "", fmt.Errorf("think: total_thoughts must be >= 1")
	}

	incomplete := args.NextThoughtNeeded || args.NeedsMoreThoughts || args.ThoughtNumber < args.TotalThoughts
	if incomplete {
		return "Thought process recorded - unfinished steps remain, continue exploring and calling tools", nil
	}
	return "Thought process recorded", nil
}
