//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package common

import "testing"

func TestAccessTokenContextRoundTrip(t *testing.T) {
	token, err := DumpAccessToken("access-token-12345678901234567890", "tenant-123456789012345678901234567890", "secret")
	if err != nil {
		t.Fatalf("DumpAccessToken() error = %v", err)
	}

	got, err := ExtractAccessTokenContext(token, "secret")
	if err != nil {
		t.Fatalf("ExtractAccessTokenContext() error = %v", err)
	}
	if got.AccessToken != "access-token-12345678901234567890" {
		t.Errorf("AccessToken = %q", got.AccessToken)
	}
	if got.TenantID != "tenant-123456789012345678901234567890" {
		t.Errorf("TenantID = %q", got.TenantID)
	}
}

func TestAccessTokenContextAllowsEmptyTenant(t *testing.T) {
	token, err := DumpAccessToken("access-token-12345678901234567890", "", "secret")
	if err != nil {
		t.Fatalf("DumpAccessToken() error = %v", err)
	}

	got, err := ExtractAccessTokenContext(token, "secret")
	if err != nil {
		t.Fatalf("ExtractAccessTokenContext() error = %v", err)
	}
	if got.TenantID != "" {
		t.Errorf("TenantID = %q, want empty", got.TenantID)
	}
}

func TestAccessTokenContextSupportsSeparators(t *testing.T) {
	token, err := DumpAccessToken("access|token", "tenant|id", "secret")
	if err != nil {
		t.Fatalf("DumpAccessToken() error = %v", err)
	}

	got, err := ExtractAccessTokenContext(token, "secret")
	if err != nil {
		t.Fatalf("ExtractAccessTokenContext() error = %v", err)
	}
	if got.AccessToken != "access|token" || got.TenantID != "tenant|id" {
		t.Fatalf("context = %+v, want access|token in tenant|id", got)
	}
}

func TestAccessTokenContextRejectsWrongSigningKey(t *testing.T) {
	token, err := DumpAccessToken("access-token", "tenant-1", "secret")
	if err != nil {
		t.Fatalf("DumpAccessToken() error = %v", err)
	}
	if _, err = ExtractAccessTokenContext(token, "wrong-secret"); err == nil {
		t.Fatal("ExtractAccessTokenContext() accepted the wrong signing key")
	}
}
