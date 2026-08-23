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
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// todoWriteToolName echoes back the current task list. It is stateless: it does
// not persist state, so the model maintains its plan within the conversation
// context.
const todoWriteToolName = "todo_write"

const todoWriteToolDescription = `Use this tool to create and manage a structured task list for retrieval and research tasks. This helps you track progress and organize complex retrieval operations.

**CRITICAL - Focus on Retrieval Tasks Only**:
- This tool is for tracking RETRIEVAL and RESEARCH tasks (e.g., searching datasets, retrieving documents, gathering information)
- DO NOT include summary or synthesis tasks in todo_write - those are handled by the think tool
- Examples of appropriate tasks: "Search for X in dataset", "Retrieve information about Y", "Compare A and B"
- Examples of tasks to EXCLUDE: "Summarize findings", "Generate final answer", "Synthesize results" - these are for the think tool

## When to Use This Tool
Use this tool proactively when a task requires 3 or more distinct retrieval steps, needs careful planning, or when the user provides multiple tasks.

## When NOT to Use This Tool
Skip for single, straightforward, or purely conversational tasks.

## Task States and Management
- pending: not started
- in_progress: currently working on (limit to ONE at a time)
- completed: finished successfully

Mark tasks complete immediately after finishing. Only mark completed when fully accomplished.
The todo_write tool tracks WHAT to retrieve; the think tool handles HOW to synthesize and present the information.`

// todoWriteArgs is the JSON the model sends into InvokableRun.
type todoWriteArgs struct {
	Task  string     `json:"task,omitempty"`
	Steps []planStep `json:"steps"`
}

// planStep is a single step in the retrieval plan.
type planStep struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// TodoWriteTool is a stateless retrieval-planning tool.
type TodoWriteTool struct{}

// NewTodoWriteTool returns a TodoWriteTool implementing eino's tool.InvokableTool.
func NewTodoWriteTool() *TodoWriteTool {
	return &TodoWriteTool{}
}

// Info returns the tool's metadata for the chat model.
func (t *TodoWriteTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: todoWriteToolName,
		Desc: todoWriteToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task": {
				Type: schema.String,
				Desc: "The complex task or question you need to create a plan for.",
			},
			"steps": {
				Type: schema.Array,
				Desc: "Array of plan steps, each with id, description, and status (pending|in_progress|completed).",
				ElemInfo: &schema.ParameterInfo{
					Type: schema.Object,
					SubParams: map[string]*schema.ParameterInfo{
						"id": {
							Type:     schema.String,
							Required: true,
							Desc:     "Unique step identifier.",
						},
						"description": {
							Type:     schema.String,
							Required: true,
							Desc:     "What this step accomplishes.",
						},
						"status": {
							Type: schema.String,
							Desc: "Step state: pending, in_progress, or completed.",
							Enum: []string{"pending", "in_progress", "completed"},
						},
					},
				},
			},
		}),
	}, nil
}

// InvokableRun parses the plan and echoes a formatted task list. Stateless: no
// persistence across calls.
func (t *TodoWriteTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	var args todoWriteArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("todo_write: parse arguments: %w", err)
	}
	if strings.TrimSpace(args.Task) == "" {
		args.Task = "No task description provided"
	}
	return generatePlanOutput(args.Task, args.Steps), nil
}

// generatePlanOutput formats the plan. Emoji are intentionally omitted.
func generatePlanOutput(task string, steps []planStep) string {
	var b strings.Builder
	b.WriteString("Plan created\n\n")
	fmt.Fprintf(&b, "**Task**: %s\n\n", task)

	if len(steps) == 0 {
		b.WriteString("Note: No specific steps provided. It is recommended to create 3-7 retrieval tasks for systematic research.\n\n")
		b.WriteString("Suggested retrieval workflow (retrieval only, excluding summarization):\n")
		b.WriteString("1. Use grep_chunks to search keywords and locate relevant documents\n")
		b.WriteString("2. Use search_chunks for semantic search to retrieve relevant content\n")
		b.WriteString("\nNote: Summarization and synthesis are handled by the think tool. Do not add summarization tasks here.\n")
		return b.String()
	}

	pending, inProgress, completed := 0, 0, 0
	for _, s := range steps {
		switch s.Status {
		case "pending":
			pending++
		case "in_progress":
			inProgress++
		case "completed":
			completed++
		}
	}

	b.WriteString("**Plan Steps**:\n\n")
	for i, s := range steps {
		fmt.Fprintf(&b, "  %d. [%s] %s\n", i+1, s.Status, s.Description)
	}

	b.WriteString("\n=== Task Progress ===\n")
	fmt.Fprintf(&b, "Total: %d tasks\n", len(steps))
	fmt.Fprintf(&b, "Completed: %d\n", completed)
	fmt.Fprintf(&b, "In Progress: %d\n", inProgress)
	fmt.Fprintf(&b, "Pending: %d\n", pending)

	b.WriteString("\n=== Important Reminder ===\n")
	if pending+inProgress > 0 {
		fmt.Fprintf(&b, "%d tasks remaining!\n\n", pending+inProgress)
		b.WriteString("All tasks must be completed before summarizing or drawing conclusions.\n\n")
		b.WriteString("Next steps:\n")
		if inProgress > 0 {
			b.WriteString("- Continue completing tasks currently in progress\n")
		}
		if pending > 0 {
			fmt.Fprintf(&b, "- Start processing %d pending tasks\n", pending)
			b.WriteString("- Complete each task in order, do not skip\n")
		}
		b.WriteString("- After completing each task, update todo_write to mark it as completed\n")
		b.WriteString("- Only generate the final summary after all tasks are completed\n")
	} else {
		b.WriteString("All tasks completed!\n\n")
		b.WriteString("You can now:\n")
		b.WriteString("- Synthesize findings from all tasks\n")
		b.WriteString("- Generate a complete final answer or report\n")
	}

	return b.String()
}
