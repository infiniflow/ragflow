//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package component

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	einotool "github.com/cloudwego/eino/components/tool"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/utility"
)

const maxToolLogResultRunes = 4096
const maxToolLogInputRunes = 8192
const maxToolLogInvocations = 64

var sensitiveToolLogText = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|refresh[_-]?token|auth[_-]?token|authorization|auth|token|password|secret)(["']?\s*[:=]\s*)("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|(?:bearer|basic)\s+[^\s,}]+|[^"'\s,}]+)`)
var bearerToolLogText = regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[^\s"',}]+`)

type toolInvocationRecorder struct {
	mu      sync.Mutex
	entries []map[string]any
}

func (r *toolInvocationRecorder) snapshot() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries := append([]map[string]any(nil), r.entries...)
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i]["started_at"].(int64) < entries[j]["started_at"].(int64)
	})
	return entries
}

type toolInvocationRecordingTool struct {
	einotool.InvokableTool
	name     string
	recorder *toolInvocationRecorder
}

func (t *toolInvocationRecordingTool) InvokableRun(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error) {
	started := time.Now()
	result, err := t.InvokableTool.InvokableRun(ctx, argsJSON, opts...)
	finished := time.Now()
	args := map[string]any{}
	if json.Unmarshal([]byte(argsJSON), &args) != nil {
		args = map[string]any{"input": "[UNPARSEABLE INPUT]"}
	}
	status := "success"
	if err != nil {
		status = "error"
	} else {
		var envelope map[string]any
		if json.Unmarshal([]byte(result), &envelope) == nil && envelope["_ERROR"] != nil {
			status = "error"
		}
	}
	logResult := result
	if err != nil {
		logResult = err.Error()
	}
	logResult = redactToolLogText(logResult)
	redactedArgs := redactToolLogValue(args)
	if encoded, marshalErr := json.Marshal(redactedArgs); marshalErr == nil && len(encoded) > maxToolLogInputRunes {
		redactedArgs = map[string]any{"_truncated": true, "preview": utility.TruncateRunes(string(encoded), maxToolLogInputRunes)}
	}
	entry := map[string]any{
		"tool_name":    t.name,
		"arguments":    redactedArgs,
		"result":       utility.TruncateRunes(logResult, maxToolLogResultRunes),
		"started_at":   started.UnixMilli(),
		"elapsed_time": finished.Sub(started).Seconds(),
		"status":       status,
	}
	if err != nil {
		entry["error"] = utility.TruncateRunes(redactToolLogText(err.Error()), maxToolLogResultRunes)
	}
	t.recorder.mu.Lock()
	if len(t.recorder.entries) < maxToolLogInvocations {
		t.recorder.entries = append(t.recorder.entries, entry)
	}
	t.recorder.mu.Unlock()
	return result, err
}

func wrapToolInvocationRecording(tools []einotool.BaseTool, recorder *toolInvocationRecorder) []einotool.BaseTool {
	wrapped := append([]einotool.BaseTool(nil), tools...)
	for i, base := range tools {
		invokable, ok := base.(einotool.InvokableTool)
		if !ok {
			continue
		}
		info, err := base.Info(context.Background())
		if err != nil || info == nil {
			continue
		}
		wrapped[i] = &toolInvocationRecordingTool{InvokableTool: invokable, name: info.Name, recorder: recorder}
	}
	return wrapped
}

func redactToolLogValue(value any) any {
	switch v := value.(type) {
	case string:
		return redactToolLogText(v)
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			lower := strings.ToLower(key)
			normalized := strings.NewReplacer("_", "", "-", "").Replace(lower)
			if strings.Contains(normalized, "apikey") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || lower == "auth" || lower == "authorization" {
				out[key] = "[REDACTED]"
			} else {
				out[key] = redactToolLogValue(child)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = redactToolLogValue(v[i])
		}
		return out
	default:
		return value
	}
}

func redactToolLogText(value string) string {
	var structured any
	if json.Unmarshal([]byte(value), &structured) == nil {
		if redacted, err := json.Marshal(redactToolLogValue(structured)); err == nil {
			return string(redacted)
		}
	}
	value = sensitiveToolLogText.ReplaceAllStringFunc(value, func(match string) string {
		parts := sensitiveToolLogText.FindStringSubmatch(match)
		redacted := parts[1] + parts[2]
		credential := parts[3]
		if len(credential) >= 2 && credential[0] == credential[len(credential)-1] && (credential[0] == '"' || credential[0] == '\'') {
			return redacted + string(credential[0]) + "[REDACTED]" + string(credential[0])
		}
		return redacted + "[REDACTED]"
	})
	return bearerToolLogText.ReplaceAllString(value, "[REDACTED]")
}

// referenceRecordingTool decorates an Agent tool so each invocation records
// the retrieval references it produced into the canvas state. Canvas tool
// nodes already do this in ToolBackedComponent.Invoke; tools called from the
// Agent's ReAct loop go straight to InvokableRun, so without this wrapper
// state.Retrieval stays empty and citation grounding has no sources.
type referenceRecordingTool struct {
	einotool.InvokableTool
	builder agenttool.ReferenceBuilder
}

func (t *referenceRecordingTool) InvokableRun(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error) {
	out, err := t.InvokableTool.InvokableRun(ctx, argsJSON, opts...)
	if err != nil {
		return out, err
	}
	envelope := parseToolEnvelope(out)
	if _, invalid := envelope["_raw"]; invalid {
		return out, nil
	}
	chunks, docAggs := t.builder.BuildReferences(ctx, envelope)
	if state, stateErr := runtime.GetStateFromContext(ctx); stateErr == nil && state != nil {
		state.SetRetrievalReferences(chunks, docAggs)
	}
	return out, nil
}

// wrapReferenceRecordingTools decorates the tools handed to the ReAct loop.
// buildAgentTools keeps returning the raw tools so metadata paths such as
// AgentComponent.GetInputForm and Reset still see the original methods.
func wrapReferenceRecordingTools(tools []einotool.BaseTool) []einotool.BaseTool {
	wrapped := make([]einotool.BaseTool, len(tools))
	copy(wrapped, tools)
	for i, t := range tools {
		builder, buildsReferences := t.(agenttool.ReferenceBuilder)
		invokable, isInvokable := t.(einotool.InvokableTool)
		if buildsReferences && isInvokable {
			wrapped[i] = &referenceRecordingTool{InvokableTool: invokable, builder: builder}
		}
	}
	return wrapped
}
