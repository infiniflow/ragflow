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
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ErrCodeExecSandboxMissing is returned when the Phase 5 gRPC client to the
// Python sandbox (agent.sandbox.client.execute_code) is not yet wired. The
// Python sandbox itself is kept as-is per plan §2.11.4 (do NOT rewrite the
// sandbox); Phase 3 batch 1 only ships the tool shell, and Phase 5 adds
// the gRPC client.
var ErrCodeExecSandboxMissing = errors.New(
	"CodeExec sandbox gRPC client not yet implemented in Go — " +
		"defer to Python Canvas",
)

const codeExecToolName = "execute_code"

const codeExecToolDescription = "This tool has a sandbox that can execute code written in 'Python'/'Javascript'. " +
	"It receives a piece of code and returns a JSON string."

// codeExecArgs is the JSON shape the model sends in. The Python tool
// accepts "lang" + "script" (see agent/tools/code_exec.py:303-310); we
// also accept "code" as a synonym since some DSLs and Phase 3 batch 1
// tests use that spelling.
type codeExecArgs struct {
	Language string         `json:"language,omitempty"`
	Lang     string         `json:"lang,omitempty"`
	Script   string         `json:"script,omitempty"`
	Code     string         `json:"code,omitempty"`
	Args     map[string]any `json:"arguments,omitempty"`
}

// codeExecResult is the JSON envelope returned to the model. The output
// shape mirrors the Python tool's `content` / `_ERROR` / `actual_type`
// fields so downstream nodes can pattern-match unchanged.
type codeExecResult struct {
	Content    string `json:"content,omitempty"`
	ActualType string `json:"actual_type,omitempty"`
	Stub       bool   `json:"stub,omitempty"`
	Error      string `json:"_ERROR,omitempty"`
}

// CodeExecTool is the Phase 3 batch 1 shell for the CodeExec tool
// (plan §2.11.4 row 3, §5 Phase 3 第 1 批). It validates language +
// non-empty code and returns a structured "not-yet-wired" error.
type CodeExecTool struct{}

// NewCodeExecTool returns a CodeExecTool implementing eino's
// tool.InvokableTool interface.
func NewCodeExecTool() *CodeExecTool {
	return &CodeExecTool{}
}

// Info returns the tool's metadata for the chat model. The schema mirrors
// the Python CodeExecParam ToolMeta (plan §5 Phase 3, 字段对齐).
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

// InvokableRun validates the inputs and returns the structured stub error.
// Phase 5 will replace the stub body with a gRPC call into the Python
// sandbox (per plan §2.11.4 "不重写沙箱" 决策).
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

	// Phase 3 batch 1: gRPC client wires in Phase 5.
	return codeExecStubResult(ErrCodeExecSandboxMissing.Error()),
		ErrCodeExecSandboxMissing
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
