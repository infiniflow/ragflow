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

package dataset

import "testing"

func TestValidateDatasetEmbeddingModelAcceptsModelRefForms(t *testing.T) {
	const groupID = "88b17c7a12344f2f9e0d5a6b7c8d9e0f"
	tests := []struct {
		name    string
		embd    string
		wantErr bool
	}{
		{"bare tenant model id", "5b6f1d0c8a2e4f3b9c7d1e2f3a4b5c6d", false},
		{"model group reference", "gp-" + groupID, false},
		{"two-part composite", "deepseek-chat@deepseek", false},
		{"three-part composite", "deepseek-chat@default@deepseek", false},
		{"group prefix without group id", "gp-", true},
		{"group prefix with malformed id", "gp-not-a-group-id", true},
		{"bare name without provider", "deepseek-chat", true},
		{"empty provider", "deepseek-chat@", true},
		{"empty model name", "@deepseek", true},
		{"empty reference", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDatasetEmbeddingModel(tt.embd)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateDatasetEmbeddingModel(%q) err=%v, wantErr=%v", tt.embd, err, tt.wantErr)
			}
		})
	}
}
