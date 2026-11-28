// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nlp

import (
	"reflect"
	"testing"
)

// TestExtractTagFeasMapAcceptsTheArrayForm covers the shape the Infinity
// backend stores so tag boosting keeps working for callers that read the raw
// column (#22454).
func TestExtractTagFeasMapAcceptsTheArrayForm(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{"物流查询": float64(7)},
		map[string]interface{}{"售后服务": 8},
	}

	got := extractTagFeasMap(raw)

	want := map[string]float64{"物流查询": 7, "售后服务": 8}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("extractTagFeasMap(array) = %#v, want %#v", got, want)
	}
}

// TestExtractTagFeasMapRejectsAForeignArray keeps the fallback from turning an
// unrelated array into tag weights.
func TestExtractTagFeasMapRejectsAForeignArray(t *testing.T) {
	raw := []interface{}{map[string]interface{}{"a": 1, "b": 2}}

	if got := extractTagFeasMap(raw); got != nil {
		t.Errorf("extractTagFeasMap(foreign array) = %#v, want nil", got)
	}
}
