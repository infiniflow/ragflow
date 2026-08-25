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
	"testing"
)

func TestResolveReferenceMetadata_NilInputs(t *testing.T) {
	include, fields := ResolveReferenceMetadata(nil, nil)
	if include {
		t.Error("expected include=false")
	}
	if fields != nil {
		t.Errorf("expected nil fields, got %v", fields)
	}
}

func TestResolveReferenceMetadata_EmptyInputs(t *testing.T) {
	include, fields := ResolveReferenceMetadata(map[string]interface{}{}, map[string]interface{}{})
	if include {
		t.Error("expected include=false")
	}
	if fields != nil {
		t.Errorf("expected nil fields, got %v", fields)
	}
}

func TestResolveReferenceMetadata_FromConfig(t *testing.T) {
	config := map[string]interface{}{
		"reference_metadata": map[string]interface{}{
			"include": true,
			"fields":  []interface{}{"author", "date"},
		},
	}
	include, fields := ResolveReferenceMetadata(nil, config)
	if !include {
		t.Error("expected include=true")
	}
	if len(fields) != 2 || fields[0] != "author" || fields[1] != "date" {
		t.Errorf("expected [author date], got %v", fields)
	}
}

func TestResolveReferenceMetadata_RequestOverridesConfig(t *testing.T) {
	config := map[string]interface{}{
		"reference_metadata": map[string]interface{}{
			"include": true,
			"fields":  []interface{}{"config_field"},
		},
	}
	request := map[string]interface{}{
		"reference_metadata": map[string]interface{}{
			"include": false,
			"fields":  []interface{}{"request_field"},
		},
	}
	include, fields := ResolveReferenceMetadata(request, config)
	if include {
		t.Error("expected include=false (request overrides)")
	}
	if len(fields) != 1 || fields[0] != "request_field" {
		t.Errorf("expected [request_field], got %v", fields)
	}
}

func TestResolveReferenceMetadata_LegacyKeys(t *testing.T) {
	request := map[string]interface{}{
		"include_metadata": true,
		"metadata_fields":  []interface{}{"author", "category"},
	}
	include, fields := ResolveReferenceMetadata(request, nil)
	if !include {
		t.Error("expected include=true from legacy key")
	}
	if len(fields) != 2 || fields[0] != "author" {
		t.Errorf("expected [author category], got %v", fields)
	}
}

func TestResolveReferenceMetadata_LegacyIncludeFalse(t *testing.T) {
	request := map[string]interface{}{
		"include_metadata": false,
	}
	include, fields := ResolveReferenceMetadata(request, nil)
	if include {
		t.Error("expected include=false")
	}
	if fields != nil {
		t.Errorf("expected nil fields, got %v", fields)
	}
}

func TestResolveReferenceMetadata_NoFields(t *testing.T) {
	config := map[string]interface{}{
		"reference_metadata": map[string]interface{}{
			"include": true,
		},
	}
	include, fields := ResolveReferenceMetadata(nil, config)
	if !include {
		t.Error("expected include=true")
	}
	if fields != nil {
		t.Errorf("expected nil fields, got %v", fields)
	}
}

func TestResolveReferenceMetadata_PartialOverride(t *testing.T) {
	config := map[string]interface{}{
		"reference_metadata": map[string]interface{}{
			"include": true,
			"fields":  []interface{}{"from_config"},
		},
	}
	request := map[string]interface{}{
		"reference_metadata": map[string]interface{}{
			"include": false, // override include only, fields stays from config
		},
	}
	include, fields := ResolveReferenceMetadata(request, config)
	if include {
		t.Error("expected include=false (request overrides)")
	}
	if len(fields) != 1 || fields[0] != "from_config" {
		t.Errorf("expected [from_config], got %v", fields)
	}
}
