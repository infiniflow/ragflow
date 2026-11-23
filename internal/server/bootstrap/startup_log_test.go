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

package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/server"
)

func TestStartupLoggerKeepsInfoVisibleUntilRuntimeLevel(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "service_conf.yaml")
	if err := os.WriteFile(configPath, []byte("log:\n  level: warn\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RAGFLOW_LOG_PATH", dir)
	t.Setenv("RAGFLOW_LOG_LEVEL", "warn")
	if err := server.Init(configPath); err != nil {
		t.Fatal(err)
	}
	logger, sugar := common.Logger, common.Sugar
	t.Cleanup(func() { common.Logger, common.Sugar = logger, sugar })

	SetLogger("startup", server.GetConfig().GetLogConfig().Level)
	common.Info("starting services")
	if err := common.SetLogLevel("warn"); err != nil {
		t.Fatal(err)
	}
	common.Info("serving request")
	common.Warn("runtime warning")
	common.SyncLog()

	data, err := os.ReadFile(filepath.Join(dir, "startup.log"))
	if err != nil {
		t.Fatal(err)
	}
	logs := string(data)
	if !strings.Contains(logs, "starting services") || strings.Contains(logs, "serving request") || !strings.Contains(logs, "runtime warning") {
		t.Fatalf("unexpected startup and runtime logs:\n%s", logs)
	}
}
