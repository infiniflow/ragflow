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
	"net/http"
	"strconv"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/permission"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// OntologyModelHandler serves the lifecycle of a compilation template: which
// version of it is in force, whether the next one may be published, and who
// moved it.
//
// It is a separate handler from CompilationTemplateHandler because it answers a
// different question. /compilation-templates/* serves the editable palette;
// /ontology-models/* serves "what is in force and who put it there", which is
// the half that has to survive an edit.
type OntologyModelHandler struct {
	governance *service.CompilationTemplateGovernanceService
	// actions serves the action half of the same model: its contract, its tool
	// binding and its audit trail. It lives on this handler because everything it
	// answers is scoped to one model's :template_id, and splitting it would mean
	// two handlers resolving the same tenant for the same resource.
	actions *service.ActionBindingService
	// insights serves the measured half of the model: what the corpus produced
	// against this model's declaration, and what that lets it answer. It is the
	// same service the ABox browser reads through, because a measurement is a
	// fold of exactly the rows that browser pages.
	insights *service.DatasetArtifactService
	// dismissals records the findings an operator decided not to act on. It is
	// scoped like the measurement (model × dataset × class) because the same
	// class can be a real gap in one corpus and an artefact of another.
	dismissals *service.OntologyDriftDismissalService
}

// NewOntologyModelHandler creates a OntologyModelHandler.
func NewOntologyModelHandler(svc *service.CompilationTemplateGovernanceService) *OntologyModelHandler {
	return &OntologyModelHandler{
		governance: svc,
		actions:    service.NewActionBindingService(),
		insights:   service.NewDatasetArtifactService(),
		dismissals: service.NewOntologyDriftDismissalService(),
	}
}

// List returns the tenant's ontology models with their published version,
// lifecycle state, and how many datasets compile against them.
//
// The kind is fixed rather than taken from the caller: a model is an ontology
// template, so a kind parameter would only offer a way to ask this endpoint for
// rows it cannot describe.
//
//	@Summary List ontology models
//	@Tags ontology model
//	@Security ApiKeyAuth
//	@Param tenant_id query string false "tenant whose models are listed (defaults to the current user)"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models [get]
func (h *OntologyModelHandler) List(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	models, err := h.governance.ListModels(c.Request.Context(), dao.DB, tenantID, kccommon.KindOntology)
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"models": models, "total": len(models)}, "success")
}

// Check reports what stands between the current draft and a publish. It writes
// nothing, so the UI can call it on every edit without side effects.
//
//	@Summary Check a model's next publish
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/check [get]
func (h *OntologyModelHandler) Check(c *gin.Context) {
	_, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	check, err := h.governance.PublishCheck(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"))
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, check, "success")
}

// SubmitForReview moves a draft to review. A model with a blocker is refused
// here, where the person who can still change it is the one reading the answer.
//
//	@Summary Submit a model for review
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/review [post]
func (h *OntologyModelHandler) SubmitForReview(c *gin.Context) {
	user, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	check, err := h.governance.SubmitForReview(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"), user.ID)
	if err != nil {
		// The check travels with the refusal: the dialog that explains why the
		// submit failed is the same one that would have shown the findings.
		common.Respond(c, http.StatusOK, common.CodeExceptionError, check, err.Error())
		return
	}
	common.RespondSuccess(c, check, "success")
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
//	@Router /v1/ontology-models/{template_id}/publish [post]
func (h *OntologyModelHandler) Publish(c *gin.Context) {
	user, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	var req publishModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, "invalid publish payload")
		return
	}
	check, err := h.governance.PublishVersion(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"), user.ID, req.Note, req.Force)
	if err != nil {
		common.Respond(c, http.StatusOK, common.CodeExceptionError, check, err.Error())
		return
	}
	common.RespondSuccess(c, check, "success")
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
//	@Router /v1/ontology-models/{template_id}/reject [post]
func (h *OntologyModelHandler) Reject(c *gin.Context) {
	user, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	var req rejectModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, "invalid reject payload")
		return
	}
	if err := h.governance.RejectReview(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"), user.ID, req.Reason); err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"template_id": c.Param("template_id"), "publish_status": "draft"}, "success")
}

// Retire withdraws a published model from use. Its versions stay readable, so
// assertions extracted under them remain explainable.
//
//	@Summary Retire a published model
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/retire [post]
func (h *OntologyModelHandler) Retire(c *gin.Context) {
	user, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	if err := h.governance.RetireModel(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"), user.ID); err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"template_id": c.Param("template_id"), "publish_status": "retired"}, "success")
}

// ListVersions returns a model's published versions, newest first.
//
//	@Summary List a model's published versions
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/versions [get]
func (h *OntologyModelHandler) ListVersions(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	versions, err := h.governance.ListVersions(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"))
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"versions": versions, "total": len(versions)}, "success")
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
//	@Router /v1/ontology-models/{template_id}/versions/diff [get]
func (h *OntologyModelHandler) VersionDiff(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	from, _ := strconv.Atoi(c.DefaultQuery("from", "0"))
	to, _ := strconv.Atoi(c.DefaultQuery("to", "0"))
	diff, err := h.governance.VersionDiff(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"), from, to)
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, diff, "success")
}

// Changes reports what the draft changes against the version compilation reads.
//
//	@Summary Draft changes against the published version
//	@Tags ontology model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/changes [get]
func (h *OntologyModelHandler) Changes(c *gin.Context) {
	// Membership: the drift panel reads this while deciding what to declare,
	// and that is a reader's question. Gating it on reviewer rights would hide
	// "what is in my draft" from the people writing it.
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	changes, err := h.governance.PendingChangeSet(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"))
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, changes, "success")
}

// ListDriftDismissals reports the findings an operator has set aside.
//
//	@Summary Dismissed drift findings of one dataset
//	@Tags ontology model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Param dataset_id path string true "dataset id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/datasets/{dataset_id}/dismissals [get]
func (h *OntologyModelHandler) ListDriftDismissals(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	rows, err := h.dismissals.List(c.Request.Context(), dao.DB, tenantID,
		c.Param("template_id"), c.Param("dataset_id"))
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"dismissals": rows}, "success")
}

type dismissDriftRequest struct {
	ClassName string `json:"class_name"`
	Reason    string `json:"reason"`
}

// DismissDriftFinding records "seen, deliberately not changing the model".
//
//	@Summary Set a drift finding aside with a reason
//	@Tags ontology model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Param dataset_id path string true "dataset id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/datasets/{dataset_id}/dismissals [post]
func (h *OntologyModelHandler) DismissDriftFinding(c *gin.Context) {
	user, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	var req dismissDriftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, "invalid dismissal payload")
		return
	}
	rows, err := h.dismissals.Dismiss(c.Request.Context(), dao.DB, tenantID,
		c.Param("template_id"), c.Param("dataset_id"), req.ClassName, req.Reason, user.ID)
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"dismissals": rows}, "success")
}

// RestoreDriftFinding lifts a dismissal so the finding is listed again.
//
//	@Summary Put a dismissed drift finding back
//	@Tags ontology model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Param dataset_id path string true "dataset id"
//	@Param class_name path string true "class name"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/datasets/{dataset_id}/dismissals/{class_name} [delete]
func (h *OntologyModelHandler) RestoreDriftFinding(c *gin.Context) {
	_, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	rows, err := h.dismissals.Restore(c.Request.Context(), dao.DB, tenantID,
		c.Param("template_id"), c.Param("dataset_id"), c.Param("class_name"))
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"dismissals": rows}, "success")
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
//	@Router /v1/ontology-models/{template_id}/datasets [get]
func (h *OntologyModelHandler) Impact(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	impact, err := h.governance.Impact(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"))
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, impact, "success")
}

// AssertionReviewQueue lists the assertions of a model by review state.
//
//	@Summary List assertions awaiting a review decision
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Param status query string false "unreviewed | accepted | rejected | all"
//	@Param class query string false "entity class"
//	@Param property query string false "relation property"
//	@Param source_type query string false "property domain class"
//	@Param target_type query string false "property range class"
//	@Param missing query string false "comma-separated contract fields the row is missing any of"
//	@Param dataset_ids query string false "comma-separated datasets to read"
//	@Param limit query int false "page size (default 30, max 200)"
//	@Param offset query int false "page offset"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/assertions/review [get]
func (h *OntologyModelHandler) AssertionReviewQueue(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	limit, offset, ok := h.parseAssertionReviewPaging(c)
	if !ok {
		return
	}
	queue, err := h.governance.ReviewAssertionQueue(c.Request.Context(), dao.DB, service.ModelAssertionReviewInput{
		TenantID:   tenantID,
		TemplateID: c.Param("template_id"),
		Status:     service.NormalizeAssertionReviewStatus(c.Query("status")),
		Class:      c.Query("class"),
		Property:   c.Query("property"),
		SourceType: c.Query("source_type"),
		TargetType: c.Query("target_type"),
		Missing:    splitCSV(c.Query("missing")),
		DatasetIDs: splitCSV(c.Query("dataset_ids")),
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, queue, "success")
}

// DecideAssertions records accept/reject/reset decisions on compiled assertions.
//
// The bar is tenant membership, not admin: a decision changes what the corpus
// calls trusted, but it is recorded (who, when, why) and reversible, unlike
// publishing a model — which changes what every dataset will produce next.
//
//	@Summary Accept or reject compiled assertions
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Param request body assertionReviewRequest true "decisions"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/assertions/review [post]
func (h *OntologyModelHandler) DecideAssertions(c *gin.Context) {
	user, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	var req assertionReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, "invalid review payload")
		return
	}
	items := make([]service.ModelAssertionReviewDecision, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, service.ModelAssertionReviewDecision{
			ID:        item.ID,
			DatasetID: item.DatasetID,
			Action:    item.Action,
			Note:      item.Note,
		})
	}
	result, err := h.governance.DecideAssertions(c.Request.Context(), dao.DB, service.ModelAssertionReviewActionInput{
		TenantID:   tenantID,
		TemplateID: c.Param("template_id"),
		ActorID:    user.ID,
		Items:      items,
	})
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, result, "success")
}

// assertionReviewRequest is the decision batch body.
type assertionReviewRequest struct {
	Items []assertionReviewItemRequest `json:"items"`
}

type assertionReviewItemRequest struct {
	ID        string `json:"id"`
	DatasetID string `json:"dataset_id"`
	Action    string `json:"action"`
	Note      string `json:"note"`
}

// parseAssertionReviewPaging reads limit/offset, writing the response itself when
// either is malformed.
func (h *OntologyModelHandler) parseAssertionReviewPaging(c *gin.Context) (int, int, bool) {
	limit, offset := 0, 0
	for _, spec := range []struct {
		name string
		into *int
	}{{"limit", &limit}, {"offset", &offset}} {
		raw := c.Query(spec.name)
		if raw == "" {
			continue
		}
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			common.RespondError(c, common.CodeExceptionError, spec.name+" must be an integer")
			return 0, 0, false
		}
		*spec.into = parsed
	}
	return limit, offset, true
}

// resolveReader resolves the caller and the tenant a read is scoped to, writing
// the response itself when the caller may not read there.
func (h *OntologyModelHandler) resolveReader(c *gin.Context) (*entity.User, string, bool) {
	return h.resolve(c, permission.TenantMember)
}

// GetInsight returns the stored measurement of one dataset against this model.
//
// It never scans. A dataset can hold millions of rows and the numbers only change
// when a compile finishes, so the read is a row lookup and the workspace offers a
// refresh instead. A model that has never been measured answers with a null
// insight rather than an error: "not measured yet" is a state the UI shows, not a
// failure it has to translate.
//
//	@Summary Read the measured drift and capability coverage
//	@Tags ontology model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Param dataset_id query string true "dataset the measurement is about"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/insight [get]
func (h *OntologyModelHandler) GetInsight(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	datasetID := c.Query("dataset_id")
	if datasetID == "" {
		common.RespondError(c, common.CodeArgumentError, "dataset_id is required")
		return
	}
	view, err := h.insights.GetInsight(c.Request.Context(), tenantID, datasetID, c.Param("template_id"))
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"insight": view}, "success")
}

// RefreshInsight measures one dataset now and stores the result.
//
// It is a write endpoint on a reader's screen because it is the only path that
// scans, and the scan is bounded by ontologyCountScanCap rather than by a page.
// Members may run it: measuring harms nothing and the person looking at the drift
// list is the one who needs it current.
//
//	@Summary Measure one dataset against the model
//	@Tags ontology model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/insight [post]
func (h *OntologyModelHandler) RefreshInsight(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	var req struct {
		DatasetID string `json:"dataset_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}
	if req.DatasetID == "" {
		common.RespondError(c, common.CodeArgumentError, "dataset_id is required")
		return
	}
	view, err := h.insights.RefreshInsight(c.Request.Context(), tenantID, req.DatasetID, c.Param("template_id"))
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, view, "success")
}

// ExportContract renders one model version as prose, YAML and a JSON schema.
//
// The three travel together because they are three renderings of one version: an
// export that could return the prose from v7 and the schema from the draft would
// be worse than useless — it would be signed off against the wrong contract.
//
//	@Summary Export a model version
//	@Tags ontology model
//	@Security ApiKeyAuth
//	@Param template_id path string true "template id"
//	@Param version query int false "published version, defaults to the current one"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/ontology-models/{template_id}/contract [get]
func (h *OntologyModelHandler) ExportContract(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	version := 0
	if raw := c.Query("version"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			common.RespondError(c, common.CodeArgumentError, "version must be an integer")
			return
		}
		version = parsed
	}
	export, err := h.insights.ExportModelContract(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"), version)
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, export, "success")
}

// resolveWriter resolves the caller for a lifecycle transition, which changes
// what every dataset compiling against the model will produce — so it asks for
// tenant admin rather than mere membership.
func (h *OntologyModelHandler) resolveWriter(c *gin.Context) (*entity.User, string, bool) {
	return h.resolve(c, permission.TenantAdmin)
}

func (h *OntologyModelHandler) resolve(c *gin.Context, requirement permission.TenantRequirement) (*entity.User, string, bool) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.RespondError(c, code, msg)
		return nil, "", false
	}
	tenantID := c.Query("tenant_id")
	if tenantID == "" {
		return user, user.ID, true
	}
	ctx := permission.WithIdentity(
		c.Request.Context(), user.ID, permission.TenantIDFromContext(c.Request.Context()),
	)
	if err := permission.NewDatabaseChecker(dao.DB).CheckTenant(ctx, tenantID, requirement); err != nil {
		respondPermissionError(c, err, false)
		return nil, "", false
	}
	return user, tenantID, true
}
