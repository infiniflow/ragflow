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
		common.Respond(c, common.Response{Code: code, Message: message})
		return
	}
	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	if datasetID == "" {
		common.Respond(c, common.Response{Code: common.CodeArgumentError, Message: "dataset_id is required"})
		return
	}
	result, code, err := h.documentService.ApplyDatasetParserConfig(c.Request.Context(), user.ID, datasetID)
	if err != nil {
		common.Respond(c, common.Response{Code: code, Message: err.Error()})
		return
	}
	common.Respond(c, common.Response{Code: common.CodeSuccess, Message: "Parser settings applied. Re-parse files manually to update their content.", Data: result})
}
