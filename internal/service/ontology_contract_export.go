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

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
)

// ModelContractExport is the read-only product of a model version.
//
// Three outputs come from one read because they are three renderings of the same
// version, not three endpoints a reader has to know about: the prose is what a
// business reviewer reads, the schema is what another system loads, and the row
// listing is what a pipeline ingests. They must never disagree about which
// version they describe, so they are produced together.
type ModelContractExport struct {
	TemplateID string `json:"template_id"`
	Name       string `json:"name"`
	// Version is the version the export describes. Zero means the working draft,
	// which only ever happens when a model has never been published.
	Version       int    `json:"version"`
	PublishStatus string `json:"publish_status"`
	PublishedAt   string `json:"published_at,omitempty"`
	PublishedBy   string `json:"published_by,omitempty"`
	// Markdown is the human-readable rendering.
	Markdown string `json:"markdown"`
	// Machine is the same model as YAML, ready to be committed to a repository.
	Machine string `json:"machine"`
	// Schema is the machine form in the shape a consumer parses.
	Schema map[string]interface{} `json:"schema"`
	// CoverageNote / DriftNote are the two findings the prose page puts above
	// the contract. They are empty when nothing has been measured, rather than
	// claiming the model is complete.
	CoverageNote string `json:"coverage_note,omitempty"`
	DriftNote    string `json:"drift_note,omitempty"`
}

// ExportModelContract renders one model version.
//
// version 0 reads the working draft and is only meaningful for a model that has
// never been published — the design point is that a reader exports what the
// corpus was compiled against, so the published snapshot is the default.
func (s *DatasetArtifactService) ExportModelContract(ctx context.Context, db *gorm.DB, tenantID, templateID string, version int) (*ModelContractExport, error) {
	governance := NewCompilationTemplateGovernanceService()
	template, err := governance.templateDAO.GetTemplate(ctx, db, tenantID, templateID)
	if err != nil {
		return nil, err
	}
	config := map[string]interface{}(template.Config)
	name := template.Name
	version = normalizeExportVersion(version, template.Version)
	if version > 0 {
		snapshot, err := governance.versionDAO.GetByVersion(ctx, db, tenantID, templateID, version)
		if err != nil {
			return nil, err
		}
		config = map[string]interface{}(snapshot.Config)
		name = snapshot.Name
	}

	export := &ModelContractExport{
		TemplateID:    template.ID,
		Name:          name,
		Version:       version,
		PublishStatus: normalizePublishStatus(template),
		PublishedAt:   timeStr(template.PublishedAt),
		PublishedBy:   stringOrEmpty(template.PublishedBy),
	}
	export.Schema = buildContractSchema(config, version)
	machine, err := yaml.Marshal(export.Schema)
	if err != nil {
		return nil, err
	}
	export.Machine = string(machine)
	export.CoverageNote, export.DriftNote = s.exportFindings(ctx, db, tenantID, templateID)
	export.Markdown = renderContractMarkdown(export, config)
	return export, nil
}

// normalizeExportVersion turns "no version asked for" into the current one.
//
// A negative version is not meaningful here and is treated as "current": the
// alternative is failing an export over a query parameter the caller did not
// need to supply.
func normalizeExportVersion(requested, current int) int {
	if requested < 0 {
		return current
	}
	if requested == 0 {
		return current
	}
	return requested
}

// exportFindings pulls the two prose findings from the newest measurement.
//
// Only the newest one is read: the export states "as of this compile", and
// averaging several datasets into one sentence would make the number describe
// nothing a reader can check.
func (s *DatasetArtifactService) exportFindings(ctx context.Context, db *gorm.DB, tenantID, templateID string) (string, string) {
	insights, err := dao.NewOntologyInsightDAO().ListByTemplate(ctx, db, tenantID, templateID)
	if err != nil || len(insights) == 0 {
		return "", ""
	}
	newest := insights[0]
	for _, insight := range insights[1:] {
		if insight.AsOf > newest.AsOf {
			newest = insight
		}
	}
	payload, err := decodeInsightPayload(newest.Payload)
	if err != nil {
		return "", ""
	}
	coverage := ""
	if payload.Required > 0 && payload.Satisfied < payload.Required {
		unanswered := unansweredQuestionIDs(payload.Questions)
		coverage = fmt.Sprintf("%d 条能力问题当前模型无法回答（%s）",
			len(unanswered), strings.Join(unanswered, "、"))
	}
	drift := ""
	if len(payload.Drift.UndeclaredClasses) > 0 {
		parts := make([]string, 0, len(payload.Drift.UndeclaredClasses))
		for _, class := range payload.Drift.UndeclaredClasses {
			parts = append(parts, fmt.Sprintf("%s %d 行", class.Type, class.Entities))
		}
		drift = fmt.Sprintf("语料中存在 %d 个未声明类型（%s）",
			len(payload.Drift.UndeclaredClasses), strings.Join(parts, "、"))
	}
	return coverage, drift
}

// buildContractSchema turns a template config into the machine contract.
//
// It is not a copy of the config: the config is the editor's storage format
// (sections, fields, display strings interleaved), while a consumer wants the
// classes and relations enumerated. Rendering it here also keeps the two exports
// from drifting apart — both come from this one shape.
func buildContractSchema(config map[string]interface{}, version int) map[string]interface{} {
	declared := parseDeclaredOntology(config)
	classes := make([]map[string]interface{}, 0, len(declared.classOrder))
	for _, name := range declared.classOrder {
		class := declared.classes[name]
		entry := map[string]interface{}{
			"type": name,
		}
		if class.label != "" {
			entry["label"] = class.label
		}
		if class.description != "" {
			entry["description"] = class.description
		}
		if len(class.parents) > 0 {
			entry["parent"] = class.parents
		}
		if len(class.aliases) > 0 {
			entry["aliases"] = class.aliases
		}
		classes = append(classes, entry)
	}

	relations := make([]map[string]interface{}, 0, len(declared.propOrder))
	attributes := make([]map[string]interface{}, 0)
	for _, name := range declared.propOrder {
		property := declared.props[name]
		if property.isDatatype() {
			entry := map[string]interface{}{
				"type": name,
				"kind": "datatype",
			}
			if property.datatype != "" {
				entry["datatype"] = property.datatype
			}
			if len(property.domains) > 0 {
				entry["domain"] = property.domains
			}
			if property.required {
				entry["required"] = true
			}
			attributes = append(attributes, entry)
			continue
		}
		entry := map[string]interface{}{
			"type": name,
			"kind": "object",
		}
		if len(property.domains) > 0 {
			entry["from"] = property.domains
		}
		if len(property.ranges) > 0 {
			entry["to"] = property.ranges
		}
		if property.label != "" {
			entry["label"] = property.label
		}
		if role := relationViaRole(config, name); role != "" {
			entry["via"] = role
		}
		relations = append(relations, entry)
	}

	return map[string]interface{}{
		"kind":       "ontology",
		"version":    version,
		"classes":    classes,
		"relations":  relations,
		"attributes": attributes,
		"actions":    declaredActions(config),
	}
}

// relationViaRole reads the reified-relation class a relation admits, if any.
//
// A relation with a `via` class is a fact that carries attributes of its own
// (who, in what role, over what interval). The export states it because a
// consumer that flattens the relation to an edge would lose exactly those.
func relationViaRole(config map[string]interface{}, relation string) string {
	for _, raw := range jsonListAt(jsonMapAt(config, "relation"), "fields") {
		field, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSpace(jsonStringAt(field, "type")) != relation {
			continue
		}
		return strings.TrimSpace(jsonStringAt(field, "via"))
	}
	return ""
}

// declaredActions lists the entities that carry an executable contract.
func declaredActions(config map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0)
	for _, raw := range jsonListAt(jsonMapAt(config, "entity"), "fields") {
		field, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSpace(jsonStringAt(field, "contract")) == "" {
			continue
		}
		entry := map[string]interface{}{"type": jsonStringAt(field, "type")}
		if label := jsonStringAt(field, "label"); label != "" {
			entry["label"] = label
		}
		out = append(out, entry)
	}
	return out
}

// renderContractMarkdown writes the page a business reviewer reads.
//
// It puts the two negative findings first on purpose. A contract page that opens
// with "5 classes, 7 relations" reads as complete; the reader has to be told what
// the model cannot do before they are told what it says.
func renderContractMarkdown(export *ModelContractExport, config map[string]interface{}) string {
	declared := parseDeclaredOntology(config)
	var b strings.Builder

	fmt.Fprintf(&b, "# %s", export.Name)
	if export.Version > 0 {
		fmt.Fprintf(&b, " · v%d", export.Version)
	} else {
		b.WriteString(" · 未发布草稿")
	}
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "状态：%s", export.PublishStatus)
	if export.PublishedAt != "" {
		fmt.Fprintf(&b, " · %s", export.PublishedAt)
	}
	if export.PublishedBy != "" {
		fmt.Fprintf(&b, " · %s", export.PublishedBy)
	}
	b.WriteString("\n\n")

	if export.CoverageNote != "" || export.DriftNote != "" {
		b.WriteString("## 未验证\n\n")
		if export.CoverageNote != "" {
			fmt.Fprintf(&b, "- %s\n", export.CoverageNote)
		}
		if export.DriftNote != "" {
			fmt.Fprintf(&b, "- %s\n", export.DriftNote)
		}
		b.WriteString("\n")
	}

	b.WriteString("## 声明的契约\n\n")
	classNames := make([]string, 0, len(declared.classOrder))
	for _, name := range declared.classOrder {
		if class := declared.classes[name]; class.label != "" {
			classNames = append(classNames, fmt.Sprintf("%s（%s）", name, class.label))
		} else {
			classNames = append(classNames, name)
		}
	}
	fmt.Fprintf(&b, "- %d 个类：%s\n", len(classNames), strings.Join(classNames, " · "))

	relationNames := make([]string, 0, len(declared.propOrder))
	for _, name := range declared.propOrder {
		property := declared.props[name]
		if property.isDatatype() {
			continue
		}
		edge := name
		if len(property.domains) > 0 && len(property.ranges) > 0 {
			edge = fmt.Sprintf("%s → %s（%s）", property.domains[0], property.ranges[0], name)
		}
		if role := relationViaRole(config, name); role != "" {
			edge += " · 经 " + role
		}
		relationNames = append(relationNames, edge)
	}
	fmt.Fprintf(&b, "- %d 个关系：%s\n", len(relationNames), strings.Join(relationNames, "、"))

	actions := declaredActions(config)
	if len(actions) > 0 {
		names := make([]string, 0, len(actions))
		for _, action := range actions {
			names = append(names, graphStr(action["type"]))
		}
		fmt.Fprintf(&b, "- %d 个动作：%s\n", len(names), strings.Join(names, "、"))
	}
	b.WriteString("\n")

	b.WriteString("## 能回答什么\n\n")
	b.WriteString(renderCoverageProse(export))
	b.WriteString("\n")
	return b.String()
}

// renderCoverageProse writes the per-question ledger. The caller supplies the
// findings; when nothing has been measured this states that plainly instead of
// printing an empty list that reads as "no questions".
func renderCoverageProse(export *ModelContractExport) string {
	if export.CoverageNote == "" && export.DriftNote == "" {
		return "尚未对该模型跑过编译，因此没有可核对的能力问题覆盖度。\n"
	}
	lines := make([]string, 0, 2)
	if export.CoverageNote != "" {
		lines = append(lines, "- ✗ "+export.CoverageNote)
	}
	if export.DriftNote != "" {
		lines = append(lines, "- 声明之外的类型："+export.DriftNote)
	}
	if len(lines) == 0 {
		return "- ✓ 所有已声明的能力问题都可回答。\n"
	}
	return strings.Join(lines, "\n") + "\n"
}

// InstanceExportColumns is the whitelist a row export carries.
//
// It is a whitelist and not the whole row: the stored row also holds retrieval
// bookkeeping (weight vectors, scope stamps, index internals) that means nothing
// outside this system, and exporting it would make the file a dump of our schema
// rather than a contract.
var InstanceExportColumns = []string{
	"id", "doc_id", "type_kwd", "scope_kwd",
	"name_kwd", "entity_type_kwd", "attr",
	"from_entity_kwd", "to_entity_kwd", "prop_kwd", "from_type_kwd", "to_type_kwd",
	"content_with_weight", "mention_count_int",
	"compilation_template_ids", "compilation_template_version_int",
	"source_chunk_ids",
}

// instanceExportPageSize is how many rows one request reads while streaming.
// Large enough that the per-request overhead disappears, small enough that the
// response starts inside a second even on a large dataset.
const instanceExportPageSize = 500

// StreamModelInstances writes one model's compiled rows as JSON Lines.
//
// It streams because the file is bounded by the corpus, not by memory: a dataset
// with a million instances produces a file nobody wants to see assembled in RAM,
// and the caller is a download that can be aborted at any line.
func (s *DatasetArtifactService) StreamModelInstances(
	ctx context.Context, tenantID, datasetID, templateID string, write func([]byte) error,
) (int, error) {
	datasetID = strings.TrimSpace(datasetID)
	templateID = strings.TrimSpace(templateID)
	if datasetID == "" || templateID == "" {
		return 0, fmt.Errorf("dataset and template are both required")
	}
	template, err := dao.NewCompilationTemplateDAO().GetTemplate(ctx, dao.DB, tenantID, templateID)
	if err != nil {
		return 0, err
	}
	filter := datasetBucketFilter(string(template.Kind), templateID)
	encoder := json.NewEncoder(newLineWriter(write))
	written := 0
	for offset := 0; ; offset += instanceExportPageSize {
		rows, total, err := graphRowSearch(ctx, tenantID, datasetID, InstanceExportColumns, filter,
			(&types.OrderByExpr{}).Asc("id"), offset, instanceExportPageSize, nil)
		if err != nil {
			return written, err
		}
		if len(rows) == 0 {
			break
		}
		ids := make([]string, 0, len(rows))
		for id := range rows {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if err := encoder.Encode(exportInstanceRecord(rows[id])); err != nil {
				return written, err
			}
			written++
		}
		if int64(offset+len(rows)) >= total {
			break
		}
	}
	return written, nil
}

// exportInstanceRecord projects one stored row onto the export shape.
func exportInstanceRecord(row map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(row))
	for _, column := range InstanceExportColumns {
		value, ok := row[column]
		if !ok {
			continue
		}
		out[exportColumnName(column)] = exportColumnValue(value)
	}
	return out
}

// exportColumnName drops the engine's `_kwd` / `_int` suffixes so the file reads
// like data rather than like our column naming convention.
func exportColumnName(column string) string {
	return strings.TrimSuffix(strings.TrimSuffix(column, "_kwd"), "_int")
}

// exportColumnValue unwraps the single-element arrays the engines return for
// scalar columns, and passes structured values through unchanged.
func exportColumnValue(value interface{}) interface{} {
	if list, ok := value.([]interface{}); ok && len(list) == 1 {
		return list[0]
	}
	return value
}

// newLineWriter adapts a byte writer into an io.Writer for json.Encoder, which
// appends the newline that makes the output JSON Lines.
type lineWriter struct{ write func([]byte) error }

func newLineWriter(write func([]byte) error) *lineWriter { return &lineWriter{write: write} }

func (w *lineWriter) Write(p []byte) (int, error) {
	if err := w.write(p); err != nil {
		return 0, err
	}
	return len(p), nil
}
