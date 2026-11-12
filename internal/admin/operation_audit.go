package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type auditResponseWriter struct {
	gin.ResponseWriter
	body      bytes.Buffer
	truncated bool
}

func (w *auditResponseWriter) Write(data []byte) (int, error) {
	const limit = 8192
	remaining := limit - w.body.Len()
	if len(data) > remaining {
		w.truncated = true
	}
	if remaining > 0 {
		w.body.Write(data[:min(len(data), remaining)])
	}
	return w.ResponseWriter.Write(data)
}

func adminAction(method, path string) string {
	switch path {
	case "/api/v1/admin/users":
		return "user.create"
	case "/api/v1/admin/users/:username":
		return "user.delete"
	case "/api/v1/admin/users/:username/password":
		return "user.password.reset"
	case "/api/v1/admin/users/:username/activate":
		return "user.status.update"
	case "/api/v1/admin/users/:username/admin":
		if method == http.MethodPut {
			return "user.admin.grant"
		}
		return "user.admin.revoke"
	case "/api/v1/admin/logout":
		return "admin.logout"
	default:
		return "admin." + strings.ToLower(method)
	}
}

// auditMiddleware persists an attempt before executing a protected mutation.
// An interrupted request retains its started record for later investigation.
func (h *Handler) auditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*entity.User)
		entry := entity.AdminOperationLog{
			ID: common.GenerateUUID(), EventTime: time.Now().UTC(),
			ActorID: user.ID, ActorEmail: user.Email,
			TargetEmail: c.Param("username"), Action: adminAction(c.Request.Method, c.FullPath()),
			APIPath: c.FullPath(), IPAddress: c.RemoteIP(), Result: "started",
		}
		if entry.TargetEmail != "" {
			var target entity.User
			if err := dao.DB.WithContext(c.Request.Context()).Select("id").
				Where("email = ?", entry.TargetEmail).Take(&target).Error; err == nil {
				entry.TargetID = target.ID
			}
		} else if entry.Action == "admin.logout" {
			entry.TargetID, entry.TargetEmail = user.ID, user.Email
		}
		if err := dao.DB.WithContext(c.Request.Context()).Create(&entry).Error; err != nil {
			common.Error("Cannot persist administrator audit attempt", err)
			common.Respond(c, http.StatusServiceUnavailable, common.CodeServerError, nil, "Cannot record administrator operation")
			c.Abort()
			return
		}
		writer := &auditResponseWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		started := time.Now()
		completed := false
		defer func() {
			result, code := "unknown", writer.Status()
			var response struct {
				Code *int `json:"code"`
			}
			if !writer.truncated && json.Unmarshal(writer.body.Bytes(), &response) == nil && response.Code != nil {
				code = *response.Code
				result = "success"
				if code != int(common.CodeSuccess) {
					result = "failure"
				}
			}
			if !completed || writer.Status() >= http.StatusBadRequest {
				result = "failure"
			}
			status := writer.Status()
			if !completed {
				status, code = http.StatusInternalServerError, int(common.CodeServerError)
			}
			ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 3*time.Second)
			defer cancel()
			err := dao.DB.WithContext(ctx).Model(&entity.AdminOperationLog{}).Where("id = ?", entry.ID).
				Updates(map[string]interface{}{
					"result": result, "http_status": status, "result_code": code,
					"target_id":    auditTarget(c, "audit_target_id", entry.TargetID),
					"target_email": auditTarget(c, "audit_target_email", entry.TargetEmail),
					"duration_ms":  time.Since(started).Milliseconds(),
				}).Error
			if err != nil {
				common.Error("Cannot finalize administrator audit record", err, zap.String("audit_id", entry.ID))
			}
		}()
		c.Next()
		completed = true
	}
}

func (h *Handler) recordLogin(c *gin.Context, user *entity.User, email string, code common.ErrorCode) error {
	entry := entity.AdminOperationLog{
		ID: common.GenerateUUID(), EventTime: time.Now().UTC(), ActorEmail: email,
		Action: "admin.login", APIPath: "/api/v1/admin/login", IPAddress: c.RemoteIP(),
		Result: "failure", HTTPStatus: http.StatusOK, ResultCode: int(code),
	}
	if user != nil {
		entry.ActorID, entry.ActorEmail = user.ID, user.Email
		entry.TargetID, entry.TargetEmail = user.ID, user.Email
	}
	if code == common.CodeSuccess {
		entry.Result = "success"
	}
	return dao.DB.WithContext(c.Request.Context()).Create(&entry).Error
}

func (h *Handler) ListOperationLogs(c *gin.Context) {
	page, size, err := common.ParsePagination(c.Request.URL.Query())
	if err != nil {
		common.RespondError(c, common.CodeBadRequest, err.Error())
		return
	}
	query := dao.DB.WithContext(c.Request.Context()).Model(&entity.AdminOperationLog{})
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		pattern := "%" + escapeAdminKeyword(strings.ToLower(keyword)) + "%"
		query = query.Where("(LOWER(actor_email) LIKE ? ESCAPE '!' OR LOWER(target_email) LIKE ? ESCAPE '!')", pattern, pattern)
	}
	if action := c.Query("action"); action != "" {
		query = query.Where("action = ?", action)
	}
	if result := c.Query("result"); result != "" {
		query = query.Where("result = ?", result)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.Error("Could not list administrator operations", err)
		common.RespondError(c, common.CodeServerError, "Could not list administrator operations")
		return
	}
	entries := make([]entity.AdminOperationLog, 0)
	if err := query.Order("event_time DESC").Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&entries).Error; err != nil {
		common.Error("Could not list administrator operations", err)
		common.RespondError(c, common.CodeServerError, "Could not list administrator operations")
		return
	}
	common.RespondSuccessList(c, entries, "Administrator operations", total)
}

func escapeAdminKeyword(keyword string) string {
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(keyword)
}

func auditTarget(c *gin.Context, key, fallback string) string {
	if value := c.GetString(key); value != "" {
		return value
	}
	return fallback
}
