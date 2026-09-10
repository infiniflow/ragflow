package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
const stepKeyRunCount = "run_count"

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
	pipelineLogDAO      *dao.PipelineOperationLogDAO
	kbDAO               *dao.KnowledgebaseDAO
	userCanvasDAO       *dao.UserCanvasDAO
	taskPublisher       TaskPublisher
}

func NewIngestionTaskService() *IngestionTaskService {
	return &IngestionTaskService{
		documentDAO:         dao.NewDocumentDAO(),
		userDAO:             dao.NewUserDAO(),
		ingestionTaskDAO:    dao.NewIngestionTaskDAO(),
		ingestionTaskLogDAO: dao.NewIngestionTaskLogDAO(),
		pipelineLogDAO:      dao.NewPipelineOperationLogDAO(),
		kbDAO:               dao.NewKnowledgebaseDAO(),
		userCanvasDAO:       dao.NewUserCanvasDAO(),
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

	// The knowledge base row is identical for every document in the batch;
	// resolve it once so the early-log bookkeeping below does not re-read it
	// per document.
	kb, err := s.kbDAO.GetByID(ctx, dao.DB, datasetID)
	if err != nil {
		return nil, fmt.Errorf("get knowledgebase %s: %w", datasetID, err)
	}
	if kb == nil {
		return nil, fmt.Errorf("knowledgebase %s not found", datasetID)
	}
	kbCache := map[string]*entity.Knowledgebase{datasetID: kb}

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
		task, err = s.createAndEnqueueWithKBCache(ctx, task, kbCache)
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

func (s *IngestionTaskService) StartRunning(ctx context.Context, taskID string) (*entity.IngestionTask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case common.CREATED, common.SCHEDULED:
		task, err = s.transition(ctx, taskID, common.RUNNING)
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
		s.advanceEarlyLogBestEffort(ctx, task.DocumentID, string(entity.TaskStatusRunning), "Task is running...")
		return task, nil
	case common.STOPPING:
		task, err = s.transition(ctx, taskID, common.STOPPED)
		if err != nil {
			return nil, err
		}
		// The stop is finalized here without a worker (e.g. MQ redelivery of
		// a task that was nacked before execution), so the Redis cancel flag
		// that RequestStop set would otherwise leak until TTL and cancel the
		// next run of this task at the worker's pre-start check.
		clearCancelFlag(ctx, taskID)
		return task, nil
	case common.RUNNING, common.COMPLETED, common.STOPPED, common.FAILED:
		return task, nil
	default:
		return task, fmt.Errorf("task %s has unsupported status %s", taskID, task.Status)
	}
}

func (s *IngestionTaskService) RequestStop(ctx context.Context, taskID string) (*entity.IngestionTask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case common.CREATED, common.SCHEDULED:
		stopped, err := s.transition(ctx, taskID, common.STOPPED)
		if err != nil {
			return nil, err
		}
		// The stop finalizes without a worker (no RUNNING phase, so no
		// terminal pipeline-log writer will run). Advance the early row to
		// CANCEL here, otherwise the detail page keeps a queued entry.
		s.advanceEarlyLogBestEffort(ctx, stopped.DocumentID, string(entity.TaskStatusCancel), "Task stopped by user.")
		return stopped, nil
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

func (s *IngestionTaskService) MarkCompleted(ctx context.Context, taskID string) error {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status == common.COMPLETED || task.Status == common.STOPPED || task.Status == common.FAILED {
		return nil // already terminal, idempotent — mirrors MarkStopped
	}
	_, err = s.transition(ctx, taskID, common.COMPLETED)
	return err
}

func (s *IngestionTaskService) MarkFailed(ctx context.Context, taskID string) error {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status == common.FAILED || task.Status == common.COMPLETED || task.Status == common.STOPPED {
		return nil // already terminal, idempotent — mirrors MarkStopped
	}
	_, err = s.transition(ctx, taskID, common.FAILED)
	return err
}

// MarkStopped transitions the task from STOPPING to STOPPED (terminal).
// Idempotent: returns nil if the task is already in a terminal state
// (STOPPED, COMPLETED, or FAILED).
func (s *IngestionTaskService) MarkStopped(ctx context.Context, taskID string) error {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status == common.STOPPED || task.Status == common.COMPLETED || task.Status == common.FAILED {
		return nil
	}
	_, err = s.transition(ctx, taskID, common.STOPPED)
	return err
}

func (s *IngestionTaskService) Remove(ctx context.Context, taskID string, userID *string) (*dao.TaskInfo, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	info, err := s.ingestionTaskDAO.Delete(ctx, dao.DB, taskID, userID)
	if err != nil {
		return nil, err
	}
	// The task row is gone, so no worker will ever reach the terminal writer
	// for this run. Drop its open early row, otherwise the detail page keeps
	// a queued entry and a later retry's terminal write could adopt it.
	s.deleteEarlyLogBestEffort(ctx, task.DocumentID)
	return info, nil
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
	case common.CREATED:
		if to == common.SCHEDULED || to == common.RUNNING || to == common.STOPPED {
			return nil
		}
	case common.SCHEDULED:
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
	return s.createAndEnqueueWithKBCache(ctx, task, nil)
}

// createAndEnqueueWithKBCache is CreateAndEnqueue with an optional
// knowledge-base cache for batch callers (CreateForDocuments resolves one kb
// for the whole batch). A nil cache behaves exactly like CreateAndEnqueue.
func (s *IngestionTaskService) createAndEnqueueWithKBCache(ctx context.Context, task *entity.IngestionTask, kbCache map[string]*entity.Knowledgebase) (*entity.IngestionTask, error) {
	existing, err := s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, task.DocumentID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		switch existing.Status {
		case common.CREATED:
			// An existing CREATED task may already carry a queued row from
			// before a crash; ensure one exists without duplicating it, then
			// advance it through the schedule transition below.
			s.createEarlyLogBestEffort(ctx, existing, string(entity.TaskStatusUnstart), "Task is queued...", kbCache)
			if err = s.enqueueTask(existing.ID); err != nil {
				return nil, err
			}
			return s.markScheduledAfterPublish(ctx, existing.ID)
		case common.FAILED, common.STOPPED:
			originalStatus := existing.Status
			existing, err = s.transition(ctx, existing.ID, common.CREATED)
			if err != nil {
				return nil, err
			}
			// The previous run is terminal, so any leftover Redis cancel flag
			// is stale: a genuine cancel of the new run can only come through
			// RequestStop once the task is RUNNING again. Clear it so the
			// re-queued task is not cancelled at the worker's pre-start check.
			clearCancelFlag(ctx, existing.ID)
			// A retry starts a fresh pipeline-operation-log row: the prior
			// run's row is terminal and GetOpenLogByDocumentID no longer
			// matches it, so this Create always opens a new row.
			s.createEarlyLogBestEffort(ctx, existing, string(entity.TaskStatusUnstart), "Task is queued...", kbCache)
			if err = s.enqueueTask(existing.ID); err != nil {
				s.deleteEarlyLogBestEffort(ctx, existing.DocumentID)
				if rollbackErr := s.rollbackRetriedTask(ctx, existing.ID, originalStatus); rollbackErr != nil {
					return nil, fmt.Errorf("enqueue task %s: %w (rollback failed: %w)", existing.ID, err, rollbackErr)
				}
				return nil, err
			}
			return s.markScheduledAfterPublish(ctx, existing.ID)
		default:
			return nil, fmt.Errorf("document id %s already exists, status: %s, task id: %s", task.DocumentID, existing.Status, existing.ID)
		}
	}
	task.Status = common.CREATED
	created, err := s.ingestionTaskDAO.Create(ctx, dao.DB, task)
	if err != nil {
		return nil, err
	}
	// Publish first: the worker (or request stop) can only observe the task
	// once the message is out, and the early-log write below deliberately
	// lags so the queued entry never appears before the task is dispatchable.
	// A worker that wins the race lands the task in RUNNING; the terminal
	// writer then Creates its row (no open row yet), which the late
	// markScheduled write must not regress (see below).
	if err = s.enqueueTask(created.ID); err != nil {
		if rollbackErr := s.rollbackCreatedTask(ctx, created.ID); rollbackErr != nil {
			return nil, fmt.Errorf("enqueue task %s: %w (rollback failed: %w)", created.ID, err, rollbackErr)
		}
		return nil, err
	}
	// Open the pre-terminal row the dataset detail page reads. The prior run
	// (if any) is terminal, so no open row exists and this never duplicates.
	// Best-effort: on a DB blip the row is simply missing and the
	// markScheduled fallback below re-opens it at "5".
	s.createEarlyLogBestEffort(ctx, created, string(entity.TaskStatusUnstart), "Task is queued...", kbCache)
	return s.markScheduledAfterPublish(ctx, created.ID)
}

func (s *IngestionTaskService) rollbackRetriedTask(ctx context.Context, taskID, status string) error {
	updated, err := s.ingestionTaskDAO.UpdateStatusIfCurrent(ctx, dao.DB, taskID, common.CREATED, status)
	if err != nil {
		return err
	}
	if !updated {
		return s.newTaskStatusConflictError(ctx, taskID, common.CREATED, status)
	}
	return nil
}

func (s *IngestionTaskService) rollbackCreatedTask(ctx context.Context, taskID string) error {
	_, err := s.ingestionTaskDAO.Delete(ctx, dao.DB, taskID, nil)
	return err
}

// markScheduledAfterPublish records a successful NATS publish. A worker can
// claim the CREATED task before this write and move it to RUNNING, which is
// also a successful outcome. The early-log write below is idempotent in the
// forward direction only: it advances 0->5 and never regresses a row that is
// already RUNNING (a worker that won the publish race).
func (s *IngestionTaskService) markScheduledAfterPublish(ctx context.Context, taskID string) (*entity.IngestionTask, error) {
	updated, err := s.ingestionTaskDAO.UpdateStatusIfCurrent(ctx, dao.DB, taskID, common.CREATED, common.SCHEDULED)
	if err != nil {
		return nil, err
	}
	if updated {
		task, err := s.GetTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		if !s.advanceOrCreateEarlyLogBestEffort(ctx, task.DocumentID, string(entity.TaskStatusSchedule), "Task is queued...") {
			s.reconcileScheduledEarlyLog(ctx, task)
		}
		return task, nil
	}

	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case common.SCHEDULED, common.RUNNING, common.STOPPING, common.COMPLETED, common.FAILED, common.STOPPED:
		return task, nil
	default:
		return nil, s.newTaskStatusConflictError(ctx, taskID, common.CREATED, common.SCHEDULED)
	}
}

// reconcileScheduledEarlyLog repairs the early-log row after a
// CREATED->SCHEDULED transition whose advance/create write did not durably
// record "5". Advance-returns-false happens in two cases: (a) a worker won
// the publish race and the task is already RUNNING — in that case the row is
// either already "1" (nothing to do) or the worker's terminal writer has not
// run yet (its CAS will pick the row up); or (b) the row is genuinely missing
// (a DB blip dropped both the CREATED create and the SCHEDULED advance).
// Case (b) is repaired by re-opening the row at "5"; case (a) must never
// regress a "1" row back to "5", so the repair only fires when no open row
// exists at all.
func (s *IngestionTaskService) reconcileScheduledEarlyLog(ctx context.Context, task *entity.IngestionTask) {
	if task == nil || s.pipelineLogDAO == nil {
		return
	}
	open, err := s.pipelineLogDAO.GetOpenLogByDocumentID(ctx, dao.DB, task.DocumentID)
	if err != nil {
		common.Warn(fmt.Sprintf("reconcile scheduled early log for document %s: %v", task.DocumentID, err))
		return
	}
	if open != nil {
		return
	}
	s.createEarlyLogBestEffort(ctx, task, string(entity.TaskStatusSchedule), "Task is queued...", nil)
}

// ScheduleCreatedTasks publishes the tasks that were persisted before a
// process stopped but were not confirmed as scheduled. It is intended for the
// single startup recovery pass; a publish error leaves the task CREATED for a
// future startup or explicit parse request to retry.
//
// A recovered task may already carry a queued row written before the crash;
// markScheduledAfterPublish advances it in place instead of opening a second
// row, so startup recovery never duplicates the early row.
func (s *IngestionTaskService) ScheduleCreatedTasks(ctx context.Context) error {
	tasks, err := s.ingestionTaskDAO.ListByStatus(ctx, dao.DB, common.CREATED)
	if err != nil {
		return err
	}
	var recoveryErr error
	for _, task := range tasks {
		if err := s.enqueueTask(task.ID); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("schedule created task %s: %w", task.ID, err))
			continue
		}
		if _, err := s.markScheduledAfterPublish(ctx, task.ID); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("mark task %s scheduled: %w", task.ID, err))
		}
	}
	return recoveryErr
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
	return s.taskPublisher.PublishTaskMessage(common.TaskSubject, taskMessage)
}

// createEarlyLogBestEffort opens the pre-terminal pipeline-operation-log row
// the dataset detail page reads. All writes here are best-effort: a DB blip
// must not fail task creation or enqueue and trigger a redelivery loop. When
// an open row already exists (enqueue retry of an existing CREATED task), it
// is left untouched so a run never owns two queued rows. The caller passes
// the status/message the row should carry so a late-created row (e.g. the
// SCHEDULED fallback, or a DB blip during the CREATED write) does not regress
// to an older status.
func (s *IngestionTaskService) createEarlyLogBestEffort(ctx context.Context, task *entity.IngestionTask, operationStatus, progressMsg string, kbCache map[string]*entity.Knowledgebase) {
	if task == nil || s.pipelineLogDAO == nil {
		return
	}
	open, err := s.pipelineLogDAO.GetOpenLogByDocumentID(ctx, dao.DB, task.DocumentID)
	if err != nil {
		common.Warn(fmt.Sprintf("CreateAndEnqueue: check open pipeline log for document %s: %v", task.DocumentID, err))
		return
	}
	if open != nil {
		return
	}
	input, err := s.buildEarlyLogInput(ctx, task, operationStatus, progressMsg, kbCache)
	if err != nil {
		common.Warn(fmt.Sprintf("CreateAndEnqueue: build early pipeline log for document %s: %v", task.DocumentID, err))
		return
	}
	if _, err = s.pipelineLogDAO.CreateEarlyLog(ctx, dao.DB, input); err != nil {
		common.Warn(fmt.Sprintf("CreateAndEnqueue: create early pipeline log for document %s: %v", task.DocumentID, err))
	}
}

// advanceEarlyLogBestEffort moves the open row to a later status. Best-effort:
// a missing open row (legacy run that started before early rows existed) is
// fine — the terminal writer falls back to Create.
func (s *IngestionTaskService) advanceEarlyLogBestEffort(ctx context.Context, documentID, operationStatus, progressMsg string) {
	if documentID == "" || s.pipelineLogDAO == nil {
		return
	}
	if _, err := s.pipelineLogDAO.AdvanceEarlyLog(ctx, dao.DB, documentID, operationStatus, progressMsg); err != nil {
		common.Warn(fmt.Sprintf("advance early pipeline log for document %s to %s: %v", documentID, operationStatus, err))
	}
}

// advanceOrCreateEarlyLogBestEffort advances the open row when one exists and
// opens a fresh row otherwise. Used by the SCHEDULED transition, which is the
// funnel for recovered tasks whose queued row may predate the crash. The
// returned bool reports whether the row was advanced in place (true) or a
// fresh row had to be opened / nothing could be done (false); callers use it
// to decide whether the status was durably recorded.
func (s *IngestionTaskService) advanceOrCreateEarlyLogBestEffort(ctx context.Context, documentID, operationStatus, progressMsg string) bool {
	if documentID == "" || s.pipelineLogDAO == nil {
		return false
	}
	advanced, err := s.pipelineLogDAO.AdvanceEarlyLog(ctx, dao.DB, documentID, operationStatus, progressMsg)
	if err != nil {
		common.Warn(fmt.Sprintf("advance early pipeline log for document %s to %s: %v", documentID, operationStatus, err))
		return false
	}
	if advanced {
		return true
	}
	task, err := s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, documentID)
	if err != nil || task == nil {
		if err != nil {
			common.Warn(fmt.Sprintf("advance early pipeline log: load task for document %s: %v", documentID, err))
		}
		return false
	}
	s.createEarlyLogBestEffort(ctx, task, operationStatus, progressMsg, nil)
	return false
}

// deleteEarlyLogBestEffort removes the open rows for a document. Used when
// task creation rolls back after the early row was already written, so the
// detail page is not left with a permanently queued entry.
func (s *IngestionTaskService) deleteEarlyLogBestEffort(ctx context.Context, documentID string) {
	if documentID == "" || s.pipelineLogDAO == nil {
		return
	}
	if err := s.pipelineLogDAO.DeleteOpenLogsByDocumentID(ctx, dao.DB, documentID); err != nil {
		common.Warn(fmt.Sprintf("CreateAndEnqueue: delete early pipeline log for document %s: %v", documentID, err))
	}
}

// buildEarlyLogInput assembles the bookkeeping for the pre-terminal row from
// the document and its knowledge base. It mirrors the identity resolution in
// the terminal writer (parser_id fallback title, canvas title/avatar override,
// source_from prefix) but leaves DSL and progress for the terminal write.
// The caller supplies the status/message so a row created late (e.g. the
// SCHEDULED fallback path) carries the current status, not UNSTART.
func (s *IngestionTaskService) buildEarlyLogInput(ctx context.Context, task *entity.IngestionTask, operationStatus, progressMsg string, kbCache map[string]*entity.Knowledgebase) (dao.EarlyLogInput, error) {
	var input dao.EarlyLogInput
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, task.DocumentID)
	if err != nil {
		return input, err
	}
	if doc == nil {
		return input, fmt.Errorf("document %s not found", task.DocumentID)
	}
	kbID := task.DatasetID
	if kbID == "" {
		kbID = doc.KbID
	}
	kb := kbCache[kbID]
	if kb == nil {
		kb, err = s.kbDAO.GetByID(ctx, dao.DB, kbID)
		if err != nil {
			return input, err
		}
		if kb == nil {
			return input, fmt.Errorf("knowledgebase %s not found", kbID)
		}
		if kbCache != nil {
			kbCache[kbID] = kb
		}
	}
	input = dao.EarlyLogInput{
		DocumentID:      doc.ID,
		KbID:            kbID,
		TenantID:        kb.TenantID,
		ParserID:        doc.ParserID,
		DocumentSuffix:  doc.Suffix,
		DocumentType:    doc.Type,
		OperationStatus: operationStatus,
		ProgressMsg:     progressMsg,
		PipelineTitle:   doc.ParserID,
		Avatar:          doc.Thumbnail,
	}
	if doc.Name != nil {
		input.DocumentName = *doc.Name
	}
	if parts := strings.SplitN(doc.SourceType, "/", 2); len(parts) > 0 {
		input.SourceFrom = parts[0]
	} else {
		input.SourceFrom = doc.SourceType
	}
	if doc.PipelineID != nil {
		if pipelineID := strings.TrimSpace(*doc.PipelineID); pipelineID != "" {
			input.PipelineID = pipelineID
			if canvas, err := s.userCanvasDAO.GetByID(ctx, dao.DB, pipelineID); err == nil && canvas != nil {
				if canvas.Title != nil && *canvas.Title != "" {
					input.PipelineTitle = *canvas.Title
				}
				input.Avatar = canvas.Avatar
			} else if err != nil && !errors.Is(err, dao.ErrUserCanvasNotFound) {
				common.Warn(fmt.Sprintf("CreateAndEnqueue: load pipeline %s for early log: %v", pipelineID, err))
			}
		}
	}
	return input, nil
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
