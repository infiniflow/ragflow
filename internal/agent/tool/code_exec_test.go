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
	"strings"
	"testing"
)

func TestCodeExec_StubsErrorWhenClientMissing(t *testing.T) {
	t.Parallel()

	c := NewCodeExecTool()
	out, err := c.InvokableRun(context.Background(), `{"language":"python","code":"def main(): return {}"}`)
	if !errors.Is(err, ErrCodeExecSandboxMissing) {
		t.Fatalf("err = %v, want ErrCodeExecSandboxMissing", err)
	}

	var got codeExecResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if !got.Stub {
		t.Errorf("Stub = false, want true")
	}
	if !strings.Contains(got.Error, "sandbox") {
		t.Errorf("Error = %q, want to mention 'sandbox'", got.Error)
	}
}

func TestCodeExec_RejectsEmptyCode(t *testing.T) {
	t.Parallel()

	c := NewCodeExecTool()
	_, err := c.InvokableRun(context.Background(), `{"language":"python","code":""}`)
	if err == nil || !strings.Contains(err.Error(), "code") {
		t.Fatalf("err = %v, want to mention empty code", err)
	}
}

func TestCodeExec_RejectsBadLanguage(t *testing.T) {
	t.Parallel()

	c := NewCodeExecTool()
	_, err := c.InvokableRun(context.Background(), `{"language":"brainfuck","code":"x"}`)
	if err == nil || !strings.Contains(err.Error(), "language") {
		t.Fatalf("err = %v, want to reject unsupported language", err)
	}
}

func TestCodeExec_AcceptsLangAlias(t *testing.T) {
	t.Parallel()

	c := NewCodeExecTool()
	// Python tool also accepts "lang" as the field name; the Go shell
	// should still reach the stub branch.
	_, err := c.InvokableRun(context.Background(), `{"lang":"nodejs","script":"async function main() {}"}`)
	if !errors.Is(err, ErrCodeExecSandboxMissing) {
		t.Fatalf("err = %v, want ErrCodeExecSandboxMissing", err)
	}
}

func TestCodeExec_Info(t *testing.T) {
	t.Parallel()

	c := NewCodeExecTool()
	info, err := c.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "execute_code" {
		t.Errorf("Name = %q, want execute_code", info.Name)
	}
	if !strings.Contains(info.Desc, "Python") {
		t.Errorf("Desc = %q, want to mention Python", info.Desc)
	}
}

// TestCodeExec_ResultExtractsArtifacts pins the artifact
// collection: SandboxResponse.Metadata["artifacts"] must be
// surfaced as `_ARTIFACTS` in the tool's JSON envelope so the
// Message
// component's artifact markdown formatter can render them.
func TestCodeExec_ResultExtractsArtifacts(t *testing.T) {
	t.Parallel()

	resp := &SandboxResponse{
		Returned: "ok",
		ExitCode: 0,
		Metadata: map[string]any{
			"artifacts": []any{
				map[string]any{"name": "chart.png", "url": "minio://b/chart.png"},
				map[string]any{"name": "data.csv", "url": "minio://b/data.csv"},
			},
		},
	}
	out, err := codeExecResultJSON(resp)
	if err != nil {
		t.Fatalf("codeExecResultJSON: %v", err)
	}
	var got codeExecResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output not valid JSON: %v (raw=%s)", jerr, out)
	}
	if len(got.Artifacts) != 2 {
		t.Fatalf("Artifacts len = %d, want 2", len(got.Artifacts))
	}
	if got.Artifacts[0]["name"] != "chart.png" {
		t.Errorf("Artifacts[0][name] = %v, want chart.png", got.Artifacts[0]["name"])
	}
}

// TestCodeExec_ResultDropsBadArtifactShape ensures the extractor
// silently drops entries that aren't map[string]any rather than
// aborting the run.
func TestCodeExec_ResultDropsBadArtifactShape(t *testing.T) {
	t.Parallel()

	resp := &SandboxResponse{
		Returned: "ok",
		Metadata: map[string]any{
			"artifacts": []any{
				"just a string",                  // bad shape
				map[string]any{"name": "ok.png"}, // good
				42,                               // bad shape
			},
		},
	}
	out, err := codeExecResultJSON(resp)
	if err != nil {
		t.Fatalf("codeExecResultJSON: %v", err)
	}
	var got codeExecResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output not valid JSON: %v", jerr)
	}
	if len(got.Artifacts) != 1 {
		t.Errorf("Artifacts len = %d, want 1 (bad shapes dropped)", len(got.Artifacts))
	}
	if got.Artifacts[0]["name"] != "ok.png" {
		t.Errorf("Artifacts[0][name] = %v, want ok.png", got.Artifacts[0]["name"])
	}
}

// TestCodeExec_ResultExtractsAttachments pins the attachments
// (rendered to downstream Message markdown) path. Distinct from
// artifacts so renderers can route them differently.
func TestCodeExec_ResultExtractsAttachments(t *testing.T) {
	t.Parallel()

	resp := &SandboxResponse{
		Returned: "ok",
		Metadata: map[string]any{
			"attachments": []any{
				map[string]any{"name": "report.pdf", "url": "minio://b/report.pdf"},
			},
		},
	}
	out, err := codeExecResultJSON(resp)
	if err != nil {
		t.Fatalf("codeExecResultJSON: %v", err)
	}
	var got codeExecResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output not valid JSON: %v", jerr)
	}
	if len(got.Attachments) != 1 {
		t.Fatalf("Attachments len = %d, want 1", len(got.Attachments))
	}
}

// TestCodeExec_ResultSurfacesActualType pins the actual_type
// surface used by Message component to render the right Markdown
// formatting (Number → <code>, Object → JSON dump, etc.).
func TestCodeExec_ResultSurfacesActualType(t *testing.T) {
	t.Parallel()

	resp := &SandboxResponse{
		Returned:         `{"x": 1}`,
		StructuredResult: map[string]any{"actual_type": "Object"},
	}
	out, err := codeExecResultJSON(resp)
	if err != nil {
		t.Fatalf("codeExecResultJSON: %v", err)
	}
	var got codeExecResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output not valid JSON: %v", jerr)
	}
	if got.ActualType != "Object" {
		t.Errorf("ActualType = %q, want Object", got.ActualType)
	}
	if got.Content != `{"x": 1}` {
		t.Errorf("Content = %q, want %q", got.Content, `{"x": 1}`)
	}
}

// TestCodeExec_PassesTimeoutToSandbox verifies the new
// `timeout` arg flows into the SandboxRequest.Timeout field so
// the model can dial per-script budgets. Note: this test
// mutates the global sandbox client; it must NOT run in
// parallel with the other CodeExec tests that depend on the
// default (loud-fail) stub.
func TestCodeExec_PassesTimeoutToSandbox(t *testing.T) {
	var captured SandboxRequest
	prev := GetSandboxClient()
	SetSandboxClient(stubSandbox(func(_ context.Context, req SandboxRequest) (*SandboxResponse, error) {
		captured = req
		return &SandboxResponse{Returned: "ok", ExitCode: 0}, nil
	}))
	t.Cleanup(func() { SetSandboxClient(prev) })

	c := NewCodeExecTool()
	_, err := c.InvokableRun(context.Background(),
		`{"language":"python","code":"def main(): return {}","timeout":42}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if captured.Timeout != 42 {
		t.Errorf("SandboxRequest.Timeout = %d, want 42", captured.Timeout)
	}
}

// TestCodeExec_PassesArgumentsToSandbox verifies the `arguments`
// arg (Python `**kwargs` to main()) is propagated. Like the
// timeout test, this mutates the global sandbox client and must
// not run in parallel with sibling CodeExec tests.
func TestCodeExec_PassesArgumentsToSandbox(t *testing.T) {
	var captured SandboxRequest
	prev := GetSandboxClient()
	SetSandboxClient(stubSandbox(func(_ context.Context, req SandboxRequest) (*SandboxResponse, error) {
		captured = req
		return &SandboxResponse{Returned: "ok", ExitCode: 0}, nil
	}))
	t.Cleanup(func() { SetSandboxClient(prev) })

	c := NewCodeExecTool()
	_, err := c.InvokableRun(context.Background(),
		`{"language":"python","code":"def main(**kw): return kw","arguments":{"x":1,"y":"z"}}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if captured.Arguments["x"].(float64) != 1 || captured.Arguments["y"].(string) != "z" {
		t.Errorf("Arguments = %v, want {x:1, y:z}", captured.Arguments)
	}
}

// stubSandbox adapts a function literal to the SandboxClient
// interface so the timeout / arguments tests can capture the
// request without depending on the default stub.
type stubSandbox func(ctx context.Context, req SandboxRequest) (*SandboxResponse, error)

func (s stubSandbox) ExecuteCode(ctx context.Context, req SandboxRequest) (*SandboxResponse, error) {
	return s(ctx, req)
}
