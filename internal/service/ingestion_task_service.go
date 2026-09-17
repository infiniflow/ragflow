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
	"ragflow/internal/observability"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ingestionEventKind intentionally has an invalid zero value. The persisted
// event protocol starts at zero for lifecycle events, so callers cannot pass a
// Go zero value and silently create a lifecycle row.
type ingestionEventKind uint8

const (
	ingestionEventInvalid ingestionEventKind = iota
	ingestionEventLifecycle
	ingestionEventMessage
	ingestionEventTerminal
	ingestionEventSystem
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

// InvalidRunIdentityError reports a durable invariant violation discovered
// after a worker moves a task to RUNNING. Callers distinguish it from storage
// errors: the former must be settled as FAILED, while the latter is retried.
type InvalidRunIdentityError struct {
	TaskID string
	Reason string
}

func (e *InvalidRunIdentityError) Error() string {
	return fmt.Sprintf("task %s has invalid run identity: %s", e.TaskID, e.Reason)
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
	cleanupClaimDAO     *dao.DocumentCleanupClaimDAO
	kbDAO               *dao.KnowledgebaseDAO
	userCanvasDAO       *dao.UserCanvasDAO
	taskPublisher       TaskPublisher
	logSettings         IngestionLogSettings
}

type cleanupClaimContextKey struct{}

type cleanupClaimContextValue struct {
	DocumentID string
	Token      string
}

// WithDocumentCleanupClaim marks a new-run request as the current cleanup
// owner. It is only used by the document cleanup workflow before enqueueing.
func WithDocumentCleanupClaim(ctx context.Context, documentID, token string) context.Context {
	return context.WithValue(ctx, cleanupClaimContextKey{}, cleanupClaimContextValue{
		DocumentID: documentID,
		Token:      token,
	})
}

func cleanupClaimFromContext(ctx context.Context, documentID string) (string, bool) {
	claim, ok := ctx.Value(cleanupClaimContextKey{}).(cleanupClaimContextValue)
	return claim.Token, ok && claim.DocumentID == documentID && claim.Token != ""
}

// DocumentCleanupClaimFromContext returns the fencing token attached to a
// document cleanup request. Storage-facing cleanup code uses this accessor to
// renew and verify ownership around each bounded external operation without
// exposing the context key or value type.
func DocumentCleanupClaimFromContext(ctx context.Context, documentID string) (string, bool) {
	return cleanupClaimFromContext(ctx, documentID)
}

func NewIngestionTaskService() *IngestionTaskService {
	return &IngestionTaskService{
		documentDAO:         dao.NewDocumentDAO(),
		userDAO:             dao.NewUserDAO(),
		ingestionTaskDAO:    dao.NewIngestionTaskDAO(),
		ingestionTaskLogDAO: dao.NewIngestionTaskLogDAO(),
		pipelineLogDAO:      dao.NewPipelineOperationLogDAO(),
		cleanupClaimDAO:     dao.NewDocumentCleanupClaimDAO(),
		kbDAO:               dao.NewKnowledgebaseDAO(),
		userCanvasDAO:       dao.NewUserCanvasDAO(),
		taskPublisher:       NewMessageQueueTaskPublisher(),
		logSettings:         DefaultIngestionLogSettings(),
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

	// Populate this cache lazily so a batch avoids repeated knowledge-base reads.
	kbCache := make(map[string]*entity.Knowledgebase)

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

		if task.PipelineLogID != nil && *task.PipelineLogID != "" {
			if run, runErr := s.pipelineLogDAO.GetByID(ctx, dao.DB, *task.PipelineLogID); runErr == nil && run.RunCount != nil && *run.RunCount > 0 {
				showTask["run_count"] = *run.RunCount
			}
		}

		showTask["component_total"] = task.ComponentTotal
		if task.ComponentTotal > 0 {
			progress := (*dao.TaskProgress)(nil)
			if task.PipelineLogID != nil && *task.PipelineLogID != "" {
				progress, err = s.ingestionTaskLogDAO.AggregateProgressByPipelineLogID(ctx, dao.DB, *task.PipelineLogID, task.ComponentTotal)
			} else {
				err = errors.New("task has no pipeline log identity")
			}
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

// TransitionTaskToRunning performs only the task-state transition required
// before worker identity validation. It intentionally does not reset the
// document or advance a pipeline log.
func (s *IngestionTaskService) TransitionTaskToRunning(ctx context.Context, taskID string) (*entity.IngestionTask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case common.CREATED, common.SCHEDULED:
		task, err = s.transitionFrom(ctx, taskID, []string{common.CREATED, common.SCHEDULED}, common.RUNNING)
		if err != nil {
			return nil, err
		}
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
		// close the open row. Close it here or it stays RUNNING forever.
		s.advanceOpenLog(ctx, task, dao.OpenPipelineOperationStatuses(), string(entity.TaskStatusCancel))
		s.recordRunTerminal(ctx, task, "Task stopped by user.")
		return task, nil
	case common.RUNNING, common.COMPLETED, common.STOPPED, common.FAILED:
		return task, nil
	default:
		return task, fmt.Errorf("task %s has unsupported status %s", taskID, task.Status)
	}
}

// PrepareValidatedRun performs the document and run-log initialization only
// after ReloadAndValidateRunIdentity has accepted the task's captured binding.
func (s *IngestionTaskService) PrepareValidatedRun(ctx context.Context, task *entity.IngestionTask) {
	if task == nil {
		return
	}
	if err := s.documentDAO.UpdateByID(ctx, dao.DB, task.DocumentID, map[string]interface{}{
		"progress":         float64(0),
		"chunk_num":        int64(0),
		"token_num":        int64(0),
		"process_begin_at": time.Now(),
	}); err != nil {
		common.Warn(fmt.Sprintf("prepare validated run: mark document %s running for task %s: %v", task.DocumentID, task.ID, err))
	}
	s.advanceOpenLog(ctx, task, logFromUnstartOrScheduled, string(entity.TaskStatusRunning))
	s.recordRunMessage(ctx, task, "Task is running...")
}

func (s *IngestionTaskService) RequestStop(ctx context.Context, taskID string) (*entity.IngestionTask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	switch task.Status {
	case common.CREATED, common.SCHEDULED:
		stopped, err := s.transitionFrom(ctx, taskID, []string{common.CREATED, common.SCHEDULED}, common.STOPPED)
		if err != nil {
			return nil, err
		}
		// The stop finalizes without a worker (no RUNNING phase, so no
		// terminal pipeline-log writer will run). Advance the open row to
		// CANCEL here, otherwise the detail page keeps a queued entry.
		s.advanceOpenLog(ctx, stopped, logFromUnstartOrScheduled, string(entity.TaskStatusCancel))
		s.recordRunTerminal(ctx, stopped, "Task stopped by user.")
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

// SupersedeUnstartedTask closes a queued run before a replacement parse is
// created. A numbered pipeline log is an immutable run ledger, so this must
// record its cancellation rather than deleting the row with the task.
func (s *IngestionTaskService) SupersedeUnstartedTask(ctx context.Context, taskID string) error {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	switch task.Status {
	case common.CREATED, common.SCHEDULED:
		stopped, err := s.transition(ctx, taskID, common.STOPPED)
		if err != nil {
			return err
		}
		s.advanceOpenLog(ctx, stopped, dao.OpenPipelineOperationStatuses(), string(entity.TaskStatusCancel))
		s.recordRunTerminal(ctx, stopped, "Task superseded by a new parse request.")
		return nil
	case common.RUNNING, common.STOPPING:
		return fmt.Errorf("task %s is %s and cannot be superseded", taskID, task.Status)
	default:
		return nil
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
	if userID != nil && task.UserID != *userID {
		return nil, errors.New("task does not belong to the user")
	}
	// Numbered pipeline logs are immutable run ledgers. A queued run must be
	// settled before its task row is deleted, otherwise no worker remains to
	// write the cancellation terminal event and the log stays open forever.
	if task.Status == common.CREATED || task.Status == common.SCHEDULED {
		if task.PipelineLogID != nil && *task.PipelineLogID != "" {
			run, runErr := s.pipelineLogDAO.GetByID(ctx, dao.DB, *task.PipelineLogID)
			if runErr != nil {
				return nil, runErr
			}
			if run.RunCount != nil && *run.RunCount > 0 {
				if err := s.SupersedeUnstartedTask(ctx, taskID); err != nil {
					return nil, err
				}
			}
		}
	}
	info, err := s.ingestionTaskDAO.Delete(ctx, dao.DB, taskID, userID)
	if err != nil {
		return nil, err
	}
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

// ReloadAndValidateRunIdentity reloads a worker task and verifies that its
// immutable run binding exists, belongs to the same document/dataset, and has
// a positive Go-owned display number. It performs no writes and never creates
// a replacement row; callers can safely retry transient database errors.
func (s *IngestionTaskService) ReloadAndValidateRunIdentity(ctx context.Context, taskID string) (*entity.IngestionTask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.PipelineLogID == nil || *task.PipelineLogID == "" {
		return nil, &InvalidRunIdentityError{TaskID: taskID, Reason: "missing_pipeline_log_id"}
	}
	run, err := s.pipelineLogDAO.GetByID(ctx, dao.DB, *task.PipelineLogID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, &InvalidRunIdentityError{TaskID: taskID, Reason: "pipeline_log_not_found"}
		}
		return nil, err
	}
	if run.DocumentID != task.DocumentID {
		return nil, &InvalidRunIdentityError{TaskID: taskID, Reason: "pipeline_log_document_mismatch"}
	}
	if run.KbID != task.DatasetID {
		return nil, &InvalidRunIdentityError{TaskID: taskID, Reason: "pipeline_log_dataset_mismatch"}
	}
	if run.RunCount == nil || *run.RunCount <= 0 {
		return nil, &InvalidRunIdentityError{TaskID: taskID, Reason: "invalid_run_count"}
	}
	return task, nil
}

func validateTransition(from, to string) error {
	switch from {
	case common.CREATED:
		if to == common.SCHEDULED || to == common.RUNNING || to == common.STOPPED || to == common.FAILED {
			return nil
		}
	case common.SCHEDULED:
		if to == common.RUNNING || to == common.STOPPED || to == common.FAILED {
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

func (s *IngestionTaskService) transitionFrom(ctx context.Context, taskID string, fromStatuses []string, to string) (*entity.IngestionTask, error) {
	for _, from := range fromStatuses {
		if err := validateTransition(from, to); err != nil {
			var transitionErr *InvalidTaskTransitionError
			if errors.As(err, &transitionErr) {
				return nil, &InvalidTaskTransitionError{TaskID: taskID, From: transitionErr.From, To: transitionErr.To}
			}
			return nil, err
		}
	}
	updated, err := s.ingestionTaskDAO.UpdateStatusIfCurrent(ctx, dao.DB, taskID, fromStatuses, to)
	if err != nil {
		return nil, err
	}
	if !updated {
		expected := strings.Join(fromStatuses, "/")
		return nil, s.newTaskStatusConflictError(ctx, taskID, expected, to)
	}
	return s.GetTask(ctx, taskID)
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
	updated, err := s.ingestionTaskDAO.UpdateStatusIfCurrent(ctx, dao.DB, taskID, []string{task.Status}, to)
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
			if err = s.ensureRunIdentity(ctx, existing, kbCache); err != nil {
				return nil, fmt.Errorf("ensure run identity for task %s: %w", existing.ID, err)
			}
			if err = s.enqueueTask(existing.ID); err != nil {
				s.settlePublishFailure(ctx, existing)
				return nil, err
			}
			return s.markScheduledAfterPublish(ctx, existing.ID)
		case common.FAILED, common.STOPPED:
			originalStatus := existing.Status
			existing, err = s.transition(ctx, existing.ID, common.CREATED)
			if err != nil {
				return nil, err
			}
			if err = s.ingestionTaskDAO.ClearPipelineLogID(ctx, dao.DB, existing.ID); err != nil {
				return nil, fmt.Errorf("clear previous run identity for task %s: %w", existing.ID, err)
			}
			existing.PipelineLogID = nil
			// The previous run is terminal, so any leftover Redis cancel flag
			// is stale: a genuine cancel of the new run can only come through
			// RequestStop once the task is RUNNING again. Clear it so the
			// re-queued task is not cancelled at the worker's pre-start check.
			clearCancelFlag(ctx, existing.ID)
			if err = s.ensureRunIdentity(ctx, existing, kbCache); err != nil {
				if rollbackErr := s.rollbackRetriedTask(ctx, existing.ID, originalStatus); rollbackErr != nil {
					return nil, fmt.Errorf("ensure run identity for task %s: %w (rollback failed: %v)", existing.ID, err, rollbackErr)
				}
				return nil, fmt.Errorf("ensure run identity for task %s: %w", existing.ID, err)
			}
			if err = s.enqueueTask(existing.ID); err != nil {
				s.settlePublishFailure(ctx, existing)
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
	if err = s.ensureRunIdentity(ctx, created, kbCache); err != nil {
		if rollbackErr := s.rollbackCreatedTask(ctx, created.ID); rollbackErr != nil {
			return nil, fmt.Errorf("ensure run identity for task %s: %w (rollback failed: %v)", created.ID, err, rollbackErr)
		}
		return nil, fmt.Errorf("ensure run identity for task %s: %w", created.ID, err)
	}
	if err = s.enqueueTask(created.ID); err != nil {
		s.settlePublishFailure(ctx, created)
		return nil, err
	}
	return s.markScheduledAfterPublish(ctx, created.ID)
}

func (s *IngestionTaskService) rollbackRetriedTask(ctx context.Context, taskID, status string) error {
	updated, err := s.ingestionTaskDAO.UpdateStatusIfCurrent(ctx, dao.DB, taskID, []string{common.CREATED}, status)
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
	updated, err := s.ingestionTaskDAO.UpdateStatusIfCurrent(ctx, dao.DB, taskID, []string{common.CREATED}, common.SCHEDULED)
	if err != nil {
		return nil, err
	}
	if updated {
		task, err := s.GetTask(ctx, taskID)
		if err != nil {
			return nil, err
		}
		s.advanceOpenLog(ctx, task, logFromUnstart, string(entity.TaskStatusSchedule))
		s.recordRunMessage(ctx, task, "Task is queued...")
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
// row, so startup recovery never duplicates the open row.
func (s *IngestionTaskService) ScheduleCreatedTasks(ctx context.Context) error {
	tasks, err := s.ingestionTaskDAO.ListByStatus(ctx, dao.DB, common.CREATED)
	if err != nil {
		return err
	}
	var recoveryErr error
	for _, task := range tasks {
		if err := s.ensureRunIdentity(ctx, task, nil); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("ensure run identity for created task %s: %w", task.ID, err))
			continue
		}
		if err := s.enqueueTask(task.ID); err != nil {
			s.settlePublishFailure(ctx, task)
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("schedule created task %s: %w", task.ID, err))
			continue
		}
		if _, err := s.markScheduledAfterPublish(ctx, task.ID); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("mark task %s scheduled: %w", task.ID, err))
		}
	}
	return recoveryErr
}

// settlePublishFailure closes the already-numbered run after a broker publish
// failure. The number is an immutable document ledger entry, so rolling the
// task back or deleting the log would allow a future request to reuse it.
func (s *IngestionTaskService) settlePublishFailure(ctx context.Context, task *entity.IngestionTask) {
	if task == nil {
		return
	}
	if err := s.MarkFailed(ctx, task.ID); err != nil {
		common.Error(fmt.Sprintf("mark task %s failed after publish failure", task.ID), err)
		return
	}
	task.Status = common.FAILED
	s.advanceOpenLog(ctx, task, dao.OpenPipelineOperationStatuses(), string(entity.TaskStatusFail))
	s.recordRunTerminal(ctx, task, "Task publish failed.")
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

// logFromUnstart and logFromUnstartOrScheduled are the operation_status values
// an open pipeline-log row may carry when a stage advances it. Scoping each
// advance to its valid from-states keeps the transitions monotonic
// (unstart -> schedule -> running), so a late queued write cannot regress a row
// a worker already moved to running. Closing a row to a terminal status accepts
// any open state, i.e. dao.OpenPipelineOperationStatuses itself.
var (
	logFromUnstart            = []string{string(entity.TaskStatusUnstart)}
	logFromUnstartOrScheduled = []string{string(entity.TaskStatusUnstart), string(entity.TaskStatusSchedule)}
)

// ensureRunIdentity is the only creation path for a Go ingestion run. It is
// called before publication, never by a worker. A document row lock serializes
// MAX(run_count)+1 allocation, then the task is bound to the created row in
// the same short transaction.
func (s *IngestionTaskService) ensureRunIdentity(ctx context.Context, task *entity.IngestionTask, kbCache map[string]*entity.Knowledgebase) error {
	if task == nil || task.ID == "" || task.DocumentID == "" || s.pipelineLogDAO == nil {
		return errors.New("run identity requires a task, document, and pipeline log DAO")
	}
	input, err := s.buildOpenLogInput(ctx, task, kbCache)
	if err != nil {
		return err
	}
	input.OperationStatus = string(entity.TaskStatusUnstart)
	var boundLogID string
	err = dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedDocument, err := s.documentDAO.GetByIDForUpdate(ctx, tx, task.DocumentID)
		if err != nil {
			return err
		}
		now, err := dao.CurrentUnixTime(ctx, tx)
		if err != nil {
			return err
		}
		claim, err := s.cleanupClaimDAO.GetBlocking(ctx, tx, lockedDocument.ID, now, dao.DefaultDocumentCleanupTakeoverGraceSeconds)
		if err != nil {
			return err
		}
		if claim != nil {
			token, owned := cleanupClaimFromContext(ctx, lockedDocument.ID)
			if !owned || token != claim.Token {
				return fmt.Errorf("document %s cleanup claim is active", lockedDocument.ID)
			}
		}
		lockedTask, err := s.ingestionTaskDAO.GetByIDForUpdate(ctx, tx, task.ID)
		if err != nil {
			return err
		}
		if lockedTask.DocumentID != lockedDocument.ID || lockedTask.DatasetID != input.KbID {
			return fmt.Errorf("task %s no longer matches document or dataset", lockedTask.ID)
		}
		if isTerminalIngestionTask(lockedTask.Status) {
			return fmt.Errorf("task %s is terminal", lockedTask.ID)
		}
		if lockedTask.PipelineLogID != nil && *lockedTask.PipelineLogID != "" {
			bound, err := s.pipelineLogDAO.GetByID(ctx, tx, *lockedTask.PipelineLogID)
			if err != nil {
				return err
			}
			if bound.DocumentID != lockedTask.DocumentID || bound.KbID != input.KbID || bound.RunCount == nil || *bound.RunCount <= 0 {
				return fmt.Errorf("task %s has invalid pipeline run binding %s", lockedTask.ID, bound.ID)
			}
			boundLogID = bound.ID
			return nil
		}

		runCount, err := s.pipelineLogDAO.NextRunCount(ctx, tx, lockedTask.DocumentID)
		if err != nil {
			return err
		}
		input.RunCount = runCount
		log, err := s.pipelineLogDAO.CreateOpenLog(ctx, tx, input)
		if err != nil {
			return err
		}
		if err := s.ingestionTaskDAO.UpdatePipelineLogID(ctx, tx, lockedTask.ID, log.ID); err != nil {
			return err
		}
		boundLogID = log.ID
		return nil
	})
	if err != nil {
		return err
	}
	if boundLogID == "" {
		return errors.New("run identity was not bound")
	}
	task.PipelineLogID = &boundLogID
	return nil
}

func isTerminalIngestionTask(status string) bool {
	return status == common.COMPLETED || status == common.STOPPED || status == common.FAILED
}

// advanceOpenLog moves a task's already-bound row to operationStatus. It never
// creates or adopts a row, so a worker cannot repair a missing identity or
// affect a newer run.
func (s *IngestionTaskService) advanceOpenLog(ctx context.Context, task *entity.IngestionTask, fromStatuses []string, operationStatus string) {
	if task == nil || task.DocumentID == "" || s.pipelineLogDAO == nil {
		return
	}
	logID := ""
	if task.PipelineLogID != nil {
		logID = *task.PipelineLogID
	}
	if logID == "" {
		common.Warn(fmt.Sprintf("advance open pipeline log for task %s: missing run identity", task.ID))
		return
	}
	if err := s.pipelineLogDAO.AdvanceOpenLog(ctx, dao.DB, logID, fromStatuses, operationStatus); err != nil {
		common.Warn(fmt.Sprintf("advance open pipeline log for document %s to %s: %v", task.DocumentID, operationStatus, err))
	}
}

func (s *IngestionTaskService) recordRunMessage(ctx context.Context, task *entity.IngestionTask, message string) {
	if task == nil || task.PipelineLogID == nil || *task.PipelineLogID == "" {
		return
	}
	if err := s.RecordMessage(ctx, *task.PipelineLogID, task.ID, message); err != nil {
		common.Warn(fmt.Sprintf("record run message for task %s: %v", task.ID, err))
	}
}

func (s *IngestionTaskService) recordRunTerminal(ctx context.Context, task *entity.IngestionTask, message string) {
	if task == nil || task.PipelineLogID == nil || *task.PipelineLogID == "" {
		return
	}
	if err := s.RecordTerminal(ctx, *task.PipelineLogID, task.ID, message); err != nil {
		common.Warn(fmt.Sprintf("record run terminal event for task %s: %v", task.ID, err))
	}
}

// buildOpenLogInput resolves immutable run metadata from the document and its
// knowledge base. The queued status and allocated run number are assigned by
// EnsureRunIdentity in its short database transaction.
func (s *IngestionTaskService) buildOpenLogInput(ctx context.Context, task *entity.IngestionTask, kbCache map[string]*entity.Knowledgebase) (dao.OpenLogInput, error) {
	var input dao.OpenLogInput
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
	input = dao.OpenLogInput{
		DocumentID:     doc.ID,
		KbID:           kbID,
		TenantID:       kb.TenantID,
		ParserID:       doc.ParserID,
		DocumentSuffix: doc.Suffix,
		DocumentType:   doc.Type,
		PipelineTitle:  doc.ParserID,
		Avatar:         doc.Thumbnail,
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
				common.Warn(fmt.Sprintf("CreateAndEnqueue: load pipeline %s for open log: %v", pipelineID, err))
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

// RecordLifecycle writes one component lifecycle event for an already
// validated run. The worker supplies the captured pipeline log ID, never a
// value reloaded from the mutable task row.
func (s *IngestionTaskService) RecordLifecycle(ctx context.Context, pipelineLogID, taskID, component string, phase int, message string) error {
	if component == "" {
		return rejectIngestionEvent("missing_component", pipelineLogID, taskID, errors.New("ingestion lifecycle event requires a component"))
	}
	if phase < 0 || phase > 2 {
		return rejectIngestionEvent("invalid_phase", pipelineLogID, taskID, fmt.Errorf("ingestion lifecycle event has invalid phase %d", phase))
	}
	return s.insertEvent(ctx, ingestionEventLifecycle, pipelineLogID, taskID, component, phase, message)
}

// RecordMessage writes supplemental process detail without affecting component
// progress aggregation.
func (s *IngestionTaskService) RecordMessage(ctx context.Context, pipelineLogID, taskID, message string) error {
	return s.insertEvent(ctx, ingestionEventMessage, pipelineLogID, taskID, "", 0, message)
}

// RecordTerminal writes the terminal explanation associated with a run.
func (s *IngestionTaskService) RecordTerminal(ctx context.Context, pipelineLogID, taskID, message string) error {
	if err := s.insertEvent(ctx, ingestionEventTerminal, pipelineLogID, taskID, "", 0, message); err != nil {
		return err
	}
	foldContext := context.WithoutCancel(ctx)
	foldStarted := time.Now()
	foldResult, foldErr := s.foldIngestionRun(foldContext, pipelineLogID, s.logSettings.MaxRowsPerRun)
	logIngestionFoldResult(taskID, pipelineLogID, foldResult, time.Since(foldStarted), foldErr)
	if foldErr != nil {
		common.Warn(fmt.Sprintf("fold terminal ingestion run %s: %v", pipelineLogID, foldErr))
	}
	run, err := s.pipelineLogDAO.GetByID(foldContext, dao.DB, pipelineLogID)
	if err != nil {
		common.Warn(fmt.Sprintf("load terminal ingestion run %s for document trimming: %v", pipelineLogID, err))
		logIngestionTrimResult("", taskID, pipelineLogID, ingestionDocumentTrimResult{}, 0, err)
		return nil
	}
	trimStarted := time.Now()
	trimResult, trimErr := s.trimIngestionDocument(foldContext, run.DocumentID, pipelineLogID, s.logSettings.MaxRowsPerDocument)
	logIngestionTrimResult(run.DocumentID, taskID, pipelineLogID, trimResult, time.Since(trimStarted), trimErr)
	if trimErr != nil {
		common.Warn(fmt.Sprintf("trim ingestion document %s after terminal run %s: %v", run.DocumentID, pipelineLogID, trimErr))
	}
	return nil
}

func rejectIngestionEvent(reason, pipelineLogID, taskID string, err error) error {
	observability.RecordIngestionLogEventRejected(reason)
	common.Warn("ingestion_log_event_rejected",
		zap.String("event", "ingestion_log_event_rejected"),
		zap.String("reason", reason),
		zap.String("pipeline_log_id", pipelineLogID),
		zap.String("task_id", taskID),
	)
	return err
}

func (s *IngestionTaskService) insertEvent(ctx context.Context, kind ingestionEventKind, pipelineLogID, taskID, component string, phase int, message string) error {
	if pipelineLogID == "" {
		return rejectIngestionEvent("missing_pipeline_log_id", pipelineLogID, taskID, errors.New("ingestion event requires a pipeline log id"))
	}
	if taskID == "" {
		return rejectIngestionEvent("missing_task_id", pipelineLogID, taskID, errors.New("ingestion event requires a task id"))
	}
	eventType := 0
	switch kind {
	case ingestionEventLifecycle:
		eventType = dao.EventTypeLifecycle
	case ingestionEventMessage:
		eventType = dao.EventTypeMessage
	case ingestionEventTerminal:
		eventType = dao.EventTypeTerminal
	case ingestionEventSystem:
		eventType = dao.EventTypeSystem
	default:
		return rejectIngestionEvent("invalid_kind", pipelineLogID, taskID, errors.New("ingestion event has invalid kind"))
	}
	if kind != ingestionEventLifecycle {
		component = ""
		phase = 0
	}
	message = s.truncateIngestionEventMessage(message)
	now := time.Now().Local()
	// EventTypeLifecycle is zero while the database default is the defensive
	// legacy value. A map keeps the explicitly mapped protocol value intact;
	// GORM otherwise substitutes a default-tag value for a zero struct field.
	return dao.DB.WithContext(ctx).Model(&entity.IngestionTaskLog{}).Create(map[string]interface{}{
		"task_id":         taskID,
		"pipeline_log_id": pipelineLogID,
		"checkpoint":      entity.JSONMap{},
		"phase":           phase,
		"event_type":      eventType,
		"component":       component,
		"message":         message,
		"create_time":     now.UnixMilli(),
		"create_date":     now.Truncate(time.Second),
		"update_time":     now.UnixMilli(),
		"update_date":     now.Truncate(time.Second),
	}).Error
}

// truncateIngestionEventMessage enforces both limits on the persisted text.
// The marker is part of the limit, and the dropped count is measured in runes
// so the result never splits a UTF-8 sequence or misreports multibyte text.
func (s *IngestionTaskService) truncateIngestionEventMessage(message string) string {
	limits := s.logSettings
	if limits.MaxMessageChars <= 0 || limits.MaxMessageBytes <= 0 {
		limits = DefaultIngestionLogSettings()
	}
	if len([]rune(message)) <= limits.MaxMessageChars && len([]byte(message)) <= limits.MaxMessageBytes {
		return message
	}

	runes := []rune(message)
	prefixLen := len(runes)
	if prefixLen > limits.MaxMessageChars {
		prefixLen = limits.MaxMessageChars
	}
	for prefixLen >= 0 {
		dropped := len(runes) - prefixLen
		marker := fmt.Sprintf("… [truncated, %d chars dropped]", dropped)
		candidate := string(runes[:prefixLen]) + marker
		if len([]rune(candidate)) <= limits.MaxMessageChars && len([]byte(candidate)) <= limits.MaxMessageBytes {
			return candidate
		}
		prefixLen--
	}

	// The marker is tiny relative to the configured limits for any practical
	// input. Keep a defensive fallback for an unexpectedly huge rune count.
	marker := fmt.Sprintf("… [truncated, %d chars dropped]", len(runes))
	if len([]rune(marker)) > limits.MaxMessageChars {
		return string([]rune(marker)[:limits.MaxMessageChars])
	}
	return marker
}

// AggregateTaskProgressByPipelineLogID returns component progress for one
// immutable ingestion run.
func (s *IngestionTaskService) AggregateTaskProgressByPipelineLogID(ctx context.Context, pipelineLogID string, total int) (*dao.TaskProgress, error) {
	return s.ingestionTaskLogDAO.AggregateProgressByPipelineLogID(ctx, dao.DB, pipelineLogID, total)
}
