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
	"context"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// setupChangesDB adds the measurement table: the drift projection reads it, and
// the governance tests alone never migrate it.
func setupChangesDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupGovernanceDB(t)
	if err := db.AutoMigrate(&entity.OntologyInsight{}); err != nil {
		t.Fatalf("failed to migrate insight: %v", err)
	}
	return db
}

func saveGovernanceInsight(t *testing.T, db *gorm.DB, templateID, datasetID string, version int, drift entity.JSONMap) {
	t.Helper()
	insight := &entity.OntologyInsight{
		ID:         templateID + "-" + datasetID,
		TenantID:   governanceTenant,
		TemplateID: templateID,
		DatasetID:  datasetID,
		Version:    version,
		AsOf:       time.Now().UnixMilli(),
		Payload:    entity.JSONMap{"drift": drift},
	}
	if err := db.Create(insight).Error; err != nil {
		t.Fatalf("save insight %q: %v", datasetID, err)
	}
}

// editDraftAfterPublish rewrites the working config and returns the model to
// draft, which is the state the group save path leaves behind.
func editDraftAfterPublish(t *testing.T, db *gorm.DB, templateID string, config map[string]interface{}) {
	t.Helper()
	if err := db.Model(&entity.CompilationTemplate{}).Where("id = ?", templateID).
		Updates(map[string]interface{}{
			"config":         entity.JSONMap(config),
			"publish_status": entity.PublishStatusDraft,
		}).Error; err != nil {
		t.Fatalf("edit working config: %v", err)
	}
}

func configWithExtraClass(t *testing.T, base map[string]interface{}, class string) map[string]interface{} {
	t.Helper()
	edited := map[string]interface{}{}
	for key, value := range base {
		edited[key] = value
	}
	section, ok := edited["entity"].(map[string]interface{})
	if !ok {
		t.Fatalf("base config has no entity section: %#v", edited["entity"])
	}
	copied := map[string]interface{}{}
	for key, value := range section {
		copied[key] = value
	}
	fields, _ := copied["fields"].([]interface{})
	copied["fields"] = append(append([]interface{}{}, fields...), map[string]interface{}{
		"type": class, "label": class, "parent": "",
	})
	edited["entity"] = copied
	return edited
}

// The drift panel asks the same two questions the publish dialog does — what does
// the draft change, and what would still disagree — but it must be answerable
// without reviewer rights.
func TestPendingChangeSetReportsTheDraftAndTheDriftItWouldLeave(t *testing.T) {
	db := setupChangesDB(t)
	template := saveGovernanceTemplate(t, db, "tpl-pending", ontologyTemplateConfig("place"))
	svc := NewCompilationTemplateGovernanceService()
	ctx := context.Background()

	if _, err := svc.PublishVersion(ctx, db, governanceTenant, template.ID, "user-1", "first release", false); err != nil {
		t.Fatalf("publish: %v", err)
	}
	editDraftAfterPublish(t, db, template.ID, configWithExtraClass(t, ontologyTemplateConfig("place"), "Invoice"))
	saveGovernanceInsight(t, db, template.ID, "ds-1", 1, entity.JSONMap{
		"undeclared_classes": []interface{}{
			map[string]interface{}{"type": "Invoice", "entities": 18401},
			map[string]interface{}{"type": "Shipment", "entities": 4210},
		},
		"unused_classes": []interface{}{
			map[string]interface{}{"type": "Warranty", "entities": 0},
		},
		"unused_properties": []interface{}{
			map[string]interface{}{"type": "ships_to", "source": "Vendor", "target": "Place"},
		},
	})

	pending, err := svc.PendingChangeSet(ctx, db, governanceTenant, template.ID)
	if err != nil {
		t.Fatalf("pending changes: %v", err)
	}
	if pending.CurrentVersion != 1 {
		t.Fatalf("current version = %d, want 1", pending.CurrentVersion)
	}
	if !pending.DraftPending {
		t.Fatal("expected the edit after publishing to count as pending")
	}
	if len(pending.Changes) == 0 {
		t.Fatal("expected the added class to appear in the change list")
	}
	if pending.Drift == nil {
		t.Fatal("expected a drift projection when a dataset has been measured")
	}
	if pending.Drift.Datasets != 1 {
		t.Fatalf("datasets = %d, want 1", pending.Drift.Datasets)
	}
	if pending.Drift.Undeclared != 2 {
		t.Fatalf("undeclared = %d, want 2", pending.Drift.Undeclared)
	}
	// Invoice is declared by the draft, so only Shipment survives the publish.
	if pending.Drift.UndeclaredAfter != 1 {
		t.Fatalf("undeclared_after = %d, want 1 (Invoice is covered by the draft)", pending.Drift.UndeclaredAfter)
	}
	if got := strings.Join(pending.Drift.Remaining, ","); got != "Shipment" {
		t.Fatalf("remaining = %q, want Shipment", got)
	}
	if pending.Drift.UnusedClasses != 1 || pending.Drift.UnusedRelations != 1 {
		t.Fatalf("unused = %d/%d, want 1 class and 1 relation",
			pending.Drift.UnusedClasses, pending.Drift.UnusedRelations)
	}
}

// Publishing folds the state back into the version, so a fresh publish has
// nothing pending and the projection does not claim otherwise.
func TestPendingChangeSetIsEmptyRightAfterPublishing(t *testing.T) {
	db := setupChangesDB(t)
	template := saveGovernanceTemplate(t, db, "tpl-clean", ontologyTemplateConfig("place"))
	svc := NewCompilationTemplateGovernanceService()
	ctx := context.Background()

	if _, err := svc.PublishVersion(ctx, db, governanceTenant, template.ID, "user-1", "first release", false); err != nil {
		t.Fatalf("publish: %v", err)
	}

	pending, err := svc.PendingChangeSet(ctx, db, governanceTenant, template.ID)
	if err != nil {
		t.Fatalf("pending changes: %v", err)
	}
	if pending.DraftPending {
		t.Fatalf("draft_pending = true with changes %#v, want false", pending.Changes)
	}
	if len(pending.Changes) != 0 {
		t.Fatalf("changes = %#v, want none", pending.Changes)
	}
	if pending.Drift != nil {
		t.Fatalf("drift = %#v, want nil when no dataset has been measured", pending.Drift)
	}
}

// The projection subtracts only classes the draft actually declares. A class
// that was already declared before the publish must not be treated as covered.
func TestPublishDriftCountsOnlyClassesTheDraftAdds(t *testing.T) {
	db := setupChangesDB(t)
	template := saveGovernanceTemplate(t, db, "tpl-existing", ontologyTemplateConfig("place"))
	svc := NewCompilationTemplateGovernanceService()
	ctx := context.Background()

	if _, err := svc.PublishVersion(ctx, db, governanceTenant, template.ID, "user-1", "first release", false); err != nil {
		t.Fatalf("publish: %v", err)
	}
	// person/place are declared in both versions; the edit only changes a label.
	edited := ontologyTemplateConfig("place")
	editDraftAfterPublish(t, db, template.ID, edited)
	saveGovernanceInsight(t, db, template.ID, "ds-1", 1, entity.JSONMap{
		"undeclared_classes": []interface{}{
			map[string]interface{}{"type": "person", "entities": 12},
		},
	})

	pending, err := svc.PendingChangeSet(ctx, db, governanceTenant, template.ID)
	if err != nil {
		t.Fatalf("pending changes: %v", err)
	}
	if pending.Drift == nil {
		t.Fatal("expected a drift projection")
	}
	// `person` is already declared by v1, so the draft does not cover it.
	if pending.Drift.UndeclaredAfter != 1 {
		t.Fatalf("undeclared_after = %d, want 1", pending.Drift.UndeclaredAfter)
	}
	if got := strings.Join(pending.Drift.Remaining, ","); got != "person" {
		t.Fatalf("remaining = %q, want person", got)
	}
}

// The publish dialog renders the same two blocks from one response.
func TestPublishCheckCarriesChangesAndDrift(t *testing.T) {
	db := setupChangesDB(t)
	template := saveGovernanceTemplate(t, db, "tpl-check", ontologyTemplateConfig("place"))
	svc := NewCompilationTemplateGovernanceService()
	ctx := context.Background()

	if _, err := svc.PublishVersion(ctx, db, governanceTenant, template.ID, "user-1", "first release", false); err != nil {
		t.Fatalf("publish: %v", err)
	}
	editDraftAfterPublish(t, db, template.ID, configWithExtraClass(t, ontologyTemplateConfig("place"), "Invoice"))
	saveGovernanceInsight(t, db, template.ID, "ds-1", 1, entity.JSONMap{
		"undeclared_classes": []interface{}{
			map[string]interface{}{"type": "Invoice", "entities": 18401},
		},
	})

	check, err := svc.PublishCheck(ctx, db, governanceTenant, template.ID)
	if err != nil {
		t.Fatalf("publish check: %v", err)
	}
	if len(check.Changes) == 0 {
		t.Fatal("expected the check to carry the pending change list")
	}
	if check.Drift == nil {
		t.Fatal("expected the check to carry the drift projection")
	}
	if check.Drift.UndeclaredAfter != 0 {
		t.Fatalf("undeclared_after = %d, want 0 (the only undeclared class is being declared)",
			check.Drift.UndeclaredAfter)
	}
}
