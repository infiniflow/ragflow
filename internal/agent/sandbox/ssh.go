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

// ssh.go is the Go port of `agent/sandbox/providers/ssh.py`.
//
// SSHProvider runs the user's code on a remote host via SSH. The
// Go equivalent of Python's `paramiko` library is
// `golang.org/x/crypto/ssh`. The provider opens a single SSH
// client per CodeExec, creates a remote work_dir under the
// configured base, uploads the wrapped code, runs the script
// via `cd <work_dir> && <bin> <script>`, collects artifacts
// from the remote artifacts/ subdir, and tears the workspace
// down on DestroyInstance.
//
// Wire format matches the Python provider: the script is written
// to `<remote_work_dir>/main.py` or `main.js`, and the
// execution command is `cd <work_dir> && <python_bin|node_bin>
// <script_path>`. The `__RAGFLOW_RESULT__:` marker extraction
// works identically across all providers.
//
// File ops use SSH exec (cat heredoc / find / cat | base64) rather
// than the SFTP subsystem. This avoids the
// `github.com/pkg/sftp` dependency and keeps the import surface
// at just `golang.org/x/crypto/ssh` (already a transitive dep).
// The Python side uses SFTP for some operations; the result is
// equivalent functionally. The SFTP path is the obvious next
// step if profiling shows exec overhead is meaningful.

package sandbox

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// sshDefaultTimeout / sshDefaultPort mirror the Python provider
// defaults.
const (
	sshDefaultTimeout      = 30
	sshDefaultPort         = 22
	sshDefaultMaxOutput    = 1 << 20
	sshDefaultMaxArtifacts = 20
	sshDefaultMaxArtifact  = 10 << 20
	sshDefaultPythonBin    = "python3"
	sshDefaultNodeBin      = "node"
	sshDefaultWorkDir      = "/tmp"
)

// SSHProvider is the Go port of
// `agent/sandbox/providers/ssh.py::SSHProvider`.
type SSHProvider struct {
	host             string
	port             int
	username         string
	password         string
	privateKey       []byte
	passphrase       string
	pythonBin        string
	nodeBin          string
	workDir          string
	timeout          int
	maxOutputBytes   int
	maxArtifacts     int
	maxArtifactBytes int
	knownHosts       string

	mu          sync.Mutex
	instances   map[string]*sshInstance
	initialized bool
}

// sshInstance holds the per-connection state. Mirrors the Python
// provider's _instances dict.
type sshInstance struct {
	client        *ssh.Client
	remoteWorkDir string
}

// newSSHProviderFromEnv reads SSH_* env vars and returns a
// provider ready for Initialize. The provider requires host +
// username + (password OR private key) at Initialize time.
func newSSHProviderFromEnv() *SSHProvider {
	return newSSHProviderFromConfig(sshConfigFromEnv())
}

// sshConfigFromEnv builds a config map from the SSH_* env vars.
// PRIVATE_KEY is the literal key contents; PRIVATE_KEY_PATH is
// a path on disk (read at provider-init time). KNOWN_HOSTS is the
// path to an OpenSSH-format known_hosts file used to verify the
// remote host's key (fail-closed when unset).
func sshConfigFromEnv() map[string]any {
	return map[string]any{
		"HOST":               os.Getenv("SSH_HOST"),
		"PORT":               os.Getenv("SSH_PORT"),
		"USERNAME":           os.Getenv("SSH_USERNAME"),
		"PASSWORD":           os.Getenv("SSH_PASSWORD"),
		"PRIVATE_KEY":        os.Getenv("SSH_PRIVATE_KEY"),
		"PRIVATE_KEY_PATH":   os.Getenv("SSH_PRIVATE_KEY_PATH"),
		"PASSPHRASE":         os.Getenv("SSH_PASSPHRASE"),
		"PYTHON_BIN":         os.Getenv("SSH_PYTHON_BIN"),
		"NODE_BIN":           os.Getenv("SSH_NODE_BIN"),
		"WORK_DIR":           os.Getenv("SSH_WORK_DIR"),
		"TIMEOUT":            os.Getenv("SSH_TIMEOUT"),
		"MAX_OUTPUT_BYTES":   os.Getenv("SSH_MAX_OUTPUT_BYTES"),
		"MAX_ARTIFACTS":      os.Getenv("SSH_MAX_ARTIFACTS"),
		"MAX_ARTIFACT_BYTES": os.Getenv("SSH_MAX_ARTIFACT_BYTES"),
		"KNOWN_HOSTS":        os.Getenv("SSH_KNOWN_HOSTS"),
	}
}

// newSSHProviderFromConfig builds the provider from a JSON config
// map. Config keys mirror the env-var names without the SSH_
// prefix. PRIVATE_KEY is the literal key contents (preferred);
// PRIVATE_KEY_PATH is a filesystem path (loaded here, like the
// env path). KNOWN_HOSTS is the path to a known_hosts file used
// to verify the remote host key (required for security; the dial
// fails closed when unset).
func newSSHProviderFromConfig(cfg map[string]any) *SSHProvider {
	p := &SSHProvider{
		host:             configString(cfg, "HOST"),
		port:             configInt(cfg, "PORT", sshDefaultPort),
		username:         configString(cfg, "USERNAME"),
		password:         configString(cfg, "PASSWORD"),
		passphrase:       configString(cfg, "PASSPHRASE"),
		pythonBin:        configString(cfg, "PYTHON_BIN"),
		nodeBin:          configString(cfg, "NODE_BIN"),
		workDir:          configString(cfg, "WORK_DIR"),
		timeout:          configInt(cfg, "TIMEOUT", sshDefaultTimeout),
		maxOutputBytes:   configInt(cfg, "MAX_OUTPUT_BYTES", sshDefaultMaxOutput),
		maxArtifacts:     configInt(cfg, "MAX_ARTIFACTS", sshDefaultMaxArtifacts),
		maxArtifactBytes: configInt(cfg, "MAX_ARTIFACT_BYTES", sshDefaultMaxArtifact),
		knownHosts:       configString(cfg, "KNOWN_HOSTS"),
		instances:        map[string]*sshInstance{},
	}
	if p.pythonBin == "" {
		p.pythonBin = sshDefaultPythonBin
	}
	if p.nodeBin == "" {
		p.nodeBin = sshDefaultNodeBin
	}
	if p.workDir == "" {
		p.workDir = sshDefaultWorkDir
	}
	// Private key: prefer the literal content if set; otherwise
	// read from the path.
	if v := configString(cfg, "PRIVATE_KEY"); v != "" {
		p.privateKey = []byte(v)
	} else if keyPath := configString(cfg, "PRIVATE_KEY_PATH"); keyPath != "" {
		if b, err := os.ReadFile(keyPath); err == nil {
			p.privateKey = b
		}
	}
	return p
}

// ProviderType returns ProviderSSH.
func (p *SSHProvider) ProviderType() ProviderType { return ProviderSSH }

// Initialize validates the config (host, username, auth) and
// flips the initialized flag. The Python side raises
// SandboxProviderConfigError on missing fields; we return a
// plain Go error wrapped with the same intent. We do NOT open
// a connection here — connectivity is verified by HealthCheck
// and by CreateInstance.
func (p *SSHProvider) Initialize(ctx context.Context) error {
	if p.host == "" {
		return errors.New("ssh: SSH_HOST env var is required")
	}
	if p.username == "" {
		return errors.New("ssh: SSH_USERNAME env var is required")
	}
	if p.password == "" && len(p.privateKey) == 0 {
		return errors.New("ssh: SSH_PASSWORD or SSH_PRIVATE_KEY is required")
	}
	if p.port < 1 || p.port > 65535 {
		return fmt.Errorf("ssh: invalid port %d", p.port)
	}
	p.mu.Lock()
	p.initialized = true
	p.mu.Unlock()
	return nil
}

// SupportedLanguages returns the languages the SSH provider
// can run on the remote host. The Python version is the same.
func (p *SSHProvider) SupportedLanguages() []string {
	return []string{"python", "nodejs", "javascript"}
}

// CreateInstance opens a new SSH client, creates a remote
// work_dir under the configured base, and registers the
// instance for later teardown.
func (p *SSHProvider) CreateInstance(ctx context.Context, template string) (*SandboxInstance, error) {
	if !p.isInitialized() {
		return nil, fmt.Errorf("ssh: provider not initialized")
	}
	lang := normalizeLanguage(template)
	if lang == "" {
		return nil, fmt.Errorf("ssh: unsupported language %q", template)
	}
	client, err := p.dial(ctx)
	if err != nil {
		return nil, err
	}

	remoteBase := p.workDir
	remoteWorkDir := path.Join(remoteBase, "ragflow-ssh-"+uuid.NewString())
	// Create the work_dir and an artifacts/ subdir on the remote.
	if err := p.remoteMkdirAll(client, remoteWorkDir); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ssh: mkdir remote work_dir: %w", err)
	}
	if err := p.remoteMkdirAll(client, path.Join(remoteWorkDir, "artifacts")); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ssh: mkdir remote artifacts: %w", err)
	}

	instanceID := uuid.NewString()
	p.mu.Lock()
	p.instances[instanceID] = &sshInstance{
		client:        client,
		remoteWorkDir: remoteWorkDir,
	}
	p.mu.Unlock()
	return &SandboxInstance{
		InstanceID: instanceID,
		Provider:   ProviderSSH,
		Status:     "running",
		Metadata: map[string]any{
			"language":        lang,
			"remote_work_dir": remoteWorkDir,
			"host":            p.host,
			"port":            p.port,
			"username":        p.username,
		},
	}, nil
}

// ExecuteCode uploads the wrapped code to the remote work_dir,
// runs it via `cd <work_dir> && <bin> <script>`, captures
// stdout / stderr, and collects artifacts. The wire format
// matches the Python provider's `_upload_script` +
// `_run_remote_command` + `_collect_artifacts` sequence.
func (p *SSHProvider) ExecuteCode(
	ctx context.Context,
	inst *SandboxInstance,
	code, language string,
	timeoutSec int,
	args map[string]any,
) (*ExecutionResult, error) {
	if !p.isInitialized() {
		return nil, fmt.Errorf("ssh: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return nil, fmt.Errorf("ssh: instance id required")
	}
	lang := normalizeLanguage(language)
	if lang == "" {
		return nil, fmt.Errorf("ssh: unsupported language %q", language)
	}
	timeout, err := validateTimeout(timeoutSec)
	if err != nil {
		return nil, err
	}
	if timeout == 0 {
		timeout = p.timeout
	}

	p.mu.Lock()
	instance, ok := p.instances[inst.InstanceID]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("ssh: unknown instance id %q", inst.InstanceID)
	}

	// Wrap the code + write to remote via heredoc.
	argsJSON, err := argsToJSON(args)
	if err != nil {
		return nil, err
	}
	var (
		scriptName string
		wrapped    string
		bin        string
	)
	if lang == "python" {
		scriptName = "main.py"
		wrapped = BuildPythonWrapper(code, argsJSON)
		bin = p.pythonBin
	} else {
		scriptName = "main.js"
		wrapped = BuildJavaScriptWrapper(code, argsJSON)
		bin = p.nodeBin
	}
	remoteScriptPath := path.Join(instance.remoteWorkDir, scriptName)
	if err := p.remoteWriteFile(instance.client, remoteScriptPath, wrapped); err != nil {
		return nil, fmt.Errorf("ssh: upload script: %w", err)
	}

	// Build the command. We quote the work_dir, the binary, and
	// the script path with shlex-like quoting.
	command := fmt.Sprintf(
		"cd %s && %s %s",
		shq(instance.remoteWorkDir), shq(bin), shq(remoteScriptPath),
	)

	start := time.Now()
	stdout, stderr, exitCode, runErr := p.runRemoteCommand(ctx, instance.client, command, timeout)
	if runErr != nil {
		return nil, fmt.Errorf("ssh: exec: %w", runErr)
	}
	execTime := time.Since(start).Seconds()

	// Validate output size.
	if p.maxOutputBytes > 0 && len(stdout)+len(stderr) > p.maxOutputBytes {
		return nil, fmt.Errorf("ssh: output exceeds %d bytes", p.maxOutputBytes)
	}

	// Extract the structured result from stdout.
	cleanedStdout, structured := ExtractStructuredResult(stdout)

	// Collect artifacts.
	artifacts, err := p.collectArtifacts(instance.client, path.Join(instance.remoteWorkDir, "artifacts"))
	if err != nil {
		return nil, fmt.Errorf("ssh: collect artifacts: %w", err)
	}

	return &ExecutionResult{
		Stdout:        cleanedStdout,
		Stderr:        stderr,
		ExitCode:      exitCode,
		ExecutionTime: execTime,
		Metadata: map[string]any{
			"instance_id":       inst.InstanceID,
			"language":          lang,
			"script_path":       remoteScriptPath,
			"remote_work_dir":   instance.remoteWorkDir,
			"command":           command,
			"status":            statusFromExitCode(exitCode),
			"timeout":           timeout,
			"artifacts":         artifacts,
			"structured_result": structured,
		},
	}, nil
}

// DestroyInstance removes the remote work_dir (via `rm -rf` over
// SSH) and closes the SSH client. Mirrors the Python provider's
// destroy_instance.
func (p *SSHProvider) DestroyInstance(ctx context.Context, inst *SandboxInstance) error {
	if !p.isInitialized() {
		return fmt.Errorf("ssh: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return fmt.Errorf("ssh: instance id required")
	}
	p.mu.Lock()
	instance, ok := p.instances[inst.InstanceID]
	if !ok {
		p.mu.Unlock()
		return nil // already gone — idempotent
	}
	delete(p.instances, inst.InstanceID)
	p.mu.Unlock()

	// Best-effort remote cleanup via SSH exec. The Python side
	// uses `rm -rf` for the same purpose; we mirror that.
	_, _, _, _ = p.runRemoteCommand(ctx, instance.client,
		fmt.Sprintf("rm -rf %s", shq(instance.remoteWorkDir)),
		minTimeout(p.timeout, 10),
	)
	_ = instance.client.Close()
	return nil
}

// HealthCheck verifies SSH connectivity by opening a session
// and running `true`. The Python side's _assert_connectivity
// does the same.
func (p *SSHProvider) HealthCheck(ctx context.Context) error {
	if !p.isInitialized() {
		return errors.New("ssh: provider not initialized")
	}
	client, err := p.dial(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh: open session: %w", err)
	}
	defer sess.Close()
	if err := sess.Run("true"); err != nil {
		return fmt.Errorf("ssh: run health probe: %w", err)
	}
	return nil
}

func (p *SSHProvider) isInitialized() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initialized
}

// dial opens an SSH client. The auth method is password OR
// private key (whichever is set); the Python side accepts the
// same two methods.
func (p *SSHProvider) dial(ctx context.Context) (*ssh.Client, error) {
	auth := []ssh.AuthMethod{}
	if len(p.privateKey) > 0 {
		signer, err := ssh.ParsePrivateKey(p.privateKey)
		if err != nil {
			return nil, fmt.Errorf("ssh: parse private key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if p.password != "" {
		auth = append(auth, ssh.Password(p.password))
	}
	if len(auth) == 0 {
		return nil, errors.New("ssh: no auth method configured")
	}
	hostKeyCallback, err := p.hostKeyCallback()
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            p.username,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback,
		Timeout:         time.Duration(p.timeout) * time.Second,
	}
	addr := net.JoinHostPort(p.host, strconv.Itoa(p.port))
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh: dial %s: %w", addr, err)
	}
	return client, nil
}

// hostKeyCallback builds an ssh.HostKeyCallback backed by an OpenSSH
// known_hosts file. The provider fails closed when no known_hosts
// path is configured: this protects against man-in-the-middle attacks
// on the SSH transport used to run sandboxed code.
func (p *SSHProvider) hostKeyCallback() (ssh.HostKeyCallback, error) {
	if p.knownHosts == "" {
		return nil, errors.New("ssh: KNOWN_HOSTS not configured; refusing to connect without host key verification (set SSH_KNOWN_HOSTS)")
	}
	callback, err := knownhosts.New(p.knownHosts)
	if err != nil {
		return nil, fmt.Errorf("ssh: load known_hosts %q: %w", p.knownHosts, err)
	}
	return callback, nil
}

// runRemoteCommand runs command over SSH and returns
// (stdout, stderr, exit_code, error). The error is non-nil only
// for transport-level failures; non-zero exit codes are reported
// via exit_code, not error.
//
// All in-package callers build the command argument via shq(),
// which single-quote escapes any value so the shell cannot be
// tricked into re-interpreting it (see remoteMkdirAll,
// remoteRemoveAll, remoteReadFile, remoteWriteFile, etc).
func (p *SSHProvider) runRemoteCommand(ctx context.Context, client *ssh.Client, command string, timeoutSec int) (string, string, int, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", "", -1, fmt.Errorf("ssh: open session: %w", err)
	}
	defer sess.Close()
	stdoutBuf, stderrBuf := &strings.Builder{}, &strings.Builder{}
	sess.Stdout = stdoutBuf
	sess.Stderr = stderrBuf
	// from shq()-escaped arguments only (see callers above); user
	// input never reaches the shell unsanitized.
	// codeql[go/command-injection] False positive: command is built
	if err := sess.Run(command); err != nil {
		// ssh.ExitError carries the remote exit code; we surface
		// it as a normal non-zero exit (the caller can branch on
		// the ExitCode field).
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			return stdoutBuf.String(), stderrBuf.String(), exitErr.ExitStatus(), nil
		}
		return stdoutBuf.String(), stderrBuf.String(), -1, err
	}
	return stdoutBuf.String(), stderrBuf.String(), 0, nil
}

// remoteMkdirAll runs `mkdir -p` on the remote. The Python
// side uses paramiko's mkdir + walk-and-mkdir loop; SSH exec
// with `mkdir -p` is simpler and equivalent.
func (p *SSHProvider) remoteMkdirAll(client *ssh.Client, remotePath string) error {
	_, stderr, exitCode, err := p.runRemoteCommand(context.Background(), client,
		fmt.Sprintf("mkdir -p %s", shq(remotePath)),
		minTimeout(p.timeout, 10),
	)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("mkdir -p %s: exit=%d stderr=%q", remotePath, exitCode, stderr)
	}
	return nil
}

// remoteWriteFile writes content to remotePath via a
// `cat > file <<'__RAGFLOW_SSH_EOF__' ... EOF` heredoc. The
// heredoc tag is unique enough to never collide with user
// code (it includes the package name). For very large scripts
// (>1 MiB) this is inefficient vs. SFTP; the threshold is
// intentionally not implemented here — Python's paramiko
// also writes via SFTP for the same reason.
func (p *SSHProvider) remoteWriteFile(client *ssh.Client, remotePath, content string) error {
	const tag = "__RAGFLOW_SSH_EOF__"
	cmd := fmt.Sprintf(
		"cat > %s <<'%s'\n%s\n%s",
		shq(remotePath), tag, content, tag,
	)
	_, stderr, exitCode, err := p.runRemoteCommand(context.Background(), client, cmd, p.timeout)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("write %s: exit=%d stderr=%q", remotePath, exitCode, stderr)
	}
	return nil
}

// remoteReadFile reads a remote file's content as a string.
// Used by collectArtifacts.
func (p *SSHProvider) remoteReadFile(client *ssh.Client, remotePath string) (string, error) {
	stdout, stderr, exitCode, err := p.runRemoteCommand(context.Background(), client,
		fmt.Sprintf("cat %s", shq(remotePath)),
		p.timeout,
	)
	if err != nil {
		return "", err
	}
	if exitCode != 0 {
		return "", fmt.Errorf("read %s: exit=%d stderr=%q", remotePath, exitCode, stderr)
	}
	return stdout, nil
}

// remoteListDir lists a remote directory's entries. The format
// is `name<TAB>size<TAB>mode` per line, sorted lexically by the
// remote `find` call. We use `find` rather than `ls -la` because
// its output is unambiguous across distros (no header rows).
func (p *SSHProvider) remoteListDir(client *ssh.Client, remotePath string) ([]remoteEntry, error) {
	// -mindepth 1 / -maxdepth 1: only direct children, not
	// the dir itself. -printf 'P\t%s\t%m\n' is the GNU find
	// format; the leading P is a literal path placeholder
	// filled in below. -print0 + IFS split is more robust
	// but adds complexity; for the artifact collection use
	// case filenames don't contain newlines, so the simpler
	// format is fine.
	cmd := fmt.Sprintf(
		"find %s -mindepth 1 -maxdepth 1 -printf '%%p\\t%%s\\t%%m\\n'",
		shq(remotePath),
	)
	stdout, stderr, exitCode, err := p.runRemoteCommand(context.Background(), client, cmd, p.timeout)
	if err != nil {
		return nil, err
	}
	if exitCode != 0 {
		// `find` returns non-zero if the dir does not exist
		// (e.g. no artifacts produced). That's expected.
		if strings.Contains(stderr, "No such file or directory") {
			return nil, nil
		}
		return nil, fmt.Errorf("find %s: exit=%d stderr=%q", remotePath, exitCode, stderr)
	}
	var out []remoteEntry
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		size, _ := strconv.ParseInt(parts[1], 10, 64)
		mode, _ := strconv.ParseInt(parts[2], 8, 32) // octal mode
		name := strings.TrimPrefix(parts[0], remotePath+"/")
		out = append(out, remoteEntry{Name: name, Size: size, Mode: mode})
	}
	return out, nil
}

// remoteEntry is one row from remoteListDir.
type remoteEntry struct {
	Name string
	Size int64
	Mode int64
}

// collectArtifacts walks the remote artifacts/ dir and returns
// the list of files as {name, content_b64, mime_type, size}.
// Enforces the same limits the local provider does.
func (p *SSHProvider) collectArtifacts(client *ssh.Client, root string) ([]map[string]any, error) {
	entries, err := p.remoteListDir(client, root)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, e := range entries {
		remote := path.Join(root, e.Name)
		// Mode bits: S_ISDIR = 0o040000, S_ISREG = 0o100000.
		if e.Mode&0o170000 == 0o040000 {
			sub, err := p.collectArtifacts(client, remote)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
			continue
		}
		if e.Mode&0o170000 != 0o100000 {
			return nil, fmt.Errorf("unsupported artifact entry: %s", e.Name)
		}
		if len(out) >= p.maxArtifacts {
			return nil, fmt.Errorf("ssh execution produced more than %d artifacts", p.maxArtifacts)
		}
		if e.Size > int64(p.maxArtifactBytes) {
			return nil, fmt.Errorf("artifact exceeds %d bytes: %s", p.maxArtifactBytes, e.Name)
		}
		ext := strings.ToLower(path.Ext(e.Name))
		if _, ok := allowedArtifactExts[ext]; !ok {
			return nil, fmt.Errorf("unsupported artifact type: %s", e.Name)
		}
		body, err := p.remoteReadFile(client, remote)
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"name":        e.Name,
			"content_b64": base64.StdEncoding.EncodeToString([]byte(body)),
			"mime_type":   mime.TypeByExtension(ext),
			"size":        e.Size,
		})
	}
	return out, nil
}

// shq single-quotes a string for shell-safe inclusion. Matches
// the Python `shlex.quote` behavior the SSH provider uses for
// building `cd <work_dir> && <bin> <script>` commands. The
// escape sequence for an embedded single quote is `\'` (a
// backslash followed by a single quote).
func shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
}

// minTimeout returns the smaller of a and b, with a floor of 1.
func minTimeout(a, b int) int {
	if a < 1 {
		a = 1
	}
	if b < 1 {
		b = 1
	}
	if a < b {
		return a
	}
	return b
}
