package handler

import (
	"strings"

	"ragflow/internal/common"

	"github.com/gin-gonic/gin"
)

// ApplyDatasetParserConfig handles POST /api/v1/datasets/:dataset_id/documents/apply-parser-config.
func (h *DocumentHandler) ApplyDatasetParserConfig(c *gin.Context) {
	common.StartOperationLog(c, "apply_dataset_parser_config")
	defer common.FinishOperationLog(c)

	_, code, message := GetUser(c)
	if code != common.CodeSuccess {
		common.RespondError(c, code, message)
		return
	}
	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.RespondError(c, common.CodeArgumentError, "dataset_id is required")
		return
	}
	result, code, err := h.documentService.ApplyDatasetParserConfig(c.Request.Context(), datasetID)
	if err != nil {
		common.RespondError(c, code, err.Error())
		return
	}
	common.RespondSuccess(c, result, "Parser settings applied. Re-parse files manually to update their content.")
}
