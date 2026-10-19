// Package component — ScoredSwitch.
//
// One scoring-model call ranks every configured option. The request is the
// public choice contract (state, one choice question, a probability per
// option). Image bytes travel as content parts, or as asset references when
// the endpoint requires that shape. JEV 1.4 is the specimen that contract
// was checked against; the component itself is not vendor-specific.
//
// Escalation is a single extra output. It fires when the top two scores
// are tied, the margin is below the node threshold, or the top score is
// below the node floor. The canvas author connects that output to an LLM
// node. Turning escalation off removes the branch: the highest score wins,
// and an exact tie keeps the earliest option.
package component

import (
	"context"
	"fmt"
	"math"
	"strings"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

const componentNameScoredSwitch = "ScoredSwitch"

// ScoredSwitchComponent scores a fixed option list and routes to one branch.
type ScoredSwitchComponent struct {
	param  scoredSwitchParam
	poster scoringPoster
	caps   *scoringCapabilities
}

type scoredSwitchParam struct {
	ModelID         string
	Driver          string
	APIKey          string
	BaseURL         string
	Query           string
	Image           string
	Instructions    string
	RequestFormat   string
	MarginThreshold *float64
	MinTopScore     *float64
	EscalateEnabled *bool
	Options         []scoredOption
	EscalateTo      []string
	Tasks           []scoringTask
}

// NewScoredSwitchComponent builds a component from resolved parameters.
func NewScoredSwitchComponent(p scoredSwitchParam) *ScoredSwitchComponent {
	return &ScoredSwitchComponent{param: p}
}

// Name returns the registered component id.
func (c *ScoredSwitchComponent) Name() string { return componentNameScoredSwitch }

// Invoke scores the case once and returns exactly one route.
func (c *ScoredSwitchComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	if inputs == nil {
		inputs = map[string]any{}
	}
	p, err := mergeScoredSwitchParam(c.param, inputs)
	if err != nil {
		return nil, err
	}
	originalModelID := p.ModelID
	p.ModelID, p.Driver, p.APIKey, p.BaseURL, err = resolveTypedModelRef(ctx, db, entity.ModelTypeScoring, p.ModelID, p.Driver, p.APIKey, p.BaseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.ModelID) == "" {
		return nil, &ParamError{Field: "model_id", Reason: "required"}
	}
	format, err := normalizeRequestFormat(p.RequestFormat)
	if err != nil {
		return nil, err
	}
	if _, _, err := scoringEndpoint(p.BaseURL, format); err != nil {
		return nil, err
	}
	options := normalizeScoredOptions(p.Options)
	if err := validateScoredOptions(options); err != nil {
		return nil, err
	}
	policy := p.routePolicy()
	if policy.EscalateEnabled {
		if err := validateUnitInterval("margin_threshold", policy.MarginThreshold); err != nil {
			return nil, err
		}
		if err := validateUnitInterval("min_top_score", policy.MinTopScore); err != nil {
			return nil, err
		}
	}
	query, err := resolveScoredQuery(ctx, p.Query, inputs)
	if err != nil {
		return nil, err
	}
	_, wire, err := scoringEndpoint(p.BaseURL, format)
	if err != nil {
		return nil, err
	}
	caps := loadScoringCapabilities(ctx, db, originalModelID)
	if c.caps != nil {
		caps = *c.caps
	}
	allowImages := !caps.Known || caps.Image
	if !allowImages {
		p.Image = ""
	}
	var images []scoringImage
	if allowImages {
		images, err = gatherScoringImages(ctx, query, p.Image, inputs, wire)
		if err != nil {
			return nil, err
		}
	}
	tasks := filterTasks(p.Tasks, caps)
	call, err := buildScoringCall(scoringCallInput{
		Model:        p.ModelID,
		BaseURL:      p.BaseURL,
		APIKey:       p.APIKey,
		Format:       format,
		Instructions: p.Instructions,
		Query:        query,
		Options:      options,
		Images:       images,
		Tasks:        tasks,
	})
	if err != nil {
		return nil, err
	}
	body, status, err := c.posterOrDefault().Post(ctx, call)
	if err != nil {
		return nil, fmt.Errorf("component: ScoredSwitch: model request failed: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("component: ScoredSwitch: %w", statusError(status, body, p.APIKey))
	}
	parsed, err := parseScoringPayload(body)
	if err != nil {
		return nil, fmt.Errorf("component: ScoredSwitch: unparseable scoring payload: %w", err)
	}
	aligned, err := alignAndNormalize(options, parsed.Probabilities)
	if err != nil {
		return nil, fmt.Errorf("component: ScoredSwitch: unparseable scoring payload: %w", err)
	}
	decision, err := routeScoredOptions(options, aligned, policy)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"chosen_id":       decision.ChosenID,
		"chosen_label":    decision.ChosenLabel,
		"scores":          decision.Scores,
		"margin":          decision.Margin,
		"escalated":       decision.Escalated,
		"selected_handle": decision.Handle,
		"raw":             truncateUTF8(body, rawPayloadLimit),
		"answers":         parseTypedAnswers(body),
		"_next":           stringListToAny(decision.Next),
	}
	if parsed.HasConfidence {
		out["confidence"] = parsed.Confidence
	}
	return out, nil
}

// Stream mirrors Invoke as one chunk.
func (c *ScoredSwitchComponent) Stream(ctx context.Context, db *gorm.DB, inputs map[string]any) (<-chan map[string]any, error) {
	out := make(chan map[string]any, 1)
	go func() {
		defer close(out)
		result, err := c.Invoke(ctx, db, inputs)
		if err != nil {
			out <- map[string]any{"error": err.Error()}
			return
		}
		out <- result
	}()
	return out, nil
}

// Inputs describes accepted parameters.
func (c *ScoredSwitchComponent) Inputs() map[string]string {
	return map[string]string{
		"model_id":         "Scoring model configured for this tenant. It must return a probability for every option.",
		"query":            "Case text. Defaults to sys.query.",
		"image":            "Optional image variable. PNG, JPEG, or WebP bytes. Empty sends text only.",
		"instructions":     "Choice question. Empty uses the default.",
		"request_format":   "content_parts or assets.",
		"options":          "Ordered options. Each has a name, optional description, id, and downstream targets.",
		"margin_threshold": "Escalate when top score minus the runner-up is below this value. Default 0.15.",
		"min_top_score":    "Escalate when the top score is below this value. 0 disables the floor.",
		"escalate_enabled": "When false, the highest score always wins and exact ties keep the earliest option.",
		"escalate_to":      "Downstream component ids for the escalate branch.",
	}
}

// Outputs describes values downstream nodes can read.
func (c *ScoredSwitchComponent) Outputs() map[string]string {
	return map[string]string{
		"chosen_id":       "Winning option id. Empty when the run escalates.",
		"chosen_label":    "Winning option label. Empty when the run escalates.",
		"scores":          "Probability for every option, keyed by label.",
		"margin":          "Top score minus the runner-up.",
		"escalated":       "True when the escalate branch was taken.",
		"selected_handle": "Option id or escalate.",
		"confidence":      "Model confidence when the payload includes it. Not used for routing.",
		"raw":             "Truncated model payload for debugging.",
		"_next":           "Downstream component id for the single chosen branch.",
	}
}

// GetInputForm exposes the case text and optional image to the run form.
func (c *ScoredSwitchComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{"type": "line", "name": "Query"},
		"image": map[string]any{"type": "line", "name": "Image"},
	}
}

func (p scoredSwitchParam) routePolicy() routePolicy {
	margin := defaultMarginThreshold
	if p.MarginThreshold != nil {
		margin = *p.MarginThreshold
	}
	minScore := defaultMinTopScore
	if p.MinTopScore != nil {
		minScore = *p.MinTopScore
	}
	enabled := true
	if p.EscalateEnabled != nil {
		enabled = *p.EscalateEnabled
	}
	return routePolicy{
		EscalateEnabled: enabled,
		MarginThreshold: margin,
		MinTopScore:     minScore,
		EscalateTo:      append([]string(nil), p.EscalateTo...),
	}
}

func validateUnitInterval(field string, v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
		return &ParamError{Field: field, Reason: "must be between 0 and 1"}
	}
	return nil
}

func normalizeScoredOptions(options []scoredOption) []scoredOption {
	out := make([]scoredOption, len(options))
	for i, opt := range options {
		opt.Label = strings.TrimSpace(opt.Label)
		opt.Description = strings.TrimSpace(opt.Description)
		opt.ID = strings.TrimSpace(opt.ID)
		if opt.ID == "" {
			opt.ID = opt.Label
		}
		opt.To = compactStrings(opt.To)
		out[i] = opt
	}
	return out
}

func validateScoredOptions(options []scoredOption) error {
	if len(options) < 2 {
		return &ParamError{Field: "options", Reason: "at least two options are required"}
	}
	if len(options) > 255 {
		return &ParamError{Field: "options", Reason: "at most 255 options are allowed"}
	}
	labels := make(map[string]struct{}, len(options))
	ids := make(map[string]struct{}, len(options))
	for _, opt := range options {
		if opt.Label == "" {
			return &ParamError{Field: "options", Reason: "every option needs a name"}
		}
		if _, ok := labels[opt.Label]; ok {
			return &ParamError{Field: "options", Reason: "option names must be unique"}
		}
		labels[opt.Label] = struct{}{}
		if opt.ID == scoredSwitchEscalateHandle {
			return &ParamError{Field: "options", Reason: "option id escalate is reserved"}
		}
		if _, ok := ids[opt.ID]; ok {
			return &ParamError{Field: "options", Reason: "option ids must be unique"}
		}
		ids[opt.ID] = struct{}{}
	}
	return nil
}

func mergeScoredSwitchParam(base scoredSwitchParam, inputs map[string]any) (scoredSwitchParam, error) {
	p := base
	p.Options = append([]scoredOption(nil), base.Options...)
	p.EscalateTo = append([]string(nil), base.EscalateTo...)
	p.Tasks = append([]scoringTask(nil), base.Tasks...)
	if v, ok := stringFrom(inputs, "model_id"); ok && strings.TrimSpace(v) != "" {
		p.ModelID = v
	} else if v, ok := stringFrom(inputs, "llm_id"); ok && strings.TrimSpace(v) != "" {
		p.ModelID = v
	}
	if v, ok := stringFrom(inputs, "query"); ok {
		p.Query = v
	}
	if v, ok := stringFrom(inputs, "image"); ok {
		p.Image = v
	}
	if v, ok := stringFrom(inputs, "instructions"); ok {
		p.Instructions = v
	}
	if v, ok := stringFrom(inputs, "request_format"); ok {
		p.RequestFormat = v
	}
	if _, ok := inputs["margin_threshold"]; ok {
		v, ok := floatFrom(inputs, "margin_threshold")
		if !ok {
			return p, &ParamError{Field: "margin_threshold", Reason: "must be a number"}
		}
		p.MarginThreshold = &v
	}
	if _, ok := inputs["min_top_score"]; ok {
		v, ok := floatFrom(inputs, "min_top_score")
		if !ok {
			return p, &ParamError{Field: "min_top_score", Reason: "must be a number"}
		}
		p.MinTopScore = &v
	}
	if _, ok := inputs["escalate_enabled"]; ok {
		v, ok := boolPtrFrom(inputs, "escalate_enabled")
		if !ok {
			return p, &ParamError{Field: "escalate_enabled", Reason: "must be a boolean"}
		}
		p.EscalateEnabled = v
	}
	if raw, ok := inputs["options"]; ok {
		opts, err := parseScoredOptions(raw)
		if err != nil {
			return p, err
		}
		p.Options = opts
	} else if raw, ok := inputs["items"]; ok {
		opts, err := parseScoredOptions(raw)
		if err != nil {
			return p, err
		}
		if len(opts) > 0 {
			p.Options = opts
		}
	}
	if _, ok := inputs["escalate_to"]; ok {
		p.EscalateTo = stringListFrom(inputs["escalate_to"])
	}
	return p, nil
}

func boolPtrFrom(inputs map[string]any, name string) (*bool, bool) {
	v, ok := inputs[name]
	if !ok || v == nil {
		return nil, false
	}
	switch t := v.(type) {
	case bool:
		b := t
		return &b, true
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes", "on":
			b := true
			return &b, true
		case "false", "0", "no", "off":
			b := false
			return &b, true
		default:
			return nil, false
		}
	case float64:
		b := t != 0
		return &b, true
	case int:
		b := t != 0
		return &b, true
	default:
		return nil, false
	}
}

func parseScoredOptions(raw any) ([]scoredOption, error) {
	items, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]scoredOption); ok {
			return append([]scoredOption(nil), typed...), nil
		}
		return nil, &ParamError{Field: "options", Reason: "must be a list"}
	}
	out := make([]scoredOption, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, &ParamError{Field: "options", Reason: "each option must be an object"}
		}
		label, _ := m["name"].(string)
		if strings.TrimSpace(label) == "" {
			label, _ = m["label"].(string)
		}
		id, _ := m["uuid"].(string)
		if strings.TrimSpace(id) == "" {
			id, _ = m["id"].(string)
		}
		description, _ := m["description"].(string)
		targets := stringListFrom(m["to"])
		out = append(out, scoredOption{
			ID:          id,
			Label:       label,
			Description: description,
			To:          targets,
		})
	}
	return out, nil
}

func stringListFrom(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return compactStrings([]string{t})
	case []string:
		return compactStrings(t)
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return compactStrings(out)
	default:
		return nil
	}
}

func compactStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			out = append(out, item)
		}
	}
	return out
}

func stringListToAny(items []string) []any {
	if len(items) == 0 {
		return []any{}
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func init() {
	Register(componentNameScoredSwitch, func(params map[string]any) (Component, error) {
		p := scoredSwitchParam{}
		if v, ok := stringFrom(params, "model_id"); ok {
			p.ModelID = v
		} else if v, ok := stringFrom(params, "llm_id"); ok {
			p.ModelID = v
		}
		if v, ok := stringFrom(params, "query"); ok {
			p.Query = v
		}
		if v, ok := stringFrom(params, "image"); ok {
			p.Image = v
		}
		if v, ok := stringFrom(params, "instructions"); ok {
			p.Instructions = v
		}
		if v, ok := stringFrom(params, "request_format"); ok {
			p.RequestFormat = v
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
		if _, ok := params["margin_threshold"]; ok {
			v, ok := floatFrom(params, "margin_threshold")
			if !ok {
				return nil, &ParamError{Field: "margin_threshold", Reason: "must be a number"}
			}
			p.MarginThreshold = &v
		}
		if _, ok := params["min_top_score"]; ok {
			v, ok := floatFrom(params, "min_top_score")
			if !ok {
				return nil, &ParamError{Field: "min_top_score", Reason: "must be a number"}
			}
			p.MinTopScore = &v
		}
		if _, ok := params["escalate_enabled"]; ok {
			v, ok := boolPtrFrom(params, "escalate_enabled")
			if !ok {
				return nil, &ParamError{Field: "escalate_enabled", Reason: "must be a boolean"}
			}
			p.EscalateEnabled = v
		}
		if raw, ok := params["options"]; ok {
			opts, err := parseScoredOptions(raw)
			if err != nil {
				return nil, err
			}
			p.Options = opts
		} else if raw, ok := params["items"]; ok {
			opts, err := parseScoredOptions(raw)
			if err != nil {
				return nil, err
			}
			p.Options = opts
		}
		p.EscalateTo = stringListFrom(params["escalate_to"])
		if raw, ok := params["tasks"]; ok {
			tasks, err := parseScoringTasks(raw)
			if err != nil {
				return nil, err
			}
			p.Tasks = tasks
		}
		return NewScoredSwitchComponent(p), nil
	})
}
