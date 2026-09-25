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

package common

import (
	"bytes"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

// Fixed public test vector shared with the Python implementation.
const (
	testConnectorKey          = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	testConnectorCipherVector = "enc:v1:ZGVmZ2hpamtsbW5vMzm/FhC2IvFVBzHK4EVIiS2pKzu5X9Feh/PZO57Rh3K0yyGkLTDmruUEeZB6LtRoLedehJBJUg=="
)

func TestConnectorKey(t *testing.T) {
	t.Setenv(EnvRAGFlowConnectorKey, "")
	key, err := ConnectorKey()
	if err != nil || key != nil {
		t.Fatalf("unset key: got (%v, %v), want (nil, nil)", key, err)
	}

	t.Setenv(EnvRAGFlowConnectorKey, testConnectorKey)
	key, err = ConnectorKey()
	if err != nil {
		t.Fatalf("valid key: %v", err)
	}
	want := make([]byte, 32)
	for i := range want {
		want[i] = byte(i)
	}
	if !bytes.Equal(key, want) {
		t.Fatalf("valid key: got %v, want %v", key, want)
	}

	for _, bad := range []string{"not-base64-!!", base64.StdEncoding.EncodeToString(make([]byte, 16))} {
		t.Setenv(EnvRAGFlowConnectorKey, bad)
		key, err = ConnectorKey()
		if err == nil || key != nil {
			t.Fatalf("bad key %q: got (%v, %v), want an error", bad, key, err)
		}
		if !strings.Contains(err.Error(), EnvRAGFlowConnectorKey) || strings.Contains(err.Error(), bad) {
			t.Fatalf("bad key %q: error %q must name the variable and not the value", bad, err)
		}
	}
}

func TestConnectorCredentialsRoundTrip(t *testing.T) {
	t.Setenv(EnvRAGFlowConnectorKey, testConnectorKey)
	input := map[string]interface{}{
		"credentials": map[string]interface{}{"api_token": "tok-123"},
		"batch_size":  5,
	}

	encrypted, err := EncryptConnectorCredentials(input)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	stored, ok := encrypted["credentials"].(string)
	if !ok || !strings.HasPrefix(stored, "enc:v1:") || strings.Contains(stored, "tok-123") {
		t.Fatalf("encrypted credentials = %#v, want an enc:v1: string", encrypted["credentials"])
	}
	if encrypted["batch_size"] != 5 {
		t.Fatalf("batch_size = %#v, want 5", encrypted["batch_size"])
	}
	if _, ok := input["credentials"].(map[string]interface{}); !ok {
		t.Fatalf("encrypt mutated its input: %#v", input)
	}

	again, err := EncryptConnectorCredentials(encrypted)
	if err != nil || again["credentials"] != stored {
		t.Fatalf("encrypting an encrypted config: got (%#v, %v), want it unchanged", again, err)
	}

	decrypted, err := DecryptConnectorCredentials(encrypted)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	want := map[string]interface{}{
		"credentials": map[string]interface{}{"api_token": "tok-123"},
		"batch_size":  5,
	}
	if !reflect.DeepEqual(decrypted, want) {
		t.Fatalf("decrypted = %#v, want %#v", decrypted, want)
	}
	if encrypted["credentials"] != stored {
		t.Fatalf("decrypt mutated its input: %#v", encrypted)
	}
}

func TestDecryptConnectorCredentialsCrossRuntimeVector(t *testing.T) {
	t.Setenv(EnvRAGFlowConnectorKey, testConnectorKey)
	got, err := DecryptConnectorCredentials(map[string]interface{}{"credentials": testConnectorCipherVector, "wiki": "x"})
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	want := map[string]interface{}{
		"credentials": map[string]interface{}{"api_token": "tok-123", "user": "ada"},
		"wiki":        "x",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decrypted = %#v, want %#v", got, want)
	}
}

func TestDecryptConnectorCredentialsErrors(t *testing.T) {
	sealed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(testConnectorCipherVector, "enc:v1:"))
	if err != nil {
		t.Fatal(err)
	}
	sealed[len(sealed)-1] ^= 0x01
	tampered := "enc:v1:" + base64.StdEncoding.EncodeToString(sealed)
	wrongKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))

	cases := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{"no key", "", testConnectorCipherVector, "connector credentials are encrypted but RAGFLOW_CONNECTOR_KEY is not set"},
		{"wrong key", wrongKey, testConnectorCipherVector, "cannot decrypt connector credentials"},
		{"tampered", testConnectorKey, tampered, "cannot decrypt connector credentials"},
		{"bad base64", testConnectorKey, "enc:v1:%%%", "cannot decrypt connector credentials"},
		{"too short", testConnectorKey, "enc:v1:" + base64.StdEncoding.EncodeToString([]byte("short")), "cannot decrypt connector credentials"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvRAGFlowConnectorKey, tc.key)
			got, err := DecryptConnectorCredentials(map[string]interface{}{"credentials": tc.value})
			if err == nil {
				t.Fatalf("got %#v, want an error", got)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.want) {
				t.Fatalf("error %q does not contain %q", msg, tc.want)
			}
			for _, secret := range []string{testConnectorKey, wrongKey, "tok-123", strings.TrimPrefix(tc.value, "enc:v1:")} {
				if strings.Contains(msg, secret) {
					t.Fatalf("error %q leaks %q", msg, secret)
				}
			}
		})
	}
}

func TestConnectorCredentialsNothingToDo(t *testing.T) {
	plain := map[string]interface{}{"credentials": map[string]interface{}{"api_token": "tok-123"}}

	t.Setenv(EnvRAGFlowConnectorKey, "")
	got, err := EncryptConnectorCredentials(plain)
	if err != nil || !reflect.DeepEqual(got, plain) {
		t.Fatalf("encrypt without key: got (%#v, %v), want the input", got, err)
	}

	t.Setenv(EnvRAGFlowConnectorKey, testConnectorKey)
	noCredentials := map[string]interface{}{"wiki": "x"}
	got, err = EncryptConnectorCredentials(noCredentials)
	if err != nil || !reflect.DeepEqual(got, noCredentials) {
		t.Fatalf("encrypt without credentials: got (%#v, %v), want the input", got, err)
	}
	got, err = DecryptConnectorCredentials(plain)
	if err != nil || !reflect.DeepEqual(got, plain) {
		t.Fatalf("decrypt plaintext: got (%#v, %v), want the input", got, err)
	}
	got, err = DecryptConnectorCredentials(nil)
	if err != nil || got != nil {
		t.Fatalf("decrypt nil: got (%#v, %v), want (nil, nil)", got, err)
	}
}

func TestHasEncryptedConnectorCredentials(t *testing.T) {
	cases := []struct {
		config map[string]interface{}
		want   bool
	}{
		{map[string]interface{}{"credentials": testConnectorCipherVector}, true},
		{map[string]interface{}{"credentials": "enc:v2:abc"}, true},
		{map[string]interface{}{"credentials": "plain-token"}, false},
		{map[string]interface{}{"credentials": map[string]interface{}{"token": "enc:v1:x"}}, false},
		{map[string]interface{}{"wiki": "enc:v1:x"}, false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := HasEncryptedConnectorCredentials(tc.config); got != tc.want {
			t.Fatalf("HasEncryptedConnectorCredentials(%#v) = %v, want %v", tc.config, got, tc.want)
		}
	}
}
