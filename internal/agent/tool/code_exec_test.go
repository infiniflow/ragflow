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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/storage"
)

func TestCodeExec_StubsErrorWhenClientMissing(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	c := NewCodeExecTool()
	out, err := c.InvokableRun(ctx, `{"language":"python","code":"def main(): return {}"}`)
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
	ctx := t.Context()

	c := NewCodeExecTool()
	_, err := c.InvokableRun(ctx, `{"language":"python","code":""}`)
	if err == nil || !strings.Contains(err.Error(), "code") {
		t.Fatalf("err = %v, want to mention empty code", err)
	}
}

func TestCodeExec_RejectsBadLanguage(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	c := NewCodeExecTool()
	_, err := c.InvokableRun(ctx, `{"language":"brainfuck","code":"x"}`)
	if err == nil || !strings.Contains(err.Error(), "language") {
		t.Fatalf("err = %v, want to reject unsupported language", err)
	}
}

func TestCodeExec_AcceptsLangAlias(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	c := NewCodeExecTool()
	// Python tool also accepts "lang" as the field name; the Go shell
	// should still reach the stub branch.
	_, err := c.InvokableRun(ctx, `{"lang":"nodejs","script":"async function main() {}"}`)
	if !errors.Is(err, ErrCodeExecSandboxMissing) {
		t.Fatalf("err = %v, want ErrCodeExecSandboxMissing", err)
	}
}

func TestCodeExec_ReturnsSandboxFailureAsTerminalError(t *testing.T) {
	prev := GetSandboxClient()
	SetSandboxClient(stubSandbox(func(context.Context, SandboxRequest) (*SandboxResponse, error) {
		return nil, errors.New("provider unavailable")
	}))
	t.Cleanup(func() { SetSandboxClient(prev) })

	out, err := NewCodeExecTool().InvokableRun(t.Context(), `{"language":"python","code":"def main(): pass"}`)
	if err == nil || !strings.Contains(err.Error(), "provider unavailable") {
		t.Fatalf("InvokableRun error = %v, want provider unavailable", err)
	}
	var got codeExecResult
	if json.Unmarshal([]byte(out), &got) != nil || !strings.Contains(got.Error, "provider unavailable") {
		t.Fatalf("result = %s, want error envelope", out)
	}
}

func TestCodeExec_Info(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	c := NewCodeExecTool()
	info, err := c.Info(ctx)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "execute_code" {
		t.Errorf("Name = %q, want execute_code", info.Name)
	}
	if !strings.Contains(info.Desc, "Python") {
		t.Errorf("Desc = %q, want to mention Python", info.Desc)
	}

	params, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("Info schema: %v", err)
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal Info schema: %v", err)
	}
	var schema map[string]any
	if err = json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("decode Info schema: %v", err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("Info schema properties = %#v, want object", schema["properties"])
	}
	for _, name := range []string{"lang", "script"} {
		if _, ok = properties[name]; !ok {
			t.Errorf("Info schema missing %q", name)
		}
	}
	for _, name := range []string{"language", "code", "arguments", "outputs"} {
		if _, ok = properties[name]; ok {
			t.Errorf("Info schema unexpectedly exposes node field %q", name)
		}
	}
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("Info schema required = %#v, want array", schema["required"])
	}
	requiredFields := make(map[string]bool, len(required))
	for _, field := range required {
		if name, ok := field.(string); ok {
			requiredFields[name] = true
		}
	}
	if !requiredFields["lang"] || !requiredFields["script"] {
		t.Errorf("Info schema required = %#v, want lang and script", required)
	}
	langProp, ok := properties["lang"].(map[string]any)
	if !ok {
		t.Fatalf("lang property = %#v, want object", properties["lang"])
	}
	if typ, _ := langProp["type"].(string); typ != "string" {
		t.Errorf("lang.type = %q, want string", typ)
	}
	enum, ok := langProp["enum"].([]any)
	if !ok {
		t.Fatalf("lang.enum = %#v, want array", langProp["enum"])
	}
	gotEnum := make([]string, len(enum))
	for i, e := range enum {
		s, ok := e.(string)
		if !ok {
			t.Fatalf("lang.enum[%d] = %#v, want string", i, e)
		}
		gotEnum[i] = s
	}
	if len(gotEnum) != 2 || gotEnum[0] != "python" || gotEnum[1] != "javascript" {
		t.Errorf("lang.enum = %v, want [python javascript]", gotEnum)
	}
}

func TestCodeExecPublicFormattingHandlesTypedNilSlice(t *testing.T) {
	t.Parallel()

	var value []any
	if got := InferCodeExecActualType(value); got != "Array<Any>" {
		t.Fatalf("InferCodeExecActualType(typed nil) = %q, want Array<Any>", got)
	}
	if got := RenderCodeExecCanonicalContent(value); got != "[]" {
		t.Fatalf("RenderCodeExecCanonicalContent(typed nil) = %q, want []", got)
	}

	contract, err := BuildCodeExecContract(map[string]any{"result": nil}, value)
	if err != nil {
		t.Fatalf("BuildCodeExecContract(typed nil): %v", err)
	}
	normalized, ok := contract.Value.([]any)
	if !ok || normalized == nil {
		t.Fatalf("contract.Value = %#v, want non-nil empty []any", contract.Value)
	}
}

// TestCodeExec_ResultExtractsArtifacts pins the artifact
// collection: SandboxResponse.Metadata["artifacts"] entries that
// already carry a hosted URL surface unchanged as `_ARTIFACTS` in
// the tool's JSON envelope.
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
	out, err := codeExecResultJSON(t.Context(), resp)
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
	if got.Artifacts[0]["url"] != "minio://b/chart.png" {
		t.Errorf("Artifacts[0][url] = %v, want minio://b/chart.png", got.Artifacts[0]["url"])
	}
}

// TestCodeExec_ResultExtractsArtifactsFromProviderShape pins the
// extractor against the shape the sandbox providers actually store:
// collectArtifacts (local.go / ssh.go / self_managed.go) returns
// []map[string]any, and the extractor must surface that directly as
// `_ARTIFACTS` in the tool envelope instead of dropping it (the
// []any assertion alone silently lost every sandbox artifact).
func TestCodeExec_ResultExtractsArtifactsFromProviderShape(t *testing.T) {
	t.Parallel()

	// The sandbox providers (local.go / ssh.go / self_managed.go)
	// store Metadata["artifacts"] as []map[string]any; the extractor
	// must surface that shape instead of dropping it. The []any
	// assertion alone silently lost every sandbox artifact.
	got := extractArtifactList(map[string]any{
		"artifacts": []map[string]any{
			{"name": "simple_plot.png", "mime_type": "image/png", "size": 20365, "content_b64": "aGVsbG8="},
			{"name": "data.csv", "mime_type": "text/csv", "size": 12, "content_b64": "YQpi"},
		},
	}, "artifacts")
	if len(got) != 2 {
		t.Fatalf("extractArtifactList len = %d, want 2", len(got))
	}
	if got[0]["name"] != "simple_plot.png" {
		t.Errorf("got[0][name] = %v, want simple_plot.png", got[0]["name"])
	}
	if got[1]["name"] != "data.csv" {
		t.Errorf("got[1][name] = %v, want data.csv", got[1]["name"])
	}
}

// TestCodeExec_ResultDropsBadArtifactShape ensures the extractor
// silently drops entries that aren't map[string]any, and entries
// without a URL or uploadable payload, rather than aborting the run.
func TestCodeExec_ResultDropsBadArtifactShape(t *testing.T) {
	t.Parallel()

	resp := &SandboxResponse{
		Returned: "ok",
		Metadata: map[string]any{
			"artifacts": []any{
				"just a string",                  // bad shape
				map[string]any{"name": "ok.png"}, // no url, no content_b64
				42,                               // bad shape
			},
		},
	}
	out, err := codeExecResultJSON(t.Context(), resp)
	if err != nil {
		t.Fatalf("codeExecResultJSON: %v", err)
	}
	var got codeExecResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output not valid JSON: %v", jerr)
	}
	if len(got.Artifacts) != 0 {
		t.Errorf("Artifacts len = %d, want 0 (unpublishable dropped)", len(got.Artifacts))
	}
}

// TestCodeExec_UploadsArtifactBlobs pins the base64-leak fix: sandbox
// artifact payloads must be uploaded to the sandbox artifact bucket
// and referenced by /api/v1/documents/artifact/<uuid><ext> URLs, with
// the raw content_b64 kept out of the model-visible envelope.
func TestCodeExec_UploadsArtifactBlobs(t *testing.T) {
	factory := storage.GetStorageFactory()
	prev := factory.GetStorage()
	mem := storage.NewMemoryStorage()
	factory.SetStorage(mem)
	t.Cleanup(func() { factory.SetStorage(prev) })

	png := []byte("fake-png-bytes")
	encoded := base64.StdEncoding.EncodeToString(png)
	resp := &SandboxResponse{
		Returned: "ok",
		Metadata: map[string]any{
			"artifacts": []any{
				map[string]any{
					"name":        "sales_trend.png",
					"content_b64": encoded,
					"mime_type":   "image/png",
					"size":        float64(len(png)),
				},
				map[string]any{
					"name":        "pre-hosted.png",
					"url":         "minio://b/pre-hosted.png",
					"content_b64": encoded,
					"mime_type":   "image/png",
				},
			},
		},
	}
	out, err := codeExecResultJSON(t.Context(), resp)
	if err != nil {
		t.Fatalf("codeExecResultJSON: %v", err)
	}
	if strings.Contains(out, "content_b64") || strings.Contains(out, encoded) {
		t.Fatalf("envelope leaks artifact base64: %s", out)
	}
	var got codeExecResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output not valid JSON: %v (raw=%s)", jerr, out)
	}
	if len(got.Artifacts) != 2 {
		t.Fatalf("Artifacts len = %d, want 2", len(got.Artifacts))
	}
	uploaded, _ := got.Artifacts[0]["url"].(string)
	if !strings.HasPrefix(uploaded, "/api/v1/documents/artifact/") || !strings.HasSuffix(uploaded, ".png") {
		t.Errorf("Artifacts[0][url] = %q, want hosted artifact URL", uploaded)
	}
	if m, _ := got.Artifacts[0]["mime_type"].(string); m != "image/png" {
		t.Errorf("Artifacts[0][mime_type] = %v, want image/png", got.Artifacts[0]["mime_type"])
	}
	if hosted, _ := got.Artifacts[1]["url"].(string); hosted != "minio://b/pre-hosted.png" {
		t.Errorf("Artifacts[1][url] = %v, want passthrough of existing url", got.Artifacts[1]["url"])
	}

	objName := strings.TrimPrefix(uploaded, "/api/v1/documents/artifact/")
	data, gerr := mem.Get(t.Context(), common.SandboxArtifactBucket(), objName)
	if gerr != nil || !bytes.Equal(data, png) {
		t.Errorf("stored object %q = (%v, %v), want uploaded blob", objName, data, gerr)
	}
}

// TestCodeExec_DropsDataURLArtifacts pins that inline data: urls are
// dropped instead of passed through as hosted references — a data: url
// would put its base64 payload back into the model-visible envelope
// and the rendered chat message.
func TestCodeExec_DropsDataURLArtifacts(t *testing.T) {
	t.Parallel()

	resp := &SandboxResponse{
		Returned: "ok",
		Metadata: map[string]any{
			"artifacts": []any{
				map[string]any{
					"name":      "inline.png",
					"url":       "data:image/png;base64,iVBORw0KGgo=",
					"mime_type": "image/png",
				},
				map[string]any{"name": "hosted.png", "url": "minio://b/hosted.png"},
			},
		},
	}
	out, err := codeExecResultJSON(t.Context(), resp)
	if err != nil {
		t.Fatalf("codeExecResultJSON: %v", err)
	}
	if strings.Contains(out, "data:") || strings.Contains(out, "iVBORw0KGgo") {
		t.Fatalf("envelope leaks inline artifact data: %s", out)
	}
	var got codeExecResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if len(got.Artifacts) != 1 || got.Artifacts[0]["name"] != "hosted.png" {
		t.Fatalf("Artifacts = %#v, want only the hosted entry", got.Artifacts)
	}
}

// TestCodeExec_UploadsArtifactWithDerivedExtension pins that every
// published URL carries an extension the artifact route serves: names
// without a servable extension fall back to one derived from the
// MIME type, and descriptors with neither are dropped.
func TestCodeExec_UploadsArtifactWithDerivedExtension(t *testing.T) {
	factory := storage.GetStorageFactory()
	prev := factory.GetStorage()
	mem := storage.NewMemoryStorage()
	factory.SetStorage(mem)
	t.Cleanup(func() { factory.SetStorage(prev) })

	pdf := []byte("%PDF-fake")
	encoded := base64.StdEncoding.EncodeToString(pdf)
	resp := &SandboxResponse{
		Returned: "ok",
		Metadata: map[string]any{
			"artifacts": []any{
				map[string]any{
					"name":        "report",
					"content_b64": encoded,
					"mime_type":   "application/pdf",
					"size":        float64(len(pdf)),
				},
				map[string]any{
					"name":        "dump.bin",
					"content_b64": encoded,
					"mime_type":   "text/csv",
				},
				map[string]any{
					"name":        "mystery.blob",
					"content_b64": encoded,
				},
			},
		},
	}
	out, err := codeExecResultJSON(t.Context(), resp)
	if err != nil {
		t.Fatalf("codeExecResultJSON: %v", err)
	}
	var got codeExecResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if len(got.Artifacts) != 2 {
		t.Fatalf("Artifacts len = %d, want 2 (unservable descriptor dropped)", len(got.Artifacts))
	}
	url0, _ := got.Artifacts[0]["url"].(string)
	if !strings.HasPrefix(url0, "/api/v1/documents/artifact/") || !strings.HasSuffix(url0, ".pdf") {
		t.Errorf("Artifacts[0][url] = %q, want hosted URL ending in .pdf", url0)
	}
	url1, _ := got.Artifacts[1]["url"].(string)
	if !strings.HasSuffix(url1, ".csv") {
		t.Errorf("Artifacts[1][url] = %q, want extension derived from text/csv", url1)
	}
	for _, u := range []string{url0, url1} {
		objName := strings.TrimPrefix(u, "/api/v1/documents/artifact/")
		data, gerr := mem.Get(t.Context(), common.SandboxArtifactBucket(), objName)
		if gerr != nil || !bytes.Equal(data, pdf) {
			t.Errorf("stored object %q = (%v, %v), want uploaded blob", objName, data, gerr)
		}
	}
}

// TestCodeExec_ResultExtractsAttachments pins the attachments
// (rendered to downstream Message Markdown) path. Distinct from
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
	out, err := codeExecResultJSON(t.Context(), resp)
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
		StructuredResult: map[string]any{
			"present": true,
			"value": map[string]any{
				"x": float64(1),
			},
		},
	}
	out, err := codeExecResultJSON(t.Context(), resp)
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
	if got.Content != "{\n  \"x\": 1\n}" {
		t.Errorf("Content = %q, want pretty JSON object", got.Content)
	}
}

func TestCodeExec_ResultUsesStructuredResultValue(t *testing.T) {
	t.Parallel()

	resp := &SandboxResponse{
		Returned: "8",
		StructuredResult: map[string]any{
			"present": true,
			"value":   float64(8),
		},
	}
	out, err := codeExecResultJSON(t.Context(), resp)
	if err != nil {
		t.Fatalf("codeExecResultJSON: %v", err)
	}
	var got map[string]any
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output not valid JSON: %v", jerr)
	}
	if got["raw_result"] != float64(8) {
		t.Fatalf("raw_result = %#v, want 8", got["raw_result"])
	}
	if got["content"] != "8" {
		t.Fatalf("content = %#v, want \"8\"", got["content"])
	}
	if got["actual_type"] != "Number" {
		t.Fatalf("actual_type = %#v, want Number", got["actual_type"])
	}
}

func TestCodeExec_ResultPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		response   *SandboxResponse
		wantResult any
		wantType   string
	}{
		{
			name: "structured result wins over legacy and streams",
			response: &SandboxResponse{
				StructuredResult: map[string]any{"present": true, "value": float64(8)},
				Returned:         "legacy",
				Stdout:           "stdout",
				Stderr:           "warning",
			},
			wantResult: float64(8),
			wantType:   "Number",
		},
		{
			name: "explicit structured null wins over legacy and streams",
			response: &SandboxResponse{
				StructuredResult: map[string]any{"present": true, "value": nil},
				Returned:         "legacy",
				Stdout:           "stdout",
				Stderr:           "warning",
			},
			wantType: "Null",
		},
		{
			name: "legacy returned value tolerates warning streams",
			response: &SandboxResponse{
				Returned: "legacy result",
				Stderr:   "warning",
			},
			wantResult: "legacy result",
			wantType:   "String",
		},
		{
			name: "legacy returned value wins over stdout and stderr",
			response: &SandboxResponse{
				Returned: "legacy result",
				Stdout:   "diagnostic output",
				Stderr:   "warning",
			},
			wantResult: "legacy result",
			wantType:   "String",
		},
		{
			name:       "stdout remains the final fallback",
			response:   &SandboxResponse{Stdout: `{"a":[1,2]}`},
			wantResult: map[string]any{"a": []any{float64(1), float64(2)}},
			wantType:   "Object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := codeExecResultJSON(t.Context(), tt.response)
			if err != nil {
				t.Fatalf("codeExecResultJSON: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("output not valid JSON: %v", err)
			}
			if got["_ERROR"] != nil {
				t.Fatalf("_ERROR = %#v, want successful result", got["_ERROR"])
			}
			if tt.wantResult == nil {
				if _, ok := got["raw_result"]; ok {
					t.Fatalf("raw_result = %#v, want omitted JSON null", got["raw_result"])
				}
			} else if !reflect.DeepEqual(got["raw_result"], tt.wantResult) {
				t.Fatalf("raw_result = %#v, want %#v", got["raw_result"], tt.wantResult)
			}
			if got["actual_type"] != tt.wantType {
				t.Fatalf("actual_type = %#v, want %q", got["actual_type"], tt.wantType)
			}
		})
	}
}

func TestCodeExec_ResultFallsBackToStdoutJSON(t *testing.T) {
	t.Parallel()

	resp := &SandboxResponse{
		Stdout: `{"a":[1,2]}`,
	}
	out, err := codeExecResultJSON(t.Context(), resp)
	if err != nil {
		t.Fatalf("codeExecResultJSON: %v", err)
	}
	var got map[string]any
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output not valid JSON: %v", jerr)
	}
	raw, ok := got["raw_result"].(map[string]any)
	if !ok {
		t.Fatalf("raw_result type = %T, want map[string]any", got["raw_result"])
	}
	arr, ok := raw["a"].([]any)
	if !ok || len(arr) != 2 || arr[0] != float64(1) || arr[1] != float64(2) {
		t.Fatalf("raw_result[a] = %#v, want [1 2]", raw["a"])
	}
	if got["actual_type"] != "Object" {
		t.Fatalf("actual_type = %#v, want Object", got["actual_type"])
	}
	if got["content"] != "{\n  \"a\": [\n    1,\n    2\n  ]\n}" {
		t.Fatalf("content = %#v, want pretty JSON", got["content"])
	}
}

// TestCodeExec_PassesTimeoutToSandbox verifies the new
// `timeout` arg flows into the SandboxRequest.Timeout field so
// the model can dial per-script budgets. Note: this test
// mutates the global sandbox client; it must NOT run in
// parallel with the other CodeExec tests that depend on the
// default (loud-fail) stub.
func TestCodeExec_PassesTimeoutToSandbox(t *testing.T) {
	ctx := t.Context()
	var captured SandboxRequest
	prev := GetSandboxClient()
	SetSandboxClient(stubSandbox(func(_ context.Context, req SandboxRequest) (*SandboxResponse, error) {
		captured = req
		return &SandboxResponse{Returned: "ok", ExitCode: 0}, nil
	}))
	t.Cleanup(func() { SetSandboxClient(prev) })

	c := NewCodeExecTool()
	_, err := c.InvokableRun(ctx,
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
	ctx := t.Context()
	var captured SandboxRequest
	prev := GetSandboxClient()
	SetSandboxClient(stubSandbox(func(_ context.Context, req SandboxRequest) (*SandboxResponse, error) {
		captured = req
		return &SandboxResponse{Returned: "ok", ExitCode: 0}, nil
	}))
	t.Cleanup(func() { SetSandboxClient(prev) })

	c := NewCodeExecTool()
	_, err := c.InvokableRun(ctx,
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
