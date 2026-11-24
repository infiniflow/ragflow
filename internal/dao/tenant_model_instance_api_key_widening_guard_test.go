//
//  Copyright the InfiniFlow Authors. All Rights Reserved.
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
	"errors"
	"strings"
	"testing"
)

// TestFailedAPIKeyWideningStopsTheTenantModelBackfill pins the ordering the
// migration depends on: the tenant model backfill copies tenant_llm keys into
// tenant_model_instance.api_key, so it must not run against a column the
// widening left at the legacy varchar(512). The check is driven through the
// migration flow rather than through applyColumnSpec alone, because the
// failure it has to prevent is the backfill's, not the ALTER's.
//
// The backfill's own first statement is the source SELECT. No expectation is
// registered for it, so any attempt to read tenant_llm is an unexpected call
// and fails the test — that is how "the backfill did not run" is asserted.
func TestFailedAPIKeyWideningStopsTheTenantModelBackfill(t *testing.T) {
	db, mock := openMySQLSpecMock(t)

	// The widening ALTER fails: the column keeps its legacy narrow shape.
	expectSchemaLookups(mock, "tenant_model_instance", "api_key", "varchar(512)", "NO")
	mock.ExpectExec(
		`ALTER TABLE tenant_model_instance MODIFY COLUMN api_key varchar\(4096\) NOT NULL`,
	).WillReturnError(errors.New("ERROR 1071 (42000): Specified key was too long; max key length is 3072 bytes"))

	err := applyColumnSpec(context.Background(), db, tenantModelInstanceAPIKeyColumnSpec)
	if err == nil {
		t.Fatal("the widening ALTER failure must be reported, not swallowed")
	}

	// The migration flow re-reads the column and refuses to continue: the
	// column is still varchar(512), not the varchar(4096) the backfill reads
	// through.
	expectSchemaLookups(mock, "tenant_model_instance", "api_key", "varchar(512)", "NO")
	err = requireTenantModelInstanceAPIKeyWidened(context.Background(), db)
	if err == nil {
		t.Fatal("the migration must refuse to continue with a narrow api_key column")
	}
	if !strings.Contains(err.Error(), "api_key") || !strings.Contains(err.Error(), "varchar(512)") {
		t.Fatalf("the refusal must name the column and its actual width, got %q", err.Error())
	}

	// The backfill never ran: it issues no statement of its own here, and the
	// mock has no expectation left for it.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestWidenedAPIKeyColumnLetsTheBackfillProceed pins the other side of the
// guard: once the column carries the target width, the check passes and the
// backfill is free to run.
func TestWidenedAPIKeyColumnLetsTheBackfillProceed(t *testing.T) {
	db, mock := openMySQLSpecMock(t)

	expectSchemaLookups(mock, "tenant_model_instance", "api_key", "varchar(4096)", "NO")
	if err := requireTenantModelInstanceAPIKeyWidened(context.Background(), db); err != nil {
		t.Fatalf("a widened api_key column must not block the backfill: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestAPIKeyWideningGuardSkipsAMissingTable pins the guard's tolerance for a
// fresh or pre-split install: without the table there is nothing to widen and
// nothing to refuse, and the backfill creates the table itself.
func TestAPIKeyWideningGuardSkipsAMissingTable(t *testing.T) {
	db, mock := openMySQLSpecMock(t)

	expectTableProbe(mock, "tenant_model_instance", false)
	if err := requireTenantModelInstanceAPIKeyWidened(context.Background(), db); err != nil {
		t.Fatalf("a missing table must not block the migration: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("a missing table must trigger no column reads: %v", err)
	}
}
