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
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"ragflow/internal/server/config"
)

func TestParseAddresses(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{
			input: "http://host1:9200, http://host2:9200,http://host3:9200 ",
			want:  []string{"http://host1:9200", "http://host2:9200", "http://host3:9200"},
		},
		{
			input: "https://single-host:9200",
			want:  []string{"https://single-host:9200"},
		},
		{
			input: "",
			want:  []string{"http://localhost:1201"},
		},
		{
			input: "  , , ",
			want:  []string{"http://localhost:1201"},
		},
	} {
		got := parseAddresses(tc.input)
		if !slices.Equal(got, tc.want) {
			t.Errorf("parseAddresses(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestNewEngine_MTLSValidation(t *testing.T) {
	// Missing key when cert provided
	_, err := NewEngine(context.Background(), config.OpenSearchConfig{
		Hosts:          "http://localhost:9200",
		ClientCertPath: "/path/to/cert.pem",
	})
	if err == nil || !strings.Contains(err.Error(), "both client_cert_path and client_key_path must be provided") {
		t.Fatalf("expected mTLS pair error, got %v", err)
	}

	// Missing cert when key provided
	_, err = NewEngine(context.Background(), config.OpenSearchConfig{
		Hosts:         "http://localhost:9200",
		ClientKeyPath: "/path/to/key.pem",
	})
	if err == nil || !strings.Contains(err.Error(), "both client_cert_path and client_key_path must be provided") {
		t.Fatalf("expected mTLS pair error, got %v", err)
	}

	// Non-existent CA cert
	_, err = NewEngine(context.Background(), config.OpenSearchConfig{
		Hosts:      "http://localhost:9200",
		CACertPath: "/nonexistent/ca.pem",
	})
	if err == nil || !strings.Contains(err.Error(), "failed to read OpenSearch CA certificate") {
		t.Fatalf("expected non-existent CA cert error, got %v", err)
	}
}

func TestNewEngine_VersionValidation(t *testing.T) {
	cases := []struct {
		name            string
		infoBody        string
		wantErrContains string
	}{
		{
			name:            "rejects OpenSearch 2.10",
			infoBody:        `{"version":{"distribution":"opensearch","number":"2.10.0"}}`,
			wantErrContains: "requires OpenSearch >= 2.19",
		},
		{
			name:            "rejects OpenSearch 2.17",
			infoBody:        `{"version":{"distribution":"opensearch","number":"2.17.1"}}`,
			wantErrContains: "requires OpenSearch >= 2.19",
		},
		{
			name:            "rejects OpenSearch 2.18",
			infoBody:        `{"version":{"distribution":"opensearch","number":"2.18.0"}}`,
			wantErrContains: "requires OpenSearch >= 2.19",
		},
		{
			name:            "rejects Elasticsearch compatibility mode 7.10.2",
			infoBody:        `{"version":{"distribution":"opensearch","number":"7.10.2"}}`,
			wantErrContains: "compatibility.override_main_response_version",
		},
		{
			name:            "rejects non-OpenSearch distribution",
			infoBody:        `{"version":{"distribution":"elasticsearch","number":"8.17.0"}}`,
			wantErrContains: "unsupported OpenSearch distribution",
		},
		{
			name:            "rejects empty version",
			infoBody:        `{"version":{"distribution":"opensearch","number":""}}`,
			wantErrContains: "requires OpenSearch >= 2.19",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			templateCalled := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet && r.URL.Path == "/" {
					_, _ = w.Write([]byte(tc.infoBody))
					return
				}
				templateCalled = true
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"acknowledged":true}`))
			}))
			defer server.Close()

			_, err := NewEngine(t.Context(), config.OpenSearchConfig{
				Hosts: server.URL,
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantErrContains) {
				t.Fatalf("NewEngine() error = %v, want substring %q", err, tc.wantErrContains)
			}
			if templateCalled {
				t.Fatalf("NewEngine() must fail fast before issuing template or mapping requests")
			}
		})
	}

	t.Run("accepts OpenSearch 2.19 and proceeds to template setup", func(t *testing.T) {
		var paths []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			paths = append(paths, r.Method+" "+r.URL.Path)
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/":
				_, _ = w.Write([]byte(`{"version":{"distribution":"opensearch","number":"2.19.0"}}`))
			case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/_index_template/"):
				_, _ = w.Write([]byte(`{"acknowledged":true}`))
			case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/_cat/indices/"):
				_, _ = w.Write([]byte(`[]`))
			default:
				_, _ = w.Write([]byte(`{}`))
			}
		}))
		defer server.Close()

		eng, err := NewEngine(t.Context(), config.OpenSearchConfig{
			Hosts: server.URL,
		})
		if err != nil {
			t.Fatalf("NewEngine() unexpected error for 2.19.0: %v", err)
		}
		if eng == nil {
			t.Fatal("NewEngine() returned nil engine for 2.19.0")
		}
		if len(paths) < 3 {
			t.Fatalf("expected info + template setup calls, got %v", paths)
		}
	})
}
