package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/entity"
	ingestionpipeline "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/observability"
	"ragflow/internal/permission"
	"ragflow/internal/utility"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

const (
	taskStateCleanupTimeout     = 5 * time.Second
	taskStateCleanupCapacity    = 1024
	taskTerminalMessageComplete = "Task completed."
	taskTerminalMessageFailed   = "Task failed."
)

type taskStatePurgeRequest struct {
	ctx       context.Context
	taskID    string
	purgeTask func(context.Context, string) error
}

var taskStateCleanupQueue = struct {
	once  sync.Once
	queue chan taskStatePurgeRequest
}{
	queue: make(chan taskStatePurgeRequest, taskStateCleanupCapacity),
}

func scheduleTaskStatePurge(req taskStatePurgeRequest) bool {
	taskStateCleanupQueue.once.Do(func() {
		go func() {
			for req := range taskStateCleanupQueue.queue {
				started := time.Now()
				ctx, cancel := context.WithTimeout(context.WithoutCancel(req.ctx), taskStateCleanupTimeout)
				err := req.purgeTask(ctx, req.taskID)
				cancel()
				elapsed := time.Since(started)
				if err != nil {
					common.Warn("purge task state after delete",
						zap.String("task_id", req.taskID),
						zap.Duration("elapsed", elapsed),
						zap.Error(err),
					)
					continue
				}
				common.Debug("purged task state after delete",
					zap.String("task_id", req.taskID),
					zap.Duration("elapsed", elapsed),
				)
			}
		}()
	})
	select {
	case taskStateCleanupQueue.queue <- req:
		return true
	default:
		return false
	}
}

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

// tierResolver resolves the ingestion queue tier for a tenant. It is an
// injected seam so tests can drive paid/free/error outcomes without a live
// billing service. A nil field falls back to defaultTierResolver, whose
// implementation is edition-specific (ee uses billing; community is single
// tier).
type tierResolver func(ctx context.Context, tenantID string) (common.Tier, error)

type IngestionTaskService struct {
	documentDAO      *dao.DocumentDAO
	ingestionTaskDAO *dao.IngestionTaskDAO
	pipelineLogDAO   *dao.PipelineOperationLogDAO
	kbDAO            *dao.KnowledgebaseDAO
	userCanvasDAO    *dao.UserCanvasDAO
	taskPublisher    TaskPublisher
	logSettings      IngestionLogSettings
	eventEmitter     observability.IngestionEventEmitter
	// purgeTaskState removes the deleted task's run-scoped chunk and media
	// cache state. Nil resolves to the process-wide implementation.
	purgeTaskState func(context.Context, string) error
	tierResolver   tierResolver
}

func NewIngestionTaskService() *IngestionTaskService {
	return &IngestionTaskService{
		documentDAO:      dao.NewDocumentDAO(),
		ingestionTaskDAO: dao.NewIngestionTaskDAO(),
		pipelineLogDAO:   dao.NewPipelineOperationLogDAO(),
		kbDAO:            dao.NewKnowledgebaseDAO(),
		userCanvasDAO:    dao.NewUserCanvasDAO(),
		taskPublisher:    NewMessageQueueTaskPublisher(),
		logSettings:      DefaultIngestionLogSettings(),
		eventEmitter:     &observability.CommonLogEmitter{},
	}
}

func (s *IngestionTaskService) SetTaskPublisher(taskPublisher TaskPublisher) {
	if taskPublisher == nil {
		return
	}
	s.taskPublisher = taskPublisher
}

// SetTierResolver swaps the tenant->tier resolver. A nil argument is ignored so
// callers cannot accidentally drop back to the edition-specific default.
// Tests inject a deterministic resolver to keep routing assertions
// edition-independent.
func (s *IngestionTaskService) SetTierResolver(resolver tierResolver) {
	if resolver == nil {
		return
	}
	s.tierResolver = resolver
}

// SetEventEmitter swaps the lifecycle event sink. A nil argument is ignored so
// callers cannot accidentally disable emission by passing nil.
func (s *IngestionTaskService) SetEventEmitter(emitter observability.IngestionEventEmitter) {
	if emitter == nil {
		return
	}
	s.eventEmitter = emitter
}

// emitLifecycleEvent builds and emits a structured observability event for a
// task lifecycle transition. It is best-effort: any lookup failure or a nil
// emitter is silently ignored so the parse pipeline is never disrupted by
// observability. Enrichment reads the document (file name/type/size/parser) and
// the bound pipeline run (run number, begin/end, progress) and forwards them to
// observability.BuildIngestLog, which owns the field mapping onto common.IngestionLog.
// emitIngestLogTimeout bounds the lifetime of the background instrumentation
// goroutine so a hung DB cannot leak it. It is a single coarse bound for the
// whole emit, not a per-field timeout on the business path.
const emitIngestLogTimeout = 3 * time.Second

// emitLifecycleEvent forwards a task lifecycle transition to the observability
// sink. It is fire-and-forget: the business flow (e.g. task creation) must never
// block on or be affected by instrumentation work, regardless of what happens
// inside it (slow sink, DB error, panic). The caller snapshots the identity
// scalars and returns immediately; enrichment + emit run on a detached,
// panic-recovered goroutine.
func (s *IngestionTaskService) emitLifecycleEvent(ctx context.Context, task *entity.IngestionTask, typ, status, errorInfo string) {
	if s.eventEmitter == nil || task == nil {
		return
	}
	// Snapshot the database handle synchronously, at emit time. The background
	// goroutine must read the store the business event fired against, not
	// whatever global dao.DB a later test (or caller) has since swapped in via
	// pushServiceDB. Reading a stale/missing global handle is what let a prior
	// test's instrumentation goroutine corrupt or race a later test's database.
	// Capturing the value here keeps each goroutine isolated to its own emit-time
	// store; if that store is later closed by teardown the best-effort read
	// simply fails instead of reaching into another test's data.
	db := dao.DB
	snap := ingestEventSnapshot{
		taskID:        task.ID,
		documentID:    task.DocumentID,
		userID:        task.UserID,
		datasetID:     task.DatasetID,
		pipelineLogID: task.PipelineLogID,
		typ:           typ,
		status:        status,
		errorInfo:     errorInfo,
	}
	go s.emitLifecycleEventAsync(ctx, snap, db)
}

// ingestEventSnapshot carries the immutable identity values needed to build an
// ingest log. It is copied at emit time so the background goroutine never shares
// the caller's *entity.IngestionTask (which the business flow may mutate after
// emitLifecycleEvent returns).
type ingestEventSnapshot struct {
	taskID        string
	documentID    string
	userID        string
	datasetID     string
	pipelineLogID *string
	typ           string
	status        string
	errorInfo     string
}

// emitLifecycleEventAsync performs the actual enrichment + emit off the business
// path. It is intentionally isolated: any panic is recovered (instrumentation
// must never crash the caller), and the context is detached from the caller's
// cancellation while preserving upstream-injected identity values.
func (s *IngestionTaskService) emitLifecycleEventAsync(parentCtx context.Context, snap ingestEventSnapshot, db *gorm.DB) {
	defer func() {
		if r := recover(); r != nil {
			common.Warn("ingest lifecycle emit panicked; isolated, business flow unaffected",
				zap.Any("panic", r))
		}
	}()
	// Detach from the caller's cancellation/deadline so the emit completes even
	// after the business request returns, keeping upstream-injected identity
	// (ingestor name, NATS subject, worker). A single coarse timeout bounds the
	// goroutine's lifetime; this is goroutine hygiene, not per-field processing.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parentCtx), emitIngestLogTimeout)
	defer cancel()

	task := &entity.IngestionTask{
		ID:            snap.taskID,
		DocumentID:    snap.documentID,
		UserID:        snap.userID,
		DatasetID:     snap.datasetID,
		PipelineLogID: snap.pipelineLogID,
	}
	var doc *entity.Document
	if s.documentDAO != nil && db != nil {
		doc, _ = s.documentDAO.GetByID(ctx, db, task.DocumentID)
	}
	logID := task.PipelineLogID
	if (logID == nil || *logID == "") && s.ingestionTaskDAO != nil && db != nil {
		if fresh, err := s.ingestionTaskDAO.GetByID(ctx, db, task.ID); err == nil {
			logID = fresh.PipelineLogID
		}
	}
	var run *entity.PipelineOperationLog
	if logID != nil && *logID != "" && s.pipelineLogDAO != nil && db != nil {
		run, _ = s.pipelineLogDAO.GetByID(ctx, db, *logID)
	}
	ctx = s.enrichIngestIdentity(ctx, db, task, doc)
	s.eventEmitter.EmitIngestLog(ctx, observability.BuildIngestLog(ctx, task, doc, run, snap.typ, snap.status, snap.errorInfo))
}

// enrichIngestIdentity attaches the owning user and tenant display names to the
// context from which observability.BuildIngestLog reads them. The ingestor name,
// NATS subject and worker are injected upstream on the ingestor's own context;
// this completes the user/tenant half, which is only knowable from the task's
// owner id and its document's knowledgebase. Resolution is best-effort: a missing
// dao handle, an unresolvable id, or an empty name leaves the field empty so the
// sink omits it rather than disrupting the parse pipeline.
func (s *IngestionTaskService) enrichIngestIdentity(ctx context.Context, db *gorm.DB, task *entity.IngestionTask, doc *entity.Document) context.Context {
	if db == nil {
		return ctx
	}
	if task != nil && task.UserID != "" {
		if nick, err := dao.NewUserDAO().GetNicknameByID(ctx, db, task.UserID); err == nil && nick != "" {
			ctx = observability.WithUserName(ctx, nick)
		}
	}
	if doc != nil && doc.KbID != "" {
		tenantID, err := dao.GetTenantIDByKBID(ctx, db, doc.KbID)
		if err == nil && tenantID != "" {
			if t, err := dao.NewTenantDAO().GetByID(ctx, db, tenantID); err == nil && t != nil && t.Name != nil && *t.Name != "" {
				ctx = observability.WithTenantName(ctx, *t.Name)
			}
		}
	}
	return ctx
}

func (s *IngestionTaskService) List(ctx context.Context, datasetID *string, page, pageSize int) ([]*entity.IngestionTask, error) {
	userID := permission.UserIDFromContext(ctx)
	if userID == "" {
		return nil, permission.ErrUnauthenticated
	}
	if datasetID == nil {
		return s.ingestionTaskDAO.ListByUserID(ctx, dao.DB, userID, page, pageSize)
	}
	return s.ingestionTaskDAO.ListByUserIDAndDatasetID(ctx, dao.DB, userID, *datasetID, page, pageSize)
}

func (s *IngestionTaskService) CreateForDocuments(ctx context.Context, datasetID string, docIDs []string) ([]*ParseDocumentResponse, error) {
	userID := permission.UserIDFromContext(ctx)
	if userID == "" {
		return nil, permission.ErrUnauthenticated
	}
	uniqueDocIDs := common.Deduplicate(docIDs)
	if len(uniqueDocIDs) == 0 {
		return nil, fmt.Errorf("no documents to parse")
	}

	// Populate this cache lazily so a batch avoids repeated knowledge-base reads.
	kbCache := make(map[string]*entity.Knowledgebase)

	responses := make([]*ParseDocumentResponse, 0, len(uniqueDocIDs))
	for _, docID := range uniqueDocIDs {
		// Validate dataset membership before creating or reusing a task.
		_, err := s.documentDAO.GetByDocumentIDAndDatasetID(ctx, dao.DB, docID, datasetID)
		if err != nil {
			responses = append(responses, &ParseDocumentResponse{
				DocumentID: docID,
				Result:     err.Error(),
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
		s.emitLifecycleEvent(ctx, task, "stopped", common.STOPPED, "Task stopped by user.")
		return task, nil
	case common.RUNNING, common.COMPLETED, common.STOPPED, common.FAILED:
		return task, nil
	default:
		return task, fmt.Errorf("task %s has unsupported status %s", taskID, task.Status)
	}
}

// PrepareValidatedRun performs the document and run-log initialization only
// after ReloadAndValidateRunIdentity has accepted the task's captured binding.
// Committed chunk/token counts must survive redelivery so ApplyDocCounts rolls
// only the result's delta into the knowledge base aggregate.
func (s *IngestionTaskService) PrepareValidatedRun(ctx context.Context, task *entity.IngestionTask) {
	if task == nil {
		return
	}
	if err := s.documentDAO.UpdateByID(ctx, dao.DB, task.DocumentID, map[string]interface{}{
		"progress":         float64(0),
		"process_duration": float64(0),
		"process_begin_at": time.Now(),
	}); err != nil {
		common.Warn(fmt.Sprintf("prepare validated run: mark document %s running for task %s: %v", task.DocumentID, task.ID, err))
	}
	s.advanceOpenLog(ctx, task, logFromUnstartOrScheduled, string(entity.TaskStatusRunning))
	s.recordRunMessage(ctx, task, "Task is running...")
	// Emit the running lifecycle event only after the document's worker-claim
	// time has been stamped, so the event's BeginTime (and wait_duration_s)
	// reflect this run rather than a zero or previous-run value. Emitting here
	// (guarded by a successful claim in the caller) keeps one running event per
	// actual run start.
	s.emitLifecycleEvent(ctx, task, "running", common.RUNNING, "")
}

// ErrTaskNotInScope indicates RequestStopScoped was asked to stop a task whose
// current status is not among the caller-supplied allowed statuses. It is not a
// failure of the stop operation itself; callers treat it as a no-op for that task
// and may still count an already-stopped task as an idempotent success.
var ErrTaskNotInScope = errors.New("task status is not within the requested cancel scope")

// RequestStop stops a task unconditionally (as long as its status is one it can
// act on: CREATED/SCHEDULED/RUNNING). It is the per-task cancel path.
func (s *IngestionTaskService) RequestStop(ctx context.Context, taskID string) (*entity.IngestionTask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return s.stopActiveTask(ctx, task)
}

// stopActiveTask performs the state transition for a task already loaded and known
// to be in a stoppable state. It centralizes the CREATED/SCHEDULED -> STOPPED and
// RUNNING -> STOPPING logic so RequestStop (unconditional) and RequestStopScoped
// (scope-checked) share one implementation and cannot drift apart.
func (s *IngestionTaskService) stopActiveTask(ctx context.Context, task *entity.IngestionTask) (*entity.IngestionTask, error) {
	switch task.Status {
	case common.CREATED, common.SCHEDULED:
		stopped, err := s.transitionFrom(ctx, task.ID, []string{common.CREATED, common.SCHEDULED}, common.STOPPED)
		if err != nil {
			return nil, err
		}
		// The stop finalizes without a worker (no RUNNING phase, so no
		// terminal pipeline-log writer will run). Advance the open row to
		// CANCEL here, otherwise the detail page keeps a queued entry.
		s.advanceOpenLog(ctx, stopped, logFromUnstartOrScheduled, string(entity.TaskStatusCancel))
		s.recordRunTerminal(ctx, stopped, "Task stopped by user.")
		s.emitLifecycleEvent(ctx, stopped, "stopped", common.STOPPED, "Task stopped by user.")
		return stopped, nil
	case common.RUNNING:
		task, err := s.transition(ctx, task.ID, common.STOPPING)
		if err != nil {
			return nil, err
		}
		// Mirror Python's cancel_all_task_of: set Redis cancel flag so the
		// running worker's pollCancel detects the stop on its next 500ms
		// tick instead of the slower DB fallback path.
		if rc := kvrocks.Get(); rc != nil {
			rc.Set(ctx, fmt.Sprintf("%s-cancel", task.ID), "x", 1*time.Hour)
		}
		return task, nil
	default:
		return task, nil
	}
}

// RequestStopScoped stops a task only when its current status is among allowed.
// The scope check runs after the task is loaded (GetTask) and before any
// transition, so a status that drifts between the caller's list/lookup query and
// this call cannot cause an out-of-scope stop: the TOCTOU window the caller's
// earlier read opened is closed here, atomically with the CAS that performs the
// transition. A status outside allowed returns the loaded task and
// ErrTaskNotInScope without any state change. An empty allowed set permits no
// status, so every task is out of scope.
func (s *IngestionTaskService) RequestStopScoped(ctx context.Context, taskID string, allowed []string) (*entity.IngestionTask, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if !statusInSet(task.Status, allowed) {
		return task, ErrTaskNotInScope
	}
	return s.stopActiveTask(ctx, task)
}

// statusInSet reports whether status equals one of the members of set.
func statusInSet(status string, set []string) bool {
	for _, s := range set {
		if s == status {
			return true
		}
	}
	return false
}

// SupersedeUnstartedTask closes a queued run before a replacement parse is
// created. A numbered pipeline log is an immutable run ledger, so this must
// record its cancellation rather than deleting the row with the task.
func (s *IngestionTaskService) SupersedeUnstartedTask(ctx context.Context, taskID string) error {
	_, err := s.supersedeUnstartedTask(ctx, taskID, "Task superseded by a new parse request.")
	return err
}

func (s *IngestionTaskService) supersedeUnstartedTask(ctx context.Context, taskID, terminalMessage string) (*entity.IngestionTask, error) {
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
		if err := s.settleTaskRun(ctx, stopped, entity.TaskStatusCancel, terminalMessage); err != nil {
			return nil, err
		}
		s.emitLifecycleEvent(ctx, stopped, "stopped", common.STOPPED, terminalMessage)
		return stopped, nil
	case common.RUNNING, common.STOPPING:
		return nil, fmt.Errorf("task %s is %s and cannot be superseded", taskID, task.Status)
	default:
		return task, nil
	}
}

// PrepareDocumentTaskForCleanup rejects an active task and freezes a queued
// task before destructive document cleanup. The caller deletes the returned
// terminal task after purging its run-scoped cache state.
func (s *IngestionTaskService) PrepareDocumentTaskForCleanup(ctx context.Context, documentID, terminalMessage string) (*entity.IngestionTask, error) {
	task, err := s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, documentID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, nil
	}
	switch task.Status {
	case common.CREATED, common.SCHEDULED:
		return s.supersedeUnstartedTask(ctx, task.ID, terminalMessage)
	case common.RUNNING, common.STOPPING:
		return nil, fmt.Errorf("document %s ingestion task %s is %s; stop it and wait for a terminal state before cleanup", documentID, task.ID, task.Status)
	// Terminal cleanup can proceed when its ledger has already been deleted.
	// Queued tasks above must still settle successfully before becoming removable.
	case common.STOPPED:
		if err := s.settleTaskRun(ctx, task, entity.TaskStatusCancel, terminalMessage); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return task, nil
	case common.COMPLETED:
		if err := s.settleTaskRun(ctx, task, entity.TaskStatusDone, taskTerminalMessageComplete); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return task, nil
	case common.FAILED:
		if err := s.settleTaskRun(ctx, task, entity.TaskStatusFail, taskTerminalMessageFailed); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return task, nil
	default:
		return nil, fmt.Errorf("document %s ingestion task %s has unrecognized status %q", documentID, task.ID, task.Status)
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
	updated, err := s.transition(ctx, taskID, common.COMPLETED)
	if err != nil {
		return err
	}
	s.emitLifecycleEvent(ctx, updated, "completed", common.COMPLETED, "")
	return nil
}

// MarkFailed transitions a task to FAILED and emits a lifecycle event carrying
// the terminal reason. This is the public entry point used by the ingestor,
// which has the failure reason (timeout/panic/run-identity error) available at
// the call site; passing it here is what populates the event's ErrorInfo.
func (s *IngestionTaskService) MarkFailed(ctx context.Context, taskID, reason string) error {
	return s.markFailed(ctx, taskID, reason)
}

// markFailed transitions a task to FAILED and emits a lifecycle event carrying
// the terminal reason. The public MarkFailed forwards the reason supplied by
// the ingestor (timeout/panic/run-identity error); internal paths such as
// settlePublishFailure pass an explicit reason too.
func (s *IngestionTaskService) markFailed(ctx context.Context, taskID, reason string) error {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status == common.FAILED || task.Status == common.COMPLETED || task.Status == common.STOPPED {
		return nil // already terminal, idempotent — mirrors MarkStopped
	}
	updated, err := s.transition(ctx, taskID, common.FAILED)
	if err != nil {
		return err
	}
	s.emitLifecycleEvent(ctx, updated, "failed", common.FAILED, reason)
	return nil
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
	updated, err := s.transition(ctx, taskID, common.STOPPED)
	if err != nil {
		return err
	}
	s.emitLifecycleEvent(ctx, updated, "stopped", common.STOPPED, "")
	return nil
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
	// A terminal task normally closes its own run, but that terminal write is
	// best-effort: when it was lost the ledger stays open with no worker left to
	// settle it. Settlement is idempotent, so finish it before deleting the row
	// — the same rule PrepareDocumentTaskForCleanup applies to document cleanup.
	if runStatus, message, ok := terminalRunSettlement(task.Status); ok && task.PipelineLogID != nil && *task.PipelineLogID != "" {
		run, runErr := s.pipelineLogDAO.GetByID(ctx, dao.DB, *task.PipelineLogID)
		if runErr != nil && !errors.Is(runErr, gorm.ErrRecordNotFound) {
			return nil, runErr
		}
		if run != nil && run.RunCount != nil && *run.RunCount > 0 {
			if err := s.settleTaskRun(ctx, task, runStatus, message); err != nil {
				return nil, err
			}
		}
	}
	info, err := s.ingestionTaskDAO.Delete(ctx, dao.DB, taskID, userID)
	if err != nil {
		return nil, err
	}
	// The row is gone, so purge failures cannot be retried by the caller and
	// TTL remains the backstop. Keep backend I/O off the request path; RemoveMany
	// may otherwise accumulate one slow Kvrocks round-trip per deleted task.
	purgeTaskState := s.purgeTaskState
	if purgeTaskState == nil {
		purgeTaskState = ingestionpipeline.PurgeTaskState
	}
	if !scheduleTaskStatePurge(taskStatePurgeRequest{
		ctx:       ctx,
		taskID:    taskID,
		purgeTask: purgeTaskState,
	}) {
		common.Warn("task-state cleanup queue is full",
			zap.String("task_id", taskID),
			zap.Int("capacity", taskStateCleanupCapacity),
		)
	}
	return info, nil
}

// terminalRunSettlement maps a terminal task to the run status and terminal
// message its ledger should carry.
func terminalRunSettlement(taskStatus string) (entity.TaskStatus, string, bool) {
	switch taskStatus {
	case common.STOPPED:
		return entity.TaskStatusCancel, "Task superseded by a new parse request.", true
	case common.COMPLETED:
		return entity.TaskStatusDone, taskTerminalMessageComplete, true
	case common.FAILED:
		return entity.TaskStatusFail, taskTerminalMessageFailed, true
	default:
		return "", "", false
	}
}

func (s *IngestionTaskService) GetTask(ctx context.Context, taskID string) (*entity.IngestionTask, error) {
	if dao.DB == nil {
		// Every task-status write funnels through here (MarkFailed/Stopped/
		// Completed), and that includes the panic-recovery path in the ingestor
		// worker. Dereferencing a nil handle there turns a recovered task panic
		// into a panicking recovery handler, which kills the worker process.
		return nil, errors.New("ingestion task: nil database")
	}
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

// GetTaskByDocument returns the document's latest ingestion task. A document
// may be parsed multiple times over its lifetime; the newest task (by
// create_time) is the current parse round, so doc-level run state derives from
// it alone.
func (s *IngestionTaskService) GetTaskByDocument(ctx context.Context, documentID string) (*entity.IngestionTask, error) {
	return s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, documentID)
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
		if existing.DatasetID != task.DatasetID {
			return nil, fmt.Errorf("document id %s has an ingestion task in a different dataset", task.DocumentID)
		}
		switch existing.Status {
		case common.CREATED:
			if err = s.ensureRunIdentity(ctx, existing, kbCache); err != nil {
				return nil, fmt.Errorf("ensure run identity for task %s: %w", existing.ID, err)
			}
			s.emitLifecycleEvent(ctx, existing, "created", common.CREATED, "")
			if err = s.enqueueTask(ctx, existing, kbCache); err != nil {
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
			s.emitLifecycleEvent(ctx, existing, "created", common.CREATED, "")
			if err = s.enqueueTask(ctx, existing, kbCache); err != nil {
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
	s.emitLifecycleEvent(ctx, created, "created", common.CREATED, "")
	if err = s.enqueueTask(ctx, created, kbCache); err != nil {
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
		s.emitLifecycleEvent(ctx, task, "scheduled", common.SCHEDULED, "")
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
	// Shared across the loop so repeated tasks for the same dataset resolve the
	// KB once rather than once per task.
	kbCache := make(map[string]*entity.Knowledgebase)
	for _, task := range tasks {
		if err := s.ensureRunIdentity(ctx, task, nil); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("ensure run identity for created task %s: %w", task.ID, err))
			continue
		}
		if err := s.enqueueTask(ctx, task, kbCache); err != nil {
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
	if err := s.markFailed(ctx, task.ID, "Task publish failed."); err != nil {
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
	if rc := kvrocks.Get(); rc != nil {
		rc.Delete(ctx, fmt.Sprintf("%s-cancel", taskID))
	}
}

// resolveTier determines the priority lane for a task by resolving the tenant
// that owns its dataset. A resolver error (knowledgebase lookup failure,
// unknown tier) propagates so the caller settles the task as FAILED instead of
// silently dropping it onto a default lane.
//
// kbCache is an optional per-batch map of datasetID -> knowledgebase. Callers
// that enqueue many tasks for the same dataset (CreateForDocuments, the
// ScheduleCreatedTasks recovery loop) supply one so the KB lookup is done once
// per dataset, not once per task. A nil cache always queries the DAO; a cache
// miss falls back to the DAO and fills the (non-nil) cache on success.
func (s *IngestionTaskService) resolveTier(ctx context.Context, datasetID string, kbCache map[string]*entity.Knowledgebase) (common.Tier, error) {
	kb := kbCache[datasetID]
	if kb == nil {
		var err error
		kb, err = s.kbDAO.GetByID(ctx, dao.DB, datasetID)
		if err != nil {
			return "", fmt.Errorf("resolve tier: load knowledgebase %s: %w", datasetID, err)
		}
		if kb == nil {
			return "", fmt.Errorf("resolve tier: knowledgebase %s not found", datasetID)
		}
		if kbCache != nil {
			kbCache[datasetID] = kb
		}
	}
	resolver := s.tierResolver
	if resolver == nil {
		resolver = defaultTierResolver
	}
	return resolver(ctx, kb.TenantID)
}

func (s *IngestionTaskService) enqueueTask(ctx context.Context, task *entity.IngestionTask, kbCache map[string]*entity.Knowledgebase) error {
	tier, err := s.resolveTier(ctx, task.DatasetID, kbCache)
	if err != nil {
		return err
	}
	subject, err := common.SubjectForTier(tier)
	if err != nil {
		return err
	}
	taskMessage := common.TaskMessage{
		TaskID:   task.ID,
		TaskType: common.TaskTypeIngestionTask,
	}
	return s.taskPublisher.PublishTaskMessage(subject, taskMessage)
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

// advanceOpenLog performs a best-effort ledger advance for non-cleanup flows.
func (s *IngestionTaskService) advanceOpenLog(ctx context.Context, task *entity.IngestionTask, fromStatuses []string, operationStatus string) {
	_, _ = s.advanceOpenLogIfOpen(ctx, task, fromStatuses, operationStatus)
}

// advanceOpenLogIfOpen moves a task's already-bound row to operationStatus and
// reports whether this call changed it. It never creates or adopts a row, so a
// worker cannot repair a missing identity or affect a newer run.
func (s *IngestionTaskService) advanceOpenLogIfOpen(ctx context.Context, task *entity.IngestionTask, fromStatuses []string, operationStatus string) (bool, error) {
	if task == nil || task.DocumentID == "" || s.pipelineLogDAO == nil {
		return false, nil
	}
	logID := ""
	if task.PipelineLogID != nil {
		logID = *task.PipelineLogID
	}
	if logID == "" {
		common.Warn(fmt.Sprintf("advance open pipeline log for task %s: missing run identity", task.ID))
		return false, nil
	}
	advanced, err := s.pipelineLogDAO.AdvanceOpenLog(ctx, dao.DB, logID, fromStatuses, operationStatus)
	if err != nil {
		common.Warn(fmt.Sprintf("advance open pipeline log for document %s to %s: %v", task.DocumentID, operationStatus, err))
	}
	return advanced, err
}

// settleTaskRun closes a terminal task's run ledger and ensures its terminal
// event exists. The event write is retried even when a previous attempt already
// advanced the operation row, so a partial settlement never becomes permanent.
func (s *IngestionTaskService) settleTaskRun(ctx context.Context, task *entity.IngestionTask, operationStatus entity.TaskStatus, terminalMessage string) error {
	if task == nil || task.PipelineLogID == nil || *task.PipelineLogID == "" {
		return nil
	}
	if _, err := s.advanceOpenLogIfOpen(ctx, task, dao.OpenPipelineOperationStatuses(), string(operationStatus)); err != nil {
		return err
	}
	if err := s.RecordTerminal(ctx, *task.PipelineLogID, task.ID, terminalMessage); err != nil {
		return fmt.Errorf("record terminal event for task %s run %s: %w", task.ID, *task.PipelineLogID, err)
	}
	return nil
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
			if doc.PipelineVersionID != nil {
				input.PipelineVersionID = strings.TrimSpace(*doc.PipelineVersionID)
			}
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
	input.PipelineTitle = utility.TruncateRunes(input.PipelineTitle, common.PipelineTitleLimit)
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
	if err := s.ensureTerminalEvent(ctx, pipelineLogID, taskID, message); err != nil {
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

// ensureTerminalEvent serializes terminal writers on the run row and inserts
// the terminal event only when one is not already present. This makes retrying
// settlement safe after the operation status has already advanced.
func (s *IngestionTaskService) ensureTerminalEvent(ctx context.Context, pipelineLogID, taskID, message string) error {
	if dao.DB == nil {
		return errors.New("ingestion task: nil database")
	}
	return dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run entity.PipelineOperationLog
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Take(&run, "id = ?", pipelineLogID).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&entity.IngestionTaskLog{}).
			Where("pipeline_log_id = ? AND event_type = ?", pipelineLogID, dao.EventTypeTerminal).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		return s.insertEventOnDB(ctx, tx, ingestionEventTerminal, pipelineLogID, taskID, "", 0, message)
	})
}

func rejectIngestionEvent(reason, pipelineLogID, taskID string, err error) error {
	common.Warn("ingestion_log_event_rejected",
		zap.String("event", "ingestion_log_event_rejected"),
		zap.String("reason", reason),
		zap.String("pipeline_log_id", pipelineLogID),
		zap.String("task_id", taskID),
	)
	return err
}

func (s *IngestionTaskService) insertEvent(ctx context.Context, kind ingestionEventKind, pipelineLogID, taskID, component string, phase int, message string) error {
	return s.insertEventOnDB(ctx, dao.DB, kind, pipelineLogID, taskID, component, phase, message)
}

func (s *IngestionTaskService) insertEventOnDB(ctx context.Context, db *gorm.DB, kind ingestionEventKind, pipelineLogID, taskID, component string, phase int, message string) error {
	if pipelineLogID == "" {
		return rejectIngestionEvent("missing_pipeline_log_id", pipelineLogID, taskID, errors.New("ingestion event requires a pipeline log id"))
	}
	if taskID == "" {
		return rejectIngestionEvent("missing_task_id", pipelineLogID, taskID, errors.New("ingestion event requires a task id"))
	}
	if db == nil {
		return errors.New("ingestion task: nil database")
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
	return db.WithContext(ctx).Model(&entity.IngestionTaskLog{}).Create(map[string]interface{}{
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
