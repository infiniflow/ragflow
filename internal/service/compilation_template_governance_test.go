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
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

const governanceTenant = "tenant-1"

func setupGovernanceDB(t *testing.T) *gorm.DB {
	t.Helper()
	// A shared-cache memory database keeps the transaction's connection and the
	// pool's other connections on one schema.
	dsn := "file:" + url.QueryEscape(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.CompilationTemplate{},
		&entity.CompilationTemplateVersion{},
		&entity.Knowledgebase{},
		&entity.DatasetModelRef{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// saveGovernanceDataset writes a dataset the way the application does, so its
// compilation model references are indexed by the write path under test.
func saveGovernanceDataset(t *testing.T, db *gorm.DB, id, name string, config entity.JSONMap) *entity.Knowledgebase {
	t.Helper()
	valid := string(entity.StatusValid)
	kb := &entity.Knowledgebase{
		ID:           id,
		TenantID:     governanceTenant,
		Name:         name,
		Status:       &valid,
		ParserConfig: config,
	}
	if err := dao.NewKnowledgebaseDAO().Create(context.Background(), db, kb); err != nil {
		t.Fatalf("create dataset %q: %v", id, err)
	}
	return kb
}

// templateRefsOf reads the raw reference rows of a dataset, including the rows
// of a soft-deleted one.
func templateRefsOf(t *testing.T, db *gorm.DB, datasetID string) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&entity.DatasetModelRef{}).Where("dataset_id = ?", datasetID).
		Count(&count).Error; err != nil {
		t.Fatalf("count refs of %q: %v", datasetID, err)
	}
	return count
}

// ontologyTemplateConfig builds a two-class model with one object property. The
// property's range is the parameter, so a test can point it at a class the model
// never declares.
func ontologyTemplateConfig(rangeClass string) map[string]interface{} {
	return map[string]interface{}{
		"kind": "ontology",
		"entity": map[string]interface{}{
			"fields": []interface{}{
				map[string]interface{}{"type": "person", "label": "Person", "parent": ""},
				map[string]interface{}{"type": "place", "label": "Place", "parent": ""},
			},
		},
		"relation": map[string]interface{}{
			"fields": []interface{}{
				map[string]interface{}{
					"type": "born_in", "kind": "object", "domain": "person", "range": rangeClass,
				},
			},
		},
	}
}

// propertylessOntologyConfig declares classes that no property touches: a model
// that can be compiled, but that will only ever produce entities.
func propertylessOntologyConfig() map[string]interface{} {
	return map[string]interface{}{
		"kind": "ontology",
		"entity": map[string]interface{}{
			"fields": []interface{}{
				map[string]interface{}{"type": "person", "label": "Person", "parent": ""},
			},
		},
		"relation": map[string]interface{}{"fields": []interface{}{}},
	}
}

func saveGovernanceTemplate(t *testing.T, db *gorm.DB, id string, config map[string]interface{}) *entity.CompilationTemplate {
	t.Helper()
	tenantID := governanceTenant
	valid := string(entity.StatusValid)
	template := &entity.CompilationTemplate{
		ID:            id,
		TenantID:      &tenantID,
		Name:          "Model " + id,
		Kind:          "ontology",
		Config:        entity.JSONMap(config),
		Status:        &valid,
		PublishStatus: entity.PublishStatusDraft,
	}
	if err := db.Create(template).Error; err != nil {
		t.Fatalf("create template: %v", err)
	}
	return template
}

// A property whose range names a class the model never declares is the finding
// that must stop a publish: the projection silently flips every edge of that
// property, so the loss is invisible in the result.
func TestPublishCheckBlocksADanglingRange(t *testing.T) {
	db := setupGovernanceDB(t)
	template := saveGovernanceTemplate(t, db, "tpl-dangling", ontologyTemplateConfig("country"))
	svc := NewCompilationTemplateGovernanceService()

	check, err := svc.PublishCheck(context.Background(), db, governanceTenant, template.ID)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if check.Publishable {
		t.Fatalf("check = %#v, want a dangling range to block the publish", check)
	}
	if len(check.Blockers) == 0 {
		t.Fatal("a dangling range has to be reported as a blocker, not a warning")
	}
	if check.Blockers[0].Message == "" {
		t.Fatal("the blocker must carry the reason, not just a code")
	}
	if check.NextVersion != 1 {
		t.Fatalf("next version = %d, want the first publish of an unpublished model", check.NextVersion)
	}
}

// Submitting a blocked model would spend a reviewer's attention on something the
// author can already see is wrong.
func TestSubmitForReviewRefusesABlockedModel(t *testing.T) {
	db := setupGovernanceDB(t)
	template := saveGovernanceTemplate(t, db, "tpl-blocked", ontologyTemplateConfig("country"))
	svc := NewCompilationTemplateGovernanceService()

	check, err := svc.SubmitForReview(context.Background(), db, governanceTenant, template.ID, "user-1")
	if err == nil {
		t.Fatal("expected a blocked model to be refused for review")
	}
	if check == nil || check.Publishable {
		t.Fatalf("check = %#v, want the findings returned with the refusal", check)
	}
	stored, err := NewCompilationTemplateGovernanceService().PublishCheck(context.Background(), db, governanceTenant, template.ID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	_ = stored
	status := readPublishStatus(t, db, template.ID)
	if status != entity.PublishStatusDraft {
		t.Fatalf("publish_status = %q, want the model left as a draft", status)
	}
}

// The load-bearing property of the version table: publishing copies the config,
// and a later edit to the working draft does not reach the published version.
// Without it, "published" would only mean "the last thing saved".
func TestPublishVersionSnapshotsAndLeavesLaterEditsOut(t *testing.T) {
	db := setupGovernanceDB(t)
	template := saveGovernanceTemplate(t, db, "tpl-snapshot", ontologyTemplateConfig("place"))
	svc := NewCompilationTemplateGovernanceService()
	ctx := context.Background()

	check, err := svc.PublishVersion(ctx, db, governanceTenant, template.ID, "user-1", "first release", false)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if check.CurrentVersion != 1 {
		t.Fatalf("current version = %d, want 1 after the first publish", check.CurrentVersion)
	}
	if status := readPublishStatus(t, db, template.ID); status != entity.PublishStatusPublished {
		t.Fatalf("publish_status = %q, want published", status)
	}
	if version := readVersion(t, db, template.ID); version != 1 {
		t.Fatalf("version = %d, want the template moved onto the snapshot", version)
	}

	// Edit the working draft after publishing. The group save path also moves a
	// published model back to draft, which is the state an edit leaves it in —
	// the config write is done directly here, so the transition is applied with
	// it.
	if err := db.Model(&entity.CompilationTemplate{}).Where("id = ?", template.ID).
		Updates(map[string]interface{}{
			"config":         entity.JSONMap(ontologyTemplateConfig("person")),
			"publish_status": entity.PublishStatusDraft,
		}).Error; err != nil {
		t.Fatalf("edit working config: %v", err)
	}
	snapshot, err := svc.versionDAO.GetByVersion(ctx, db, governanceTenant, template.ID, 1)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	relations, _ := snapshot.Config["relation"].(map[string]interface{})
	fields, _ := relations["fields"].([]interface{})
	if len(fields) != 1 {
		t.Fatalf("snapshot relations = %#v, want the one published property", relations)
	}
	property, _ := fields[0].(map[string]interface{})
	if property["range"] != "place" {
		t.Fatalf("snapshot range = %v, want the value as published (place)", property["range"])
	}
	if note := snapshot.Note; note == nil || *note != "first release" {
		t.Fatalf("snapshot note = %v, want the note the publisher entered", note)
	}

	// Publishing again creates a second version rather than rewriting the first.
	// The edit above points the property at its own domain class, which the
	// declaration checks warn about, so this publish has to be confirmed —
	// exactly the confirmation an operator would have to give in the UI.
	secondCheck, err := svc.PublishCheck(ctx, db, governanceTenant, template.ID)
	if err != nil {
		t.Fatalf("second check: %v", err)
	}
	if len(secondCheck.Warnings) == 0 {
		t.Fatalf("second check = %#v, want the self-referential property to warn", secondCheck)
	}
	second, err := svc.PublishVersion(ctx, db, governanceTenant, template.ID, "user-1", "second release", true)
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	if second.CurrentVersion != 2 {
		t.Fatalf("current version = %d, want 2", second.CurrentVersion)
	}
	firstSnapshot, err := svc.versionDAO.GetByVersion(ctx, db, governanceTenant, template.ID, 1)
	if err != nil {
		t.Fatalf("reload version 1: %v", err)
	}
	if head := firstSnapshot.Config["relation"].(map[string]interface{})["fields"].([]interface{})[0].(map[string]interface{}); head["range"] != "place" {
		t.Fatalf("version 1 range = %v, want it untouched by the second publish", head["range"])
	}
}

// A warning is a judgement the publisher is allowed to overrule, so it needs the
// explicit confirmation; without it the warning would be decorative.
func TestPublishVersionRequiresForceForWarnings(t *testing.T) {
	db := setupGovernanceDB(t)
	template := saveGovernanceTemplate(t, db, "tpl-warning", propertylessOntologyConfig())
	svc := NewCompilationTemplateGovernanceService()
	ctx := context.Background()

	check, err := svc.PublishCheck(ctx, db, governanceTenant, template.ID)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(check.Warnings) == 0 {
		t.Fatalf("check = %#v, want a property-less class to warn", check)
	}
	if !check.Publishable {
		t.Fatalf("check = %#v, want warnings to leave the model publishable", check)
	}
	if _, err := svc.PublishVersion(ctx, db, governanceTenant, template.ID, "user-1", "", false); err == nil {
		t.Fatal("expected a publish with unconfirmed warnings to be refused")
	}
	if version := readVersion(t, db, template.ID); version != 0 {
		t.Fatalf("version = %d, want the refusal to have published nothing", version)
	}
	if _, err := svc.PublishVersion(ctx, db, governanceTenant, template.ID, "user-1", "", true); err != nil {
		t.Fatalf("forced publish: %v", err)
	}
	if version := readVersion(t, db, template.ID); version != 1 {
		t.Fatalf("version = %d, want the forced publish to have gone through", version)
	}
}

// The diff has to name the declaration that changed. An index-keyed diff would
// report "every later index changed" the moment a class is inserted, which is
// the one case a publisher most needs to read.
func TestVersionDiffNamesChangedDeclarations(t *testing.T) {
	db := setupGovernanceDB(t)
	template := saveGovernanceTemplate(t, db, "tpl-diff", ontologyTemplateConfig("place"))
	svc := NewCompilationTemplateGovernanceService()
	ctx := context.Background()

	if _, err := svc.PublishVersion(ctx, db, governanceTenant, template.ID, "user-1", "v1", false); err != nil {
		t.Fatalf("publish v1: %v", err)
	}
	edited := ontologyTemplateConfig("place")
	entityFields := edited["entity"].(map[string]interface{})["fields"].([]interface{})
	// Give `place` a parent, and insert a new class in front of both existing
	// ones: the diff must report two named changes, not a renumbering.
	entityFields[1].(map[string]interface{})["parent"] = "person"
	edited["entity"].(map[string]interface{})["fields"] = append(
		[]interface{}{map[string]interface{}{"type": "agent", "label": "Agent", "parent": ""}},
		entityFields...,
	)
	if err := db.Model(&entity.CompilationTemplate{}).Where("id = ?", template.ID).
		Update("config", entity.JSONMap(edited)).Error; err != nil {
		t.Fatalf("edit working config: %v", err)
	}

	diff, err := svc.VersionDiff(ctx, db, governanceTenant, template.ID, 1, 0)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	byPath := make(map[string]ModelVersionChange, len(diff.Changes))
	for _, change := range diff.Changes {
		byPath[change.Path] = change
	}
	added, ok := byPath["entity.fields[agent]"]
	if !ok || added.Change != "added" {
		t.Fatalf("changes = %#v, want the inserted class reported as entity.fields[agent] added", diff.Changes)
	}
	modified, ok := byPath["entity.fields[place].parent"]
	if !ok {
		t.Fatalf("changes = %#v, want entity.fields[place].parent", diff.Changes)
	}
	if modified.Before != "" || modified.After != "person" {
		t.Fatalf("parent change = %#v, want an empty before and person after", modified)
	}
	if _, renumbered := byPath["entity.fields[0].type"]; renumbered {
		t.Fatalf("changes = %#v, want named paths instead of index shifts", diff.Changes)
	}
}

// The library has to answer "what breaks if I change this" without being asked,
// and it has to tell "published, then edited" apart from "never published".
func TestListModelsReportsReferencesAndUnpublishedChanges(t *testing.T) {
	db := setupGovernanceDB(t)
	groupID := "group-1"
	template := saveGovernanceTemplate(t, db, "tpl-library", ontologyTemplateConfig("place"))
	if err := db.Model(&entity.CompilationTemplate{}).Where("id = ?", template.ID).
		Update("group_id", groupID).Error; err != nil {
		t.Fatalf("attach group: %v", err)
	}
	// The group id is nested the way the dataset form writes it, to prove the
	// count walks the config rather than only matching a top-level string.
	saveGovernanceDataset(t, db, "kb-1", "Dataset", entity.JSONMap{
		"Parser:Compiler": map[string]interface{}{
			"compilation_template_group_id": groupID,
		},
	})
	svc := NewCompilationTemplateGovernanceService()
	ctx := context.Background()

	if _, err := svc.PublishVersion(ctx, db, governanceTenant, template.ID, "user-1", "v1", false); err != nil {
		t.Fatalf("publish: %v", err)
	}
	models, err := svc.ListModels(ctx, db, governanceTenant, "ontology")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %#v, want the one ontology model", models)
	}
	model := models[0]
	if model.DatasetCount != 1 {
		t.Fatalf("dataset count = %d, want the one dataset referencing the group", model.DatasetCount)
	}
	if model.GroupID != groupID {
		t.Fatalf("group id = %q, want %q", model.GroupID, groupID)
	}
	if model.UnpublishedChanges {
		t.Fatalf("model = %#v, want a just-published model to report no unpublished changes", model)
	}

	// A config edit that bypasses the lifecycle leaves the status claiming
	// "published"; the library has to notice anyway, because it is what tells a
	// reader whether the version on screen is the one the corpus ran on.
	if err := db.Model(&entity.CompilationTemplate{}).Where("id = ?", template.ID).
		Update("config", entity.JSONMap(ontologyTemplateConfig("person"))).Error; err != nil {
		t.Fatalf("edit config: %v", err)
	}
	edited, err := svc.ListModels(ctx, db, governanceTenant, "ontology")
	if err != nil {
		t.Fatalf("list after edit: %v", err)
	}
	if !edited[0].UnpublishedChanges {
		t.Fatalf("model = %#v, want the edited config to be reported as unpublished", edited[0])
	}
	// Only ontology models are in this listing.
	if _, err := svc.ListModels(ctx, db, governanceTenant, "wiki"); err != nil {
		t.Fatalf("list wiki: %v", err)
	}
}

// The library is the *model* library, so an unfiltered list has to mean "the
// models": an ontology template is the only kind that declares the domain the
// compiled assertions are held to, and the only one whose published version and
// affected datasets mean anything. Listing the other kinds files artifacts and
// wiki pages as models, where those columns have no answer.
func TestListModelsWithoutKindListsOntologyTemplatesOnly(t *testing.T) {
	db := setupGovernanceDB(t)
	ctx := context.Background()
	ontology := saveGovernanceTemplate(t, db, "tpl-ontology", ontologyTemplateConfig("place"))

	tenantID := governanceTenant
	valid := string(entity.StatusValid)
	wiki := &entity.CompilationTemplate{
		ID:            "tpl-wiki",
		TenantID:      &tenantID,
		Name:          "Wiki",
		Kind:          "wiki",
		Config:        entity.JSONMap{"kind": "wiki", "entity": map[string]interface{}{}},
		Status:        &valid,
		PublishStatus: entity.PublishStatusDraft,
	}
	if err := db.Create(wiki).Error; err != nil {
		t.Fatalf("create wiki template: %v", err)
	}

	models, err := NewCompilationTemplateGovernanceService().ListModels(ctx, db, governanceTenant, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(models) != 1 || models[0].ID != ontology.ID {
		t.Fatalf("models = %#v, want only the ontology template", models)
	}
}

// A model with nothing wrong still has to answer with lists. A nil slice
// marshals to JSON null, and the dialog that reports "nothing stands in the way"
// asks those lists for their length first — so the clean case is the one that
// breaks the reader unless the empty case is spelled as an empty list.
func TestPublishCheckSerializesEmptyFindingsAsLists(t *testing.T) {
	db := setupGovernanceDB(t)
	template := saveGovernanceTemplate(t, db, "tpl-clean", ontologyTemplateConfig("place"))

	check, err := NewCompilationTemplateGovernanceService().PublishCheck(
		context.Background(), db, governanceTenant, template.ID)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(check.Blockers) != 0 || len(check.Warnings) != 0 {
		t.Fatalf("check = %#v, want a model with no findings", check)
	}

	payload, err := json.Marshal(check)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, field := range []string{`"blockers":[]`, `"warnings":[]`} {
		if !strings.Contains(string(payload), field) {
			t.Fatalf("payload = %s, want %s", payload, field)
		}
	}
}

// A dataset may name a template directly instead of a group. Counting groups
// alone would report zero affected datasets for such a model — the silent zero
// on the one number that tells a publisher whether a change reaches anything.
func TestListModelsCountsADatasetNamingTheTemplateDirectly(t *testing.T) {
	db := setupGovernanceDB(t)
	ctx := context.Background()
	template := saveGovernanceTemplate(t, db, "tpl-direct", ontologyTemplateConfig("place"))
	saveGovernanceDataset(t, db, "kb-direct", "Direct", entity.JSONMap{
		"Parser:Compiler": map[string]interface{}{"compilation_template_id": template.ID},
	})
	svc := NewCompilationTemplateGovernanceService()

	models, err := svc.ListModels(ctx, db, governanceTenant, "ontology")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("models = %#v, want the one ontology model", models)
	}
	if models[0].DatasetCount != 1 {
		t.Fatalf("dataset count = %d, want the dataset naming the template by id", models[0].DatasetCount)
	}
	impact, err := svc.Impact(ctx, db, governanceTenant, template.ID)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if impact.Ref.Kind != common.ModelRefTemplate || impact.Ref.ID != template.ID {
		t.Fatalf("ref = %#v, want the template itself", impact.Ref)
	}
}

// The count is a prompt and the drill-down is the decision: a publisher told
// "one dataset is affected" has to know which one. Both answers come from the
// references the write path derives, so a config rewrite has to move them.
func TestImpactListsAffectedDatasetsAndFollowsConfigEdits(t *testing.T) {
	db := setupGovernanceDB(t)
	ctx := context.Background()
	const groupID = "group-impact"
	template := saveGovernanceTemplate(t, db, "tpl-impact", ontologyTemplateConfig("place"))
	if err := db.Model(&entity.CompilationTemplate{}).Where("id = ?", template.ID).
		Update("group_id", groupID).Error; err != nil {
		t.Fatalf("attach group: %v", err)
	}
	kb := saveGovernanceDataset(t, db, "kb-a", "Alpha", entity.JSONMap{
		"Parser:Compiler": map[string]interface{}{"compilation_template_group_id": groupID},
	})
	// A dataset compiling against another group is out of scope: the impact has
	// to answer for this model, not for the model library.
	saveGovernanceDataset(t, db, "kb-b", "Beta", entity.JSONMap{
		"Parser:Compiler": map[string]interface{}{"compilation_template_group_id": "group-other"},
	})

	svc := NewCompilationTemplateGovernanceService()
	impact, err := svc.Impact(ctx, db, governanceTenant, template.ID)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if impact.Ref.Kind != common.ModelRefGroup || impact.Ref.ID != groupID {
		t.Fatalf("ref = %#v, want the group the model belongs to", impact.Ref)
	}
	if len(impact.Datasets) != 1 || impact.Datasets[0].ID != "kb-a" || impact.Datasets[0].Name != "Alpha" {
		t.Fatalf("datasets = %#v, want only the dataset compiling against this group", impact.Datasets)
	}

	// Rewriting the config without the group drops the dataset from the impact,
	// which only holds because the write path re-derives the references.
	if err := dao.NewKnowledgebaseDAO().UpdateByID(ctx, db, kb.ID, map[string]interface{}{
		"parser_config": entity.JSONMap{"chunk_token_num": 512},
	}); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if refs := templateRefsOf(t, db, kb.ID); refs != 0 {
		t.Fatalf("refs = %d, want the references of a config that names no model", refs)
	}

	// A soft-deleted dataset keeps its rows — a restore must not silently lose
	// the references — and stops being counted.
	if err := dao.NewKnowledgebaseDAO().UpdateByID(ctx, db, kb.ID, map[string]interface{}{
		"parser_config": entity.JSONMap{"compilation_template_group_id": groupID},
	}); err != nil {
		t.Fatalf("re-attach group: %v", err)
	}
	if err := dao.NewKnowledgebaseDAO().Delete(ctx, db, kb.ID); err != nil {
		t.Fatalf("delete dataset: %v", err)
	}
	impact, err = svc.Impact(ctx, db, governanceTenant, template.ID)
	if err != nil {
		t.Fatalf("impact after delete: %v", err)
	}
	if len(impact.Datasets) != 0 {
		t.Fatalf("datasets = %#v, want a deleted dataset to stop being affected", impact.Datasets)
	}
	if refs := templateRefsOf(t, db, kb.ID); refs != 1 {
		t.Fatalf("refs = %d, want the row kept for a restore", refs)
	}
	models, err := svc.ListModels(ctx, db, governanceTenant, "ontology")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if models[0].DatasetCount != 0 {
		t.Fatalf("dataset count = %d, want a deleted dataset not to be counted", models[0].DatasetCount)
	}
}

func readPublishStatus(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var stored entity.CompilationTemplate
	if err := db.Where("id = ?", id).Take(&stored).Error; err != nil {
		t.Fatalf("read template: %v", err)
	}
	return stored.PublishStatus
}

func readVersion(t *testing.T, db *gorm.DB, id string) int {
	t.Helper()
	var stored entity.CompilationTemplate
	if err := db.Where("id = ?", id).Take(&stored).Error; err != nil {
		t.Fatalf("read template: %v", err)
	}
	return stored.Version
}
