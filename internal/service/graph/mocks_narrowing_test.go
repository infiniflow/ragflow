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

package graph

import (
	"reflect"
	"testing"
)

// TestGraphMocksEmbedNarrowRole pins that every test double in the graph
// package depends only on the narrow engine.Searcher role, not the full
// 31-method engine.DocEngine. service/graph was narrowed to Searcher; a mock
// embedding DocEngine keeps the fat-interface pattern this decomposition
// removes and hides the package's real dependency behind the whole engine
// surface. The reflection check fails if a wide interface is embedded again.
func TestGraphMocksEmbedNarrowRole(t *testing.T) {
	mocks := []struct {
		name string
		val  any
	}{
		{"mockKGEngine", &mockKGEngine{}},
		{"mockRetrievalEngine", &mockRetrievalEngine{}},
		{"searchCaptureEngine", &searchCaptureEngine{}},
	}
	for _, m := range mocks {
		t.Run(m.name, func(t *testing.T) {
			ft := reflect.TypeOf(m.val).Elem()
			if ft.NumField() == 0 {
				t.Fatalf("%s has no fields; want an embedded engine interface", m.name)
			}
			emb := ft.Field(0)
			if got, want := emb.Type.String(), "engine.Searcher"; got != want {
				t.Errorf("%s embeds %s, want %s (narrow role, not the full DocEngine)", m.name, got, want)
			}
		})
	}
}
