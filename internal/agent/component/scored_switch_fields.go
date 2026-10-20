package component

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	maxFieldsImages      = 4
	maxFieldsImageBytes  = 4 << 20
	maxFieldsImagesBytes = 8 << 20
)

var fieldNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]{0,63}$`)

// invokeFields sends one typed-field request and returns answers without a branch.
// The body follows the public fields schema: model, state, fields, and optional
// images, reference_date, and options. Choice questions are not sent.
func (c *ScoredSwitchComponent) invokeFields(ctx context.Context, p scoredSwitchParam, caps scoringCapabilities, inputs map[string]any) (map[string]any, error) {
	query, err := resolveOptionalScoredQuery(ctx, p.Query, inputs)
	if err != nil {
		return nil, err
	}
	var images []scoringImage
	if caps.Image {
		images, err = gatherScoringImages(ctx, query, p.Image, inputs, requestFormatContentParts)
		if err != nil {
			return nil, err
		}
	}
	tasks := filterTasks(p.Tasks, caps)
	includeQuantiles := p.IncludeQuantiles != nil && *p.IncludeQuantiles
	returnStateText := p.ReturnStateText != nil && *p.ReturnStateText
	call, err := buildFieldsCall(scoringCallInput{
		Model:            p.ModelID,
		BaseURL:          p.BaseURL,
		APIKey:           p.APIKey,
		Query:            query,
		Images:           images,
		Tasks:            tasks,
		ReferenceDate:    p.ReferenceDate,
		IncludeQuantiles: includeQuantiles,
		ReturnStateText:  returnStateText,
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
	answers, err := parseFieldsAnswers(body)
	if err != nil {
		return nil, fmt.Errorf("component: ScoredSwitch: unparseable scoring payload: %w", err)
	}
	out := map[string]any{
		"chosen_id":       "",
		"chosen_label":    "",
		"scores":          map[string]float64{},
		"margin":          0.0,
		"escalated":       false,
		"selected_handle": "",
		"raw":             truncateUTF8(body, rawPayloadLimit),
		"answers":         answers,
		"_next":           []any{},
	}
	if usage := parseScoringUsage(body); usage != nil {
		out["usage"] = usage
	}
	if text := parseStateText(body); text != "" {
		out["state_text"] = text
	}
	return out, nil
}

func resolveOptionalScoredQuery(ctx context.Context, configured string, inputs map[string]any) (string, error) {
	if s, ok := stringFrom(inputs, "query"); ok {
		return strings.TrimSpace(s), nil
	}
	ref := strings.TrimSpace(configured)
	if ref == "" || ref == "sys.query" {
		if v := resolveCanvasRef(ctx, "sys.query"); v != nil {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s), nil
			}
		}
		return "", nil
	}
	query, err := resolveScoredQuery(ctx, configured, inputs)
	if err != nil {
		if pe, ok := err.(*ParamError); ok && pe.Field == "query" {
			return "", nil
		}
		return "", err
	}
	return query, nil
}

func fieldsEndpoint(baseURL string) (string, error) {
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return "", &ParamError{Field: "base_url", Reason: "the selected model has no API endpoint"}
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", &ParamError{Field: "base_url", Reason: "must be an absolute http(s) URL"}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", &ParamError{Field: "base_url", Reason: "must be http or https"}
	}
	parsed.Path = strings.TrimRight(stripSuffixFold(parsed.Path, "/chat/completions"), "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func buildFieldsCall(in scoringCallInput) (scoringCall, error) {
	endpoint, err := fieldsEndpoint(in.BaseURL)
	if err != nil {
		return scoringCall{}, err
	}
	if strings.TrimSpace(in.Query) == "" && len(in.Images) == 0 {
		return scoringCall{}, &ParamError{Field: "query", Reason: "state is required unless images are sent"}
	}
	if len(in.Images) > maxFieldsImages {
		return scoringCall{}, &ParamError{Field: "image", Reason: "at most 4 images"}
	}
	if err := checkFieldsImageBudget(in.Images); err != nil {
		return scoringCall{}, err
	}
	fields := make(map[string]any, len(in.Tasks))
	for _, task := range in.Tasks {
		spec, err := fieldSpec(task)
		if err != nil {
			return scoringCall{}, err
		}
		fields[strings.TrimSpace(task.ID)] = spec
	}
	if len(fields) == 0 {
		return scoringCall{}, &ParamError{Field: "tasks", Reason: "at least one field is required"}
	}
	body := map[string]any{
		"model":  in.Model,
		"state":  in.Query,
		"fields": fields,
	}
	if len(in.Images) > 0 {
		images := make([]any, 0, len(in.Images))
		for _, img := range in.Images {
			if !strings.HasPrefix(strings.ToLower(img.DataURL), "data:") {
				return scoringCall{}, &ParamError{Field: "image", Reason: "remote image URLs are not accepted; provide image bytes"}
			}
			images = append(images, img.DataURL)
		}
		body["images"] = images
	}
	if ref := strings.TrimSpace(in.ReferenceDate); ref != "" {
		if _, err := time.Parse("2006-01-02", ref); err != nil {
			return scoringCall{}, &ParamError{Field: "reference_date", Reason: "must be an ISO date"}
		}
		body["reference_date"] = ref
	}
	options := map[string]any{}
	if in.IncludeQuantiles {
		options["include_quantiles"] = true
	}
	if in.ReturnStateText {
		options["return_state_text"] = true
	}
	if len(options) > 0 {
		body["options"] = options
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return scoringCall{}, fmt.Errorf("encode scoring request: %w", err)
	}
	return scoringCall{Endpoint: endpoint, APIKey: in.APIKey, Body: encoded}, nil
}

func checkFieldsImageBudget(images []scoringImage) error {
	total := 0
	for _, img := range images {
		if len(img.Bytes) > maxFieldsImageBytes {
			return &ParamError{Field: "image", Reason: "an image exceeds 4 MiB"}
		}
		total += len(img.Bytes)
	}
	if total > maxFieldsImagesBytes {
		return &ParamError{Field: "image", Reason: "images exceed 8 MiB combined"}
	}
	return nil
}

func fieldSpec(task scoringTask) (map[string]any, error) {
	name := strings.TrimSpace(task.ID)
	if !fieldNamePattern.MatchString(name) {
		return nil, &ParamError{Field: "tasks", Reason: "field name must start with a letter or underscore and stay within 64 characters"}
	}
	kind := strings.ToLower(strings.TrimSpace(task.Type))
	if !isFieldTask(kind) {
		return nil, &ParamError{Field: "tasks", Reason: "field type is not supported"}
	}
	if task.Decimals != nil && (*task.Decimals < 0 || *task.Decimals > 6) {
		return nil, &ParamError{Field: "tasks", Reason: "decimals must be from 0 to 6"}
	}
	spec := map[string]any{"type": kind}
	if instructions := strings.TrimSpace(task.Instructions); instructions != "" {
		spec["instructions"] = instructions
	}
	switch kind {
	case "text", "date", "time", "datetime":
	case "number":
		addMeasure(spec, task)
	case "duration":
		if unit := strings.ToLower(strings.TrimSpace(task.Unit)); unit != "" {
			switch unit {
			case "seconds", "minutes", "hours", "days", "weeks", "months", "years":
				spec["unit"] = unit
			default:
				return nil, &ParamError{Field: "tasks", Reason: "duration unit is not supported"}
			}
		}
		addIntegerAndDecimals(spec, task)
	case "list":
		items := strings.ToLower(strings.TrimSpace(task.Items))
		switch items {
		case "text", "number", "date":
			spec["items"] = items
		default:
			return nil, &ParamError{Field: "tasks", Reason: "a list field needs items of text, number, or date"}
		}
		if items == "number" {
			addIntegerAndDecimals(spec, task)
		}
	case "estimate", "forecast":
		if task.Min == nil || task.Max == nil {
			return nil, &ParamError{Field: "tasks", Reason: "an estimate needs a minimum and a maximum"}
		}
		spec["min"] = task.Min
		spec["max"] = task.Max
		if task.Step != nil {
			spec["step"] = task.Step
		}
		if scale := strings.ToLower(strings.TrimSpace(task.Scale)); scale != "" {
			switch scale {
			case "linear", "log":
				spec["scale"] = scale
			default:
				return nil, &ParamError{Field: "tasks", Reason: "scale must be linear or log"}
			}
		}
		if kind == "forecast" {
			if task.Horizon == nil || *task.Horizon < 1 || *task.Horizon > 64 {
				return nil, &ParamError{Field: "tasks", Reason: "a forecast needs a horizon from 1 to 64"}
			}
			interval := strings.ToLower(strings.TrimSpace(task.Interval))
			switch interval {
			case "step", "minute", "hour", "day", "week", "month", "quarter", "year":
				spec["interval"] = interval
			default:
				return nil, &ParamError{Field: "tasks", Reason: "a forecast needs an interval"}
			}
			spec["horizon"] = *task.Horizon
		}
	}
	if task.NullThreshold != nil {
		if *task.NullThreshold <= 0 || *task.NullThreshold > 1 {
			return nil, &ParamError{Field: "tasks", Reason: "null threshold must be greater than 0 and at most 1"}
		}
		spec["null_threshold"] = *task.NullThreshold
	}
	return spec, nil
}

func addMeasure(spec map[string]any, task scoringTask) {
	if unit := strings.TrimSpace(task.Unit); unit != "" {
		spec["unit"] = unit
	}
	addIntegerAndDecimals(spec, task)
}

func addIntegerAndDecimals(spec map[string]any, task scoringTask) {
	if task.Integer != nil {
		spec["integer"] = *task.Integer
	}
	if task.Decimals != nil && *task.Decimals >= 0 && *task.Decimals <= 6 {
		spec["decimals"] = *task.Decimals
	}
}

func parseFieldsAnswers(body []byte) (map[string]any, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return nil, fmt.Errorf("payload is not a JSON object")
	}
	if _, ok := root["answers"].(map[string]any); !ok {
		return nil, fmt.Errorf("field answers not found")
	}
	answers := parseTypedAnswers(body)
	if len(answers) == 0 {
		return nil, fmt.Errorf("field answers not found")
	}
	return answers, nil
}

func parseScoringUsage(body []byte) map[string]any {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return nil
	}
	usage, ok := root["usage"].(map[string]any)
	if !ok || usage == nil {
		return nil
	}
	return usage
}

func parseStateText(body []byte) string {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return ""
	}
	text, _ := root["state_text"].(string)
	return text
}

func intFromJSON(v any) (int, bool) {
	n, ok := floatFromJSON(v)
	if !ok || n != float64(int(n)) {
		return 0, false
	}
	return int(n), true
}

func boundFromJSON(v any) (any, bool) {
	switch typed := v.(type) {
	case nil:
		return nil, false
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, false
		}
		if n, ok := floatFromJSON(trimmed); ok {
			return n, true
		}
		return trimmed, true
	case float64, int, int64, json.Number:
		n, ok := floatFromJSON(typed)
		if !ok {
			return nil, false
		}
		return n, true
	default:
		return nil, false
	}
}
