package handler

import (
	"context"
	"net/http"
	"strings"

	"ragflow/internal/common"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type datasetTagsService interface {
	AggregateTags(ctx context.Context, datasetIDs []string, userID string) ([]map[string]interface{}, common.ErrorCode, error)
	ListTags(ctx context.Context, datasetID, userID string) ([][2]interface{}, common.ErrorCode, error)
	RenameTag(ctx context.Context, datasetID, userID, fromTag, toTag string) (map[string]interface{}, common.ErrorCode, error)
}

// ListTags handles GET /api/v1/datasets/:dataset_id/tags.
// @Summary List dataset tags
// @Description List tags for a dataset
// @Tags datasets
// @Produce json
// @Security ApiKeyAuth
// @Param dataset_id path string true "Dataset ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/{dataset_id}/tags [get]
func (h *DatasetsHandler) ListTags(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	result, code, err := h.datasetTagsService.ListTags(c.Request.Context(), datasetID, user.ID)
	if err != nil {
		datasetTagsError(c, code, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": result})
}

func (h *DatasetsHandler) RenameTag(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ErrorWithCode(c, common.CodeDataError, "Lack of from_tag or to_tag in request body")
		return
	}
	fromValue, hasFrom := payload["from_tag"]
	toValue, hasTo := payload["to_tag"]
	if !hasFrom || !hasTo {
		common.ErrorWithCode(c, common.CodeDataError, "Lack of from_tag or to_tag in request body")
		return
	}
	fromTag, okFrom := fromValue.(string)
	toTag, okTo := toValue.(string)
	if !okFrom || !okTo {
		common.ErrorWithCode(c, common.CodeArgumentError, "from_tag and to_tag must be strings")
		return
	}
	if strings.TrimSpace(fromTag) == "" || strings.TrimSpace(toTag) == "" {
		common.ErrorWithCode(c, common.CodeArgumentError, "from_tag and to_tag must not be empty")
		return
	}

	datasetID := strings.TrimSpace(c.Param("dataset_id"))
	result, code, err := h.datasetTagsService.RenameTag(c.Request.Context(), datasetID, user.ID, fromTag, toTag)
	if err != nil {
		datasetTagsError(c, code, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": result})
}

// AggregateTags handles GET /api/v1/datasets/tags/aggregation.
// @Summary Aggregate dataset tags
// @Description Aggregate tags across multiple datasets
// @Tags datasets
// @Produce json
// @Security ApiKeyAuth
// @Param dataset_ids query string true "Comma-separated dataset IDs"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/datasets/tags/aggregation [get]
func (h *DatasetsHandler) AggregateTags(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	rawIDs := strings.Split(c.Query("dataset_ids"), ",")
	datasetIDs := make([]string, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		if rawID != "" {
			datasetIDs = append(datasetIDs, rawID)
		}
	}
	if len(datasetIDs) == 0 {
		common.ErrorWithCode(c, common.CodeDataError, "Lack of dataset_ids in query parameters")
		return
	}
	if len(datasetIDs) > 100 {
		common.ErrorWithCode(c, common.CodeArgumentError, "dataset_ids must contain at most 100 IDs")
		return
	}

	result, code, err := h.datasetTagsService.AggregateTags(c.Request.Context(), datasetIDs, user.ID)
	if err != nil {
		datasetTagsError(c, code, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": result})
}

func datasetTagsError(c *gin.Context, code common.ErrorCode, err error) {
	message := err.Error()
	if code == common.CodeServerError {
		common.Warn("dataset tags request failed",
			zap.Error(err),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
		)
		code = common.CodeDataError
		message = "Internal server error"
	}
	common.ErrorWithCode(c, code, message)
}
