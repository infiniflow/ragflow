package agentic_rag

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Override contains only the per-chat fields that replace template values.
type Override struct {
	Content *string   `json:"content,omitempty"`
	Tools   *[]string `json:"tools,omitempty"`
}

// ParseOverride validates the decoded prompt_config.agentic_rag value.
// A nil value resets inheritance; omitted fields continue to inherit.
func ParseOverride(raw any) (*Override, error) {
	if raw == nil {
		return nil, nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("agentic_rag must be an object or null")
	}
	result := &Override{}
	for key, value := range object {
		switch key {
		case "content":
			content, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("agentic_rag.content must be a string")
			}
			result.Content = &content
		case "tools":
			var names []string
			switch values := value.(type) {
			case []string:
				names = slices.Clone(values)
			case []any:
				names = make([]string, len(values))
				for i, item := range values {
					name, ok := item.(string)
					if !ok {
						return nil, fmt.Errorf("agentic_rag.tools[%d] must be a string", i)
					}
					names[i] = name
				}
			default:
				return nil, fmt.Errorf("agentic_rag.tools must be an array of strings")
			}
			result.Tools = &names
		default:
			return nil, fmt.Errorf("agentic_rag has unknown field %q", key)
		}
	}
	if err := validateOverride(result); err != nil {
		return nil, err
	}
	return result, nil
}

func validateOverride(override *Override) error {
	if override == nil {
		return nil
	}
	if override.Content != nil {
		content := *override.Content
		switch {
		case !utf8.ValidString(content):
			return fmt.Errorf("agentic_rag.content must be valid UTF-8")
		case strings.ContainsRune(content, 0):
			return fmt.Errorf("agentic_rag.content must not contain NUL")
		case strings.TrimSpace(content) == "":
			return fmt.Errorf("agentic_rag.content must not be empty")
		case utf8.RuneCountInString(content) > 32768:
			return fmt.Errorf("agentic_rag.content must contain at most 32768 characters")
		}
	}
	if override.Tools == nil {
		return nil
	}
	registry := toolRegistry()
	seen := make(map[string]bool, len(*override.Tools))
	hasSearch := false
	for _, name := range *override.Tools {
		if _, ok := registry[name]; !ok {
			return fmt.Errorf("agentic_rag.tools contains unknown tool %q", name)
		}
		if seen[name] {
			return fmt.Errorf("agentic_rag.tools contains duplicate tool %q", name)
		}
		seen[name] = true
		switch name {
		case "grep_chunks", "search_chunks", "search_bm25_chunks", "search_semantic_chunks":
			hasSearch = true
		}
	}
	if !hasSearch {
		return fmt.Errorf("agentic_rag.tools requires at least one corpus search tool: grep_chunks, search_chunks, search_bm25_chunks, or search_semantic_chunks")
	}
	if !seen["list_chunks"] {
		return fmt.Errorf("agentic_rag.tools requires list_chunks to read document evidence")
	}
	return nil
}

// ResolveTemplate applies a chat override to a private copy of the selected
// current YAML template. It never changes the cached defaults.
func ResolveTemplate(id string, override *Override) (Template, error) {
	if err := validateOverride(override); err != nil {
		return Template{}, err
	}
	template, err := resolveTemplateFor(id)
	if err != nil {
		return template, err
	}
	return applyOverride(template, override), nil
}

func applyOverride(template Template, override *Override) Template {
	template.Tools = slices.Clone(template.Tools)
	if template.Temperature != nil {
		template.Temperature = new(*template.Temperature)
	}
	if override == nil {
		return template
	}
	if override.Content != nil {
		template.Content = *override.Content
		template.customContent = true
	}
	if override.Tools != nil {
		template.Tools = slices.Clone(*override.Tools)
		template.strictTools = true
	}
	return template
}

type ToolDescription struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Settings describes configured tools, whose execution still depends on the
// dataset capabilities and request authorization.
type Settings struct {
	BaseTemplate   string            `json:"base_template"`
	Defaults       Template          `json:"defaults"`
	Override       *Override         `json:"override"`
	Effective      Template          `json:"effective"`
	AvailableTools []ToolDescription `json:"available_tools"`
	UsingDefaults  bool              `json:"using_defaults"`
}

func GetSettings(ctx context.Context, override *Override) (*Settings, error) {
	defaults, err := ResolveTemplate("smart-reasoning", nil)
	if err != nil {
		return nil, err
	}
	if err := validateOverride(override); err != nil {
		return nil, err
	}
	effective := applyOverride(defaults, override)
	registry := toolRegistry()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	slices.Sort(names)
	available := make([]ToolDescription, 0, len(names))
	for _, name := range names {
		info, err := registry[name]("", nil).Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("agentic_rag: describe tool %q: %w", name, err)
		}
		if info == nil {
			return nil, fmt.Errorf("agentic_rag: tool %q has no description", name)
		}
		available = append(available, ToolDescription{Name: name, Description: info.Desc})
	}
	return &Settings{
		BaseTemplate: defaults.ID, Defaults: defaults, Override: override,
		Effective: effective, AvailableTools: available,
		UsingDefaults: override == nil || (override.Content == nil && override.Tools == nil),
	}, nil
}
