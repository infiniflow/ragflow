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
	"net/url"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// newDriftDismissalDB migrates just the dismissal table: the service takes the
// connection as an argument, so the test does not need dao.DB or any other
// schema.
func newDriftDismissalDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := "file:" + url.QueryEscape(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err = db.AutoMigrate(
		&entity.OntologyDriftDismissal{},
		// The last test rewrites a measurement; the table has to exist for that
		// to be a real rewrite rather than a missing-table error.
		&entity.OntologyInsight{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func dismissTestService() *OntologyDriftDismissalService {
	return NewOntologyDriftDismissalService()
}

func TestDismissDriftFindingRequiresReason(t *testing.T) {
	db := newDriftDismissalDB(t)
	svc := dismissTestService()

	for _, reason := range []string{"", "   \n\t "} {
		if _, err := svc.Dismiss(t.Context(), db, "t1", "model1", "ds1", "Vendor", reason, "u1"); err == nil {
			t.Fatalf("expected an error for reason %q", reason)
		}
	}

	rows, err := svc.List(t.Context(), db, "t1", "model1", "ds1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("a refused dismissal must not be stored, got %d rows", len(rows))
	}
}

func TestDismissDriftFindingRequiresClass(t *testing.T) {
	db := newDriftDismissalDB(t)
	svc := dismissTestService()

	if _, err := svc.Dismiss(t.Context(), db, "t1", "model1", "ds1", "  ", "why", "u1"); err == nil {
		t.Fatal("expected an error for an empty class name")
	}
}

func TestDismissDriftFindingRecordsReasonAndActor(t *testing.T) {
	db := newDriftDismissalDB(t)
	svc := dismissTestService()

	rows, err := svc.Dismiss(t.Context(), db, "t1", "model1", "ds1", " Vendor ", " invoices inside contracts ", "u42")
	if err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected the fresh list back, got %d rows", len(rows))
	}
	got := rows[0]
	if got.ClassName != "Vendor" {
		t.Errorf("class name = %q, want the trimmed %q", got.ClassName, "Vendor")
	}
	if got.Reason != "invoices inside contracts" {
		t.Errorf("reason = %q, want the trimmed text", got.Reason)
	}
	if got.DismissedBy != "u42" {
		t.Errorf("dismissed_by = %q, want u42", got.DismissedBy)
	}
}

// Re-dismissing is a correction of the reason, not a second decision: two rows
// for one class would leave the reader guessing which one is current.
func TestDismissDriftFindingReplacesEarlierReason(t *testing.T) {
	db := newDriftDismissalDB(t)
	svc := dismissTestService()

	if _, err := svc.Dismiss(t.Context(), db, "t1", "model1", "ds1", "Vendor", "first reading", "u1"); err != nil {
		t.Fatalf("first dismiss: %v", err)
	}
	rows, err := svc.Dismiss(t.Context(), db, "t1", "model1", "ds1", "Vendor", "second reading", "u2")
	if err != nil {
		t.Fatalf("second dismiss: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row after re-dismissing, got %d", len(rows))
	}
	if rows[0].Reason != "second reading" || rows[0].DismissedBy != "u2" {
		t.Fatalf("row was not replaced: %+v", rows[0])
	}
}

// The same class can be a real gap in one corpus and an artefact of another, so
// a decision may not leak across dataset, model or tenant boundaries.
func TestDismissDriftFindingIsScopedToOneDataset(t *testing.T) {
	db := newDriftDismissalDB(t)
	svc := dismissTestService()

	if _, err := svc.Dismiss(t.Context(), db, "t1", "model1", "ds1", "Vendor", "not a gap here", "u1"); err != nil {
		t.Fatalf("dismiss: %v", err)
	}

	cases := []struct{ tenant, template, dataset string }{
		{"t1", "model1", "ds2"},
		{"t1", "model2", "ds1"},
		{"t2", "model1", "ds1"},
	}
	for _, scope := range cases {
		rows, err := svc.List(t.Context(), db, scope.tenant, scope.template, scope.dataset)
		if err != nil {
			t.Fatalf("list %+v: %v", scope, err)
		}
		if len(rows) != 0 {
			t.Fatalf("dismissal leaked into %+v: %+v", scope, rows)
		}
	}
}

func TestRestoreDriftFindingBringsTheRowBack(t *testing.T) {
	db := newDriftDismissalDB(t)
	svc := dismissTestService()

	if _, err := svc.Dismiss(t.Context(), db, "t1", "model1", "ds1", "Vendor", "not a gap here", "u1"); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if _, err := svc.Dismiss(t.Context(), db, "t1", "model1", "ds1", "Contract", "folded into another type", "u1"); err != nil {
		t.Fatalf("dismiss: %v", err)
	}

	rows, err := svc.Restore(t.Context(), db, "t1", "model1", "ds1", "Vendor")
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(rows) != 1 || rows[0].ClassName != "Contract" {
		t.Fatalf("restore must remove only the named class, got %+v", rows)
	}

	// Lifting something that was never set aside is a no-op, not an error: the
	// reader may have two tabs open.
	rows, err = svc.Restore(t.Context(), db, "t1", "model1", "ds1", "Vendor")
	if err != nil {
		t.Fatalf("restore of a missing class: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("restore of a missing class changed the list: %+v", rows)
	}

	if _, err = svc.Restore(t.Context(), db, "t1", "model1", "ds1", " "); err == nil {
		t.Fatal("expected an error for an empty class name")
	}
}

// A recompile rewrites the insight row; the decision lives in its own table so
// it must survive a rerun of the measurement.
func TestDismissDriftFindingSurvivesMeasurementRewrite(t *testing.T) {
	db := newDriftDismissalDB(t)
	svc := dismissTestService()

	if _, err := svc.Dismiss(t.Context(), db, "t1", "model1", "ds1", "Vendor", "not a gap here", "u1"); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	insight := &entity.OntologyInsight{
		ID:         "insight1",
		TenantID:   "t1",
		TemplateID: "model1",
		DatasetID:  "ds1",
		Version:    3,
		AsOf:       1,
		Payload:    entity.JSONMap{"drift": map[string]any{}},
	}
	if err := db.Create(insight).Error; err != nil {
		t.Fatalf("seed measurement: %v", err)
	}
	// A recompile writes the row; the next measurement must not carry the
	// decision with it.
	if err := db.Where("tenant_id = ?", "t1").Delete(&entity.OntologyInsight{}).Error; err != nil {
		t.Fatalf("clear measurements: %v", err)
	}

	rows, err := svc.List(t.Context(), db, "t1", "model1", "ds1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].ClassName != "Vendor" {
		t.Fatalf("the decision did not survive the measurement being rewritten: %+v", rows)
	}
}
