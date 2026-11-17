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

package sandbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeKosa8 is an in-memory stand-in for the kosa8 REST API.
type fakeKosa8 struct {
	t *testing.T

	mu           sync.Mutex
	requests     []string
	execBodies   []map[string]any
	labels       []map[string]any
	egress       []map[string]any
	files        map[string][]byte
	listings     map[string][]map[string]any
	deleted      []string
	created      int
	createStates []string
	pollStates   []string
	execAnswers  []func(w http.ResponseWriter, body map[string]any)
	filesStatus  int
	deleteStatus int
	// dropDeletes is how many DELETEs get their connection closed
	// without an answer.
	dropDeletes int
	auth        string
}

func newFakeKosa8(t *testing.T) (*fakeKosa8, *httptest.Server) {
	f := &fakeKosa8{t: t, files: map[string][]byte{}, listings: map[string][]map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func execOK(stdout, stderr string, exit int) func(http.ResponseWriter, map[string]any) {
	return func(w http.ResponseWriter, _ map[string]any) {
		writeJSON(w, http.StatusOK, map[string]any{"exit": exit, "stdout": stdout, "stderr": stderr})
	}
}

func (f *fakeKosa8) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = r.Header.Get("Authorization")
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	p := r.URL.Path
	switch {
	case r.Method == http.MethodGet && p == "/v1/me":
		writeJSON(w, 200, map[string]any{"org": map[string]any{"id": "org_1"}})
	case r.Method == http.MethodPost && p == "/v1/sandboxes":
		f.created++
		state := "running"
		if len(f.createStates) > 0 {
			state, f.createStates = f.createStates[0], f.createStates[1:]
		}
		f.execBodies = append(f.execBodies, map[string]any{"create": body})
		writeJSON(w, 201, map[string]any{"sandbox": map[string]any{"id": fmt.Sprintf("sbx_%d", f.created), "state": state}})
	case r.Method == http.MethodGet && strings.Count(p, "/") == 3:
		state := "running"
		if len(f.pollStates) > 0 {
			state, f.pollStates = f.pollStates[0], f.pollStates[1:]
		}
		writeJSON(w, 200, map[string]any{"sandbox": map[string]any{"id": "sbx_1", "state": state, "last_error": "no capacity"}})
	case r.Method == http.MethodDelete && strings.Count(p, "/") == 3:
		if f.dropDeletes > 0 {
			f.dropDeletes--
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				_ = conn.Close()
			}
			return
		}
		if f.deleteStatus != 0 {
			writeJSON(w, f.deleteStatus, map[string]any{"error": "not_found", "message": "gone"})
			return
		}
		f.deleted = append(f.deleted, strings.TrimPrefix(p, "/v1/sandboxes/"))
		writeJSON(w, 200, map[string]any{"deleted": true})
	case strings.HasSuffix(p, "/labels"):
		f.labels = append(f.labels, body)
		writeJSON(w, 200, body)
	case strings.HasSuffix(p, "/egress"):
		f.egress = append(f.egress, body)
		writeJSON(w, 200, map[string]any{"egress": body})
	case strings.HasSuffix(p, "/exec"):
		f.execBodies = append(f.execBodies, body)
		if len(f.execAnswers) == 0 {
			execOK("", "", 0)(w, body)
			return
		}
		answer := f.execAnswers[0]
		f.execAnswers = f.execAnswers[1:]
		answer(w, body)
	case strings.HasSuffix(p, "/files/list"):
		if f.filesStatus != 0 {
			writeJSON(w, f.filesStatus, map[string]any{"error": "not_supported", "message": "no data plane"})
			return
		}
		dir := r.URL.Query().Get("path")
		if _, ok := f.listings[dir]; !ok {
			writeJSON(w, 404, map[string]any{"error": "not_found", "message": "no such dir"})
			return
		}
		writeJSON(w, 200, map[string]any{"path": dir, "files": f.listings[dir]})
	case strings.HasSuffix(p, "/files"):
		if f.filesStatus != 0 {
			writeJSON(w, f.filesStatus, map[string]any{"error": "not_supported", "message": "no data plane"})
			return
		}
		filePath := r.URL.Query().Get("path")
		if r.Method == http.MethodPut {
			f.files[filePath] = raw
			writeJSON(w, 200, map[string]any{"file": map[string]any{"path": filePath}})
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(f.files[filePath])
	default:
		writeJSON(w, 404, map[string]any{"error": "not_found", "message": "no route " + r.Method + " " + p})
	}
}

// commands returns the exec bodies, leaving out the create bodies.
func (f *fakeKosa8) commands() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for _, b := range f.execBodies {
		if _, ok := b["create"]; !ok {
			out = append(out, b)
		}
	}
	return out
}

func (f *fakeKosa8) createBody() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range f.execBodies {
		if c, ok := b["create"]; ok {
			return c.(map[string]any)
		}
	}
	return nil
}

func cmdOf(body map[string]any) []string {
	raw, _ := body["cmd"].([]any)
	out := make([]string, len(raw))
	for i, v := range raw {
		out[i], _ = v.(string)
	}
	return out
}

func newTestKosa8Provider(t *testing.T, srv *httptest.Server, extra map[string]any) *Kosa8Provider {
	t.Helper()
	cfg := map[string]any{"api_key": "k8c_live_test", "base_url": srv.URL}
	for k, v := range extra {
		cfg[k] = v
	}
	p := newKosa8ProviderFromConfig(cfg)
	p.startPoll = time.Millisecond
	p.deleteBackoff = time.Millisecond
	// httptest serves plain HTTP on 127.0.0.1, which Initialize allows.
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return p
}

func TestKosa8Provider_ProviderTypeAndLanguages(t *testing.T) {
	t.Parallel()
	p := newKosa8ProviderFromConfig(nil)
	if p.ProviderType() != ProviderKosa8 {
		t.Errorf("ProviderType = %q, want %q", p.ProviderType(), ProviderKosa8)
	}
	if got := strings.Join(p.SupportedLanguages(), ","); got != "python,javascript" {
		t.Errorf("SupportedLanguages = %s", got)
	}
}

func TestKosa8Provider_Defaults(t *testing.T) {
	for _, k := range []string{"KOSA8_API_KEY", "KOSA8_API_URL", "KOSA8_PYTHON_TEMPLATE", "KOSA8_NODE_TEMPLATE", "KOSA8_TIMEOUT", "KOSA8_ALLOW_INTERNET"} {
		t.Setenv(k, "")
	}
	p := newKosa8ProviderFromEnv()
	if p.pythonTemplate != "python" || p.nodeTemplate != "node" {
		t.Errorf("templates = %q/%q, want python/node", p.pythonTemplate, p.nodeTemplate)
	}
	if p.allowInternet {
		t.Error("allowInternet = true, want false by default")
	}
	if p.timeoutSec != 30 || p.maxLifetimeSec != 3600 || p.maxOutputBytes != 1<<20 || p.maxArtifacts != 20 || p.maxArtifactBytes != 10<<20 {
		t.Errorf("unexpected defaults: %+v", p)
	}
}

func TestKosa8Provider_EnvOverride(t *testing.T) {
	t.Setenv("KOSA8_API_KEY", "k8c_live_env")
	t.Setenv("KOSA8_API_URL", "https://kosa8.example.com/")
	t.Setenv("KOSA8_PYTHON_TEMPLATE", "py-ml")
	t.Setenv("KOSA8_NODE_TEMPLATE", "node-web")
	t.Setenv("KOSA8_TIMEOUT", "120")
	t.Setenv("KOSA8_ALLOW_INTERNET", "true")
	p := newKosa8ProviderFromEnv()
	if p.apiKey != "k8c_live_env" || p.baseURL != "https://kosa8.example.com" || p.pythonTemplate != "py-ml" ||
		p.nodeTemplate != "node-web" || p.timeoutSec != 120 || !p.allowInternet {
		t.Errorf("env not applied: %+v", p)
	}
}

func TestKosa8Provider_InitializeRejectsBadConfig(t *testing.T) {
	t.Setenv("KOSA8_API_KEY", "")
	t.Setenv("KOSA8_API_URL", "")
	if err := newKosa8ProviderFromConfig(map[string]any{}).Initialize(context.Background()); err == nil || !strings.Contains(err.Error(), "API key is required") {
		t.Errorf("missing key: err = %v", err)
	}
	err := newKosa8ProviderFromConfig(map[string]any{"api_key": "k", "base_url": "http://kosa8.example.com"}).Initialize(context.Background())
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("plain http: err = %v", err)
	}
	err = newKosa8ProviderFromConfig(map[string]any{"api_key": "k", "max_lifetime": 30}).Initialize(context.Background())
	if err == nil {
		t.Error("max_lifetime below 60 accepted")
	}
	p := newKosa8ProviderFromConfig(map[string]any{"api_key": "k"})
	if err := p.Initialize(context.Background()); err != nil || p.baseURL != kosa8DefaultBaseURL {
		t.Errorf("default URL: err = %v, baseURL = %q", err, p.baseURL)
	}
}

func TestKosa8Provider_AllOpsBeforeInit(t *testing.T) {
	t.Parallel()
	p := newKosa8ProviderFromConfig(map[string]any{"api_key": "k"})
	ctx := context.Background()
	inst := &SandboxInstance{InstanceID: "sbx_1"}
	if _, err := p.CreateInstance(ctx, "python"); err == nil {
		t.Error("CreateInstance before Initialize succeeded")
	}
	if _, err := p.ExecuteCode(ctx, inst, "x", "python", 5, nil); err == nil {
		t.Error("ExecuteCode before Initialize succeeded")
	}
	if err := p.DestroyInstance(ctx, inst); err == nil {
		t.Error("DestroyInstance before Initialize succeeded")
	}
	if err := p.HealthCheck(ctx); err == nil {
		t.Error("HealthCheck before Initialize succeeded")
	}
}

func TestKosa8Provider_HealthCheckSendsTheKey(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, nil)
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if f.auth != "Bearer k8c_live_test" {
		t.Errorf("Authorization = %q", f.auth)
	}
}

func TestKosa8Provider_FullLifecycle(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, map[string]any{"max_lifetime": 600})
	ctx := context.Background()

	inst, err := p.CreateInstance(ctx, "python3")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if inst.InstanceID != "sbx_1" || inst.Provider != ProviderKosa8 {
		t.Fatalf("instance = %+v", inst)
	}
	if got := f.createBody(); got["template"] != "python" || got["ttl_s"] != float64(600) {
		t.Errorf("create body = %v", got)
	}
	if len(f.labels) != 1 || len(f.egress) != 1 || f.egress[0]["internet"] != false {
		t.Errorf("labels = %v, egress = %v", f.labels, f.egress)
	}
	workDir := inst.Metadata["remote_work_dir"].(string)
	if !strings.HasPrefix(workDir, "/workspace/ragflow-codeexec-") {
		t.Errorf("remote_work_dir = %q", workDir)
	}
	if mkdir := cmdOf(f.commands()[0]); strings.Join(mkdir, " ") != "mkdir -p "+workDir+"/artifacts" {
		t.Errorf("workspace command = %v", mkdir)
	}

	png := []byte("\x89PNG fake")
	f.mu.Lock()
	f.execAnswers = []func(http.ResponseWriter, map[string]any){execOK("hello\n__RAGFLOW_RESULT__:"+base64.StdEncoding.EncodeToString([]byte(`{"present":true,"value":42,"type":"json"}`))+"\n", "warn\n", 0)}
	f.listings[workDir+"/artifacts"] = []map[string]any{
		{"name": "plots", "path": workDir + "/artifacts/plots", "type": "dir", "size": 0},
	}
	f.listings[workDir+"/artifacts/plots"] = []map[string]any{
		{"name": "chart.png", "path": workDir + "/artifacts/plots/chart.png", "type": "file", "size": len(png)},
	}
	f.files[workDir+"/artifacts/plots/chart.png"] = png
	f.mu.Unlock()

	res, err := p.ExecuteCode(ctx, inst, "def main():\n    return 42\n", "python", 10, map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if res.ExitCode != 0 || res.Stderr != "warn\n" || !strings.Contains(res.Stdout, "hello") {
		t.Errorf("result = %+v", res)
	}
	if script := string(f.files[workDir+"/main.py"]); !strings.Contains(script, "return 42") {
		t.Errorf("script not written through the files API: %q", script)
	}
	run := f.commands()[1]
	if got := cmdOf(run); strings.Join(got[3:], " ") != "ragflow "+workDir+" python3 main.py" || got[0] != "sh" {
		t.Errorf("run command = %v", got)
	}
	if run["timeout_s"] != float64(10) {
		t.Errorf("timeout_s = %v, want 10", run["timeout_s"])
	}
	artifacts := res.Metadata["artifacts"].([]map[string]any)
	if len(artifacts) != 1 || artifacts[0]["name"] != "plots/chart.png" || artifacts[0]["mime_type"] != "image/png" ||
		artifacts[0]["content_b64"] != base64.StdEncoding.EncodeToString(png) {
		t.Errorf("artifacts = %v", artifacts)
	}

	if err := p.DestroyInstance(ctx, inst); err != nil {
		t.Fatalf("DestroyInstance: %v", err)
	}
	if err := p.DestroyInstance(ctx, inst); err != nil {
		t.Fatalf("second DestroyInstance: %v", err)
	}
	if strings.Join(f.deleted, ",") != "sbx_1" {
		t.Errorf("deleted = %v", f.deleted)
	}
}

func TestKosa8Provider_JavaScriptUsesTheNodeTemplate(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, map[string]any{"node_template": "node-22", "allow_internet": true})
	ctx := context.Background()
	inst, err := p.CreateInstance(ctx, "javascript")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if f.createBody()["template"] != "node-22" || f.egress[0]["internet"] != true {
		t.Errorf("create = %v, egress = %v", f.createBody(), f.egress)
	}
	if _, err := p.ExecuteCode(ctx, inst, "function main() { return 1 }", "js", 5, nil); err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if got := cmdOf(f.commands()[1]); got[len(got)-2] != "node" || got[len(got)-1] != "main.js" {
		t.Errorf("run command = %v", got)
	}
}

func TestKosa8Provider_TimeoutIsClampedAndReported(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, map[string]any{"timeout": 20})
	ctx := context.Background()
	inst, err := p.CreateInstance(ctx, "python")
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.execAnswers = []func(http.ResponseWriter, map[string]any){func(w http.ResponseWriter, _ map[string]any) {
		writeJSON(w, http.StatusGatewayTimeout, map[string]any{"error": "exec_timeout", "message": "ran longer than its timeout_s", "stdout": "", "stderr": ""})
	}}
	f.mu.Unlock()
	_, err = p.ExecuteCode(ctx, inst, "while True: pass", "python", 300, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out after 20 seconds") {
		t.Fatalf("err = %v", err)
	}
	if got := f.commands()[1]["timeout_s"]; got != float64(20) {
		t.Errorf("timeout_s = %v, want the configured 20", got)
	}
}

func TestKosa8Provider_LongTimeoutStreams(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, map[string]any{"timeout": 300})
	ctx := context.Background()
	inst, err := p.CreateInstance(ctx, "python")
	if err != nil {
		t.Fatal(err)
	}
	frame := func(stream, text string) string {
		b, _ := json.Marshal(map[string]any{"stream": stream, "data": base64.StdEncoding.EncodeToString([]byte(text))})
		return string(b) + "\n"
	}
	f.mu.Lock()
	f.execAnswers = []func(http.ResponseWriter, map[string]any){func(w http.ResponseWriter, _ map[string]any) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, frame("stdout", "a")+`{"stream":"stdout","data":""}`+"\n"+frame("stderr", "b")+frame("stdout", "c\n")+`{"done":true,"exit":3}`+"\n")
	}}
	f.mu.Unlock()
	res, err := p.ExecuteCode(ctx, inst, "print('x')", "python", 200, nil)
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if res.Stdout != "ac\n" || res.Stderr != "b" || res.ExitCode != 3 {
		t.Errorf("result = %+v", res)
	}
	run := f.commands()[1]
	if run["stream"] != true || run["timeout_s"] != float64(200) {
		t.Errorf("streamed body = %v", run)
	}
}

func TestKosa8Provider_HostWithoutStreamedTimeoutTimesInTheSandbox(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, map[string]any{"timeout": 300})
	ctx := context.Background()
	inst, err := p.CreateInstance(ctx, "python")
	if err != nil {
		t.Fatal(err)
	}
	refuse := func(w http.ResponseWriter, _ map[string]any) {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "not_available", "message": "timeout_s on a streamed exec is not available on this sandbox's host yet"})
	}
	done := func(exit int) func(http.ResponseWriter, map[string]any) {
		return func(w http.ResponseWriter, _ map[string]any) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = fmt.Fprintf(w, "{\"done\":true,\"exit\":%d}\n", exit)
		}
	}
	f.mu.Lock()
	f.execAnswers = []func(http.ResponseWriter, map[string]any){refuse, done(0), done(143)}
	f.mu.Unlock()

	for i, want := range []int{0, 143} {
		res, err := p.ExecuteCode(ctx, inst, "print('x')", "python", 200, nil)
		if err != nil {
			t.Fatalf("ExecuteCode %d: %v", i, err)
		}
		// A 143 well before the deadline is the command's own exit.
		if res.ExitCode != want {
			t.Errorf("ExecuteCode %d: exit %d, want %d", i, res.ExitCode, want)
		}
	}
	cmds := f.commands()
	if len(cmds) != 4 {
		t.Fatalf("exec calls = %d, want 4 (mkdir, refused, two fallbacks)", len(cmds))
	}
	if cmds[1]["timeout_s"] != float64(200) {
		t.Errorf("first streamed body = %v", cmds[1])
	}
	// The refusal is remembered: the next command goes straight to the fallback.
	for _, body := range cmds[2:] {
		if _, ok := body["timeout_s"]; ok || body["stream"] != true {
			t.Errorf("fallback body = %v", body)
		}
		cmd := cmdOf(body)
		if len(cmd) < 6 || cmd[0] != "sh" || !strings.Contains(cmd[2], `timeout "$t" "$@"; exit $?`) || cmd[4] != "200" || cmd[5] != "sh" {
			t.Errorf("fallback cmd = %q", cmd)
		}
	}
}

func TestKosa8Provider_HostWithoutFilesAPIUsesExec(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	f.filesStatus = http.StatusNotImplemented
	p := newTestKosa8Provider(t, srv, nil)
	ctx := context.Background()
	inst, err := p.CreateInstance(ctx, "python")
	if err != nil {
		t.Fatal(err)
	}
	code := strings.Repeat("x = 1\n", 2000) // 12,000 bytes, more than one chunk once wrapped
	res, err := p.ExecuteCode(ctx, inst, code, "python", 5, nil)
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	cmds := f.commands()
	writes := cmds[1 : len(cmds)-1]
	if len(writes) < 2 {
		t.Fatalf("writes = %d, want the script in several chunks", len(writes))
	}
	var script []byte
	for i, w := range writes {
		args := cmdOf(w)
		wantRedirect := `>> "$1"`
		if i == 0 {
			wantRedirect = `> "$1"`
		}
		if !strings.HasSuffix(args[2], wantRedirect) || !strings.HasSuffix(args[4], "/main.py") {
			t.Errorf("write %d = %v", i, args[:5])
		}
		chunk, err := base64.StdEncoding.DecodeString(args[5])
		if err != nil {
			t.Fatal(err)
		}
		script = append(script, chunk...)
	}
	if !strings.Contains(string(script), code) {
		t.Error("written script does not hold the code")
	}
	if res.Metadata["artifacts_error"] == nil || len(res.Metadata["artifacts"].([]map[string]any)) != 0 {
		t.Errorf("metadata = %v", res.Metadata)
	}
}

func TestKosa8Provider_ColdBootIsAwaited(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	f.createStates = []string{"creating"}
	f.pollStates = []string{"creating", "running"}
	p := newTestKosa8Provider(t, srv, nil)
	if _, err := p.CreateInstance(context.Background(), "python"); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	polls := 0
	for _, r := range f.requests {
		if r == "GET /v1/sandboxes/sbx_1" {
			polls++
		}
	}
	if polls != 2 {
		t.Errorf("polls = %d, want 2", polls)
	}
}

func TestKosa8Provider_FailedStartDeletesTheSandbox(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	f.createStates = []string{"creating"}
	f.pollStates = []string{"error"}
	p := newTestKosa8Provider(t, srv, nil)
	_, err := p.CreateInstance(context.Background(), "python")
	if err == nil || !strings.Contains(err.Error(), "no capacity") {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(f.deleted, ",") != "sbx_1" {
		t.Errorf("deleted = %v", f.deleted)
	}
}

func TestKosa8Provider_FailedWorkspaceDeletesTheSandbox(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	f.execAnswers = []func(http.ResponseWriter, map[string]any){execOK("", "read-only file system", 1)}
	p := newTestKosa8Provider(t, srv, nil)
	if _, err := p.CreateInstance(context.Background(), "python"); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("err = %v", err)
	}
	if strings.Join(f.deleted, ",") != "sbx_1" {
		t.Errorf("deleted = %v", f.deleted)
	}
}

func TestKosa8Provider_APIErrorsCarryTheCode(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusPaymentRequired, map[string]any{"error": "payment_required", "message": "add a card"})
	}))
	t.Cleanup(srv.Close)
	p := newTestKosa8Provider(t, srv, nil)
	_, err := p.CreateInstance(context.Background(), "python")
	if err == nil || !strings.Contains(err.Error(), "payment_required") || !strings.Contains(err.Error(), "add a card") {
		t.Fatalf("err = %v", err)
	}
}

func TestKosa8Provider_AlreadyDeletedSandboxIsClean(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, nil)
	ctx := context.Background()
	inst, err := p.CreateInstance(ctx, "python")
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.deleteStatus = http.StatusNotFound
	f.mu.Unlock()
	if err := p.DestroyInstance(ctx, inst); err != nil {
		t.Fatalf("DestroyInstance of a sandbox already gone: %v", err)
	}
	if got := f.countRequests("DELETE /v1/sandboxes/sbx_1"); got != 1 {
		t.Errorf("DELETE requests = %d, want 1 (404 is not retried)", got)
	}
	if _, err := p.ExecuteCode(ctx, inst, "x", "python", 5, nil); err == nil || !strings.Contains(err.Error(), "unknown instance") {
		t.Errorf("instance still known after a 404 delete: %v", err)
	}
}

func (f *fakeKosa8) countRequests(want string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if r == want {
			n++
		}
	}
	return n
}

func TestKosa8Provider_FailedDeleteKeepsTheInstanceForRetry(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, nil)
	ctx := context.Background()
	inst, err := p.CreateInstance(ctx, "python")
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.deleteStatus = http.StatusServiceUnavailable
	f.mu.Unlock()
	if err := p.DestroyInstance(ctx, inst); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("DestroyInstance with a failing DELETE: %v", err)
	}
	if got := f.countRequests("DELETE /v1/sandboxes/sbx_1"); got != kosa8DeleteAttempts {
		t.Errorf("DELETE requests = %d, want %d (a 5xx is retried)", got, kosa8DeleteAttempts)
	}
	p.mu.RLock()
	_, kept := p.instances[inst.InstanceID]
	p.mu.RUnlock()
	if !kept {
		t.Fatal("instance forgotten although its sandbox was not deleted")
	}

	// A second destroy sends DELETE again and then forgets the instance.
	f.mu.Lock()
	f.deleteStatus = 0
	f.mu.Unlock()
	if err := p.DestroyInstance(ctx, inst); err != nil {
		t.Fatalf("second DestroyInstance: %v", err)
	}
	if strings.Join(f.deleted, ",") != "sbx_1" {
		t.Errorf("deleted = %v", f.deleted)
	}
	if err := p.DestroyInstance(ctx, inst); err != nil {
		t.Fatalf("third DestroyInstance: %v", err)
	}
	if got := f.countRequests("DELETE /v1/sandboxes/sbx_1"); got != kosa8DeleteAttempts+1 {
		t.Errorf("DELETE requests = %d, want %d (no DELETE once deleted)", got, kosa8DeleteAttempts+1)
	}
}

func TestKosa8Provider_DeleteRetriesOnlyTransientFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status int
		want   int
	}{
		{http.StatusTooManyRequests, kosa8DeleteAttempts},
		{http.StatusInternalServerError, kosa8DeleteAttempts},
		{http.StatusForbidden, 1},
	} {
		f, srv := newFakeKosa8(t)
		p := newTestKosa8Provider(t, srv, nil)
		ctx := context.Background()
		inst, err := p.CreateInstance(ctx, "python")
		if err != nil {
			t.Fatal(err)
		}
		f.mu.Lock()
		f.deleteStatus = tc.status
		f.mu.Unlock()
		if err := p.DestroyInstance(ctx, inst); err == nil {
			t.Errorf("HTTP %d: DestroyInstance succeeded", tc.status)
		}
		if got := f.countRequests("DELETE /v1/sandboxes/sbx_1"); got != tc.want {
			t.Errorf("HTTP %d: DELETE requests = %d, want %d", tc.status, got, tc.want)
		}
	}
}

func TestKosa8Provider_DeleteRetriesNetworkErrors(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, nil)
	ctx := context.Background()
	inst, err := p.CreateInstance(ctx, "python")
	if err != nil {
		t.Fatal(err)
	}
	// The first DELETE is cut off before an answer; the retry lands.
	f.mu.Lock()
	f.dropDeletes = 1
	f.mu.Unlock()
	if err := p.DestroyInstance(ctx, inst); err != nil {
		t.Fatalf("DestroyInstance after a dropped connection: %v", err)
	}
	if strings.Join(f.deleted, ",") != "sbx_1" {
		t.Errorf("deleted = %v", f.deleted)
	}
}

func TestKosa8Provider_TemplateWithoutTimeoutFails(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, map[string]any{"timeout": 300})
	ctx := context.Background()
	inst, err := p.CreateInstance(ctx, "python")
	if err != nil {
		t.Fatal(err)
	}
	refuse := func(w http.ResponseWriter, _ map[string]any) {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "not_available", "message": "timeout_s on a streamed exec is not available on this sandbox's host yet"})
	}
	// What kosa8Timed prints and exits with when `timeout` is missing.
	noTimeout := func(w http.ResponseWriter, _ map[string]any) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintf(w, "{\"stream\":\"stderr\",\"data\":%q}\n{\"done\":true,\"exit\":%d}\n",
			base64.StdEncoding.EncodeToString([]byte(kosa8NoTimeoutMarker+"\n")), kosa8NoTimeoutExit)
	}
	f.mu.Lock()
	f.execAnswers = []func(http.ResponseWriter, map[string]any){refuse, noTimeout}
	f.mu.Unlock()
	_, err = p.ExecuteCode(ctx, inst, "print('x')", "python", 200, nil)
	if err == nil || !strings.Contains(err.Error(), "no `timeout` command") {
		t.Fatalf("err = %v, want a missing-timeout error", err)
	}
	// The command is never run untimed: the wrapper has no other branch.
	cmd := cmdOf(f.commands()[2])
	if strings.Contains(cmd[2], "exec") || !strings.Contains(cmd[2], "exit 127") {
		t.Errorf("fallback script = %q", cmd[2])
	}
}

func TestKosa8Provider_OversizedBufferedAnswerIsRejectedBeforeDecoding(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, map[string]any{"max_output_bytes": 100})
	ctx := context.Background()
	inst, err := p.CreateInstance(ctx, "python")
	if err != nil {
		t.Fatal(err)
	}
	limit := p.bufferedExecLimit()
	// Past the cap the body is not JSON at all: a decode would fail with
	// a syntax error, so the size error shows the body was never decoded.
	huge := func(w http.ResponseWriter, _ map[string]any) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"exit":0,"stdout":"`))
		_, _ = w.Write([]byte(strings.Repeat("x", int(limit)) + "\x00not json"))
	}
	f.mu.Lock()
	f.execAnswers = []func(http.ResponseWriter, map[string]any){huge}
	f.mu.Unlock()
	_, err = p.ExecuteCode(ctx, inst, "print('x')", "python", 5, nil)
	if err == nil || !strings.Contains(err.Error(), "output exceeded 100 bytes") {
		t.Fatalf("err = %v, want the output limit error", err)
	}
}

func TestKosa8Provider_BufferedAnswerAllowsJSONEscaping(t *testing.T) {
	t.Parallel()
	f, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, map[string]any{"max_output_bytes": 100})
	ctx := context.Background()
	inst, err := p.CreateInstance(ctx, "python")
	if err != nil {
		t.Fatal(err)
	}
	// 100 control bytes escape to 600 (\u0001 each): within the cap, and
	// exactly max_output_bytes once decoded.
	escaped := strings.Repeat("\x01", 100)
	f.mu.Lock()
	f.execAnswers = []func(http.ResponseWriter, map[string]any){execOK(escaped, "", 0), execOK(escaped, "x", 0)}
	f.mu.Unlock()
	res, err := p.ExecuteCode(ctx, inst, "print('x')", "python", 5, nil)
	if err != nil {
		t.Fatalf("ExecuteCode with escaped output at the limit: %v", err)
	}
	if res.Stdout != escaped {
		t.Errorf("stdout = %q", res.Stdout)
	}
	// One byte over max_output_bytes after decoding is still refused.
	if _, err := p.ExecuteCode(ctx, inst, "print('x')", "python", 5, nil); err == nil || !strings.Contains(err.Error(), "output exceeded 100 bytes") {
		t.Fatalf("err = %v, want the output limit error", err)
	}
}

func TestKosa8Provider_RejectsUnknownLanguageAndInstance(t *testing.T) {
	t.Parallel()
	_, srv := newFakeKosa8(t)
	p := newTestKosa8Provider(t, srv, nil)
	ctx := context.Background()
	if _, err := p.CreateInstance(ctx, "ruby"); err == nil {
		t.Error("ruby accepted")
	}
	if _, err := p.ExecuteCode(ctx, &SandboxInstance{InstanceID: "sbx_404"}, "x", "python", 5, nil); err == nil {
		t.Error("unknown instance accepted")
	}
}
