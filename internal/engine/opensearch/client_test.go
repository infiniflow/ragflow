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
