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

type Ingestor struct {
	id     string
	name   string
	serverAddr string
	conn   *grpc.ClientConn
	client common.IngestionManagerClient
	stream common.IngestionManager_ActionClient
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	reconnectMu sync.Mutex

	// Configuration
	maxConcurrency    int32
	supportedDocTypes []string
	version           string

	// Runtime state
	currentTasks map[string]*TaskContext
	tasksMu      sync.RWMutex
}

type TaskContext struct {
	Task                   *common.TaskAssignment
	Status                 string // PENDING, RUNNING, COMPLETED, FAILED, CANCELLING, CANCELLED
	StartTime              time.Time
	estimatedRemainingTime time.Duration // estimated cost in seconds to complete the task
	Progress               int32
	ErrorMessage           string
	CancelFunc             context.CancelFunc
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
		return err
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
		return err
	}

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
	taskIDs := make([]string, 0, len(e.currentTasks))
	for tid := range e.currentTasks {
		taskIDs = append(taskIDs, tid)
	}
	e.tasksMu.RUnlock()

	taskStates := make([]*common.TaskState, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		taskCtx := e.currentTasks[taskID]
		taskStates = append(taskStates, &common.TaskState{
			TaskId:                        taskID,
			Status:                        taskCtx.Status,
			EstimatedRemainingTimeSeconds: int64(taskCtx.estimatedRemainingTime),
			ErrorMessage:                  taskCtx.ErrorMessage,
		})
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
			TaskStates: taskStates,
			CpuUsage:   float32(cpuPercent),
			VmsUsage:   float32(VmsUsage),
			RssUsage:   float32(RssUsage),
			ProcessId:  pid,
		},
	}
	return e.stream.Send(msg)
}

func (e *Ingestor) sendTaskResult(taskID, status, resultURL, errorMsg string) error {
	msg := &common.IngestionMessage{
		IngestorId:  e.id,
		MessageType: "TASK_RESULT",
		TaskResult: &common.TaskResult{
			TaskId:       taskID,
			Status:       status,
			ResultUrl:    resultURL,
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
			log.Printf("Receive error: %v, attempting to reconnect", err)
			e.reconnect()
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

func (e *Ingestor) handleTaskAssignment(task *common.TaskAssignment) {
	if task == nil {
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

func (e *Ingestor) executeTask(ctx context.Context, taskCtx *TaskContext) {
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
func (e *Ingestor) executeWithSubTasks(task *common.TaskAssignment) {
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

func (e *Ingestor) heartbeatLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			if err := e.sendHeartbeat(); err != nil {
				log.Printf("Failed to send heartbeat: %v, attempting to reconnect", err)
				e.reconnect()
				return
			}
		}
	}
}

// reconnect closes the old connection and establishes a new one with exponential backoff.
// Only one reconnection attempt runs at a time; concurrent callers return immediately.
func (e *Ingestor) reconnect() {
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

// Stop gracefully shuts down the executor
func (e *Ingestor) Stop() {
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
