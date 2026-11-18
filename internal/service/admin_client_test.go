package service

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/utility"
)

type heartbeatTestServer struct {
	serverType common.ServerType
	name       string
	port       int
}

var heartbeatTestServers = []heartbeatTestServer{
	{serverType: common.ServerTypeAPI, name: "test-api", port: 9380},
	{serverType: common.ServerTypeIngestion, name: "test-ingestor", port: 0},
	{serverType: common.ServerTypeFileSyncer, name: "test-syncer", port: 0},
}

func newHeartbeatTestClient(t *testing.T, server heartbeatTestServer, handler http.HandlerFunc) *AdminClient {
	t.Helper()
	previousCode := licenseStatusCode
	t.Cleanup(func() { licenseStatusCode = previousCode })
	admin := httptest.NewServer(handler)
	t.Cleanup(admin.Close)
	addr := admin.Listener.Addr().(*net.TCPAddr)
	client := NewAdminClient(server.serverType, server.name, "127.0.0.1", server.port)
	client.client = utility.NewHTTPClientBuilder().
		WithHost(addr.IP.String()).
		WithPort(addr.Port).
		WithTimeout(time.Second).
		Build()
	return client
}

func TestAdminClientSendsEveryHeartbeat(t *testing.T) {
	for _, server := range heartbeatTestServers {
		t.Run(string(server.serverType), func(t *testing.T) {
			reports := make(chan common.BaseMessage, 12)
			client := newHeartbeatTestClient(t, server, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/reports" {
					t.Errorf("unexpected heartbeat request: %s %s", r.Method, r.URL.Path)
				}
				var report common.BaseMessage
				if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
					t.Errorf("decode heartbeat: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				reports <- report
				_ = json.NewEncoder(w).Encode(map[string]any{"code": common.CodeLicenseValid, "data": map[string]any{}})
			})

			for i := 0; i < cap(reports); i++ {
				started := time.Now()
				if err := client.SendHeartbeat(); err != nil {
					t.Fatalf("heartbeat %d: %v", i+1, err)
				}
				select {
				case report := <-reports:
					if report.ServerName != server.name || report.ServerType != server.serverType ||
						report.MessageType != common.MessageHeartbeat || report.Host != "127.0.0.1" || report.Port != server.port ||
						report.Version != client.version {
						t.Fatalf("unexpected heartbeat identity: %+v", report)
					}
					if report.Timestamp.Before(started) || report.Timestamp.After(time.Now()) {
						t.Fatalf("heartbeat %d has a stale timestamp: %s", i+1, report.Timestamp)
					}
				default:
					t.Fatalf("heartbeat %d returned success without sending a report", i+1)
				}
			}
		})
	}
}

func TestAdminClientDetectsFailureAfterSuccess(t *testing.T) {
	for _, server := range heartbeatTestServers {
		t.Run(string(server.serverType), func(t *testing.T) {
			for _, tt := range []struct {
				name   string
				status int
				body   string
			}{
				{name: "HTTP error", status: http.StatusServiceUnavailable, body: `{"code":320}`},
				{name: "rejected heartbeat", status: http.StatusOK, body: `{"code":0,"message":"heartbeat rejected"}`},
				{name: "invalid response", status: http.StatusOK, body: `{`},
			} {
				t.Run(tt.name, func(t *testing.T) {
					var requests atomic.Int32
					client := newHeartbeatTestClient(t, server, func(w http.ResponseWriter, r *http.Request) {
						if requests.Add(1) == 2 {
							w.WriteHeader(tt.status)
							_, _ = w.Write([]byte(tt.body))
							return
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"code": common.CodeLicenseValid, "data": map[string]any{}})
					})

					if err := client.SendHeartbeat(); err != nil {
						t.Fatalf("initial heartbeat: %v", err)
					}
					if err := client.SendHeartbeat(); err == nil {
						t.Fatal("heartbeat failure after a successful report was not detected")
					}
					if err := client.SendHeartbeat(); err != nil {
						t.Fatalf("heartbeat did not recover: %v", err)
					}
					if got := requests.Load(); got != 3 {
						t.Fatalf("received %d heartbeat requests, want 3", got)
					}
				})
			}
		})
	}
}
