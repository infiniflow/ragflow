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
	"encoding/json"
	"testing"
)

// Issue #15714: tenant_rerank_id is a tenant-LLM id (an integer), matching the
// *int64 column on the entity structs — not a string. The request struct must
// accept a JSON integer and reject a JSON string (so the old strconv.ParseInt
// compensation is no longer needed).
func TestRetrievalTestRequestTenantRerankIDIsInt(t *testing.T) {
	var r RetrievalTestRequest
	if err := json.Unmarshal([]byte(`{"tenant_rerank_id": 5}`), &r); err != nil {
		t.Fatalf("integer tenant_rerank_id should unmarshal, got error: %v", err)
	}
	if r.TenantRerankID == nil || *r.TenantRerankID != 5 {
		t.Fatalf("want TenantRerankID=5, got %v", r.TenantRerankID)
	}

	var r2 RetrievalTestRequest
	if err := json.Unmarshal([]byte(`{"tenant_rerank_id": "5"}`), &r2); err == nil {
		t.Fatalf("a string tenant_rerank_id must be rejected now that the field is *int")
	}

	// omitted field stays nil (optional)
	var r3 RetrievalTestRequest
	if err := json.Unmarshal([]byte(`{}`), &r3); err != nil || r3.TenantRerankID != nil {
		t.Fatalf("absent tenant_rerank_id should be nil, got %v (err %v)", r3.TenantRerankID, err)
	}
}
