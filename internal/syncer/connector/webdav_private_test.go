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

package connector

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
)

func TestWebDAVPrivateHostAllowlistPinsOnlyNamedNetwork(t *testing.T) {
	connectorAllowLoopbackForTest = false
	t.Cleanup(func() { connectorAllowLoopbackForTest = false })
	lookup := common.LookupHost
	common.LookupHost = func(host string) ([]string, error) {
		switch host {
		case "webdav":
			return []string{"172.18.0.16"}, nil
		case "mixed":
			return []string{"172.18.0.16", "10.0.0.9"}, nil
		case "metadata":
			return []string{"169.254.169.254"}, nil
		default:
			return []string{"10.0.0.9"}, nil
		}
	}
	t.Cleanup(func() { common.LookupHost = lookup })
	t.Setenv("RAGFLOW_WEBDAV_PRIVATE_HOSTS", "webdav=172.18.0.0/16,mixed=172.18.0.0/16,metadata=169.254.0.0/16")

	host, ip, err := assertWebDAVURLSafe("http://webdav:6060/docs")
	if err != nil || host != "webdav" || !ip.Equal(net.ParseIP("172.18.0.16")) {
		t.Fatalf("allowed WebDAV URL = (%q, %v, %v)", host, ip, err)
	}
	for _, raw := range []string{
		"http://other:6060/docs", "http://mixed:6060/docs",
		"http://metadata/latest", "ftp://webdav:6060/docs",
	} {
		if _, _, err := assertWebDAVURLSafe(raw); err == nil {
			t.Errorf("unexpectedly allowed %s", raw)
		}
	}
	if err := validateConnectorURL("http://webdav:6060/docs"); err == nil {
		t.Fatal("private WebDAV setting changed the shared URL guard")
	}
	t.Setenv("RAGFLOW_WEBDAV_PRIVATE_HOSTS", "mixed=172.18.0.0/16,mixed=10.0.0.0/8")
	if _, _, err := assertWebDAVURLSafe("http://mixed:6060/docs"); err != nil {
		t.Fatalf("all resolved addresses have an explicit matching network: %v", err)
	}
	t.Setenv("RAGFLOW_WEBDAV_PRIVATE_HOSTS", "webdav=invalid")
	if _, _, err := assertWebDAVURLSafe("http://webdav:6060/docs"); err == nil {
		t.Fatal("malformed allowlist entry was accepted")
	}
}

func TestWebDAVPrivateHostAllowlistEndToEnd(t *testing.T) {
	connectorAllowLoopbackForTest = false
	t.Cleanup(func() { connectorAllowLoopbackForTest = false })
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
			return
		}
		if r.Method == "PROPFIND" {
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<d:multistatus xmlns:d="DAV:"></d:multistatus>`)
			return
		}
		_, _ = io.WriteString(w, "private document")
	}))
	defer server.Close()
	t.Setenv("RAGFLOW_WEBDAV_PRIVATE_HOSTS", "")
	t.Setenv(common.EnvAllowAnyHost, "1")
	c, err := NewWebDAVConnector(map[string]any{
		"base_url":    server.URL,
		"credentials": map[string]any{"username": "user", "password": "pass"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(context.Background()); err == nil {
		t.Fatal("loopback WebDAV worked without an operator allowlist")
	}
	if requests != 0 {
		t.Fatalf("sent %d requests before destination validation", requests)
	}
	t.Setenv("RAGFLOW_WEBDAV_PRIVATE_HOSTS", "127.0.0.1=10.0.0.0/8")
	if err := c.Validate(context.Background()); err == nil || requests != 0 {
		t.Fatalf("wrong network was allowed: err=%v, requests=%d", err, requests)
	}
	t.Setenv("RAGFLOW_WEBDAV_PRIVATE_HOSTS", "127.0.0.1=127.0.0.1/32")
	if err := c.Validate(context.Background()); err != nil {
		t.Fatalf("allowed private WebDAV validation: %v", err)
	}
	content, err := c.client.download(context.Background(), server.URL+"/file.txt")
	if err != nil || string(content) != "private document" {
		t.Fatalf("download = %q, %v", content, err)
	}
	_, err = c.client.download(context.Background(), server.URL+"/redirect")
	if err == nil || !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("redirect to metadata endpoint was not rejected: %v", err)
	}
}
