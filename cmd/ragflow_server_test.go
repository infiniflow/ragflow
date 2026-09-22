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

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withArgsAndEnv(t *testing.T, args []string, env map[string]string, fn func()) {
	t.Helper()
	oldArgs := os.Args
	os.Args = append([]string{"ragflow_server"}, args...)
	t.Cleanup(func() { os.Args = oldArgs })
	keys := []string{
		"RAGFLOW_MCP_HOST",
		"RAGFLOW_MCP_PORT",
		"RAGFLOW_MCP_LAUNCH_MODE",
		"RAGFLOW_MCP_HOST_API_KEY",
		"RAGFLOW_MCP_ENABLED",
		"RAGFLOW_MCP_TRANSPORT_SSE_ENABLED",
		"RAGFLOW_MCP_TRANSPORT_STREAMABLE_ENABLED",
		"RAGFLOW_MCP_JSON_RESPONSE",
	}
	oldEnv := make(map[string]*string, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			v := value
			oldEnv[key] = &v
		} else {
			oldEnv[key] = nil
		}
		os.Unsetenv(key)
	}
	t.Cleanup(func() {
		for key, value := range oldEnv {
			if value == nil {
				os.Unsetenv(key)
				continue
			}
			os.Setenv(key, *value)
		}
	})
	for key, value := range env {
		os.Setenv(key, value)
	}
	fn()
}

func TestParseArgsMCPEnvOverridesCLI(t *testing.T) {
	withArgsAndEnv(t,
		[]string{"--api", "--enable-mcpserver", "--mcp-host=cli", "--mcp-port=10001", "--mcp-mode=host", "--mcp-host-api-key=cli-key", "--no-transport-sse-enabled", "--no-transport-streamable-http-enabled", "--no-json-response"},
		map[string]string{
			"RAGFLOW_MCP_HOST":                         "env-host",
			"RAGFLOW_MCP_PORT":                         "10002",
			"RAGFLOW_MCP_LAUNCH_MODE":                  "self-host",
			"RAGFLOW_MCP_HOST_API_KEY":                 "env-key",
			"RAGFLOW_MCP_ENABLED":                      "yes",
			"RAGFLOW_MCP_TRANSPORT_SSE_ENABLED":        "true",
			"RAGFLOW_MCP_TRANSPORT_STREAMABLE_ENABLED": "true",
			"RAGFLOW_MCP_JSON_RESPONSE":                "true",
		}, func() {
			args, err := parseArgs()
			if err != nil {
				t.Fatalf("parseArgs: %v", err)
			}
			if args.mcpHost != "env-host" || args.mcpPort != 10002 || args.mcpMode != "self-host" || args.mcpAPIKey != "env-key" {
				t.Fatalf("env did not override CLI: %#v", args)
			}
			if !args.mcpEnabled || !args.mcpSSE || !args.mcpStreamable || !args.mcpJSON {
				t.Fatalf("env bools did not resolve true: %#v", args)
			}
		})
}

func TestParseArgsMCPTransportFallbackMatchesPython(t *testing.T) {
	withArgsAndEnv(t,
		[]string{"--api", "--enable-mcpserver", "--mcp-mode=host", "--no-transport-sse-enabled", "--no-transport-streamable-http-enabled"},
		nil, func() {
			args, err := parseArgs()
			if err != nil {
				t.Fatalf("parseArgs: %v", err)
			}
			if args.mcpSSE {
				t.Fatal("SSE should stay disabled")
			}
			if !args.mcpStreamable {
				t.Fatal("streamable HTTP should be re-enabled when both transports are disabled")
			}
			if args.mcpJSON {
				t.Fatal("JSON response should be false because Python disables JSON before both-disabled fallback")
			}
		})
}

func TestParseArgsMCPValidation(t *testing.T) {
	t.Run("invalid mode", func(t *testing.T) {
		withArgsAndEnv(t, []string{"--api", "--mcp-mode=bogus"}, nil, func() {
			if _, err := parseArgs(); err == nil {
				t.Fatal("expected invalid mode error")
			}
		})
	})
	t.Run("invalid port", func(t *testing.T) {
		withArgsAndEnv(t, []string{"--api", "--mcp-port=0"}, nil, func() {
			if _, err := parseArgs(); err == nil {
				t.Fatal("expected invalid port error")
			}
		})
	})
	t.Run("missing self-host key only when enabled", func(t *testing.T) {
		withArgsAndEnv(t, []string{"--api", "--enable-mcpserver"}, nil, func() {
			if _, err := parseArgs(); err == nil {
				t.Fatal("expected missing self-host key error")
			}
		})
	})
	t.Run("self-host key not required while disabled", func(t *testing.T) {
		withArgsAndEnv(t, []string{"--api"}, nil, func() {
			if _, err := parseArgs(); err != nil {
				t.Fatalf("parseArgs: %v", err)
			}
		})
	})
}

func parseArgsForTest(t *testing.T, argv ...string) (*serverArgs, error) {
	t.Helper()
	orig := os.Args
	os.Args = append([]string{"ragflow_server"}, argv...)
	defer func() { os.Args = orig }()
	return parseArgs()
}

func TestParseArgsMigrateIsStandalone(t *testing.T) {
	args, err := parseArgsForTest(t, "--migrate")
	if err != nil {
		t.Fatalf("parseArgs(--migrate) error = %v", err)
	}
	if !args.migrateDB {
		t.Fatal("migrateDB = false, want true")
	}
	if args.mode != nil {
		t.Fatalf("mode = %q, want nil: --migrate must not select a server mode", *args.mode)
	}
}

func TestParseArgsLogLevel(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		for _, argv := range [][]string{{"--api", "--log-level", level}, {"--migrate", "--log-level=" + level}} {
			args, err := parseArgsForTest(t, argv...)
			if err != nil {
				t.Fatalf("parseArgs(%v) error = %v", argv, err)
			}
			if got := selectedLogLevel(args, "info"); got != level {
				t.Errorf("selectedLogLevel(%v) = %q, want %q", argv, got, level)
			}
		}
	}

	for _, argv := range [][]string{{"--api", "--log-level"}, {"--api", "--log-level=trace"}, {"--api", "--log-level", "fatal"}} {
		if _, err := parseArgsForTest(t, argv...); err == nil {
			t.Errorf("parseArgs(%v) error = nil, want error", argv)
		}
	}
}

func TestSelectedLogLevelPrecedence(t *testing.T) {
	args, err := parseArgsForTest(t, "--api")
	if err != nil {
		t.Fatalf("parseArgs(--api) error = %v", err)
	}
	if got := selectedLogLevel(args, ""); got != "warn" {
		t.Errorf("default log level = %q, want warn", got)
	}
	if got := selectedLogLevel(args, "info"); got != "info" {
		t.Errorf("configured log level = %q, want info", got)
	}

	args, err = parseArgsForTest(t, "--api", "--log-level", "error")
	if err != nil {
		t.Fatalf("parseArgs with --log-level error = %v", err)
	}
	if got := selectedLogLevel(args, "info"); got != "error" {
		t.Errorf("--log-level log level = %q, want error", got)
	}
	if _, err := parseArgsForTest(t, "--api", "--debug"); err == nil {
		t.Error("parseArgs(--debug) error = nil, want unknown parameter error")
	}
}

func TestParseArgsMigrateRejectsMode(t *testing.T) {
	for _, mode := range []string{"--api", "--admin", "--ingestor", "--syncer"} {
		if _, err := parseArgsForTest(t, mode, "--migrate"); err == nil {
			t.Errorf("parseArgs(%s --migrate) error = nil, want error", mode)
		}
		if _, err := parseArgsForTest(t, "--migrate", mode); err == nil {
			t.Errorf("parseArgs(--migrate %s) error = nil, want error", mode)
		}
	}
}

func TestParseArgsModeResetsMigrate(t *testing.T) {
	args, err := parseArgsForTest(t, "--api")
	if err != nil {
		t.Fatalf("parseArgs(--api) error = %v", err)
	}
	if args.mode == nil || *args.mode != "api" {
		t.Fatalf("mode = %v, want api", args.mode)
	}
	if args.migrateDB {
		t.Fatal("migrateDB = true, want false")
	}
}

func TestMainMigrateReachesConfiguration(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_MIGRATE_MAIN") == "1" {
		os.Args = append([]string{os.Args[0]}, os.Args[len(os.Args)-3:]...)
		main()
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	dir := t.TempDir()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMainMigrateReachesConfiguration$", "--", "--migrate", "--config", filepath.Join(dir, "missing.yaml"))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "RAGFLOW_TEST_MIGRATE_MAIN=1")
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "initialize configuration") || strings.Contains(string(output), "Usage:") {
		t.Fatalf("migration did not reach config: %v\n%s", err, output)
	}
}
