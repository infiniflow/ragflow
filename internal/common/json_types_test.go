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
	"encoding/json"
	"testing"

	"github.com/bytedance/sonic"
)

func TestStringSlice_UnmarshalJSON_Array(t *testing.T) {
	var s StringSlice
	if err := json.Unmarshal([]byte(`["a","b","c"]`), &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) != 3 || s[0] != "a" || s[1] != "b" || s[2] != "c" {
		t.Fatalf("got %v", []string(s))
	}
}

func TestStringSlice_UnmarshalJSON_SingleString(t *testing.T) {
	var s StringSlice
	if err := json.Unmarshal([]byte(`"kb1"`), &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) != 1 || s[0] != "kb1" {
		t.Fatalf("got %v", []string(s))
	}
}

func TestStringSlice_UnmarshalJSON_EmptyArray(t *testing.T) {
	var s StringSlice
	if err := json.Unmarshal([]byte(`[]`), &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) != 0 {
		t.Fatalf("expected empty, got %v", []string(s))
	}
}

func TestStringSlice_UnmarshalJSON_EmptyString(t *testing.T) {
	var s StringSlice
	if err := json.Unmarshal([]byte(`""`), &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) != 1 || s[0] != "" {
		t.Fatalf("got %v", []string(s))
	}
}

func TestStringSlice_UnmarshalJSON_InvalidValue(t *testing.T) {
	var s StringSlice
	err := json.Unmarshal([]byte(`123`), &s)
	if err == nil {
		t.Fatal("expected error for number value")
	}
}

func TestStringSlice_MarshalJSON(t *testing.T) {
	s := StringSlice{"x", "y"}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `["x","y"]` {
		t.Fatalf("got %s", data)
	}
}

func TestStringSlice_MarshalJSON_Empty(t *testing.T) {
	s := StringSlice{}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `[]` {
		t.Fatalf("got %s", data)
	}
}

func TestStringSlice_EmbeddedInStruct(t *testing.T) {
	type req struct {
		KbIDs StringSlice `json:"kb_id"`
	}
	// Single string
	var r1 req
	if err := json.Unmarshal([]byte(`{"kb_id":"kb1"}`), &r1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r1.KbIDs) != 1 || r1.KbIDs[0] != "kb1" {
		t.Fatalf("got %v", []string(r1.KbIDs))
	}
	// Array
	var r2 req
	if err := json.Unmarshal([]byte(`{"kb_id":["a","b"]}`), &r2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r2.KbIDs) != 2 || r2.KbIDs[0] != "a" || r2.KbIDs[1] != "b" {
		t.Fatalf("got %v", []string(r2.KbIDs))
	}
}

func TestFlexibleJSONString_StringOrObject(t *testing.T) {
	type req struct {
		APIKey FlexibleJSONString `json:"api_key"`
	}
	objectBody := []byte(`{"api_key":{"mineru_apiserver":"http://127.0.0.1:8000","mineru_backend":"standard"}}`)
	stringBody := []byte(`{"api_key":"{\"mineru_apiserver\":\"http://127.0.0.1:8000\"}"}`)
	issueBody := []byte(`{"instance_name":"ddd","api_key":{"mineru_apiserver":"http://127.0.0.1:8000"}}`)

	var viaJSON req
	if err := json.Unmarshal(objectBody, &viaJSON); err != nil {
		t.Fatalf("encoding/json object: %v", err)
	}
	if viaJSON.APIKey != `{"mineru_apiserver":"http://127.0.0.1:8000","mineru_backend":"standard"}` {
		t.Fatalf("encoding/json object api_key = %q", viaJSON.APIKey)
	}

	var viaSonic req
	if err := sonic.Unmarshal(objectBody, &viaSonic); err != nil {
		t.Fatalf("sonic object: %v", err)
	}
	if viaSonic.APIKey != viaJSON.APIKey {
		t.Fatalf("sonic object api_key = %q, want %q", viaSonic.APIKey, viaJSON.APIKey)
	}

	var asString req
	if err := sonic.Unmarshal(stringBody, &asString); err != nil {
		t.Fatalf("sonic string: %v", err)
	}
	if asString.APIKey != `{"mineru_apiserver":"http://127.0.0.1:8000"}` {
		t.Fatalf("sonic string api_key = %q", asString.APIKey)
	}

	type createReq struct {
		InstanceName string             `json:"instance_name"`
		APIKey       FlexibleJSONString `json:"api_key"`
	}
	var issue createReq
	if err := sonic.Unmarshal(issueBody, &issue); err != nil {
		t.Fatalf("issue #20881 body: %v", err)
	}
	if issue.InstanceName != "ddd" {
		t.Fatalf("instance_name = %q", issue.InstanceName)
	}
	if issue.APIKey != `{"mineru_apiserver":"http://127.0.0.1:8000"}` {
		t.Fatalf("issue api_key = %q", issue.APIKey)
	}
}
