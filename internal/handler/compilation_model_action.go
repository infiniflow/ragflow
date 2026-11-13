package handler

import (
	"errors"
	"strconv"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// Action routes: the sixth rule layer's operator surface.
//
// Reads are tenant-member (an analyst has to see why an action is not runnable);
// every write is tenant-admin, because a contract decides what the system is
// allowed to do and a binding decides which system does it.

// ListActions returns the model's actions with contract, binding and verdict.
//
//	@Summary List model actions
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models/{template_id}/actions [get]
func (h *CompilationModelHandler) ListActions(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	actions, err := h.actions.ListModelActions(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"))
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"actions": actions, "total": len(actions)}, "success")
}

// actionContractEditRequest is the PATCH body.
type actionContractEditRequest struct {
	// Contract maps a contract field to its new value; null clears the field.
	Contract    map[string]interface{} `json:"contract"`
	Description *string                `json:"description"`
	Reason      string                 `json:"reason"`
}

// UpdateAction edits one action's contract or description.
//
//	@Summary Edit action contract
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Param template_id path string true "model id"
//	@Param action_id path string true "compiled action row id"
//	@Success 200 {object} map[string]interface{}
//	@Router /v1/compilation-models/{template_id}/actions/{action_id} [patch]
func (h *CompilationModelHandler) UpdateAction(c *gin.Context) {
	user, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	var req actionContractEditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}
	result, err := h.governance.EditActionContract(c.Request.Context(), dao.DB, service.ActionContractEditInput{
		TenantID:    tenantID,
		TemplateID:  c.Param("template_id"),
		ActionID:    c.Param("action_id"),
		ActorID:     user.ID,
		DatasetID:   c.Query("dataset_id"),
		Values:      req.Contract,
		Description: req.Description,
		Reason:      req.Reason,
	})
	if err != nil {
		respondActionError(c, err)
		return
	}
	common.RespondSuccess(c, result, "success")
}

// bindingRequest is the PUT body.
type bindingRequest struct {
	MCPServer  string            `json:"mcp_server"`
	MCPTool    string            `json:"mcp_tool"`
	ArgMapping map[string]string `json:"arg_mapping"`
	ReadOnly   bool              `json:"read_only"`
	Enabled    bool              `json:"enabled"`
	QACallable bool              `json:"qa_callable"`
	RiskAck    bool              `json:"risk_ack"`
	Note       string            `json:"note"`
	DatasetID  string            `json:"dataset_id"`
}

// ListBindings returns every binding of the model with live availability.
//
//	@Summary List action bindings
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Router /v1/compilation-models/{template_id}/action-bindings [get]
func (h *CompilationModelHandler) ListBindings(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	bindings, err := h.actions.ListBindings(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"))
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{
		"bindings": bindings,
		"total":    len(bindings),
		// The process switch is reported so the page can explain why nothing is
		// runnable without the operator having to read a config file.
		"exec_enabled": service.ActionExecEnabled(),
	}, "success")
}

// SaveBinding creates or replaces one action's binding.
//
//	@Summary Save action binding
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Router /v1/compilation-models/{template_id}/action-bindings/{action_name} [put]
func (h *CompilationModelHandler) SaveBinding(c *gin.Context) {
	user, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	var req bindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}
	view, err := h.actions.SaveBinding(c.Request.Context(), dao.DB, service.ActionBindingInput{
		TenantID:   tenantID,
		TemplateID: c.Param("template_id"),
		BindingID:  c.Query("binding_id"),
		ActionName: c.Param("action_name"),
		DatasetID:  req.DatasetID,
		MCPServer:  req.MCPServer,
		MCPTool:    req.MCPTool,
		ArgMapping: req.ArgMapping,
		ReadOnly:   req.ReadOnly,
		Enabled:    req.Enabled,
		QACallable: req.QACallable,
		RiskAck:    req.RiskAck,
		Note:       req.Note,
		ActorID:    user.ID,
	})
	if err != nil {
		respondActionError(c, err)
		return
	}
	common.RespondSuccess(c, view, "success")
}

// DeleteBinding removes one binding.
//
//	@Summary Delete action binding
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Router /v1/compilation-models/{template_id}/action-bindings/{action_name} [delete]
func (h *CompilationModelHandler) DeleteBinding(c *gin.Context) {
	_, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	if err := h.actions.DeleteBinding(c.Request.Context(), dao.DB, tenantID, c.Query("binding_id")); err != nil {
		respondActionError(c, err)
		return
	}
	common.RespondSuccess(c, gin.H{"deleted": true}, "success")
}

// actionCallRequest is the body of a dry run or a manual execution.
type actionCallRequest struct {
	Arguments map[string]interface{} `json:"arguments"`
	Reason    string                 `json:"reason"`
}

// DryRunAction reports the verdict and the exact arguments without calling.
//
//	@Summary Dry-run an action
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Router /v1/compilation-models/{template_id}/actions/{action_name}/dry-run [post]
func (h *CompilationModelHandler) DryRunAction(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	var req actionCallRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}
	result, err := h.actions.ActionDryRun(c.Request.Context(), dao.DB, service.ActionCallInput{
		TenantID:   tenantID,
		TemplateID: c.Param("template_id"),
		ActionName: c.Param("action_name"),
		Arguments:  req.Arguments,
	})
	if err != nil {
		respondActionError(c, err)
		return
	}
	common.RespondSuccess(c, result, "success")
}

// ExecuteAction runs one action on an operator's behalf.
//
//	@Summary Execute an action manually
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Router /v1/compilation-models/{template_id}/actions/{action_name}/execute [post]
func (h *CompilationModelHandler) ExecuteAction(c *gin.Context) {
	user, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	var req actionCallRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		common.RespondError(c, common.CodeArgumentError, err.Error())
		return
	}
	if req.Reason == "" {
		// Running an action changes something outside RAGFlow. The reason is stored
		// in the audit row next to the verdict, because a trail without "why" only
		// records that something happened.
		common.RespondError(c, common.CodeArgumentError, "reason is required")
		return
	}
	result, err := h.actions.RunAction(c.Request.Context(), dao.DB, service.ActionCallInput{
		TenantID:   tenantID,
		TemplateID: c.Param("template_id"),
		ActionName: c.Param("action_name"),
		Mode:       entity.ActionExecutionModeManual,
		OperatorID: user.ID,
		// The operator pressing run is the confirmation for a `confirm` verdict.
		ConfirmedBy: user.ID,
		Arguments:   req.Arguments,
	})
	if err != nil {
		if result != nil {
			common.RespondSuccess(c, result, "refused")
			return
		}
		respondActionError(c, err)
		return
	}
	common.RespondSuccess(c, result, "success")
}

// ListExecutions returns the audit trail.
//
//	@Summary List action executions
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Router /v1/compilation-models/{template_id}/action-executions [get]
func (h *CompilationModelHandler) ListExecutions(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	fromMS, _ := strconv.ParseInt(c.Query("from"), 10, 64)
	toMS, _ := strconv.ParseInt(c.Query("to"), 10, 64)
	rows, total, err := h.actions.ListExecutions(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"),
		service.ExecutionQuery{
			ActionName: c.Query("action_name"),
			Verdict:    c.Query("verdict"),
			Status:     c.Query("status"),
			Mode:       c.Query("mode"),
			FromMS:     fromMS,
			ToMS:       toMS,
			Page:       page,
			PageSize:   pageSize,
		})
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"executions": rows, "total": total}, "success")
}

// ListContractEdits returns the manual edit history of one action.
//
//	@Summary List manual contract edits of an action
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Router /v1/compilation-models/{template_id}/actions/{action_name}/contract-edits [get]
func (h *CompilationModelHandler) ListContractEdits(c *gin.Context) {
	_, tenantID, ok := h.resolveReader(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	rows, total, err := h.governance.ListActionContractEdits(c.Request.Context(), dao.DB, tenantID,
		c.Param("template_id"), c.Param("action_name"), (page-1)*pageSize, pageSize)
	if err != nil {
		respondActionError(c, err)
		return
	}
	common.RespondSuccess(c, gin.H{"edits": rows, "total": total}, "success")
}

// respondActionError maps the two rejection families to their status codes.
//
// A contract edit that the caller got wrong is 400; a refusal to execute is
// reported with the verdict so the caller can render the reason instead of
// guessing from a generic message.
func respondActionError(c *gin.Context, err error) {
	var rejected *service.ActionEditRejectedError
	if errors.As(err, &rejected) {
		common.Respond(c, int(common.CodeArgumentError), common.CodeArgumentError, gin.H{
			"code":   rejected.Code,
			"fields": rejected.Fields,
		}, rejected.Message)
		return
	}
	var refused *service.ActionCallError
	if errors.As(err, &refused) {
		common.Respond(c, int(common.CodeForbidden), common.CodeForbidden, gin.H{
			"verdict": refused.Verdict,
			"reasons": refused.Reasons,
		}, refused.Error())
		return
	}
	common.RespondError(c, common.CodeExceptionError, err.Error())
}

// BackfillActions stamps the contract columns on action rows compiled before
// those columns existed.
//
//	@Summary Backfill action contracts
//	@Tags compilation model
//	@Security ApiKeyAuth
//	@Router /v1/compilation-models/{template_id}/actions/backfill [post]
func (h *CompilationModelHandler) BackfillActions(c *gin.Context) {
	_, tenantID, ok := h.resolveWriter(c)
	if !ok {
		return
	}
	updated, err := h.actions.BackfillActionContracts(c.Request.Context(), dao.DB, tenantID, c.Param("template_id"))
	if err != nil {
		common.RespondError(c, common.CodeExceptionError, err.Error())
		return
	}
	common.RespondSuccess(c, gin.H{"updated": updated}, "success")
}
