//
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
//

package service

import (
	"testing"
)

type mockDocEngineType struct {
	skillSearchEngine
	t string
}

func (m *mockDocEngineType) GetType() string {
	return m.t
}

func TestIsElasticsearchIncludesOpenSearch(t *testing.T) {
	for _, tc := range []struct {
		engineType string
		want       bool
	}{
		{"elasticsearch", true},
		{"opensearch", true},
		{"infinity", false},
		{"oceanbase", false},
		{"", false},
	} {
		got := isElasticsearch(&mockDocEngineType{t: tc.engineType})
		if got != tc.want {
			t.Errorf("isElasticsearch(%q) = %v, want %v", tc.engineType, got, tc.want)
		}
	}
}
