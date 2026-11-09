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

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func TestFutureAGISettingsTenantIsolation(t *testing.T) {
	useFutureAGITestDB(t)
	verifier := &stubFutureAGIVerifier{}
	svc := &FutureAGIService{futureAGIDAO: dao.NewFutureAGI(), verifier: verifier}
	member := permission.WithIdentity(t.Context(), "user-1", "tenant-1")
	req := FutureAGISettings{APIKey: "api-1234", SecretKey: "secret-5678", ProjectName: "project"}
	if _, _, err := svc.SetAPIKey(member, req); err != nil {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{
		t.Context(),
		permission.WithIdentity(t.Context(), "user-1", "tenant-2"),
		permission.WithIdentity(t.Context(), "outsider", "tenant-1"),
	} {
		if _, _, err := svc.SetAPIKey(ctx, req); err == nil {
			t.Fatal("unauthorized save succeeded")
		}
		if _, _, _, err := svc.GetAPIKey(ctx); err == nil {
			t.Fatal("unauthorized read succeeded")
		}
		if _, _, _, err := svc.DeleteAPIKey(ctx); err == nil {
			t.Fatal("unauthorized delete succeeded")
		}
	}
	if verifier.calls != 1 {
		t.Fatalf("unauthorized request reached collector: calls=%d", verifier.calls)
	}
	row, err := dao.NewFutureAGI().GetByTenantID(member, dao.DB, "tenant-1")
	if err != nil || row == nil || row.APIKey != req.APIKey {
		t.Fatal("tenant configuration changed")
	}
	if client := futureAGIClientForTenant(member, "tenant-2"); client != nil {
		t.Fatal("another tenant inherited the exporter")
	}
	if got, err := dao.NewFutureAGI().GetByTenantID(member, dao.DB, "user-1"); err != nil || got != nil {
		t.Fatal("settings were stored against the user instead of the tenant")
	}
}

func TestFutureAGISettingsRetainKeysOnlyForSameEndpoint(t *testing.T) {
	useFutureAGITestDB(t)
	svc := &FutureAGIService{futureAGIDAO: dao.NewFutureAGI(), verifier: &stubFutureAGIVerifier{}}
	ctx := permission.WithIdentity(t.Context(), "user-1", "tenant-1")
	if _, _, err := svc.SetAPIKey(ctx, FutureAGISettings{APIKey: "api-1234", SecretKey: "secret-5678", ProjectName: "project"}); err != nil {
		t.Fatal(err)
	}
	info, _, err := svc.SetAPIKey(ctx, FutureAGISettings{ProjectName: "updated", Redact: true})
	if err != nil || info == nil || info.SecretKey != "****5678" || !info.Redact {
		t.Fatalf("retain keys: info=%+v err=%v", info, err)
	}
	if _, code, err := svc.SetAPIKey(ctx, FutureAGISettings{ProjectName: "updated", Endpoint: "https://other.example"}); err == nil || code != common.CodeDataError {
		t.Fatal("retained keys sent to a new destination")
	}
	row, _ := dao.NewFutureAGI().GetByTenantID(ctx, dao.DB, "tenant-1")
	if row.SecretKey != "secret-5678" || row.Endpoint != FutureAGIDefaultEndpoint {
		t.Fatal("rejected update changed credentials")
	}
}

func TestFutureAGISettingsRejectUnsafeURLParts(t *testing.T) {
	useFutureAGITestDB(t)
	verifier := &stubFutureAGIVerifier{}
	svc := &FutureAGIService{futureAGIDAO: dao.NewFutureAGI(), verifier: verifier}
	ctx := permission.WithIdentity(t.Context(), "user-1", "tenant-1")
	for _, endpoint := range []string{"file:///etc/passwd", "https://user:pass@example.com", "https://example.com?key=secret", "https://example.com#fragment", "http://:80"} {
		if _, code, err := svc.SetAPIKey(ctx, FutureAGISettings{APIKey: "a", SecretKey: "b", ProjectName: "p", Endpoint: endpoint}); err == nil || code != common.CodeDataError {
			t.Fatalf("accepted endpoint %q", endpoint)
		}
	}
	if verifier.calls != 0 {
		t.Fatal("invalid endpoint reached verifier")
	}
}

func TestFutureAGICollectorDoesNotFollowRedirects(t *testing.T) {
	var followed atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed.Store(true) }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	client := &FutureAGIClient{Endpoint: source.URL, APIKey: "private-api", SecretKey: "private-secret", HTTP: source.Client()}
	err := client.post(t.Context(), []byte(`{"resourceSpans":[]}`))
	if err == nil || followed.Load() || strings.Contains(err.Error(), "private") {
		t.Fatalf("redirect behavior: followed=%v err=%v", followed.Load(), err)
	}
	if err := (defaultFutureAGIVerifier{}).Verify(t.Context(), source.URL, "a", "b"); err == nil {
		t.Fatal("redirect accepted during verification")
	}
}

func TestFutureAGIShutdownCancelsSlowDelivery(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release }))
	defer srv.Close()
	defer close(release)
	client := NewFutureAGIClient(srv.URL, "a", "b", "p", false)
	client.enqueue([]byte(`{"resourceSpans":[]}`))
	<-entered
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := client.Shutdown(ctx); err == nil {
		t.Fatal("expected bounded flush timeout")
	}
	select {
	case <-client.stopped:
	case <-time.After(time.Second):
		t.Fatal("worker survived shutdown deadline")
	}
}

func TestFutureAGIQueueBudgets(t *testing.T) {
	client := &FutureAGIClient{events: make(chan []byte, 1)}
	client.enqueue(make([]byte, futureAGIMaxBodyBytes+1))
	if len(client.events) != 0 || client.dropped.Load() != 1 {
		t.Fatal("oversized trace entered queue")
	}
	before := futureAGIQueuedBytes.Load()
	futureAGIQueuedBytes.Store(futureAGIMaxQueueBytes)
	defer futureAGIQueuedBytes.Store(before)
	client.enqueue([]byte("trace"))
	if len(client.events) != 0 || client.dropped.Load() != 2 {
		t.Fatal("global queue budget exceeded")
	}
}

func TestFutureAGIClientCacheBound(t *testing.T) {
	useFutureAGITestDB(t)
	// Populate only the map to exercise eviction without creating hundreds of workers.
	futureAGIClients.Lock()
	original := futureAGIClients.byTenant
	futureAGIClients.byTenant = map[string]*FutureAGIClient{}
	for i := 0; i < futureAGIMaxClients; i++ {
		stopped := make(chan struct{})
		close(stopped)
		futureAGIClients.byTenant[strings.Repeat("x", i+1)] = &FutureAGIClient{stop: make(chan struct{}), stopped: stopped}
	}
	futureAGIClients.Unlock()
	defer func() {
		forgetFutureAGIClient("tenant-1")
		futureAGIClients.Lock()
		futureAGIClients.byTenant = original
		futureAGIClients.Unlock()
	}()
	if err := dao.DB.Create(&entity.TenantFutureAGI{TenantID: "tenant-1", APIKey: "a", SecretKey: "b", ProjectName: "p"}).Error; err != nil {
		t.Fatal(err)
	}
	if futureAGIClientForTenant(t.Context(), "tenant-1") == nil {
		t.Fatal("missing client")
	}
	futureAGIClients.Lock()
	n := len(futureAGIClients.byTenant)
	futureAGIClients.Unlock()
	if n != futureAGIMaxClients {
		t.Fatalf("cache size=%d", n)
	}
}
