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

import (
	"database/sql"
	"strings"
	"testing"
)

// TestDropOverlengthTenantModelInstanceKeys pins the backfill's handling of
// legacy tenant_llm keys that exceed the tenant_model_instance.api_key bound.
// The backfill copies tenant_llm.api_key — a TEXT column that can hold keys
// past 4,096 characters — into the bounded varchar(4096): in strict mode one
// over-length key would fail the whole backfill transaction and repeat on
// every startup, and a non-strict session would store the credential silently
// truncated, so such rows are skipped instead. The key is counted in
// characters, matching how MySQL measures a varchar length, not in bytes.
func TestDropOverlengthTenantModelInstanceKeys(t *testing.T) {
	rows := []tenantLLMSourceRecord{
		// Exactly at the bound: still migrates.
		{TenantID: "tenant-a", Factory: "Factory", ProviderID: "prov-at-bound",
			APIKey: sql.NullString{String: strings.Repeat("k", tenantModelInstanceAPIKeyMaxLen), Valid: true}},
		// One character past the bound: skipped instead of failing the
		// backfill or storing a truncated credential.
		{TenantID: "tenant-b", Factory: "Factory", ProviderID: "prov-over",
			APIKey: sql.NullString{String: strings.Repeat("i", tenantModelInstanceAPIKeyMaxLen+1), Valid: true}},
		// 3,000 multi-byte characters are 9,000 bytes but within the
		// 4,096-character bound MySQL counts.
		{TenantID: "tenant-c", Factory: "Factory", ProviderID: "prov-multibyte",
			APIKey: sql.NullString{String: strings.Repeat("激", 3000), Valid: true}},
		// No key at all is not an over-length row.
		{TenantID: "tenant-d", Factory: "Factory", ProviderID: "prov-null", APIKey: sql.NullString{}},
	}

	kept, overlength := dropOverlengthTenantModelInstanceKeys(rows)

	if len(overlength) != 1 {
		t.Fatalf("overlength rows = %d, want exactly the 4,097-character key", len(overlength))
	}
	if got := overlength[0].ProviderID; got != "prov-over" {
		t.Fatalf("overlength row provider = %q, want prov-over", got)
	}
	if len(kept) != 3 {
		t.Fatalf("kept rows = %d, want the at-bound, multi-byte and NULL keys kept", len(kept))
	}
	for _, row := range kept {
		if row.ProviderID == "prov-over" {
			t.Fatalf("the over-length key leaked into the kept rows")
		}
	}
}
