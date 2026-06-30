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
	taskDAO *dao.TaskDAO
}

// NewTaskService create task service
func NewTaskService() *TaskService {
	return &TaskService{
		taskDAO: dao.NewTaskDAO(),
	}
}

// CancelTask Sets a Redis cancel flag, updates the task progress to -1 (cancelled),
//
//	and marks the associated document's run status as CANCEL if applicable.
func (s *TaskService) CancelTask(ctx context.Context, userID, taskID string) (common.ErrorCode, error) {
	key := taskID + "-cancel"
	redisClient := redis.Get()
	if redisClient == nil || !redisClient.Set(key, "x", 0) {
		err := errors.New("Failed to stop task")
		common.Error("Failed to set cancel flag for task", err, zap.String("task_id", taskID))
		return common.CodeConnectionError, err
	}

	task, err := s.taskDAO.GetByID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return common.CodeSuccess, nil
		}
		common.Warn("Failed to get task after cancellation", zap.String("task_id", taskID), zap.Error(err))
		return common.CodeSuccess, nil
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

	if task.DocID != "" && task.DocID != canvasDebugDocID && task.DocID != graphRaptorFakeDocID {
		if err := db.Model(&entity.Document{}).
			Where("id = ? AND run IN ?", task.DocID, []string{string(entity.TaskStatusRunning), string(entity.TaskStatusSchedule)}).
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

func appendProgressMsgExpr(db *gorm.DB, msg string) interface{} {
	if db != nil && db.Dialector != nil && db.Dialector.Name() == "sqlite" {
		return gorm.Expr("COALESCE(progress_msg, '') || ?", msg)
	}
	return gorm.Expr("CONCAT(COALESCE(progress_msg, ''), ?)", msg)
}
