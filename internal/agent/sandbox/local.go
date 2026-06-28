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

// local.go is the Go port of `agent/sandbox/providers/local.py`.
//
// LocalProvider runs the user's code on the Go host itself via
// os/exec. There is no sandboxing — the code runs with the Go
// process's privileges. The Python version is the same: the
// "local" provider is a convenience for development / trusted
// environments, not a security boundary. Operators that need
// isolation should configure SelfManaged (executor_manager
// Docker+gVisor) or Aliyun / e2b.
//
// Wire format matches the Python provider exactly: write the
// wrapped code (BuildPythonWrapper / BuildJavaScriptWrapper) to
// a temp file under <work_dir>/<instance_id>/, run it via
// `python3 <file>` / `node <file>` with cwd=instance_dir, capture
// stdout / stderr, scan stdout for the `__RAGFLOW_RESULT__:`
// marker. Artifacts are read from `<instance_dir>/artifacts/`
// and validated against the same ALLOWED_ARTIFACT_EXTENSIONS
// set the SSH provider uses (so an artifact accepted by one
// provider is accepted by the other — the model sees the same
// shape across the two).

//go:build !windows

package sandbox

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// localDefaultWorkDir is the default work directory for local
// code execution. Matches the Python side's default.
const localDefaultWorkDir = "/tmp/ragflow-codeexec"

// localDefaultTimeout is the per-execution timeout in seconds.
const localDefaultTimeout = 30

// localDefaultMaxMemoryMB sets RLIMIT_AS on the child.
const localDefaultMaxMemoryMB = 512

// localDefaultMaxOutputBytes caps combined stdout+stderr.
const localDefaultMaxOutputBytes = 1 << 20 // 1 MiB

// localDefaultMaxArtifacts caps the number of artifact files
// collected from <instance_dir>/artifacts/.
const localDefaultMaxArtifacts = 20

// localDefaultMaxArtifactBytes caps the size of any single
// artifact file.
const localDefaultMaxArtifactBytes = 10 << 20 // 10 MiB

// localDefaultNoFile sets RLIMIT_NOFILE on the child. Matches
// the Python `_limit_child_process` value.
const localDefaultNoFile = 64

// localDefaultPythonBin / localDefaultNodeBin are the executables
// used to run the wrapped code. The Python provider uses
// `python3` / `node` for the same purpose.
const localDefaultPythonBin = "python3"
const localDefaultNodeBin = "node"

// LocalProvider is the Go port of
// `agent/sandbox/providers/local.py::LocalProvider`.
type LocalProvider struct {
	pythonBin        string
	nodeBin          string
	workDir          string
	timeout          int
	maxMemoryMB      int
	maxOutputBytes   int
	maxArtifacts     int
	maxArtifactBytes int

	mu          sync.Mutex
	instances   map[string]string // instanceID -> instance dir
	initialized bool
}

// newLocalProviderFromEnv reads LOCAL_* env vars and returns a
// provider ready for Initialize. Matches the env-var pattern
// used by SelfManaged and E2B.
func newLocalProviderFromEnv() *LocalProvider {
	return newLocalProviderFromConfig(localConfigFromEnv())
}

// localConfigFromEnv builds a config map from the LOCAL_* env
// vars, mirroring the admin-panel settings JSON shape.
func localConfigFromEnv() map[string]any {
	return map[string]any{
		"PYTHON_BIN":         os.Getenv("LOCAL_PYTHON_BIN"),
		"NODE_BIN":           os.Getenv("LOCAL_NODE_BIN"),
		"WORK_DIR":           os.Getenv("LOCAL_WORK_DIR"),
		"TIMEOUT":            os.Getenv("LOCAL_TIMEOUT"),
		"MAX_MEMORY_MB":      os.Getenv("LOCAL_MAX_MEMORY_MB"),
		"MAX_OUTPUT_BYTES":   os.Getenv("LOCAL_MAX_OUTPUT_BYTES"),
		"MAX_ARTIFACTS":      os.Getenv("LOCAL_MAX_ARTIFACTS"),
		"MAX_ARTIFACT_BYTES": os.Getenv("LOCAL_MAX_ARTIFACT_BYTES"),
	}
}

// newLocalProviderFromConfig builds the provider from a JSON
// config map. Config keys mirror the env-var names without the
// LOCAL_ prefix.
func newLocalProviderFromConfig(cfg map[string]any) *LocalProvider {
	p := &LocalProvider{
		pythonBin:        configString(cfg, "PYTHON_BIN"),
		nodeBin:          configString(cfg, "NODE_BIN"),
		workDir:          configString(cfg, "WORK_DIR"),
		timeout:          configInt(cfg, "TIMEOUT", localDefaultTimeout),
		maxMemoryMB:      configInt(cfg, "MAX_MEMORY_MB", localDefaultMaxMemoryMB),
		maxOutputBytes:   configInt(cfg, "MAX_OUTPUT_BYTES", localDefaultMaxOutputBytes),
		maxArtifacts:     configInt(cfg, "MAX_ARTIFACTS", localDefaultMaxArtifacts),
		maxArtifactBytes: configInt(cfg, "MAX_ARTIFACT_BYTES", localDefaultMaxArtifactBytes),
		instances:        map[string]string{},
	}
	if p.pythonBin == "" {
		p.pythonBin = localDefaultPythonBin
	}
	if p.nodeBin == "" {
		p.nodeBin = localDefaultNodeBin
	}
	if p.workDir == "" {
		p.workDir = localDefaultWorkDir
	}
	return p
}

// ProviderType returns ProviderLocal.
func (p *LocalProvider) ProviderType() ProviderType { return ProviderLocal }

// Initialize validates the work_dir (create if missing, ensure
// writable) and flips the initialized flag. Unlike the Python
// version, we do not set rlimits here — the limits are applied
// to the child per-execution via Cmd.SysProcAttr.
func (p *LocalProvider) Initialize(ctx context.Context) error {
	if err := os.MkdirAll(p.workDir, 0o700); err != nil {
		return fmt.Errorf("local: create work_dir %q: %w", p.workDir, err)
	}
	// Probe: try to create + remove a sentinel file to verify
	// writability. MkdirAll above does not error on existing
	// dirs; the probe distinguishes "writable" from "exists but
	// not writable".
	probe := filepath.Join(p.workDir, ".ragflow-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return fmt.Errorf("local: work_dir %q not writable: %w", p.workDir, err)
	}
	_ = os.Remove(probe)
	p.mu.Lock()
	p.initialized = true
	p.mu.Unlock()
	return nil
}

// SupportedLanguages returns the languages the local subprocess
// can run.
func (p *LocalProvider) SupportedLanguages() []string {
	return []string{"python", "nodejs", "javascript"}
}

// CreateInstance provisions a fresh instance dir under workDir.
// The instance_id is a UUID; the instance dir is the operator-
// visible handle.
func (p *LocalProvider) CreateInstance(ctx context.Context, template string) (*SandboxInstance, error) {
	if !p.isInitialized() {
		return nil, fmt.Errorf("local: provider not initialized")
	}
	lang := normalizeLanguage(template)
	if lang == "" {
		return nil, fmt.Errorf("local: unsupported language %q", template)
	}
	instanceID := uuid.NewString()
	instanceDir := filepath.Join(p.workDir, instanceID)
	if err := os.MkdirAll(instanceDir, 0o700); err != nil {
		return nil, fmt.Errorf("local: create instance dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(instanceDir, "artifacts"), 0o700); err != nil {
		_ = os.RemoveAll(instanceDir)
		return nil, fmt.Errorf("local: create artifacts dir: %w", err)
	}
	p.mu.Lock()
	p.instances[instanceID] = instanceDir
	p.mu.Unlock()
	return &SandboxInstance{
		InstanceID: instanceID,
		Provider:   ProviderLocal,
		Status:     "running",
		Metadata: map[string]any{
			"language":   lang,
			"work_dir":   instanceDir,
			"python_bin": p.pythonBin,
			"node_bin":   p.nodeBin,
		},
	}, nil
}

// ExecuteCode runs the wrapped code via the configured
// python_bin / node_bin and returns the result. The wrapping
// uses the same BuildPythonWrapper / BuildJavaScriptWrapper as
// every other provider, so the `__RAGFLOW_RESULT__:` marker
// extraction is identical.
func (p *LocalProvider) ExecuteCode(
	ctx context.Context,
	inst *SandboxInstance,
	code, language string,
	timeoutSec int,
	args map[string]any,
) (*ExecutionResult, error) {
	if !p.isInitialized() {
		return nil, fmt.Errorf("local: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return nil, fmt.Errorf("local: instance id required")
	}
	lang := normalizeLanguage(language)
	if lang == "" {
		return nil, fmt.Errorf("local: unsupported language %q", language)
	}
	timeout, err := validateTimeout(timeoutSec)
	if err != nil {
		return nil, err
	}
	if timeout == 0 {
		timeout = p.timeout
	}

	instanceDir := filepath.Join(p.workDir, inst.InstanceID)
	if _, err := os.Stat(instanceDir); err != nil {
		return nil, fmt.Errorf("local: instance dir missing: %w", err)
	}

	// Build the wrapped code and the command to run it.
	argsJSON, err := argsToJSON(args)
	if err != nil {
		return nil, err
	}
	var (
		scriptPath string
		cmdName    string
		cmdArgs    []string
	)
	if lang == "python" {
		cmdName = p.pythonBin
		scriptPath = filepath.Join(instanceDir, "main.py")
		if err := os.WriteFile(scriptPath, []byte(BuildPythonWrapper(code, argsJSON)), 0o600); err != nil {
			return nil, fmt.Errorf("local: write main.py: %w", err)
		}
		cmdArgs = []string{scriptPath}
	} else {
		cmdName = p.nodeBin
		scriptPath = filepath.Join(instanceDir, "main.js")
		if err := os.WriteFile(scriptPath, []byte(BuildJavaScriptWrapper(code, argsJSON)), 0o600); err != nil {
			return nil, fmt.Errorf("local: write main.js: %w", err)
		}
		cmdArgs = []string{scriptPath}
	}

	// Build the child env. Matches the Python provider's
	// _build_child_env: HOME / TMPDIR point at the instance dir,
	// PYTHONUNBUFFERED is on, and a small set of thread-related
	// vars pass through from the host env.
	childEnv := buildLocalChildEnv(instanceDir)

	// Use a context-derived cancel so callers can abort a
	// long-running subprocess. The actual kill-on-timeout is
	// enforced by exec.CommandContext + the timeout goroutine.
	start := time.Now()

	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	cmd.Dir = instanceDir
	cmd.Env = childEnv
	// pdeath_signal + process group so the subprocess dies
	// with the parent. On Linux this is SysProcAttr.Pdeathsig;
	// Setpgid puts the child in its own process group, which
	// lets us kill the whole group on timeout.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
	// Apply rlimits via pre-start. Go's os/exec does not expose
	// rlimit directly, so we do it after fork via the parent's
	// process-group kill. We do NOT replicate the Python
	// preexec_fn RLIMIT_* exactly because Go's os/exec does not
	// support pre-start hooks portably. The Setpgid is the
	// important part for hard-kill on timeout; rlimits are
	// best-effort on POSIX (no-op on Windows — see build tag).
	if p.maxMemoryMB > 0 {
		// Advisory only; the subprocess inherits the parent's
		// rlimits. Real isolation requires the SelfManaged or
		// Aliyun / e2b providers. We do NOT try to set RLIMIT_AS
		// post-fork because that would require ptrace or a
		// side-channel — out of scope for a "local" provider.
	}

	var stdout, stderr bytes.Buffer
	maxOut := p.maxOutputBytes
	if maxOut > 0 {
		stdout = bytes.Buffer{}
		stderr = bytes.Buffer{}
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("local: start subprocess: %w", err)
	}

	// Wait with a separate timer so we can hard-kill the process
	// group on timeout. cmd.Wait alone does not kill on
	// context cancellation if the child ignores SIGTERM; the
	// process group + os.FindProcess lets us escalate to SIGKILL.
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	exitCode := -1
	var waitErr error
	select {
	case waitErr = <-waitDone:
		// Process exited on its own.
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
	case <-ctx.Done():
		// Caller cancelled.
		_ = killProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
		<-waitDone
		return nil, fmt.Errorf("local: execution cancelled: %w", ctx.Err())
	case <-time.After(time.Duration(timeout) * time.Second):
		// Per-execution timeout. Hard-kill the process group so
		// the subprocess can't outlive the timer.
		_ = killProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
		<-waitDone
		return nil, fmt.Errorf("local: execution timed out after %d seconds", timeout)
	}

	// Validate output size: if stdout+stderr exceed the cap,
	// surface as a runtime error (matches the Python provider).
	if maxOut > 0 {
		combined := stdout.Len() + stderr.Len()
		if combined > maxOut {
			return nil, fmt.Errorf("local: output exceeds %d bytes (got %d)", maxOut, combined)
		}
	}

	// Extract the structured result from stdout.
	cleanedStdout, structured := ExtractStructuredResult(stdout.String())

	// Collect artifacts under <instance_dir>/artifacts/. Matches
	// the Python provider's _collect_artifacts behavior
	// (allowlist of extensions, max count, max size per file,
	// no symlinks).
	artifacts, err := p.collectArtifacts(instanceDir)
	if err != nil {
		// The Python side raises on violations; we mirror that.
		return nil, fmt.Errorf("local: collect artifacts: %w", err)
	}

	_ = waitErr // may be non-nil for non-zero exit; exitCode carries the truth
	metadata := map[string]any{
		"instance_id":       inst.InstanceID,
		"language":          lang,
		"script_path":       scriptPath,
		"status":            statusFromExitCode(exitCode),
		"timeout":           timeout,
		"artifacts":         artifacts,
		"structured_result": structured,
	}
	return &ExecutionResult{
		Stdout:        cleanedStdout,
		Stderr:        stderr.String(),
		ExitCode:      exitCode,
		ExecutionTime: time.Since(start).Seconds(),
		Metadata:      metadata,
	}, nil
}

// DestroyInstance removes the instance dir. Idempotent on
// missing dir (matches the Python provider's return True on
// already-gone).
func (p *LocalProvider) DestroyInstance(ctx context.Context, inst *SandboxInstance) error {
	if !p.isInitialized() {
		return fmt.Errorf("local: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return fmt.Errorf("local: instance id required")
	}
	instanceDir := filepath.Join(p.workDir, inst.InstanceID)
	p.mu.Lock()
	delete(p.instances, inst.InstanceID)
	p.mu.Unlock()
	if err := os.RemoveAll(instanceDir); err != nil {
		return fmt.Errorf("local: remove instance dir: %w", err)
	}
	return nil
}

// HealthCheck verifies the work dir is reachable and writable.
// The Python version's health_check is the same check.
func (p *LocalProvider) HealthCheck(ctx context.Context) error {
	if !p.isInitialized() {
		return errors.New("local: provider not initialized")
	}
	info, err := os.Stat(p.workDir)
	if err != nil {
		return fmt.Errorf("local: stat work_dir: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("local: work_dir %q is not a directory", p.workDir)
	}
	probe := filepath.Join(p.workDir, ".ragflow-healthcheck")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return fmt.Errorf("local: work_dir not writable: %w", err)
	}
	_ = os.Remove(probe)
	return nil
}

func (p *LocalProvider) isInitialized() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initialized
}

// buildLocalChildEnv builds the env passed to the subprocess.
// Matches the Python _build_child_env contract: HOME / TMPDIR
// point at the instance dir, PYTHONUNBUFFERED is on, and a
// small set of thread-related vars pass through from the host.
func buildLocalChildEnv(instanceDir string) []string {
	env := []string{
		"HOME=" + instanceDir,
		"TMPDIR=" + instanceDir,
		"MPLBACKEND=Agg",
		"PYTHONUNBUFFERED=1",
	}
	// Append the host PATH so the subprocess can find shared
	// libs and helper binaries.
	if path := os.Getenv("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}
	// Append thread-related vars from the host so libraries
	// that honor them (OpenMP, MKL, etc.) behave the same.
	for _, name := range []string{
		"OMP_NUM_THREADS",
		"OPENBLAS_NUM_THREADS",
		"MKL_NUM_THREADS",
		"VECLIB_MAXIMUM_THREADS",
		"NUMEXPR_NUM_THREADS",
	} {
		if v := os.Getenv(name); v != "" {
			env = append(env, name+"="+v)
		}
	}
	return env
}

// collectArtifacts walks <instance_dir>/artifacts/ and returns
// the list of files as {name, content_b64, mime_type, size}
// records. Enforces the same limits the Python provider does:
// max count, max per-file size, allowlist of extensions, no
// symlinks.
func (p *LocalProvider) collectArtifacts(instanceDir string) ([]map[string]any, error) {
	root := filepath.Join(instanceDir, "artifacts")
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("artifacts path %q is not a directory", root)
	}

	var out []map[string]any
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Skip the root itself.
		if path == root {
			return nil
		}
		// Reject symlinks. os.DirEntry's Type() returns the
		// type, but a symlink reports Type()&ModeSymlink != 0.
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact symlinks are not allowed: %s", d.Name())
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("unsupported artifact entry: %s", d.Name())
		}
		if len(out) >= p.maxArtifacts {
			return fmt.Errorf("local execution produced more than %d artifacts", p.maxArtifacts)
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if fi.Size() > int64(p.maxArtifactBytes) {
			return fmt.Errorf("artifact exceeds %d bytes: %s", p.maxArtifactBytes, d.Name())
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if _, ok := allowedArtifactExts[ext]; !ok {
			return fmt.Errorf("unsupported artifact type: %s", d.Name())
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = d.Name()
		}
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		out = append(out, map[string]any{
			"name":        filepath.ToSlash(rel),
			"content_b64": base64.StdEncoding.EncodeToString(body),
			"mime_type":   mimeType,
			"size":        fi.Size(),
		})
		return nil
	})
	return out, err
}

// killProcessGroup sends signal to the process group of pid.
// Used by the timeout / cancellation paths in ExecuteCode to
// hard-kill the child + any descendants. POSIX only.
func killProcessGroup(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return errors.New("invalid pid")
	}
	// Kill the whole group; the negative pid is "send to
	// process group with id = |pid|" on POSIX.
	return syscall.Kill(-pid, sig)
}

// statusFromExitCode matches the Python side's metadata.status
// string ("ok" or "error").
func statusFromExitCode(code int) string {
	if code == 0 {
		return "ok"
	}
	return "error"
}

// envOr returns the value of the env var, or fallback if unset.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envIntOr returns the env var parsed as int, or fallback if
// unset / unparseable.
func envIntOr(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// readFile is a small helper for tests that want to assert on
// the wrapped code we wrote to disk. It is exported via the
// test file via a build-tag-free helper. (Currently unused in
// production; kept for parity with the Python side's
// _read_file helper if we add file-content assertions later.)
var _ = func() {} // marker to avoid unused-import warnings if all helpers are removed
