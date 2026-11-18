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
	"time"
)

func TestSafeJSONMarshalPreservesStandardJSONTypes(t *testing.T) {
	when := time.Date(2026, time.September, 24, 1, 2, 3, 0, time.UTC)
	raw, err := SafeJSONMarshal(map[string]any{
		"when":  when,
		"bytes": []byte{1, 2},
	})
	if err != nil {
		t.Fatalf("SafeJSONMarshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got["when"] != "2026-09-24T01:02:03Z" {
		t.Errorf("when = %v, want RFC3339 timestamp", got["when"])
	}
	if got["bytes"] != "AQI=" {
		t.Errorf("bytes = %v, want base64-encoded bytes", got["bytes"])
	}
}
