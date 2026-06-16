// Package coding provides a ready-to-use coding agent built on top of agentcore's
// ReAct agent, middleware stack, and profile system.
//
// It is the agentcore equivalent of deepagents-code — a production-grade coding
// assistant with file operations, shell security, Git safety, and optional sub-agents.
//
// Quick start:
//
//	agent := coding.New(&coding.Config{
//	    Model: myModel,
//	})
//	runner := core.NewTypedRunner(core.RunnerConfig[*schema.Message]{Agent: agent})
//	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("fix this bug")})
//
// With the profile system:
//
//	coding.RegisterHarnessProfile()
//	agent, _ := profile.NewAgent(ctx, &profile.AgentConfig{
//	    ModelSpec: "anthropic:claude-sonnet-4-6",
//	    HarnessProfileName: "coding-agent",
//	})
package coding

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/middlewares/filesystem"
	"ragflow/internal/harness/core/middlewares/subagent"
	"ragflow/internal/harness/core/profile"
	"ragflow/internal/harness/core/schema"
)

// Config configures the coding agent.
type Config struct {
	// Name of the agent. Default: "coding_agent".
	Name string

	// Model is the chat model. Required (unless using profile system).
	Model core.Model[*schema.Message]

	// Tools are additional tools to register beyond the built-in coding tools.
	Tools []core.Tool

	// Instruction overrides the default coding system prompt.
	Instruction string

	// MaxIterations limits the ReAct loop. Default: 30.
	MaxIterations int

	// EnableShell when true adds shell execution capability via a local backend.
	// Default: false (read-only file operations).
	EnableShell bool

	// ShellBackend is the filesystem backend to use for shell execution.
	// When nil and EnableShell is true, a default local shell backend is created.
	ShellBackend filesystem.Backend

	// ShellAllowList configures which shell commands are allowed.
	// When nil, the default allow-list is used (if EnableShell is true).
	// When EnableShell is false, this field is ignored.
	ShellAllowList *ShellAllowListConfig

	// FilesystemBackend is the filesystem backend for file operations.
	// When nil, an InMemoryBackend is used (read-only from agent perspective).
	// Use a LocalFilesystemBackend for real file system access.
	FilesystemBackend filesystem.Backend

	// SubAgentSpecs declares sub-agents for task delegation.
	SubAgentSpecs []subagent.SubAgentSpec

	// SubAgentConfig configures the SubAgentMiddleware (recursion depth, events).
	SubAgentConfig *subagent.Config

	// RegisterHarness when true also registers the "coding-agent" harness profile.
	// Default: false.
	RegisterHarness bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Name:          "coding_agent",
		MaxIterations: 30,
		EnableShell:   false,
	}
}

// New creates a fully-configured coding ReActAgent.
//
// The agent includes:
//   - Coding-optimized system prompt (with Git safety rules)
//   - Filesystem middleware (read/write/edit/ls/glob/grep)
//   - Shell allow-list middleware (when EnableShell is true)
//   - SubAgentMiddleware (when SubAgentSpecs is non-empty)
//   - Optional "coding-agent" harness profile registration
func New(cfg *Config) *core.ReActAgent[*schema.Message] {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 30
	}
	if cfg.Name == "" {
		cfg.Name = "coding_agent"
	}
	instruction := cfg.Instruction
	if instruction == "" {
		instruction = systemPrompt
	}

	// Build middleware stack.
	var middlewares []core.ReActMiddleware

	// 1. Shell allow-list middleware (applied BEFORE filesystem to intercept execute calls).
	if cfg.EnableShell {
		shellCfg := cfg.ShellAllowList
		if shellCfg == nil {
			shellCfg = &ShellAllowListConfig{
				AllowedCommands: DefaultShellAllowList(),
				BlockedCommands: DefaultBlockedCommands(),
			}
		}
		middlewares = append(middlewares, NewShellAllowList(shellCfg))
	}

	// 2. Filesystem middleware (provides read/write/edit/ls/glob/grep/execute).
	fsCfg := &filesystem.Config{
		Backend: cfg.FilesystemBackend,
	}
	if cfg.EnableShell && cfg.ShellBackend != nil {
		fsCfg.Backend = cfg.ShellBackend
	} else if cfg.EnableShell && cfg.FilesystemBackend == nil {
		// Create default local shell backend.
		fsCfg.Backend = &localShellBackend{}
	}
	middlewares = append(middlewares, filesystem.New(fsCfg))

	// 3. SubAgentMiddleware (when sub-agents are declared).
	if len(cfg.SubAgentSpecs) > 0 {
		saCfg := cfg.SubAgentConfig
		if saCfg == nil {
			saCfg = &subagent.Config{MaxDepth: 5}
		}
		saMW := subagent.New(cfg.SubAgentSpecs, saCfg)
		middlewares = append(middlewares, saMW)

		// Build react config with BindToConfig.
		reactCfg := &core.ReActConfig[*schema.Message]{
			Model:         cfg.Model,
			Instruction:   instruction,
			MaxIterations: cfg.MaxIterations,
			Middlewares:   middlewares,
			Tools:         cfg.Tools,
		}
		saMW.BindToConfig(context.Background(), reactCfg)
		return core.NewReActAgent(reactCfg)
	}

	// Build react config without sub-agents.
	reactCfg := &core.ReActConfig[*schema.Message]{
		Model:         cfg.Model,
		Instruction:   instruction,
		MaxIterations: cfg.MaxIterations,
		Middlewares:   middlewares,
		Tools:         cfg.Tools,
	}
	if cfg.EnableShell || cfg.FilesystemBackend != nil {
		// Must have at least one tool for ReAct loop.
		if len(reactCfg.Tools) == 0 {
			// Filesystem middleware adds tools via BeforeAgent, but we need
			// at least one tool to trigger the ReAct loop.
			reactCfg.Tools = append(reactCfg.Tools, &execTool{})
		}
	}
	return core.NewReActAgent(reactCfg)
}

// ---- Local shell backend ----

// localShellBackend implements filesystem.Backend with local shell execution.
type localShellBackend struct{}

func (b *localShellBackend) Read(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (b *localShellBackend) Write(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func (b *localShellBackend) Edit(path, old, new string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	if !strings.Contains(content, old) {
		return fmt.Errorf("edit_file: string %q not found in %s", old, path)
	}
	content = strings.Replace(content, old, new, 1)
	return os.WriteFile(path, []byte(content), 0644)
}

func (b *localShellBackend) Ls(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

func (b *localShellBackend) Glob(pattern string) ([]string, error) {
	// Simple glob via the shell.
	cmd := exec.Command("sh", "-c", fmt.Sprintf("ls -d %s 2>/dev/null", pattern))
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	lines := strings.TrimSpace(string(out))
	if lines == "" {
		return nil, nil
	}
	return strings.Split(lines, "\n"), nil
}

func (b *localShellBackend) Grep(pattern, path string) (string, error) {
	cmd := exec.Command("grep", "-rn", pattern, path)
	out, err := cmd.Output()
	if err != nil {
		// grep returns exit code 1 when no matches.
		return "", nil
	}
	return string(out), nil
}

func (b *localShellBackend) Execute(command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("execute: %w\n%s", err, string(out))
	}
	return string(out), nil
}

// ---- Dummy tool to bootstrap ReAct loop ----

type execTool struct{}

func (t *execTool) Name() string             { return "_bootstrap_tool" }
func (t *execTool) Description() string       { return "Internal bootstrap tool" }
func (t *execTool) Invoke(ctx context.Context, args string, opts ...core.ToolOption) (string, error) {
	return "", nil
}
func (t *execTool) Stream(ctx context.Context, args string, opts ...core.ToolOption) (*schema.StreamReader[string], error) {
	return schema.StreamReaderFromArray([]string{""}), nil
}

// ---- Harness profile registration ----

// HarnessProfile returns a pre-configured HarnessProfile for coding agents.
// It registers the standard coding agent middleware stack.
func HarnessProfile() *profile.HarnessProfile {
	return &profile.HarnessProfile{
		Name:             "coding-agent",
		BaseSystemPrompt: strPtr(systemPrompt),
		MaxIterations:    30,
		RecursionDepth:   5,
	}
}

// RegisterHarnessProfile registers the "coding-agent" harness profile globally.
// After calling this, users can create coding agents via profile.NewAgent:
//
//	agent, _ := profile.NewAgent(ctx, &profile.AgentConfig{
//	    ModelSpec: "anthropic:claude-sonnet-4-6",
//	    HarnessProfileName: "coding-agent",
//	})
func RegisterHarnessProfile() {
	if profile.LookupHarness("coding-agent") != nil {
		return // already registered
	}
	profile.RegisterHarness(HarnessProfile())
}

func strPtr(s string) *string { return &s }
