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

package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTitleContextParameters checks zero defaults, serialization and validation.
func TestTitleContextParameters(t *testing.T) {
	defaults := TitleChunkerParam{}.Defaults()
	if defaults.TableContextSize != 0 || defaults.ImageContextSize != 0 {
		t.Fatal("media context must default to disabled")
	}
	for _, raw := range []string{
		`{}`, `{"table_context_size":0,"image_context_size":0}`,
		`{"table_context_size":32,"image_context_size":64}`,
	} {
		var params TitleChunkerParam
		if err := json.Unmarshal([]byte(raw), &params); err != nil {
			t.Fatal(err)
		}
		if err := params.Validate(); err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		var reloaded TitleChunkerParam
		if err := json.Unmarshal(encoded, &reloaded); err != nil {
			t.Fatal(err)
		}
		if reloaded.TableContextSize != params.TableContextSize || reloaded.ImageContextSize != params.ImageContextSize {
			t.Fatal("context windows did not survive serialization")
		}
	}
	for _, field := range []string{"table_context_size", "image_context_size"} {
		var params TitleChunkerParam
		if err := json.Unmarshal([]byte(`{"`+field+`":-1}`), &params); err != nil {
			t.Fatal(err)
		}
		if err := params.Validate(); err == nil || !strings.Contains(err.Error(), field) {
			t.Fatalf("%s was not rejected before method dispatch: %v", field, err)
		}
	}
}
