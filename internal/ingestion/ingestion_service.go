package ingestion

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"ragflow/internal/common"
)

type taskWrapper struct {
	task *common.TaskAssignment
}

type Ingestor struct {
	id          string
	name        string
	serverAddr  string
	conn        *grpc.ClientConn
	client      common.IngestionManagerClient
	stream      common.IngestionManager_ActionClient
	ctx         context.Context
	cancel      context.CancelFunc
	reconnectMu sync.Mutex

	// Configuration
	maxConcurrency    int32
	supportedDocTypes []string
	version           string

	// Runtime state
	currentTasks map[string]*TaskContext
	tasksMu      sync.RWMutex

	// Shutdown channel - receive on this to trigger graceful shutdown
	ShutdownCh chan struct{}

	// Worker pool
	taskChan  chan *TaskContext
	workerWg  sync.WaitGroup
	startOnce sync.Once
}

type TaskContext struct {
	Ctx                    context.Context
	CancelFunc             context.CancelFunc
	Task                   *common.TaskAssignment
	Status                 string // PENDING, RUNNING, COMPLETED, FAILED, CANCELLING, CANCELLED
	StartTime              time.Time
	EndTime                time.Time
	estimatedRemainingTime time.Duration // estimated cost in seconds to complete the task
	Progress               int32
	ErrorMessage           string
}

func NewIngestor(name string, maxConcurrency int32, supportedTypes []string) *Ingestor {
	ctx, cancel := context.WithCancel(context.Background())
	id := common.GenerateUUID()
	return &Ingestor{
		id:                id,
		name:              name,
		ctx:               ctx,
		cancel:            cancel,
		maxConcurrency:    maxConcurrency,
		supportedDocTypes: supportedTypes,
		version:           "1.0.0",
		currentTasks:      make(map[string]*TaskContext),
		taskChan:          make(chan *TaskContext, maxConcurrency*2),
		ShutdownCh:        make(chan struct{}, 1),
	}
}

// Connect connects to the admin and establishes a bidirectional stream
func (e *Ingestor) Connect(serverAddr string) error {
	e.serverAddr = serverAddr
	conn, err := grpc.Dial(serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("fail to connect admin server: %s", err.Error())
	}
	e.conn = conn

	e.client = common.NewIngestionManagerClient(conn)

	stream, err := e.client.Action(e.ctx)
	if err != nil {
		conn.Close()
		return err
	}
	e.stream = stream

	common.Info(fmt.Sprintf("Ingestor %s connected to admin", e.id))

	// 1. Send registration message
	if err = e.sendRegister(); err != nil {
		conn.Close()
		return err
	}

	// Ensure worker pool is started on first task
	e.startWorkerPool()

	// 2. Start receive loop
	go e.receiveLoop()

	// 3. Start heartbeat loop
	go e.heartbeatLoop()

	return nil
}

func (e *Ingestor) sendRegister() error {
	msg := &common.IngestionMessage{
		IngestorId:  e.id,
		MessageType: "REGISTER",
		RegisterInfo: &common.RegisterInfo{
			MaxConcurrency:    e.maxConcurrency,
			SupportedDocTypes: e.supportedDocTypes,
			Version:           e.version,
			Name:              e.name,
		},
	}
	return e.stream.Send(msg)
}

func (e *Ingestor) sendHeartbeat() error {
	e.tasksMu.RLock()

	cutoff := time.Now().Add(-10 * time.Minute)
	var toDeleteTask []string
	taskStates := make([]*common.TaskState, 0, len(e.currentTasks))

	for tid, tc := range e.currentTasks {
		// Check if task is in a terminal state and expired beyond 10 minutes
		if (tc.Status == "CANCELED" || tc.Status == "COMPLETED" || tc.Status == "REJECTED") &&
			!tc.EndTime.IsZero() && tc.EndTime.Before(cutoff) {
			toDeleteTask = append(toDeleteTask, tid)
		} else {
			taskStates = append(taskStates, &common.TaskState{
				TaskId:                        tid,
				Status:                        tc.Status,
				EstimatedRemainingTimeSeconds: int64(tc.estimatedRemainingTime),
				ErrorMessage:                  tc.ErrorMessage,
				StartTime:                     tc.StartTime.UnixNano(),
				ComeFrom:                      tc.Task.ComeFrom,
			})
		}
	}
	e.tasksMu.RUnlock()

	// Delete expired terminal tasks from currentTasks
	if len(toDeleteTask) > 0 {
		e.tasksMu.Lock()
		for _, id := range toDeleteTask {
			delete(e.currentTasks, id)
		}
		e.tasksMu.Unlock()
	}

	var pid = int64(os.Getpid())
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		log.Fatal(err)
	}

	var cpuPercent float64
	cpuPercent, err = p.Percent(100 * time.Millisecond)
	if err != nil {
		cpuPercent = math.NaN()
		common.Info(fmt.Sprintf("Fail to read CPU usage: %v", err))
	}

	RssUsage := math.NaN()
	VmsUsage := math.NaN()
	memInfo, err := p.MemoryInfo()
	if err == nil {
		RssUsage = float64(memInfo.RSS)
		VmsUsage = float64(memInfo.VMS)
	} else {
		common.Info(fmt.Sprintf("Fail to read memory usage: %v", err))
	}
	msg := &common.IngestionMessage{
		IngestorId:  e.id,
		MessageType: "HEARTBEAT",
		HeartbeatInfo: &common.HeartbeatInfo{
			TaskStates:    taskStates,
			DeleteTaskIds: toDeleteTask,
			CpuUsage:      float32(cpuPercent),
			VmsUsage:      float32(VmsUsage),
			RssUsage:      float32(RssUsage),
			ProcessId:     pid,
		},
	}
	return e.stream.Send(msg)
}

func (e *Ingestor) sendTaskResult(taskID, status, errorMsg string) error {
	msg := &common.IngestionMessage{
		IngestorId:  e.id,
		MessageType: "TASK_RESULT",
		TaskResult: &common.TaskResult{
			TaskId:       taskID,
			Status:       status,
			ErrorMessage: errorMsg,
		},
	}
	return e.stream.Send(msg)
}

func (e *Ingestor) sendTaskProgress(taskID string, progress int32, info string) error {
	msg := &common.IngestionMessage{
		IngestorId:  e.id,
		MessageType: "TASK_PROGRESS",
		TaskProgress: &common.TaskProgress{
			TaskId:   taskID,
			Progress: progress,
			Info:     info,
		},
	}
	return e.stream.Send(msg)
}

func (e *Ingestor) receiveLoop() {
	for {
		msg, err := e.stream.Recv()
		if err != nil {
			if e.ctx.Err() != nil {
				common.Info(fmt.Sprintf("Ingestor %s context cancelled, receive loop exiting", e.id))
				return
			}
			common.Info(fmt.Sprintf("Receive error: %v", err))
			common.Info("Admin connection lost, attempting to reconnect")
			e.reconnect()
			return
		}

		switch msg.MessageType {
		case "TASK_ASSIGNMENT":
			e.handleTaskAssignment(msg.TaskAssignment)

		case "ACK":
			common.Info(fmt.Sprintf("Received ACK: task=%s, success=%v, msg=%s",
				msg.AckInfo.TaskId, msg.AckInfo.Success, msg.AckInfo.Message))

		case "ERROR":
			common.Info(fmt.Sprintf("Received error from admin: %s", msg.ErrorMessage))

		default:
			common.Info(fmt.Sprintf("Unknown admin message type: %s", msg.MessageType))
		}
	}
}

func (e *Ingestor) handleTaskAssignment(task *common.TaskAssignment) {
	if task == nil {
		return
	}

	common.Info(fmt.Sprintf("Received task: %s, task_type=%s", task.TaskId, task.TaskType))

	switch task.TaskType {
	case "shutdown_ingestor":
		if e.id == task.AssignedTo {
			e.handleShutdownIngestor()
			return
		}

		common.Error("unmatched ingestor id", fmt.Errorf("attempt to shutdown ingestor: %s, current ingestor: %s, mismatched", task.AssignedTo, e.id))
		return
	case "cancel_ingestion_task":
		e.handleCancelTask(task.TaskId)
		return
	}

	// Construct TaskContext with a cancellable context
	ctx, cancel := context.WithCancel(e.ctx)
	taskCtx := &TaskContext{
		Ctx:        ctx,
		CancelFunc: cancel,
		Task:       task,
		Status:     "QUEUED",
	}

	// Register in currentTasks immediately so heartbeat sees PENDING state
	e.tasksMu.Lock()
	e.currentTasks[task.TaskId] = taskCtx
	e.tasksMu.Unlock()

	common.Info("wait for 10 seconds")
	time.Sleep(time.Second * 10)
	// Push to task channel; if full, reject the task (backpressure)
	select {
	case e.taskChan <- taskCtx:
		common.Info(fmt.Sprintf("Task %s queued (channel: %d/%d)", task.TaskId, len(e.taskChan), cap(e.taskChan)))
	default:
		common.Info(fmt.Sprintf("No available slot for task %s, rejecting", task.TaskId))
		//e.tasksMu.Lock()
		//delete(e.currentTasks, task.TaskId)
		//e.tasksMu.Unlock()
		taskCtx.Status = "REJECTED"
		taskCtx.EndTime = time.Now()
		e.sendTaskResult(taskCtx.Task.TaskId, "REJECTED", "task rejected before execution")
	}
}

func (e *Ingestor) handleCancelTask(taskID string) {
	e.tasksMu.Lock()
	taskCtx, exists := e.currentTasks[taskID]
	e.tasksMu.Unlock()

	if !exists {
		common.Info(fmt.Sprintf("Cancel request for unknown task %s, ignoring", taskID))
		return
	}

	common.Info(fmt.Sprintf("Cancelling task %s (current status: %s)", taskID, taskCtx.Status))
	taskCtx.CancelFunc()
}

func (e *Ingestor) handleShutdownIngestor() {
	common.Info(fmt.Sprintf("Shutdown task received, initiating graceful shutdown of ingestor %s", e.id))
	select {
	case e.ShutdownCh <- struct{}{}:
	default:
	}
	return
}

func (e *Ingestor) startWorkerPool() {
	e.startOnce.Do(func() {
		for i := int32(0); i < e.maxConcurrency; i++ {
			e.workerWg.Add(1)
			go e.workerLoop(i)
		}
		common.Info(fmt.Sprintf("Worker pool started with %d workers", e.maxConcurrency))
	})
}

func (e *Ingestor) workerLoop(id int32) {
	defer e.workerWg.Done()
	common.Info(fmt.Sprintf("Worker %d started", id))
	for {
		select {
		case <-e.ctx.Done():
			return
		case taskCtx := <-e.taskChan:
			// Skip tasks that were canceled while queued
			select {
			case <-taskCtx.Ctx.Done():
				common.Info(fmt.Sprintf("Task %s was cancelled while queued, skipping", taskCtx.Task.TaskId))
				taskCtx.Status = "CANCELED"
				taskCtx.EndTime = time.Now()
				//e.tasksMu.Lock()
				//delete(e.currentTasks, taskCtx.Task.TaskId)
				//e.tasksMu.Unlock()
				e.sendTaskResult(taskCtx.Task.TaskId, "CANCELED", "task cancelled before execution")
				continue
			default:
			}

			// Mark as RUNNING
			taskCtx.Status = "RUNNING"
			taskCtx.StartTime = time.Now()

			// Execute the task (synchronously within worker)
			e.executeTask(taskCtx)
		}
	}
}

func (e *Ingestor) executeTask(taskCtx *TaskContext) {
	defer func() {
		//e.tasksMu.Lock()
		//delete(e.currentTasks, taskCtx.Task.TaskId)
		//e.tasksMu.Unlock()
	}()

	ctx := taskCtx.Ctx
	task := taskCtx.Task
	common.Info(fmt.Sprintf("Starting task %s", task.TaskId))

	// Simulate task execution progress
	// In production, this would split into subtasks and execute in parallel
	for progress := int32(0); progress <= 100; progress += 10 {
		select {
		case <-ctx.Done():
			// Task canceled
			common.Info(fmt.Sprintf("Task %s cancelled", task.TaskId))
			taskCtx.Status = "CANCELED"
			taskCtx.EndTime = time.Now()
			e.sendTaskResult(task.TaskId, "CANCELED", "task cancelled")
			return
		case <-time.After(5000 * time.Millisecond):
			// Simulate progress update
			taskCtx.Progress = progress
			e.sendTaskProgress(task.TaskId, progress, "processing...")
			common.Info(fmt.Sprintf("Task %s progress: %d%%", task.TaskId, progress))
		}
	}

	// Simulate subtask splitting and execution (demonstration)
	e.executeWithSubTasks(task)

	taskCtx.Status = "COMPLETED"
	taskCtx.EndTime = time.Now()

	time.Sleep(time.Second * 10)

	// Task completed
	e.sendTaskResult(task.TaskId, "COMPLETED", "")
	common.Info(fmt.Sprintf("Task %s completed", task.TaskId))
}

// executeWithSubTasks demonstrates subtask splitting and parallel execution
func (e *Ingestor) executeWithSubTasks(task *common.TaskAssignment) {
	common.Info(fmt.Sprintf("Task %s: splitting into subtasks", task.TaskId))

	// Simulate splitting into 4 subtasks
	subTasks := []struct {
		id    string
		index int
	}{
		{task.TaskId + "-sub1", 1},
		{task.TaskId + "-sub2", 2},
		{task.TaskId + "-sub3", 3},
		{task.TaskId + "-sub4", 4},
	}

	// Wait for all subtasks to complete
	var wg sync.WaitGroup
	results := make(chan error, len(subTasks))

	// Execute subtasks in parallel
	for _, st := range subTasks {
		wg.Add(1)
		go func(subID string, idx int) {
			defer wg.Done()

			common.Info(fmt.Sprintf("Subtask %s started", subID))
			// Simulate subtask execution
			time.Sleep(1 * time.Second)
			common.Info(fmt.Sprintf("Subtask %s completed", subID))
			results <- nil
		}(st.id, st.index)
	}

	// Wait for all subtasks to complete
	wg.Wait()
	close(results)

	// Check if any subtasks failed
	failedCount := 0
	for err := range results {
		if err != nil {
			failedCount++
		}
	}

	if failedCount > 0 {
		common.Info(fmt.Sprintf("Task %s: %d subtasks failed", task.TaskId, failedCount))
	} else {
		common.Info(fmt.Sprintf("Task %s: all subtasks completed successfully", task.TaskId))
	}
}

func (e *Ingestor) heartbeatLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			if err := e.sendHeartbeat(); err != nil {
				common.Info(fmt.Sprintf("Failed to send heartbeat: %v", err))
				if e.ctx.Err() != nil {
					common.Info(fmt.Sprintf("Ingestor %s context cancelled, heartbeat loop exiting", e.id))
					return
				}
				common.Info(fmt.Sprintf("Admin connection lost, attempting to reconnect"))
				e.reconnect()
				return
			}
		}
	}
}

// reconnect closes the old connection and establishes a new one with exponential backoff.
// Only one reconnection attempt runs at a time; concurrent callers return immediately.
func (e *Ingestor) reconnect() {
	if e.ctx.Err() != nil {
		common.Info(fmt.Sprintf("Ingestor %s is shutting down, skipping reconnection", e.id))
		return
	}

	if !e.reconnectMu.TryLock() {
		return
	}
	defer e.reconnectMu.Unlock()

	common.Info(fmt.Sprintf("Ingestor %s attempting to reconnect to admin at %s", e.id, e.serverAddr))

	// Close old stream and connection
	if e.stream != nil {
		e.stream.CloseSend()
	}
	if e.conn != nil {
		e.conn.Close()
	}

	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		conn, err := grpc.Dial(e.serverAddr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithTimeout(5*time.Second),
		)
		if err != nil {
			common.Info(fmt.Sprintf("Reconnect dial failed: %v, retrying in %v", err, backoff))
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		e.conn = conn
		e.client = common.NewIngestionManagerClient(conn)

		stream, err := e.client.Action(e.ctx)
		if err != nil {
			conn.Close()
			common.Info(fmt.Sprintf("Reconnect create stream failed: %v, retrying in %v", err, backoff))
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		e.stream = stream

		if err = e.sendRegister(); err != nil {
			stream.CloseSend()
			conn.Close()
			common.Info(fmt.Sprintf("Reconnect register failed: %v, retrying in %v", err, backoff))
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		common.Info(fmt.Sprintf("Ingestor %s reconnected to admin", e.id))
		break
	}

	// Restart the loops on the new stream
	go e.receiveLoop()
	go e.heartbeatLoop()
}

// Stop gracefully shuts down the ingestor
func (e *Ingestor) Stop() {
	common.Info(fmt.Sprintf("Stopping ingestor %s", e.id))
	e.cancel()

	// Wait for all workers to finish (they exit on ctx.Done())
	e.workerWg.Wait()
	common.Info("All tasks completed")

	if e.stream != nil {
		e.stream.CloseSend()
	}
}
