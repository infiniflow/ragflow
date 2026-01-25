package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ragflow/internal/dao"
	"ragflow/internal/service"
)

// KnowledgebaseHandler knowledge base handler
type KnowledgebaseHandler struct {
	kbService *service.KnowledgebaseService
	userDAO   *dao.UserDAO
}

// NewKnowledgebaseHandler create knowledge base handler
func NewKnowledgebaseHandler(kbService *service.KnowledgebaseService, userDAO *dao.UserDAO) *KnowledgebaseHandler {
	return &KnowledgebaseHandler{
		kbService: kbService,
		userDAO:   userDAO,
	}
}

// ListKbs list knowledge bases
// @Summary List Knowledge Bases
// @Description Get list of knowledge bases with filtering and pagination
// @Tags knowledgebase
// @Accept json
// @Produce json
// @Param keywords query string false "search keywords"
// @Param page query int false "page number"
// @Param page_size query int false "items per page"
// @Param parser_id query string false "parser ID filter"
// @Param orderby query string false "order by field"
// @Param desc query bool false "descending order"
// @Param request body service.ListKbsRequest true "filter options"
// @Success 200 {object} service.ListKbsResponse
// @Router /v1/kb/list [post]
func (h *KnowledgebaseHandler) ListKbs(c *gin.Context) {
	// Get query parameters
	keywords := c.Query("keywords")
	pageStr := c.Query("page")
	pageSizeStr := c.Query("page_size")
	parserID := c.Query("parser_id")
	orderby := c.Query("orderby")
	descStr := c.Query("desc")

	// Parse page and page_size
	page := 0
	if pageStr != "" {
		page, _ = strconv.Atoi(pageStr)
	}

	pageSize := 0
	if pageSizeStr != "" {
		pageSize, _ = strconv.Atoi(pageSizeStr)
	}

	// Parse desc
	desc := true
	if descStr == "false" {
		desc = false
	}

	// Parse request body - allow empty body
	var req service.ListKbsRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}
	} // else req remains zero-valued

	// Override with query parameters if not provided in body
	if req.Keywords == nil && keywords != "" {
		req.Keywords = &keywords
	}
	if req.Page == nil && page > 0 {
		req.Page = &page
	}
	if req.PageSize == nil && pageSize > 0 {
		req.PageSize = &pageSize
	}
	if req.ParserID == nil && parserID != "" {
		req.ParserID = &parserID
	}
	if req.Orderby == nil && orderby != "" {
		req.Orderby = &orderby
	}
	if req.Desc == nil {
		req.Desc = &desc
	}

	// Get access token from Authorization header
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Missing Authorization header",
		})
		return
	}

	// Get user by access token
	user, err := h.userDAO.GetByAccessToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "Invalid access token",
		})
		return
	}
	userID := user.ID

	// List knowledge bases
	result, err := h.kbService.ListKbs(&req, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    result,
		"message": "success",
	})
}
