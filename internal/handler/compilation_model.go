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

package handler

import (
	"strconv"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// CompilationModelHandler serves the lifecycle of a compilation template: which
// version of it is in force, whether the next one may be published, and who
// moved it.
//
// It is a separate handler from CompilationTemplateHandler because it answers a
// different question. /compilation-templates/* serves the editable palette;
// /compilation-models/* serves "what is in force and who put it there", which is
// the half that has to survive an edit.
type CompilationModelHandler struct {
	governance *service.CompilationTemplateGovernanceService
}

// NewCompilationModelHandler creates a CompilationModelHandler.
func NewCompilationModelHandler(svc *service.CompilationTemplateGovernanceService) *CompilationModelHandler {
	return &CompilationModelHandler{governance: svc}
}

// List returns the tenant's models with their published version, lifecycle
// state, and how many datasets compile against them.
//
//	@Summary List models
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param kind query string false "restrict to one template kind; defaults to ontology, because a model is an ontology template"
//	@Param tenant_id query string false "tenant whose models are listed (defaults to the current user)"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models [get]
func (h *CompilationModelHandler) List(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	models, err := h.governance.ListModels(c.Request.Context(), dao.DB, tenantID, c.Query("kind"))
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, gin.H{"models": models, "total": len(models)}, "success")
}

// Check reports what stands between the current draft and a publish. It writes
// nothing, so the UI can call it on every edit without side effects.
//
//	@Summary Check a model's next publish
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models/{template_id}/check [get]
func (h *CompilationModelHandler) Check(c *gin.Context) {
	_, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	check, err := h.governance.PublishCheck(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"))
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, check, "success")
}

// SubmitForReview moves a draft to review. A model with a blocker is refused
// here, where the person who can still change it is the one reading the answer.
//
//	@Summary Submit a model for review
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models/{template_id}/review [post]
func (h *CompilationModelHandler) SubmitForReview(c *gin.Context) {
	user, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	check, err := h.governance.SubmitForReview(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"), user.ID)
	if err != nil {
		// The check travels with the refusal: the dialog that explains why the
		// submit failed is the same one that would have shown the findings.
		common.ResponseWithCodeData(c, common.CodeExceptionError, check, err.Error())
		return
	}
	common.SuccessWithData(c, check, "success")
}

type publishModelRequest struct {
	// Note is the change description recorded on the version. It is what the
	// version list shows as "what changed and why".
	Note string `json:"note"`
	// Force publishes despite warnings. It cannot override a blocker.
	Force bool `json:"force"`
}

// Publish records the current draft as a new immutable version and moves
// compilation onto it.
//
//	@Summary Publish a model version
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Param request body publishModelRequest true "publish options"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models/{template_id}/publish [post]
func (h *CompilationModelHandler) Publish(c *gin.Context) {
	user, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	var req publishModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeArgumentError, "invalid publish payload")
		return
	}
	check, err := h.governance.PublishVersion(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"), user.ID, req.Note, req.Force)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, check, err.Error())
		return
	}
	common.SuccessWithData(c, check, "success")
}

type rejectModelRequest struct {
	Reason string `json:"reason"`
}

// Reject sends a model under review back to draft, recording why.
//
//	@Summary Reject a model under review
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Param request body rejectModelRequest true "rejection reason"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models/{template_id}/reject [post]
func (h *CompilationModelHandler) Reject(c *gin.Context) {
	user, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	var req rejectModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ErrorWithCode(c, common.CodeArgumentError, "invalid reject payload")
		return
	}
	if err := h.governance.RejectReview(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"), user.ID, req.Reason); err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, gin.H{"template_id": c.Param("template_id"), "publish_status": "draft"}, "success")
}

// Retire withdraws a published model from use. Its versions stay readable, so
// assertions extracted under them remain explainable.
//
//	@Summary Retire a published model
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models/{template_id}/retire [post]
func (h *CompilationModelHandler) Retire(c *gin.Context) {
	user, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	if err := h.governance.RetireModel(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"), user.ID); err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, gin.H{"template_id": c.Param("template_id"), "publish_status": "retired"}, "success")
}

// ListVersions returns a model's published versions, newest first.
//
//	@Summary List a model's published versions
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models/{template_id}/versions [get]
func (h *CompilationModelHandler) ListVersions(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	versions, err := h.governance.ListVersions(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"))
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, gin.H{"versions": versions, "total": len(versions)}, "success")
}

// VersionDiff reports the difference between two versions. `from=0` means the
// working draft, so the same call answers both "what changed last time" and
// "what would publishing change".
//
//	@Summary Diff two versions of a model
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Param from query int false "source version, 0 for the working draft"
//	@Param to query int false "target version, 0 for the working draft"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models/{template_id}/versions/diff [get]
func (h *CompilationModelHandler) VersionDiff(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	from, _ := strconv.Atoi(c.DefaultQuery("from", "0"))
	to, _ := strconv.Atoi(c.DefaultQuery("to", "0"))
	diff, err := h.governance.VersionDiff(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"), from, to)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, diff, "success")
}

type trialModelRequest struct {
	// Datasets and Documents bound the sample. Zero takes the server default.
	Datasets  int `json:"datasets"`
	Documents int `json:"documents"`
}

// Trial compiles the draft model over sample documents of the datasets that use
// it and reports what publishing would change in the knowledge they hold.
//
// It is a POST because it is not a read: a trial runs the model, which costs an
// LLM pass per document. The free check stays on /check; this is the action an
// operator chooses when a count alone does not tell them enough.
//
//	@Summary Rehearse a model change over real documents
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Param request body trialModelRequest false "sample bounds"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models/{template_id}/trial [post]
func (h *CompilationModelHandler) Trial(c *gin.Context) {
	_, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	var req trialModelRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			common.ErrorWithCode(c, common.CodeArgumentError, "invalid trial payload")
			return
		}
	}
	trial, err := h.governance.Trial(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"), service.ModelTrialOptions{
		Datasets:  req.Datasets,
		Documents: req.Documents,
	})
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, trial, "success")
}

// Impact lists the datasets a model reaching a change will affect.
//
// The library shows the count; this is the drill-down behind it, because "3
// datasets" is a prompt and the names are the decision.
//
//	@Summary List the datasets a model affects
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models/{template_id}/datasets [get]
func (h *CompilationModelHandler) Impact(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	impact, err := h.governance.Impact(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"))
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, impact, "success")
}

// Instances returns what a model produced: the instances of one declared class,
// or the assertions of one declared property, merged over the datasets that
// compile with the model.
//
// It is the model-scope counterpart of the dataset-scope ontology drill-downs
// (/datasets/<id>/ontology/...). Those answer "what is in this library"; this
// answers "what did this model extract", which is the question an author has
// while editing it — and the one that needs the fan-out, because a model is one
// template over many datasets while every compiled row is stored per dataset.
//
//	@Summary Inspect what a model produced
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "model (template) id"
//	@Param class query string false "declared class name; answer its instances"
//	@Param property query string false "declared property name; answer its assertions"
//	@Param source_type query string false "property domain class, to pin one polymorphic edge"
//	@Param target_type query string false "property range class"
//	@Param scope query string false "doc | dataset | corpus (default dataset)"
//	@Param dataset_ids query string false "comma-separated datasets to read; default every dataset of the model, capped"
//	@Param document_id query string false "document id; doc scope only"
//	@Param limit query int false "merged page size (default 50, max 200)"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models/{template_id}/instances [get]
func (h *CompilationModelHandler) Instances(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			common.ResponseWithCodeData(c, common.CodeExceptionError, nil, "limit must be an integer")
			return
		}
		limit = parsed
	}
	result, err := h.governance.InspectInstances(c.Request.Context(), dao.DB, service.ModelInspectInput{
		TenantID:   tenantID,
		TemplateID: c.Param("template_id"),
		Class:      c.Query("class"),
		Property:   c.Query("property"),
		SourceType: c.Query("source_type"),
		TargetType: c.Query("target_type"),
		Scope:      service.ModelInspectScope(c.Query("scope")),
		DatasetIDs: splitCSV(c.Query("dataset_ids")),
		DocumentID: c.Query("document_id"),
		Limit:      limit,
	})
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeExceptionError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, result, "success")
}

// resolveReader resolves the caller and the tenant a read is scoped to, writing
// the response itself when the caller may not read there.
func (h *CompilationModelHandler) resolveReader(c *gin.Context) (*entity.User, string, bool) {
	return h.resolve(c, permission.TenantMember)
}

// resolveWriter resolves the caller for a lifecycle transition, which changes
// what every dataset compiling against the model will produce — so it asks for
// tenant admin rather than mere membership.
func (h *CompilationModelHandler) resolveWriter(c *gin.Context) (*entity.User, string, bool) {
	return h.resolve(c, permission.TenantAdmin)
}

func (h *CompilationModelHandler) resolve(c *gin.Context, requirement permission.TenantRequirement) (*entity.User, string, bool) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ErrorWithCode(c, code, msg)
		return nil, "", false
	}
	tenantID := c.Query("tenant_id")
	if tenantID == "" {
		return user, user.ID, true
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckTenant(
		c.Request.Context(), permission.Subject{UserID: user.ID}, tenantID, requirement,
	); err != nil {
		respondPermissionFailure(c, err, false)
		return nil, "", false
	}
	return user, tenantID, true
}
