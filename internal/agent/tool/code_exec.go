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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/storage"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"go.uber.org/zap"
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
	"It receives a piece of code and returns a JSON string. " +
	"The code must define a main function (Python) or export main (JavaScript); " +
	"the return value of main is returned as the tool result. " +
	"To generate charts or files (images, PDFs, CSVs, etc.), save them to the `artifacts/` " +
	"directory (relative to the working directory); the sandbox automatically collects those " +
	"files and returns them as artifacts. " +
	"Example: `plt.savefig('artifacts/chart.png', dpi=150, bbox_inches='tight')`. " +
	"Supported artifact file types: .png, .jpg, .jpeg, .svg, .pdf, .csv, .json, .html."

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
// consumption (e.g. Message component's artifact Markdown formatter).
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
// the Python CodeExecParam ToolMeta (plan, field alignment).
func (c *CodeExecTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: codeExecToolName,
		Desc: codeExecToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"lang": {
				Type:     schema.String,
				Desc:     "The programming language of this piece of code.",
				Enum:     []string{"python", "javascript"},
				Required: true,
			},
			"script": {
				Type:     schema.String,
				Desc:     "A piece of code in the correct format. It must define main(...).",
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
		// Providers return user-code failures as non-zero SandboxResponses.
		// An error here is a sandbox or transport failure and must stop the
		// ReAct loop instead of prompting retries against broken infrastructure.
		return codeExecStubResult(err.Error()), err
	}
	out, mErr := codeExecResultJSON(ctx, resp)
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
//   - Metadata["artifacts"] → Artifacts, published to the sandbox
//     artifact bucket and exposed as `_ARTIFACTS` references
//     ({name, url, mime_type, size}); the blob payload stays out of
//     the model-visible envelope.
//   - Metadata["attachments"] → Attachments (rendered into
//     downstream Markdown by Message via the same path the Agent
//     tool artifact Markdown uses).
//
// Artifacts / Attachments with the wrong element type (anything
// other than map[string]any) are silently dropped with a log
// warning. This matches the Python tool's "skip on shape mismatch"
// semantics — better to lose one artifact than to abort the run.
func codeExecResultJSON(ctx context.Context, r *SandboxResponse) (string, error) {
	if r == nil {
		return codeExecStubResult("empty response"), nil
	}
	out := codeExecResult{
		ExitCode: r.ExitCode,
		Stdout:   r.Stdout,
		Stderr:   r.Stderr,
	}
	if r.Metadata != nil {
		out.Artifacts = publishSandboxArtifacts(ctx, extractArtifactList(r.Metadata, "artifacts"))
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

// publishSandboxArtifacts turns sandbox artifact descriptors into
// hosted references: each blob is uploaded to the sandbox artifact
// bucket and the entry keeps only {name, url, mime_type, size}, so
// raw base64 never enters the model-visible envelope or the final
// chat message. Entries that cannot be published are dropped — the
// /api/v1/documents/artifact route only serves uploaded objects.
func publishSandboxArtifacts(ctx context.Context, artifacts []map[string]any) []map[string]any {
	if len(artifacts) == 0 {
		return nil
	}
	published := make([]map[string]any, 0, len(artifacts))
	for _, art := range artifacts {
		if entry := publishSandboxArtifact(ctx, art); entry != nil {
			published = append(published, entry)
		}
	}
	if len(published) == 0 {
		return nil
	}
	return published
}

func publishSandboxArtifact(ctx context.Context, art map[string]any) map[string]any {
	name, _ := art["name"].(string)
	if name == "" {
		return nil
	}
	mime, _ := art["mime_type"].(string)
	if url, _ := art["url"].(string); url != "" {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(url)), "data:") {
			fmt.Fprintf(os.Stderr, "code_exec: artifact %q carries an inline data: url; dropping\n", name)
			return nil
		}
		return map[string]any{"name": name, "url": url, "mime_type": mime, "size": art["size"]}
	}
	contentB64, _ := art["content_b64"].(string)
	if contentB64 == "" {
		return nil
	}
	storageExt := sandboxArtifactStorageExt(name, mime)
	if storageExt == "" {
		fmt.Fprintf(os.Stderr, "code_exec: artifact %q (mime %q) has no servable file type; dropping\n", name, mime)
		return nil
	}
	blob, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "code_exec: artifact %q is not valid base64: %v; dropping\n", name, err)
		return nil
	}
	impl := storage.GetStorageFactory().GetStorage()
	if impl == nil {
		fmt.Fprintf(os.Stderr, "code_exec: storage not initialized; dropping artifact %q\n", name)
		return nil
	}
	storageName := uuid.NewString() + storageExt
	if err := impl.Put(ctx, common.SandboxArtifactBucket(), storageName, blob); err != nil {
		fmt.Fprintf(os.Stderr, "code_exec: upload artifact %q: %v; dropping\n", name, err)
		return nil
	}
	return map[string]any{
		"name":      name,
		"url":       "/api/v1/documents/artifact/" + storageName,
		"mime_type": mime,
		"size":      art["size"],
	}
}

// sandboxArtifactStorageExt picks the extension the artifact route
// serves: the descriptor's own extension when it is servable, else one
// derived from the MIME type.
func sandboxArtifactStorageExt(name, mime string) string {
	if ext := strings.ToLower(filepath.Ext(name)); ext != "" {
		if _, ok := common.SandboxArtifactContentTypes[ext]; ok {
			return ext
		}
	}
	m := strings.ToLower(strings.TrimSpace(mime))
	for ext, contentType := range common.SandboxArtifactContentTypes {
		if contentType == m {
			return ext
		}
	}
	return ""
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
	switch arr := raw.(type) {
	case []map[string]any:
		// The sandbox providers decode the JSON array into
		// []map[string]any (see collectArtifacts in local.go,
		// ssh.go and self_managed.go), so accept that shape
		// directly. Without this the tool envelope silently
		// loses every artifact the sandbox collected.
		return arr
	case []any:
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
	default:
		return nil
	}
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
	if r != nil && r.Returned != "" {
		return r.Returned, false
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
