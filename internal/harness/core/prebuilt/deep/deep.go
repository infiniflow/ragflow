// Package deep provides DeepAgent — a depth-first task decomposition and execution agent.
// It combines a ReAct loop with built-in task management, filesystem access,
// and optional shell execution for a production-grade coding/operations agent.
package deep

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// SubAgentSpec defines a sub-agent available for task delegation.
type SubAgentSpec struct {
	Name        string
	Description string
	Agent       core.Agent
}

// Config holds configuration for the Deep Agent.
type Config struct {
	Name          string
	Description   string
	Model         core.Model[*schema.Message]
	Tools         []core.Tool
	MaxIterations int
	Instruction   string                      // Custom system prompt (overrides default)
	EnableShell   bool                        // Enable shell command execution tool
	SubAgents     []SubAgentSpec              // NEW: Sub-agents for task delegation
	FailoverModel core.Model[*schema.Message] // NEW: Failover model
	OutputKey     string                      // NEW: Session output storage key
}

func DefaultConfig() *Config {
	return &Config{
		Name:          "deep_agent",
		Description:   "A depth-first task decomposition and execution agent",
		MaxIterations: 20,
		EnableShell:   false,
	}
}

// NewTyped creates a new DeepAgent as a TypedReActAgent.
func NewTyped(cfg *Config) *core.ReActAgent[*schema.Message] {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 20
	}
	if cfg.Name == "" {
		cfg.Name = "deep_agent"
	}

	instruction := cfg.Instruction
	if instruction == "" {
		instruction = systemPrompt
	}

	// Append OutputKey hint to system prompt if set
	if cfg.OutputKey != "" {
		instruction += "\n\nStore the final answer in the session under key {" + cfg.OutputKey + "}."
	}

	// Build tool set: user tools + task management
	tools := make([]core.Tool, 0, len(cfg.Tools)+8)
	tools = append(tools, cfg.Tools...)

	// Task management (write_todos)
	taskMgr := NewTaskManager()
	tools = append(tools,
		TaskCreateTool(taskMgr),
		TaskListTool(taskMgr),
		TaskUpdateTool(taskMgr),
	)

	// Optional shell tool
	if cfg.EnableShell {
		tools = append(tools, ShellTool("."))
	}

	chatCfg := &core.ReActConfig[*schema.Message]{
		Model:         cfg.Model,
		Tools:         tools,
		Instruction:   instruction,
		MaxIterations: cfg.MaxIterations,
		OutputKey:     cfg.OutputKey,
	}

	// Set up failover if configured
	if cfg.FailoverModel != nil {
		chatCfg.FailoverConfig = &core.FailoverConfig[*schema.Message]{
			Models: []core.Model[*schema.Message]{cfg.FailoverModel},
		}
	}

	a := core.NewReActAgent(chatCfg)
	return a.WithName(cfg.Name).WithDescription(cfg.Description)
}

// NewWithSubAgents creates a DeepAgent with sub-agent delegation support.
// The deep agent can transfer tasks to sub-agents and receive results.
// If no sub-agents are configured, returns the plain deep agent.
func NewWithSubAgents(ctx context.Context, cfg *Config) (core.ResumableAgent, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	deep := NewTyped(cfg)
	if cfg == nil || len(cfg.SubAgents) == 0 {
		return deep, nil
	}
	subs := make([]core.Agent, 0, len(cfg.SubAgents))
	for _, sa := range cfg.SubAgents {
		subs = append(subs, sa.Agent)
	}
	return core.SetSubAgents(ctx, deep, subs)
}

// New creates a DeepAgent as a generic agent.
func New(cfg *Config) core.Agent { return NewTyped(cfg) }

// Prompt returns the default system prompt.
func Prompt() string { return systemPrompt }

// ---- Shell Tool ----

// ShellTool creates a tool that executes shell commands.
// WARNING: Enable only in trusted environments. This provides arbitrary code execution.
func ShellTool(workDir string) core.Tool {
	return core.NewBaseTool(
		"shell",
		"Execute a shell command and return its output. Args: {\"command\":\"ls -la\"}",
		func(ctx context.Context, args string) (string, error) {
			var in struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", err
			}

			cmd := exec.CommandContext(ctx, "sh", "-c", in.Command)
			if workDir != "" {
				cmd.Dir = workDir
			}
			output, err := cmd.CombinedOutput()
			result := shellResult{
				Command:  in.Command,
				Output:   string(output),
				ExitCode: 0,
			}
			if exitErr, ok := err.(*exec.ExitError); ok {
				result.ExitCode = exitErr.ExitCode()
			} else if err != nil {
				return "", fmt.Errorf("shell exec: %w", err)
			}
			b, _ := json.Marshal(result)
			return string(b), nil
		},
	)
}

type shellResult struct {
	Command  string `json:"command"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

// StreamingShellTool creates a streaming version of shell execution.
func StreamingShellTool(workDir string) core.Tool {
	return core.NewBaseTool(
		"streaming_shell",
		"Execute a shell command with streaming output. Args: {\"command\":\"tail -f log.txt\"}",
		func(ctx context.Context, args string) (string, error) {
			var in struct {
				Command string `json:"command"`
			}
			json.Unmarshal([]byte(args), &in) // ignore error

			cmd := exec.CommandContext(ctx, "sh", "-c", in.Command)
			if workDir != "" {
				cmd.Dir = workDir
			}
			output, err := cmd.CombinedOutput()
			if err != nil {
				exitCode := -1
				if ee, ok := err.(*exec.ExitError); ok {
					exitCode = ee.ExitCode()
				}
				return fmt.Sprintf(`{"exit_code":%d,"error":"%s","output":"%s"}`, exitCode, err, escapeShell(string(output))), nil
			}
			return fmt.Sprintf(`{"exit_code":0,"output":%q}`, escapeShell(string(output))), nil
		},
	)
}

func escapeShell(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// ---- Task Manager (embedded in Deep Agent) ----

// TaskState represents the lifecycle state of a sub-task.
type TaskState string

const (
	TaskPending   TaskState = "pending"
	TaskRunning   TaskState = "running"
	TaskCompleted TaskState = "completed"
	TaskFailed    TaskState = "failed"
)

// Task is a unit of work tracked by the Deep Agent's task manager.
type Task struct {
	ID           string    `json:"id"`
	Description  string    `json:"description"`
	State        TaskState `json:"state"`
	Result       string    `json:"result,omitempty"`
	Error        string    `json:"error,omitempty"`
	Dependencies []string  `json:"dependencies,omitempty"`
}

// TaskManager tracks sub-tasks within a Deep Agent session.
type TaskManager struct{ tasks []*Task }

func NewTaskManager() *TaskManager { return &TaskManager{} }

func (m *TaskManager) Create(desc string, deps ...string) *Task {
	t := &Task{
		ID:          fmt.Sprintf("task_%d", len(m.tasks)+1),
		Description: desc, State: TaskPending,
		Dependencies: deps,
	}
	m.tasks = append(m.tasks, t)
	return t
}

func (m *TaskManager) List() []*Task { return m.tasks }
func (m *TaskManager) Get(id string) (*Task, error) {
	for _, t := range m.tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, fmt.Errorf("task %q not found", id)
}

func (m *TaskManager) Update(id, result string, state TaskState) error {
	t, err := m.Get(id)
	if err != nil {
		return err
	}
	t.Result = result
	t.State = state
	return nil
}

// TaskCreateTool returns an core.Tool for creating sub-tasks.
func TaskCreateTool(m *TaskManager) core.Tool {
	return core.NewBaseTool(
		"write_todos",
		"Create a todo/sub-task. Args: {\"todos\":[{\"desc\":\"...\",\"deps\":[]}]}",
		func(ctx context.Context, args string) (string, error) {
			var in struct {
				Todos []struct {
					Desc    string   `json:"desc"`
					Depends []string `json:"deps,omitempty"`
				} `json:"todos"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", err
			}
			var created []*Task
			for _, td := range in.Todos {
				t := m.Create(td.Desc, td.Depends...)
				created = append(created, t)
			}
			b, _ := json.Marshal(created)
			return string(b), nil
		},
	)
}

// TaskListTool returns an core.Tool for listing all sub-tasks.
func TaskListTool(m *TaskManager) core.Tool {
	return core.NewBaseTool(
		"list_todos",
		"List all sub-tasks and their status.",
		func(ctx context.Context, args string) (string, error) {
			b, _ := json.Marshal(m.List())
			return string(b), nil
		},
	)
}

// TaskUpdateTool returns an core.Tool for updating a sub-task's status.
func TaskUpdateTool(m *TaskManager) core.Tool {
	return core.NewBaseTool(
		"update_todo",
		"Update a sub-task status. Args: {\"id\":\"task_1\",\"result\":\"done!\",\"status\":\"completed\"}",
		func(ctx context.Context, args string) (string, error) {
			var in struct {
				ID     string `json:"id"`
				Result string `json:"result,omitempty"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", err
			}
			if err := m.Update(in.ID, in.Result, TaskState(in.Status)); err != nil {
				return "", err
			}
			b, _ := json.Marshal(map[string]string{"updated": in.ID})
			return string(b), nil
		},
	)
}

// ---- i18n Prompts ----

const systemPrompt = `You are a Deep Agent — a depth-first task decomposition and execution agent.

Your role:
1. Break down complex tasks into specific, actionable sub-steps
2. Execute each step, verifying results before proceeding
3. Track sub-task completion using the write_todos / update_todo tools
4. Read files before editing them; test changes when appropriate
5. Report final results clearly when all tasks are complete

Guidelines:
- Verify actions before executing
- Read files before editing  
- Test changes when appropriate
- Track sub-tasks and their completion status
- Each sub-task should be specific, actionable, ordered logically
- After completing each sub-task, verify the output is correct`

var prompts = map[string]struct{ System, TaskPrompt, VerifyPrompt, TransferDesc string }{
	"en": {systemPrompt, "Each sub-task should be specific, actionable, ordered logically.", "After completing each sub-task, verify the output is correct.", "Transfer the question to another agent."},
	"zh": {`你是一个深度代理 —— 一个深度优先的任务分解和执行代理。

你的角色：
1. 将复杂任务分解为具体的、可执行的子步骤
2. 执行每个步骤，在继续之前验证结果
3. 使用 write_todos / update_todo 工具跟踪子任务完成情况
4. 在编辑文件前先阅读文件；适当时候测试变更
5. 所有任务完成后清晰报告最终结果

准则：
- 执行前验证操作
- 编辑前先阅读文件
- 适当时测试变更
- 跟踪子任务及其完成状态
- 每个子任务应具体、可操作、逻辑有序
- 完成每个子任务后，验证输出是否正确`, "每个子任务应该是具体的、可操作的、逻辑有序的。", "完成每个子任务后，验证输出是否正确。", "将问题移交给其他代理。"},
}

func SelectPrompt(lang string) string {
	if p, ok := prompts[lang]; ok {
		return p.System
	}
	return systemPrompt
}
