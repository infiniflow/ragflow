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
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

// Run-count key for IngestionTaskLog.Checkpoint, consumed by
// ListAllForAdmin and IncrementRunCount to track how many times
// the task has been picked up by a worker.
const (
	stepKeyRunCount = "run_count"
	// dispatchGracePeriod prevents a second reconciler from immediately
	// republishing a task after the first reconciler reserved it.
	dispatchGracePeriod = 2 * time.Minute
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

func (s *IngestionTaskService) ListByUser(ctx context.Context, userID string, datasetID *string, page, pageSize int) ([]*entity.IngestionTask, error) {
	if datasetID == nil {
		return s.ingestionTaskDAO.ListByUserID(ctx, dao.DB, userID, page, pageSize)
	}
	return s.ingestionTaskDAO.ListByUserIDAndDatasetID(ctx, dao.DB, userID, *datasetID, page, pageSize)
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
		if doc.KbID != datasetID {
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     "document does not belong to the requested dataset",
			})
			continue
		}

		task := &entity.IngestionTask{
			DocumentID: docID,
			UserID:     userID,
			DatasetID:  datasetID,
			Schema:     nil,
			Status:     common.SCHEDULED,
		}
		task, err = s.CreateAndEnqueue(ctx, task)
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

func (s *IngestionTaskService) RequestStopMany(ctx context.Context, tasks []string, ownerUserID *string) ([]*entity.IngestionTask, error) {
	taskResponses := make([]*entity.IngestionTask, 0, len(tasks))
	for _, taskID := range tasks {
		if ownerUserID != nil {
			task, err := s.GetTask(ctx, taskID)
			if err != nil {
				return nil, err
			}
			if task.UserID != *ownerUserID {
				return nil, errors.New("task does not belong to the user")
			}
		}
		task, err := s.RequestStop(ctx, taskID)
		if err != nil {
			return nil, err
		}
		taskResponses = append(taskResponses, task)
	}
	return taskResponses, nil
}

func (s *IngestionTaskService) RemoveMany(ctx context.Context, tasks []string, ownerUserID *string) ([]map[string]string, error) {
	deletedTasks := make([]map[string]string, 0, len(tasks))
	for _, taskID := range tasks {
		taskRecord := map[string]string{"task_id": taskID}
		if _, err := s.Remove(ctx, taskID, ownerUserID); err != nil {
			taskRecord["remove"] = fmt.Sprintf("fail: %s", err.Error())
		} else {
			taskRecord["remove"] = "success"
		}
		deletedTasks = append(deletedTasks, taskRecord)
	}
	return deletedTasks, nil
}

func (s *IngestionTaskService) ListAllForAdmin(ctx context.Context) ([]map[string]interface{}, error) {
	ingestionTasks, err := s.ingestionTaskDAO.GetAllTasks(ctx, dao.DB, 0, 0)
	if err != nil {
		return nil, err
	}

	showTasks := make([]map[string]interface{}, 0, len(ingestionTasks))
	for _, task := range ingestionTasks {
		var user *entity.User
		user, err = s.userDAO.GetByTenantID(ctx, dao.DB, task.UserID)
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

		if count, ok := s.lastRunCount(ctx, task.ID); ok {
			showTask["run_count"] = count
		}

		showTask["component_total"] = task.ComponentTotal
		if task.ComponentTotal > 0 {
			progress, err := s.ingestionTaskLogDAO.AggregateProgress(ctx, dao.DB, task.ID, task.ComponentTotal)
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

// TryClaim starts a scheduled task only when this worker wins its database
// lease. A false claimed result is a normal redelivery/race outcome.
func (s *IngestionTaskService) TryClaim(ctx context.Context, taskID string, ttl time.Duration) (*entity.IngestionTask, bool, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, false, err
	}
	if task.Status != common.SCHEDULED {
		return task, false, nil
	}

	now := time.Now()
	token := utility.GenerateUUID()
	claimed, err := s.ingestionTaskDAO.TryClaim(ctx, dao.DB, taskID, token, now, ttl)
	if err != nil || !claimed {
		return task, claimed, err
	}
	task.Status = common.RUNNING
	task.ClaimToken = token
	task.ClaimExpiresAt = now.Add(ttl).UnixMilli()
	if err = s.documentDAO.UpdateByID(ctx, dao.DB, task.DocumentID, map[string]interface{}{
		"run":              string(entity.TaskStatusRunning),
		"progress":         float64(0),
		"chunk_num":        int64(0),
		"token_num":        int64(0),
		"process_begin_at": now,
		"progress_msg":     "",
	}); err != nil {
		common.Warn(fmt.Sprintf("TryClaim: mark document %s running for task %s: %v", task.DocumentID, taskID, err))
	}
	return task, true, nil
}

// FinalizeClaim writes a terminal status only when the supplied token still
// owns the task in fromStatus.
func (s *IngestionTaskService) FinalizeClaim(ctx context.Context, taskID, token, fromStatus, toStatus string) (bool, error) {
	return s.ingestionTaskDAO.FinalizeClaim(ctx, dao.DB, taskID, token, fromStatus, toStatus)
}

// TouchClaim extends a live worker lease. A false result means the worker lost
// ownership; callers must stop work without writing further task state.
func (s *IngestionTaskService) TouchClaim(ctx context.Context, taskID, token string, ttl time.Duration) (bool, error) {
	return s.ingestionTaskDAO.TouchClaim(ctx, dao.DB, taskID, token, time.Now(), ttl)
}

// ReleaseClaim returns a task to SCHEDULED only while the worker still owns
// its lease. It is used when shutdown wins before a claimed task reaches a
// worker, so the next delivery can claim it normally.
func (s *IngestionTaskService) ReleaseClaim(ctx context.Context, taskID, token string) (bool, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return false, err
	}
	released, err := s.ingestionTaskDAO.ReleaseClaim(ctx, dao.DB, taskID, token)
	if err != nil || !released {
		return released, err
	}
	s.markDocumentScheduled(ctx, task.DocumentID, taskID)
	return true, nil
}

func (s *IngestionTaskService) RequestStop(ctx context.Context, taskID string) (*entity.IngestionTask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case common.SCHEDULED:
		return s.transition(ctx, taskID, common.STOPPED)
	case common.RUNNING:
		task, err = s.transition(ctx, taskID, common.STOPPING)
		if err != nil {
			return nil, err
		}
		// Mirror Python's cancel_all_task_of: set Redis cancel flag so the
		// running worker's pollCancel detects the stop immediately rather
		// than waiting for the next DB poll (up to 3s).
		if rc := redis2.Get(); rc != nil {
			rc.Set(ctx, fmt.Sprintf("%s-cancel", taskID), "x", 1*time.Hour)
		}
		return task, nil
	default:
		return task, nil
	}
}

func (s *IngestionTaskService) Remove(ctx context.Context, taskID string, userID *string) (*dao.TaskInfo, error) {
	return s.ingestionTaskDAO.Delete(ctx, dao.DB, taskID, userID)
}

func (s *IngestionTaskService) GetTask(ctx context.Context, taskID string) (*entity.IngestionTask, error) {
	task, err := s.ingestionTaskDAO.GetByID(ctx, dao.DB, taskID)
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
	case common.SCHEDULED:
		if to == common.RUNNING || to == common.STOPPED {
			return nil
		}
	case common.RUNNING:
		if to == common.STOPPING {
			return nil
		}
	case common.STOPPING:
		if to == common.STOPPED {
			return nil
		}
	}
	return &InvalidTaskTransitionError{From: from, To: to}
}

func (s *IngestionTaskService) newTaskStatusConflictError(ctx context.Context, taskID, expectedFrom, attemptedTo string) error {
	current, err := s.GetTask(ctx, taskID)
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

func (s *IngestionTaskService) transition(ctx context.Context, taskID string, to string) (*entity.IngestionTask, error) {
	task, err := s.GetTask(ctx, taskID)
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
	updated, err := s.ingestionTaskDAO.UpdateStatusIfCurrent(ctx, dao.DB, taskID, task.Status, to)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, s.newTaskStatusConflictError(ctx, taskID, task.Status, to)
	}
	task.Status = to
	return task, nil
}

func (s *IngestionTaskService) CreateAndEnqueue(ctx context.Context, task *entity.IngestionTask) (*entity.IngestionTask, error) {
	now := time.Now()
	existing, err := s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, task.DocumentID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		switch existing.Status {
		case common.FAILED, common.STOPPED:
			scheduled, err := s.ingestionTaskDAO.Schedule(ctx, dao.DB, existing.ID, existing.Status, now)
			if err != nil {
				return nil, err
			}
			if !scheduled {
				return nil, s.newTaskStatusConflictError(ctx, existing.ID, existing.Status, common.SCHEDULED)
			}
			existing.Status = common.SCHEDULED
			existing.ScheduledAt = now.UnixMilli()
			existing.LastDispatchedAt = 0
			existing.ClaimToken = ""
			existing.ClaimExpiresAt = 0
			s.markDocumentScheduled(ctx, existing.DocumentID, existing.ID)
			// The previous run is terminal, so any leftover Redis cancel flag
			// is stale: a genuine cancel of the new run can only come through
			// RequestStop once the task is RUNNING again. Clear it so the
			// re-queued task is not cancelled at the worker's pre-start check.
			clearCancelFlag(ctx, existing.ID)
			s.dispatchScheduledTask(ctx, existing.ID, now)
			return existing, nil
		default:
			return nil, fmt.Errorf("document id %s already exists, status: %s, task id: %s", task.DocumentID, existing.Status, existing.ID)
		}
	}
	task.Status = common.SCHEDULED
	task.ScheduledAt = now.UnixMilli()
	task.LastDispatchedAt = 0
	task.ClaimToken = ""
	task.ClaimExpiresAt = 0
	created, err := s.ingestionTaskDAO.Create(ctx, dao.DB, task)
	if err != nil {
		return nil, err
	}
	s.markDocumentScheduled(ctx, created.DocumentID, created.ID)
	s.dispatchScheduledTask(ctx, created.ID, now)
	return created, nil
}

// clearCancelFlag removes the Redis cancel marker ({task_id}-cancel) that
// RequestStop sets for a RUNNING task. No-op when Redis is unavailable —
// the DB STOPPING status remains the fallback cancel signal.
func clearCancelFlag(ctx context.Context, taskID string) {
	if rc := redis2.Get(); rc != nil {
		rc.Delete(ctx, fmt.Sprintf("%s-cancel", taskID))
	}
}

func (s *IngestionTaskService) enqueueTask(taskID string) error {
	taskMessage := common.TaskMessage{
		TaskID:   taskID,
		TaskType: common.TaskTypeIngestionTask,
	}
	return s.taskPublisher.PublishTaskMessage("tasks.RAGFLOW", taskMessage)
}

func (s *IngestionTaskService) dispatchScheduledTask(ctx context.Context, taskID string, now time.Time) {
	if err := s.DispatchScheduledTask(ctx, taskID, now); err != nil {
		common.Warn(fmt.Sprintf("dispatch scheduled task %s: %v", taskID, err))
	}
}

// DispatchScheduledTask reserves and publishes a persisted scheduling intent.
// The reservation is a compare-and-swap on last_dispatched_at, so concurrent
// ingestors do not publish the same scheduled row in the same reconciliation
// pass. A reservation records an attempt before publishing; a failed publish
// remains recoverable because the task stays SCHEDULED and will be eligible
// again after the dispatch grace period.
func (s *IngestionTaskService) DispatchScheduledTask(ctx context.Context, taskID string, now time.Time) error {
	task, err := s.ingestionTaskDAO.GetByID(ctx, dao.DB, taskID)
	if err != nil {
		return err
	}
	if task.Status != common.SCHEDULED {
		return nil
	}
	if task.LastDispatchedAt != 0 && task.LastDispatchedAt >= now.Add(-dispatchGracePeriod).UnixMilli() {
		return nil
	}
	reserved, err := s.ingestionTaskDAO.TryReserveDispatch(ctx, dao.DB, taskID, task.LastDispatchedAt, now)
	if err != nil {
		return err
	}
	if !reserved {
		return nil
	}
	if err := s.enqueueTask(taskID); err != nil {
		return err
	}
	return nil
}

func (s *IngestionTaskService) markDocumentScheduled(ctx context.Context, documentID, taskID string) {
	if err := s.documentDAO.UpdateByID(ctx, dao.DB, documentID, map[string]interface{}{
		"run": string(entity.TaskStatusSchedule),
	}); err != nil {
		common.Warn(fmt.Sprintf("mark document %s scheduled for task %s: %v", documentID, taskID, err))
	}
}

// UpdateComponentTotal records the number of components in the task's DSL
// graph - the authoritative denominator for progress percentage.
func (s *IngestionTaskService) UpdateComponentTotal(ctx context.Context, taskID string, total int) error {
	return s.ingestionTaskDAO.UpdateComponentTotal(ctx, dao.DB, taskID, total)
}

// RecordComponentProgress appends a component lifecycle row to
// ingestion_task_log (phase: 0 started / 1 done / 2 errored). The row's
// Checkpoint is empty; component progress and step checkpoints are distinct
// row models sharing the same table.
func (s *IngestionTaskService) RecordComponentProgress(ctx context.Context, taskID, component string, phase int, message string) error {
	entry := &entity.IngestionTaskLog{
		TaskID:     taskID,
		Checkpoint: entity.JSONMap{},
		Phase:      phase,
		Component:  component,
		Message:    message,
	}
	return s.ingestionTaskLogDAO.Create(ctx, dao.DB, entry)
}

// ClearComponentProgress removes lifecycle rows left by a previous attempt of
// the same reusable ingestion task. Run-count checkpoint rows are retained.
func (s *IngestionTaskService) ClearComponentProgress(ctx context.Context, taskID string) error {
	_, err := s.ingestionTaskLogDAO.DeleteComponentLogsByTaskID(ctx, dao.DB, taskID)
	return err
}

// AggregateTaskProgress returns the SQL-aggregated component progress for a
// task (done/failed/running/percent against the given total denominator).
func (s *IngestionTaskService) AggregateTaskProgress(ctx context.Context, taskID string, total int) (*dao.TaskProgress, error) {
	return s.ingestionTaskLogDAO.AggregateProgress(ctx, dao.DB, taskID, total)
}

// lastRunCount scans all task logs (newest first) for a run_count entry,
// skipping component-progress rows whose Checkpoint is empty. It returns
// the counter and whether one was found.
func (s *IngestionTaskService) lastRunCount(ctx context.Context, taskID string) (int, bool) {
	logs, err := s.ingestionTaskLogDAO.ListLogsByTaskID(ctx, dao.DB, taskID)
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
// ignored). A failure to persist the new row is returned so the caller can
// fail the task before running the pipeline.
func (s *IngestionTaskService) IncrementRunCount(ctx context.Context, taskID string) error {
	prevCount, _ := s.lastRunCount(ctx, taskID)

	entry := &entity.IngestionTaskLog{
		TaskID:     taskID,
		Checkpoint: entity.JSONMap{stepKeyRunCount: prevCount + 1},
	}
	return s.ingestionTaskLogDAO.Create(ctx, dao.DB, entry)
}
