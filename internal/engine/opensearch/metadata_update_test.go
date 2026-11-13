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

package opensearch

import "testing"

func TestCheckUpdateByQueryResult(t *testing.T) {
	cases := []struct {
		name    string
		result  map[string]interface{}
		wantErr bool
	}{
		{
			name:   "clean response",
			result: map[string]interface{}{"total": float64(1), "timed_out": false},
		},
		{
			name:    "timed out",
			result:  map[string]interface{}{"timed_out": true},
			wantErr: true,
		},
		{
			name:    "document failures",
			result:  map[string]interface{}{"failures": []interface{}{map[string]interface{}{"cause": "boom"}}},
			wantErr: true,
		},
		{
			name:   "empty failure list",
			result: map[string]interface{}{"failures": []interface{}{}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkUpdateByQueryResult(tc.result, "ragflow_meta", "update metadata by query")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
