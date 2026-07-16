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
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// CategorizeComponent is an LLM classifier.
type CategorizeComponent struct {
	param CategorizeParam
}

// CategorizeParam captures the (resolved) DSL parameters for a Categorize node.
type CategorizeParam struct {
	ModelID                  string
	Query                    string
	Items                    []string
	Categories               []string
	CategoryRoutes           map[string]string
	CategoryDescriptions     map[string]string
	CategoryExamples         map[string][]string
	SysPrompt                string
	DefaultCategory          string
	MessageHistoryWindowSize int
	Driver                   string
	APIKey                   string
	BaseURL                  string
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
	var err error
	p.ModelID, p.Driver, p.APIKey, p.BaseURL, err = resolveChatModelRef(ctx, p.ModelID, p.Driver, p.APIKey, p.BaseURL)
	if err != nil {
		return nil, err
	}
	if p.ModelID == "" {
		return nil, &ParamError{Field: "model_id", Reason: "required"}
	}
	// Split composite llm_id (e.g. "model@provider" or
	// "model@instance@provider") into bare model name + driver.
	if modelName, driver, hasDriver := splitCompositeLLMID(p.ModelID); hasDriver {
		p.ModelID = modelName
		if p.Driver == "" {
			p.Driver = driver
		}
	}
	// Resolve missing APIKey / BaseURL from the tenant DB when
	// credentials were not provided directly in the DSL params.
	if p.APIKey == "" && p.Driver != "" {
		p = resolveCategorizeCredentials(ctx, p)
	}
	if len(p.Categories) == 0 {
		return nil, &ParamError{Field: "categories", Reason: "at least one category is required"}
	}
	if p.MessageHistoryWindowSize < 0 {
		return nil, &ParamError{Field: "message_history_window_size", Reason: "cannot be negative"}
	}
	if p.DefaultCategory == "" {
		// Fall back to the first category so the run never fails purely
		// because the user omitted the default.
		p.DefaultCategory = p.Categories[0]
	}

	inv := getDefaultChatInvoker()
	sysPrompt := p.SysPrompt
	if sysPrompt == "" {
		sysPrompt = buildCategorizeSystemPrompt(p)
	}
	query := resolveCategorizeQuery(ctx, p, inputs)
	userPrompt := buildCategorizePrompt(categorizeHistory(ctx, p.MessageHistoryWindowSize, query))
	msgs := []ComponentMessage{
		{Role: RoleSystem, Content: sysPrompt},
		{Role: RoleUser, Content: userPrompt},
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
	next := resolveNext(chosen, p.CategoryRoutes, inputs)
	return map[string]any{
		"category_name": chosen,
		"scores":        score,
		"_next":         next,
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
		"model_id":                    "Provider-side model identifier",
		"query":                       "Variable reference or literal text to classify. Defaults to sys.query.",
		"items":                       "Optional list of items to classify (added to the prompt as context)",
		"categories":                  "List of allowed category names (response must match one)",
		"sys_prompt":                  "Optional system prompt; defaults to a strict classifier instruction",
		"default_category":            "Category returned if the model's answer is not in `categories` (defaults to categories[0])",
		"message_history_window_size": "How many prior conversation turns to include in the Real Data prompt.",
		"driver":                      "Provider driver name",
		"api_key":                     "Override API key",
	}
}

func (c *CategorizeComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"type": "line",
			"name": "Query",
		},
	}
}

// Outputs returns output metadata.
func (c *CategorizeComponent) Outputs() map[string]string {
	return map[string]string{
		"category_name": "Chosen category name (one of the configured list, or the default)",
		"scores":        "Score map (1.0 for the chosen category, 0.0 for the rest)",
		"_next":         "Reserved for canvas/multibranch.go routing; currently empty",
	}
}

func buildCategorizeSystemPrompt(p CategorizeParam) string {
	cats := append([]string(nil), p.Categories...)
	sort.Strings(cats)
	var b strings.Builder
	b.WriteString("You are an advanced classification system that categorizes user questions into specific types. Analyze the input question and classify it into ONE of the following categories:\n")
	for _, c := range cats {
		b.WriteString("- ")
		b.WriteString(c)
		b.WriteString("\n")
	}

	if len(p.CategoryDescriptions) > 0 {
		b.WriteString("\nHere's description of each category:\n")
		for _, c := range cats {
			desc := strings.TrimSpace(p.CategoryDescriptions[c])
			if desc == "" {
				continue
			}
			b.WriteString("\n------\nCategory: ")
			b.WriteString(c)
			b.WriteString("\nDescription: ")
			b.WriteString(desc)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n---- Instructions ----\n")
	b.WriteString("- Consider both explicit mentions and implied context\n")
	b.WriteString("- Prioritize the most specific applicable category\n")
	b.WriteString("- Return only the category name without explanations\n")
	if containsCategory(cats, "Other") {
		b.WriteString("- Use \"Other\" only when no other category fits\n")
	}

	examples := categorizeExamples(p, cats)
	if len(examples) > 0 {
		b.WriteString("\n---- Examples ----\n")
		for _, line := range examples {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func containsCategory(categories []string, target string) bool {
	for _, c := range categories {
		if c == target {
			return true
		}
	}
	return false
}

func categorizeExamples(p CategorizeParam, cats []string) []string {
	var lines []string
	for _, c := range cats {
		for _, example := range p.CategoryExamples[c] {
			example = strings.TrimSpace(strings.ReplaceAll(example, "\n", "    "))
			if example == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("USER: %q -> %s", example, c))
		}
	}
	for _, it := range p.Items {
		it = strings.TrimSpace(strings.ReplaceAll(it, "\n", "    "))
		if it == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("USER: %q", it))
	}
	return lines
}

type categorizeHistoryMessage struct {
	Role    string
	Content string
}

func categorizeHistory(ctx context.Context, window int, query string) []categorizeHistoryMessage {
	msgs := []categorizeHistoryMessage{}
	if window > 0 {
		if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
			start := 0
			if len(state.History) > window {
				start = len(state.History) - window
			}
			for i := start; i < len(state.History); i++ {
				entry := state.History[i]
				role, _ := entry["role"].(string)
				content, _ := entry["content"].(string)
				if role == "" || content == "" {
					continue
				}
				msgs = append(msgs, categorizeHistoryMessage{Role: role, Content: content})
			}
		}
	}
	if len(msgs) == 0 {
		msgs = append(msgs, categorizeHistoryMessage{Role: "user"})
	}
	msgs[len(msgs)-1].Content = query
	return msgs
}

func buildCategorizePrompt(history []categorizeHistoryMessage) string {
	parts := make([]string, 0, len(history))
	for _, msg := range history {
		role := strings.ToUpper(strings.TrimSpace(msg.Role))
		if role == "" {
			role = "USER"
		}
		content := strings.NewReplacer("\r\n", "", "\n", "", "\r", "").Replace(msg.Content)
		parts = append(parts, fmt.Sprintf("%s: %q", role, content))
	}
	return fmt.Sprintf("\n---- Real Data ----\n%s ->\n", strings.Join(parts, " | "))
}

func resolveCategorizeQuery(ctx context.Context, p CategorizeParam, inputs map[string]any) string {
	if v, ok := stringValueFromAny(inputs["query"]); ok {
		return v
	}
	queryRef := strings.TrimSpace(p.Query)
	if queryRef == "" {
		queryRef = "sys.query"
	}
	if v, ok := stringValueFromAny(inputs[queryRef]); ok && strings.TrimSpace(v) != "" {
		return v
	}
	if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
		if v, err := state.GetVar(queryRef); err == nil {
			if s, ok := stringValueFromAny(v); ok {
				return s
			}
		}
	}
	return ""
}

func stringValueFromAny(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	default:
		return fmt.Sprint(t), true
	}
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
	if v, ok := stringFrom(inputs, "query"); ok {
		p.Query = v
	}
	if v, ok := sliceFrom(inputs, "items"); ok {
		p.Items = v
	}
	if v, ok := sliceFrom(inputs, "categories"); ok {
		p.Categories = v
	}
	if meta, ok := categoryMetadataFrom(inputs, "category_description"); ok && len(meta.Names) > 0 {
		if len(p.Categories) == 0 {
			p.Categories = meta.Names
		}
		p.CategoryDescriptions = mergeStringMap(p.CategoryDescriptions, meta.Descriptions)
		p.CategoryExamples = mergeStringSliceMap(p.CategoryExamples, meta.Examples)
	}
	if routes, ok := categoryRoutesFrom(inputs, "category_description"); ok {
		p.CategoryRoutes = mergeStringMap(p.CategoryRoutes, routes)
	}
	if v, ok := stringFrom(inputs, "sys_prompt"); ok {
		p.SysPrompt = v
	} else if v, ok := stringFrom(inputs, "system_prompt"); ok {
		p.SysPrompt = v
	}
	if v, ok := stringFrom(inputs, "default_category"); ok {
		p.DefaultCategory = v
	}
	if v, ok := intFrom(inputs, "message_history_window_size"); ok {
		p.MessageHistoryWindowSize = v
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

type categorizeMetadata struct {
	Names        []string
	Descriptions map[string]string
	Examples     map[string][]string
}

func categoryMetadataFrom(inputs map[string]any, name string) (categorizeMetadata, bool) {
	v, ok := inputs[name]
	if !ok {
		return categorizeMetadata{}, false
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return categorizeMetadata{}, false
	}
	out := categorizeMetadata{
		Names:        make([]string, 0, len(raw)),
		Descriptions: make(map[string]string, len(raw)),
		Examples:     make(map[string][]string, len(raw)),
	}
	for k, child := range raw {
		out.Names = append(out.Names, k)
		if s, ok := child.(string); ok {
			out.Descriptions[k] = s
			continue
		}
		if nested, ok := child.(map[string]any); ok {
			if s, ok := nested["description"].(string); ok {
				out.Descriptions[k] = s
			}
			out.Examples[k] = examplesFromAny(nested["examples"])
			continue
		}
	}
	sort.Strings(out.Names)
	return out, true
}

func examplesFromAny(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		switch t := item.(type) {
		case string:
			if t != "" {
				out = append(out, t)
			}
		case map[string]any:
			if s, _ := t["value"].(string); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func mergeStringMap(base, override map[string]string) map[string]string {
	if len(override) == 0 {
		return base
	}
	out := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func mergeStringSliceMap(base, override map[string][]string) map[string][]string {
	if len(override) == 0 {
		return base
	}
	out := make(map[string][]string, len(base)+len(override))
	for k, v := range base {
		out[k] = append([]string(nil), v...)
	}
	for k, v := range override {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// resolveCategorizeCredentials tries to populate APIKey and BaseURL from
// the tenant's LLM configuration in the database when the DSL params did
// not provide them directly.  Mirrors the server_main.go modelLocator
// logic so unit tests without a boot-time modelLocator also work.
func resolveCategorizeCredentials(ctx context.Context, p CategorizeParam) CategorizeParam {
	if p.Driver == "" {
		return p
	}
	tenantID := ""
	if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
		if tid, ok := state.Sys["tenant_id"].(string); ok {
			tenantID = tid
		}
	}
	if tenantID == "" {
		return p
	}
	// 1) Try TenantLLM: unique index on (tenant_id, llm_factory, llm_name).
	if dao.DB != nil {
		var rec entity.TenantLLM
		if err := dao.DB.Where("tenant_id = ? AND llm_factory = ? AND llm_name = ?",
			tenantID, p.Driver, p.ModelID).First(&rec).Error; err == nil {
			if rec.APIKey != nil {
				p.APIKey = *rec.APIKey
			}
			if p.BaseURL == "" && rec.APIBase != nil {
				p.BaseURL = *rec.APIBase
			}
			return p
		}
	}
	// 2) Fall back to TenantModelInstance (provider-based).
	if dao.DB != nil {
		var provider entity.TenantModelProvider
		if err := dao.DB.Where("tenant_id = ? AND provider_name = ?",
			tenantID, p.Driver).First(&provider).Error; err == nil {
			var inst entity.TenantModelInstance
			// Try matching instance_name first, then any active instance.
			err = dao.DB.Where("provider_id = ? AND instance_name = ?",
				provider.ID, "default").First(&inst).Error
			if err != nil {
				err = dao.DB.Where("provider_id = ? AND status = ?",
					provider.ID, "active").First(&inst).Error
			}
			if err == nil {
				p.APIKey = inst.APIKey
				if p.BaseURL == "" && inst.Extra != "" {
					var extra struct {
						BaseURL string `json:"base_url"`
					}
					if json.Unmarshal([]byte(inst.Extra), &extra) == nil && extra.BaseURL != "" {
						p.BaseURL = extra.BaseURL
					}
				}
			}
		}
	}
	return p
}

func categoryRoutesFrom(inputs map[string]any, name string) (map[string]string, bool) {
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
		if m, ok := child.(map[string]any); ok {
			if route, ok := firstRouteTarget(m["to"]); ok {
				out[k] = route
			} else if uuid, ok := m["uuid"].(string); ok && uuid != "" {
				out[k] = uuid
			}
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func firstRouteTarget(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	switch x := v.(type) {
	case string:
		return x, true
	case []any:
		if len(x) > 0 {
			if s, ok := x[0].(string); ok {
				return s, true
			}
		}
	}
	return "", false
}

// init registers CategorizeComponent with the orchestrator-owned registry.
func init() {
	Register("Categorize", func(params map[string]any) (Component, error) {
		p := CategorizeParam{MessageHistoryWindowSize: 1}
		if v, ok := stringFrom(params, "model_id"); ok {
			p.ModelID = v
		} else if v, ok := stringFrom(params, "llm_id"); ok {
			p.ModelID = v
		}
		if v, ok := stringFrom(params, "query"); ok {
			p.Query = v
		}
		// Check the object-style []any of maps first. sliceFrom would
		// otherwise match the same []any input and return (empty, true)
		// for non-string elements, making the object branch unreachable.
		if items, ok := params["items"].([]any); ok && len(items) > 0 {
			names := make([]string, 0, len(items))
			routes := make(map[string]string, len(items))
			for _, item := range items {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				name, _ := m["name"].(string)
				if name == "" {
					continue
				}
				names = append(names, name)
				if desc, _ := m["description"].(string); desc != "" {
					if p.CategoryDescriptions == nil {
						p.CategoryDescriptions = map[string]string{}
					}
					p.CategoryDescriptions[name] = desc
				}
				if route, ok := firstRouteTarget(m["to"]); ok {
					routes[name] = route
				} else if uuid, _ := m["uuid"].(string); uuid != "" {
					routes[name] = uuid
				}
				if examples, ok := m["examples"].([]any); ok {
					for _, example := range examples {
						if p.CategoryExamples == nil {
							p.CategoryExamples = map[string][]string{}
						}
						switch em := example.(type) {
						case map[string]any:
							if v, _ := em["value"].(string); v != "" {
								p.CategoryExamples[name] = append(p.CategoryExamples[name], v)
							}
						case string:
							if em != "" {
								p.CategoryExamples[name] = append(p.CategoryExamples[name], em)
							}
						}
					}
				}
			}
			if len(names) > 0 {
				p.Categories = names
			}
			if len(routes) > 0 {
				p.CategoryRoutes = routes
			}
		} else if v, ok := sliceFrom(params, "items"); ok {
			p.Items = v
		}
		if v, ok := sliceFrom(params, "categories"); ok {
			p.Categories = v
		}
		if meta, ok := categoryMetadataFrom(params, "category_description"); ok && len(meta.Names) > 0 {
			if len(p.Categories) == 0 {
				p.Categories = meta.Names
			}
			p.CategoryDescriptions = mergeStringMap(p.CategoryDescriptions, meta.Descriptions)
			p.CategoryExamples = mergeStringSliceMap(p.CategoryExamples, meta.Examples)
			if routes, ok := categoryRoutesFrom(params, "category_description"); ok {
				p.CategoryRoutes = mergeStringMap(p.CategoryRoutes, routes)
			}
		}
		if v, ok := stringFrom(params, "sys_prompt"); ok {
			p.SysPrompt = v
		} else if v, ok := stringFrom(params, "system_prompt"); ok {
			p.SysPrompt = v
		}
		if v, ok := stringFrom(params, "default_category"); ok {
			p.DefaultCategory = v
		}
		if v, ok := intFrom(params, "message_history_window_size"); ok {
			p.MessageHistoryWindowSize = v
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

// resolveNext determines the routing target for the chosen category.
// It checks CategoryRoutes first (for simple category→node mapping),
// then falls back to category_description in the inputs map.
func resolveNext(chosen string, categoryRoutes map[string]string, inputs map[string]any) []string {
	// 1. Direct mapping from constructor param.
	if dst, ok := categoryRoutes[chosen]; ok && dst != "" {
		return []string{dst}
	}
	// 2. Fall back to category_description in runtime inputs.
	if catDescs, ok := inputs["category_description"].(map[string]any); ok {
		if desc, ok := catDescs[chosen].(map[string]any); ok {
			if toRaw, ok := desc["to"]; ok {
				switch to := toRaw.(type) {
				case []string:
					return to
				case []any:
					dests := make([]string, 0, len(to))
					for _, v := range to {
						if s, ok := v.(string); ok {
							dests = append(dests, s)
						}
					}
					if len(dests) > 0 {
						return dests
					}
				}
			}
		}
	}
	return []string{}
}
