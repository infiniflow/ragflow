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
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/server"
	"ragflow/internal/server/local"
)

func TestHeartbeatReportersKeepSeparateIdentities(t *testing.T) {
	var mu sync.Mutex
	reports := map[string]int{}
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var message common.BaseMessage
		if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
			t.Error(err)
			http.Error(w, "invalid message", http.StatusBadRequest)
			return
		}
		mu.Lock()
		reports[message.ServerName]++
		mu.Unlock()
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
	local.InitAdminStatus()
	api := StartHeartbeat(common.ServerTypeAPI, "api-test", port, time.Hour)
	if api == nil {
		t.Fatal("API heartbeat did not start")
	}
	defer api.Wait()
	defer api.Stop()
	ingestor := StartHeartbeat(common.ServerTypeIngestion, "ingestor-test", 0, time.Hour)
	if ingestor == nil {
		t.Fatal("ingestor heartbeat did not start")
	}
	defer ingestor.Wait()
	defer ingestor.Stop()
	// Successful reporters skip ten ticks before their next HTTP request.
	for range 11 {
		api.Job()
	}
	for range 11 {
		ingestor.Job()
	}
	mu.Lock()
	defer mu.Unlock()
	if reports["api-test"] != 2 || reports["ingestor-test"] != 2 {
		t.Fatalf("reports = %v, want two reports from each service", reports)
	}
}
