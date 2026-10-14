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

package service

import (
	"reflect"
	"testing"
)

func TestValidateKnowledgeCompilationScope(t *testing.T) {
	for _, scope := range []string{"", "both", "dataset", "doc"} {
		if err := ValidateKnowledgeCompilationScope(scope); err != nil {
			t.Errorf("scope %q: unexpected error %v", scope, err)
		}
	}
	for _, scope := range []string{"all", "Dataset", "docs", " doc"} {
		err := ValidateKnowledgeCompilationScope(scope)
		if err == nil || err.Error() != "knowledge_compilation_scope must be one of dataset, doc, both" {
			t.Errorf("scope %q: error = %v", scope, err)
		}
	}
}

func TestCompiledChunksFilter(t *testing.T) {
	include, exclude := true, false
	dropCompiled := map[string]interface{}{"must_not": map[string]interface{}{"exists": "compile_kwd"}}
	keepScope := func(scope string) map[string]interface{} {
		return map[string]interface{}{"must_not": map[string]interface{}{
			"exists":   "compile_kwd",
			"must_not": map[string]interface{}{"scope_kwd": scope},
		}}
	}
	tests := []struct {
		name    string
		include *bool
		scope   string
		want    map[string]interface{}
	}{
		{name: "defaults keep every row", want: nil},
		{name: "both keeps every row", include: &include, scope: "both", want: nil},
		{name: "dataset keeps dataset-level compiled rows", scope: "dataset", want: keepScope("dataset")},
		{name: "doc keeps doc-level compiled rows", include: &include, scope: "doc", want: keepScope("doc")},
		{name: "exclude drops compiled rows", include: &exclude, want: dropCompiled},
		{name: "exclude wins over scope", include: &exclude, scope: "dataset", want: dropCompiled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompiledChunksFilter(tt.include, tt.scope); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filter = %#v, want %#v", got, tt.want)
			}
		})
	}
}
