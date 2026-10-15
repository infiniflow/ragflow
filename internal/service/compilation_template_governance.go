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
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// The two severities a pre-publish finding can carry.
//
// The split is the whole mechanism of the gate: a warning is a judgement the
// publisher is allowed to overrule, a blocker is not. A single flat list would
// force every check to be either advisory or absolute, and the checks are not
// alike — a class no property reaches costs coverage (the publisher may know
// better), while a property whose range names an undeclared class kills every
// edge of that property silently (nobody may wave that through).
const (
	ModelIssueBlocker = "blocker"
	ModelIssueWarning = "warning"
)

// ModelPublishIssue is one pre-publish finding.
type ModelPublishIssue struct {
	Code     string   `json:"code"`
	Category string   `json:"category,omitempty"`
	Severity string   `json:"severity"`
	Message  string   `json:"message"`
	Subjects []string `json:"subjects,omitempty"`
}

// ModelPublishCheck is the report the publish gate decides on, and it is
// returned on every path — including the refusals — so the UI can render the
// same dialog whether the model was published or blocked.
type ModelPublishCheck struct {
	TemplateID string `json:"template_id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	// CurrentVersion is what compilation reads right now; 0 when the template has
	// never been published and compilation is reading the working config.
	CurrentVersion int `json:"current_version"`
	// NextVersion is the version this publish would create.
	NextVersion int                 `json:"next_version"`
	Blockers    []ModelPublishIssue `json:"blockers"`
	Warnings    []ModelPublishIssue `json:"warnings"`
	// Publishable is false when at least one blocker stands, which no force flag
	// can override.
	Publishable bool `json:"publishable"`
}

// ModelSummary is one row of the model library.
type ModelSummary struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	GroupID       string `json:"group_id,omitempty"`
	Description   string `json:"description,omitempty"`
	Version       int    `json:"version"`
	PublishStatus string `json:"publish_status"`
	PublishedAt   string `json:"published_at,omitempty"`
	PublishedBy   string `json:"published_by,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	ReviewNote    string `json:"review_note,omitempty"`
	// UnpublishedChanges distinguishes "published, then edited" from "never
	// published". The first has a version the corpus was compiled against that no
	// longer matches the model on screen; the second has no version at all.
	UnpublishedChanges bool `json:"unpublished_changes"`
	// DatasetCount is how many datasets compile against this model. It is the
	// answer to "what breaks if I change this", so the library shows it without
	// being asked.
	DatasetCount int64 `json:"dataset_count"`
}

// ModelImpact is the answer to "what does changing this model reach": the
// datasets that compile against it.
//
// It is separate from the count on the library listing because the count is a
// prompt and this is the decision: a publisher told "3 datasets are affected"
// has to know which three to tell whether the change can be made now or has to
// be scheduled.
type ModelImpact struct {
	TemplateID string `json:"template_id"`
	// Ref is the reference datasets store for this model — its group, or the
	// template itself when it is in no group.
	Ref common.ModelRef `json:"ref"`
	// Refs is every reference that reaches the model, which is what Datasets was
	// resolved from. Reported so a reader who sees no datasets can tell whether
	// the dataset names a different reference than the one that was looked up.
	Refs     []common.ModelRef         `json:"refs"`
	Datasets []*dao.ModelImpactDataset `json:"datasets"`
}

// ModelVersionSummary is one row of a model's version history.
type ModelVersionSummary struct {
	Version     int    `json:"version"`
	Name        string `json:"name"`
	Note        string `json:"note,omitempty"`
	PublishedBy string `json:"published_by,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
	IsCurrent   bool   `json:"is_current"`
}

// ModelVersionChange is one leaf difference between two versions' configs.
type ModelVersionChange struct {
	// Path is the JSON path of the leaf. Declarations are addressed by name
	// (`entity.fields[place].parent`) rather than by index, because the editor
	// lets a class be inserted anywhere and an index-keyed diff would report
	// every later declaration as rewritten.
	Path   string `json:"path"`
	Change string `json:"change"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// ModelVersionDiff is the difference between two published versions.
type ModelVersionDiff struct {
	TemplateID string               `json:"template_id"`
	From       int                  `json:"from"`
	To         int                  `json:"to"`
	Changes    []ModelVersionChange `json:"changes"`
}

// CompilationTemplateGovernanceService owns the model lifecycle: what a model
// says, which version of it is in force, and who moved it there.
//
// The working config on the template row is the draft. A published version is an
// immutable snapshot in compilation_template_version, and compilation reads the
// snapshot — so this service is the only place that decides when an edit starts
// affecting the corpus. Without it the answer was "the moment it was saved".
type CompilationTemplateGovernanceService struct {
	templateDAO *dao.CompilationTemplateDAO
	versionDAO  *dao.CompilationTemplateVersionDAO
}

// NewCompilationTemplateGovernanceService creates the governance service.
func NewCompilationTemplateGovernanceService() *CompilationTemplateGovernanceService {
	return &CompilationTemplateGovernanceService{
		templateDAO: dao.NewCompilationTemplateDAO(),
		versionDAO:  dao.NewCompilationTemplateVersionDAO(),
	}
}

// ListModels returns the tenant's models with the state each one is in.
func (s *CompilationTemplateGovernanceService) ListModels(ctx context.Context, db *gorm.DB, tenantID, kind string) ([]*ModelSummary, error) {
	// A model is an ontology template: it is the only kind whose config declares
	// the domain the compiled assertions are held to, and therefore the only one
	// the library's columns — published version, publishing actor, affected
	// datasets — describe anything about. Listing every compilation template
	// here would file artifacts, trees and wiki pages as models, where those
	// questions have no answer.
	if strings.TrimSpace(kind) == "" {
		kind = kccommon.KindOntology
	}
	templates, err := s.templateDAO.ListByTenant(ctx, db, tenantID, kind)
	if err != nil {
		return nil, err
	}
	refDAO := dao.NewDatasetModelRefDAO()
	counts, err := refDAO.CountByModel(ctx, db, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]*ModelSummary, 0, len(templates))
	for _, template := range templates {
		summary := &ModelSummary{
			ID:            template.ID,
			Name:          template.Name,
			Kind:          template.Kind,
			Description:   stringOrEmpty(template.Description),
			Version:       template.Version,
			PublishStatus: normalizePublishStatus(template),
			PublishedAt:   timeStr(template.PublishedAt),
			PublishedBy:   stringOrEmpty(template.PublishedBy),
			UpdatedAt:     timeStr(template.UpdateTime),
			ReviewNote:    stringOrEmpty(template.ReviewNote),
		}
		if template.GroupID != nil {
			summary.GroupID = *template.GroupID
		}
		// UnpublishedChanges is computed from the published snapshot rather than
		// read off the status column: a writer that edits the config without
		// going through the group save path would otherwise leave the library
		// claiming the corpus runs on the version it is looking at.
		if template.Version > 0 {
			snapshot, err := s.versionDAO.GetByVersion(ctx, db, tenantID, template.ID, template.Version)
			if err != nil {
				return nil, err
			}
			summary.UnpublishedChanges = summary.PublishStatus != entity.PublishStatusPublished ||
				!reflectEqual(map[string]interface{}(template.Config), map[string]interface{}(snapshot.Config))
		}
		// A model in a group is reachable by two references (see modelRefsFor),
		// so the pre-aggregated per-reference count only answers for a model
		// with one. The grouped case asks for a distinct count instead of adding
		// the two, which would count a dataset that names both twice.
		refs := modelRefsFor(template)
		if len(refs) == 1 {
			summary.DatasetCount = counts[refs[0]]
		} else {
			count, err := refDAO.CountByModelRefs(ctx, db, tenantID, refs)
			if err != nil {
				return nil, err
			}
			summary.DatasetCount = count
		}
		out = append(out, summary)
	}
	return out, nil
}

// Impact returns the datasets that compile against one model.
func (s *CompilationTemplateGovernanceService) Impact(ctx context.Context, db *gorm.DB, tenantID, templateID string) (*ModelImpact, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	refs := modelRefsFor(template)
	datasets, err := dao.NewDatasetModelRefDAO().DatasetsByModelRefs(ctx, db, tenantID, refs)
	if err != nil {
		return nil, err
	}
	return &ModelImpact{
		TemplateID: template.ID,
		Ref:        modelRefOf(template),
		Refs:       refs,
		Datasets:   datasets,
	}, nil
}

// PublishCheck reports what stands between the current draft and a publish.
func (s *CompilationTemplateGovernanceService) PublishCheck(ctx context.Context, db *gorm.DB, tenantID, templateID string) (*ModelPublishCheck, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	return modelPublishCheck(template), nil
}

// SubmitForReview moves a draft to review, refusing while a blocker stands.
//
// Refusing here rather than at publish is not redundant: it stops a model that
// cannot compile correctly from occupying a reviewer's attention, and it puts
// the finding in front of the person who can still change the model.
func (s *CompilationTemplateGovernanceService) SubmitForReview(ctx context.Context, db *gorm.DB, tenantID, templateID, actor string) (*ModelPublishCheck, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	check := modelPublishCheck(template)
	switch normalizePublishStatus(template) {
	case entity.PublishStatusDraft:
	case entity.PublishStatusReview:
		// Idempotent: a second submit from the same model is not an error the
		// caller should have to handle.
		return check, nil
	default:
		return check, fmt.Errorf("model %s is %s; only a draft can be submitted for review", templateID, template.PublishStatus)
	}
	if !check.Publishable {
		return check, fmt.Errorf("model %s cannot be submitted: %s", templateID, blockerSummary(check))
	}
	if err := s.templateDAO.UpdateFields(ctx, db, templateID, map[string]interface{}{
		"publish_status": entity.PublishStatusReview,
		"review_note":    nil,
		"reviewed_by":    actor,
		"reviewed_at":    time.Now().UnixMilli(),
	}); err != nil {
		return nil, err
	}
	return check, nil
}

// PublishVersion records the current draft as a new immutable version and moves
// compilation onto it.
//
// force overrides warnings only. A blocker is not a judgement call this service
// is willing to delegate: those are the findings whose effect on the extracted
// knowledge is invisible in the result, so "publish anyway" would mean shipping
// a model whose failures nobody can see.
func (s *CompilationTemplateGovernanceService) PublishVersion(ctx context.Context, db *gorm.DB, tenantID, templateID, actor, note string, force bool) (*ModelPublishCheck, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	check := modelPublishCheck(template)
	if !check.Publishable {
		return check, fmt.Errorf("model %s cannot be published: %s", templateID, blockerSummary(check))
	}
	if len(check.Warnings) > 0 && !force {
		return check, fmt.Errorf("model %s has %d warning(s); confirm to publish anyway", templateID, len(check.Warnings))
	}
	status := normalizePublishStatus(template)
	if status != entity.PublishStatusDraft && status != entity.PublishStatusReview {
		return check, fmt.Errorf("model %s is %s; only a draft or a model under review can be published", templateID, status)
	}

	now := time.Now().UnixMilli()
	publishedBy := actor
	snapshot := &entity.CompilationTemplateVersion{
		ID:          common.GenerateUUID(),
		TemplateID:  template.ID,
		TenantID:    tenantID,
		Version:     check.NextVersion,
		Name:        template.Name,
		Kind:        template.Kind,
		Config:      template.Config,
		PublishedBy: &publishedBy,
		PublishedAt: &now,
	}
	if trimmed := strings.TrimSpace(note); trimmed != "" {
		snapshot.Note = &trimmed
	}
	if err := s.templateDAO.PublishSnapshot(ctx, db, template.ID, tenantID, template.Version, snapshot); err != nil {
		return check, err
	}
	check.CurrentVersion = snapshot.Version
	check.NextVersion = snapshot.Version + 1
	return check, nil
}

// RejectReview sends a model under review back to draft, recording why.
//
// The note is kept on the template rather than only in a log because the next
// person to open the model is the one who needs it, and they are not reading
// logs.
func (s *CompilationTemplateGovernanceService) RejectReview(ctx context.Context, db *gorm.DB, tenantID, templateID, actor, reason string) error {
	template, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return err
	}
	if normalizePublishStatus(template) != entity.PublishStatusReview {
		return fmt.Errorf("model %s is not under review", templateID)
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("a rejection has to say why")
	}
	return s.templateDAO.UpdateFields(ctx, db, templateID, map[string]interface{}{
		"publish_status": entity.PublishStatusDraft,
		"review_note":    strings.TrimSpace(reason),
		"reviewed_by":    actor,
		"reviewed_at":    time.Now().UnixMilli(),
	})
}

// RetireModel withdraws a published model from use.
//
// The snapshots are kept and stay readable: assertions already extracted under
// them have to remain explainable, and "which model version said this" is part
// of explaining them.
func (s *CompilationTemplateGovernanceService) RetireModel(ctx context.Context, db *gorm.DB, tenantID, templateID, actor string) error {
	template, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return err
	}
	if normalizePublishStatus(template) != entity.PublishStatusPublished {
		return fmt.Errorf("model %s is not published", templateID)
	}
	return s.templateDAO.UpdateFields(ctx, db, templateID, map[string]interface{}{
		"publish_status": entity.PublishStatusRetired,
		"reviewed_by":    actor,
		"reviewed_at":    time.Now().UnixMilli(),
	})
}

// ListVersions returns a model's published versions, newest first.
func (s *CompilationTemplateGovernanceService) ListVersions(ctx context.Context, db *gorm.DB, tenantID, templateID string) ([]*ModelVersionSummary, error) {
	template, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	versions, err := s.versionDAO.ListByTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	out := make([]*ModelVersionSummary, 0, len(versions))
	for _, version := range versions {
		out = append(out, &ModelVersionSummary{
			Version:     version.Version,
			Name:        version.Name,
			Note:        stringOrEmpty(version.Note),
			PublishedBy: stringOrEmpty(version.PublishedBy),
			PublishedAt: timeStr(version.PublishedAt),
			IsCurrent:   version.Version == template.Version,
		})
	}
	return out, nil
}

// VersionDiff reports the difference between two published versions, and
// accepts 0 as "the working draft" so the UI can ask "what would publishing
// change" with the same call as "what changed last time".
func (s *CompilationTemplateGovernanceService) VersionDiff(ctx context.Context, db *gorm.DB, tenantID, templateID string, from, to int) (*ModelVersionDiff, error) {
	fromConfig, err := s.configAtVersion(ctx, db, tenantID, templateID, from)
	if err != nil {
		return nil, err
	}
	toConfig, err := s.configAtVersion(ctx, db, tenantID, templateID, to)
	if err != nil {
		return nil, err
	}
	return &ModelVersionDiff{
		TemplateID: templateID,
		From:       from,
		To:         to,
		Changes:    diffConfigs(fromConfig, toConfig),
	}, nil
}

// configAtVersion returns the config a version number refers to; version 0 is
// the working draft, which is what the template row carries.
func (s *CompilationTemplateGovernanceService) configAtVersion(ctx context.Context, db *gorm.DB, tenantID, templateID string, version int) (map[string]interface{}, error) {
	if version <= 0 {
		template, err := s.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}(template.Config), nil
	}
	snapshot, err := s.versionDAO.GetByVersion(ctx, db, tenantID, templateID, version)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}(snapshot.Config), nil
}

// modelPublishCheck runs every finding the gate knows about. The declaration
// checks are the same ones the artifacts view runs, minus the instance-dependent
// ones — a model is checked before the corpus has been compiled against it.
func modelPublishCheck(template *entity.CompilationTemplate) *ModelPublishCheck {
	check := &ModelPublishCheck{
		TemplateID:     template.ID,
		Name:           template.Name,
		Kind:           template.Kind,
		CurrentVersion: template.Version,
		NextVersion:    template.Version + 1,
		// Empty findings are still a list. A nil slice marshals to JSON null,
		// and every reader of this payload asks the findings for their length
		// before deciding whether there is anything to show — so a clean model
		// would break the dialog that reports it is clean.
		Blockers: make([]ModelPublishIssue, 0),
		Warnings: make([]ModelPublishIssue, 0),
	}
	issues := make([]ModelPublishIssue, 0)
	config := map[string]interface{}(template.Config)
	if len(config) == 0 {
		issues = append(issues, ModelPublishIssue{
			Code:     "empty_config",
			Severity: ModelIssueBlocker,
			Message:  "the model declares nothing, so publishing it would compile the corpus against an empty contract",
		})
	}
	if kccommon.IsOntologyKind(template.Kind) {
		if err := validateOntologyTemplate(config); err != nil {
			// A dangling parent, domain, range or datatype is a blocker: the
			// projection flips edges whose endpoints do not match the
			// declaration, so the property silently extracts nothing.
			issues = append(issues, ModelPublishIssue{
				Code:     "invalid_declaration",
				Severity: ModelIssueBlocker,
				Message:  err.Error(),
			})
		}
		for _, pitfall := range declarationPitfalls(parseDeclaredOntology(config)) {
			severity := ModelIssueWarning
			if pitfall.Severity == "error" {
				severity = ModelIssueBlocker
			}
			issues = append(issues, ModelPublishIssue{
				Code:     pitfall.Code,
				Category: pitfall.Category,
				Severity: severity,
				Message:  pitfall.Message,
				Subjects: pitfall.Subjects,
			})
		}
	}
	for _, issue := range issues {
		if issue.Severity == ModelIssueBlocker {
			check.Blockers = append(check.Blockers, issue)
			continue
		}
		check.Warnings = append(check.Warnings, issue)
	}
	check.Publishable = len(check.Blockers) == 0
	return check
}

// diffConfigs walks two configs and reports every leaf that differs.
//
// It is deliberately structural rather than semantic: a field-level "this class
// gained a parent" would be friendlier, but it also has to guess what the author
// meant, and the diff is read right before publishing — the moment a wrong guess
// is most expensive. Paths and raw values cannot be wrong.
func diffConfigs(before, after map[string]interface{}) []ModelVersionChange {
	changes := make([]ModelVersionChange, 0)
	walkConfigDiff("", before, after, &changes)
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}

func walkConfigDiff(path string, before, after interface{}, out *[]ModelVersionChange) {
	beforeMap, beforeIsMap := before.(map[string]interface{})
	afterMap, afterIsMap := after.(map[string]interface{})
	if beforeIsMap && afterIsMap {
		for _, key := range unionKeys(beforeMap, afterMap) {
			walkConfigDiff(joinConfigPath(path, key), beforeMap[key], afterMap[key], out)
		}
		return
	}
	beforeList, beforeIsList := before.([]interface{})
	afterList, afterIsList := after.([]interface{})
	if beforeIsList && afterIsList {
		walkConfigListDiff(path, beforeList, afterList, out)
		return
	}
	if reflectEqual(before, after) {
		return
	}
	*out = append(*out, ModelVersionChange{
		Path:   path,
		Change: configChangeKind(before, after),
		Before: renderConfigValue(before),
		After:  renderConfigValue(after),
	})
}

// walkConfigListDiff compares two arrays.
//
// Field lists are ordered but their order carries no meaning — the editor lets a
// class be inserted anywhere — so when every item carries a `type` name the
// items are matched by that name, and an insertion reads as one addition instead
// of as every later index having changed. Arrays whose items are not named (a
// `domain` list, a `range` list) fall back to index order.
func walkConfigListDiff(path string, before, after []interface{}, out *[]ModelVersionChange) {
	beforeItems, beforeNamed := namedConfigItems(before)
	afterItems, afterNamed := namedConfigItems(after)
	if beforeNamed && afterNamed {
		for _, name := range unionKeys(beforeItems, afterItems) {
			walkConfigDiff(path+"["+name+"]", beforeItems[name], afterItems[name], out)
		}
		return
	}
	length := len(before)
	if len(after) > length {
		length = len(after)
	}
	for i := 0; i < length; i++ {
		var beforeItem, afterItem interface{}
		if i < len(before) {
			beforeItem = before[i]
		}
		if i < len(after) {
			afterItem = after[i]
		}
		walkConfigDiff(fmt.Sprintf("%s[%d]", path, i), beforeItem, afterItem, out)
	}
}

// namedConfigItems indexes a list of objects by their `type` name. It reports
// false when any item is not an object or carries no name, in which case the
// caller cannot match items up and has to fall back to index order.
func namedConfigItems(items []interface{}) (map[string]interface{}, bool) {
	if len(items) == 0 {
		return nil, false
	}
	out := make(map[string]interface{}, len(items))
	for _, item := range items {
		object, ok := item.(map[string]interface{})
		if !ok {
			return nil, false
		}
		name, _ := object["type"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, false
		}
		out[name] = item
	}
	return out, true
}

func unionKeys[V any](before, after map[string]V) []string {
	keys := make([]string, 0, len(before)+len(after))
	for key := range before {
		keys = append(keys, key)
	}
	for key := range after {
		if _, ok := before[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func joinConfigPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func configChangeKind(before, after interface{}) string {
	if before == nil {
		return "added"
	}
	if after == nil {
		return "removed"
	}
	return "modified"
}

func renderConfigValue(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}

// reflectEqual compares two decoded JSON values, which are maps, slices, strings,
// float64 and bool.
func reflectEqual(a, b interface{}) bool {
	aEncoded, aErr := json.Marshal(a)
	bEncoded, bErr := json.Marshal(b)
	if aErr != nil || bErr != nil {
		return false
	}
	return string(aEncoded) == string(bEncoded)
}

// normalizePublishStatus reads the lifecycle state, treating the zero value as a
// draft: every template that predates the lifecycle has an empty status and is
// exactly that.
func normalizePublishStatus(template *entity.CompilationTemplate) string {
	status := strings.TrimSpace(template.PublishStatus)
	if status == "" {
		return entity.PublishStatusDraft
	}
	return status
}

func blockerSummary(check *ModelPublishCheck) string {
	messages := make([]string, 0, len(check.Blockers))
	for _, blocker := range check.Blockers {
		messages = append(messages, blocker.Message)
	}
	return strings.Join(messages, "; ")
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// modelRefOf returns the reference datasets store for this model: the group it
// belongs to when it is in one, its own id otherwise.
//
// The compiler accepts either spelling (ParseParam), so a model outside a group
// is still reachable by naming it directly. Counting groups alone would report
// zero datasets for every ungrouped model — a silent zero on the one number
// that tells a publisher whether a change reaches anything at all.
func modelRefOf(template *entity.CompilationTemplate) common.ModelRef {
	if template.GroupID != nil {
		if groupID := strings.TrimSpace(*template.GroupID); groupID != "" {
			return common.ModelRef{Kind: common.ModelRefGroup, ID: groupID}
		}
	}
	return common.ModelRef{Kind: common.ModelRefTemplate, ID: template.ID}
}

// modelRefsFor lists every reference a dataset could name to reach this model:
// the group it belongs to, then the template itself.
//
// modelRefOf answers "which one reference does this model prefer" — the group,
// because that is the cheaper thing for a model library to resolve. Reachability
// is not the same question: a dataset that names the template directly reaches
// the model just as much as one that names its group, and resolving only the
// preferred reference makes a model that is in use look unused.
func modelRefsFor(template *entity.CompilationTemplate) []common.ModelRef {
	refs := make([]common.ModelRef, 0, 2)
	if template.GroupID != nil {
		if groupID := strings.TrimSpace(*template.GroupID); groupID != "" {
			refs = append(refs, common.ModelRef{Kind: common.ModelRefGroup, ID: groupID})
		}
	}
	return append(refs, common.ModelRef{Kind: common.ModelRefTemplate, ID: template.ID})
}
