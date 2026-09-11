package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
		s.advanceEarlyLog(ctx, task, earlyLogFromStarted, string(entity.TaskStatusRunning), earlyLogMsgRunning, true)
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
		// Same reason as RequestStop's CREATED/SCHEDULED branch: the stop
		// finalizes without a worker, so no terminal pipeline-log writer will
		// close the early row. Close it here or it stays RUNNING forever.
		s.advanceEarlyLog(ctx, task, earlyLogFromOpen, string(entity.TaskStatusCancel), earlyLogMsgStopped, false)
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
		s.advanceEarlyLog(ctx, stopped, earlyLogFromStarted, string(entity.TaskStatusCancel), earlyLogMsgStopped, false)
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
	// for this run. Drop its open early row, otherwise the detail page keeps a
	// queued entry.
	s.deleteEarlyLogBestEffort(ctx, task)
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
			// before a crash; the helper adopts it (binding the task to it)
			// instead of opening a second row, then the schedule transition
			// below advances it.
			s.createEarlyLogBestEffort(ctx, existing, string(entity.TaskStatusUnstart), earlyLogMsgQueued, kbCache)
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
			// run's row is terminal, so no open row matches and the helper
			// opens (and binds) a new one.
			s.createEarlyLogBestEffort(ctx, existing, string(entity.TaskStatusUnstart), earlyLogMsgQueued, kbCache)
			if err = s.enqueueTask(existing.ID); err != nil {
				s.deleteEarlyLogBestEffort(ctx, existing)
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
	// Open the pre-terminal row the dataset detail page reads *before*
	// publishing. Once the message is out a worker can claim the task and run
	// it to a terminal state, and the terminal writer must always find the row
	// it is bound to; a row created after publication could be inserted too
	// late and stay queued forever. The prior run (if any) is terminal, so no
	// open row exists and this never duplicates. Best-effort: on a DB blip the
	// row is simply missing and markScheduledAfterPublish re-opens it at "5".
	s.createEarlyLogBestEffort(ctx, created, string(entity.TaskStatusUnstart), earlyLogMsgQueued, kbCache)
	if err = s.enqueueTask(created.ID); err != nil {
		s.deleteEarlyLogBestEffort(ctx, created)
		if rollbackErr := s.rollbackCreatedTask(ctx, created.ID); rollbackErr != nil {
			return nil, fmt.Errorf("enqueue task %s: %w (rollback failed: %w)", created.ID, err, rollbackErr)
		}
		return nil, err
	}
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

// markScheduledAfterPublish records a successful NATS publish. A worker may
// claim the task first and move it to RUNNING; the log write below is scoped to
// the unstarted from-state, so it never regresses a row the worker already
// advanced.
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
		s.advanceEarlyLog(ctx, task, earlyLogFromQueued, string(entity.TaskStatusSchedule), earlyLogMsgQueued, true)
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

// Messages the queued stages show on the dataset detail page.
const (
	earlyLogMsgQueued  = "Task is queued..."
	earlyLogMsgRunning = "Task is running..."
	earlyLogMsgStopped = "Task stopped by user."
)

// earlyLogFromQueued and earlyLogFromStarted are the operation_status values an
// early pipeline-log row may carry when a stage advances it. Scoping each
// advance to its valid from-states keeps the transitions monotonic
// (unstart -> schedule -> running), so a late queued write cannot regress a row
// a worker already moved to running.
var (
	earlyLogFromQueued  = []string{string(entity.TaskStatusUnstart)}
	earlyLogFromStarted = []string{string(entity.TaskStatusUnstart), string(entity.TaskStatusSchedule)}
	// earlyLogFromOpen accepts every pre-terminal state: closing a row to a
	// terminal status is a valid successor of any of them.
	earlyLogFromOpen = dao.PipelineOperationStatusOpen()
)

// createEarlyLogBestEffort opens the pre-terminal row the dataset detail page
// reads and binds it to the task. Best-effort: never fails task creation. The
// caller passes the status/message so a late-created row carries the current
// status.
//
// A run that already owns a row keeps it: a still-open row is reused, and a row
// left over from a previous run (terminal, so no longer open) is replaced.
//
// A run with no bound row must open its own. It must not adopt the document's
// open row: ingestion_task.document_id is unique, so this task is the
// document's only task and any open row for the document therefore belongs to a
// run that no longer exists. Adopting it would inherit stale status, content
// and identity, and a stale RUNNING row could never be advanced again because
// the monotonic from-state guards reject it. Drop the leftover instead, so it
// neither gets reused nor lingers as a permanently queued entry.
func (s *IngestionTaskService) createEarlyLogBestEffort(ctx context.Context, task *entity.IngestionTask, operationStatus, progressMsg string, kbCache map[string]*entity.Knowledgebase) {
	if task == nil || s.pipelineLogDAO == nil {
		return
	}
	if bound := task.PipelineLogID; bound != nil && *bound != "" {
		if s.earlyLogRowIsOpen(ctx, *bound) {
			return
		}
	} else if !s.dropLeftoverEarlyLog(ctx, task.DocumentID) {
		return
	}
	input, err := s.buildEarlyLogInput(ctx, task, operationStatus, progressMsg, kbCache)
	if err != nil {
		common.Warn(fmt.Sprintf("CreateAndEnqueue: build early pipeline log for document %s: %v", task.DocumentID, err))
		return
	}
	log, err := s.pipelineLogDAO.CreateEarlyLog(ctx, dao.DB, input)
	if err != nil {
		common.Warn(fmt.Sprintf("CreateAndEnqueue: create early pipeline log for document %s: %v", task.DocumentID, err))
		return
	}
	s.bindEarlyLog(ctx, task, log.ID)
}

// dropLeftoverEarlyLog removes the open row a document may still carry for a
// run that no longer exists, so an unbound run starts from a clean slate. It
// reports whether the caller may proceed to create its own row: false only when
// the leftover could not be cleared, in which case creating another row would
// leave two open entries for the document.
func (s *IngestionTaskService) dropLeftoverEarlyLog(ctx context.Context, documentID string) bool {
	open, err := s.pipelineLogDAO.GetOpenLogByDocumentID(ctx, dao.DB, documentID)
	if err != nil {
		common.Warn(fmt.Sprintf("CreateAndEnqueue: check open pipeline log for document %s: %v", documentID, err))
		return false
	}
	if open == nil {
		return true
	}
	if err := s.pipelineLogDAO.DeleteOpenLogByID(ctx, dao.DB, open.ID); err != nil {
		common.Warn(fmt.Sprintf("CreateAndEnqueue: drop leftover pipeline log %s for document %s: %v", open.ID, documentID, err))
		return false
	}
	common.Warn(fmt.Sprintf("dropped leftover pipeline log %s for document %s (status %s) before opening a fresh row", open.ID, documentID, open.OperationStatus))
	return true
}

// earlyLogRowIsOpen reports whether the row a task is bound to still exists in
// a pre-terminal state. A missing or already-final row means the run needs a
// fresh one.
func (s *IngestionTaskService) earlyLogRowIsOpen(ctx context.Context, logID string) bool {
	log, err := s.pipelineLogDAO.GetByID(ctx, dao.DB, logID)
	if err != nil {
		if !dao.IsNotFoundErr(err) {
			common.Warn(fmt.Sprintf("CreateAndEnqueue: load pipeline log %s: %v", logID, err))
		}
		return false
	}
	return slices.Contains(dao.PipelineOperationStatusOpen(), log.OperationStatus)
}

// bindEarlyLog records the run's log row on the task so the running advance and
// the terminal write target exactly this row instead of whichever row happens
// to be open for the document. Best-effort.
func (s *IngestionTaskService) bindEarlyLog(ctx context.Context, task *entity.IngestionTask, logID string) {
	if task == nil || logID == "" {
		return
	}
	task.PipelineLogID = &logID
	if err := s.ingestionTaskDAO.UpdatePipelineLogID(ctx, dao.DB, task.ID, logID); err != nil {
		common.Warn(fmt.Sprintf("bind early pipeline log %s to task %s: %v", logID, task.ID, err))
	}
}

// advanceEarlyLog moves the task's own bound row to operationStatus from one of
// fromStatuses. Best-effort: never fails task transitions.
//
// It never looks the row up by document: only the row this run is bound to is
// advanced, so a concurrent run's row can be neither regressed nor adopted. A
// task with no bound row (legacy task, or a lost best-effort create) opens one,
// but only while the task is still live — a terminal task must never resurrect
// a closed row as a permanently queued entry.
func (s *IngestionTaskService) advanceEarlyLog(ctx context.Context, task *entity.IngestionTask, fromStatuses []string, operationStatus, progressMsg string, reopen bool) {
	if task == nil || task.DocumentID == "" || s.pipelineLogDAO == nil {
		return
	}
	logID := ""
	if task.PipelineLogID != nil {
		logID = *task.PipelineLogID
	}
	if logID == "" {
		terminal := task.Status == common.COMPLETED || task.Status == common.STOPPED || task.Status == common.FAILED
		if !reopen || terminal {
			return
		}
		s.createEarlyLogBestEffort(ctx, task, operationStatus, progressMsg, nil)
		return
	}
	if err := s.pipelineLogDAO.AdvanceEarlyLog(ctx, dao.DB, logID, fromStatuses, operationStatus, progressMsg); err != nil {
		common.Warn(fmt.Sprintf("advance early pipeline log for document %s to %s: %v", task.DocumentID, operationStatus, err))
	}
}

// deleteEarlyLogBestEffort drops the open row a run owns, so a rolled-back or
// deleted run leaves no permanently queued entry on the detail page. Only the
// run's own open row goes: a terminal row is history, and a newer run's row is
// not this caller's to drop.
func (s *IngestionTaskService) deleteEarlyLogBestEffort(ctx context.Context, task *entity.IngestionTask) {
	if task == nil || task.PipelineLogID == nil || s.pipelineLogDAO == nil {
		return
	}
	if err := s.pipelineLogDAO.DeleteOpenLogByID(ctx, dao.DB, *task.PipelineLogID); err != nil {
		common.Warn(fmt.Sprintf("CreateAndEnqueue: delete early pipeline log for document %s: %v", task.DocumentID, err))
	}
}

// buildEarlyLogInput resolves the pre-terminal row from the document and its
// knowledge base. Status/message come from the caller so a late-created row
// carries the current status. DSL and progress stay empty for the terminal write.
func (s *IngestionTaskService) buildEarlyLogInput(ctx context.Context, task *entity.IngestionTask, operationStatus, progressMsg string, kbCache map[string]*entity.Knowledgebase) (dao.EarlyLogInput, error) {
	var input dao.EarlyLogInput
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, task.DocumentID)
	if err != nil {
		return input, err
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
	// SplitN always returns at least one part, so the prefix before "/" is
	// always available.
	input.SourceFrom = strings.SplitN(doc.SourceType, "/", 2)[0]
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
