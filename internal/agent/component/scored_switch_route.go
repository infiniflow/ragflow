// Package component — score parsing and selective routing for ScoredSwitch.
//
// The payload shape follows the public scoring-model contract checked
// against the JEV 1.4 specimen: one choice answer, a probability for
// every option, and an optional confidence that this router does not
// use. Routing compares those probabilities directly (no temperature).
// A thin top-1/top-2 margin, a low top score, or an exact tie escalates
// when escalation is enabled. When it is disabled, the highest score
// wins and an exact tie keeps the earliest option in the author's list.
package component

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	scoredSwitchEscalateHandle = "escalate"
	scoringQuestionID          = "route"
	defaultMarginThreshold     = 0.15
	defaultMinTopScore         = 0.0
	scoreSumTolerance          = 0.02
	scoreEpsilon               = 1e-12
	rawPayloadLimit            = 4096
)

type scoredOption struct {
	ID          string
	Label       string
	Description string
	To          []string
}

type routePolicy struct {
	EscalateEnabled bool
	MarginThreshold float64
	MinTopScore     float64
	EscalateTo      []string
}

type routeDecision struct {
	Escalated   bool
	ChosenID    string
	ChosenLabel string
	Handle      string
	Next        []string
	Margin      float64
	Scores      map[string]float64
}

type parsedScores struct {
	Probabilities map[string]float64
	Confidence    float64
	HasConfidence bool
}

func parseScoringPayload(body []byte) (parsedScores, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return parsedScores{}, errors.New("payload is not a JSON object")
	}
	obj, err := findChoiceObject(root, 0)
	if err != nil {
		return parsedScores{}, err
	}
	return scoresFromChoice(obj)
}

func findChoiceObject(root map[string]any, depth int) (map[string]any, error) {
	if depth > 3 || root == nil {
		return nil, errors.New("choice answer not found")
	}
	if answers, ok := root["answers"].(map[string]any); ok {
		if raw, ok := answers[scoringQuestionID]; ok {
			obj, err := asChoiceObject(raw)
			if err != nil {
				return nil, err
			}
			if obj != nil {
				return obj, nil
			}
		}
		var found []map[string]any
		for _, raw := range answers {
			obj, err := asChoiceObject(raw)
			if err != nil {
				return nil, err
			}
			if obj != nil {
				found = append(found, obj)
			}
		}
		switch len(found) {
		case 1:
			return found[0], nil
		case 0:
		default:
			return nil, errors.New("more than one choice answer")
		}
	}
	if obj, err := asChoiceObject(root); err != nil {
		return nil, err
	} else if obj != nil {
		return obj, nil
	}
	if nested := chatContentObject(root); nested != nil {
		return findChoiceObject(nested, depth+1)
	}
	return nil, errors.New("choice answer not found")
}

func asChoiceObject(raw any) (map[string]any, error) {
	obj, ok := raw.(map[string]any)
	if !ok || obj == nil {
		return nil, nil
	}
	kind, _ := obj["type"].(string)
	if !strings.EqualFold(strings.TrimSpace(kind), "choice") {
		return nil, nil
	}
	if _, ok := obj["probabilities"].(map[string]any); !ok {
		return nil, errors.New("choice answer has no probabilities")
	}
	return obj, nil
}

func chatContentObject(root map[string]any) map[string]any {
	choices, ok := root["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil
	}
	first, ok := choices[0].(map[string]any)
	if !ok {
		return nil
	}
	message, ok := first["message"].(map[string]any)
	if !ok {
		return nil
	}
	content, ok := message["content"].(string)
	if !ok {
		return nil
	}
	content = stripJSONFence(content)
	if content == "" {
		return nil
	}
	var nested map[string]any
	if err := json.Unmarshal([]byte(content), &nested); err != nil {
		return nil
	}
	return nested
}

func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "json")
	s = strings.TrimPrefix(s, "JSON")
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

func scoresFromChoice(obj map[string]any) (parsedScores, error) {
	raw, _ := obj["probabilities"].(map[string]any)
	if len(raw) == 0 {
		return parsedScores{}, errors.New("choice answer has no probabilities")
	}
	probs := make(map[string]float64, len(raw))
	for key, value := range raw {
		label := strings.TrimSpace(key)
		if label == "" {
			return parsedScores{}, errors.New("probability key is empty")
		}
		score, ok := floatFromJSON(value)
		if !ok || math.IsNaN(score) || math.IsInf(score, 0) {
			return parsedScores{}, errors.New("probability is not a finite number")
		}
		if score < 0 {
			return parsedScores{}, errors.New("probability is negative")
		}
		probs[label] = score
	}
	parsed := parsedScores{Probabilities: probs}
	if score, ok := floatFromJSON(obj["confidence"]); ok && !math.IsNaN(score) && !math.IsInf(score, 0) {
		parsed.Confidence = score
		parsed.HasConfidence = true
	}
	return parsed, nil
}

func floatFromJSON(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// alignAndNormalize maps model keys onto the configured options.
// Official probabilities (sum within scoreSumTolerance of 1) are kept.
// Any other non-negative mass is divided by its sum.
func alignAndNormalize(options []scoredOption, raw map[string]float64) (map[string]float64, error) {
	if len(raw) == 0 {
		return nil, errors.New("probabilities are empty")
	}
	byLabel := make(map[string]scoredOption, len(options))
	lower := make(map[string][]scoredOption, len(options))
	for _, opt := range options {
		byLabel[opt.Label] = opt
		key := strings.ToLower(opt.Label)
		lower[key] = append(lower[key], opt)
	}
	aligned := make(map[string]float64, len(options))
	for _, opt := range options {
		aligned[opt.Label] = 0
	}
	for key, score := range raw {
		if _, ok := byLabel[key]; ok {
			aligned[key] = score
			continue
		}
		matches := lower[strings.ToLower(key)]
		if len(matches) == 1 {
			aligned[matches[0].Label] = score
		}
	}
	sum := 0.0
	for _, opt := range options {
		sum += aligned[opt.Label]
	}
	if sum == 0 {
		return nil, errors.New("probabilities did not include the configured options")
	}
	if math.Abs(sum-1) > scoreSumTolerance {
		for _, opt := range options {
			aligned[opt.Label] = aligned[opt.Label] / sum
		}
	}
	return aligned, nil
}

func routeScoredOptions(options []scoredOption, scores map[string]float64, policy routePolicy) (routeDecision, error) {
	if len(options) < 2 {
		return routeDecision{}, &ParamError{Field: "options", Reason: "at least two options are required"}
	}
	rows := rankScoreRows(options, scores)
	top, second := rows[0], rows[1]
	margin := top.score - second.score
	if margin < 0 {
		margin = 0
	}
	tie := scoresEqual(top.score, second.score)
	outScores := make(map[string]float64, len(rows))
	for _, row := range rows {
		outScores[row.opt.Label] = row.score
	}
	decision := routeDecision{Margin: margin, Scores: outScores}
	// Strictly below, with an epsilon so 0.6-0.4 is not treated as under 0.2.
	if policy.EscalateEnabled && (tie || scoreBelow(margin, policy.MarginThreshold) || scoreBelow(top.score, policy.MinTopScore)) {
		if len(policy.EscalateTo) == 0 {
			return routeDecision{}, errors.New("escalate handle is not connected")
		}
		decision.Escalated = true
		decision.Handle = scoredSwitchEscalateHandle
		decision.Next = append([]string(nil), policy.EscalateTo...)
		return decision, nil
	}
	if len(top.opt.To) == 0 {
		return routeDecision{}, fmt.Errorf("option %q is not connected", top.opt.Label)
	}
	decision.ChosenID = top.opt.ID
	decision.ChosenLabel = top.opt.Label
	decision.Handle = top.opt.ID
	decision.Next = append([]string(nil), top.opt.To...)
	return decision, nil
}

type scoreRow struct {
	opt   scoredOption
	score float64
	index int
}

func rankScoreRows(options []scoredOption, scores map[string]float64) []scoreRow {
	rows := make([]scoreRow, len(options))
	for i, opt := range options {
		rows[i] = scoreRow{opt: opt, score: scores[opt.Label], index: i}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if scoresEqual(rows[i].score, rows[j].score) {
			return rows[i].index < rows[j].index
		}
		return rows[i].score > rows[j].score
	})
	return rows
}

func scoresEqual(a, b float64) bool {
	return math.Abs(a-b) <= scoreEpsilon
}

func scoreBelow(a, b float64) bool {
	return a < b && !scoresEqual(a, b)
}

func truncateUTF8(b []byte, limit int) string {
	if len(b) <= limit {
		return string(b)
	}
	cut := limit
	for cut > 0 && !utf8.Valid(b[:cut]) {
		cut--
	}
	return string(b[:cut]) + "\n…(truncated)"
}

func redactSecret(s, secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "[redacted]")
}
