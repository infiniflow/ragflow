// Package skill provides skill loading and execution middleware.
// Skills are defined in SKILL.md files with YAML frontmatter.
package skill

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// ExecMode defines how a skill is executed.
type ExecMode int

const (
	ModeInline ExecMode = iota // Skill content injected into instruction
	ModeFork                   // Skill loaded via a tool
	ModeForkWithContext        // Skill loaded via a tool with parent context
)

// FileSystemBackend reads skill definitions from a file system.
type FileSystemBackend interface {
	Read(path string) (string, error)
	List() ([]string, error)
}

// Config defines a single skill.
type Config struct {
	Name          string
	Description   string
	Content       string
	ExecutionMode ExecMode
	Model         string // Model name for fork modes
	Agent         string // Agent name for fork modes
}

// TypedConfig configures the skill middleware.
type TypedConfig[M core.MessageType] struct {
	Skills       []Config
	Backend      FileSystemBackend
	CustomSystemPrompt  func(name, desc string) string
	CustomToolParams    func(name string) string
	BuildContent        func(ctx context.Context, cfg Config) (string, error)
	BuildForkMessages   func(ctx context.Context, cfg Config, request string) (string, error)
	FormatForkResult    func(ctx context.Context, result string) (string, error)
}

type middleware[M core.MessageType] struct {
	core.BaseMiddleware[M]
	cfg *TypedConfig[M]
}

func NewTyped[M core.MessageType](cfg *TypedConfig[M]) core.TypedReActMiddleware[M] {
	return &middleware[M]{cfg: cfg}
}

func New(cfg *TypedConfig[*schema.Message]) core.TypedReActMiddleware[*schema.Message] {
	return NewTyped[*schema.Message](cfg)
}

func (m *middleware[M]) BeforeAgent(ctx context.Context, rc *core.ReActAgentContext) (context.Context, *core.ReActAgentContext, error) {
	if m.cfg == nil { return ctx, rc, nil }
	skills := m.cfg.Skills
	if len(skills) == 0 && m.cfg.Backend != nil {
		names, err := m.cfg.Backend.List()
		if err == nil {
			for _, name := range names {
				content, err := m.cfg.Backend.Read(name)
				if err != nil { continue }
				parsed := parseSkill(content)
				if parsed != nil {
					skills = append(skills, *parsed)
				}
			}
		}
	}

	for _, s := range skills {
		switch s.ExecutionMode {
		case ModeInline:
			rc.Instruction = applyCustomInstruction(rc.Instruction, s, m.cfg.CustomSystemPrompt)
		case ModeFork, ModeForkWithContext:
			rc.Tools = append(rc.Tools, m.newSkillTool(s))
		}
	}
	return ctx, rc, nil
}

func (m *middleware[M]) newSkillTool(s Config) core.Tool {
	return core.NewBaseTool("skill_"+s.Name,
		fmt.Sprintf("Execute the '%s' skill. %s", s.Name, s.Description),
		func(ctx context.Context, args string) (string, error) {
			if m.cfg.BuildContent != nil {
				content, err := m.cfg.BuildContent(ctx, s)
				if err != nil { return "", err }
				if m.cfg.FormatForkResult != nil {
					return m.cfg.FormatForkResult(ctx, content)
				}
				return content, nil
			}
			content := s.Content
			if content == "" && m.cfg.Backend != nil {
				loaded, err := m.cfg.Backend.Read(s.Name)
				if err == nil { content = loaded }
			}
			if m.cfg.BuildForkMessages != nil {
				result, err := m.cfg.BuildForkMessages(ctx, s, args)
				if err != nil { return "", err }
				return result, nil
			}
			if m.cfg.FormatForkResult != nil {
				return m.cfg.FormatForkResult(ctx, content)
			}
			return fmt.Sprintf("### Skill: %s\n\n%s\n\nArgs: %s", s.Name, truncate(content, 2000), args), nil
		})
}

// ---- Helpers ----

func parseSkill(content string) *Config {
	cfg := &Config{ExecutionMode: ModeInline}
	content = strings.TrimSpace(content)

	// Parse YAML-like frontmatter
	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content[3:], "---", 2)
		if len(parts) == 2 {
			front := strings.TrimSpace(parts[0])
			body := strings.TrimSpace(parts[1])
			for _, line := range strings.Split(front, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name:") {
					cfg.Name = strings.TrimSpace(line[5:])
				} else if strings.HasPrefix(line, "description:") {
					cfg.Description = strings.TrimSpace(line[12:])
				} else if strings.HasPrefix(line, "model:") {
					cfg.Model = strings.TrimSpace(line[6:])
				}
			}
			cfg.Content = body
			return cfg
		}
	}
	// No frontmatter: use full content
	cfg.Content = content
	return cfg
}

func applyCustomInstruction(instruction string, s Config, customFn func(name, desc string) string) string {
	if customFn != nil {
		return instruction + "\n\n" + customFn(s.Name, s.Description)
	}
	return instruction + "\n\n## Skill: " + s.Name + "\n" + truncate(s.Content, 4000)
}

func truncate(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "\n...(truncated)"
}
