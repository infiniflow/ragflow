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

package dao

import "testing"

// The model-reference migration walks a fixed column list; a reference column
// missing from it would be skipped silently.
func TestModelIDColumnsCoverDecision(t *testing.T) {
	for _, table := range []string{"tenant", "dialog"} {
		found := false
		for _, column := range modelIDStringColumns[table] {
			if column == "decision_id" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s columns = %v, want decision_id included", table, modelIDStringColumns[table])
		}
	}
	if !modelIDFieldSet["decision_id"] {
		t.Error("modelIDFieldSet must recognise decision_id")
	}
}
