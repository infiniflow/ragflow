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

//go:build integration && !ee

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/server"
)

// This configuration must point to isolated test infrastructure: startup runs migrations.
func TestMigrateStartupAndExit(t *testing.T) {
	configPath := os.Getenv("RAGFLOW_SINGLE_PROCESS_TEST_CONFIG")
	if configPath == "" {
		t.Skip("set RAGFLOW_SINGLE_PROCESS_TEST_CONFIG to an isolated service configuration")
	}
	args, err := parseArgsFrom([]string{"server", "--migrate", "--config", configPath, "--log-level", "warn"})
	if err != nil {
		t.Fatal(err)
	}
	args.Ready = func() error { return errors.New("migration mode must not start services") }
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	if err := runServer(ctx, cancel, args); err != nil {
		t.Fatalf("migration startup: %v", err)
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("migration mode did not exit before cancellation: %v", err)
	}
}

// This configuration must point to isolated test infrastructure: startup runs migrations.
func TestSingleProcessStartupAndShutdown(t *testing.T) {
	configPath := os.Getenv("RAGFLOW_SINGLE_PROCESS_TEST_CONFIG")
	if configPath == "" {
		t.Skip("set RAGFLOW_SINGLE_PROCESS_TEST_CONFIG to an isolated service configuration")
	}
	args, err := parseArgsFrom([]string{"server", "--config", configPath, "--log-level", "warn"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	ready := make(chan struct{})
	done := make(chan struct{})
	var runErr error
	args.Ready = func() error { close(ready); return nil }
	go func() {
		defer close(done)
		runErr = runServer(ctx, cancel, args)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
			if runErr != nil {
				t.Errorf("server shutdown: %v", runErr)
			}
		case <-time.After(45 * time.Second):
			t.Error("server did not finish shutting down")
		}
	}()
	select {
	case <-ready:
	case <-done:
		t.Fatalf("server stopped before all services were ready: %v", runErr)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if got := common.GetLogLevel(); got != "warn" {
		t.Errorf("log level after all services are ready = %q, want warn", got)
	}
	config := server.GetConfig()
	client := &http.Client{Timeout: 10 * time.Second}
	for _, address := range []string{
		fmt.Sprintf("http://127.0.0.1:%d/healthz", config.GetAdminServerConfig().HTTPPort),
		fmt.Sprintf("http://127.0.0.1:%d/api/v1/system/healthz", config.GetAPIServerConfig().HTTPPort),
	} {
		response, err := client.Get(address)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s returned %s", address, response.Status)
		}
	}
	// Linux exposes children by thread. No thread should launch a ragflow_server child.
	if entries, err := os.ReadDir("/proc/self/task"); err == nil {
		selfExecutable, executableErr := os.Executable()
		if executableErr != nil {
			t.Fatal(executableErr)
		}
		for _, entry := range entries {
			children, err := os.ReadFile("/proc/self/task/" + entry.Name() + "/children")
			if err != nil {
				continue // A runtime thread can exit between the directory listing and read.
			}
			for _, pid := range strings.Fields(string(children)) {
				executable, err := os.Readlink("/proc/" + pid + "/exe")
				if err == nil && (executable == selfExecutable || strings.Contains(executable, "ragflow_server")) {
					t.Errorf("server spawned child process %s: %s", pid, executable)
				}
			}
		}
	}
}
