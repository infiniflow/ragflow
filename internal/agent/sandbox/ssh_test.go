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
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSSH_ProviderTypeAndLanguages(t *testing.T) {
	t.Parallel()
	p := newSSHProviderFromEnv()
	if p.ProviderType() != ProviderSSH {
		t.Errorf("ProviderType = %q, want %q", p.ProviderType(), ProviderSSH)
	}
	langs := p.SupportedLanguages()
	want := map[string]bool{"python": true, "nodejs": true, "javascript": true}
	if len(langs) != len(want) {
		t.Errorf("SupportedLanguages count = %d, want %d", len(langs), len(want))
	}
	got := make(map[string]bool, len(langs))
	for _, l := range langs {
		got[l] = true
		if !want[l] {
			t.Errorf("unexpected language: %q", l)
		}
	}
	for l := range want {
		if !got[l] {
			t.Errorf("missing expected language: %q", l)
		}
	}
}

func TestSSH_EnvDefaults(t *testing.T) {
	for _, k := range []string{
		"SSH_HOST", "SSH_PORT", "SSH_USERNAME",
		"SSH_PYTHON_BIN", "SSH_NODE_BIN", "SSH_WORK_DIR",
		"SSH_TIMEOUT",
	} {
		t.Setenv(k, "")
	}
	p := newSSHProviderFromEnv()
	if p.pythonBin != sshDefaultPythonBin {
		t.Errorf("pythonBin = %q, want %q", p.pythonBin, sshDefaultPythonBin)
	}
	if p.nodeBin != sshDefaultNodeBin {
		t.Errorf("nodeBin = %q, want %q", p.nodeBin, sshDefaultNodeBin)
	}
	if p.workDir != sshDefaultWorkDir {
		t.Errorf("workDir = %q, want %q", p.workDir, sshDefaultWorkDir)
	}
	if p.port != sshDefaultPort {
		t.Errorf("port = %d, want %d", p.port, sshDefaultPort)
	}
	if p.timeout != sshDefaultTimeout {
		t.Errorf("timeout = %d, want %d", p.timeout, sshDefaultTimeout)
	}
}

func TestSSH_PrivateKeyFromPath(t *testing.T) {
	keyPath := t.TempDir() + "/id_rsa"
	keyContent := "-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----\n"
	if err := os.WriteFile(keyPath, []byte(keyContent), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	t.Setenv("SSH_PRIVATE_KEY", "")
	t.Setenv("SSH_PRIVATE_KEY_PATH", keyPath)
	p := newSSHProviderFromEnv()
	if string(p.privateKey) != keyContent {
		t.Errorf("privateKey not loaded from SSH_PRIVATE_KEY_PATH")
	}
}

func TestSSH_PrivateKeyInline(t *testing.T) {
	inline := "-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----\n"
	t.Setenv("SSH_PRIVATE_KEY", inline)
	t.Setenv("SSH_PRIVATE_KEY_PATH", "")
	p := newSSHProviderFromEnv()
	if string(p.privateKey) != inline {
		t.Errorf("privateKey not loaded from SSH_PRIVATE_KEY (inline takes precedence)")
	}
}

func TestSSH_Initialize_MissingHost(t *testing.T) {
	t.Setenv("SSH_HOST", "")
	t.Setenv("SSH_USERNAME", "u")
	t.Setenv("SSH_PASSWORD", "p")
	p := newSSHProviderFromEnv()
	if err := p.Initialize(context.Background()); err == nil {
		t.Errorf("Initialize with empty host: got nil error, want one")
	} else if !strings.Contains(err.Error(), "SSH_HOST") {
		t.Errorf("err = %v, want to mention SSH_HOST", err)
	}
}

func TestSSH_Initialize_MissingUsername(t *testing.T) {
	t.Setenv("SSH_HOST", "h")
	t.Setenv("SSH_USERNAME", "")
	t.Setenv("SSH_PASSWORD", "p")
	p := newSSHProviderFromEnv()
	if err := p.Initialize(context.Background()); err == nil {
		t.Errorf("Initialize with empty username: got nil error, want one")
	}
}

func TestSSH_Initialize_MissingAuth(t *testing.T) {
	t.Setenv("SSH_HOST", "h")
	t.Setenv("SSH_USERNAME", "u")
	t.Setenv("SSH_PASSWORD", "")
	t.Setenv("SSH_PRIVATE_KEY", "")
	t.Setenv("SSH_PRIVATE_KEY_PATH", "")
	p := newSSHProviderFromEnv()
	if err := p.Initialize(context.Background()); err == nil {
		t.Errorf("Initialize with no auth: got nil error, want one")
	} else if !strings.Contains(err.Error(), "SSH_PASSWORD") {
		t.Errorf("err = %v, want to mention SSH_PASSWORD", err)
	}
}

func TestSSH_AllOps_BeforeInit(t *testing.T) {
	t.Parallel()
	p := &SSHProvider{}
	inst := &SandboxInstance{InstanceID: "x", Provider: ProviderSSH}
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

func TestSSH_CreateInstance_RejectsBadLanguage(t *testing.T) {
	t.Setenv("SSH_HOST", "h")
	t.Setenv("SSH_USERNAME", "u")
	t.Setenv("SSH_PASSWORD", "p")
	p := newSSHProviderFromEnv()
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if _, err := p.CreateInstance(context.Background(), "ruby"); err == nil {
		t.Errorf("CreateInstance(ruby): got nil error, want one")
	}
}

// TestSSH_Dial_ConnectionRefused verifies that dial() surfaces
// a clear error when the host is unreachable. We bind then close
// an ephemeral listener to obtain a guaranteed-closed port.
func TestSSH_Dial_ConnectionRefused(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for ephemeral port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	t.Setenv("SSH_HOST", "127.0.0.1")
	t.Setenv("SSH_PORT", strconv.Itoa(port))
	t.Setenv("SSH_USERNAME", "u")
	t.Setenv("SSH_PASSWORD", "p")
	t.Setenv("SSH_TIMEOUT", "2")
	p := newSSHProviderFromEnv()
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := p.dial(ctx); err == nil {
		t.Errorf("dial: got nil error, want one")
	}
}

func TestSSH_ExecuteCode_RejectsBadInputs(t *testing.T) {
	t.Setenv("SSH_HOST", "h")
	t.Setenv("SSH_USERNAME", "u")
	t.Setenv("SSH_PASSWORD", "p")
	p := newSSHProviderFromEnv()
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

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
			name: "unknown instance id",
			fn: func() error {
				_, err := p.ExecuteCode(context.Background(),
					&SandboxInstance{InstanceID: "nope"}, "x", "python", 5, nil)
				return err
			},
			want: "unknown instance",
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

func TestShq_Extra(t *testing.T) {
	t.Parallel()
	// Backtick, dollar sign, newline — all must be safely quoted.
	cases := []struct {
		in, want string
	}{
		{"$VAR", "'$VAR'"},
		{"`cmd`", "'`cmd`'"},
		{"a\nb", "'a\nb'"},
		{"path with spaces", "'path with spaces'"},
	}
	for _, tc := range cases {
		if got := shq(tc.in); got != tc.want {
			t.Errorf("shq(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMinTimeout(t *testing.T) {
	if got := minTimeout(10, 5); got != 5 {
		t.Errorf("minTimeout(10, 5) = %d, want 5", got)
	}
	if got := minTimeout(3, 7); got != 3 {
		t.Errorf("minTimeout(3, 7) = %d, want 3", got)
	}
	if got := minTimeout(0, 5); got != 1 {
		t.Errorf("minTimeout(0, 5) = %d, want 1 (floor is 1)", got)
	}
}
