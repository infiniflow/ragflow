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
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// startSSHExecServer runs an in-process SSH server whose exec'd command
// writes stdoutBytes of filler to stdout and exits 0. It returns the
// address to dial and the PEM-encoded private key the client must offer.
func startSSHExecServer(t *testing.T, stdoutBytes int) (addr string, keyPEM []byte) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(key, "test", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, presented ssh.PublicKey) (*ssh.Permissions, error) {
			if string(presented.Marshal()) != string(signer.PublicKey().Marshal()) {
				return nil, os.ErrPermission
			}
			return nil, nil
		},
	}
	cfg.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveSSHExecConn(conn, cfg, stdoutBytes)
		}
	}()

	return listener.Addr().String(), pem.EncodeToMemory(block)
}

func serveSSHExecConn(conn net.Conn, cfg *ssh.ServerConfig, stdoutBytes int) {
	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)
	for newCh := range chans {
		if newCh.ChannelType() != "session" {
			_ = newCh.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		ch, chReqs, err := newCh.Accept()
		if err != nil {
			continue
		}
		go func(ch ssh.Channel, chReqs <-chan *ssh.Request) {
			defer ch.Close()
			for req := range chReqs {
				if req.Type != "exec" {
					if req.WantReply {
						_ = req.Reply(false, nil)
					}
					continue
				}
				if req.WantReply {
					_ = req.Reply(true, nil)
				}
				chunk := []byte(strings.Repeat("x", 32*1024))
				for written := 0; written < stdoutBytes; {
					n := min(len(chunk), stdoutBytes-written)
					if _, err := ch.Write(chunk[:n]); err != nil {
						return
					}
					written += n
				}
				_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
				return
			}
		}(ch, chReqs)
	}
}

// dialSSHExecServer wires a provider to startSSHExecServer and returns an
// authenticated client, with known_hosts already trusted.
func dialSSHExecServer(t *testing.T, addr string, keyPEM []byte) (*SSHProvider, *ssh.Client) {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKeyWithPassphrase(keyPEM, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	knownHosts := filepath.Join(home, ".ssh", "known_hosts")
	line := knownhosts.Line([]string{addr}, signer.PublicKey()) + "\n"
	if err := os.WriteFile(knownHosts, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	p := newSSHProviderFromConfig(map[string]any{
		"host": host, "port": port, "username": "test",
		"private_key": string(keyPEM), "passphrase": "secret",
	})
	client, err := p.dial(t.Context())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return p, client
}

// TestSSH_RunRemoteCommand_BoundsRetainedOutput is the regression test for
// unbounded output buffering. runRemoteCommand used to attach an unbounded
// strings.Builder to sess.Stdout, so a remote command that printed without
// limit inflated this process's heap in full before max_output_bytes was
// ever consulted. ExecuteCode runs caller-supplied code on that remote host,
// so the flood is attacker-controlled.
func TestSSH_RunRemoteCommand_BoundsRetainedOutput(t *testing.T) {
	const (
		floodBytes  = 8 << 20 // what the remote host pushes
		budgetBytes = 64 << 10
	)
	addr, keyPEM := startSSHExecServer(t, floodBytes)
	p, client := dialSSHExecServer(t, addr, keyPEM)

	stdout, stderr, _, err := p.runRemoteCommand(t.Context(), client, "flood", 30, budgetBytes)
	if err == nil {
		t.Fatal("runRemoteCommand accepted oversized output, want an overflow error")
	}
	if !strings.Contains(err.Error(), "output exceeds 65536 bytes") {
		t.Errorf("err = %v, want it to report the 65536-byte cap", err)
	}
	// The point of the fix: the retained buffer stops at the cap instead of
	// holding the whole flood. Check both streams.
	if len(stdout) > budgetBytes {
		t.Errorf("retained %d bytes of stdout, want at most %d", len(stdout), budgetBytes)
	}
	if len(stderr) > budgetBytes {
		t.Errorf("retained %d bytes of stderr, want at most %d", len(stderr), budgetBytes)
	}
}

// TestSSH_RunRemoteCommand_ReportsTrueTotalWhenTruncated guards the
// detection side of the fix. The cap bounds what is retained, so the
// overflow has to be recognised from the bytes that were dropped rather
// than from the truncated length — otherwise the bound would silently
// turn every oversized stream into a passing result.
func TestSSH_RunRemoteCommand_ReportsTrueTotalWhenTruncated(t *testing.T) {
	const (
		floodBytes  = 1 << 20
		budgetBytes = 4 << 10
	)
	addr, keyPEM := startSSHExecServer(t, floodBytes)
	p, client := dialSSHExecServer(t, addr, keyPEM)

	_, _, _, err := p.runRemoteCommand(t.Context(), client, "flood", 30, budgetBytes)
	if err == nil {
		t.Fatal("runRemoteCommand accepted oversized output, want an overflow error")
	}
	if want := "got 1048576"; !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want it to report the true total %q", err, want)
	}
}

// TestSSH_RunRemoteCommand_WithinLimitIsUnaffected keeps the bound from
// changing the ordinary path: output at or under the cap is returned whole
// with no error.
func TestSSH_RunRemoteCommand_WithinLimitIsUnaffected(t *testing.T) {
	const payload = 4 << 10
	addr, keyPEM := startSSHExecServer(t, payload)
	p, client := dialSSHExecServer(t, addr, keyPEM)

	stdout, stderr, exitCode, err := p.runRemoteCommand(t.Context(), client, "small", 30, 64<<10)
	if err != nil {
		t.Fatalf("runRemoteCommand: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
	if len(stdout) != payload {
		t.Errorf("stdout length = %d, want %d", len(stdout), payload)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

// TestSSH_RunRemoteCommand_UnlimitedBudgetRetainsAll documents that a
// provider configured without max_output_bytes is unaffected: the bound
// only applies when a positive budget is passed.
func TestSSH_RunRemoteCommand_UnlimitedBudgetRetainsAll(t *testing.T) {
	const payload = 64 << 10
	addr, keyPEM := startSSHExecServer(t, payload)
	p, client := dialSSHExecServer(t, addr, keyPEM)

	stdout, _, _, err := p.runRemoteCommand(t.Context(), client, "flood", 30, 0)
	if err != nil {
		t.Fatalf("runRemoteCommand with no budget: %v", err)
	}
	if len(stdout) != payload {
		t.Errorf("stdout length = %d, want %d", len(stdout), payload)
	}
}
