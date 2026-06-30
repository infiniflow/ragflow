package handler

import (
	"errors"
	"io"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

// TaskHandler task handler
type TaskHandler struct {
	taskService *service.TaskService
}

type patchTaskRequest struct {
	Action string `json:"action"`
}

// NewTaskHandler create task handler
func NewTaskHandler(taskService *service.TaskService) *TaskHandler {
	return &TaskHandler{taskService: taskService}
}

// CancelTask cancel a running task
func (h *TaskHandler) CancelTask(c *gin.Context) {
	user, errorCode, errorMsg := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMsg)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		jsonError(c, common.CodeAuthenticationError, "user_id is required")
		return
	}

	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		jsonError(c, common.CodeArgumentError, "task_id is required")
		return
	}

	code, err := h.taskService.CancelTask(c.Request.Context(), userID, taskID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    code,
		"data":    true,
		"message": "success",
	})
}

// PatchTask modify a running task
func (h *TaskHandler) PatchTask(c *gin.Context) {
	user, errorCode, errorMsg := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMsg)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		jsonError(c, common.CodeAuthenticationError, "user_id is required")
		return
	}

	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		jsonError(c, common.CodeArgumentError, "task_id is required")
		return
	}

	var req patchTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		jsonError(c, common.CodeArgumentError, err.Error())
		return
	}
	if req.Action != "stop" {
		jsonError(c, common.CodeArgumentError, "Invalid action '"+req.Action+"'. Only 'stop' is supported.")
		return
	}

	code, err := h.taskService.CancelTask(c.Request.Context(), userID, taskID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    code,
		"data":    true,
		"message": "success",
	})
}
