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

package runtime

import (
	"encoding/json"
	"testing"
)

func TestUnmarshalCheckpointJSONTypedContainers(t *testing.T) {
	type item struct {
		ID      uint64         `json:"id"`
		Count   json.Number    `json:"count"`
		Dynamic map[string]any `json:"dynamic"`
	}
	var values []item
	err := UnmarshalCheckpointJSON([]byte(`[{"id":11099848653006015581,"count":2,"dynamic":{"ids":[11099848653006015581,-9007199254740992],"count":2,"score":1.5,"none":null}}]`), &values)
	if err != nil {
		t.Fatal(err)
	}
	if values[0].ID != uint64(11099848653006015581) {
		t.Fatalf("typed ID = %d", values[0].ID)
	}
	if values[0].Count != json.Number("2") {
		t.Fatalf("typed json.Number = %#v", values[0].Count)
	}
	dynamic := values[0].Dynamic
	ids := dynamic["ids"].([]any)
	if ids[0] != json.Number("11099848653006015581") || ids[1] != json.Number("-9007199254740992") {
		t.Fatalf("dynamic IDs = %#v", ids)
	}
	if dynamic["count"] != float64(2) || dynamic["score"] != float64(1.5) || dynamic["none"] != nil {
		t.Fatalf("dynamic fields = %#v", dynamic)
	}
}

func TestUnmarshalCheckpointJSONRejectsMalformedNumbersAndTrailingData(t *testing.T) {
	for _, input := range []string{`{"value":1e999}`, `{"value":1} {"value":2}`, `{"value":1} garbage`} {
		t.Run(input, func(t *testing.T) {
			var value map[string]any
			if err := UnmarshalCheckpointJSON([]byte(input), &value); err == nil {
				t.Fatalf("accepted %s", input)
			}
		})
	}
	var value any
	if err := UnmarshalCheckpointJSON([]byte("42 \n\t"), &value); err != nil || value != float64(42) {
		t.Fatalf("whitespace: %v, %v", value, err)
	}
}
