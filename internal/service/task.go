package service

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	canvasDebugDocID     = "dataflow_x"
	graphRaptorFakeDocID = "graph_raptor_x"
)

// TaskService task service
type TaskService struct {
	taskDAO     *dao.TaskDAO
	documentDAO *dao.DocumentDAO
	kbDAO       *dao.KnowledgebaseDAO
	tenantDAO   *dao.TenantDAO
}

// NewTaskService create task service
func NewTaskService() *TaskService {
	return &TaskService{
		taskDAO:     dao.NewTaskDAO(),
		documentDAO: dao.NewDocumentDAO(),
		kbDAO:       dao.NewKnowledgebaseDAO(),
		tenantDAO:   dao.NewTenantDAO(),
	}
}

// CancelTask Sets a Redis cancel flag, updates the task progress to -1 (cancelled),
//
//	and marks the associated document's run status as CANCEL if applicable.
func (s *TaskService) CancelTask(ctx context.Context, userID, taskID string) (common.ErrorCode, error) {
	task, err := s.taskDAO.GetByID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return common.CodeSuccess, nil
		}
		common.Warn("Failed to get task before cancellation", zap.String("task_id", taskID), zap.Error(err))
		return common.CodeServerError, err
	}

	doc, code, err := s.authorizeCancelTask(userID, task)
	if err != nil {
		common.Warn("Failed to authorize task cancellation", zap.String("task_id", taskID), zap.String("user_id", userID), zap.Error(err))
		return code, err
	}

	key := taskID + "-cancel"
	redisClient := redis.Get()
	if redisClient == nil || !redisClient.Set(key, "x", 0) {
		err := errors.New("Failed to stop task")
		common.Error("Failed to set cancel flag for task", err, zap.String("task_id", taskID))
		return common.CodeConnectionError, err
	}

	db := dao.GetDB().WithContext(ctx)
	cancelMsg := fmt.Sprintf("\n%s Task stopped by user.", time.Now().Format("15:04:05"))
	if err := db.Model(&entity.Task{}).
		Where("id = ? AND progress >= ? AND progress < ?", taskID, 0, 1).
		Updates(map[string]interface{}{
			"progress_msg": appendProgressMsgExpr(db, cancelMsg),
			"progress":     -1,
		}).Error; err != nil {
		common.Warn("Failed to update task progress after cancellation", zap.String("task_id", taskID), zap.Error(err))
	}

	if doc != nil {
		if err := db.Model(&entity.Document{}).
			Where("id = ? AND kb_id = ? AND run IN ?", doc.ID, doc.KbID, []string{string(entity.TaskStatusRunning), string(entity.TaskStatusSchedule)}).
			Updates(map[string]interface{}{
				"run":      string(entity.TaskStatusCancel),
				"progress": 0,
			}).Error; err != nil {
			common.Warn("Failed to update document run status for task", zap.String("task_id", taskID), zap.String("doc_id", task.DocID), zap.Error(err))
		}
	}

	common.Info("Cancel task succeeded", zap.String("task_id", taskID), zap.String("doc_id", task.DocID), zap.String("user_id", userID))
	return common.CodeSuccess, nil
}

func (s *TaskService) authorizeCancelTask(userID string, task *entity.Task) (*entity.Document, common.ErrorCode, error) {
	if task.DocID == "" || task.DocID == canvasDebugDocID || task.DocID == graphRaptorFakeDocID {
		return nil, common.CodeSuccess, nil
	}

	doc, err := s.documentDAO.GetByID(task.DocID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.CodeAuthenticationError, errors.New("No authorization.")
		}
		return nil, common.CodeServerError, err
	}

	kb, err := s.kbDAO.GetByID(doc.KbID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.CodeAuthenticationError, errors.New("No authorization.")
		}
		return nil, common.CodeServerError, err
	}
	if !hasKBTeamPermission(kb, userID, s.tenantDAO) {
		return nil, common.CodeAuthenticationError, errors.New("No authorization.")
	}

	return doc, common.CodeSuccess, nil
}

func appendProgressMsgExpr(db *gorm.DB, msg string) interface{} {
	if db != nil && db.Dialector != nil && db.Dialector.Name() == "sqlite" {
		return gorm.Expr("COALESCE(progress_msg, '') || ?", msg)
	}
	return gorm.Expr("CONCAT(COALESCE(progress_msg, ''), ?)", msg)
}
