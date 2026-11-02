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

package elasticsearch

import (
	"reflect"
	"testing"
)

func TestMessageSubstringEscapesWildcardCharacters(t *testing.T) {
	got := buildBoolQueryFromConditionRaw(map[string]interface{}{"session_id_contains": `a*b?c\d`}, nil, false, true)
	want := map[string]interface{}{"filter": []interface{}{map[string]interface{}{"wildcard": map[string]interface{}{"session_id": `*a\*b\?c\\d*`}}}}
	if !reflect.DeepEqual(got, map[string]interface{}{"bool": want}) {
		t.Fatalf("query = %#v, want %#v", got, want)
	}
}
