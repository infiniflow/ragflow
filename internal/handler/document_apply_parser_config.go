package handler

import (
	"strings"

	"ragflow/internal/common"

	"github.com/gin-gonic/gin"
)

// ApplyDatasetParserConfig handles POST /api/v1/datasets/:dataset_id/documents/apply-parser-config.
func (h *DocumentHandler) ApplyDatasetParserConfig(c *gin.Context) {
	user, code, message := GetUser(c)
	if code != common.CodeSuccess {
		common.ErrorWithCode(c, code, message)
		return
	}
	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "dataset_id is required")
		return
	}
	result, code, err := h.documentService.ApplyDatasetParserConfig(c.Request.Context(), user.ID, datasetID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}
	common.SuccessWithData(c, result, "Parser settings applied. Re-parse files manually to update their content.")
}
