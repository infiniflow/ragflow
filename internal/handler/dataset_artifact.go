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

package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/service"
	dataset "ragflow/internal/service/dataset"
	"ragflow/internal/service/datasetversion"
	"ragflow/internal/service/file"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// DatasetArtifactHandler exposes the knowledge-compilation artifact REST APIs
// backed by DatasetArtifactService.
type DatasetArtifactHandler struct {
	svc           *service.DatasetArtifactService
	datasetSvc    *dataset.DatasetService
	fileCommitSvc *file.FileCommitService
}

// NewDatasetArtifactHandler creates a DatasetArtifactHandler.
func NewDatasetArtifactHandler(svc *service.DatasetArtifactService, datasetSvc *dataset.DatasetService, fileCommitSvc *file.FileCommitService) *DatasetArtifactHandler {
	return &DatasetArtifactHandler{svc: svc, datasetSvc: datasetSvc, fileCommitSvc: fileCommitSvc}
}

// datasetOwner resolves the dataset and returns its tenant id. It also enforces
// dataset access permission for the requesting user. When the request is
// unauthenticated, the dataset is missing, or the user is not allowed to access
// it, an error response is written and (nil, "", "") is returned so callers can
// abort without sending a second response.
func (h *DatasetArtifactHandler) datasetOwner(c *gin.Context, datasetID string) (*entity.User, string, string) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.RespondError(c, code, msg)
		return nil, "", ""
	}
	kb, err := h.datasetSvc.GetKnowledgebaseByID(c.Request.Context(), datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			common.RespondError(c, common.CodeNotFound, "dataset not found")
		} else {
			common.Error("failed to resolve dataset", err)
			common.RespondError(c, common.CodeServerError, "failed to resolve dataset")
		}
		return nil, "", ""
	}
	if kb == nil {
		common.RespondError(c, common.CodeNotFound, "dataset not found")
		return nil, "", ""
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckResource(c.Request.Context(), permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID}, permission.OperationRead); err != nil {
		respondPermissionError(c, err, false)
		return nil, "", ""
	}
	return user, kb.TenantID, msg
}

func flattenSourceGroups(groups [][]string) []string {
	ids := make([]string, 0)
	for _, group := range groups {
		ids = append(ids, group...)
	}
	return ids
}

func sourceIDsFromMap(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		ids := make([]string, 0, len(v))
		for _, item := range v {
			if id, ok := item.(string); ok {
				ids = append(ids, id)
			}
		}
		return ids
	case string:
		if v != "" {
			return []string{v}
		}
	}
	return nil
}

func (h *DatasetArtifactHandler) filterStructureSources(c *gin.Context, datasetID string, graph *service.DatasetStructureGraphResponse) error {
	if graph == nil {
		return nil
	}
	graph.TotalEntities, graph.TotalRelations = 0, 0
	graph.ReturnedEntities, graph.ReturnedRelations = 0, 0
	templates := graph.Templates[:0]
	for ti := range graph.Templates {
		template := &graph.Templates[ti]
		groups := make([][]string, len(template.Entities))
		for i := range template.Entities {
			groups[i] = sourceIDsFromMap(template.Entities[i]["source_doc_ids"])
		}
		visible, err := readableArtifactSources(c, datasetID, groups)
		if err != nil {
			return err
		}
		visibleNames := make(map[string]struct{}, len(template.Entities))
		entities := template.Entities[:0]
		for i, node := range template.Entities {
			if i >= len(visible) || !visible[i] {
				continue
			}
			for _, key := range []string{"name", "id", "slug"} {
				if name, ok := node[key].(string); ok && name != "" {
					visibleNames[name] = struct{}{}
				}
			}
			delete(node, "source_doc_ids")
			entities = append(entities, node)
		}
		template.Entities = entities
		relationGroups := make([][]string, len(template.Relations))
		for i := range template.Relations {
			relationGroups[i] = sourceIDsFromMap(template.Relations[i]["source_doc_ids"])
		}
		relationVisible, err := readableArtifactSources(c, datasetID, relationGroups)
		if err != nil {
			return err
		}
		relations := template.Relations[:0]
		for i, relation := range template.Relations {
			if i >= len(relationVisible) || !relationVisible[i] {
				continue
			}
			from, _ := relation["from"].(string)
			to, _ := relation["to"].(string)
			if _, ok := visibleNames[from]; !ok {
				continue
			}
			if _, ok := visibleNames[to]; !ok {
				continue
			}
			delete(relation, "source_doc_ids")
			relations = append(relations, relation)
		}
		template.Relations = relations
		if len(entities) == 0 && len(relations) == 0 {
			continue
		}
		templates = append(templates, *template)
		graph.ReturnedEntities += len(entities)
		graph.ReturnedRelations += len(relations)
	}
	graph.Templates = templates
	graph.TotalEntities = graph.ReturnedEntities
	graph.TotalRelations = graph.ReturnedRelations
	return nil
}

func (h *DatasetArtifactHandler) filterDocumentStructureSources(c *gin.Context, datasetID string, graph *service.DocumentStructureGraphResponse) error {
	if graph == nil {
		return nil
	}
	filtered := &service.DatasetStructureGraphResponse{Templates: graph.Templates}
	if err := h.filterStructureSources(c, datasetID, filtered); err != nil {
		return err
	}
	graph.Templates = filtered.Templates
	graph.TotalEntities = filtered.ReturnedEntities
	graph.ReturnedEntities = filtered.ReturnedEntities
	return nil
}

// AnyArtifact handles HEAD /artifacts — any wiki artifact present?
func (h *DatasetArtifactHandler) AnyArtifact(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	items, _, err := h.svc.ListWikiPagesWithSourceAccess(c.Request.Context(), tenantID, c.Param("dataset_id"), "", "", "", 1, 1,
		func(groups [][]string) ([]bool, error) {
			return readableArtifactSources(c, c.Param("dataset_id"), groups)
		})
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	if len(items) > 0 {
		c.Status(http.StatusOK)
	} else {
		c.Status(http.StatusNotFound)
	}
}

// ListArtifacts handles GET /artifacts — list wiki pages.
func (h *DatasetArtifactHandler) ListArtifacts(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	pageType := c.Query("page_type")
	topic := c.Query("topic")
	keywords := strings.TrimSpace(c.Query("keywords"))
	page, pageSize, err := common.ParsePagination(c.Request.URL.Query())
	if err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}

	items, total, err := h.svc.ListWikiPagesWithSourceAccess(c.Request.Context(), tenantID, datasetID, pageType, topic, keywords, page, pageSize,
		func(groups [][]string) ([]bool, error) { return readableArtifactSources(c, datasetID, groups) })
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	// Python's list_wiki_pages returns {total, items}; align the Go port so the
	// shared frontend (which reads data.items) stays compatible.
	common.RespondSuccess(c, gin.H{"total": total, "items": items}, "success")
}

// UpdateArtifact handles PUT /artifacts/<page_type>/<slug> — edit a wiki page.
// The slug may contain nested path segments, such as location/长社.
func (h *DatasetArtifactHandler) UpdateArtifact(c *gin.Context) {
	common.StartOperationLog(c, "update_dataset_artifact")
	defer common.FinishOperationLog(c)

	user, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	before, err := h.svc.GetWikiPage(c.Request.Context(), tenantID, datasetID, c.Param("page_type"), strings.TrimPrefix(c.Param("slug"), "/"))
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	if before == nil {
		common.RespondError(c, common.CodeNotFound, "page not found")
		return
	}
	if !h.checkArtifactWrite(c, datasetID, before.SourceDocIDs) {
		return
	}
	pageType := c.Param("page_type")
	slug := strings.TrimPrefix(c.Param("slug"), "/")
	var req struct {
		ContentMd string   `json:"content_md"`
		Title     string   `json:"title"`
		Comments  string   `json:"comments"`
		Outlinks  []string `json:"outlinks"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}

	// Read the current page before mutating so an audit commit can record an
	// accurate old-to-new diff. Only record when the prior state was read
	// successfully; a failed read must not produce a diff against an unknown
	// old value.
	var oldContent string
	canRecordAudit := false
	if req.ContentMd != "" {
		oldContent = before.ContentMd
		canRecordAudit = true
	}

	detail, err := h.svc.UpdateWikiPage(c.Request.Context(), tenantID, datasetID, pageType, slug, req.ContentMd, req.Title, req.Outlinks)
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	if detail == nil {
		common.RespondError(c, common.CodeNotFound, "page not found")
		return
	}

	// Record a wiki-page edit audit commit (git-style parent chain) when the
	// content actually changed. Best-effort: a commit write failure must not
	// roll back the page edit already persisted above.
	//
	// The commit id is kept so the version entry below can name the commit that
	// explains it: the diff a page edit produced is stored on that commit's item,
	// and the versions panel reads it back instead of diffing by timestamp.
	var pageCommitID string
	if canRecordAudit && req.ContentMd != oldContent && h.fileCommitSvc != nil {
		title := req.Title
		if title == "" {
			title = detail.Title
		}
		// The snapshot is the whole page, not only the body it just wrote: a restore
		// of this version has to bring the summary, links and entities back with the
		// text, and a body-only snapshot cannot.
		page := map[string]any{
			"content_with_weight":  detail.ContentMd,
			"title_kwd":            detail.Title,
			"summary_with_weight":  detail.Summary,
			"topic_kwd":            detail.Topic,
			"entity_names_kwd":     detail.EntityNames,
			"outlinks_kwd":         detail.Outlinks,
			"related_kb_pages_kwd": detail.RelatedKbPages,
			"source_chunk_ids":     detail.SourceChunkIDs,
			"source_doc_ids":       detail.SourceDocIDs,
			"slug_kwd":             pageType + "/" + slug,
			"page_type_kwd":        pageType,
			"kb_id":                datasetID,
		}
		commit, cerr := h.fileCommitSvc.RecordPageEdit(c.Request.Context(), dao.DB, file.PageEditCommitInput{
			DatasetID:  datasetID,
			Slug:       slug,
			PageType:   pageType,
			Title:      title,
			Comments:   req.Comments,
			AuthorID:   user.ID,
			OldContent: oldContent,
			NewContent: req.ContentMd,
			Page:       page,
		})
		if cerr != nil {
			common.Warn("failed to record wiki page edit commit", zap.Error(cerr))
		} else if commit != nil {
			pageCommitID = commit.ID
		}
	}

	// The page edit is a version of the dataset, not only of the page: the
	// versions page lists it beside the document edits that happened around it,
	// and a restore of any later version puts the page back with the documents
	// (see datasetcontent.restoreWikiPagesAtEE).
	//
	// Recorded on the same condition as the page's own commit - the content
	// actually moved, and the prior body was read - because a version that only
	// repeats the previous body marks a moment that did not happen. Unlike the
	// commit this does not need the commit service: a deployment without page
	// history still has a timeline.
	if canRecordAudit && req.ContentMd != oldContent {
		pageTitle := req.Title
		if pageTitle == "" {
			pageTitle = detail.Title
		}
		if _, verr := datasetversion.RecordContentVersionChangesOwnTx(c.Request.Context(), datasetID,
			entity.DatasetVersionKindArtifactEdit,
			[]entity.DatasetVersionChange{{
				// The page id is the same one the page history keys on, so the
				// timeline entry and the commit that explains it name one thing.
				DocID:  datasetID + "/" + pageType + "/" + slug,
				Entity: entity.DatasetVersionChangeEntityArtifact,
				Action: entity.DatasetVersionChangeActionUpdate,
				Name:   pageTitle,
				// Which commit holds this edit's diff; empty when the page
				// history could not be written, and the panel then reads the
				// page's newest commit at the version's anchor instead.
				CommitID: pageCommitID,
			}}); verr != nil {
			// The page body was written, but the edit is not on the timeline: the
			// caller is told rather than given a success whose version is missing.
			common.RespondError(c, common.CodeServerError, verr.Error())
			return
		}
	}

	common.RespondSuccess(c, detail, "success")
}

// GetArtifact handles GET /artifacts/<page_type>/<slug> — single wiki page.
// The slug may contain nested path segments, such as location/长社.
func (h *DatasetArtifactHandler) GetArtifact(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	pageType := c.Param("page_type")
	slug := strings.TrimPrefix(c.Param("slug"), "/")
	detail, err := h.svc.GetWikiPage(c.Request.Context(), tenantID, datasetID, pageType, slug)
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	if detail == nil {
		common.RespondError(c, common.CodeNotFound, "page not found")
		return
	}
	visible, err := readableArtifactSources(c, datasetID, [][]string{detail.SourceDocIDs})
	if err != nil {
		common.RespondError(c, common.CodeServerError, "failed to validate artifact source permissions")
		return
	}
	if len(visible) == 0 || !visible[0] {
		respondPermissionError(c, permission.ErrPermissionDenied, true)
		return
	}
	detail.CanEdit = h.artifactWriteAllowed(c, datasetID, detail.SourceDocIDs)
	common.RespondSuccess(c, detail, "success")
}

// ListArtifactTopics handles GET /artifacts/topics — list wiki topics.
func (h *DatasetArtifactHandler) ListArtifactTopics(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	keywords := strings.TrimSpace(c.Query("keywords"))
	pages := make([]service.WikiPageItem, 0)
	for _, pageType := range []string{"concept", "entity", "topic"} {
		visiblePages, _, err := h.svc.ListWikiPagesWithSourceAccess(c.Request.Context(), tenantID, datasetID, pageType, "", "", 1, 1000000,
			func(groups [][]string) ([]bool, error) { return readableArtifactSources(c, datasetID, groups) })
		if err != nil {
			common.RespondError(c, common.CodeDataError, err.Error())
			return
		}
		pages = append(pages, visiblePages...)
	}
	counts := make(map[string]int)
	for _, page := range pages {
		if page.Topic != "" {
			counts[page.Topic]++
		}
	}
	orderedTopics, _, err := h.svc.ListWikiTopics(c.Request.Context(), tenantID, datasetID, "")
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	items := make([]service.WikiTopicItem, 0, len(counts))
	lowerKeyword := strings.ToLower(keywords)
	for _, item := range orderedTopics {
		count, exists := counts[item.Topic]
		if !exists {
			continue
		}
		item.PageCount = count
		topic := item.Topic
		if keywords != "" && !strings.Contains(strings.ToLower(topic), lowerKeyword) && !strings.Contains(strings.ToLower(item.Title), lowerKeyword) {
			matchedPage := false
			for _, page := range pages {
				if page.Topic == topic && (strings.Contains(strings.ToLower(page.Title), lowerKeyword) || strings.Contains(strings.ToLower(page.Slug), lowerKeyword) || strings.Contains(strings.ToLower(page.Summary), lowerKeyword)) {
					matchedPage = true
					break
				}
			}
			if !matchedPage {
				continue
			}
		}
		items = append(items, item)
	}
	total := int64(len(items))
	// Python's list_wiki_topics returns {total, items}; align the Go port so the
	// shared frontend (which reads data.items) stays compatible.
	common.RespondSuccess(c, gin.H{"total": total, "items": items}, "success")
}

// GetArtifactAlteration handles GET /artifacts/alteration — wiki alteration summary.
func (h *DatasetArtifactHandler) GetArtifactAlteration(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	alt, err := h.svc.GetWikiAlteration(c.Request.Context(), tenantID, datasetID)
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	all := append([]string{}, alt.RemovedDocIDs...)
	all = append(all, alt.NewlyUploadedDocIDs...)
	all = append(all, alt.InvolvedDocIDs...)
	all = append(all, alt.EligibleDocIDs...)
	groups := make([][]string, len(all))
	for i, id := range all {
		groups[i] = []string{id}
	}
	visible, err := readableArtifactSources(c, datasetID, groups)
	if err != nil {
		common.RespondError(c, common.CodeServerError, "failed to validate artifact source permissions")
		return
	}
	readable := make(map[string]struct{})
	for i, id := range all {
		if i < len(visible) && visible[i] {
			readable[id] = struct{}{}
		}
	}
	filterIDs := func(ids []string) []string {
		filtered := make([]string, 0, len(ids))
		for _, id := range ids {
			if _, ok := readable[id]; ok {
				filtered = append(filtered, id)
			}
		}
		return filtered
	}
	alt.RemovedDocIDs = filterIDs(alt.RemovedDocIDs)
	alt.NewlyUploadedDocIDs = filterIDs(alt.NewlyUploadedDocIDs)
	alt.InvolvedDocIDs = filterIDs(alt.InvolvedDocIDs)
	alt.EligibleDocIDs = filterIDs(alt.EligibleDocIDs)
	alt.Removed = len(alt.RemovedDocIDs)
	alt.NewlyUploaded = len(alt.NewlyUploadedDocIDs)
	common.RespondSuccess(c, alt, "success")
}

// GetArtifactGraph handles GET /artifacts/graph — wiki entity/relation graph.
// keywords seeds the graph with BM25 matches; top_n/topN controls the entity cap.
func (h *DatasetArtifactHandler) GetArtifactGraph(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	keywords := strings.TrimSpace(c.Query("keywords"))
	topNValue := c.Query("top_n")
	if topNValue == "" {
		topNValue = c.Query("topN")
	}
	var topN *int
	if topNValue != "" {
		// A non-empty but non-integer top_n must surface as an argument error
		// (matches the validation pattern used elsewhere in this package):
		// silently coercing to the default makes a malformed request
		// indistinguishable from an omitted one and lets callers rely on a
		// budget the route never actually applied.
		value, parseErr := strconv.Atoi(topNValue)
		if parseErr != nil {
			common.RespondError(c, common.CodeArgumentError, fmt.Sprintf("top_n must be an integer (got %q)", topNValue))
			return
		}
		topN = &value
	}
	graph, err := h.svc.GetWikiGraph(c.Request.Context(), tenantID, datasetID, keywords, topN)
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	entityGroups := make([][]string, len(graph.Entities))
	for i := range graph.Entities {
		entityGroups[i] = graph.Entities[i].SourceDocIDs
	}
	entityVisible, err := readableArtifactSources(c, datasetID, entityGroups)
	if err != nil {
		common.RespondError(c, common.CodeServerError, "failed to validate artifact source permissions")
		return
	}
	visibleSlugs := make(map[string]struct{}, len(graph.Entities))
	entities := graph.Entities[:0]
	for i, item := range graph.Entities {
		if i >= len(entityVisible) || !entityVisible[i] {
			continue
		}
		visibleSlugs[item.Slug] = struct{}{}
		item.SourceDocIDs = nil
		entities = append(entities, item)
	}
	graph.Entities = entities
	relationGroups := make([][]string, len(graph.Relations))
	for i := range graph.Relations {
		relationGroups[i] = graph.Relations[i].SourceDocIDs
	}
	relationVisible, err := readableArtifactSources(c, datasetID, relationGroups)
	if err != nil {
		common.RespondError(c, common.CodeServerError, "failed to validate artifact source permissions")
		return
	}
	relations := graph.Relations[:0]
	for i, item := range graph.Relations {
		if i >= len(relationVisible) || !relationVisible[i] {
			continue
		}
		if _, ok := visibleSlugs[item.From]; !ok {
			continue
		}
		if _, ok := visibleSlugs[item.To]; !ok {
			continue
		}
		relations = append(relations, service.WikiGraphRelation{From: item.From, To: item.To})
	}
	graph.Relations = relations
	graph.TotalEntities = len(graph.Entities)
	graph.TotalRelations = len(graph.Relations)
	graph.ReturnedEntities = len(graph.Entities)
	graph.ReturnedRelations = len(graph.Relations)
	common.RespondSuccess(c, graph, "success")
}

// ListStructures handles GET /artifacts/structure?kind=<kind>&keywords=<query> —
// the dataset-scope structure graph for a resolved kind (mirrors Python
// get_dataset_structure). A non-empty keywords value returns the matching
// entity subgraph.
// kind is REQUIRED: missing or invalid → 400 ARGUMENT_ERROR.
func (h *DatasetArtifactHandler) ListStructures(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	kind := c.Query("kind")
	if kind == "" {
		common.RespondError(c, common.CodeArgumentError, "kind is required")
		return
	}
	in := service.DatasetStructureGraphInput{
		TenantID:  tenantID,
		DatasetID: datasetID,
		Kind:      kind,
		Keywords:  strings.TrimSpace(c.Query("keywords")),
	}
	resp, err := h.svc.GetDatasetStructure(c.Request.Context(), in)
	if err != nil {
		if errors.Is(err, service.ErrInvalidStructureKind) {
			common.RespondError(c, common.CodeArgumentError, err.Error())
		} else {
			common.RespondError(c, common.CodeServerError, err.Error())
		}
		return
	}
	if err := h.filterStructureSources(c, datasetID, resp); err != nil {
		common.RespondError(c, common.CodeServerError, "failed to validate artifact source permissions")
		return
	}
	common.RespondSuccess(c, resp, "success")
}

// AnySkill handles HEAD /skills — any skill artifact present?
func (h *DatasetArtifactHandler) AnySkill(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	groups, err := h.svc.SkillSourceDocGroups(c.Request.Context(), tenantID, c.Param("dataset_id"), "")
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	visible, err := readableArtifactSources(c, c.Param("dataset_id"), groups)
	if err != nil {
		common.RespondError(c, common.CodeServerError, "failed to validate artifact source permissions")
		return
	}
	has := false
	for _, allowed := range visible {
		if allowed {
			has = true
			break
		}
	}
	if has {
		c.Status(http.StatusOK)
	} else {
		c.Status(http.StatusNotFound)
	}
}

// ListNavigation handles GET /navigation — list navigation clusters. The
// optional keywords query searches navigation rows across the dataset.
func (h *DatasetArtifactHandler) ListNavigation(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	keywords := strings.TrimSpace(c.Query("keywords"))
	items, total, err := h.svc.ListNavClusters(c.Request.Context(), tenantID, c.Param("dataset_id"), keywords)
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	// Response key is "items" (not "nav") — the frontend DatasetNavList reads
	// data.items and Python list_nav_clusters/_nav_search return {"total","items"}.
	common.RespondSuccess(c, gin.H{"total": total, "items": items}, "success")
}

// ListNavigationChildren handles GET /navigation/<name>/children — list
// children of a navigation cluster, optionally filtered by keywords.
func (h *DatasetArtifactHandler) ListNavigationChildren(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	keywords := strings.TrimSpace(c.Query("keywords"))
	items, total, err := h.svc.ListNavChildren(c.Request.Context(), tenantID, c.Param("dataset_id"), c.Param("name"), keywords)
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	// Same contract as the top-level nav list: Python list_nav_children returns
	// {"total","items"} and the frontend reads data.items for child expansion.
	common.RespondSuccess(c, gin.H{"total": total, "items": items}, "success")
}

// GetSkillTree handles GET /skills — skill tree.
func (h *DatasetArtifactHandler) GetSkillTree(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	kwd := c.Query("kwd")
	items, total, err := h.svc.GetSkillTree(c.Request.Context(), tenantID, c.Param("dataset_id"), kwd)
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	groups := make([][]string, len(items))
	for i := range items {
		groups[i] = items[i].SourceDocIDs
	}
	visible, err := readableArtifactSources(c, c.Param("dataset_id"), groups)
	if err != nil {
		common.RespondError(c, common.CodeServerError, "failed to validate artifact source permissions")
		return
	}
	filtered := items[:0]
	for i, item := range items {
		if i < len(visible) && visible[i] {
			filtered = append(filtered, item)
		}
	}
	items = filtered
	total = int64(len(items))
	common.RespondSuccess(c, gin.H{"total": total, "tree": items}, "success")
}

// DeleteSkills handles DELETE /skills — delete all skills.
func (h *DatasetArtifactHandler) DeleteSkills(c *gin.Context) {
	common.StartOperationLog(c, "delete_dataset_skills")
	defer common.FinishOperationLog(c)

	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	groups, err := h.svc.SkillSourceDocGroups(c.Request.Context(), tenantID, datasetID, "")
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	if len(groups) == 0 {
		if !checkDatasetArtifactWrite(c, datasetID) {
			return
		}
	} else if !h.checkArtifactWrite(c, datasetID, flattenSourceGroups(groups)) {
		return
	}
	n, err := h.svc.DeleteSkills(c.Request.Context(), tenantID, datasetID, "")
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"deleted": n}, "success")
}

// GetSkillPage handles GET /skills/<skill_kwd> — single skill page.
func (h *DatasetArtifactHandler) GetSkillPage(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	detail, err := h.svc.GetSkillPage(c.Request.Context(), tenantID, c.Param("dataset_id"), c.Param("skill_kwd"))
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	if detail == nil {
		common.RespondError(c, common.CodeNotFound, "skill not found")
		return
	}
	visible, err := readableArtifactSources(c, c.Param("dataset_id"), [][]string{detail.SourceDocIDs})
	if err != nil {
		common.RespondError(c, common.CodeServerError, "failed to validate artifact source permissions")
		return
	}
	if len(visible) == 0 || !visible[0] {
		respondPermissionError(c, permission.ErrPermissionDenied, true)
		return
	}
	common.RespondSuccess(c, detail, "success")
}

// DeleteSkill handles DELETE /skills/<skill_kwd> — delete a single skill.
func (h *DatasetArtifactHandler) DeleteSkill(c *gin.Context) {
	common.StartOperationLog(c, "delete_dataset_skill")
	defer common.FinishOperationLog(c)

	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	groups, err := h.svc.SkillSourceDocGroups(c.Request.Context(), tenantID, datasetID, c.Param("skill_kwd"))
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	if len(groups) == 0 {
		if !checkDatasetArtifactWrite(c, datasetID) {
			return
		}
	} else if !h.checkArtifactWrite(c, datasetID, flattenSourceGroups(groups)) {
		return
	}
	n, err := h.svc.DeleteSkills(c.Request.Context(), tenantID, datasetID, c.Param("skill_kwd"))
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"deleted": n}, "success")
}

// GetDocumentGraph handles GET /documents/<document_id>/structure/graph — document structure graph.
func (h *DatasetArtifactHandler) GetDocumentGraph(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	if err := service.CheckDocumentAccess(c.Request.Context(), documentID, datasetID, permission.OperationRead); err != nil {
		respondPermissionError(c, err, true)
		return
	}
	resp, err := h.svc.GetDocumentGraph(c.Request.Context(), service.DocumentStructureGraphInput{
		TenantID:   tenantID,
		DatasetID:  datasetID,
		DocumentID: documentID,
		Keywords:   c.Query("keywords"),
	})
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	if resp == nil {
		resp = &service.DocumentStructureGraphResponse{Templates: []service.DocumentStructureGraphTemplate{}}
	}
	if err := h.filterDocumentStructureSources(c, datasetID, resp); err != nil {
		common.RespondError(c, common.CodeServerError, "failed to validate artifact source permissions")
		return
	}
	common.RespondSuccess(c, resp, "success")
}

// GetDocumentClaims handles GET /documents/<document_id>/structure/claims —
// page one document's claim/evidence rows (entity_type_kwd="claim"). The tree
// UI fetches them per leaf cluster on demand: chunk_ids carries the cluster's
// members, template_id scopes the compilation template, limit is capped at 100
// (mirrors Python chunk_api.get_document_structure_claims).
func (h *DatasetArtifactHandler) GetDocumentClaims(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	if err := service.CheckDocumentAccess(c.Request.Context(), documentID, datasetID, permission.OperationRead); err != nil {
		respondPermissionError(c, err, true)
		return
	}

	templateID := strings.TrimSpace(c.Query("template_id"))
	var chunkIDs []string
	for _, id := range strings.Split(c.Query("chunk_ids"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			chunkIDs = append(chunkIDs, id)
		}
	}
	offset := 0
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v > 0 {
		offset = v
	}
	limit := 20
	if v, err := strconv.Atoi(c.Query("limit")); err == nil {
		limit = v
	}
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}

	claims, total, err := h.svc.ListDocumentStructureClaims(c.Request.Context(), tenantID, datasetID, documentID, templateID, chunkIDs, offset, limit)
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"claims": claims, "total": total, "offset": offset, "limit": limit}, "success")
}

// DeleteDocumentGraph handles DELETE /documents/<document_id>/structure/graph — delete document structure graph.
func (h *DatasetArtifactHandler) DeleteDocumentGraph(c *gin.Context) {
	common.StartOperationLog(c, "delete_dataset_document_graph")
	defer common.FinishOperationLog(c)

	_, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.RespondError(c, code, msg)
		return
	}
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	kb, err := h.datasetSvc.GetKnowledgebaseByID(c.Request.Context(), datasetID)
	if err != nil || kb == nil {
		if err != nil && !dao.IsNotFoundErr(err) {
			common.RespondError(c, common.CodeServerError, "failed to resolve dataset")
		} else {
			common.RespondError(c, common.CodeNotFound, "dataset not found")
		}
		return
	}
	if err := checkDocumentGraphWrite(c, documentID); err != nil {
		respondPermissionError(c, err, true)
		return
	}
	n, err := h.svc.DeleteDocumentGraph(c.Request.Context(), kb.TenantID, datasetID, documentID)
	if err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"deleted": n}, "success")
}
