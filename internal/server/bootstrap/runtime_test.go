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

//go:build !ee

package bootstrap

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/server"
)

func TestStartHeartbeatDoesNotReportInCommunityEdition(t *testing.T) {
	var reports atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reports.Add(1)
		fmt.Fprintf(w, `{"code":%d}`, common.CodeLicenseValid)
	}))
	defer httpServer.Close()
	_, portText, err := net.SplitHostPort(httpServer.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "service_conf.yaml")
	if err := os.WriteFile(configPath, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RAGFLOW_ADMIN_HOST", "127.0.0.1")
	t.Setenv("RAGFLOW_ADMIN_HTTP_PORT", portText)
	if err := server.Init(configPath); err != nil {
		t.Fatal(err)
	}
	for _, serverType := range []common.ServerType{
		common.ServerTypeAPI,
		common.ServerTypeIngestion,
		common.ServerTypeFileSyncer,
		common.ServerTypeChatter,
	} {
		t.Run(string(serverType), func(t *testing.T) {
			if task := StartHeartbeat(serverType, "test-server", port, time.Hour); task != nil {
				task.Stop()
				task.Wait()
				t.Fatal("community heartbeat reporter started")
			}
		})
	}
	if got := reports.Load(); got != 0 {
		t.Fatalf("heartbeat reports = %d, want 0", got)
	}
}
