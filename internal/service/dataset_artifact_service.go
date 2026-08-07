//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
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

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/service/nav"
)

// Compile keyword constants used by the knowledge-compilation artifacts stored
// in the document engine.
const (
	CompileKwdWikiPage     = "wiki_page"
	CompileKwdWikiEntity   = "wiki_entity"
	CompileKwdWikiRelation = "wiki_relation"
	CompileKwdWikiAlter    = "wiki_alter"
	CompileKwdSkill        = "skill"
	CompileKwdSkillAll     = "skill_all"
	CompileKwdDatasetNav   = "dataset_nav"
	CompileKwdRaptorGraph  = "raptor_graph"

	CompileKwdStructure          = "structure"
	CompileKwdStructureIndex     = "structureIndex"
	CompileKwdStructureEntity    = "structureEntity"
	CompileKwdStructureRelation  = "structureRelation"
	CompileKwdStructureCommunity = "structureCommunity"

	FieldStructureIndexType = "structure_index_type"
	FieldStructureKind      = "structure_kind"
	FieldPageID             = "page_id"
	FieldGraphType          = "graph_type"
)

// DatasetArtifactService reads knowledge-compilation artifacts (wiki pages,
// graphs, structures, navigation, skills) from the document engine.
type DatasetArtifactService struct{}

// NewDatasetArtifactService creates a DatasetArtifactService.
func NewDatasetArtifactService() *DatasetArtifactService {
	return &DatasetArtifactService{}
}

// wikiIndexName returns the tenant document index name.
func wikiIndexName(tenantID string) string {
	return fmt.Sprintf("ragflow_%s", tenantID)
}

// searchCompiled runs a filtered search over the tenant's document index for
// the given dataset, returning the matching chunks.
func (s *DatasetArtifactService) searchCompiled(ctx context.Context, tenantID, datasetID string, filter map[string]interface{}, selectFields []string, offset, limit int, orderBy *types.OrderByExpr) ([]map[string]interface{}, int64, error) {
	docEngine := engine.Get()
	if docEngine == nil {
		return nil, 0, fmt.Errorf("document engine is not initialized")
	}
	// Copy caller filters first, then pin the dataset scope so kb_id always wins
	// even if a caller passes its own kb_id.
	merged := make(map[string]interface{}, len(filter)+1)
	for k, v := range filter {
		merged[k] = v
	}
	merged["kb_id"] = []string{datasetID}
	res, err := docEngine.Search(ctx, &types.SearchRequest{
		IndexNames:   []string{wikiIndexName(tenantID)},
		KbIDs:        []string{datasetID},
		Offset:       offset,
		Limit:        limit,
		SelectFields: selectFields,
		Filter:       merged,
		OrderBy:      orderBy,
	})
	if err != nil {
		return nil, 0, err
	}
	if res == nil {
		return nil, 0, nil
	}
	return res.Chunks, res.Total, nil
}

// intValue coerces an engine field value into an int, tolerating the numeric
// types a document engine may decode a number as (float64, int, int64,
// json.Number) as well as a string form and a list-valued form (first element
// wins). It returns 0 for anything else.
func intValue(v interface{}) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case int32:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	case json.Number:
		if iv, err := n.Int64(); err == nil {
			return int(iv)
		}
		if fv, err := n.Float64(); err == nil {
			return int(fv)
		}
	case string:
		var iv int
		if _, err := fmt.Sscanf(n, "%d", &iv); err == nil {
			return iv
		}
	case []interface{}:
		if len(n) > 0 {
			return intValue(n[0])
		}
	}
	return 0
}

// HasWiki reports whether the dataset has any compiled wiki artifact.
func (s *DatasetArtifactService) HasWiki(ctx context.Context, tenantID, datasetID string) (bool, error) {
	_, total, err := s.searchCompiled(ctx, tenantID, datasetID,
		map[string]interface{}{"compile_kwd": []string{CompileKwdWikiPage}}, nil, 0, 1, nil)
	if err != nil {
		return false, err
	}
	return total > 0, nil
}

// HasSkill reports whether the dataset has any compiled skill artifact.
func (s *DatasetArtifactService) HasSkill(ctx context.Context, tenantID, datasetID string) (bool, error) {
	_, total, err := s.searchCompiled(ctx, tenantID, datasetID,
		map[string]interface{}{"compile_kwd": []string{CompileKwdSkillAll}}, nil, 0, 1, nil)
	if err != nil {
		return false, err
	}
	return total > 0, nil
}

// WikiPageItem is a single wiki page summary returned by ListWikiPages.
type WikiPageItem struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	PageType string `json:"page_type"`
	Topic    string `json:"topic"`
	Summary  string `json:"summary"`
}

// ListWikiPages lists wiki pages for a dataset with optional page_type/topic
// filters and pagination.
func (s *DatasetArtifactService) ListWikiPages(ctx context.Context, tenantID, datasetID, pageType, topic string, page, pageSize int) ([]WikiPageItem, int64, error) {
	// Only surface the merged dataset-level pages. Each unique (page_type, slug)
	// can also have a per-document source row (available_int=0); without this
	// filter the same entity/concept would appear once per source doc. Python's
	// list_wiki_pages has no such duplication because its writer emits one row
	// per page, so mirror that by selecting the merged rows (available_int=1).
	filter := map[string]interface{}{
		"compile_kwd":   []string{CompileKwdWikiPage},
		"available_int": 1, // merged dataset-level rows only (see engine available_int handling)
	}
	if pageType != "" {
		filter["page_type_kwd"] = []string{pageType}
	}
	if topic != "" {
		filter["topic_kwd"] = []string{topic}
	}
	offset := (page - 1) * pageSize
	chunks, total, err := s.searchCompiled(ctx, tenantID, datasetID, filter,
		[]string{"slug_kwd", "title_kwd", "page_type_kwd", "topic_kwd", "outlinks_int", "summary_with_weight"},
		offset, pageSize, (&types.OrderByExpr{}).Desc("outlinks_int").Asc("title_kwd"))
	if err != nil {
		return nil, 0, err
	}
	items := make([]WikiPageItem, 0, len(chunks))
	for _, c := range chunks {
		pageType := firstStringValue(c["page_type_kwd"])
		// slug_kwd is stored as the full "<page_type>/<slug>" form (Python
		// contract); expose the bare slug to the frontend so it can be placed in
		// a single URL path segment (gin :slug does not match '/').
		bareSlug := firstStringValue(c["slug_kwd"])
		if pageType != "" {
			bareSlug = strings.TrimPrefix(bareSlug, pageType+"/")
		}
		items = append(items, WikiPageItem{
			Slug:     bareSlug,
			Title:    firstStringValue(c["title_kwd"]),
			PageType: pageType,
			Topic:    firstStringValue(c["topic_kwd"]),
			Summary:  firstStringValue(c["summary_with_weight"]),
		})
	}
	return items, total, nil
}

// WikiPageDetail is the full wiki page payload. The content field is exposed as
// content_md_rendered to match the frontend IArtifactPage contract (and Python's
// get_wiki_page), which renders it directly.
type WikiPageDetail struct {
	Slug           string   `json:"slug"`
	Title          string   `json:"title"`
	PageType       string   `json:"page_type"`
	Topic          string   `json:"topic"`
	ContentMd      string   `json:"content_md_rendered"`
	Summary        string   `json:"summary"`
	EntityNames    []string `json:"entity_names"`
	Outlinks       []string `json:"outlinks"`
	RelatedKbPages []string `json:"related_kb_pages"`
	SourceChunkIDs []string `json:"source_chunk_ids"`
	SourceDocIDs   []string `json:"source_doc_ids"`
}

// GetWikiPage returns a single wiki page by page_type and slug.
func (s *DatasetArtifactService) GetWikiPage(ctx context.Context, tenantID, datasetID, pageType, slug string) (*WikiPageDetail, error) {
	// Match Python's get_wiki_page contract (dataset_api_service.py): slug_kwd
	// is stored as the full "<page_type>/<slug>" form, and list_wiki_pages
	// returns the bare slug. Reconstruct the full form deterministically so the
	// filter matches the stored value exactly.
	slugKwd := pageType + "/" + slug
	filter := map[string]interface{}{
		"compile_kwd":   []string{CompileKwdWikiPage},
		"page_type_kwd": []string{pageType},
		"slug_kwd":      []string{slugKwd},
		"available_int": 1, // merged dataset-level page, not the per-doc source row
	}
	chunks, _, err := s.searchCompiled(ctx, tenantID, datasetID, filter,
		[]string{"slug_kwd", "title_kwd", "page_type_kwd", "topic_kwd", "md_with_weight",
			"content_with_weight", "summary_with_weight", "entity_names_kwd", "outlinks_kwd",
			"related_kb_pages_kwd", "source_chunk_ids", "source_doc_ids"},
		0, 1, nil)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	c := chunks[0]
	// Python stores the page body in md_with_weight (incremental writer), falling
	// back to content_with_weight for legacy rows; mirror that here.
	content := firstStringValue(c["md_with_weight"])
	if content == "" {
		content = firstStringValue(c["content_with_weight"])
	}
	// slug_kwd is the full "<page_type>/<slug>" form; expose the bare slug so a
	// client can pass it straight back to GetWikiPage/UpdateWikiPage without the
	// "<page_type>/" prefix being doubled (matches ListWikiPages).
	detailPageType := firstStringValue(c["page_type_kwd"])
	detailSlug := firstStringValue(c["slug_kwd"])
	if detailPageType != "" {
		detailSlug = strings.TrimPrefix(detailSlug, detailPageType+"/")
	}
	detail := &WikiPageDetail{
		Slug:           detailSlug,
		Title:          firstStringValue(c["title_kwd"]),
		PageType:       detailPageType,
		Topic:          firstStringValue(c["topic_kwd"]),
		ContentMd:      content,
		Summary:        firstStringValue(c["summary_with_weight"]),
		EntityNames:    toStringSlice(c["entity_names_kwd"]),
		Outlinks:       toStringSlice(c["outlinks_kwd"]),
		RelatedKbPages: toStringSlice(c["related_kb_pages_kwd"]),
		SourceChunkIDs: toStringSlice(c["source_chunk_ids"]),
		SourceDocIDs:   toStringSlice(c["source_doc_ids"]),
	}
	return detail, nil
}

// UpdateWikiPage performs a partial field update of a wiki page's content,
// title and outlinks through the document engine, then returns the refreshed
// page.
func (s *DatasetArtifactService) UpdateWikiPage(ctx context.Context, tenantID, datasetID, pageType, slug, contentMd, title string, outlinks []string) (*WikiPageDetail, error) {
	docEngine := engine.Get()
	if docEngine == nil {
		return nil, fmt.Errorf("document engine is not initialized")
	}
	// Python contract: slug_kwd is stored as "page_type/slug"; reconstruct it
	// deterministically from the bare slug (see GetWikiPage).
	slugKwd := pageType + "/" + slug
	filter := map[string]interface{}{
		"compile_kwd":   []string{CompileKwdWikiPage},
		"page_type_kwd": []string{pageType},
		"slug_kwd":      []string{slugKwd},
		"available_int": 1, // merged dataset-level page only
	}
	chunks, _, err := s.searchCompiled(ctx, tenantID, datasetID, filter, []string{"id"}, 0, 1, nil)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	id, ok := chunks[0]["id"].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("wiki page chunk has no id")
	}
	update := map[string]interface{}{}
	if contentMd != "" {
		// GetWikiPage prefers md_with_weight and falls back to content_with_weight,
		// so write both to keep the edit readable regardless of the row's writer.
		update["md_with_weight"] = contentMd
		update["content_with_weight"] = contentMd
	}
	if title != "" {
		update["title_kwd"] = title
	}
	if len(outlinks) > 0 {
		update["outlinks_kwd"] = outlinks
	}
	if len(update) > 0 {
		cond := map[string]interface{}{"id": id, "kb_id": datasetID}
		if err := docEngine.UpdateChunks(ctx, cond, update, wikiIndexName(tenantID), datasetID); err != nil {
			return nil, err
		}
	}
	return s.GetWikiPage(ctx, tenantID, datasetID, pageType, slug)
}

// WikiTopicItem is a single topic entry returned by ListWikiTopics.
type WikiTopicItem struct {
	Topic     string `json:"topic"`
	Title     string `json:"title"`
	Slug      string `json:"slug"`
	PageCount int    `json:"page_count"`
}

// ListWikiTopics aggregates wiki topics for a dataset.
func (s *DatasetArtifactService) ListWikiTopics(ctx context.Context, tenantID, datasetID string) ([]WikiTopicItem, int64, error) {
	filter := map[string]interface{}{
		"compile_kwd":   []string{CompileKwdWikiPage},
		"page_type_kwd": []string{"concept", "entity"},
		"available_int": 1, // count only merged pages, not per-doc source rows
	}
	chunks, _, err := s.searchCompiled(ctx, tenantID, datasetID, filter,
		[]string{"topic_kwd", "title_kwd", "slug_kwd", "page_type_kwd"}, 0, 1000, nil)
	if err != nil {
		return nil, 0, err
	}
	counts := map[string]int{}
	metas := map[string]WikiTopicItem{}
	for _, c := range chunks {
		t := firstStringValue(c["topic_kwd"])
		if t == "" {
			continue
		}
		pageType := firstStringValue(c["page_type_kwd"])
		bareSlug := firstStringValue(c["slug_kwd"])
		if pageType != "" {
			bareSlug = strings.TrimPrefix(bareSlug, pageType+"/")
		}
		counts[t]++
		if _, ok := metas[t]; !ok {
			metas[t] = WikiTopicItem{
				Topic: t,
				Title: firstStringValue(c["title_kwd"]),
				Slug:  bareSlug,
			}
		}
	}
	items := make([]WikiTopicItem, 0, len(metas))
	for t, it := range metas {
		it.PageCount = counts[t]
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Topic < items[j].Topic })
	return items, int64(len(items)), nil
}

// WikiGraph is the entity/relation graph for a dataset's wiki artifacts.
type WikiGraph struct {
	Entities  []WikiGraphEntity   `json:"entities"`
	Relations []WikiGraphRelation `json:"relations"`
}

// WikiGraphEntity is a single wiki graph entity.
type WikiGraphEntity struct {
	Slug           string   `json:"slug"`
	Name           string   `json:"name"`
	Aliases        []string `json:"aliases"`
	Description    string   `json:"description"`
	Type           string   `json:"type"`
	Weight         int      `json:"weight"`
	SourceChunkIDs []string `json:"source_chunk_ids"`
}

// WikiGraphRelation is a single wiki graph relation.
type WikiGraphRelation struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// GetWikiGraph returns the wiki entity/relation graph for a dataset.
func (s *DatasetArtifactService) GetWikiGraph(ctx context.Context, tenantID, datasetID string) (*WikiGraph, error) {
	entityChunks, _, err := s.searchCompiled(ctx, tenantID, datasetID,
		map[string]interface{}{"compile_kwd": []string{CompileKwdWikiEntity}},
		[]string{"slug_kwd", "title_kwd", "aliases_kwd", "description_with_weight", "entity_type_kwd", "weight_int", "source_chunk_ids"},
		0, 1000, (&types.OrderByExpr{}).Desc("weight_int"))
	if err != nil {
		return nil, err
	}
	fromKwds := make([]string, 0, len(entityChunks))
	for _, c := range entityChunks {
		fromKwds = append(fromKwds, firstStringValue(c["slug_kwd"]))
	}
	graph := &WikiGraph{Entities: []WikiGraphEntity{}, Relations: []WikiGraphRelation{}}
	for _, c := range entityChunks {
		w := intValue(c["weight_int"])
		graph.Entities = append(graph.Entities, WikiGraphEntity{
			Slug:           firstStringValue(c["slug_kwd"]),
			Name:           firstStringValue(c["title_kwd"]),
			Aliases:        toStringSlice(c["aliases_kwd"]),
			Description:    firstStringValue(c["description_with_weight"]),
			Type:           firstStringValue(c["entity_type_kwd"]),
			Weight:         w,
			SourceChunkIDs: toStringSlice(c["source_chunk_ids"]),
		})
	}
	if len(fromKwds) > 0 {
		relChunks, _, err := s.searchCompiled(ctx, tenantID, datasetID,
			map[string]interface{}{"compile_kwd": []string{CompileKwdWikiRelation}, "from_kwd": fromKwds},
			[]string{"from_kwd", "to_kwd"}, 0, 10000, nil)
		if err != nil {
			return nil, err
		}
		for _, c := range relChunks {
			graph.Relations = append(graph.Relations, WikiGraphRelation{
				From: firstStringValue(c["from_kwd"]),
				To:   firstStringValue(c["to_kwd"]),
			})
		}
	}
	return graph, nil
}

// WikiAlteration is the alteration summary for a dataset's wiki artifacts.
type WikiAlteration struct {
	Removed             int      `json:"removed"`
	NewlyUploaded       int      `json:"newly_uploaded"`
	RemovedDocIDs       []string `json:"removed_doc_ids"`
	NewlyUploadedDocIDs []string `json:"newly_uploaded_doc_ids"`
	InvolvedDocIDs      []string `json:"involved_doc_ids"`
	EligibleDocIDs      []string `json:"eligible_doc_ids"`
}

// GetWikiAlteration returns the wiki alteration summary for a dataset.
func (s *DatasetArtifactService) GetWikiAlteration(ctx context.Context, tenantID, datasetID string) (*WikiAlteration, error) {
	chunks, _, err := s.searchCompiled(ctx, tenantID, datasetID,
		map[string]interface{}{"compile_kwd": []string{CompileKwdWikiPage}},
		[]string{"source_doc_ids"}, 0, 10000, nil)
	if err != nil {
		return nil, err
	}
	involved := map[string]struct{}{}
	for _, c := range chunks {
		for _, d := range toStringSlice(c["source_doc_ids"]) {
			involved[d] = struct{}{}
		}
	}
	ids := make([]string, 0, len(involved))
	for d := range involved {
		ids = append(ids, d)
	}
	sort.Strings(ids)
	return &WikiAlteration{
		Removed:             0,
		NewlyUploaded:       0,
		RemovedDocIDs:       []string{},
		NewlyUploadedDocIDs: []string{},
		InvolvedDocIDs:      ids,
		EligibleDocIDs:      ids,
	}, nil
}

// ClearWiki deletes all wiki artifacts for a dataset.
func (s *DatasetArtifactService) ClearWiki(ctx context.Context, tenantID, datasetID string) (map[string]int, error) {
	docEngine := engine.Get()
	if docEngine == nil {
		return nil, fmt.Errorf("document engine is not initialized")
	}
	kwds := []string{CompileKwdWikiPage, CompileKwdWikiEntity, CompileKwdWikiRelation, CompileKwdWikiAlter}
	deleted := map[string]int{}
	for _, kwd := range kwds {
		chunks, _, err := s.searchCompiled(ctx, tenantID, datasetID,
			map[string]interface{}{"compile_kwd": []string{kwd}}, []string{"id"}, 0, 10000, nil)
		if err != nil {
			return nil, err
		}
		if len(chunks) == 0 {
			deleted[kwd] = 0
			continue
		}
		ids := make([]string, 0, len(chunks))
		for _, c := range chunks {
			if id, ok := c["id"].(string); ok {
				ids = append(ids, id)
			}
		}
		cond := map[string]interface{}{"id": ids, "kb_id": datasetID}
		if _, err := docEngine.DeleteChunks(ctx, cond, wikiIndexName(tenantID), datasetID); err != nil {
			return nil, err
		}
		deleted[kwd] = len(ids)
	}
	return deleted, nil
}

// StructureItem is a single compiled structure entry for a dataset.
type StructureItem struct {
	PageID             string `json:"page_id"`
	StructureKind      string `json:"structure_kind"`
	StructureIndexType string `json:"structure_index_type"`
	Data               string `json:"data"`
}

// ListStructures returns the compiled structures of a dataset, filtered by
// optional structure_kind and structure_index_type.
func (s *DatasetArtifactService) ListStructures(ctx context.Context, tenantID, datasetID, structureKind, structureIndexType string) ([]StructureItem, int64, error) {
	filter := map[string]interface{}{"compile_kwd": []string{CompileKwdStructure}}
	if structureKind != "" {
		filter[FieldStructureKind] = []string{structureKind}
	}
	if structureIndexType != "" {
		filter[FieldStructureIndexType] = []string{structureIndexType}
	}
	chunks, total, err := s.searchCompiled(ctx, tenantID, datasetID, filter,
		[]string{FieldPageID, FieldStructureKind, FieldStructureIndexType, "content_with_weight"}, 0, 10000, nil)
	if err != nil {
		return nil, 0, err
	}
	items := make([]StructureItem, 0, len(chunks))
	for _, c := range chunks {
		items = append(items, StructureItem{
			PageID:             firstStringValue(c[FieldPageID]),
			StructureKind:      firstStringValue(c[FieldStructureKind]),
			StructureIndexType: firstStringValue(c[FieldStructureIndexType]),
			Data:               firstStringValue(c["content_with_weight"]),
		})
	}
	return items, total, nil
}

// DeleteStructures deletes the compiled structures of a dataset, optionally
// scoped by structure_kind and structure_index_type.
func (s *DatasetArtifactService) DeleteStructures(ctx context.Context, tenantID, datasetID, structureKind, structureIndexType string) (int, error) {
	docEngine := engine.Get()
	if docEngine == nil {
		return 0, fmt.Errorf("document engine is not initialized")
	}
	filter := map[string]interface{}{"compile_kwd": []string{CompileKwdStructure}}
	if structureKind != "" {
		filter[FieldStructureKind] = []string{structureKind}
	}
	if structureIndexType != "" {
		filter[FieldStructureIndexType] = []string{structureIndexType}
	}
	chunks, _, err := s.searchCompiled(ctx, tenantID, datasetID, filter, []string{"id"}, 0, 10000, nil)
	if err != nil {
		return 0, err
	}
	if len(chunks) == 0 {
		return 0, nil
	}
	ids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if id, ok := c["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	cond := map[string]interface{}{"id": ids, "kb_id": datasetID}
	if _, err := docEngine.DeleteChunks(ctx, cond, wikiIndexName(tenantID), datasetID); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// DocGraphItem is a single node/edge entry in a document's structure graph.
type DocGraphItem struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	SourceID string `json:"source_id"`
}

// GetDocumentGraph returns the structure graph of a single document.
func (s *DatasetArtifactService) GetDocumentGraph(ctx context.Context, tenantID, datasetID, documentID, graphType string) ([]DocGraphItem, int64, error) {
	filter := map[string]interface{}{
		"doc_id":             []string{documentID},
		"compiled_graph_kwd": []string{"graph"},
	}
	if graphType != "" {
		filter[FieldGraphType] = []string{graphType}
	}
	chunks, total, err := s.searchCompiled(ctx, tenantID, datasetID, filter,
		[]string{"id", "content_with_weight", "source_id"}, 0, 10000, nil)
	if err != nil {
		return nil, 0, err
	}
	items := make([]DocGraphItem, 0, len(chunks))
	for _, c := range chunks {
		items = append(items, DocGraphItem{
			ID:       firstStringValue(c["id"]),
			Content:  firstStringValue(c["content_with_weight"]),
			SourceID: firstStringValue(c["source_id"]),
		})
	}
	return items, total, nil
}

// DeleteDocumentGraph deletes the structure graph of a single document.
func (s *DatasetArtifactService) DeleteDocumentGraph(ctx context.Context, tenantID, datasetID, documentID string) (int, error) {
	docEngine := engine.Get()
	if docEngine == nil {
		return 0, fmt.Errorf("document engine is not initialized")
	}
	filter := map[string]interface{}{
		"doc_id":             []string{documentID},
		"compiled_graph_kwd": []string{"graph"},
	}
	chunks, _, err := s.searchCompiled(ctx, tenantID, datasetID, filter, []string{"id"}, 0, 10000, nil)
	if err != nil {
		return 0, err
	}
	if len(chunks) == 0 {
		return 0, nil
	}
	ids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if id, ok := c["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	cond := map[string]interface{}{"id": ids, "kb_id": datasetID}
	if _, err := docEngine.DeleteChunks(ctx, cond, wikiIndexName(tenantID), datasetID); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// NavigationItem is a single navigation cluster (REST response shape, kept
// stable for frontend compatibility).
type NavigationItem struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

// NavChildItem is a single child entry under a navigation cluster (REST
// response shape, kept stable for frontend compatibility).
type NavChildItem struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

// ListNavClusters returns the navigation clusters of a dataset. It is DEPRECATED
// and now delegates to the ES-backed NavService (internal/service datasetnav):
// the previous implementation queried nav_cluster_kwd/count_int fields that
// Python never writes, so it could never read the real nav tree. Do not add
// field-level patches here — route everything through NavService.
func (s *DatasetArtifactService) ListNavClusters(ctx context.Context, tenantID, datasetID string) ([]NavigationItem, int64, error) {
	ns := nav.GetNavService()
	if ns == nil {
		return nil, 0, fmt.Errorf("datasetnav: NavService not initialized (SetNavService must be called at bootstrap)")
	}
	nodes, total, err := ns.ListClusters(ctx, tenantID, datasetID, 0, 10000)
	if err != nil {
		return nil, 0, err
	}
	items := make([]NavigationItem, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, NavigationItem{Name: n.Name, Title: n.Description, Count: n.DocCount})
	}
	return items, total, nil
}

// ListNavChildren returns the children of a navigation cluster. DEPRECATED —
// delegates to NavService.ListChildren.
func (s *DatasetArtifactService) ListNavChildren(ctx context.Context, tenantID, datasetID, name string) ([]NavChildItem, int64, error) {
	ns := nav.GetNavService()
	if ns == nil {
		return nil, 0, fmt.Errorf("datasetnav: NavService not initialized (SetNavService must be called at bootstrap)")
	}
	nodes, total, err := ns.ListChildren(ctx, tenantID, datasetID, name, 0, 10000)
	if err != nil {
		return nil, 0, err
	}
	items := make([]NavChildItem, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, NavChildItem{Name: n.Name, Title: n.Description, Count: n.DocCount})
	}
	return items, total, nil
}

// DeleteNav removes the direct nav_doc children of every root cluster of a
// dataset. DEPRECATED — this is the minimal-loop approximation of Python
// delete_nav: it drains only the immediate nav_doc rows under root clusters and
// does NOT implement Python's full subtree traversal or empty-cluster cascade
// cleanup. Prefer the NavService (future work) for a complete delete. Returns
// the number of nav_doc rows removed.
func (s *DatasetArtifactService) DeleteNav(ctx context.Context, tenantID, datasetID string) (int, error) {
	ns := nav.GetNavService()
	if ns == nil {
		return 0, fmt.Errorf("datasetnav: NavService not initialized (SetNavService must be called at bootstrap)")
	}
	clusters, _, err := ns.ListClusters(ctx, tenantID, datasetID, 0, 10000)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, c := range clusters {
		children, _, err := ns.ListChildren(ctx, tenantID, datasetID, c.Name, 0, 10000)
		if err != nil {
			continue
		}
		for _, ch := range children {
			if ch.DocID != "" {
				if err := ns.RemoveDoc(ctx, tenantID, datasetID, ch.DocID); err != nil {
					return deleted, err
				}
				deleted++
			}
		}
	}
	return deleted, nil
}

// DeleteNavNode deletes the direct nav_doc children of a named cluster.
// DEPRECATED — the minimal loop only drains immediate doc children (returns the
// count); it does NOT delete sub-clusters recursively nor perform Python's
// empty-cluster cascade. A full tree-node delete is future NavService work.
func (s *DatasetArtifactService) DeleteNavNode(ctx context.Context, tenantID, datasetID, name string) (int, error) {
	ns := nav.GetNavService()
	if ns == nil {
		return 0, fmt.Errorf("datasetnav: NavService not initialized (SetNavService must be called at bootstrap)")
	}
	// Minimal loop has no per-node delete; drain direct children's docs.
	children, _, err := ns.ListChildren(ctx, tenantID, datasetID, name, 0, 10000)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, ch := range children {
		if ch.DocID != "" {
			if err := ns.RemoveDoc(ctx, tenantID, datasetID, ch.DocID); err != nil {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}

// SkillTreeItem is a single skill-tree page summary.
type SkillTreeItem struct {
	Kwd      string `json:"kwd"`
	Title    string `json:"title"`
	PageType string `json:"page_type"`
	Outlinks int    `json:"outlinks"`
	Inlinks  int    `json:"inlinks"`
}

// GetSkillTree returns the skill tree of a dataset.
func (s *DatasetArtifactService) GetSkillTree(ctx context.Context, tenantID, datasetID, kwd string) ([]SkillTreeItem, int64, error) {
	filter := map[string]interface{}{"compile_kwd": []string{CompileKwdSkillAll}}
	if kwd != "" {
		filter["kwd"] = []string{kwd}
	}
	chunks, total, err := s.searchCompiled(ctx, tenantID, datasetID, filter,
		[]string{"kwd", "title_kwd", "page_type_kwd", "outlinks_int", "inlinks_int"}, 0, 10000, nil)
	if err != nil {
		return nil, 0, err
	}
	items := make([]SkillTreeItem, 0, len(chunks))
	for _, c := range chunks {
		outlinks, inlinks := intValue(c["outlinks_int"]), intValue(c["inlinks_int"])
		items = append(items, SkillTreeItem{
			Kwd:      firstStringValue(c["kwd"]),
			Title:    firstStringValue(c["title_kwd"]),
			PageType: firstStringValue(c["page_type_kwd"]),
			Outlinks: outlinks,
			Inlinks:  inlinks,
		})
	}
	return items, total, nil
}

// SkillPageDetail is the full skill page payload.
type SkillPageDetail struct {
	Kwd       string   `json:"kwd"`
	Title     string   `json:"title"`
	ContentMd string   `json:"content_md"`
	Outlinks  []string `json:"outlinks"`
	Inlinks   []string `json:"inlinks"`
}

// GetSkillPage returns a single skill page by keyword.
func (s *DatasetArtifactService) GetSkillPage(ctx context.Context, tenantID, datasetID, kwd string) (*SkillPageDetail, error) {
	chunks, _, err := s.searchCompiled(ctx, tenantID, datasetID,
		map[string]interface{}{"compile_kwd": []string{CompileKwdSkillAll}, "kwd": []string{kwd}},
		[]string{"kwd", "title_kwd", "content_with_weight", "outlinks_kwd", "inlinks_kwd"}, 0, 1, nil)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	c := chunks[0]
	return &SkillPageDetail{
		Kwd:       firstStringValue(c["kwd"]),
		Title:     firstStringValue(c["title_kwd"]),
		ContentMd: firstStringValue(c["content_with_weight"]),
		Outlinks:  toStringSlice(c["outlinks_kwd"]),
		Inlinks:   toStringSlice(c["inlinks_kwd"]),
	}, nil
}

// DeleteSkills deletes skills of a dataset, optionally scoped by keyword.
func (s *DatasetArtifactService) DeleteSkills(ctx context.Context, tenantID, datasetID, kwd string) (int, error) {
	docEngine := engine.Get()
	if docEngine == nil {
		return 0, fmt.Errorf("document engine is not initialized")
	}
	filter := map[string]interface{}{"compile_kwd": []string{CompileKwdSkillAll}}
	if kwd != "" {
		filter["kwd"] = []string{kwd}
	}
	chunks, _, err := s.searchCompiled(ctx, tenantID, datasetID, filter, []string{"id"}, 0, 10000, nil)
	if err != nil {
		return 0, err
	}
	if len(chunks) == 0 {
		return 0, nil
	}
	ids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if id, ok := c["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	cond := map[string]interface{}{"id": ids, "kb_id": datasetID}
	if _, err := docEngine.DeleteChunks(ctx, cond, wikiIndexName(tenantID), datasetID); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// firstStringValue returns the first string in a possibly-list ES field value.
func firstStringValue(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []interface{}:
		if len(t) > 0 {
			if s, ok := t[0].(string); ok {
				return s
			}
		}
	case []string:
		if len(t) > 0 {
			return t[0]
		}
	}
	return ""
}

// toStringSlice normalizes an ES field value (string or list) to []string.
func toStringSlice(v interface{}) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return []string{}
		}
		return []string{t}
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	}
	return []string{}
}
