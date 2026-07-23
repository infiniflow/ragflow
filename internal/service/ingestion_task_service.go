package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	redis2 "ragflow/internal/engine/redis"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// Run-count key for IngestionTaskLog.Checkpoint, consumed by
// ListAllForAdmin and IncrementRunCount to track how many times
// the task has been picked up by a worker.
const (
	stepKeyRunCount = "run_count"
)

type InvalidTaskTransitionError struct {
	TaskID string
	From   string
	To     string
}

func (e *InvalidTaskTransitionError) Error() string {
	return fmt.Sprintf("task %s status cannot transition from %s to %s", e.TaskID, e.From, e.To)
}

type TaskStatusConflictError struct {
	TaskID        string
	ExpectedFrom  string
	AttemptedTo   string
	ActualCurrent string
}

func (e *TaskStatusConflictError) Error() string {
	return fmt.Sprintf("task %s status conflict: expected %s -> %s, actual current %s", e.TaskID, e.ExpectedFrom, e.AttemptedTo, e.ActualCurrent)
}

type IngestionTaskService struct {
	documentDAO         *dao.DocumentDAO
	userDAO             *dao.UserDAO
	ingestionTaskDAO    *dao.IngestionTaskDAO
	ingestionTaskLogDAO *dao.IngestionTaskLogDAO
	taskPublisher       TaskPublisher
}

func NewIngestionTaskService() *IngestionTaskService {
	return &IngestionTaskService{
		documentDAO:         dao.NewDocumentDAO(),
		userDAO:             dao.NewUserDAO(),
		ingestionTaskDAO:    dao.NewIngestionTaskDAO(),
		ingestionTaskLogDAO: dao.NewIngestionTaskLogDAO(),
		taskPublisher:       NewMessageQueueTaskPublisher(),
	}
}

func (s *IngestionTaskService) SetTaskPublisher(taskPublisher TaskPublisher) {
	if taskPublisher == nil {
		return
	}
	s.taskPublisher = taskPublisher
}

func (s *IngestionTaskService) ListByUser(userID string, datasetID *string, page, pageSize int) ([]*entity.IngestionTask, error) {
	if datasetID == nil {
		return s.ingestionTaskDAO.ListByUserID(userID, page, pageSize)
	}
	return s.ingestionTaskDAO.ListByUserIDAndDatasetID(userID, *datasetID, page, pageSize)
}

func (s *IngestionTaskService) CreateForDocuments(ctx context.Context, datasetID, userID string, docIDs []string) ([]*ParseDocumentResponse, error) {
	uniqueDocIDs := common.Deduplicate(docIDs)
	if len(uniqueDocIDs) == 0 {
		return nil, fmt.Errorf("no documents to parse")
	}

	responses := make([]*ParseDocumentResponse, 0, len(uniqueDocIDs))
	for _, docID := range uniqueDocIDs {
		doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
		if err != nil {
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     err.Error(),
			})
			continue
		}
		if doc == nil {
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     "no such document",
			})
			continue
		}

		task := &entity.IngestionTask{
			DocumentID: docID,
			UserID:     userID,
			DatasetID:  datasetID,
			Schema:     nil,
			Status:     common.CREATED,
		}
		task, err = s.CreateAndEnqueue(task)
		if err != nil {
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     err.Error(),
			})
			continue
		}

		responses = append(responses, &ParseDocumentResponse{
			DocumentID: docID,
			Result:     fmt.Sprintf("task_id: %s", task.ID),
		})
	}
	return responses, nil
}

func (s *IngestionTaskService) RequestStopMany(tasks []string, ownerUserID *string) ([]*entity.IngestionTask, error) {
	taskResponses := make([]*entity.IngestionTask, 0, len(tasks))
	for _, taskID := range tasks {
		if ownerUserID != nil {
			task, err := s.GetTask(taskID)
			if err != nil {
				return nil, err
			}
			if task.UserID != *ownerUserID {
				return nil, errors.New("task does not belong to the user")
			}
		}
		task, err := s.RequestStop(taskID)
		if err != nil {
			return nil, err
		}
		taskResponses = append(taskResponses, task)
	}
	return taskResponses, nil
}

func (s *IngestionTaskService) RemoveMany(tasks []string, ownerUserID *string) ([]map[string]string, error) {
	deletedTasks := make([]map[string]string, 0, len(tasks))
	for _, taskID := range tasks {
		taskRecord := map[string]string{"task_id": taskID}
		if _, err := s.Remove(taskID, ownerUserID); err != nil {
			taskRecord["remove"] = fmt.Sprintf("fail: %s", err.Error())
		} else {
			taskRecord["remove"] = "success"
		}
		deletedTasks = append(deletedTasks, taskRecord)
	}
	return deletedTasks, nil
}

func (s *IngestionTaskService) ListAllForAdmin() ([]map[string]interface{}, error) {
	ingestionTasks, err := s.ingestionTaskDAO.GetAllTasks(0, 0)
	if err != nil {
		return nil, err
	}

	showTasks := make([]map[string]interface{}, 0, len(ingestionTasks))
	for _, task := range ingestionTasks {
		var user *entity.User
		user, err = s.userDAO.GetByTenantID(task.UserID)
		if err != nil {
			return nil, err
		}

		showTask := map[string]interface{}{
			"id":          task.ID,
			"user_id":     task.UserID,
			"user":        user.Email,
			"document_id": task.DocumentID,
			"status":      task.Status,
		}

		if count, ok := s.lastRunCount(task.ID); ok {
			showTask["run_count"] = count
		}

		showTask["component_total"] = task.ComponentTotal
		if task.ComponentTotal > 0 {
			progress, err := s.ingestionTaskLogDAO.AggregateProgress(task.ID, task.ComponentTotal)
			if err == nil {
				showTask["component_done"] = progress.Done
			} else {
				showTask["component_done"] = 0
			}
		} else {
			showTask["component_done"] = 0
		}

		showTasks = append(showTasks, showTask)
	}
	return showTasks, nil
}

func (s *IngestionTaskService) StartRunning(ctx context.Context, taskID string) (*entity.IngestionTask, error) {
	task, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case common.CREATED:
		task, err = s.transition(taskID, common.RUNNING)
		if err != nil {
			return nil, err
		}
		// The task just started running: mirror it to the document so its
		// run status and progress counters reflect real processing, not
		// just API acceptance. Best-effort - a DB blip here must not fail
		// the task transition and trigger a redelivery loop. run uses the
		// document's numeric TaskStatus enum ("1"), not the task's string
		// status label.
		if err = s.documentDAO.UpdateByID(ctx, dao.DB, task.DocumentID, map[string]interface{}{
			"run":              string(entity.TaskStatusRunning),
			"progress":         float64(0),
			"chunk_num":        int64(0),
			"token_num":        int64(0),
			"process_begin_at": time.Now(),
			"progress_msg":     "",
		}); err != nil {
			common.Warn(fmt.Sprintf("StartRunning: mark document %s running for task %s: %v", task.DocumentID, taskID, err))
		}
		return task, nil
	case common.STOPPING:
		return s.transition(taskID, common.STOPPED)
	case common.RUNNING, common.COMPLETED, common.STOPPED, common.FAILED:
		return task, nil
	default:
		return task, fmt.Errorf("task %s has unsupported status %s", taskID, task.Status)
	}
}

func (s *IngestionTaskService) RequestStop(taskID string) (*entity.IngestionTask, error) {
	task, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case common.CREATED:
		return s.transition(taskID, common.STOPPED)
	case common.RUNNING:
		task, err = s.transition(taskID, common.STOPPING)
		if err != nil {
			return nil, err
		}
		// Mirror Python's cancel_all_task_of: set Redis cancel flag so the
		// running worker's pollCancel detects the stop immediately rather
		// than waiting for the next DB poll (up to 3s).
		if rc := redis2.Get(); rc != nil {
			rc.Set(fmt.Sprintf("%s-cancel", taskID), "x", 1*time.Hour)
		}
		return task, nil
	default:
		return task, nil
	}
}

func (s *IngestionTaskService) MarkCompleted(taskID string) error {
	task, err := s.GetTask(taskID)
	if err != nil {
		return err
	}
	if task.Status == common.COMPLETED || task.Status == common.STOPPED || task.Status == common.FAILED {
		return nil // already terminal, idempotent — mirrors MarkStopped
	}
	_, err = s.transition(taskID, common.COMPLETED)
	return err
}

func (s *IngestionTaskService) MarkFailed(taskID string) error {
	task, err := s.GetTask(taskID)
	if err != nil {
		return err
	}
	if task.Status == common.FAILED || task.Status == common.COMPLETED || task.Status == common.STOPPED {
		return nil // already terminal, idempotent — mirrors MarkStopped
	}
	_, err = s.transition(taskID, common.FAILED)
	return err
}

// MarkStopped transitions the task from STOPPING to STOPPED (terminal).
// Idempotent: returns nil if the task is already in a terminal state
// (STOPPED, COMPLETED, or FAILED).
func (s *IngestionTaskService) MarkStopped(taskID string) error {
	task, err := s.GetTask(taskID)
	if err != nil {
		return err
	}
	if task.Status == common.STOPPED || task.Status == common.COMPLETED || task.Status == common.FAILED {
		return nil
	}
	_, err = s.transition(taskID, common.STOPPED)
	return err
}

func (s *IngestionTaskService) Remove(taskID string, userID *string) (*dao.TaskInfo, error) {
	return s.ingestionTaskDAO.Delete(taskID, userID)
}

func (s *IngestionTaskService) GetTask(taskID string) (*entity.IngestionTask, error) {
	task, err := s.ingestionTaskDAO.GetByID(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrTaskNotFound
		}
		return nil, err
	}
	return task, nil
}

func validateTransition(from, to string) error {
	switch from {
	case common.CREATED:
		if to == common.RUNNING || to == common.STOPPED {
			return nil
		}
	case common.RUNNING:
		if to == common.STOPPING || to == common.COMPLETED || to == common.FAILED {
			return nil
		}
	case common.STOPPING:
		if to == common.STOPPED {
			return nil
		}
	case common.FAILED, common.STOPPED:
		if to == common.CREATED {
			return nil
		}
	}
	return &InvalidTaskTransitionError{From: from, To: to}
}

func (s *IngestionTaskService) newTaskStatusConflictError(taskID, expectedFrom, attemptedTo string) error {
	current, err := s.GetTask(taskID)
	if err != nil {
		return err
	}
	return &TaskStatusConflictError{
		TaskID:        taskID,
		ExpectedFrom:  expectedFrom,
		AttemptedTo:   attemptedTo,
		ActualCurrent: current.Status,
	}
}

func (s *IngestionTaskService) transition(taskID string, to string) (*entity.IngestionTask, error) {
	task, err := s.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	if err = validateTransition(task.Status, to); err != nil {
		var transitionErr *InvalidTaskTransitionError
		if errors.As(err, &transitionErr) {
			return task, &InvalidTaskTransitionError{TaskID: taskID, From: transitionErr.From, To: transitionErr.To}
		}
		return task, err
	}
	updated, err := s.ingestionTaskDAO.UpdateStatusIfCurrent(taskID, task.Status, to)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, s.newTaskStatusConflictError(taskID, task.Status, to)
	}
	task.Status = to
	return task, nil
}

func (s *IngestionTaskService) CreateAndEnqueue(task *entity.IngestionTask) (*entity.IngestionTask, error) {
	existing, err := s.ingestionTaskDAO.GetByDocumentID(task.DocumentID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		switch existing.Status {
		case common.FAILED, common.STOPPED:
			originalStatus := existing.Status
			existing, err = s.transition(existing.ID, common.CREATED)
			if err != nil {
				return nil, err
			}
			if err = s.enqueueTask(existing.ID); err != nil {
				if rollbackErr := s.rollbackRetriedTask(existing.ID, originalStatus); rollbackErr != nil {
					return nil, fmt.Errorf("enqueue task %s: %w (rollback failed: %v)", existing.ID, err, rollbackErr)
				}
				return nil, err
			}
			return existing, nil
		default:
			return nil, fmt.Errorf("document id %s already exists, status: %s, task id: %s", task.DocumentID, existing.Status, existing.ID)
		}
	}
	created, err := s.ingestionTaskDAO.Create(task)
	if err != nil {
		return nil, err
	}
	if err = s.enqueueTask(created.ID); err != nil {
		if rollbackErr := s.rollbackCreatedTask(created.ID); rollbackErr != nil {
			return nil, fmt.Errorf("enqueue task %s: %w (rollback failed: %v)", created.ID, err, rollbackErr)
		}
		return nil, err
	}
	return created, nil
}

func (s *IngestionTaskService) rollbackRetriedTask(taskID, status string) error {
	updated, err := s.ingestionTaskDAO.UpdateStatusIfCurrent(taskID, common.CREATED, status)
	if err != nil {
		return err
	}
	if !updated {
		return s.newTaskStatusConflictError(taskID, common.CREATED, status)
	}
	return nil
}

func (s *IngestionTaskService) rollbackCreatedTask(taskID string) error {
	_, err := s.ingestionTaskDAO.Delete(taskID, nil)
	return err
}

func (s *IngestionTaskService) enqueueTask(taskID string) error {
	taskMessage := common.TaskMessage{
		TaskID:   taskID,
		TaskType: common.TaskTypeIngestionTask,
	}
	return s.taskPublisher.PublishTaskMessage("tasks.RAGFLOW", taskMessage)
}

// UpdateComponentTotal records the number of components in the task's DSL
// graph - the authoritative denominator for progress percentage.
func (s *IngestionTaskService) UpdateComponentTotal(taskID string, total int) error {
	return s.ingestionTaskDAO.UpdateComponentTotal(taskID, total)
}

// RecordComponentProgress appends a component lifecycle row to
// ingestion_task_log (phase: 0 started / 1 done / 2 errored). The row's
// Checkpoint is empty; component progress and step checkpoints are distinct
// row models sharing the same table.
func (s *IngestionTaskService) RecordComponentProgress(taskID, component string, phase int, message string) error {
	entry := &entity.IngestionTaskLog{
		TaskID:     taskID,
		Checkpoint: entity.JSONMap{},
		Phase:      phase,
		Component:  component,
		Message:    message,
	}
	return s.ingestionTaskLogDAO.Create(entry)
}

// AggregateTaskProgress returns the SQL-aggregated component progress for a
// task (done/failed/running/percent against the given total denominator).
func (s *IngestionTaskService) AggregateTaskProgress(taskID string, total int) (*dao.TaskProgress, error) {
	return s.ingestionTaskLogDAO.AggregateProgress(taskID, total)
}

// lastRunCount scans all task logs (newest first) for a run_count entry,
// skipping component-progress rows whose Checkpoint is empty. It returns
// the counter and whether one was found.
func (s *IngestionTaskService) lastRunCount(taskID string) (int, bool) {
	logs, err := s.ingestionTaskLogDAO.ListLogsByTaskID(taskID)
	if err != nil {
		return 0, false
	}
	for i := len(logs) - 1; i >= 0; i-- {
		if count, ok := common.GetInt(logs[i].Checkpoint[stepKeyRunCount]); ok {
			return count, true
		}
	}
	return 0, false
}

// IncrementRunCount scans existing task logs for the previous run_count
// (skipping component-progress rows that have no run_count), then INSERTS a
// new row with the bumped counter. This avoids the race where the latest log
// is a component-progress row whose empty Checkpoint would cause a parse
// failure. ListAllForAdmin reads run_count back to render the attempt number.
//
// A corrupted run_count value in an existing row is skipped (the row is
// ignored). A failure to persist the new row is best-effort (logged) and
// does not return an error — matching the legacy semantics that the run
// proceeds even if the counter write fails.
func (s *IngestionTaskService) IncrementRunCount(taskID string) error {
	prevCount, _ := s.lastRunCount(taskID)

	entry := &entity.IngestionTaskLog{
		TaskID:     taskID,
		Checkpoint: entity.JSONMap{stepKeyRunCount: prevCount + 1},
	}
	if err := s.ingestionTaskLogDAO.Create(entry); err != nil {
		common.Error(fmt.Sprintf("Failed to persist run_count for task %s", taskID), err)
	}
	return nil
}
