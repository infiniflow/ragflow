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
	"net/http"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	dataset "ragflow/internal/service/dataset"
	"ragflow/internal/service/file"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// maxArtifactPageSize caps the page_size query parameter for artifact list
// endpoints so a client cannot force an unbounded document-engine search.
const maxArtifactPageSize = 100

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
		common.ErrorWithCode(c, code, msg)
		return nil, "", ""
	}
	kb, err := h.datasetSvc.GetKnowledgebaseByID(c.Request.Context(), datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			common.ErrorWithCode(c, common.CodeNotFound, "dataset not found")
		} else {
			common.ErrorWithCode(c, common.CodeServerError, "failed to resolve dataset")
		}
		return nil, "", ""
	}
	if kb == nil {
		common.ErrorWithCode(c, common.CodeNotFound, "dataset not found")
		return nil, "", ""
	}
	if !h.datasetSvc.Accessible(c.Request.Context(), datasetID, user.ID) {
		common.ErrorWithCode(c, common.CodeForbidden, "no permission to access this dataset")
		return nil, "", ""
	}
	return user, kb.TenantID, msg
}

// AnyArtifact handles HEAD /artifacts — any wiki artifact present?
func (h *DatasetArtifactHandler) AnyArtifact(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	has, err := h.svc.HasWiki(c.Request.Context(), tenantID, c.Param("dataset_id"))
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	if has {
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
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 30
	}
	if pageSize > maxArtifactPageSize {
		pageSize = maxArtifactPageSize
	}
	items, total, err := h.svc.ListWikiPages(c.Request.Context(), tenantID, datasetID, pageType, topic, keywords, page, pageSize)
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	// Python's list_wiki_pages returns {total, items}; align the Go port so the
	// shared frontend (which reads data.items) stays compatible.
	common.SuccessWithData(c, gin.H{"total": total, "items": items}, "success")
}

// UpdateArtifact handles PUT /artifacts/<page_type>/<slug> — edit a wiki page.
// The slug may contain nested path segments, such as location/长社.
func (h *DatasetArtifactHandler) UpdateArtifact(c *gin.Context) {
	user, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	pageType := c.Param("page_type")
	slug := strings.TrimPrefix(c.Param("slug"), "/")
	var req struct {
		ContentMd string   `json:"content_md"`
		Title     string   `json:"title"`
		Comments  string   `json:"comments"`
		Outlinks  []string `json:"outlinks"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeArgumentError, err.Error())
		return
	}

	// Read the current page before mutating so an audit commit can record an
	// accurate old-to-new diff. Only record when the prior state was read
	// successfully; a failed read must not produce a diff against an unknown
	// old value.
	var oldContent string
	canRecordAudit := false
	if req.ContentMd != "" {
		before, gerr := h.svc.GetWikiPage(c.Request.Context(), tenantID, datasetID, pageType, slug)
		if gerr != nil {
			common.Warn("failed to read wiki page before recording edit commit", zap.Error(gerr))
		} else if before != nil {
			oldContent = before.ContentMd
			canRecordAudit = true
		}
	}

	detail, err := h.svc.UpdateWikiPage(c.Request.Context(), tenantID, datasetID, pageType, slug, req.ContentMd, req.Title, req.Outlinks)
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	if detail == nil {
		common.ErrorWithCode(c, common.CodeNotFound, "page not found")
		return
	}

	// Record a wiki-page edit audit commit (git-style parent chain) when the
	// content actually changed. Best-effort: a commit write failure must not
	// roll back the page edit already persisted above.
	if canRecordAudit && req.ContentMd != oldContent && h.fileCommitSvc != nil {
		title := req.Title
		if title == "" {
			title = detail.Title
		}
		if _, cerr := h.fileCommitSvc.RecordPageEdit(c.Request.Context(), file.PageEditCommitInput{
			DatasetID:  datasetID,
			DocID:      pageType + "/" + slug,
			Slug:       slug,
			PageType:   pageType,
			Title:      title,
			Comments:   req.Comments,
			AuthorID:   user.ID,
			OldContent: oldContent,
			NewContent: req.ContentMd,
		}); cerr != nil {
			common.Warn("failed to record wiki page edit commit", zap.Error(cerr))
		}
	}

	common.SuccessWithData(c, detail, "success")
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
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	if detail == nil {
		common.ErrorWithCode(c, common.CodeNotFound, "page not found")
		return
	}
	common.SuccessWithData(c, detail, "success")
}

// ListArtifactTopics handles GET /artifacts/topics — list wiki topics.
func (h *DatasetArtifactHandler) ListArtifactTopics(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	keywords := strings.TrimSpace(c.Query("keywords"))
	items, total, err := h.svc.ListWikiTopics(c.Request.Context(), tenantID, datasetID, keywords)
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	// Python's list_wiki_topics returns {total, items}; align the Go port so the
	// shared frontend (which reads data.items) stays compatible.
	common.SuccessWithData(c, gin.H{"total": total, "items": items}, "success")
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
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	common.SuccessWithData(c, alt, "success")
}

// GetArtifactGraph handles GET /artifacts/graph — wiki entity/relation graph.
func (h *DatasetArtifactHandler) GetArtifactGraph(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	graph, err := h.svc.GetWikiGraph(c.Request.Context(), tenantID, datasetID)
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	common.SuccessWithData(c, graph, "success")
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
		common.ErrorWithCode(c, common.CodeArgumentError, "kind is required")
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
			common.ErrorWithCode(c, common.CodeArgumentError, err.Error())
		} else {
			common.ErrorWithCode(c, common.CodeServerError, err.Error())
		}
		return
	}
	common.SuccessWithData(c, resp, "success")
}

// AnySkill handles HEAD /skills — any skill artifact present?
func (h *DatasetArtifactHandler) AnySkill(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	has, err := h.svc.HasSkill(c.Request.Context(), tenantID, c.Param("dataset_id"))
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	if has {
		c.Status(http.StatusOK)
	} else {
		c.Status(http.StatusNotFound)
	}
}

// ListNavigation handles GET /navigation — list navigation clusters.
func (h *DatasetArtifactHandler) ListNavigation(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	items, total, err := h.svc.ListNavClusters(c.Request.Context(), tenantID, c.Param("dataset_id"))
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	// Response key is "items" (not "nav") — the frontend DatasetNavList reads
	// data.items and Python list_nav_clusters/_nav_search return {"total","items"}.
	common.SuccessWithData(c, gin.H{"total": total, "items": items}, "success")
}

// ListNavigationChildren handles GET /navigation/<name>/children — list children of a navigation cluster.
func (h *DatasetArtifactHandler) ListNavigationChildren(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	items, total, err := h.svc.ListNavChildren(c.Request.Context(), tenantID, c.Param("dataset_id"), c.Param("name"))
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	// Same contract as the top-level nav list: Python list_nav_children returns
	// {"total","items"} and the frontend reads data.items for child expansion.
	common.SuccessWithData(c, gin.H{"total": total, "items": items}, "success")
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
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	common.SuccessWithData(c, gin.H{"total": total, "tree": items}, "success")
}

// DeleteSkills handles DELETE /skills — delete all skills.
func (h *DatasetArtifactHandler) DeleteSkills(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	n, err := h.svc.DeleteSkills(c.Request.Context(), tenantID, c.Param("dataset_id"), "")
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	common.SuccessWithData(c, gin.H{"deleted": n}, "success")
}

// GetSkillPage handles GET /skills/<skill_kwd> — single skill page.
func (h *DatasetArtifactHandler) GetSkillPage(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	detail, err := h.svc.GetSkillPage(c.Request.Context(), tenantID, c.Param("dataset_id"), c.Param("skill_kwd"))
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	if detail == nil {
		common.ErrorWithCode(c, common.CodeNotFound, "skill not found")
		return
	}
	common.SuccessWithData(c, detail, "success")
}

// DeleteSkill handles DELETE /skills/<skill_kwd> — delete a single skill.
func (h *DatasetArtifactHandler) DeleteSkill(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	n, err := h.svc.DeleteSkills(c.Request.Context(), tenantID, c.Param("dataset_id"), c.Param("skill_kwd"))
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	common.SuccessWithData(c, gin.H{"deleted": n}, "success")
}

// GetDocumentGraph handles GET /documents/<document_id>/structure/graph — document structure graph.
func (h *DatasetArtifactHandler) GetDocumentGraph(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	resp, err := h.svc.GetDocumentGraph(c.Request.Context(), service.DocumentStructureGraphInput{
		TenantID:   tenantID,
		DatasetID:  datasetID,
		DocumentID: documentID,
		Keywords:   c.Query("keywords"),
	})
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	if resp == nil {
		resp = &service.DocumentStructureGraphResponse{Templates: []service.DocumentStructureGraphTemplate{}}
	}
	common.SuccessWithData(c, resp, "success")
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
		common.ErrorWithCode(c, common.CodeExceptionError, err.Error())
		return
	}
	common.SuccessWithData(c, gin.H{"claims": claims, "total": total, "offset": offset, "limit": limit}, "success")
}

// DeleteDocumentGraph handles DELETE /documents/<document_id>/structure/graph — delete document structure graph.
func (h *DatasetArtifactHandler) DeleteDocumentGraph(c *gin.Context) {
	_, tenantID, _ := h.datasetOwner(c, c.Param("dataset_id"))
	if tenantID == "" {
		return
	}
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	n, err := h.svc.DeleteDocumentGraph(c.Request.Context(), tenantID, datasetID, documentID)
	if err != nil {
		common.ErrorWithCode(c, common.CodeDataError, err.Error())
		return
	}
	common.SuccessWithData(c, gin.H{"deleted": n}, "success")
}
