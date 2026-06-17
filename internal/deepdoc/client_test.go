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

package deepdoc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// withEnv unsets DEEPDOC_URL and TENSORRT_DLA_SVR for the duration
// of t, restoring whatever values were present before. NewClient
// reads these env vars, so tests must isolate the env to be
// deterministic.
func withEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"DEEPDOC_URL", "TENSORRT_DLA_SVR"} {
		prev, had := os.LookupEnv(k)
		os.Unsetenv(k)
		t.Cleanup(func() {
			if had {
				os.Setenv(k, prev)
			} else {
				os.Unsetenv(k)
			}
		})
	}
}

func TestNewClient_NoEnvVars(t *testing.T) {
	withEnv(t)
	c := NewClient()
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	if c.Enabled() {
		t.Errorf("Enabled()=true with no env vars; want false")
	}
	if c.maxAttempts != DefaultMaxAttempts {
		t.Errorf("maxAttempts=%d, want %d", c.maxAttempts, DefaultMaxAttempts)
	}
	if c.backoff != DefaultBackoff {
		t.Errorf("backoff=%v, want %v", c.backoff, DefaultBackoff)
	}
	if c.httpClient == nil {
		t.Error("httpClient is nil; default should be applied")
	}
	if c.httpClient.Timeout != DefaultPerAttemptTimeout {
		t.Errorf("httpClient.Timeout=%v, want %v", c.httpClient.Timeout, DefaultPerAttemptTimeout)
	}
}

func TestNewClient_DeepdocURLPreferred(t *testing.T) {
	withEnv(t)
	os.Setenv("DEEPDOC_URL", "http://deepdoc:8001")
	os.Setenv("TENSORRT_DLA_SVR", "http://legacy:8001")
	c := NewClient()
	if got, want := c.baseURL, "http://deepdoc:8001"; got != want {
		t.Errorf("baseURL=%q, want %q (DEEPDOC_URL should win over TENSORRT_DLA_SVR)", got, want)
	}
	if !c.Enabled() {
		t.Errorf("Enabled()=false with DEEPDOC_URL set; want true")
	}
}

func TestNewClient_LegacyAlias(t *testing.T) {
	withEnv(t)
	os.Setenv("TENSORRT_DLA_SVR", "http://legacy:8001")
	c := NewClient()
	if got, want := c.baseURL, "http://legacy:8001"; got != want {
		t.Errorf("baseURL=%q, want %q (TENSORRT_DLA_SVR should populate baseURL)", got, want)
	}
	if !c.Enabled() {
		t.Errorf("Enabled()=false with TENSORRT_DLA_SVR set; want true")
	}
}

func TestNewClientWithURL_Empty(t *testing.T) {
	c := NewClientWithURL("")
	if c.Enabled() {
		t.Errorf("Enabled()=true with empty URL; want false")
	}
}

func TestOptions_Override(t *testing.T) {
	hc := &http.Client{Timeout: 7 * time.Second}
	c := NewClientWithURL("http://x:1",
		WithHTTPClient(hc),
		WithMaxAttempts(5),
		WithBackoff(50*time.Millisecond),
	)
	if c.httpClient != hc {
		t.Errorf("WithHTTPClient did not apply")
	}
	if c.maxAttempts != 5 {
		t.Errorf("maxAttempts=%d, want 5", c.maxAttempts)
	}
	if c.backoff != 50*time.Millisecond {
		t.Errorf("backoff=%v, want 50ms", c.backoff)
	}
}

func TestClient_DLAWithoutURL(t *testing.T) {
	c := NewClientWithURL("")
	_, err := c.DLA(context.Background(), [][]byte{[]byte("jpg")})
	if err != ErrNoURL {
		t.Errorf("DLA() error=%v, want ErrNoURL", err)
	}
}

func TestClient_OCRReturnsNoRemoteEndpoint(t *testing.T) {
	c := NewClientWithURL("http://x:1")
	_, err := c.OCR(context.Background(), [][]byte{[]byte("jpg")})
	if err != ErrNoRemoteEndpoint {
		t.Errorf("OCR() error=%v, want ErrNoRemoteEndpoint", err)
	}
}

func TestClient_OCRNoRemoteEndpointEvenWithUnsetURL(t *testing.T) {
	c := NewClientWithURL("")
	_, err := c.OCR(context.Background(), nil)
	if err != ErrNoRemoteEndpoint {
		t.Errorf("OCR() error=%v, want ErrNoRemoteEndpoint (call should not fall through to ErrNoURL)", err)
	}
}

func TestClient_TSRReturnsNoRemoteEndpoint(t *testing.T) {
	c := NewClientWithURL("http://x:1")
	_, err := c.TSR(context.Background(), [][]byte{[]byte("jpg")})
	if err != ErrNoRemoteEndpoint {
		t.Errorf("TSR() error=%v, want ErrNoRemoteEndpoint", err)
	}
}

func TestClient_DLAEmptyInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("DLA() with empty input should not hit the server")
	}))
	defer srv.Close()
	c := NewClientWithURL(srv.URL)
	res, err := c.DLA(context.Background(), nil)
	if err != nil {
		t.Errorf("DLA(nil) error=%v, want nil", err)
	}
	if len(res) != 0 {
		t.Errorf("DLA(nil) len=%d, want 0", len(res))
	}
}

func TestDLAClasses_Layout(t *testing.T) {
	if len(DLAClasses) != 10 {
		t.Fatalf("DLAClasses len=%d, want 10", len(DLAClasses))
	}
	want := []string{
		"title", "text", "reference", "figure", "figure caption",
		"table", "table caption", "table caption", "equation", "figure caption",
	}
	for i, w := range want {
		if DLAClasses[i] != w {
			t.Errorf("DLAClasses[%d]=%q, want %q", i, DLAClasses[i], w)
		}
	}
	// duplicates are intentional and must be preserved.
	if DLAClasses[6] != DLAClasses[7] {
		t.Errorf("DLAClasses[6]=%q vs [7]=%q; duplicates at indices 6,7 must match", DLAClasses[6], DLAClasses[7])
	}
	if DLAClasses[4] != DLAClasses[9] {
		t.Errorf("DLAClasses[4]=%q vs [9]=%q; duplicates at indices 4,9 must match", DLAClasses[4], DLAClasses[9])
	}
}
