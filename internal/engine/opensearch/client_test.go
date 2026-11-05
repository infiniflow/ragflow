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

package opensearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"
	"strings"
	"testing"

	"github.com/opensearch-project/opensearch-go/v4"
)

func TestNewEngineAndCapabilities(t *testing.T) {
	createdTemplates := make(map[string]map[string]interface{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":         "test-opensearch-node",
				"cluster_name": "opensearch-cluster",
				"cluster_uuid": "test-uuid-1234",
				"version": map[string]interface{}{
					"distribution": "opensearch",
					"number":       "2.19.0",
					"build_type":   "tar",
				},
			})

		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/_index_template/"):
			templateName := strings.TrimPrefix(r.URL.Path, "/_index_template/")
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			createdTemplates[templateName] = body
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"acknowledged": true,
			})

		case r.Method == http.MethodHead && r.URL.Path == "/test_idx":
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodHead && r.URL.Path == "/missing_idx":
			w.WriteHeader(http.StatusNotFound)

		case r.Method == http.MethodDelete && r.URL.Path == "/test_idx":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"acknowledged": true,
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	osCfg := config.OpenSearchConfig{
		Hosts:    server.URL,
		Username: "admin",
		Password: "admin",
	}

	engine, err := NewEngine(ctx, osCfg)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	if got := engine.GetType(); got != "opensearch" {
		t.Errorf("GetType() = %q, want 'opensearch'", got)
	}

	if !engine.SupportsPageRank() {
		t.Errorf("SupportsPageRank() = false, want true")
	}

	if got := engine.Version(); got != "2.19.0" {
		t.Errorf("Version() = %q, want '2.19.0'", got)
	}

	if err := engine.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	if err := engine.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify all 3 templates were registered
	for _, expectedName := range []string{"ragflow_os_mapping", "ragflow_doc_meta_mapping", "ragflow_memory_mapping"} {
		tmpl, ok := createdTemplates[expectedName]
		if !ok {
			t.Errorf("expected template %q to be registered", expectedName)
			continue
		}
		patterns, _ := tmpl["index_patterns"].([]interface{})
		if len(patterns) == 0 {
			t.Errorf("template %q has empty index_patterns", expectedName)
		}
	}

	// Verify doc_meta mapping rewrote "dynamic": "runtime" to boolean true
	docMetaTmpl := createdTemplates["ragflow_doc_meta_mapping"]
	if templateObj, ok := docMetaTmpl["template"].(map[string]interface{}); ok {
		if mappings, ok := templateObj["mappings"].(map[string]interface{}); ok {
			if dyn, ok := mappings["dynamic"].(bool); !ok || !dyn {
				t.Errorf("expected doc_meta dynamic to be boolean true, got %v", mappings["dynamic"])
			}
		}
	}

	// Test indexExists
	exists, err := engine.indexExists(ctx, "test_idx")
	if err != nil || !exists {
		t.Fatalf("indexExists(test_idx) = (%v, %v), want (true, nil)", exists, err)
	}
	exists, err = engine.indexExists(ctx, "missing_idx")
	if err != nil || exists {
		t.Fatalf("indexExists(missing_idx) = (%v, %v), want (false, nil)", exists, err)
	}

	// Test dropIndex
	if err := engine.dropIndex(ctx, "test_idx"); err != nil {
		t.Fatalf("dropIndex(test_idx) failed: %v", err)
	}
	if err := engine.dropIndex(ctx, "missing_idx"); err != nil {
		t.Fatalf("dropIndex(missing_idx) should tolerate missing index, got: %v", err)
	}
}

func TestHelperUtilities(t *testing.T) {
	// Test isNotFoundError
	if !isNotFoundError(errors.New("index_not_found_exception: [test]")) {
		t.Errorf("expected isNotFoundError to be true for index_not_found_exception")
	}
	if !isNotFoundError(errors.New("resource_not_found_exception: [test]")) {
		t.Errorf("expected isNotFoundError to be true for resource_not_found_exception")
	}
	if !isNotFoundError(types.ErrIndexNotFound) {
		t.Errorf("expected isNotFoundError to be true for types.ErrIndexNotFound")
	}
	if !isNotFoundError(&opensearch.StructError{Status: http.StatusNotFound}) {
		t.Errorf("expected isNotFoundError to be true for StructError 404")
	}
	if isNotFoundError(errors.New("dial tcp 10.0.0.5:9404: connect: connection refused")) {
		t.Errorf("expected isNotFoundError to be false for port 9404 connection refused")
	}
	if isNotFoundError(errors.New("connection refused")) {
		t.Errorf("expected isNotFoundError to be false for connection refused")
	}
	if isNotFoundError(nil) {
		t.Errorf("expected isNotFoundError to be false for nil")
	}

	// Test buildMetadataIndexName
	if got := buildMetadataIndexName("tenant-123"); got != "ragflow_doc_meta_tenant-123" {
		t.Errorf("buildMetadataIndexName = %q, want 'ragflow_doc_meta_tenant-123'", got)
	}

	// Test extractErrorReason
	jsonErr := []byte(`{"error":{"root_cause":[{"reason":"no such index"}]}}`)
	if got := extractErrorReason(jsonErr); got != "no such index" {
		t.Errorf("extractErrorReason = %q, want 'no such index'", got)
	}

	jsonStringErr := []byte(`{"error":"Forbidden","status":403}`)
	if got := extractErrorReason(jsonStringErr); got != "Forbidden" {
		t.Errorf("extractErrorReason = %q, want 'Forbidden'", got)
	}
}

func TestParseAddresses(t *testing.T) {
	// 1. Valid single HTTP host
	addrs, err := parseAddresses("http://localhost:1201")
	if err != nil || len(addrs) != 1 || addrs[0] != "http://localhost:1201" {
		t.Fatalf("expected [http://localhost:1201], got %v, err=%v", addrs, err)
	}

	// 2. Valid single HTTPS host
	addrs, err = parseAddresses("https://os.example.com:9200")
	if err != nil || len(addrs) != 1 || addrs[0] != "https://os.example.com:9200" {
		t.Fatalf("expected [https://os.example.com:9200], got %v, err=%v", addrs, err)
	}

	// 3. Valid multi-host comma-separated
	addrs, err = parseAddresses("http://node1:9200, https://node2:9200")
	if err != nil || len(addrs) != 2 || addrs[0] != "http://node1:9200" || addrs[1] != "https://node2:9200" {
		t.Fatalf("expected [http://node1:9200, https://node2:9200], got %v, err=%v", addrs, err)
	}

	// 4. Missing scheme -> error
	_, err = parseAddresses("localhost:1201")
	if err == nil || !strings.Contains(err.Error(), "missing or unsupported scheme") {
		t.Fatalf("expected missing scheme error for 'localhost:1201', got: %v", err)
	}

	// 5. Unsupported scheme -> error
	_, err = parseAddresses("ftp://localhost:1201")
	if err == nil || !strings.Contains(err.Error(), "missing or unsupported scheme") {
		t.Fatalf("expected unsupported scheme error for 'ftp://localhost:1201', got: %v", err)
	}

	// 6. Multi-host with one invalid scheme -> error
	_, err = parseAddresses("http://node1:9200, node2:9200")
	if err == nil || !strings.Contains(err.Error(), "missing or unsupported scheme") {
		t.Fatalf("expected missing scheme error for multi-host, got: %v", err)
	}

	// 7. Empty or whitespace -> error
	_, err = parseAddresses("   ")
	if err == nil || !strings.Contains(err.Error(), "hosts configuration is required") {
		t.Fatalf("expected hosts required error for empty, got: %v", err)
	}

	// 8. Missing host in URL -> error
	_, err = parseAddresses("http://")
	if err == nil || !strings.Contains(err.Error(), "missing host in URL") {
		t.Fatalf("expected missing host error for 'http://', got: %v", err)
	}
}

func TestCertificateConfigurationValidation(t *testing.T) {
	ctx := context.Background()

	// 1. Missing client key when client cert is provided
	_, err := NewEngine(ctx, config.OpenSearchConfig{
		Hosts:          "https://localhost:9200",
		ClientCertPath: "/path/to/cert.pem",
	})
	if err == nil || !strings.Contains(err.Error(), "both client_cert_path and client_key_path must be provided") {
		t.Fatalf("expected mTLS validation error, got: %v", err)
	}

	// 2. Missing client cert when client key is provided
	_, err = NewEngine(ctx, config.OpenSearchConfig{
		Hosts:         "https://localhost:9200",
		ClientKeyPath: "/path/to/key.pem",
	})
	if err == nil || !strings.Contains(err.Error(), "both client_cert_path and client_key_path must be provided") {
		t.Fatalf("expected mTLS validation error, got: %v", err)
	}

	// 3. Non-existent CA cert path
	_, err = NewEngine(ctx, config.OpenSearchConfig{
		Hosts:      "https://localhost:9200",
		CACertPath: "/nonexistent/ca.pem",
	})
	if err == nil || !strings.Contains(err.Error(), "failed to read CA certificate") {
		t.Fatalf("expected CA read error, got: %v", err)
	}
}
