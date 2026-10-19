package component

import (
	"context"
	"encoding/json"
	"strings"

	"ragflow/internal/dao"

	"gorm.io/gorm"
)

type scoringTask struct {
	ID           string
	Type         string
	Instructions string
	TrueText     string
	FalseText    string
	Levels       []string
}

type scoringCapabilities struct {
	Known bool
	Image bool
	Box   bool
	Noul  bool
	Score bool
}

func taskQuestion(task scoringTask) (map[string]any, error) {
	instructions := strings.TrimSpace(task.Instructions)
	if instructions == "" {
		return nil, &ParamError{Field: "tasks", Reason: "every extra question needs instructions"}
	}
	switch task.Type {
	case "noul":
		question := map[string]any{
			"type":         "noul",
			"instructions": instructions,
		}
		if task.TrueText != "" || task.FalseText != "" {
			question["criteria"] = map[string]any{
				"true":  emptyAsNil(task.TrueText),
				"false": emptyAsNil(task.FalseText),
			}
		}
		return question, nil
	case "score":
		if len(task.Levels) < 2 {
			return nil, &ParamError{Field: "tasks", Reason: "an ordered level question needs at least two levels"}
		}
		return map[string]any{
			"type":         "score",
			"instructions": instructions,
			"criteria":     task.Levels,
		}, nil
	case "box":
		return map[string]any{
			"type":         "box",
			"instructions": instructions,
		}, nil
	default:
		return nil, &ParamError{Field: "tasks", Reason: "question type must be noul, score, or box"}
	}
}

func emptyAsNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func tasksNeedImageHeader(tasks []scoringTask) bool {
	for _, task := range tasks {
		if task.Type == "box" {
			return true
		}
	}
	return false
}

func filterTasks(tasks []scoringTask, caps scoringCapabilities) []scoringTask {
	if !caps.Known {
		return tasks
	}
	out := make([]scoringTask, 0, len(tasks))
	for _, task := range tasks {
		switch task.Type {
		case "box":
			if caps.Box {
				out = append(out, task)
			}
		case "noul":
			if caps.Noul {
				out = append(out, task)
			}
		case "score":
			if caps.Score {
				out = append(out, task)
			}
		}
	}
	return out
}

func loadScoringCapabilities(ctx context.Context, db *gorm.DB, modelRef string) scoringCapabilities {
	if db == nil || !isBareTenantModelID(modelRef) {
		return scoringCapabilities{}
	}
	model, err := dao.NewTenantModelDAO().GetByID(ctx, db, modelRef)
	if err != nil || model == nil {
		return scoringCapabilities{}
	}
	extra := map[string]any{}
	if model.Extra != "" {
		_ = json.Unmarshal([]byte(model.Extra), &extra)
	}
	return scoringCapabilities{
		Known: true,
		Image: extraBool(extra["supports_image"]),
		Box:   extraBool(extra["supports_box"]),
		Noul:  extraBool(extra["supports_noul"]),
		Score: extraBool(extra["supports_score"]),
	}
}

func parseTypedAnswers(body []byte) map[string]any {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return map[string]any{}
	}
	if _, ok := root["answers"]; !ok {
		if nested := chatContentObject(root); nested != nil {
			root = nested
		}
	}
	answers, ok := root["answers"].(map[string]any)
	if !ok || answers == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(answers))
	for id, raw := range answers {
		obj, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := obj["type"].(string)
		switch strings.ToLower(strings.TrimSpace(kind)) {
		case "choice", "noul", "score", "box":
			out[id] = obj
		}
	}
	return out
}

func extraBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true") || typed == "1"
	case float64:
		return typed != 0
	default:
		return false
	}
}

func parseScoringTasks(raw any) ([]scoringTask, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, &ParamError{Field: "tasks", Reason: "must be a list"}
	}
	tasks := make([]scoringTask, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, &ParamError{Field: "tasks", Reason: "each question must be an object"}
		}
		id, _ := stringFrom(obj, "id")
		id = strings.TrimSpace(id)
		if id == "" {
			id, _ = stringFrom(obj, "name")
			id = strings.TrimSpace(id)
		}
		if id == "" || id == scoringQuestionID || id == scoredSwitchEscalateHandle {
			return nil, &ParamError{Field: "tasks", Reason: "each extra question needs its own id"}
		}
		if _, ok := seen[id]; ok {
			return nil, &ParamError{Field: "tasks", Reason: "question ids must be unique"}
		}
		seen[id] = struct{}{}
		kind, _ := stringFrom(obj, "type")
		kind = strings.ToLower(strings.TrimSpace(kind))
		instructions, _ := stringFrom(obj, "instructions")
		task := scoringTask{ID: id, Type: kind, Instructions: instructions}
		task.TrueText, _ = stringFrom(obj, "true_text")
		task.FalseText, _ = stringFrom(obj, "false_text")
		if levels, ok := obj["levels"].([]any); ok {
			for _, level := range levels {
				switch typed := level.(type) {
				case string:
					if strings.TrimSpace(typed) != "" {
						task.Levels = append(task.Levels, strings.TrimSpace(typed))
					}
				case map[string]any:
					name, _ := stringFrom(typed, "name")
					if strings.TrimSpace(name) != "" {
						task.Levels = append(task.Levels, strings.TrimSpace(name))
					}
				}
			}
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}
