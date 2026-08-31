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

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
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
		filter["topic_kwd"] = []string{kccommon.NormalizeWikiTopicPath(topic)}
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
			Topic:    kccommon.NormalizeWikiTopicPath(firstStringValue(c["topic_kwd"])),
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
		Topic:          kccommon.NormalizeWikiTopicPath(firstStringValue(c["topic_kwd"])),
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

// ListWikiTopics aggregates materialized Wiki topic paths for a dataset. Topic
// is the complete path and Title is its leaf segment; the frontend may derive a
// navigation tree by splitting Topic on '/'.
func (s *DatasetArtifactService) ListWikiTopics(ctx context.Context, tenantID, datasetID string) ([]WikiTopicItem, int64, error) {
	filter := map[string]interface{}{
		"compile_kwd":   []string{CompileKwdWikiPage},
		"page_type_kwd": []string{"concept", "entity", "topic"},
		"available_int": 1, // count only merged pages, not per-doc source rows
	}
	const batchSize = 1000
	fields := []string{"topic_kwd"}
	chunks := make([]map[string]interface{}, 0, batchSize)
	for offset := 0; ; offset += batchSize {
		batch, total, err := s.searchCompiled(ctx, tenantID, datasetID, filter, fields, offset, batchSize, nil)
		if err != nil {
			return nil, 0, err
		}
		chunks = append(chunks, batch...)
		if len(batch) == 0 || int64(len(chunks)) >= total {
			break
		}
	}
	items := aggregateWikiTopicItems(chunks)
	// Sort topics by a deterministic rule. Plain UTF-8 byte order is chaotic for
	// CJK (it sorts by Unicode code point, unrelated to pinyin/stroke). We use a
	// CLDR-based collator (golang.org/x/text/collate) with the Chinese locale,
	// which orders Chinese by pinyin and handles Latin/digits/other scripts
	// correctly, case-insensitively, without assuming topics are all-Chinese.
	sort.Slice(items, func(i, j int) bool {
		return wikiTopicCollator.CompareString(items[i].Topic, items[j].Topic) < 0
	})
	return items, int64(len(items)), nil
}

func aggregateWikiTopicItems(chunks []map[string]interface{}) []WikiTopicItem {
	type aggregate struct {
		topic string
		count int
	}
	byKey := make(map[string]*aggregate)
	for _, c := range chunks {
		t := kccommon.NormalizeWikiTopicPath(firstStringValue(c["topic_kwd"]))
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		item := byKey[key]
		if item == nil {
			item = &aggregate{topic: t}
			byKey[key] = item
		}
		item.count++
	}
	items := make([]WikiTopicItem, 0, len(byKey))
	for _, aggregate := range byKey {
		items = append(items, WikiTopicItem{
			Topic:     aggregate.topic,
			Title:     kccommon.WikiTopicLeaf(aggregate.topic),
			Slug:      aggregate.topic,
			PageCount: aggregate.count,
		})
	}
	return items
}

// wikiTopicCollator is a process-wide collator for wiki topics. language.Chinese
// selects the CLDR zh collation (pinyin-based for Han), while Latin/digits and
// other scripts sort naturally, case-insensitively. It is safe for concurrent
// use after construction.
var wikiTopicCollator = collate.New(language.Chinese)

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
		map[string]interface{}{"compile_kwd": []string{CompileKwdWikiEntity}, "available_int": 1},
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
		// Match the GetWikiPage / ListWikiPages contract: expose the bare slug
		// and the pure page_type (no "wiki_" prefix, no "<page_type>/" prefix)
		// so the frontend can build artifact/<page_type>/<slug> links that
		// round-trip. entity_type_kwd stores "wiki_" + page_type (e.g.
		// "wiki_topic"); strip the prefix. slug_kwd stores the full
		// "<page_type>/<slug>" form; preserve nested slug segments.
		fullSlug := firstStringValue(c["slug_kwd"])
		bareSlug := fullSlug
		pageType := strings.TrimPrefix(firstStringValue(c["entity_type_kwd"]), "wiki_")
		if idx := strings.IndexByte(bareSlug, '/'); idx >= 0 {
			pageType = bareSlug[:idx]
			bareSlug = bareSlug[idx+1:]
		}
		graph.Entities = append(graph.Entities, WikiGraphEntity{
			Slug:           bareSlug,
			Name:           firstStringValue(c["title_kwd"]),
			Aliases:        toStringSlice(c["aliases_kwd"]),
			Description:    firstStringValue(c["description_with_weight"]),
			Type:           pageType,
			Weight:         w,
			SourceChunkIDs: toStringSlice(c["source_chunk_ids"]),
		})
	}
	if len(fromKwds) > 0 {
		relChunks, _, err := s.searchCompiled(ctx, tenantID, datasetID,
			map[string]interface{}{"compile_kwd": []string{CompileKwdWikiRelation}, "available_int": 1, "from_kwd": fromKwds},
			[]string{"from_kwd", "to_kwd"}, 0, 10000, nil)
		if err != nil {
			return nil, err
		}
		for _, c := range relChunks {
			// Relations reference endpoints by their full "<page_type>/<slug>"
			// form in ES; expose them as bare slugs so the frontend's
			// nodesBySlug (keyed on the entity's bare slug) can resolve edges.
			graph.Relations = append(graph.Relations, WikiGraphRelation{
				From: bareWikiSlug(firstStringValue(c["from_kwd"])),
				To:   bareWikiSlug(firstStringValue(c["to_kwd"])),
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
	involved := map[string]struct{}{}
	for offset := 0; ; offset += 1000 {
		chunks, total, err := s.searchCompiled(ctx, tenantID, datasetID,
			map[string]interface{}{"compile_kwd": []string{CompileKwdWikiPage}},
			[]string{"source_doc_ids"}, offset, 1000, nil)
		if err != nil {
			return nil, err
		}
		for _, c := range chunks {
			for _, d := range toStringSlice(c["source_doc_ids"]) {
				if d != "" {
					involved[d] = struct{}{}
				}
			}
		}
		if len(chunks) == 0 || int64(offset+len(chunks)) >= total {
			break
		}
	}
	// The database is the source of truth for the current document set. The
	// previous implementation returned the source_doc_ids from the compiled
	// pages as both sides of the comparison, which made deletion impossible to
	// observe. In particular, a deleted document remains in source_doc_ids until
	// the dataset-level consumer removes the old page.
	var documents []entity.Document
	if err := dao.DB.WithContext(ctx).Where("kb_id = ?", datasetID).Find(&documents).Error; err != nil {
		return nil, fmt.Errorf("list dataset documents for wiki alteration: %w", err)
	}
	eligible := make(map[string]struct{}, len(documents))
	for i := range documents {
		if documents[i].Status != nil && *documents[i].Status == "0" {
			continue
		}
		pipelineID := ""
		if documents[i].PipelineID != nil {
			pipelineID = *documents[i].PipelineID
		}
		ok, err := s.documentHasWikiTemplate(ctx, tenantID, documents[i].ParserConfig, pipelineID)
		if err != nil {
			return nil, err
		}
		if ok {
			eligible[documents[i].ID] = struct{}{}
		}
	}

	removedIDs := setDifference(involved, eligible)
	newlyUploadedIDs := setDifference(eligible, involved)
	return &WikiAlteration{
		Removed:             len(removedIDs),
		NewlyUploaded:       len(newlyUploadedIDs),
		RemovedDocIDs:       removedIDs,
		NewlyUploadedDocIDs: newlyUploadedIDs,
		InvolvedDocIDs:      sortedSetKeys(involved),
		EligibleDocIDs:      sortedSetKeys(eligible),
	}, nil
}

// documentHasWikiTemplate reports whether a document's saved pipeline config
// contains a valid wiki compiler template. A document is eligible only when it
// is configured for wiki compilation; treating every document in the dataset
// as eligible would hide both template removal and document deletion.
func (s *DatasetArtifactService) documentHasWikiTemplate(ctx context.Context, tenantID string, config entity.JSONMap, pipelineID string) (bool, error) {
	ok, err := s.valueHasWikiTemplate(ctx, tenantID, config)
	if err != nil || ok {
		return ok, err
	}
	if pipelineID == "" {
		return false, nil
	}
	var canvas entity.UserCanvas
	if err := dao.DB.WithContext(ctx).Where("id = ?", pipelineID).First(&canvas).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, fmt.Errorf("load pipeline %q for wiki alteration: %w", pipelineID, err)
	}
	return s.valueHasWikiTemplate(ctx, tenantID, canvas.DSL)
}

// valueHasWikiTemplate recursively inspects persisted parser/pipeline JSON.
// Pipeline DSLs and document parser configs use different nesting layouts, so
// looking only at a single top-level Compiler key misses pipeline documents.
func (s *DatasetArtifactService) valueHasWikiTemplate(ctx context.Context, tenantID string, value interface{}) (bool, error) {
	if items, ok := value.([]interface{}); ok {
		for _, item := range items {
			if found, err := s.valueHasWikiTemplate(ctx, tenantID, item); err != nil || found {
				return found, err
			}
		}
		return false, nil
	}
	params, isMap := value.(map[string]interface{})
	if !isMap {
		if typed, ok := value.(entity.JSONMap); ok {
			params = map[string]interface{}(typed)
			isMap = true
		}
	}
	if !isMap {
		return false, nil
	}
	if id, ok := params["compilation_template_id"].(string); ok && id != "" {
		var template entity.CompilationTemplate
		if err := dao.DB.WithContext(ctx).
			Where("id = ? AND (tenant_id = ? OR tenant_id IS NULL OR tenant_id = '') AND status = ?", id, tenantID, string(entity.StatusValid)).
			First(&template).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return false, nil
			}
			return false, fmt.Errorf("load wiki compilation template %q: %w", id, err)
		}
		if templateKind(template) == "wiki" {
			return true, nil
		}
	}
	if groupID, ok := params["compilation_template_group_id"].(string); ok && groupID != "" {
		if found, err := s.groupHasWikiTemplate(ctx, tenantID, groupID); err != nil || found {
			return found, err
		}
	}
	if rawGroupIDs, ok := params["compilation_template_group_ids"]; ok {
		groupIDs := []string{}
		switch values := rawGroupIDs.(type) {
		case string:
			groupIDs = append(groupIDs, values)
		case []interface{}:
			for _, value := range values {
				if groupID, ok := value.(string); ok {
					groupIDs = append(groupIDs, groupID)
				}
			}
		case []string:
			groupIDs = values
		}
		for _, groupID := range groupIDs {
			if strings.TrimSpace(groupID) != "" {
				if found, err := s.groupHasWikiTemplate(ctx, tenantID, groupID); err != nil || found {
					return found, err
				}
			}
		}
	}
	for _, child := range params {
		if found, err := s.valueHasWikiTemplate(ctx, tenantID, child); err != nil || found {
			return found, err
		}
	}
	return false, nil
}

func (s *DatasetArtifactService) groupHasWikiTemplate(ctx context.Context, tenantID, groupID string) (bool, error) {
	var group entity.CompilationTemplateGroup
	if err := dao.DB.WithContext(ctx).
		Where("id = ? AND (tenant_id = ? OR tenant_id = '') AND status = ?", groupID, tenantID, string(entity.StatusValid)).
		First(&group).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, fmt.Errorf("load compilation template group %q: %w", groupID, err)
	}
	var templates []entity.CompilationTemplate
	if err := dao.DB.WithContext(ctx).Where("group_id = ? AND status = ?", groupID, string(entity.StatusValid)).Find(&templates).Error; err != nil {
		return false, fmt.Errorf("load compilation template group %q: %w", groupID, err)
	}
	for _, template := range templates {
		if templateKind(template) == "wiki" {
			return true, nil
		}
	}
	return false, nil
}

func templateKind(template entity.CompilationTemplate) string {
	if kind, ok := template.Config["kind"].(string); ok && strings.TrimSpace(kind) != "" {
		return strings.ToLower(strings.TrimSpace(kind))
	}
	return strings.ToLower(strings.TrimSpace(template.Kind))
}

func setDifference(left, right map[string]struct{}) []string {
	ids := make([]string, 0)
	for id := range left {
		if _, ok := right[id]; !ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func sortedSetKeys(set map[string]struct{}) []string {
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
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

// ListNavClusters returns the navigation clusters of a dataset. It delegates to
// the ES-backed NavService and returns the frontend DatasetNavNode shape
// (snake_case NavNode JSON), matching Python GET /navigation exactly. The old
// NavigationItem{name,title,count} shape did not match the frontend interface
// and has been removed.
func (s *DatasetArtifactService) ListNavClusters(ctx context.Context, tenantID, datasetID string) ([]nav.NavNode, int64, error) {
	ns := nav.GetNavService()
	if ns == nil {
		return nil, 0, fmt.Errorf("datasetnav: NavService not initialized (SetNavService must be called at bootstrap)")
	}
	nodes, total, err := ns.ListClusters(ctx, tenantID, datasetID, 0, 10000)
	if err != nil {
		return nil, 0, err
	}
	return nodes, total, nil
}

// ListNavChildren returns the children of a navigation cluster in the frontend
// DatasetNavNode shape.
func (s *DatasetArtifactService) ListNavChildren(ctx context.Context, tenantID, datasetID, name string) ([]nav.NavNode, int64, error) {
	ns := nav.GetNavService()
	if ns == nil {
		return nil, 0, fmt.Errorf("datasetnav: NavService not initialized (SetNavService must be called at bootstrap)")
	}
	nodes, total, err := ns.ListChildren(ctx, tenantID, datasetID, name, 0, 10000)
	if err != nil {
		return nil, 0, err
	}
	return nodes, total, nil
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

// bareWikiSlug strips only the first path segment from a full wiki slug
// ("entity/location/长社" -> "location/长社"). Nested type/name segments are
// preserved so distinct typed entities remain distinguishable in graph links.
func bareWikiSlug(slug string) string {
	s := strings.TrimSpace(slug)
	if idx := strings.IndexByte(s, '/'); idx >= 0 && idx < len(s)-1 {
		return s[idx+1:]
	}
	return s
}

// toStringSlice normalizes an ES field value (string or list) to []string.
func toStringSlice(v interface{}) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return []string{}
		}
		var decoded interface{}
		if json.Unmarshal([]byte(t), &decoded) == nil {
			return toStringSlice(decoded)
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
