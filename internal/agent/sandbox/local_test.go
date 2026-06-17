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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newLocalForTest builds a LocalProvider pointing at a temp
// work_dir so tests don't pollute the operator's filesystem.
func newLocalForTest(t *testing.T) *LocalProvider {
	t.Helper()
	workDir := t.TempDir()
	p := &LocalProvider{
		pythonBin:        localDefaultPythonBin,
		nodeBin:          localDefaultNodeBin,
		workDir:          workDir,
		timeout:          5,
		maxMemoryMB:      512,
		maxOutputBytes:   1 << 20,
		maxArtifacts:     20,
		maxArtifactBytes: 10 << 20,
		instances:        map[string]string{},
	}
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return p
}

func TestLocal_ProviderTypeAndLanguages(t *testing.T) {
	t.Parallel()
	p := newLocalProviderFromEnv()
	if p.ProviderType() != ProviderLocal {
		t.Errorf("ProviderType = %q, want %q", p.ProviderType(), ProviderLocal)
	}
	langs := p.SupportedLanguages()
	want := map[string]bool{"python": true, "nodejs": true, "javascript": true}
	for _, l := range langs {
		if !want[l] {
			t.Errorf("unexpected language: %q", l)
		}
	}
}

func TestLocal_EnvDefaults(t *testing.T) {
	for _, k := range []string{
		"LOCAL_PYTHON_BIN", "LOCAL_NODE_BIN", "LOCAL_WORK_DIR",
		"LOCAL_TIMEOUT", "LOCAL_MAX_MEMORY_MB", "LOCAL_MAX_OUTPUT_BYTES",
		"LOCAL_MAX_ARTIFACTS", "LOCAL_MAX_ARTIFACT_BYTES",
	} {
		t.Setenv(k, "")
	}
	p := newLocalProviderFromEnv()
	if p.pythonBin != localDefaultPythonBin {
		t.Errorf("pythonBin = %q, want %q", p.pythonBin, localDefaultPythonBin)
	}
	if p.workDir != localDefaultWorkDir {
		t.Errorf("workDir = %q, want %q", p.workDir, localDefaultWorkDir)
	}
	if p.timeout != localDefaultTimeout {
		t.Errorf("timeout = %d, want %d", p.timeout, localDefaultTimeout)
	}
}

func TestLocal_EnvOverride(t *testing.T) {
	workDir := t.TempDir()
	t.Setenv("LOCAL_WORK_DIR", workDir)
	t.Setenv("LOCAL_TIMEOUT", "12")
	t.Setenv("LOCAL_MAX_OUTPUT_BYTES", "4096")
	p := newLocalProviderFromEnv()
	if p.workDir != workDir {
		t.Errorf("workDir = %q, want %q", p.workDir, workDir)
	}
	if p.timeout != 12 {
		t.Errorf("timeout = %d, want 12", p.timeout)
	}
	if p.maxOutputBytes != 4096 {
		t.Errorf("maxOutputBytes = %d, want 4096", p.maxOutputBytes)
	}
}

func TestLocal_Initialize_CreatesWorkDir(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "nested", "workdir")
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("workDir unexpectedly exists: %v", err)
	}
	t.Setenv("LOCAL_WORK_DIR", workDir)
	p := newLocalProviderFromEnv()
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	info, err := os.Stat(workDir)
	if err != nil {
		t.Fatalf("workDir not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("workDir is not a directory: %v", info)
	}
}

func TestLocal_CreateInstance_CreatesArtifactsDir(t *testing.T) {
	p := newLocalForTest(t)
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if inst.Provider != ProviderLocal {
		t.Errorf("provider = %q, want %q", inst.Provider, ProviderLocal)
	}
	artifactsDir := filepath.Join(p.workDir, inst.InstanceID, "artifacts")
	info, err := os.Stat(artifactsDir)
	if err != nil {
		t.Fatalf("artifacts dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("artifacts path is not a directory: %v", info)
	}
}

func TestLocal_CreateInstance_RejectsBadLanguage(t *testing.T) {
	p := newLocalForTest(t)
	if _, err := p.CreateInstance(context.Background(), "ruby"); err == nil {
		t.Errorf("CreateInstance(ruby): got nil error, want one")
	}
}

func TestLocal_AllOps_BeforeInit(t *testing.T) {
	t.Parallel()
	p := &LocalProvider{}
	inst := &SandboxInstance{InstanceID: "x", Provider: ProviderLocal}
	if _, err := p.CreateInstance(context.Background(), "python"); err == nil {
		t.Errorf("CreateInstance before init: got nil error, want one")
	}
	if _, err := p.ExecuteCode(context.Background(), inst, "x", "python", 5, nil); err == nil {
		t.Errorf("ExecuteCode before init: got nil error, want one")
	}
	if err := p.DestroyInstance(context.Background(), inst); err == nil {
		t.Errorf("DestroyInstance before init: got nil error, want one")
	}
	if err := p.HealthCheck(context.Background()); err == nil {
		t.Errorf("HealthCheck before init: got nil error, want one")
	}
}

// TestLocal_ExecuteCode_Python_RoundTrip runs a real Python
// subprocess via the local provider. Skipped if python3 is not
// on PATH (CI without python).
func TestLocal_ExecuteCode_Python_RoundTrip(t *testing.T) {
	pythonPath, err := findBinary("python3")
	if err != nil {
		t.Skip("python3 not on PATH — skipping local subprocess test")
	}
	p := newLocalForTest(t)
	p.pythonBin = pythonPath
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer p.DestroyInstance(context.Background(), inst)

	code := "def main(): return {'value': 7, 'type': 'json'}"
	result, err := p.ExecuteCode(context.Background(), inst, code, "python", 10, nil)
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
	if sr, ok := result.Metadata["structured_result"].(map[string]any); !ok {
		t.Errorf("structured_result missing or wrong type: %v", result.Metadata["structured_result"])
	} else if v, _ := sr["value"].(map[string]any)["value"].(float64); v != 7 {
		t.Errorf("structured_result[value].value = %v, want 7", sr)
	}
}

func TestLocal_ExecuteCode_RejectsBadInputs(t *testing.T) {
	p := newLocalForTest(t)
	p.initialized = true

	cases := []struct {
		name string
		fn   func() error
		want string
	}{
		{
			name: "empty instance id",
			fn: func() error {
				_, err := p.ExecuteCode(context.Background(),
					&SandboxInstance{InstanceID: ""}, "x", "python", 5, nil)
				return err
			},
			want: "instance id",
		},
		{
			name: "unsupported language",
			fn: func() error {
				_, err := p.ExecuteCode(context.Background(),
					&SandboxInstance{InstanceID: "x"}, "x", "ruby", 5, nil)
				return err
			},
			want: "unsupported language",
		},
		{
			name: "timeout too small",
			fn: func() error {
				_, err := p.ExecuteCode(context.Background(),
					&SandboxInstance{InstanceID: "x"}, "x", "python", 0, nil)
				return err
			},
			want: "timeout",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil {
				t.Fatalf("got nil error, want one containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want to contain %q", err, tc.want)
			}
		})
	}
}

func TestLocal_DestroyInstance_RemovesDir(t *testing.T) {
	p := newLocalForTest(t)
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	dir := filepath.Join(p.workDir, inst.InstanceID)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("instance dir not created: %v", err)
	}
	if err := p.DestroyInstance(context.Background(), inst); err != nil {
		t.Errorf("DestroyInstance: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("instance dir still exists after destroy: %v", err)
	}
	// Idempotent: second call should be a no-op.
	if err := p.DestroyInstance(context.Background(), inst); err != nil {
		t.Errorf("DestroyInstance (idempotent): %v", err)
	}
}

func TestLocal_HealthCheck(t *testing.T) {
	p := newLocalForTest(t)
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Errorf("HealthCheck: %v", err)
	}
	// Removing the work dir should make HealthCheck fail.
	if err := os.RemoveAll(p.workDir); err != nil {
		t.Fatalf("remove work dir: %v", err)
	}
	if err := p.HealthCheck(context.Background()); err == nil {
		t.Errorf("HealthCheck after remove: got nil error, want one")
	}
}

func TestLocal_CollectArtifacts_RejectsBadExtension(t *testing.T) {
	p := newLocalForTest(t)
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer p.DestroyInstance(context.Background(), inst)
	// Drop an unsupported extension into the artifacts dir.
	artDir := filepath.Join(p.workDir, inst.InstanceID, "artifacts")
	if err := os.WriteFile(filepath.Join(artDir, "evil.exe"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	_, err = p.collectArtifacts(p.workDir + "/" + inst.InstanceID)
	if err == nil {
		t.Errorf("collectArtifacts(.exe): got nil error, want one")
	}
	if !strings.Contains(err.Error(), "unsupported artifact type") {
		t.Errorf("err = %v, want to mention 'unsupported artifact type'", err)
	}
}

func TestLocal_CollectArtifacts_AllowsCSVRoundTrip(t *testing.T) {
	p := newLocalForTest(t)
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer p.DestroyInstance(context.Background(), inst)
	artDir := filepath.Join(p.workDir, inst.InstanceID, "artifacts")
	if err := os.WriteFile(filepath.Join(artDir, "out.csv"), []byte("a,b\n1,2\n"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	artifacts, err := p.collectArtifacts(p.workDir + "/" + inst.InstanceID)
	if err != nil {
		t.Fatalf("collectArtifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("got %d artifacts, want 1", len(artifacts))
	}
	got := artifacts[0]
	if got["name"] != "out.csv" {
		t.Errorf("name = %v, want out.csv", got["name"])
	}
	// "a,b\n1,2\n" is 8 bytes (a, comma, b, \n, 1, comma, 2, \n).
	if got["size"].(int64) != 8 {
		t.Errorf("size = %v, want 8", got["size"])
	}
	decoded, _ := base64.StdEncoding.DecodeString(got["content_b64"].(string))
	// "a,b\n1,2\n" is 8 bytes (a, comma, b, \n, 1, comma, 2, \n).
	if got["size"].(int64) != 8 {
		t.Errorf("size = %v, want 8", got["size"])
	}
	if string(decoded) != "a,b\n1,2\n" {
		t.Errorf("content mismatch: %q", decoded)
	}
}

func TestShq(t *testing.T) {
	t.Parallel()
	// shq replaces ' with \', wraps the result in single quotes.
	// "o'clock" -> 'o\'clock' (raw bytes: ', o, \, ', c, l, o, c, k, ').
	cases := []struct {
		in, want string
	}{
		{"foo", "'foo'"},
		{"o'clock", `'o\'clock'`},
		{"a b c", "'a b c'"},
		{"", "''"},
	}
	for _, tc := range cases {
		if got := shq(tc.in); got != tc.want {
			t.Errorf("shq(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStatusFromExitCode(t *testing.T) {
	if got := statusFromExitCode(0); got != "ok" {
		t.Errorf("0 -> %q, want ok", got)
	}
	if got := statusFromExitCode(1); got != "error" {
		t.Errorf("1 -> %q, want error", got)
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("RAGFLOW_TEST_ENVOR", "x")
	if got := envOr("RAGFLOW_TEST_ENVOR", "fallback"); got != "x" {
		t.Errorf("got %q, want x", got)
	}
	if got := envOr("RAGFLOW_TEST_ENVOR_UNSET", "fallback"); got != "fallback" {
		t.Errorf("got %q, want fallback", got)
	}
}

func TestEnvIntOr(t *testing.T) {
	t.Setenv("RAGFLOW_TEST_ENVINTOR", "42")
	if got := envIntOr("RAGFLOW_TEST_ENVINTOR", 10); got != 42 {
		t.Errorf("got %d, want 42", got)
	}
	if got := envIntOr("RAGFLOW_TEST_ENVINTOR_UNSET", 10); got != 10 {
		t.Errorf("got %d, want 10", got)
	}
	t.Setenv("RAGFLOW_TEST_ENVINTOR", "garbage")
	if got := envIntOr("RAGFLOW_TEST_ENVINTOR", 10); got != 10 {
		t.Errorf("garbage value: got %d, want fallback 10", got)
	}
}

// findBinary searches PATH for a binary. Used by tests that
// skip on missing runtime dependencies.
func findBinary(name string) (string, error) {
	paths := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))
	for _, dir := range paths {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", os.ErrNotExist
}

// ensure time.Duration is referenced (avoid unused import if
// we later remove other test helpers).
var _ = time.Second
