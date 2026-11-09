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

package entity

import (
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/common"
)

const testConnectorKey = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

func connectorConfigValue(t *testing.T, config ConnectorConfig) map[string]interface{} {
	t.Helper()
	value, err := config.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	raw, ok := value.([]byte)
	if !ok {
		t.Fatalf("Value returned %T, want []byte", value)
	}
	var stored map[string]interface{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("stored value %q is not JSON: %v", raw, err)
	}
	return stored
}

func TestConnectorConfigValueEncryptsCredentials(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, testConnectorKey)
	config := ConnectorConfig{"credentials": map[string]interface{}{"api_token": "tok-123"}, "wiki": "x"}

	stored := connectorConfigValue(t, config)
	credentials, ok := stored["credentials"].(string)
	if !ok || !strings.HasPrefix(credentials, "enc:v1:") {
		t.Fatalf("stored credentials = %#v, want an enc:v1: string", stored["credentials"])
	}
	if stored["wiki"] != "x" {
		t.Fatalf("stored wiki = %#v, want x", stored["wiki"])
	}
	if _, ok := config["credentials"].(map[string]interface{}); !ok {
		t.Fatalf("Value mutated the config: %#v", config)
	}
}

func TestConnectorConfigValueWithoutKey(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, "")
	stored := connectorConfigValue(t, ConnectorConfig{"credentials": map[string]interface{}{"api_token": "tok-123"}})
	credentials, ok := stored["credentials"].(map[string]interface{})
	if !ok || credentials["api_token"] != "tok-123" {
		t.Fatalf("stored credentials = %#v, want the plaintext map", stored["credentials"])
	}
}

func TestConnectorConfigValueFailsOnInvalidKey(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, "not-base64-!!")
	if value, err := (ConnectorConfig{"credentials": "tok-123"}).Value(); err == nil {
		t.Fatalf("Value = %q, want an error", value)
	}
}

func TestConnectorConfigScanDoesNotDecrypt(t *testing.T) {
	t.Setenv(common.EnvRAGFlowConnectorKey, testConnectorKey)
	var config ConnectorConfig
	if err := config.Scan([]byte(`{"credentials":"enc:v1:abc","wiki":"x"}`)); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if config["credentials"] != "enc:v1:abc" || config["wiki"] != "x" {
		t.Fatalf("scanned config = %#v, want the stored value unchanged", config)
	}
}
