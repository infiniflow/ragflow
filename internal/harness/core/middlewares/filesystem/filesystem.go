// Package filesystem provides a middleware that registers file system tools
// (read, write, edit, ls, glob, grep, execute) for agent use.
package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// Backend abstracts file system operations.
type Backend interface {
	Read(path string) (string, error)
	Write(path, content string) error
	Edit(path, old, new string) error
	Ls(path string) ([]string, error)
	Glob(pattern string) ([]string, error)
	Grep(pattern, path string) (string, error)
	Execute(command string) (string, error)
}

// ToolConfig configures a single tool.
type ToolConfig struct {
	Name        string
	Description string
	Disabled    bool
	Custom      func(ctx context.Context, args string) (string, error)
}

// TypedConfig configures the filesystem middleware.
type TypedConfig[M core.MessageType] struct {
	Backend    Backend
	ToolConfig map[string]*ToolConfig // Override individual tools
	ReadBytes  int                    // Max bytes per read (default: 1MB)
}

type Config = TypedConfig[*schema.Message]

type middleware[M core.MessageType] struct {
	core.BaseMiddleware[M]
	cfg *Config
}

func NewTyped[M core.MessageType](cfg *Config) *middleware[M] {
	if cfg == nil {
		cfg = &Config{ReadBytes: 1 << 20}
	}
	if cfg.ReadBytes <= 0 {
		cfg.ReadBytes = 1 << 20
	}
	return &middleware[M]{cfg: cfg}
}

func New(cfg *Config) *middleware[*schema.Message] { return NewTyped[*schema.Message](cfg) }

func (m *middleware[M]) ContributeTools(ctx context.Context) []core.Tool {
	if m.cfg.Backend == nil {
		return nil
	}
	return m.buildTools()
}

func (m *middleware[M]) ContributeToolInfos(ctx context.Context) []*schema.ToolInfo { return nil }

func (m *middleware[M]) ContributeReturnDirectly(ctx context.Context) map[string]bool { return nil }

// BeforeAgent is retained for backward compatibility with code that calls
// BeforeAgent directly (e.g., existing tests). The preferred path is
// ToolContributor (ContributeTools), which is automatically collected
// during agent build.
func (m *middleware[M]) BeforeAgent(ctx context.Context, rc *core.ReActAgentContext) (context.Context, *core.ReActAgentContext, error) {
	return ctx, rc, nil
}

func (m *middleware[M]) buildTools() []core.Tool {
	tools := make([]core.Tool, 0, 7)
	if tool := m.maybeTool("read_file", "Read file contents. Accepts file path.", m.newReadTool()); tool != nil {
		tools = append(tools, tool)
	}
	if tool := m.maybeTool("write_file", "Write content to a file. Args: path|content.", m.newWriteTool()); tool != nil {
		tools = append(tools, tool)
	}
	if tool := m.maybeTool("edit_file", "Edit file by replacing text. Args: path|old|new.", m.newEditTool()); tool != nil {
		tools = append(tools, tool)
	}
	if tool := m.maybeTool("ls", "List directory contents. Args: path.", m.newLsTool()); tool != nil {
		tools = append(tools, tool)
	}
	if tool := m.maybeTool("glob", "Find files matching a glob pattern. Args: pattern.", m.newGlobTool()); tool != nil {
		tools = append(tools, tool)
	}
	if tool := m.maybeTool("grep", "Search for text in files. Args: pattern|path|output_mode.", m.newGrepTool()); tool != nil {
		tools = append(tools, tool)
	}
	if tool := m.maybeTool("execute", "Execute a shell command. Args: command.", m.newExecTool()); tool != nil {
		tools = append(tools, tool)
	}
	return tools
}

func (m *middleware[M]) maybeTool(name, defaultDesc string, defaultFn func(ctx context.Context, args string) (string, error)) core.Tool {
	if m.cfg.ToolConfig != nil {
		if tc, ok := m.cfg.ToolConfig[name]; ok {
			if tc.Disabled {
				return nil
			}
			desc := defaultDesc
			if tc.Description != "" {
				desc = tc.Description
			}
			if tc.Custom != nil {
				return core.NewBaseTool(tc.Name, desc, tc.Custom)
			}
			toolName := name
			if tc.Name != "" {
				toolName = tc.Name
			}
			return core.NewBaseTool(toolName, desc, defaultFn)
		}
	}
	return core.NewBaseTool(name, defaultDesc, defaultFn)
}

func (m *middleware[M]) newReadTool() func(ctx context.Context, args string) (string, error) {
	return func(ctx context.Context, args string) (string, error) {
		content, err := m.cfg.Backend.Read(args)
		if err != nil {
			return "", err
		}
		if len(content) > m.cfg.ReadBytes {
			content = content[:m.cfg.ReadBytes] + "\n...(truncated)"
		}
		// Add line numbers
		lines := strings.Split(content, "\n")
		for i, l := range lines {
			lines[i] = fmt.Sprintf("%6d: %s", i+1, l)
		}
		return strings.Join(lines, "\n"), nil
	}
}

func (m *middleware[M]) newWriteTool() func(ctx context.Context, args string) (string, error) {
	return func(ctx context.Context, args string) (string, error) {
		var jsonArgs struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(args), &jsonArgs); err == nil && jsonArgs.Path != "" {
			if err := m.cfg.Backend.Write(jsonArgs.Path, jsonArgs.Content); err != nil {
				return "", err
			}
			return fmt.Sprintf("OK: wrote %d bytes to %s", len(jsonArgs.Content), jsonArgs.Path), nil
		}
		parts := strings.SplitN(args, "|", 2)
		if len(parts) < 2 {
			return "", fmt.Errorf("expected path|content or JSON with 'path' and 'content'")
		}
		if err := m.cfg.Backend.Write(parts[0], parts[1]); err != nil {
			return "", err
		}
		return fmt.Sprintf("OK: wrote %d bytes to %s", len(parts[1]), parts[0]), nil
	}
}

func (m *middleware[M]) newEditTool() func(ctx context.Context, args string) (string, error) {
	return func(ctx context.Context, args string) (string, error) {
		var jsonArgs struct {
			Path string `json:"path"`
			Old  string `json:"old"`
			New  string `json:"new"`
		}
		if err := json.Unmarshal([]byte(args), &jsonArgs); err == nil && jsonArgs.Path != "" && jsonArgs.Old != "" {
			return "", m.cfg.Backend.Edit(jsonArgs.Path, jsonArgs.Old, jsonArgs.New)
		}
		parts := strings.SplitN(args, "|", 3)
		if len(parts) < 3 {
			return "", fmt.Errorf("expected path|old|new or JSON with 'path', 'old', 'new'")
		}
		return "", m.cfg.Backend.Edit(parts[0], parts[1], parts[2])
	}
}

func (m *middleware[M]) newLsTool() func(ctx context.Context, args string) (string, error) {
	return func(ctx context.Context, args string) (string, error) {
		results, err := m.cfg.Backend.Ls(args)
		if err != nil {
			return "", err
		}
		if len(results) == 0 {
			return "(empty directory)", nil
		}
		return strings.Join(results, "\n"), nil
	}
}

func (m *middleware[M]) newGlobTool() func(ctx context.Context, args string) (string, error) {
	return func(ctx context.Context, args string) (string, error) {
		results, err := m.cfg.Backend.Glob(args)
		if err != nil {
			return "", err
		}
		if len(results) == 0 {
			return "No matches", nil
		}
		return strings.Join(results, "\n"), nil
	}
}

func (m *middleware[M]) newGrepTool() func(ctx context.Context, args string) (string, error) {
	return func(ctx context.Context, args string) (string, error) {
		var jsonArgs struct {
			Pattern    string `json:"pattern"`
			Path       string `json:"path"`
			OutputMode string `json:"output_mode"`
		}
		if err := json.Unmarshal([]byte(args), &jsonArgs); err == nil && jsonArgs.Pattern != "" {
			result, err := m.cfg.Backend.Grep(jsonArgs.Pattern, jsonArgs.Path)
			if err != nil {
				return "", err
			}
			return formatGrepResult(result, jsonArgs.OutputMode)
		}
		// Fall back to | separator
		parts := strings.SplitN(args, "|", 3)
		pattern, path := parts[0], "."
		if len(parts) > 1 {
			path = parts[1]
		}
		outputMode := "content"
		if len(parts) > 2 {
			outputMode = parts[2]
		}

		result, err := m.cfg.Backend.Grep(pattern, path)
		if err != nil {
			return "", err
		}
		return formatGrepResult(result, outputMode)
	}
}

func formatGrepResult(result, outputMode string) (string, error) {
	switch outputMode {
	case "count":
		if result == "" {
			return "0 matches", nil
		}
		lines := strings.Count(result, "\n") + 1
		return fmt.Sprintf("%d matches", lines), nil
	case "files":
		unique := make(map[string]bool)
		for _, line := range strings.Split(result, "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) > 0 {
				unique[parts[0]] = true
			}
		}
		names := make([]string, 0, len(unique))
		for n := range unique {
			names = append(names, n)
		}
		return strings.Join(names, "\n"), nil
	default:
		return result, nil
	}
}

func (m *middleware[M]) newExecTool() func(ctx context.Context, args string) (string, error) {
	return func(ctx context.Context, args string) (string, error) {
		return m.cfg.Backend.Execute(args)
	}
}
