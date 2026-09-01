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

package harness

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service/nav"
)

// buildCompilationMap reports which compiled artifacts each bound KB carries,
// mirroring Python _get_compilation_map + _add_template_group_compilations +
// _has_dataset_nav_rows. The Pipeline uses this map to gate compilation-requiring
// tools (ontology_navigate / mindmap_navigate / graph_explore / wiki_query) and to
// decide which tree/timeline/mindmap/wiki artifact a route may target.
//
// The map is keyed by KB id; each value is the set of compilation kinds that KB
// carries, using the SAME canonical tokens the tools filter on:
//   - "toc" / "tree"          → ontology_navigate catalog kinds
//   - "mindmap"               → mindmap_navigate
//   - "graph" / "knowledge_graph" → graph_explore
//   - "wiki"                  → wiki_query
//   - "page_index" / "timeline"   → ontology_navigate catalog kinds
func buildCompilationMap(ctx context.Context, db *gorm.DB, tenantID string, datasetIDs []string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	if db == nil || len(datasetIDs) == 0 {
		return out
	}
	kbDAO := dao.KnowledgebaseDAO{}
	kbs, err := kbDAO.GetByIDs(ctx, db, datasetIDs)
	if err != nil {
		return out
	}
	tplDAO := dao.CompilationTemplateDAO{}
	navSvc := nav.GetNavService()

	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		comps := map[string]bool{}
		pc := kb.ParserConfig

		// 1. Top-level parser_config toggles (mirrors _get_compilation_map).
		if boolVal(pc, "toc") {
			comps["toc"] = true
		}
		if boolVal(pc, "knowledge_graph") {
			comps["knowledge_graph"] = true
		}
		if boolVal(pc, "wiki") {
			comps["wiki"] = true
		}
		if boolVal(pc, "mindmap") {
			comps["mindmap"] = true
		}
		if boolVal(pc, "page_index") {
			comps["page_index"] = true
		}

		// 2. Template-group-derived compilations (mirrors _add_template_group_compilations).
		for _, groupID := range parserConfigTemplateGroupIDs(pc) {
			templates, err := tplDAO.ListByGroup(ctx, db, groupID)
			if err != nil {
				continue
			}
			for _, t := range templates {
				if t == nil {
					continue
				}
				addCompilationKind(comps, t.Kind)
			}
		}

		// 3. Dataset-navigation rows → "tree" (mirrors _has_dataset_nav_rows).
		if hasDatasetNavRows(ctx, navSvc, tenantID, kb.ID) {
			comps["tree"] = true
		}

		if len(comps) > 0 {
			out[kb.ID] = comps
		}
	}
	return out
}

// addCompilationKind maps a template kind onto the canonical compilation tokens,
// mirroring _compilation_kind_for_agentic_map + the comps accumulation in
// _add_template_group_compilations.
func addCompilationKind(comps map[string]bool, rawKind string) {
	// normalizeCompilationKind has already collapsed page_index/pageindex to
	// "timeline", so those tokens never reach the switch (mirrors Python, where
	// the same collapse makes its page_index/pageindex elif branch unreachable).
	norm := normalizeCompilationKind(rawKind)
	switch norm {
	case "knowledge_graph":
		comps["knowledge_graph"] = true
	case "tree":
		comps["tree"] = true
	case "timeline":
		comps["page_index"] = true
	case "mindmap", "mind_map":
		comps["mindmap"] = true
	case "wiki":
		comps["wiki"] = true
	}
}

// normalizeCompilationKind mirrors Python _compilation_kind_for_agentic_map:
// pageindex/page_index collapse to "timeline"; otherwise lower + '-'→'_'.
func normalizeCompilationKind(kind string) string {
	norm := strings.ToLower(strings.TrimSpace(kind))
	norm = strings.ReplaceAll(norm, "-", "_")
	switch norm {
	case "pageindex", "page_index":
		return "timeline"
	default:
		return norm
	}
}

// parserConfigTemplateGroupIDs mirrors Python _parser_config_compilation_template_group_ids:
// read "compilation_template_group_id" (top-level or under "ext"), accept a
// single string or a list, dedup, drop empties.
func parserConfigTemplateGroupIDs(pc entity.JSONMap) []string {
	if pc == nil {
		return nil
	}
	var raw interface{}
	if v, ok := pc["compilation_template_group_id"]; ok {
		raw = v
	} else if ext, ok := pc["ext"].(map[string]interface{}); ok {
		raw = ext["compilation_template_group_id"]
	}
	if raw == nil {
		return nil
	}
	var list []string
	switch v := raw.(type) {
	case string:
		list = []string{v}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				list = append(list, s)
			}
		}
	case []string:
		list = v
	}
	seen := map[string]bool{}
	var out []string
	for _, id := range list {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// hasDatasetNavRows reports whether the KB has dataset-navigation rows (mirrors
// Python _has_dataset_nav_rows). It asks the nav service for top-level clusters;
// a non-empty list means the tree artifact is present.
func hasDatasetNavRows(ctx context.Context, ns nav.NavService, tenantID, kbID string) bool {
	if ns == nil || tenantID == "" || kbID == "" {
		return false
	}
	clusters, _, err := ns.ListClusters(ctx, tenantID, kbID, 0, 1)
	if err != nil {
		return false
	}
	return len(clusters) > 0
}

func boolVal(m entity.JSONMap, key string) bool {
	if m == nil {
		return false
	}
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	case float64:
		return v != 0
	}
	return false
}
