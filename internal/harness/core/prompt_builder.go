package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// PromptBuilder builds a system prompt with dynamic context, instruction files,
// and character budgets. It mirrors claw-code's SystemPromptBuilder.
type PromptBuilder struct {
	buf strings.Builder
}

// NewPromptBuilder creates a new PromptBuilder.
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// PromptBudget defines character limits for prompt sections.
type PromptBudget struct {
	PerFile     int // Max chars per instruction file (default: 4000)
	TotalFiles  int // Max total chars from all instruction files (default: 12000)
	GitDiff     int // Max chars for git diff (default: 50000)
	MaxSections int // Max number of sections (default: 20)
}

func (b *PromptBudget) defaults() {
	if b.PerFile <= 0 {
		b.PerFile = 4000
	}
	if b.TotalFiles <= 0 {
		b.TotalFiles = 12000
	}
	if b.GitDiff <= 0 {
		b.GitDiff = 50000
	}
	if b.MaxSections <= 0 {
		b.MaxSections = 20
	}
}

// Build constructs the final prompt string.
func (pb *PromptBuilder) Build(parts []PromptSection, budget *PromptBudget) string {
	budget.defaults()
	pb.buf.Reset()

	for i, p := range parts {
		if i >= budget.MaxSections {
			break
		}
		text := p.Content
		if p.TruncateTo > 0 && len(text) > p.TruncateTo {
			text = text[:p.TruncateTo] + "\n...[truncated]"
		}
		if p.PrependNewline && pb.buf.Len() > 0 {
			pb.buf.WriteString("\n")
		}
		pb.buf.WriteString(text)
		if p.AppendNewline {
			pb.buf.WriteString("\n")
		}
	}

	return pb.buf.String()
}

// PromptSection is a named section of the prompt.
type PromptSection struct {
	Name           string
	Content        string
	TruncateTo     int // Max chars for this section (0 = no limit)
	AppendNewline  bool
	PrependNewline bool
}

// ---- Instruction file discovery ----

// InstructionFile represents a discovered instruction file.
type InstructionFile struct {
	Path    string
	Content string
}

// DiscoverInstructionFiles finds instruction files by walking up from startDir
// to the git root. Searches for: CLAUDE.md, CLAW.md, AGENTS.md, and .claw/rules/*.md
func DiscoverInstructionFiles(startDir string) ([]InstructionFile, error) {
	root := findGitRoot(startDir)
	if root == "" {
		root = startDir
	}

	var files []InstructionFile
	candidates := []string{
		"CLAUDE.md", "CLAW.md", "AGENTS.md",
		".claw/CLAUDE.md", ".claw/instructions.md",
	}

	for _, name := range candidates {
		path := filepath.Join(root, name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		files = append(files, InstructionFile{Path: path, Content: string(content)})
	}

	// Load rules directory
	rulesDir := filepath.Join(root, ".claw", "rules")
	if entries, err := os.ReadDir(rulesDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && (strings.HasSuffix(e.Name(), ".md") || strings.HasSuffix(e.Name(), ".txt")) {
				path := filepath.Join(rulesDir, e.Name())
				content, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				files = append(files, InstructionFile{Path: path, Content: string(content)})
			}
		}
	}

	return files, nil
}

// InstructionFileSections converts instruction files to prompt sections with budget.
func InstructionFileSections(files []InstructionFile, budget *PromptBudget) []PromptSection {
	budget.defaults()
	var sections []PromptSection
	remaining := budget.TotalFiles

	for _, f := range files {
		if remaining <= 0 {
			break
		}
		content := f.Content
		if len(content) > budget.PerFile {
			content = content[:budget.PerFile] + "\n...[truncated]"
		}
		if len(content) > remaining {
			content = content[:remaining] + "\n...[truncated]"
		}
		remaining -= len(content)

		name := filepath.Base(f.Path)
		sections = append(sections, PromptSection{
			Name:           "Instruction: " + name,
			Content:        content,
			AppendNewline:  true,
			PrependNewline: true,
		})
	}
	return sections
}

// ---- Dynamic context sections ----

// EnvSection creates a prompt section with OS, date, and CWD information.
func EnvSection() PromptSection {
	hostname, _ := os.Hostname()
	cwd, _ := os.Getwd()
	now := time.Now().Format(time.RFC3339)

	return PromptSection{
		Name: "Environment",
		Content: fmt.Sprintf("Date: %s\nOS: %s/%s\nHost: %s\nCWD: %s",
			now, runtime.GOOS, runtime.GOARCH, hostname, cwd),
		AppendNewline:  true,
		PrependNewline: true,
	}
}

// GitDiffSection creates a prompt section with git diff output.
func GitDiffSection(budgetChars int) PromptSection {
	diff, _ := execGitDiff()
	if diff == "" {
		return PromptSection{Name: "GitDiff"}
	}
	if budgetChars > 0 && len(diff) > budgetChars {
		diff = diff[:budgetChars] + "\n...[diff truncated]"
	}
	return PromptSection{
		Name:          "Git Diff",
		Content:       fmt.Sprintf("Working tree changes (git diff):\n%s", diff),
		AppendNewline: true,
	}
}

func execGitDiff() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root := findGitRoot(cwd)
	if root == "" {
		return "", fmt.Errorf("not a git repository")
	}
	data, err := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
	if err != nil {
		return "", err
	}
	ref := strings.TrimSpace(string(data))
	return fmt.Sprintf("HEAD: %s", ref), nil
}

// findGitRoot walks up from dir to find the .git directory.
func findGitRoot(dir string) string {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// GroupSectionsWithBudget appends sections respecting a total character budget.
func GroupSectionsWithBudget(sections []PromptSection, budget int) []PromptSection {
	var result []PromptSection
	remaining := budget
	for _, s := range sections {
		if s.Content == "" {
			continue
		}
		if remaining <= 0 {
			break
		}
		if len(s.Content) > remaining {
			s.Content = s.Content[:remaining] + "\n...[truncated]"
			s.TruncateTo = len(s.Content)
		}
		remaining -= len(s.Content)
		result = append(result, s)
	}
	return result
}

// DeduplicateSections removes sections with duplicate content (by exact match).
func DeduplicateSections(sections []PromptSection) []PromptSection {
	seen := make(map[string]bool)
	var result []PromptSection
	for _, s := range sections {
		key := strings.TrimSpace(s.Content)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, s)
	}
	return result
}

// SortSections puts core sections first, then by name.
func SortSections(sections []PromptSection) {
	sort.SliceStable(sections, func(i, j int) bool {
		core := map[string]int{
			"Environment": 0, "Capabilities": 1, "Instructions": 2,
		}
		pi := core[sections[i].Name]
		pj := core[sections[j].Name]
		if pi != pj {
			return pi < pj
		}
		return sections[i].Name < sections[j].Name
	})
}
