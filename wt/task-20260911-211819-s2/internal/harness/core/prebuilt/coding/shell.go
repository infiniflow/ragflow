package coding

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// ShellAllowListConfig configures the shell allow-list middleware.
type ShellAllowListConfig struct {
	// AllowedCommands lists shell command prefixes that are allowed.
	// A command matches if it starts with any entry in this list.
	// Example: "git", "go", "npm" allows "git commit", "go build", "npm install".
	// When empty, all commands are allowed (pass-through mode).
	AllowedCommands []string

	// BlockedCommands lists shell command prefixes that are explicitly blocked,
	// even if they match AllowedCommands. BlockedCommands takes precedence.
	// Example: "git push --force" or "rm -rf".
	BlockedCommands []string

	// DenyMessage is returned to the LLM when a command is blocked.
	DenyMessage string

	// Passthrough when true allows all commands (disables filtering).
	// Default: false.
	Passthrough bool
}

// DefaultShellAllowList returns sensible defaults for coding agents.
func DefaultShellAllowList() []string {
	return []string{
		"git", "go", "make", "npm", "npx", "yarn", "pnpm",
		"cargo", "rustc", "python", "python3", "pip", "pip3",
		"ls", "cat", "head", "tail", "wc", "sort", "uniq",
		"grep", "find", "which", "type", "file", "du", "df",
		"echo", "printf", "env", "pwd", "date",
		"ps", "top", "htop", "kill", "killall",
		"curl", "wget", "ping", "nslookup", "dig",
		"diff", "patch", "cmp", "tar", "gzip", "gunzip", "zip", "unzip",
		"docker", "docker-compose",
		"sed", "awk", "xargs",
		"ssh", "scp", "rsync",
		"goctl", "mockgen", "protoc",
	}
}

// DefaultBlockedCommands returns commands that should always be blocked.
func DefaultBlockedCommands() []string {
	return []string{
		"rm -rf /", "rm -rf ~", "rm -rf .",
		"chmod -R", "chown -R",
		"dd if=", "mkfs", "fdisk",
		"> /dev/", "> /etc/", "> /boot/",
		":(){ :|:& };:", // fork bomb
	}
}

// ShellAllowListMiddleware filters shell commands before execution.
// Place this middleware BEFORE the filesystem middleware in the chain so the
// execute tool's invocation is intercepted and checked against allow/block lists.
type ShellAllowListMiddleware struct {
	core.BaseMiddleware[*schema.Message]
	cfg    *ShellAllowListConfig
	once   sync.Once
	parsed struct {
		allowed []string
		blocked []string
	}
}

// NewShellAllowList creates a ShellAllowListMiddleware.
// When cfg is nil or cfg.Passthrough is true, all commands pass through.
func NewShellAllowList(cfg *ShellAllowListConfig) *ShellAllowListMiddleware {
	if cfg == nil {
		cfg = &ShellAllowListConfig{Passthrough: true}
	}
	if cfg.DenyMessage == "" {
		cfg.DenyMessage = "Error: command blocked by security policy. Use allowed commands only."
	}
	m := &ShellAllowListMiddleware{cfg: cfg}
	m.once.Do(m.init)
	return m
}

func (m *ShellAllowListMiddleware) init() {
	if m.cfg.AllowedCommands == nil {
		m.parsed.allowed = DefaultShellAllowList()
	} else {
		m.parsed.allowed = normalizeCommands(m.cfg.AllowedCommands)
	}
	if m.cfg.BlockedCommands == nil {
		m.parsed.blocked = DefaultBlockedCommands()
	} else {
		m.parsed.blocked = normalizeCommands(m.cfg.BlockedCommands)
	}
}

// WrapToolInvoke intercepts the "execute" tool call (or any tool matching
// the execute command) and checks if the shell command is allowed.
func (m *ShellAllowListMiddleware) WrapToolInvoke(ctx context.Context, ep core.InvokableToolEndpoint, tc *core.ToolContext) (core.InvokableToolEndpoint, error) {
	if tc.Name != "execute" || m.cfg.Passthrough {
		return ep, nil
	}
	m.once.Do(m.init)

	return func(ctx context.Context, args string, opts ...core.ToolOption) (string, error) {
		cmd := strings.TrimSpace(args)
		if cmd == "" {
			return ep(ctx, args, opts...)
		}

		// Check against blocked list first (takes precedence).
		for _, blocked := range m.parsed.blocked {
			if strings.HasPrefix(cmd, blocked) {
				return m.cfg.DenyMessage, nil
			}
		}

		// If no allow list, allow.
		if len(m.parsed.allowed) == 0 {
			return ep(ctx, args, opts...)
		}

		// Check against allow list.
		for _, allowed := range m.parsed.allowed {
			if strings.HasPrefix(cmd, allowed) {
				return ep(ctx, args, opts...)
			}
		}

		return fmt.Sprintf("Error: command %q is not in the allowed list.", strings.Split(cmd, " ")[0]), nil
	}, nil
}

// WrapEnhancedInvokableToolCall intercepts enhanced tool calls as well.
func (m *ShellAllowListMiddleware) WrapEnhancedInvokableToolCall(ctx context.Context, ep core.EnhancedInvokableToolEndpoint, tc *core.ToolContext) (core.EnhancedInvokableToolEndpoint, error) {
	if tc.Name != "execute" || m.cfg.Passthrough {
		return ep, nil
	}
	m.once.Do(m.init)

	return func(ctx context.Context, args *schema.ToolArgument, opts ...core.ToolOption) (*schema.ToolResult, error) {
		cmd := strings.TrimSpace(args.Arguments)
		if cmd == "" {
			return ep(ctx, args, opts...)
		}

		for _, blocked := range m.parsed.blocked {
			if strings.HasPrefix(cmd, blocked) {
				return &schema.ToolResult{
					Name:    args.Name,
					Content: m.cfg.DenyMessage,
				}, nil
			}
		}

		if len(m.parsed.allowed) == 0 {
			return ep(ctx, args, opts...)
		}

		for _, allowed := range m.parsed.allowed {
			if strings.HasPrefix(cmd, allowed) {
				return ep(ctx, args, opts...)
			}
		}

		return &schema.ToolResult{
			Name:    args.Name,
			Content: fmt.Sprintf("Error: command %q is not in the allowed list.", strings.Split(cmd, " ")[0]),
		}, nil
	}, nil
}

// ---- Helpers ----

func normalizeCommands(cmds []string) []string {
	out := make([]string, 0, len(cmds))
	for _, c := range cmds {
		c = strings.TrimSpace(c)
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}
