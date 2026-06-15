// Package agentsmd injects Agents.md content into model input.
// Supports @import syntax for multi-file references with cycle detection.
package agentsmd

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"ragflow/internal/harness/agentcore"
	"ragflow/internal/harness/agentcore/schema"
)

var importRegex = regexp.MustCompile(`^@([a-zA-Z0-9_.~/][a-zA-Z0-9_.~/\-]*)`)

var allowedImportExts = map[string]bool{
	".md": true, ".txt": true, ".mdx": true,
	".yaml": true, ".yml": true, ".json": true, ".toml": true,
}

// FileSystemBackend reads markdown files.
type FileSystemBackend interface {
	Read(path string) (string, error)
	Exists(path string) bool
}

// TypedConfig configures the agentsmd middleware.
type TypedConfig[M agentcore.MessageType] struct {
	Backend    FileSystemBackend
	Files      []string
	MaxBytes   int // Maximum total bytes to load (default: 50000)
	MaxDepth   int // Maximum @import recursion depth (default: 5)
	OnLoadWarning func(path string, err error)
}

type middleware[M agentcore.MessageType] struct {
	agentcore.BaseMiddleware[M]
	cfg *TypedConfig[M]
	mu  sync.Mutex
	cache map[string]string
}

func NewTyped[M agentcore.MessageType](cfg *TypedConfig[M]) agentcore.TypedReActMiddleware[M] {
	if cfg == nil { cfg = &TypedConfig[M]{MaxBytes: 50000, MaxDepth: 5} }
	if cfg.MaxBytes <= 0 { cfg.MaxBytes = 50000 }
	if cfg.MaxDepth <= 0 { cfg.MaxDepth = 5 }
	return &middleware[M]{cfg: cfg, cache: make(map[string]string)}
}

func New(cfg *TypedConfig[*schema.Message]) agentcore.TypedReActMiddleware[*schema.Message] {
	return NewTyped[*schema.Message](cfg)
}

func (m *middleware[M]) BeforeAgent(ctx context.Context, rc *agentcore.ReActAgentContext) (context.Context, *agentcore.ReActAgentContext, error) {
	// Check if already injected via session (idempotent)
	sess := getSession(ctx)
	if sess != nil {
		if v, ok := sess.Values["_agentsmd_injected"]; ok && v.(bool) {
			return ctx, rc, nil
		}
	}

	if m.cfg.Backend == nil { return ctx, rc, nil }

	var content strings.Builder
	totalBytes := 0

	for _, file := range m.cfg.Files {
		loaded, err := m.loadFile(file, make(map[string]bool), &totalBytes)
		if err != nil {
			if m.cfg.OnLoadWarning != nil {
				m.cfg.OnLoadWarning(file, err)
			}
			continue
		}
		if content.Len() > 0 { content.WriteString("\n\n") }
		content.WriteString(loaded)
	}

	if content.Len() == 0 { return ctx, rc, nil }

	// Inject before first user message (wrap in system-reminder tags)
	injection := fmt.Sprintf("<system-reminder>\nAgent Context:\n%s\n</system-reminder>", content.String())
	rc.Instruction = rc.Instruction + "\n\n" + injection

	// Mark as injected in session
	if sess != nil {
		sess.Values["_agentsmd_injected"] = true
		_ = agentcore.SetRunLocalValue(ctx, "_agentsmd_injected", true)
	}

	return ctx, rc, nil
}

// getSession extracts the run session from context.
func getSession(ctx context.Context) *runCtx {
	rc := &runCtx{Values: make(map[string]any)}
	// Try to use agentcore run-local values for session tracking
	v, ok, _ := agentcore.GetRunLocalValue(ctx, "_agentsmd_injected")
	if ok {
		rc.Values["_agentsmd_injected"] = v
	}
	return rc
}

// runCtx is a minimal session wrapper for idempotency tracking.
type runCtx struct {
	Values map[string]any
}

func (m *middleware[M]) loadFile(path string, visited map[string]bool, totalBytes *int) (string, error) {
	// Cache check
	m.mu.Lock()
	if cached, ok := m.cache[path]; ok {
		m.mu.Unlock()
		return cached, nil
	}
	m.mu.Unlock()

	// Cycle detection
	if visited[path] { return "", nil }
	visited[path] = true

	// Read
	content, err := m.cfg.Backend.Read(path)
	if err != nil { return "", fmt.Errorf("agentsmd: read %s: %w", path, err) }

	result, err := m.processContent(content, 0, visited, totalBytes)
	if err != nil { return "", err }

	// Cache
	m.mu.Lock()
	m.cache[path] = result
	m.mu.Unlock()

	return result, nil
}

func (m *middleware[M]) processContent(content string, depth int, visited map[string]bool, totalBytes *int) (string, error) {
	if depth > m.cfg.MaxDepth { return "", nil }

	var result strings.Builder
	remaining := m.cfg.MaxBytes - *totalBytes

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Handle @import with regex matching and extension whitelist
		if matches := importRegex.FindStringSubmatch(trimmed); len(matches) > 1 && m.cfg.Backend != nil {
			importPath := matches[1]
			ext := ""
			for i := len(importPath) - 1; i >= 0; i-- {
				if importPath[i] == '.' { ext = importPath[i:]; break }
			}
			if ext != "" && !allowedImportExts[ext] { continue }
			if m.cfg.Backend.Exists(importPath) {
				imported, err := m.loadFile(importPath, visited, totalBytes)
				if err != nil {
					if m.cfg.OnLoadWarning != nil {
						m.cfg.OnLoadWarning(importPath, err)
					}
					continue
				}
				if result.Len() > 0 { result.WriteString("\n") }
				result.WriteString(fmt.Sprintf("<!-- imported: %s -->\n%s\n<!-- end import -->", importPath, imported))
				continue
			}
		}

		// Normal line
		lineLen := len(line)
		if lineLen > remaining { break }
		if result.Len() > 0 { result.WriteString("\n") }
		result.WriteString(line)
		*totalBytes += lineLen
		remaining -= lineLen
	}

	return result.String(), nil
}
