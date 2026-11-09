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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

func TestFutureAGIPrivateCollectorRequiresOperatorOptIn(t *testing.T) {
	useFutureAGITestDB(t)
	t.Setenv(common.EnvAllowAnyHost, "")
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	svc := NewFutureAGIService()
	ctx := permission.WithIdentity(t.Context(), "user-1", "tenant-1")
	req := FutureAGISettings{APIKey: "synthetic-api", SecretKey: "synthetic-secret", ProjectName: "project"}
	for _, endpoint := range []string{srv.URL, "http://169.254.169.254", "http://10.0.0.1", "http://[::1]", "http://[fe80::1]"} {
		req.Endpoint = endpoint
		if _, code, err := svc.SetAPIKey(ctx, req); err == nil || code != common.CodeDataError {
			t.Fatalf("private collector accepted without opt-in: code=%v err=%v", code, err)
		}
		client := &FutureAGIClient{Endpoint: endpoint, APIKey: req.APIKey, SecretKey: req.SecretKey}
		err := client.post(t.Context(), []byte(`{"resourceSpans":[]}`))
		if err == nil || strings.Contains(err.Error(), "synthetic") || strings.Contains(err.Error(), endpoint) {
			t.Fatalf("private export not rejected safely: %v", err)
		}
	}
	row, err := dao.NewFutureAGI().GetByTenantID(ctx, dao.DB, "tenant-1")
	if err != nil || row != nil || requests.Load() != 0 {
		t.Fatalf("rejected setting was saved or dialed: row=%v requests=%d err=%v", row != nil, requests.Load(), err)
	}

	t.Setenv(common.EnvAllowAnyHost, "true")
	req.Endpoint = srv.URL
	if _, _, err := svc.SetAPIKey(ctx, req); err != nil {
		t.Fatalf("operator-enabled private collector rejected: %v", err)
	}
	client := &FutureAGIClient{Endpoint: srv.URL, APIKey: req.APIKey, SecretKey: req.SecretKey}
	if err := client.post(t.Context(), []byte(`{"resourceSpans":[]}`)); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected verification and export, got %d requests", requests.Load())
	}
	// A saved endpoint is not grandfathered past the operator's current policy.
	t.Setenv(common.EnvAllowAnyHost, "false")
	if err := client.post(t.Context(), []byte(`{"resourceSpans":[]}`)); err == nil || requests.Load() != 2 {
		t.Fatal("export did not recheck the current host policy")
	}
}

func TestFutureAGICollectorPinsValidatedAddress(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if !strings.HasPrefix(r.Host, "collector.invalid:") {
			t.Errorf("original HTTP host not preserved: %q", r.Host)
		}
		if r.URL.Path != futureAGICollectorPath {
			t.Errorf("unexpected collector path: %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	endpoint := strings.Replace(srv.URL, "127.0.0.1", "collector.invalid", 1)
	original := common.AssertURLSafeWithContext
	t.Cleanup(func() { common.AssertURLSafeWithContext = original })
	validations := 0
	common.AssertURLSafeWithContext = func(ctx context.Context, raw string) (string, string, error) {
		validations++
		if raw != endpoint+futureAGICollectorPath {
			t.Fatalf("wrong validation destination: %q", raw)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("DNS validation has no deadline")
		}
		if validations > 2 {
			return "", "", errors.New("DNS changed to a disallowed address")
		}
		return "collector.invalid", "127.0.0.1", nil
	}
	// A proxy must not replace the validated dial destination, and the invalid
	// hostname must not be resolved a second time after validation.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")
	if err := (defaultFutureAGIVerifier{}).Verify(t.Context(), endpoint, "api", "secret"); err != nil {
		t.Fatal(err)
	}
	client := &FutureAGIClient{Endpoint: endpoint, APIKey: "api", SecretKey: "secret"}
	if err := client.post(t.Context(), []byte(`{"resourceSpans":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := client.post(t.Context(), []byte(`{"resourceSpans":[]}`)); err == nil {
		t.Fatal("export reused stale address validation")
	}
	if validations != 3 || requests.Load() != 2 {
		t.Fatalf("validations=%d requests=%d", validations, requests.Load())
	}
}
