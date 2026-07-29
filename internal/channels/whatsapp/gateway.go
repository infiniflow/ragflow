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

package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	gatewayStartTimeout = 30 * time.Second
	gatewayProbeEvery   = 500 * time.Millisecond
)

type gatewayRuntime struct {
	mu  sync.Mutex
	cmd *exec.Cmd
}

var gateway gatewayRuntime

// SyncGateway starts or stops the shared WhatsApp gateway process.
func SyncGateway(ctx context.Context, enabled bool) error {
	if !gatewayEnabled() || !enabled {
		return gateway.stop()
	}
	return gateway.start(ctx)
}

// gatewayEnabled reads the environment switch controlling gateway process management.
func gatewayEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("WHATSAPP_GATEWAY_ENABLED"))
	if raw == "" {
		return true
	}
	v, err := strconv.ParseBool(raw)
	return err == nil && v
}

// gatewayCommand resolves the command and working directory used to start the gateway.
func gatewayCommand() ([]string, string) {
	if raw := strings.TrimSpace(os.Getenv("WHATSAPP_GATEWAY_COMMAND")); raw != "" {
		return strings.Fields(raw), gatewayWorkdir()
	}

	entry := filepath.Join(gatewayWorkdir(), "index.js")
	if _, err := os.Stat(entry); err != nil {
		return nil, gatewayWorkdir()
	}
	return []string{"node", entry}, gatewayWorkdir()
}

// gatewayWorkdir resolves the WhatsApp gateway-node working directory.
func gatewayWorkdir() string {
	if raw := strings.TrimSpace(os.Getenv("WHATSAPP_GATEWAY_WORKDIR")); raw != "" {
		return raw
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../api/channels/whatsapp/gateway-node"))
}

// start launches the gateway process if it is not already running.
func (r *gatewayRuntime) start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cmd != nil && r.cmd.Process != nil && r.cmd.ProcessState == nil {
		return nil
	}
	if gatewayReachable(ctx) {
		return nil
	}

	argv, cwd := gatewayCommand()
	if len(argv) == 0 {
		return errors.New("WhatsApp gateway command is not configured")
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return err
	}
	r.cmd = cmd
	go func() {
		err := cmd.Wait()
		r.mu.Lock()
		if r.cmd == cmd {
			r.cmd = nil
		}
		r.mu.Unlock()
		if err != nil && ctx.Err() == nil {
			log.Printf("whatsapp gateway exited: %v", err)
		}
	}()
	return waitForGateway(ctx, gatewayStartTimeout)
}

// stop terminates the gateway process if this runtime started one.
func (r *gatewayRuntime) stop() error {
	r.mu.Lock()
	cmd := r.cmd
	r.cmd = nil
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if cmd.ProcessState != nil {
		return nil
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	time.Sleep(10 * time.Second)
	if cmd.ProcessState == nil {
		_ = cmd.Process.Kill()
	}
	return nil
}

// gatewayReachable reports whether an externally managed gateway is already listening.
func gatewayReachable(ctx context.Context) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, gatewayHealthURL(), nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// waitForGateway blocks until the gateway health endpoint is ready or times out.
func waitForGateway(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if gatewayReachable(ctx) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("WhatsApp gateway did not become ready at %s within %s", gatewayHealthURL(), timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(gatewayProbeEvery):
		}
	}
}

// gatewayHealthURL returns the health endpoint for the default managed gateway address.
func gatewayHealthURL() string {
	host := strings.TrimSpace(os.Getenv("WHATSAPP_GATEWAY_HOST"))
	if host == "" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(os.Getenv("WHATSAPP_GATEWAY_PORT"))
	if port == "" {
		port = "3005"
	}
	return fmt.Sprintf("http://%s:%s/health", host, port)
}
