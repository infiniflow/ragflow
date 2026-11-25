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
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestMigrateTenantModelInstancesKeepsPlainKeyWhenWrapperOverlength pins the
// order of the backfill's row preparation. The source query groups by
// api_key and has no ORDER BY, so a provider can contribute both a plain key
// and a legacy {"api_key":K,"is_tools":true} wrapper around the same K. The
// wrapper is longer than the plain key, so it can exceed the
// tenant_model_instance.api_key bound while K itself still fits.
//
// Dedup compares canonical keys, and the wrapper canonicalizes to K, so the
// wrapper and the plain key share one canonical key. If dedup runs first it
// keeps whichever row it meets first — possibly the over-length wrapper — and
// the over-length filter then drops it, leaving the provider with no instance
// at all even though a bounded key was available. Filtering the over-length
// rows before deduplicating keeps the bounded plain key.
//
// The test drives the real backfill through the mysql dialector on a sqlmock
// connection and asserts that the inserted api_key is the bounded plain key.
func TestMigrateTenantModelInstancesKeepsPlainKeyWhenWrapperOverlength(t *testing.T) {
	// The plain key sits exactly on the bound, so it is migratable; the
	// wrapper adds the JSON envelope and therefore exceeds it.
	plainKey := strings.Repeat("k", tenantModelInstanceAPIKeyMaxLen)
	wrapper := `{"api_key":"` + plainKey + `","is_tools":true}`
	if len(wrapper) <= tenantModelInstanceAPIKeyMaxLen {
		t.Fatalf("the wrapper fixture (%d chars) must exceed the bound (%d)", len(wrapper), tenantModelInstanceAPIKeyMaxLen)
	}

	cases := []struct {
		name string
		rows [][]any
	}{
		{
			// The over-length wrapper is met first: dedup must not let it
			// suppress the bounded plain key of the same provider.
			name: "wrapper first",
			rows: [][]any{
				{"tenant-a", "Factory", wrapper, "active", "prov-1"},
				{"tenant-a", "Factory", plainKey, "active", "prov-1"},
			},
		},
		{
			// The plain key comes first: the same key must be inserted.
			name: "plain key first",
			rows: [][]any{
				{"tenant-a", "Factory", plainKey, "active", "prov-1"},
				{"tenant-a", "Factory", wrapper, "active", "prov-1"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := openMySQLSpecMock(t)

			expectTableProbe(mock, "tenant_llm", true)
			expectTableProbe(mock, "tenant_model_provider", true)
			expectTableProbe(mock, "tenant_model_instance", true)

			// No instance row exists yet, so nothing is filtered as already
			// migrated.
			mock.ExpectQuery(`SELECT provider_id, api_key FROM tenant_model_instance`).
				WillReturnRows(sqlmock.NewRows([]string{"provider_id", "api_key"}))

			sourceRows := sqlmock.NewRows([]string{"tenant_id", "llm_factory", "api_key", "status", "provider_id"})
			for _, row := range tc.rows {
				sourceRows.AddRow(row[0], row[1], row[2], row[3], row[4])
			}
			mock.ExpectQuery(`SELECT tl\.tenant_id AS tenant_id`).
				WillReturnRows(sourceRows)

			mock.ExpectBegin()
			mock.ExpectExec(`INSERT INTO tenant_model_instance`).
				WithArgs(sqlmock.AnyArg(), modelMigrationInstanceName, "prov-1", plainKey, "active", "{}",
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()

			if err := migrateTenantModelInstances(context.Background(), db); err != nil {
				t.Fatalf("migrateTenantModelInstances: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sqlmock expectations: %v", err)
			}
		})
	}
}
