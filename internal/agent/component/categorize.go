// Package component — Categorize (T3).
//
// LLM-based classifier. The component asks the model to pick exactly
// one of the configured categories, returns the chosen category name
// plus a uniform score map (1.0 for the chosen category, 0.0 for the
// rest). The MultiBranch wiring in canvas/multibranch.go consumes
// outputs["_next"] for runtime routing; the field is reserved for
// that consumer.
package component

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// CategorizeComponent is an LLM classifier.
type CategorizeComponent struct {
	param CategorizeParam
}

// CategorizeParam captures the (resolved) DSL parameters for a Categorize node.
type CategorizeParam struct {
	ModelID         string
	Items           []string
	Categories      []string
	SysPrompt       string
	DefaultCategory string
	Driver          string
	APIKey          string
	BaseURL         string
}

// CategorizeOutput mirrors the outputs map (per plan §2.11.3 row 6):
//
//	"category" string             — chosen category name (or default if
//	                                model returned something not in list)
//	"scores"   map[string]float64
//	"_next"    []string           — reserved for canvas/multibranch.go routing
type CategorizeOutput struct {
	Category string
	Scores   map[string]float64
	Next     []string
}

// NewCategorizeComponent builds a CategorizeComponent from raw params.
func NewCategorizeComponent(p CategorizeParam) *CategorizeComponent {
	return &CategorizeComponent{param: p}
}

// Name returns the registered component name.
func (c *CategorizeComponent) Name() string { return "Categorize" }

// Invoke calls the chat model, parses the response for a category, and
// returns the chosen category (or the default if the model returned
// something outside the configured set).
func (c *CategorizeComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	p := mergeCategorizeParam(c.param, inputs)
	if p.ModelID == "" {
		return nil, &ParamError{Field: "model_id", Reason: "required"}
	}
	if len(p.Categories) == 0 {
		return nil, &ParamError{Field: "categories", Reason: "at least one category is required"}
	}
	if p.DefaultCategory == "" {
		// Fall back to the first category so the run never fails purely
		// because the user omitted the default.
		p.DefaultCategory = p.Categories[0]
	}

	inv := getDefaultChatInvoker()
	sysPrompt := p.SysPrompt
	if sysPrompt == "" {
		sysPrompt = "You are a strict classifier."
	}
	userPrompt := buildCategorizePrompt(p)
	msgs := []schema.Message{
		{Role: schema.System, Content: sysPrompt},
		{Role: schema.User, Content: userPrompt},
	}
	resp, err := inv.Invoke(ctx, ChatInvokeRequest{
		Driver:    p.Driver,
		ModelName: p.ModelID,
		APIKey:    p.APIKey,
		BaseURL:   p.BaseURL,
		Messages:  msgs,
	})
	if err != nil {
		return nil, fmt.Errorf("component: Categorize.Invoke: %w", err)
	}

	chosen, score := pickCategory(resp.Content, p.Categories, p.DefaultCategory)
	return map[string]any{
		"category": chosen,
		"scores":   score,
		"_next":    []string{},
	}, nil
}

// Stream mirrors Invoke as a single chunk.
func (c *CategorizeComponent) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out := make(chan map[string]any, 1)
	go func() {
		defer close(out)
		result, err := c.Invoke(ctx, inputs)
		if err != nil {
			out <- map[string]any{"error": err.Error()}
			return
		}
		out <- result
	}()
	return out, nil
}

// Inputs returns parameter metadata for tooling.
func (c *CategorizeComponent) Inputs() map[string]string {
	return map[string]string{
		"model_id":         "Provider-side model identifier",
		"items":            "Optional list of items to classify (added to the prompt as context)",
		"categories":       "List of allowed category names (response must match one)",
		"sys_prompt":       "Optional system prompt; defaults to a strict classifier instruction",
		"default_category": "Category returned if the model's answer is not in `categories` (defaults to categories[0])",
		"driver":           "Provider driver name",
		"api_key":          "Override API key",
	}
}

// Outputs returns output metadata.
func (c *CategorizeComponent) Outputs() map[string]string {
	return map[string]string{
		"category": "Chosen category name (one of the configured list, or the default)",
		"scores":   "Score map (1.0 for the chosen category, 0.0 for the rest)",
		"_next":    "Reserved for canvas/multibranch.go routing; currently empty",
	}
}

// buildCategorizePrompt assembles a prompt that asks the model to pick a
// category. The categories are listed deterministically (sorted) so the
// prompt is stable across runs.
func buildCategorizePrompt(p CategorizeParam) string {
	cats := append([]string(nil), p.Categories...)
	sort.Strings(cats)
	var b strings.Builder
	b.WriteString("Classify the following item into exactly one of these categories:\n")
	for _, c := range cats {
		b.WriteString("- ")
		b.WriteString(c)
		b.WriteString("\n")
	}
	if len(p.Items) > 0 {
		b.WriteString("\nItems:\n")
		for _, it := range p.Items {
			b.WriteString("- ")
			b.WriteString(it)
			b.WriteString("\n")
		}
	}
	b.WriteString("\nRespond with ONLY the category name, no other text.")
	return b.String()
}

// pickCategory extracts a category from the model's response. Strategy:
//  1. exact match (case-sensitive)
//  2. case-insensitive match
//  3. fall back to default
//
// Substring matching is intentionally avoided — it makes the picker too
// eager ("I have no idea" would match a category named "a"). If the model
// can't produce one of the categories verbatim, the default is used.
//
// Scores are 1.0 for the chosen category, 0.0 for the rest.
func pickCategory(response string, categories []string, def string) (string, map[string]float64) {
	scores := make(map[string]float64, len(categories))
	for _, c := range categories {
		scores[c] = 0
	}
	resp := strings.TrimSpace(response)
	resp = strings.Trim(resp, "\"'`\n\r\t ")
	resp = strings.TrimPrefix(resp, "category:")
	resp = strings.TrimPrefix(resp, "Category:")
	resp = strings.TrimSpace(resp)

	for _, c := range categories {
		if resp == c {
			scores[c] = 1
			return c, scores
		}
	}
	lower := strings.ToLower(resp)
	for _, c := range categories {
		if strings.ToLower(c) == lower {
			scores[c] = 1
			return c, scores
		}
	}
	scores[def] = 1
	return def, scores
}

// mergeCategorizeParam layers raw inputs over the receiver's default param set.
//
// v1 aliases accepted alongside the v2 names: "llm_id" → "model_id",
// "category_description" (a map[string]string) → "categories" (the keys
// of the map), and "base_url" → "BaseURL". v1 fixtures use the
// short / dict forms; without these aliases the v1→v2 conversion step
// would have to run before the factory builds the component.
func mergeCategorizeParam(base CategorizeParam, inputs map[string]any) CategorizeParam {
	p := base
	if v, ok := stringFrom(inputs, "model_id"); ok {
		p.ModelID = v
	} else if v, ok := stringFrom(inputs, "llm_id"); ok {
		p.ModelID = v
	}
	if v, ok := sliceFrom(inputs, "items"); ok {
		p.Items = v
	}
	if v, ok := sliceFrom(inputs, "categories"); ok {
		p.Categories = v
	} else if m, ok := stringMapFrom(inputs, "category_description"); ok && len(m) > 0 {
		// v1 stores the categories as a map of {name: description}.
		// We only need the keys to drive the picker.
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		p.Categories = keys
	}
	if v, ok := stringFrom(inputs, "sys_prompt"); ok {
		p.SysPrompt = v
	} else if v, ok := stringFrom(inputs, "system_prompt"); ok {
		p.SysPrompt = v
	}
	if v, ok := stringFrom(inputs, "default_category"); ok {
		p.DefaultCategory = v
	}
	if v, ok := stringFrom(inputs, "driver"); ok {
		p.Driver = v
	}
	if v, ok := stringFrom(inputs, "api_key"); ok {
		p.APIKey = v
	}
	if v, ok := stringFrom(inputs, "base_url"); ok {
		p.BaseURL = v
	}
	return p
}

// stringMapFrom extracts map[string]string from inputs[name]. The v1
// "category_description" field is shaped this way (name → human
// description); we only consume the keys.
func stringMapFrom(inputs map[string]any, name string) (map[string]string, bool) {
	v, ok := inputs[name]
	if !ok {
		return nil, false
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(raw))
	for k, child := range raw {
		if s, ok := child.(string); ok {
			out[k] = s
			continue
		}
		// Some encoders nest the description under a "description"
		// key; handle that fallback defensively.
		if nested, ok := child.(map[string]any); ok {
			if s, ok := nested["description"].(string); ok {
				out[k] = s
				continue
			}
		}
		out[k] = ""
	}
	return out, true
}

// init registers CategorizeComponent with the orchestrator-owned registry.
func init() {
	Register("Categorize", func(params map[string]any) (Component, error) {
		var p CategorizeParam
		if v, ok := stringFrom(params, "model_id"); ok {
			p.ModelID = v
		} else if v, ok := stringFrom(params, "llm_id"); ok {
			p.ModelID = v
		}
		if v, ok := sliceFrom(params, "items"); ok {
			p.Items = v
		}
		if v, ok := sliceFrom(params, "categories"); ok {
			p.Categories = v
		} else if m, ok := params["category_description"].(map[string]any); ok && len(m) > 0 {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			p.Categories = keys
		}
		if v, ok := stringFrom(params, "sys_prompt"); ok {
			p.SysPrompt = v
		} else if v, ok := stringFrom(params, "system_prompt"); ok {
			p.SysPrompt = v
		}
		if v, ok := stringFrom(params, "default_category"); ok {
			p.DefaultCategory = v
		}
		if v, ok := stringFrom(params, "driver"); ok {
			p.Driver = v
		}
		if v, ok := stringFrom(params, "api_key"); ok {
			p.APIKey = v
		}
		if v, ok := stringFrom(params, "base_url"); ok {
			p.BaseURL = v
		}
		return NewCategorizeComponent(p), nil
	})
}
