package ingestion

import (
	"context"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"ragflow/internal/common"
)

type Executor struct {
	id     string
	client common.IngestionManagerClient
	stream common.IngestionManager_ActionClient
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Configuration
	maxConcurrency    int32
	supportedDocTypes []string
	version           string

	// Runtime state
	currentTasks map[string]*TaskContext
	tasksMu      sync.RWMutex
}

type TaskContext struct {
	Task       *common.TaskAssignment
	Status     string // running, completed, failed
	StartTime  time.Time
	Progress   int32
	CancelFunc context.CancelFunc
}

func NewExecutor(id string, maxConcurrency int32, supportedTypes []string) *Executor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Executor{
		id:                id,
		ctx:               ctx,
		cancel:            cancel,
		maxConcurrency:    maxConcurrency,
		supportedDocTypes: supportedTypes,
		version:           "1.0.0",
		currentTasks:      make(map[string]*TaskContext),
	}
}

// Connect connects to the admin and establishes a bidirectional stream
func (e *Executor) Connect(serverAddr string) error {
	conn, err := grpc.Dial(serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		return err
	}

	e.client = common.NewIngestionManagerClient(conn)

	stream, err := e.client.Action(e.ctx)
	if err != nil {
		return err
	}
	e.stream = stream

	log.Printf("Executor %s connected to admin", e.id)

	// 1. Send registration message
	if err := e.sendRegister(); err != nil {
		return err
	}

	// 2. Start receive loop
	go e.receiveLoop()

	// 3. Start heartbeat loop
	go e.heartbeatLoop()

	// 4. Start pull loop
	go e.pullLoop()

	return nil
}

func (e *Executor) sendRegister() error {
	msg := &common.IngestionMessage{
		IngestionServerId: e.id,
		MessageType:       "REGISTER",
		RegisterInfo: &common.RegisterInfo{
			MaxConcurrency:    e.maxConcurrency,
			SupportedDocTypes: e.supportedDocTypes,
			Version:           e.version,
		},
	}
	return e.stream.Send(msg)
}

func (e *Executor) sendHeartbeat() error {
	e.tasksMu.RLock()
	taskIDs := make([]string, 0, len(e.currentTasks))
	for tid := range e.currentTasks {
		taskIDs = append(taskIDs, tid)
	}
	e.tasksMu.RUnlock()

	msg := &common.IngestionMessage{
		IngestionServerId: e.id,
		MessageType:       "HEARTBEAT",
		HeartbeatInfo: &common.HeartbeatInfo{
			CurrentTaskIds: taskIDs,
			CurrentLoad:    int32(len(taskIDs)),
			Timestamp:      time.Now().Unix(),
		},
	}
	return e.stream.Send(msg)
}

func (e *Executor) sendPullRequest() error {
	msg := &common.IngestionMessage{
		IngestionServerId: e.id,
		MessageType:       "PULL_REQUEST",
	}
	return e.stream.Send(msg)
}

func (e *Executor) sendTaskResult(taskID, status, resultURL, errorMsg string) error {
	msg := &common.IngestionMessage{
		IngestionServerId: e.id,
		MessageType:       "TASK_RESULT",
		TaskResult: &common.TaskResult{
			TaskId:       taskID,
			Status:       status,
			ResultUrl:    resultURL,
			ErrorMessage: errorMsg,
		},
	}
	return e.stream.Send(msg)
}

func (e *Executor) sendTaskProgress(taskID string, progress int32, info string) error {
	msg := &common.IngestionMessage{
		IngestionServerId: e.id,
		MessageType:       "TASK_PROGRESS",
		TaskProgress: &common.TaskProgress{
			TaskId:   taskID,
			Progress: progress,
			Info:     info,
		},
	}
	return e.stream.Send(msg)
}

func (e *Executor) receiveLoop() {
	for {
		msg, err := e.stream.Recv()
		if err != nil {
			log.Printf("Receive error: %v", err)
			return
		}

		switch msg.MessageType {
		case "TASK_ASSIGNMENT":
			e.handleTaskAssignment(msg.TaskAssignment)

		case "ACK":
			log.Printf("Received ACK: task=%s, success=%v, msg=%s",
				msg.AckInfo.TaskId, msg.AckInfo.Success, msg.AckInfo.Message)

		case "ERROR":
			log.Printf("Received error from admin: %s", msg.ErrorMessage)

		default:
			log.Printf("Unknown admin message type: %s", msg.MessageType)
		}
	}
}

func (e *Executor) handleTaskAssignment(task *common.TaskAssignment) {
	if task == nil {
		// No task available, keep waiting
		log.Printf("No task available")
		return
	}

	log.Printf("Received task: %s, task_type=%s", task.TaskId, task.TaskType)

	// Check if there is an available slot
	e.tasksMu.RLock()
	runningCount := len(e.currentTasks)
	e.tasksMu.RUnlock()

	if runningCount >= int(e.maxConcurrency) {
		log.Printf("No available slot for task %s, rejecting", task.TaskId)
		// Could send a rejection message for admin to re-assign
		return
	}

	// Create task context
	taskCtx, cancel := context.WithCancel(e.ctx)
	taskContext := &TaskContext{
		Task:       task,
		Status:     "running",
		StartTime:  time.Now(),
		CancelFunc: cancel,
	}

	e.tasksMu.Lock()
	e.currentTasks[task.TaskId] = taskContext
	e.tasksMu.Unlock()

	// Start task execution
	e.wg.Add(1)
	go e.executeTask(taskCtx, taskContext)
}

func (e *Executor) executeTask(ctx context.Context, taskCtx *TaskContext) {
	defer e.wg.Done()
	defer func() {
		e.tasksMu.Lock()
		delete(e.currentTasks, taskCtx.Task.TaskId)
		e.tasksMu.Unlock()
	}()

	task := taskCtx.Task
	log.Printf("Starting task %s", task.TaskId)

	// Simulate task execution progress
	// In production, this would split into subtasks and execute in parallel
	for progress := int32(0); progress <= 100; progress += 10 {
		select {
		case <-ctx.Done():
			// Task cancelled
			log.Printf("Task %s cancelled", task.TaskId)
			e.sendTaskResult(task.TaskId, "CANCELLED", "", "task cancelled")
			return
		case <-time.After(500 * time.Millisecond):
			// Simulate progress update
			taskCtx.Progress = progress
			e.sendTaskProgress(task.TaskId, progress, "processing...")
			log.Printf("Task %s progress: %d%%", task.TaskId, progress)
		}
	}

	// Simulate subtask splitting and execution (demonstration)
	e.executeWithSubTasks(task)

	// Task completed
	resultURL := "http://storage.example.com/results/" + task.TaskId + ".json"
	e.sendTaskResult(task.TaskId, "COMPLETED", resultURL, "")
	log.Printf("Task %s completed", task.TaskId)
}

// executeWithSubTasks demonstrates subtask splitting and parallel execution
func (e *Executor) executeWithSubTasks(task *common.TaskAssignment) {
	log.Printf("Task %s: splitting into subtasks", task.TaskId)

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

			log.Printf("Subtask %s started", subID)
			// Simulate subtask execution
			time.Sleep(1 * time.Second)
			log.Printf("Subtask %s completed", subID)
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
		log.Printf("Task %s: %d subtasks failed", task.TaskId, failedCount)
	} else {
		log.Printf("Task %s: all subtasks completed successfully", task.TaskId)
	}
}

func (e *Executor) heartbeatLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			if err := e.sendHeartbeat(); err != nil {
				log.Printf("Failed to send heartbeat: %v", err)
				return
			}
		}
	}
}

func (e *Executor) pullLoop() {
	for {
		select {
		case <-e.ctx.Done():
			return
		default:
		}

		// Check if there is an available slot
		e.tasksMu.RLock()
		runningCount := len(e.currentTasks)
		e.tasksMu.RUnlock()

		if runningCount >= int(e.maxConcurrency) {
			// No available slot, wait and retry
			time.Sleep(1 * time.Second)
			continue
		}

		// Send pull request
		if err := e.sendPullRequest(); err != nil {
			log.Printf("Failed to send pull request: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		// Wait before sending the next pull request
		// In production, admin returns tasks in the response, no need to wait
		time.Sleep(100 * time.Millisecond)
	}
}

// Stop gracefully shuts down the executor
func (e *Executor) Stop() {
	log.Printf("Stopping executor %s", e.id)
	e.cancel()

	// Wait for all tasks to complete
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("All tasks completed")
	case <-time.After(30 * time.Second):
		log.Printf("Timeout waiting for tasks to complete")
	}

	if e.stream != nil {
		e.stream.CloseSend()
	}
}
