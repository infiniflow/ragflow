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
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/server"
	"ragflow/internal/service"
)

func TestStartHeartbeatSkippedWhenDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service_conf.yaml")
	if err := os.WriteFile(path, []byte("admin:\n  disable_heartbeat: true\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := server.Init(path); err != nil {
		t.Fatalf("server.Init: %v", err)
	}
	service.AdminServiceClient = nil

	hb := StartHeartbeat(common.ServerTypeAPI, "test-server", 9380, time.Second)
	if hb != nil {
		hb.Stop()
		t.Fatal("StartHeartbeat started a reporter, want nil when disable_heartbeat is true")
	}
	if service.AdminServiceClient != nil {
		t.Fatal("StartHeartbeat created an admin client, want none when disable_heartbeat is true")
	}
}
