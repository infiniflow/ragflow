//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
)

// Structure-graph sampling constants (mirror structure_graph_common.py).
const (
	graphFullThreshold     = 1024 // below this combined count, return all rows
	graphTopEntities       = 256  // seed set A size for large buckets
	graphKeywordCandidates = 16   // keyword/KNN candidate rows
	graphExpansionCap      = 4096 // hub-node expansion cap
)

var graphEntityFields = []string{"id", "content_with_weight", "name_kwd", "mention_count_int", "source_chunk_ids", "doc_id", "doc_ids_kwd", "source_doc_ids"}
var graphRelationFields = []string{"id", "content_with_weight", "from_entity_kwd", "to_entity_kwd", "doc_id", "doc_ids_kwd", "source_doc_ids"}
var graphAllFields = []string{
	"id", "content_with_weight", "name_kwd", "mention_count_int", "source_chunk_ids",
	"from_entity_kwd", "to_entity_kwd", "knowledge_graph_kwd", "doc_id", "doc_ids_kwd", "source_doc_ids",
}

// StructureGraphNode is a projected entity in the structure graph response.
type StructureGraphNode map[string]interface{}

// StructureGraphRelation is a projected relation in the structure graph response.
type StructureGraphRelation map[string]interface{}

// DocumentStructureGraphTemplate is one per-template bucket in the response.
type DocumentStructureGraphTemplate struct {
	TemplateID   string                   `json:"template_id"`
	TemplateName string                   `json:"template_name"`
	Kind         string                   `json:"kind"`
	Entities     []StructureGraphNode     `json:"entities"`
	Relations    []StructureGraphRelation `json:"relations"`
}

// DocumentStructureGraphResponse mirrors Python's {"templates": [...]}.
type DocumentStructureGraphResponse struct {
	Templates []DocumentStructureGraphTemplate `json:"templates"`
}

// graphRowSearch runs one raw-row search over the tenant's document index.
func graphRowSearch(ctx context.Context, tenantID, datasetID string, selectFields []string, filter map[string]interface{}, orderBy *types.OrderByExpr, offset, limit int, matchExprs []interface{}) (map[string]map[string]interface{}, int64, error) {
	docEngine := engine.Get()
	if docEngine == nil {
		return nil, 0, fmt.Errorf("document engine is not initialized")
	}
	merged := make(map[string]interface{}, len(filter)+1)
	for k, v := range filter {
		merged[k] = v
	}
	merged["kb_id"] = []string{datasetID}
	if limit < 1 {
		limit = 1
	}
	res, err := docEngine.Search(ctx, &types.SearchRequest{
		IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenantID)},
		KbIDs:        []string{datasetID},
		Offset:       offset,
		Limit:        limit,
		SelectFields: selectFields,
		Filter:       merged,
		OrderBy:      orderBy,
		MatchExprs:   matchExprs,
	})
	if err != nil {
		return nil, 0, err
	}
	if res == nil {
		return nil, 0, nil
	}
	byID := make(map[string]map[string]interface{}, len(res.Chunks))
	for _, c := range res.Chunks {
		id := firstStringValue(c["id"])
		if id != "" {
			byID[id] = c
		}
	}
	return byID, res.Total, nil
}

// graphLoadPayload parses content_with_weight into a dict.
func graphLoadPayload(row map[string]interface{}) map[string]interface{} {
	raw := firstStringValue(row["content_with_weight"])
	if raw == "" {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	return m
}

func graphIsInvalidSentinel(v string) bool {
	if v == "" {
		return true
	}
	lv := strings.ToLower(v)
	for _, bad := range []string{"unknown", "none", "null", "nan", "n/a", "undefined", "other"} {
		if lv == bad {
			return true
		}
	}
	return false
}

// projectEntity mirrors _struct_graph_entity + project_entity.
func projectEntity(row map[string]interface{}) StructureGraphNode {
	payload := graphLoadPayload(row)
	if payload == nil {
		return nil
	}
	name := ""
	for _, k := range []string{"name", "text", "term", "title"} {
		if v, ok := payload[k].(string); ok && strings.TrimSpace(v) != "" {
			name = strings.TrimSpace(v)
			break
		}
	}
	if name == "" || graphIsInvalidSentinel(name) {
		return nil
	}
	typ := "other"
	if v, ok := payload["type"].(string); ok && strings.TrimSpace(v) != "" {
		typ = strings.TrimSpace(v)
	}
	var aliases []string
	switch a := payload["aliases"].(type) {
	case string:
		if strings.TrimSpace(a) != "" {
			aliases = []string{strings.TrimSpace(a)}
		}
	case []interface{}:
		for _, e := range a {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				aliases = append(aliases, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, s := range a {
			if strings.TrimSpace(s) != "" {
				aliases = append(aliases, strings.TrimSpace(s))
			}
		}
	}
	desc := ""
	for _, k := range []string{"description", "definition_excerpt"} {
		if v, ok := payload[k].(string); ok {
			desc = strings.TrimSpace(v)
			if desc != "" {
				break
			}
		}
	}
	chunkIDs := graphSourceChunkIDs(payload, row)
	node := StructureGraphNode{
		"aliases":          aliases,
		"mention_count":    1,
		"name":             name,
		"source_chunk_ids": chunkIDs,
		"type":             typ,
		"description":      desc,
	}
	if mc, ok := graphMentionCount(row); ok {
		node["mention_count"] = mc
	}
	return node
}

func graphSourceChunkIDs(payload, row map[string]interface{}) []string {
	var out []string
	add := func(v interface{}) {
		switch x := v.(type) {
		case []string:
			for _, s := range x {
				if s != "" {
					out = append(out, s)
				}
			}
		case []interface{}:
			for _, e := range x {
				if s, ok := e.(string); ok && s != "" {
					out = append(out, s)
				}
			}
		case string:
			if x != "" {
				out = append(out, x)
			}
		}
	}
	if raw, ok := payload["source_chunk_ids"]; ok {
		add(raw)
	} else {
		add(row["source_chunk_ids"])
	}
	// dedup order-preserving
	seen := map[string]bool{}
	dedup := out[:0]
	for _, s := range out {
		if !seen[s] {
			seen[s] = true
			dedup = append(dedup, s)
		}
	}
	return dedup
}

func graphMentionCount(row map[string]interface{}) (int, bool) {
	v := row["mention_count_int"]
	if l, ok := v.([]interface{}); ok {
		if len(l) > 0 {
			v = l[0]
		} else {
			return 0, false
		}
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
	}
	return 0, false
}

// projectRelation mirrors _struct_graph_relation + project_relation.
func projectRelation(row map[string]interface{}) StructureGraphRelation {
	payload := graphLoadPayload(row)
	src, tgt := "", ""
	if payload != nil {
		for _, k := range []string{"source", "src", "from"} {
			if v, ok := payload[k].(string); ok {
				src = strings.TrimSpace(v)
				if src != "" {
					break
				}
			}
		}
		for _, k := range []string{"target", "tgt", "to"} {
			if v, ok := payload[k].(string); ok {
				tgt = strings.TrimSpace(v)
				if tgt != "" {
					break
				}
			}
		}
	}
	typ := "related"
	if payload != nil {
		if v, ok := payload["type"].(string); ok && strings.TrimSpace(v) != "" {
			typ = strings.TrimSpace(v)
		}
	}
	if src == "" || tgt == "" || graphIsInvalidSentinel(src) || graphIsInvalidSentinel(tgt) {
		// Fall back to the authoritative *_entity_kwd columns.
		fallbackSrc := strings.TrimSpace(firstStringValue(row["from_entity_kwd"]))
		fallbackTgt := strings.TrimSpace(firstStringValue(row["to_entity_kwd"]))
		if fallbackSrc != "" && fallbackTgt != "" {
			return StructureGraphRelation{"from": fallbackSrc, "to": fallbackTgt, "type": typ}
		}
		return nil
	}
	return StructureGraphRelation{"from": src, "to": tgt, "type": typ}
}

// dedupEntities order-preserving by (lowercased name, type).
func dedupEntities(entities []StructureGraphNode) []StructureGraphNode {
	var out []StructureGraphNode
	seen := map[string]bool{}
	for _, e := range entities {
		name := strings.ToLower(strings.TrimSpace(graphStr(e["name"])))
		typ := strings.ToLower(strings.TrimSpace(graphStr(e["type"])))
		key := name + "\x00" + typ
		if name == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}

func entityResponseID(entity StructureGraphNode) string {
	for _, f := range []string{"id", "name", "slug"} {
		if v, ok := entity[f].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func endpointTerms(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return sortedUnique([]string{value, strings.ToLower(value)})
}

func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// normalizeRelationEndpoints aligns relation endpoints to returned entity ids.
func normalizeRelationEndpoints(entities []StructureGraphNode, relations []StructureGraphRelation) []StructureGraphRelation {
	if len(entities) == 0 || len(relations) == 0 {
		return relations
	}
	lookup := map[string]string{}
	ambiguous := map[string]bool{}
	for _, entity := range entities {
		respID := entityResponseID(entity)
		if respID == "" {
			continue
		}
		for _, f := range []string{"id", "name", "slug"} {
			v, ok := entity[f].(string)
			if !ok || strings.TrimSpace(v) == "" {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(v))
			if cur, exists := lookup[key]; exists && cur != respID {
				ambiguous[key] = true
				continue
			}
			lookup[key] = respID
		}
	}
	for k := range ambiguous {
		delete(lookup, k)
	}
	normalized := make([]StructureGraphRelation, 0, len(relations))
	for _, rel := range relations {
		item := make(StructureGraphRelation, len(rel)+2)
		for k, v := range rel {
			item[k] = v
		}
		for _, f := range []string{"from", "to"} {
			if v, ok := item[f].(string); ok {
				if mapped, found := lookup[strings.ToLower(strings.TrimSpace(v))]; found {
					item[f] = mapped
				}
			}
		}
		normalized = append(normalized, item)
	}
	return normalized
}

// rowHasEnabledSource mirrors _row_has_enabled_source.
func rowHasEnabledSource(row map[string]interface{}, excludedDocIDs map[string]bool) bool {
	if len(excludedDocIDs) == 0 {
		return true
	}
	sourceIDs := map[string]bool{}
	flatten := func(v interface{}) {
		var walk func(interface{})
		walk = func(x interface{}) {
			switch val := x.(type) {
			case string:
				t := strings.TrimSpace(val)
				if t == "" {
					return
				}
				var parsed []interface{}
				if json.Unmarshal([]byte(t), &parsed) == nil {
					for _, p := range parsed {
						walk(p)
					}
					return
				}
				sourceIDs[t] = true
			case []interface{}:
				for _, e := range val {
					walk(e)
				}
			case []string:
				for _, s := range val {
					walk(s)
				}
			default:
				if s, ok := x.(string); ok && s != "" {
					sourceIDs[s] = true
				}
			}
		}
		walk(v)
	}
	for _, f := range []string{"doc_ids_kwd", "source_doc_ids"} {
		if v, ok := row[f]; ok && v != nil {
			flatten(v)
		}
	}
	if len(sourceIDs) > 0 {
		for id := range sourceIDs {
			if !excludedDocIDs[id] {
				return true
			}
		}
		return false
	}
	flatten(row["doc_id"])
	if len(sourceIDs) > 0 {
		for id := range sourceIDs {
			if !excludedDocIDs[id] {
				return true
			}
		}
		return false
	}
	return true
}

func graphStr(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// buildBucket mirrors sgc.build_bucket: small buckets whole, large sampled.
func (s *DatasetArtifactService) buildBucket(ctx context.Context, tenantID, datasetID string, scope map[string]interface{}, excludedDocIDs map[string]bool) ([]StructureGraphNode, []StructureGraphRelation, error) {
	excludedDocIDs = excludedDocIDsOrEmpty(excludedDocIDs)
	bothCond := copyFilter(scope)
	bothCond["knowledge_graph_kwd"] = []string{"entity", "relation"}
	_, total, err := graphRowSearch(ctx, tenantID, datasetID, []string{"id"}, bothCond, nil, 0, 1, nil)
	if err != nil {
		return nil, nil, err
	}
	if total < graphFullThreshold {
		fieldMap, _, err := graphRowSearch(ctx, tenantID, datasetID, graphAllFields, bothCond, nil, 0, int(total), nil)
		if err != nil {
			return nil, nil, err
		}
		// Initialize as empty (non-nil) slices so a bucket with entities but no
		// relations serializes "relations": [] instead of null — the frontend
		// adapters (adaptKnowledgeGraphToForceGraph et al.) call .filter() on
		// relations without a null guard (mirror Python, which always returns []).
		entities := make([]StructureGraphNode, 0)
		relations := make([]StructureGraphRelation, 0)
		for _, row := range fieldMap {
			if !rowHasEnabledSource(row, excludedDocIDs) {
				continue
			}
			kg := firstStringValue(row["knowledge_graph_kwd"])
			if kg == "relation" {
				if edge := projectRelation(row); edge != nil {
					relations = append(relations, edge)
				}
			} else {
				if node := projectEntity(row); node != nil {
					entities = append(entities, node)
				}
			}
		}
		entities = dedupEntities(entities)
		return entities, normalizeRelationEndpoints(entities, relations), nil
	}

	// Large bucket: sample. A = top entities by mention_count_int desc.
	orderBy := (&types.OrderByExpr{}).Desc("mention_count_int")
	var setA []StructureGraphNode
	entityOffset := 0
	var entityTotal int64 = -1
	for len(setA) < graphTopEntities && (entityTotal == -1 || int64(entityOffset) < entityTotal) {
		cond := copyFilter(scope)
		cond["knowledge_graph_kwd"] = []string{"entity"}
		entAMap, entTotal, err := graphRowSearch(ctx, tenantID, datasetID, graphEntityFields, cond, orderBy, entityOffset, graphTopEntities, nil)
		if err != nil {
			return nil, nil, err
		}
		entityTotal = entTotal
		if len(entAMap) == 0 {
			break
		}
		for _, row := range entAMap {
			if !rowHasEnabledSource(row, excludedDocIDs) {
				continue
			}
			if n := projectEntity(row); n != nil {
				setA = append(setA, n)
			}
		}
		entityOffset += len(entAMap)
	}
	if len(setA) > graphTopEntities {
		setA = setA[:graphTopEntities]
	}
	var aNames []string
	for _, e := range setA {
		if n := strings.TrimSpace(graphStr(e["name"])); n != "" {
			aNames = append(aNames, n)
		}
	}
	var aNameTerms []string
	for _, name := range aNames {
		aNameTerms = append(aNameTerms, endpointTerms(name)...)
	}
	aNameTerms = sortedUnique(aNameTerms)

	relations := make([]StructureGraphRelation, 0)
	targetNamesLower := map[string]bool{}
	if len(aNameTerms) > 0 {
		cond := copyFilter(scope)
		cond["knowledge_graph_kwd"] = []string{"relation"}
		cond["from_entity_kwd"] = aNameTerms
		relMap, _, err := graphRowSearch(ctx, tenantID, datasetID, graphRelationFields, cond, nil, 0, graphExpansionCap, nil)
		if err != nil {
			return nil, nil, err
		}
		for _, row := range relMap {
			if !rowHasEnabledSource(row, excludedDocIDs) {
				continue
			}
			if edge := projectRelation(row); edge != nil {
				relations = append(relations, edge)
				if tgt := strings.ToLower(strings.TrimSpace(graphStr(edge["to"]))); tgt != "" {
					targetNamesLower[tgt] = true
				}
			}
		}
	}
	var setT []StructureGraphNode
	if len(targetNamesLower) > 0 {
		cond := copyFilter(scope)
		cond["knowledge_graph_kwd"] = []string{"entity"}
		cond["name_kwd"] = sortedKeys(targetNamesLower)
		tgtMap, _, err := graphRowSearch(ctx, tenantID, datasetID, graphEntityFields, cond, nil, 0, graphExpansionCap, nil)
		if err != nil {
			return nil, nil, err
		}
		for _, row := range tgtMap {
			if !rowHasEnabledSource(row, excludedDocIDs) {
				continue
			}
			if n := projectEntity(row); n != nil {
				setT = append(setT, n)
			}
		}
	}
	entities := dedupEntities(append(setA, setT...))
	return entities, normalizeRelationEndpoints(entities, relations), nil
}

func copyFilter(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func excludedDocIDsOrEmpty(in map[string]bool) map[string]bool {
	if in == nil {
		return map[string]bool{}
	}
	return in
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// compilationTemplateKind normalizes a template kind (mirror Python
// _compilation_template_kind).
func compilationTemplateKind(kind string) string {
	k := strings.ToLower(strings.TrimSpace(kind))
	switch k {
	case "page_index", "pageindex":
		return "page_index"
	case "wiki", "wiki_page":
		return "wiki"
	case "raptor":
		return "raptor"
	}
	return k
}

// ErrInvalidStructureKind is returned when the request kind does not resolve to a
// supported dataset-structure kind. Handlers use errors.Is to map it to
// CodeArgumentError (400) while treating every other service error as a server
// fault (500).
var ErrInvalidStructureKind = errors.New("invalid structure kind")

// datasetStructureSupported mirrors the writer's engine guard: only Infinity and
// Elasticsearch resolve the dataset-structure filter keys (scope_kwd +
// knowledge_graph_kwd). The writer keeps a private copy in the knowledge_compile
// package; this service-side copy keeps the API delete path in agreement without
// exporting an ingestion-internal helper across package boundaries.
func datasetStructureSupported() bool {
	switch engine.GetEngineType() {
	case "infinity", "elasticsearch", "":
		return true
	default:
		return false
	}
}

// errDatasetStructureUnsupported returns a visible error naming the engine.
func errDatasetStructureUnsupported() error {
	return fmt.Errorf("dataset structure graph unsupported on doc engine %q", engine.GetEngineType())
}

// resolveDatasetStructureKind maps a user-facing dataset-structure kind to the
// stored top-level kind (mirror Python _DATASET_STRUCTURE_KIND_ALIASES). It is
// DELIBERATELY separate from compilationTemplateKind: that helper folds
// knowledge_graph→timeline, which would merge distinct dataset kinds, and it
// lacks the graph→knowledge_graph / mindmap→mind_map aliases (Python's
// dataset_api_service.py L1803-1805 documents exactly why it does not reuse the
// general normalizer). Returns "" for an invalid kind (caller maps that to
// 400 ARGUMENT_ERROR, never "return all").
func resolveDatasetStructureKind(kind string) string {
	k := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(kind, "-", "_")))
	switch k {
	case "graph", "knowledge_graph":
		return "knowledge_graph"
	case "mindmap", "mind_map":
		// Align with Python _DATASET_STRUCTURE_KIND_ALIASES: resolve the
		// user-facing "mindmap" to the stored compilation_template_kind_kwd value
		// "mind_map" (the template kind, which is what read/delete paths match on).
		return "mind_map"
	case "timeline":
		return "timeline"
	case "session_essence":
		return "session_essence"
	case "session_graph":
		return "session_graph"
	}
	return ""
}

// structureKindForBucket returns the normalized kind used for tree/page_index
// hierarchy handling.
func structureKindForBucket(kind string) string {
	k := compilationTemplateKind(kind)
	if k == "page_index" {
		return "page_index"
	}
	return strings.ReplaceAll(k, "-", "_")
}

// loadDocumentTemplateMeta resolves the doc's configured template group into
// configured_ids + template_meta maps, mirroring the Python resolution.
func (s *DatasetArtifactService) loadDocumentTemplateMeta(ctx context.Context, tenantID, documentID string) ([]string, map[string]map[string]interface{}, error) {
	empty := map[string]map[string]interface{}{}
	doc, err := dao.NewDocumentDAO().GetByID(ctx, dao.DB, documentID)
	if err != nil || doc == nil {
		return nil, empty, err
	}
	groupID := ""
	if doc.ParserConfig != nil {
		if gid, ok := doc.ParserConfig["compilation_template_group_id"].(string); ok && gid != "" {
			groupID = gid
		} else if ext, ok := doc.ParserConfig["ext"].(map[string]interface{}); ok {
			if gid, ok := ext["compilation_template_group_id"].(string); ok && gid != "" {
				groupID = gid
			}
		}
	}
	if groupID == "" {
		return nil, empty, nil
	}
	group, err := NewCompilationTemplateGroupService().GetSaved(ctx, tenantID, groupID)
	if err != nil {
		return nil, empty, err
	}
	configuredIDs := []string{}
	meta := map[string]map[string]interface{}{}
	seen := map[string]bool{}
	if group != nil {
		for _, t := range group.Templates {
			tid := t.ID
			if tid == "" || seen[tid] {
				continue
			}
			rawKind := ""
			if t.Config != nil {
				if k, ok := t.Config["kind"].(string); ok {
					rawKind = k
				}
			}
			if rawKind == "" {
				rawKind = t.Kind
			}
			kindNorm := compilationTemplateKind(rawKind)
			if kindNorm == "wiki" {
				continue
			}
			seen[tid] = true
			configuredIDs = append(configuredIDs, tid)
			meta[tid] = map[string]interface{}{
				"template_id":   tid,
				"template_name": t.Name,
				"kind":          rawKind,
			}
		}
	}
	return configuredIDs, meta, nil
}

// DocumentStructureGraphInput is the parsed request for GetDocumentGraph.
type DocumentStructureGraphInput struct {
	TenantID   string
	DatasetID  string
	DocumentID string
	GraphType  string
	Keywords   string
}

// GetDocumentGraph returns the per-template structure graph of a document,
// mirroring Python get_document_structure_graph (normal + keywords modes).
func (s *DatasetArtifactService) GetDocumentGraph(ctx context.Context, in DocumentStructureGraphInput) (*DocumentStructureGraphResponse, error) {
	configuredIDs, templateMeta, err := s.loadDocumentTemplateMeta(ctx, in.TenantID, in.DocumentID)
	if err != nil {
		return nil, err
	}

	resp := &DocumentStructureGraphResponse{Templates: []DocumentStructureGraphTemplate{}}

	// keywords mode: name matching/KNN → matched entities' subgraph.
	if in.Keywords != "" {
		bucketMeta, entities, relations, err := s.keywordSubgraph(ctx, in.TenantID, in.DatasetID, in.DocumentID, in.Keywords, templateMeta)
		if err != nil {
			return nil, err
		}
		if bucketMeta == nil || (len(entities) == 0 && len(relations) == 0) {
			return resp, nil
		}
		resp.Templates = append(resp.Templates, DocumentStructureGraphTemplate{
			TemplateID:   graphStr(bucketMeta["template_id"]),
			TemplateName: graphStr(bucketMeta["template_name"]),
			Kind:         graphStr(bucketMeta["kind"]),
			Entities:     entities,
			Relations:    relations,
		})
		return resp, nil
	}

	// normal mode: discover buckets from per-doc graph blob rows. "id" is
	// required for the same reason as dataset discovery (Infinity only projects
	// listed fields; graphRowSearch keys by id).
	metaFields := []string{"id", "compile_kwd", "compilation_template_ids", "compilation_template_kind_kwd"}
	metaRows, _, err := graphRowSearch(ctx, in.TenantID, in.DatasetID, metaFields,
		map[string]interface{}{"doc_id": []string{in.DocumentID}, "knowledge_graph_kwd": []string{"graph"}}, nil, 0, 1000, nil)
	if err != nil {
		return nil, err
	}

	bucketMetas := map[string]map[string]interface{}{}
	bucketScopes := map[string]map[string]interface{}{}
	for _, row := range metaRows {
		meta, scope := resolveGraphBucket(row, templateMeta, in.DocumentID)
		bid := graphStr(meta["template_id"])
		if _, ok := bucketMetas[bid]; !ok {
			bucketMetas[bid] = meta
			bucketScopes[bid] = scope
		}
	}

	grouped := map[string]DocumentStructureGraphTemplate{}
	for bid, meta := range bucketMetas {
		entities, relations, err := s.buildBucket(ctx, in.TenantID, in.DatasetID, bucketScopes[bid], nil)
		if err != nil {
			return nil, err
		}
		if len(entities) == 0 && len(relations) == 0 {
			continue
		}
		grouped[bid] = DocumentStructureGraphTemplate{
			TemplateID:   graphStr(meta["template_id"]),
			TemplateName: graphStr(meta["template_name"]),
			Kind:         graphStr(meta["kind"]),
			Entities:     entities,
			Relations:    relations,
		}
	}

	// RAPTOR summary graph blob (compile_kwd = raptor_graph).
	s.appendRaptorBlob(ctx, in.TenantID, in.DatasetID, in.DocumentID, grouped)

	// Order: configured templates first, then discovered.
	orderedIDs := []string{}
	for _, tid := range configuredIDs {
		if _, ok := grouped[tid]; ok && !containsStr(orderedIDs, tid) {
			orderedIDs = append(orderedIDs, tid)
		}
	}
	for bid := range grouped {
		if !containsStr(orderedIDs, bid) {
			orderedIDs = append(orderedIDs, bid)
		}
	}
	for _, bid := range orderedIDs {
		if g, ok := grouped[bid]; ok && (len(g.Entities) > 0 || len(g.Relations) > 0) {
			resp.Templates = append(resp.Templates, g)
		}
	}
	return resp, nil
}

func containsStr(list []string, v string) bool {
	for _, e := range list {
		if e == v {
			return true
		}
	}
	return false
}

// DatasetStructureGraphInput is the parsed request for the dataset-scope
// structure graph endpoint (GET/DELETE /datasets/:id/artifacts/structure).
// Kind is REQUIRED (mirrors Python dataset_api.py:715): missing or invalid →
// 400 ARGUMENT_ERROR. Wipe applies to DELETE: true deletes the dataset rows,
// false only cancels the task (rows are left for the next rebuild to clean).
type DatasetStructureGraphInput struct {
	TenantID  string
	DatasetID string
	Kind      string
	Wipe      bool
}

// DatasetStructureGraphResponse mirrors Python get_dataset_structure's
// {"kind": ..., "templates": [...]}.
type DatasetStructureGraphResponse struct {
	Kind      string                           `json:"kind"`
	Templates []DocumentStructureGraphTemplate `json:"templates"`
}

// GetDatasetStructure returns the dataset-scope structure graph for a resolved
// kind, mirroring Python get_dataset_structure (dataset_api_service.py). Discovery
// scans knowledge_graph_kwd=["entity"] dataset rows (scope_kwd="dataset") and
// matches the resolved kind against the stamped compilation_template_kind_kwd —
// NOT compile_kwd (compile_kwd holds the autotype "hypergraph"/"list"/"mindmap",
// which is never the kind discriminator; Python _discover_scope_templates matches
// _resolve_dataset_structure_kind against compilation_template_kind_kwd the same
// way). It collects distinct template ids, then reads each template's dataset
// entity/relation rows via buildBucket. It does NOT read kg_build_meta (write/
// delete-side only).
func (s *DatasetArtifactService) GetDatasetStructure(ctx context.Context, in DatasetStructureGraphInput) (*DatasetStructureGraphResponse, error) {
	resolved := resolveDatasetStructureKind(in.Kind)
	if resolved == "" {
		return nil, fmt.Errorf("%w: %q", ErrInvalidStructureKind, in.Kind)
	}
	if !datasetStructureSupported() {
		return nil, errDatasetStructureUnsupported()
	}
	resp := &DatasetStructureGraphResponse{Kind: resolved, Templates: []DocumentStructureGraphTemplate{}}

	// Discover distinct template ids whose stamped template kind resolves to the
	// requested kind, scanning dataset-scope entity rows only. scope_kwd="dataset"
	// is required here (unlike the legacy doc_graph fallback) because dataset rows
	// are the only ones carrying compilation_template_kind_kwd we can trust for the
	// dataset-scope kind match.
	templateIDs := map[string]struct{}{}
	// "id" must be projected: graphRowSearch keys its result map by the row id,
	// and Infinity only returns fields listed in SelectFields (it does not
	// synthesize id), so omitting it silently drops every row (review Major).
	// The resolved kind is pushed into the filter so the engine applies the
	// predicate instead of scanning all entity rows and discarding them in Go.
	metaFields := []string{"id", "compilation_template_kind_kwd", "compilation_template_ids"}
	for offset := 0; ; offset += 1000 {
		rows, total, err := graphRowSearch(ctx, in.TenantID, in.DatasetID, metaFields,
			map[string]interface{}{
				"knowledge_graph_kwd":           []string{"entity"},
				"scope_kwd":                     []string{"dataset"},
				"compilation_template_kind_kwd": []string{resolved},
			}, nil, offset, 1000, nil)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			tkind := firstStringValue(row["compilation_template_kind_kwd"])
			if tkind == "" || resolveDatasetStructureKind(tkind) != resolved {
				continue
			}
			tid := rowTemplateID(row)
			if tid != "" {
				templateIDs[tid] = struct{}{}
			}
		}
		if int64(offset+1000) >= total || len(rows) == 0 {
			break
		}
	}

	// Read each template's dataset entity/relation rows.
	for tid := range templateIDs {
		scope := map[string]interface{}{
			"scope_kwd":                     []string{"dataset"},
			"compilation_template_ids":      []string{tid},
			"compilation_template_kind_kwd": []string{resolved},
		}
		entities, relations, err := s.buildBucket(ctx, in.TenantID, in.DatasetID, scope, nil)
		if err != nil {
			return nil, err
		}
		if len(entities) == 0 && len(relations) == 0 {
			continue
		}
		resp.Templates = append(resp.Templates, DocumentStructureGraphTemplate{
			TemplateID:   tid,
			TemplateName: tid,
			Kind:         resolved,
			Entities:     entities,
			Relations:    relations,
		})
	}
	return resp, nil
}

// DeleteDatasetStructure handles DELETE /datasets/:id/artifacts/structure?kind=&wipe=.
// It validates kind like GET. wipe=false cancels the kind's task (via the task-id
// field) without deleting rows; wipe=true deletes the kind's kg_build_meta marker
// + dataset entity/relation rows (document-scope rows are never touched).
func (s *DatasetArtifactService) DeleteDatasetStructure(ctx context.Context, in DatasetStructureGraphInput) (int, error) {
	resolved := resolveDatasetStructureKind(in.Kind)
	if resolved == "" {
		return 0, fmt.Errorf("%w: %q", ErrInvalidStructureKind, in.Kind)
	}
	if !in.Wipe {
		// Cancel the task without deleting rows. Task cancellation is task-id
		// granular (mirrors Python delete_index REDIS set "{task_id}-cancel"); the
		// actual row cleanup happens on the next rebuild. Rows are preserved.
		return s.cancelDatasetStructureTask(ctx, in.TenantID, in.DatasetID, resolved)
	}
	if !datasetStructureSupported() {
		return 0, errDatasetStructureUnsupported()
	}
	docEngine := engine.Get()
	if docEngine == nil {
		return 0, fmt.Errorf("document engine is not initialized")
	}
	indexName := fmt.Sprintf("ragflow_%s", in.TenantID)
	cond := map[string]interface{}{
		"kb_id":                         in.DatasetID,
		"scope_kwd":                     "dataset",
		"compilation_template_kind_kwd": resolved,
		"knowledge_graph_kwd":           []string{"entity", "relation", "kg_build_meta"},
	}
	n, err := docEngine.DeleteChunks(ctx, cond, indexName, in.DatasetID)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// cancelDatasetStructureTask cancels the running dataset-structure task for a
// resolved kind by clearing both its per-index task-id and finish-at fields.
// This mirrors Python delete_index (dataset_api_service.py), which clears
// {task_id_field: "", task_finish_at_field: None} so the task state is fully
// reset (a stale finish-at would otherwise leave the kind looking "done" after
// cancel). The row cleanup itself is driven by the next rebuild; Go executes the
// merge synchronously inside the ingestor (no independent task/marker), so there
// is no Redis "{task_id}-cancel" marker to publish here.
func (s *DatasetArtifactService) cancelDatasetStructureTask(ctx context.Context, tenantID, datasetID, resolvedKind string) (int, error) {
	field := datasetStructureTaskIDField(resolvedKind)
	if field == "" {
		return 0, nil
	}
	updates := map[string]interface{}{
		field: "",
		// gorm.Updates ignores nil values, so use an explicit NULL expression to
		// actually clear the finish-at timestamp (mirrors Python None).
		datasetStructureTaskFinishAtField(field): gorm.Expr("NULL"),
	}
	if err := dao.NewKnowledgebaseDAO().UpdateByID(ctx, dao.DB, datasetID, updates); err != nil {
		return 0, err
	}
	return 0, nil
}

// datasetStructureTaskFinishAtField maps a task-id field to its sibling finish-at
// field, mirroring Python f"{task_id_field.replace('_task_id', '_task_finish_at')}".
func datasetStructureTaskFinishAtField(taskIDField string) string {
	return strings.Replace(taskIDField, "_task_id", "_task_finish_at", 1)
}

// datasetStructureTaskIDField maps a resolved dataset-structure kind to its kb
// task-id field name. It mirrors Python _INDEX_TYPE_TO_TASK_ID_FIELD: each
// dataset-merge kind carries its own "<index_type>_task_id" field (structure_graph,
// structure_mindmap, timeline, session_graph, session_essence), NOT the legacy
// doc-level graphrag_task_id/mindmap_task_id. Empty means "no task-id field".
func datasetStructureTaskIDField(resolvedKind string) string {
	switch resolvedKind {
	case "knowledge_graph":
		// "graph" is already normalized to "knowledge_graph" by
		// resolveDatasetStructureKind, so no separate "graph" case is needed.
		return "structure_graph_task_id"
	case "mindmap", "mind_map":
		return "structure_mindmap_task_id"
	case "timeline":
		return "timeline_task_id"
	case "session_graph":
		return "session_graph_task_id"
	case "session_essence":
		return "session_essence_task_id"
	}
	return ""
}

// resolveGraphBucket mirrors Python _resolve_bucket.
func resolveGraphBucket(row map[string]interface{}, templateMeta map[string]map[string]interface{}, documentID string) (map[string]interface{}, map[string]interface{}) {
	compileKwd := firstStringValue(row["compile_kwd"])
	kindVal := firstStringValue(row["compilation_template_kind_kwd"])
	if kindVal == "" {
		kindVal = compileKwd
	}
	tid := rowTemplateID(row)
	if tid != "" {
		kindNorm := compilationTemplateKind(kindVal)
		meta := templateMeta[tid]
		// Only a unique kind match can substitute a missing template meta; an
		// empty kindNorm must never match (review Major — the previous nil
		// round-trip stored a nil entry that the kind-only loop then matched).
		if meta == nil && kindNorm != "" {
			kindMatches := []map[string]interface{}{}
			for _, m := range templateMeta {
				if compilationTemplateKind(graphStr(m["kind"])) == kindNorm {
					kindMatches = append(kindMatches, m)
				}
			}
			if len(kindMatches) == 1 {
				meta = kindMatches[0]
			}
		}
		bucketName := graphStr(meta["template_name"])
		if bucketName == "" {
			bucketName = tid
		}
		bucketKind := graphStr(meta["kind"])
		if bucketKind == "" {
			bucketKind = kindVal
		}
		return map[string]interface{}{
				"template_id":   tid,
				"template_name": bucketName,
				"kind":          bucketKind,
			}, map[string]interface{}{
				"doc_id":                   []string{documentID},
				"compilation_template_ids": []string{tid},
			}
	}
	bucketID := "legacy:" + compileKwd
	return map[string]interface{}{
			"template_id":   bucketID,
			"template_name": "Legacy (" + compileKwd + ")",
			"kind":          kindVal,
		}, map[string]interface{}{
			"doc_id":      []string{documentID},
			"compile_kwd": []string{compileKwd},
			"must_not":    map[string]interface{}{"exists": "compilation_template_ids"},
		}
}

func rowTemplateID(row map[string]interface{}) string {
	switch v := row["compilation_template_ids"].(type) {
	case []interface{}:
		for _, e := range v {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	case string:
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (s *DatasetArtifactService) appendRaptorBlob(ctx context.Context, tenantID, datasetID, documentID string, grouped map[string]DocumentStructureGraphTemplate) {
	rows, _, err := graphRowSearch(ctx, tenantID, datasetID, []string{"id", "content_with_weight", "compile_kwd"},
		map[string]interface{}{"doc_id": []string{documentID}, "compile_kwd": []string{"raptor_graph"}}, nil, 0, 16, nil)
	if err != nil {
		return
	}
	for _, row := range rows {
		payload := graphLoadPayload(row)
		if payload == nil {
			continue
		}
		rEntities, _ := payload["entities"].([]interface{})
		rRelations, _ := payload["relations"].([]interface{})
		if len(rEntities) == 0 && len(rRelations) == 0 {
			continue
		}
		rb, ok := grouped["raptor"]
		if !ok {
			rb = DocumentStructureGraphTemplate{TemplateID: "raptor", TemplateName: "RAPTOR Summary", Kind: "raptor"}
			grouped["raptor"] = rb
		}
		rb.Entities = append(rb.Entities, toNodeSlice(rEntities)...)
		rb.Relations = append(rb.Relations, toRelationSlice(rRelations)...)
		grouped["raptor"] = rb
	}
}

func toNodeSlice(in []interface{}) []StructureGraphNode {
	var out []StructureGraphNode
	for _, e := range in {
		if m, ok := e.(map[string]interface{}); ok {
			out = append(out, StructureGraphNode(m))
		}
	}
	return out
}

func toRelationSlice(in []interface{}) []StructureGraphRelation {
	var out []StructureGraphRelation
	for _, e := range in {
		if m, ok := e.(map[string]interface{}); ok {
			out = append(out, StructureGraphRelation(m))
		}
	}
	return out
}

var graphTextQueryRE = regexp.MustCompile(`[ :|\r\n\t,，。？?/\x60!！&^%()\[\]{}<>*~'"\\=]+`)

func nameMatchesQuery(node StructureGraphNode, query string) bool {
	name := strings.ToLower(strings.TrimSpace(graphStr(node["name"])))
	q := strings.ToLower(query)
	if name == "" || q == "" {
		return false
	}
	if strings.Contains(name, q) {
		return true
	}
	terms := strings.Fields(q)
	if len(terms) == 0 {
		return false
	}
	for _, t := range terms {
		if !strings.Contains(name, t) {
			return false
		}
	}
	return true
}

// keywordSubgraph mirrors sgc.keyword_subgraph (BM25 + KNN fallback + ancestor walk).
func (s *DatasetArtifactService) keywordSubgraph(ctx context.Context, tenantID, datasetID, documentID, keywords string, templateMeta map[string]map[string]interface{}) (map[string]interface{}, []StructureGraphNode, []StructureGraphRelation, error) {
	baseEntityCond := map[string]interface{}{
		"doc_id":              []string{documentID},
		"knowledge_graph_kwd": []string{"entity"},
	}
	topFields := append(append([]string{}, graphEntityFields...), "compilation_template_ids", "compile_kwd", "compilation_template_kind_kwd")

	textQuery := strings.TrimSpace(graphTextQueryRE.ReplaceAllString(keywords, " "))
	var candidates []struct {
		row  map[string]interface{}
		node StructureGraphNode
	}
	validTop := func(rows map[string]map[string]interface{}) []struct {
		row  map[string]interface{}
		node StructureGraphNode
	} {
		var out []struct {
			row  map[string]interface{}
			node StructureGraphNode
		}
		for _, row := range rows {
			if !rowHasEnabledSource(row, map[string]bool{}) {
				continue
			}
			if node := projectEntity(row); node != nil && strings.TrimSpace(graphStr(node["name"])) != "" {
				out = append(out, struct {
					row  map[string]interface{}
					node StructureGraphNode
				}{row, node})
			}
		}
		return out
	}
	if textQuery != "" {
		textExpr := &types.MatchTextExpr{
			Fields:       []string{"content_ltks^10", "content_sm_ltks"},
			MatchingText: textQuery,
			TopN:         graphKeywordCandidates,
			ExtraOptions: map[string]interface{}{"original_query": keywords},
		}
		topMap, _, err := graphRowSearch(ctx, tenantID, datasetID, topFields, baseEntityCond, nil, 0, graphKeywordCandidates, []interface{}{textExpr})
		if err != nil {
			return nil, nil, nil, err
		}
		for _, c := range validTop(topMap) {
			if nameMatchesQuery(c.node, textQuery) {
				candidates = append(candidates, c)
			}
		}
	}
	// Prefer detail entities over title-typed ancestors.
	detailCandidates := candidates[:0]
	for _, c := range candidates {
		if strings.ToLower(strings.TrimSpace(graphStr(c.node["type"]))) != "title" {
			detailCandidates = append(detailCandidates, c)
		}
	}
	if len(detailCandidates) > 0 {
		candidates = detailCandidates
	}

	// Semantic fallback via KNN.
	if len(candidates) == 0 {
		vec, err := embedQuery(ctx, tenantID, datasetID, keywords)
		if err != nil || len(vec) == 0 {
			return nil, nil, nil, nil
		}
		denseExpr := &types.MatchDenseExpr{
			VectorColumnName:  fmt.Sprintf("q_%d_vec", len(vec)),
			EmbeddingData:     vec,
			EmbeddingDataType: "float",
			DistanceType:      "cosine",
			TopN:              graphKeywordCandidates,
			ExtraOptions:      map[string]interface{}{"similarity": 0.3},
		}
		topMap, _, err := graphRowSearch(ctx, tenantID, datasetID, topFields, baseEntityCond, nil, 0, graphKeywordCandidates, []interface{}{denseExpr})
		if err != nil {
			return nil, nil, nil, err
		}
		candidates = validTop(topMap)
	}
	if len(candidates) == 0 {
		return nil, nil, nil, nil
	}

	scopeForTemplate := func(row map[string]interface{}) (map[string]interface{}, map[string]interface{}) {
		return resolveGraphBucket(row, templateMeta, documentID)
	}
	bucketMeta, scope := scopeForTemplate(candidates[0].row)
	bucketID := graphStr(bucketMeta["template_id"])

	matchedNodes := []StructureGraphNode{}
	for _, c := range candidates {
		cMeta, _ := scopeForTemplate(c.row)
		if graphStr(cMeta["template_id"]) == bucketID {
			matchedNodes = append(matchedNodes, c.node)
		}
	}
	if len(matchedNodes) == 0 {
		return nil, nil, nil, nil
	}
	structureKind := structureKindForBucket(graphStr(bucketMeta["kind"]))

	var relations []StructureGraphRelation
	seenRel := map[string]bool{}
	neighborNamesLower := map[string]bool{}
	matchedNames := map[string]bool{}
	for _, n := range matchedNodes {
		if name := strings.ToLower(strings.TrimSpace(graphStr(n["name"]))); name != "" {
			matchedNames[name] = true
		}
	}
	if structureKind != "tree" && structureKind != "page_index" {
		for _, matchedNode := range matchedNodes {
			matchedName := strings.TrimSpace(graphStr(matchedNode["name"]))
			terms := endpointTerms(matchedName)
			for _, field := range []string{"from_entity_kwd", "to_entity_kwd"} {
				cond := copyFilter(scope)
				cond["knowledge_graph_kwd"] = []string{"relation"}
				cond[field] = terms
				relMap, _, err := graphRowSearch(ctx, tenantID, datasetID, graphRelationFields, cond, nil, 0, graphExpansionCap, nil)
				if err != nil {
					return nil, nil, nil, err
				}
				for _, row := range relMap {
					if !rowHasEnabledSource(row, map[string]bool{}) {
						continue
					}
					if edge := projectRelation(row); edge != nil {
						key := graphStr(edge["from"]) + "\x00" + graphStr(edge["to"]) + "\x00" + graphStr(edge["type"])
						if seenRel[key] {
							continue
						}
						seenRel[key] = true
						relations = append(relations, edge)
						for _, endpoint := range []string{graphStr(edge["from"]), graphStr(edge["to"])} {
							if ep := strings.TrimSpace(endpoint); ep != "" && !matchedNames[strings.ToLower(ep)] {
								neighborNamesLower[strings.ToLower(ep)] = true
							}
						}
					}
					if len(relations) >= graphExpansionCap {
						break
					}
				}
				if len(relations) >= graphExpansionCap {
					break
				}
			}
			if len(relations) >= graphExpansionCap {
				break
			}
		}
	}

	// Ancestor walk for tree/page_index.
	if (structureKind == "tree" || structureKind == "page_index") && len(relations) < graphExpansionCap {
		ancestorFrontier := map[string]bool{}
		seenAncestors := map[string]bool{}
		for n := range matchedNames {
			ancestorFrontier[n] = true
			seenAncestors[n] = true
		}
		for len(ancestorFrontier) > 0 && len(relations) < graphExpansionCap {
			nextFrontier := map[string]bool{}
			cond := copyFilter(scope)
			cond["knowledge_graph_kwd"] = []string{"relation"}
			cond["to_entity_kwd"] = sortedKeys(ancestorFrontier)
			relMap, _, err := graphRowSearch(ctx, tenantID, datasetID, graphRelationFields, cond, nil, 0, graphExpansionCap-len(relations), nil)
			if err != nil {
				return nil, nil, nil, err
			}
			for _, row := range relMap {
				if !rowHasEnabledSource(row, map[string]bool{}) {
					continue
				}
				if edge := projectRelation(row); edge != nil {
					key := graphStr(edge["from"]) + "\x00" + graphStr(edge["to"]) + "\x00" + graphStr(edge["type"])
					if seenRel[key] {
						continue
					}
					seenRel[key] = true
					relations = append(relations, edge)
					parent := strings.ToLower(strings.TrimSpace(graphStr(edge["from"])))
					if parent != "" && !seenAncestors[parent] {
						seenAncestors[parent] = true
						nextFrontier[parent] = true
					}
					if len(relations) >= graphExpansionCap {
						break
					}
				}
			}
			ancestorFrontier = nextFrontier
			for n := range nextFrontier {
				neighborNamesLower[n] = true
			}
		}
	}

	entities := append([]StructureGraphNode{}, matchedNodes...)
	if len(neighborNamesLower) > 0 {
		cond := copyFilter(scope)
		cond["knowledge_graph_kwd"] = []string{"entity"}
		cond["name_kwd"] = sortedKeys(neighborNamesLower)
		nbMap, _, err := graphRowSearch(ctx, tenantID, datasetID, graphEntityFields, cond, nil, 0, graphExpansionCap, nil)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, row := range nbMap {
			if !rowHasEnabledSource(row, map[string]bool{}) {
				continue
			}
			if n := projectEntity(row); n != nil {
				entities = append(entities, n)
			}
		}
	}
	entities = dedupEntities(entities)
	if structureKind == "tree" || structureKind == "page_index" {
		entityNames := map[string]bool{}
		for _, e := range entities {
			if n := strings.ToLower(strings.TrimSpace(graphStr(e["name"]))); n != "" {
				entityNames[n] = true
			}
		}
		var filtered []StructureGraphRelation
		for _, r := range relations {
			if entityNames[strings.ToLower(strings.TrimSpace(graphStr(r["from"])))] && entityNames[strings.ToLower(strings.TrimSpace(graphStr(r["to"])))] {
				filtered = append(filtered, r)
			}
		}
		relations = filtered
	}
	return bucketMeta, entities, normalizeRelationEndpoints(entities, relations), nil
}

// embedQuery embeds a query via the nav embedder (best-effort). The embedder
// resolves the tenant's embedding model on demand; an empty result (or error) is
// treated as "no semantic candidates", matching Python's fallback behavior.
func embedQuery(ctx context.Context, tenantID, datasetID, query string) ([]float64, error) {
	embedder := NewNavEmbedder(NewModelProviderService(), "")
	vecs, err := embedder.Encode(ctx, tenantID, []string{query})
	if err != nil || len(vecs) == 0 || len(vecs[0]) == 0 {
		return nil, fmt.Errorf("embed query failed")
	}
	f := make([]float64, len(vecs[0]))
	for i, v := range vecs[0] {
		f[i] = float64(v)
	}
	return f, nil
}
