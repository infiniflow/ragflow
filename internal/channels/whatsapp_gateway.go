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

package channels

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
	gatewayStopTimeout  = 10 * time.Second
)

type gatewayProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
}

type gatewayRuntime struct {
	mu      sync.Mutex
	process *gatewayProcess
}

var gateway gatewayRuntime

// syncWhatsAppGateway starts or stops the shared WhatsApp gateway process when management is enabled.
func syncWhatsAppGateway(ctx context.Context, enabled bool) error {
	if !enabled {
		return gateway.stop()
	}
	if !gatewayEnabled() {
		return nil
	}
	return gateway.start(ctx)
}

// gatewayEnabled reads the environment switch controlling gateway process management.
func gatewayEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("WHATSAPP_GATEWAY_ENABLED"))
	if raw == "" {
		return false
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
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../api/channels/whatsapp/gateway-node"))
}

// start launches the gateway process if it is not already running.
func (r *gatewayRuntime) start(ctx context.Context) error {
	r.mu.Lock()
	if r.process != nil {
		process := r.process
		select {
		case <-process.done:
			r.process = nil
		default:
			r.mu.Unlock()
			if err := waitForGateway(ctx, gatewayStartTimeout); err != nil {
				r.clearProcess(process)
				_ = stopGatewayProcess(process)
				return err
			}
			return nil
		}
	}
	if gatewayReachable(ctx) {
		r.mu.Unlock()
		return nil
	}

	argv, cwd := gatewayCommand()
	if len(argv) == 0 {
		r.mu.Unlock()
		return errors.New("WhatsApp gateway command is not configured")
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		r.mu.Unlock()
		return err
	}
	process := &gatewayProcess{cmd: cmd, done: make(chan struct{})}
	r.process = process
	r.mu.Unlock()

	go func() {
		err := cmd.Wait()
		close(process.done)

		r.mu.Lock()
		owned := r.process == process
		if r.process == process {
			r.process = nil
		}
		r.mu.Unlock()
		if err != nil && owned {
			log.Printf("whatsapp gateway exited: %v", err)
		}
	}()
	if err := waitForGateway(ctx, gatewayStartTimeout); err != nil {
		r.clearProcess(process)
		_ = stopGatewayProcess(process)
		return err
	}
	return nil
}

// stop terminates the gateway process if this runtime started one.
func (r *gatewayRuntime) stop() error {
	r.mu.Lock()
	process := r.process
	r.process = nil
	r.mu.Unlock()
	return stopGatewayProcess(process)
}

// clearProcess removes process from the runtime if it is still the current managed process.
func (r *gatewayRuntime) clearProcess(process *gatewayProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.process == process {
		r.process = nil
	}
}

// stopGatewayProcess terminates one owned gateway process and waits for it to exit.
func stopGatewayProcess(process *gatewayProcess) error {
	if process == nil || process.cmd == nil || process.cmd.Process == nil {
		return nil
	}
	select {
	case <-process.done:
		return nil
	default:
	}

	if err := process.cmd.Process.Signal(os.Interrupt); err != nil {
		_ = process.cmd.Process.Kill()
		return err
	}
	select {
	case <-process.done:
		return nil
	case <-time.After(gatewayStopTimeout):
		_ = process.cmd.Process.Kill()
	}
	select {
	case <-process.done:
	case <-time.After(2 * time.Second):
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
