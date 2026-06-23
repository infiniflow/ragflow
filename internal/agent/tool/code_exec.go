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

package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/common"
)

// ErrCodeExecSandboxMissing is returned when no sandbox client is
// registered. The Python sandbox itself is kept as-is (the Go
// side never reimplemented the sandbox). When a client is
// registered via SetSandboxClient at boot, the tool dispatches
// the execution.
var ErrCodeExecSandboxMissing = errors.New(
	"CodeExec sandbox client not registered — call SetSandboxClient at boot",
)

const codeExecToolName = "execute_code"

const codeExecToolDescription = "This tool has a sandbox that can execute code written in 'Python'/'Javascript'. " +
	"It receives a piece of code and returns a JSON string."

// codeExecArgs is the JSON shape the model sends in. The Python
// tool accepts "lang" + "script"; we also accept "code" as a
// synonym since some DSLs and tests use that spelling.
type codeExecArgs struct {
	Language string         `json:"language,omitempty"`
	Lang     string         `json:"lang,omitempty"`
	Script   string         `json:"script,omitempty"`
	Code     string         `json:"code,omitempty"`
	Args     map[string]any `json:"arguments,omitempty"`
	// Timeout is the per-execution wall-clock budget in seconds. 0
	// (the default) defers to the sandbox provider's own default
	// (typically 30s). Mirrors Python's
	// `code_exec.py:358 timeout_seconds = int(os.environ.get(...))`
	// but the value flows per-call rather than per-process, which
	// lets the model dial up/down for known-fast vs. known-slow
	// scripts.
	Timeout int `json:"timeout,omitempty"`
}

// codeExecResult is the JSON envelope returned to the model. The output
// shape mirrors the Python tool's `content` / `_ERROR` / `actual_type`
// fields so downstream nodes can pattern-match unchanged. Artifacts and
// Attachments are surfaced for the model and downstream component
// consumption (e.g. Message component's artifact markdown formatter).
type codeExecResult struct {
	Content     string           `json:"content,omitempty"`
	ActualType  string           `json:"actual_type,omitempty"`
	RawResult   any              `json:"raw_result,omitempty"`
	Stub        bool             `json:"stub,omitempty"`
	Error       string           `json:"_ERROR,omitempty"`
	ExitCode    int              `json:"exit_code,omitempty"`
	Stdout      string           `json:"stdout,omitempty"`
	Stderr      string           `json:"stderr,omitempty"`
	Artifacts   []map[string]any `json:"_ARTIFACTS,omitempty"`
	Attachments []map[string]any `json:"attachments,omitempty"`
}

// CodeExecTool is the  for the CodeExec tool
// ( . It validates language +
// non-empty code and returns a structured "not-yet-wired" error.
type CodeExecTool struct{}

// NewCodeExecTool returns a CodeExecTool implementing eino's
// tool.InvokableTool interface.
func NewCodeExecTool() *CodeExecTool {
	return &CodeExecTool{}
}

// Info returns the tool's metadata for the chat model. The schema mirrors
// the Python CodeExecParam ToolMeta (plan , 字段对齐).
func (c *CodeExecTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: codeExecToolName,
		Desc: codeExecToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"language": {
				Type:     schema.String,
				Desc:     "The programming language of the code. Allowed: 'python' (or 'python3'), 'javascript' (or 'nodejs').",
				Enum:     []string{"python", "python3", "javascript", "nodejs"},
				Required: true,
			},
			"code": {
				Type:     schema.String,
				Desc:     "The code to execute. Must define a `main` function (Python) or export `main` (JavaScript).",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun validates the inputs and dispatches to the
// registered sandbox client via SetSandboxClient.
func (c *CodeExecTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args codeExecArgs
	if argumentsInJSON == "" {
		return codeExecStubResult("arguments are required"), errors.New("code_exec: empty arguments")
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return codeExecStubResult("invalid JSON: " + err.Error()),
			fmt.Errorf("code_exec: parse arguments: %w", err)
	}

	lang := normalizeCodeExecLang(args.Language, args.Lang)
	if lang == "" {
		return codeExecStubResult("unsupported language: must be 'python'/'python3'/'javascript'/'nodejs'"),
			errors.New("code_exec: invalid language")
	}

	script := args.Script
	if script == "" {
		script = args.Code
	}
	if strings.TrimSpace(script) == "" {
		return codeExecStubResult("code is required"), errors.New("code_exec: empty code")
	}

	// Dispatch to the registered SandboxClient. When the default
	// stub is in place, the call surfaces
	// ErrCodeExecSandboxMissing; once a real client is
	// installed via SetSandboxClient at boot, the script runs.
	client := GetSandboxClient()
	req := SandboxRequest{
		Lang:      lang,
		Script:    script,
		Arguments: args.Args,
		Timeout:   args.Timeout,
	}
	common.Debug("CodeExec tool invoke",
		zap.String("lang", req.Lang),
		zap.Int("timeout", req.Timeout),
		zap.Int("arguments_keys", len(req.Arguments)),
		zap.Int("script_len", len(req.Script)))
	resp, err := client.ExecuteCode(ctx, req)
	if err != nil {
		return codeExecStubResult(err.Error()), err
	}
	out, mErr := codeExecResultJSON(resp)
	if mErr != nil {
		return codeExecStubResult(mErr.Error()), mErr
	}
	return out, nil
}

// codeExecResultJSON serializes a SandboxResponse into the envelope
// the eino tool contract returns. Field mapping mirrors the Python
// tool's `code_exec.py:385-490` `_process_execution_result`:
//
//   - Stdout / Stderr / ExitCode: stream directly through.
//   - Returned → Content (the model's natural "what did main() give
//     us back" field).
//   - StructuredResult["actual_type"] → ActualType (Python
//     `infer_actual_type` surface for downstream Message component).
//   - Metadata["artifacts"] → Artifacts (the model AND the Message
//     component's `_ARTIFACTS` collector both consume this; we
//     surface it as `_ARTIFACTS` to match the Python envelope).
//   - Metadata["attachments"] → Attachments (rendered into
//     downstream Markdown by Message via the same path the Agent
//     tool artifact markdown uses).
//
// Artifacts / Attachments with the wrong element type (anything
// other than map[string]any) are silently dropped with a log
// warning. This matches the Python tool's "skip on shape mismatch"
// semantics — better to lose one artifact than to abort the run.
func codeExecResultJSON(r *SandboxResponse) (string, error) {
	if r == nil {
		return codeExecStubResult("empty response"), nil
	}
	out := codeExecResult{
		ExitCode: r.ExitCode,
		Stdout:   r.Stdout,
		Stderr:   r.Stderr,
	}
	if r.Metadata != nil {
		out.Artifacts = extractArtifactList(r.Metadata, "artifacts")
		out.Attachments = extractArtifactList(r.Metadata, "attachments")
	}
	hasStructuredResult := false
	resolvedValue, usedStdoutFallback := resolveCodeExecResultValue(r)
	if r.StructuredResult != nil {
		hasStructuredResult, _ = r.StructuredResult["present"].(bool)
	}
	if strings.TrimSpace(r.Stderr) != "" &&
		!hasStructuredResult &&
		len(out.Artifacts) == 0 &&
		strings.TrimSpace(r.Stdout) == "" {
		out.Error = r.Stderr
	} else {
		if usedStdoutFallback && strings.TrimSpace(r.Stdout) != "" {
			fmt.Fprintln(os.Stderr, "code_exec: falling back to stdout deserialization because no structured result metadata was provided")
		}
		out.RawResult = NormalizeCodeExecOutputValue(resolvedValue)
		out.ActualType = InferCodeExecActualType(out.RawResult)
		out.Content = RenderCodeExecCanonicalContent(out.RawResult)
	}
	common.Debug("CodeExec tool",
		zap.Any("structured_result", r.StructuredResult),
		zap.Any("resolved_value", resolvedValue),
		zap.Any("raw_result", out.RawResult),
		zap.String("content", out.Content),
		zap.String("actual_type", out.ActualType),
		zap.Bool("stderr_present", r.Stderr != ""),
		zap.Int("stderr_len", len(r.Stderr)),
		zap.Int("stdout_len", len(r.Stdout)))
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("code_exec: marshal result: %w", err)
	}
	return string(b), nil
}

// extractArtifactList pulls a list of dict-shaped entries out of
// Metadata[key]. Items that aren't map[string]any are dropped with
// a stderr log line so the operator can see the data loss without
// the run aborting.
func extractArtifactList(meta map[string]any, key string) []map[string]any {
	raw, ok := meta[key]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			fmt.Fprintf(os.Stderr, "code_exec: %s[%d] is %T, expected map[string]any; dropping\n", key, i, item)
			continue
		}
		out = append(out, m)
	}
	return out
}

func codeExecStubResult(msg string) string {
	b, err := json.Marshal(codeExecResult{
		Stub:  true,
		Error: msg,
	})
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"code_exec: marshal stub: %s","stub":true}`, err)
	}
	return string(b)
}

func resolveCodeExecResultValue(r *SandboxResponse) (any, bool) {
	if r != nil && r.StructuredResult != nil {
		if present, _ := r.StructuredResult["present"].(bool); present {
			return r.StructuredResult["value"], false
		}
	}
	return deserializeCodeExecStdout(r.Stdout), true
}

func deserializeCodeExecStdout(stdout string) any {
	text := strings.TrimSpace(stdout)
	if text == "" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err == nil {
		return decoded
	}
	return text
}

// normalizeCodeExecLang accepts the model's literal "language" or the
// Python-style "lang" alias and maps synonyms to the canonical "python" /
// "nodejs" forms used by the Python sandbox.
func normalizeCodeExecLang(primary, alias string) string {
	v := strings.ToLower(strings.TrimSpace(primary))
	if v == "" {
		v = strings.ToLower(strings.TrimSpace(alias))
	}
	switch v {
	case "python", "python3":
		return "python"
	case "javascript", "js", "nodejs", "node":
		return "nodejs"
	}
	return ""
}
