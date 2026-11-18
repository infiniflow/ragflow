//go:build !ee

package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func resetHeartbeatStore(t *testing.T) {
	t.Helper()
	previous := GlobalServerStore
	GlobalServerStore = &ServerStore{servers: make(map[string]*common.BaseMessage)}
	t.Cleanup(func() { GlobalServerStore = previous })
}

func setHeartbeatTestInterval(t *testing.T, interval time.Duration) {
	t.Helper()
	if server.GetConfig() == nil {
		configPath := filepath.Join(t.TempDir(), "service_conf.yaml")
		if err := os.WriteFile(configPath, []byte("general:\n  heartbeat_interval: 3s\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := server.Init(configPath); err != nil {
			t.Fatal(err)
		}
	}
	cfg := server.GetConfig()
	previous := *cfg
	t.Cleanup(func() { *cfg = previous })
	v := viper.New()
	v.Set("general.heartbeat_interval", interval)
	if err := cfg.ParseGeneralConfig(v); err != nil {
		t.Fatal(err)
	}
}

func TestListIngestorsUsesHeartbeatInterval(t *testing.T) {
	for _, tt := range []struct {
		name     string
		interval time.Duration
		age      time.Duration
		want     string
	}{
		{name: "default stale", interval: 3 * time.Second, age: 20 * time.Second, want: "timeout"},
		{name: "long interval healthy", interval: 40 * time.Second, age: 35 * time.Second, want: "alive"},
		{name: "long interval stale", interval: 40 * time.Second, age: 201 * time.Second, want: "timeout"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetHeartbeatStore(t)
			setHeartbeatTestInterval(t, tt.interval)
			report := &common.BaseMessage{
				ServerName: "ingestor-test", ServerType: common.ServerTypeIngestion,
				Host: "127.0.0.1", Version: "test-version", Timestamp: time.Now().Add(-tt.age),
			}
			if err := GlobalServerStore.UpdateServerInfo(report.ServerName, report); err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			router.GET("/api/v1/admin/ingestors", (&Handler{}).ListIngestors)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/ingestors", nil))
			var response struct {
				Code common.ErrorCode    `json:"code"`
				Data []map[string]string `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusOK || response.Code != common.CodeSuccess || len(response.Data) != 1 {
				t.Fatalf("unexpected ingestor response: %s", recorder.Body.String())
			}
			if got := response.Data[0]; got["status"] != tt.want || got["name"] != report.ServerName || got["host"] != report.Host {
				t.Fatalf("ingestor response = %v, want status %q and reported identity", got, tt.want)
			}
		})
	}
}

func TestHeartbeatReportsPreserveAcknowledgement(t *testing.T) {
	for _, serverType := range []common.ServerType{common.ServerTypeAPI, common.ServerTypeIngestion, common.ServerTypeFileSyncer} {
		t.Run(string(serverType), func(t *testing.T) {
			resetHeartbeatStore(t)
			port := 0
			if serverType == common.ServerTypeAPI {
				port = 9380
			}
			report := common.BaseMessage{
				MessageType: common.MessageHeartbeat, ServerName: fmt.Sprintf("%s-test", serverType),
				ServerType: serverType, Host: "127.0.0.1", Port: port, Version: "test-version",
			}
			body, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			router.POST("/api/v1/admin/reports", (&Handler{service: &Service{}}).Reports)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/reports", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			started := time.Now()
			router.ServeHTTP(recorder, request)
			var response struct {
				Code common.ErrorCode `json:"code"`
				Data map[string]any   `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusOK || response.Code != common.CodeLicenseValid || response.Data == nil {
				t.Fatalf("heartbeat acknowledgement lost code or data: HTTP %d %s", recorder.Code, recorder.Body.String())
			}
			stored, ok := GlobalServerStore.GetServerInfo(report.ServerName)
			if !ok || stored.ServerType != serverType || stored.Host != report.Host || stored.Port != report.Port || stored.Version != report.Version {
				t.Fatalf("stored heartbeat = %+v, found %v; want reported identity", stored, ok)
			}
			if stored.Timestamp.Before(started) || stored.Timestamp.After(time.Now()) {
				t.Fatalf("missing timestamp was not set at receipt: %s", stored.Timestamp)
			}
		})
	}
}

func TestHeartbeatReportsConcurrent(t *testing.T) {
	resetHeartbeatStore(t)
	router := gin.New()
	router.POST("/api/v1/admin/reports", (&Handler{service: &Service{}}).Reports)
	const reports = 16
	var wg sync.WaitGroup
	for i := 0; i < reports; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"report_type":"heartbeat","server_type":"ingestor","server_id":"ingestor-%d"}`, i)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/reports", bytes.NewBufferString(body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			var response struct {
				Code common.ErrorCode `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Errorf("decode acknowledgement: %v", err)
			} else if recorder.Code != http.StatusOK || response.Code != common.CodeLicenseValid {
				t.Errorf("heartbeat acknowledgement: HTTP %d %s", recorder.Code, recorder.Body.String())
			}
		}(i)
	}
	wg.Wait()
	if got := len(GlobalServerStore.ListInfos()); got != reports {
		t.Fatalf("stored %d concurrent heartbeats, want %d", got, reports)
	}
}

func TestHeartbeatReportsRejectInvalidMessages(t *testing.T) {
	for _, tt := range []struct {
		name     string
		body     string
		wantHTTP int
		wantCode common.ErrorCode
	}{
		{name: "invalid JSON", body: `{`, wantHTTP: http.StatusBadRequest, wantCode: common.CodeBadRequest},
		{name: "unsupported report", body: `{"report_type":"metric","server_id":"invalid","server_type":"api"}`, wantHTTP: http.StatusBadRequest, wantCode: common.CodeBadRequest},
		{name: "unknown server", body: `{"report_type":"heartbeat","server_id":"invalid","server_type":"unknown"}`, wantHTTP: http.StatusOK, wantCode: common.CodeErrorServerStatus},
		{name: "admin is not a reporting client", body: `{"report_type":"heartbeat","server_id":"invalid","server_type":"admin"}`, wantHTTP: http.StatusOK, wantCode: common.CodeErrorServerStatus},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resetHeartbeatStore(t)
			router := gin.New()
			router.POST("/api/v1/admin/reports", (&Handler{service: &Service{}}).Reports)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/reports", bytes.NewBufferString(tt.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			var response struct {
				Code common.ErrorCode `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != tt.wantHTTP || response.Code != tt.wantCode || len(GlobalServerStore.ListInfos()) != 0 {
				t.Fatalf("invalid report response = HTTP %d %s; want HTTP %d code %d and empty registry", recorder.Code, recorder.Body.String(), tt.wantHTTP, tt.wantCode)
			}
		})
	}
}
