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

	// 配置
	maxConcurrency    int32
	supportedDocTypes []string
	version           string

	// 运行时状态
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

// Connect 连接到 Admin 并建立双向流
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

	// 1. 发送注册消息
	if err := e.sendRegister(); err != nil {
		return err
	}

	// 2. 启动接收协程
	go e.receiveLoop()

	// 3. 启动心跳协程
	go e.heartbeatLoop()

	// 4. 启动拉取协程
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
		// 无任务，继续等待
		log.Printf("No task available")
		return
	}

	log.Printf("Received task: %s, task_type=%s", task.TaskId, task.TaskType)

	// 检查是否还有空闲槽位
	e.tasksMu.RLock()
	runningCount := len(e.currentTasks)
	e.tasksMu.RUnlock()

	if runningCount >= int(e.maxConcurrency) {
		log.Printf("No available slot for task %s, rejecting", task.TaskId)
		// 可以发送拒绝消息，让 Admin 重新分配
		return
	}

	// 创建任务上下文
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

	// 启动任务执行
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

	// 模拟任务执行进度
	// 实际场景中，这里会拆解为子任务并行执行
	for progress := int32(0); progress <= 100; progress += 10 {
		select {
		case <-ctx.Done():
			// 任务被取消
			log.Printf("Task %s cancelled", task.TaskId)
			e.sendTaskResult(task.TaskId, "CANCELLED", "", "task cancelled")
			return
		case <-time.After(500 * time.Millisecond):
			// 模拟进度更新
			taskCtx.Progress = progress
			e.sendTaskProgress(task.TaskId, progress, "processing...")
			log.Printf("Task %s progress: %d%%", task.TaskId, progress)
		}
	}

	// 模拟子任务拆解和执行（演示）
	e.executeWithSubTasks(task)

	// 任务完成
	resultURL := "http://storage.example.com/results/" + task.TaskId + ".json"
	e.sendTaskResult(task.TaskId, "COMPLETED", resultURL, "")
	log.Printf("Task %s completed", task.TaskId)
}

// executeWithSubTasks 演示子任务拆解和并行执行
func (e *Executor) executeWithSubTasks(task *common.TaskAssignment) {
	log.Printf("Task %s: splitting into subtasks", task.TaskId)

	// 模拟拆解为 4 个子任务
	subTasks := []struct {
		id    string
		index int
	}{
		{task.TaskId + "-sub1", 1},
		{task.TaskId + "-sub2", 2},
		{task.TaskId + "-sub3", 3},
		{task.TaskId + "-sub4", 4},
	}

	// 用于等待所有子任务完成
	var wg sync.WaitGroup
	results := make(chan error, len(subTasks))

	// 并行执行子任务
	for _, st := range subTasks {
		wg.Add(1)
		go func(subID string, idx int) {
			defer wg.Done()

			log.Printf("Subtask %s started", subID)
			// 模拟子任务执行
			time.Sleep(1 * time.Second)
			log.Printf("Subtask %s completed", subID)
			results <- nil
		}(st.id, st.index)
	}

	// 等待所有子任务完成
	wg.Wait()
	close(results)

	// 检查是否有失败的子任务
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

		// 检查是否有空闲槽位
		e.tasksMu.RLock()
		runningCount := len(e.currentTasks)
		e.tasksMu.RUnlock()

		if runningCount >= int(e.maxConcurrency) {
			// 没有空闲槽位，等待一段时间再尝试
			time.Sleep(1 * time.Second)
			continue
		}

		// 发送拉取请求
		if err := e.sendPullRequest(); err != nil {
			log.Printf("Failed to send pull request: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		// 等待一段时间再发送下一个拉取请求
		// 实际场景中，Admin 会在响应中返回任务，不需要等待
		time.Sleep(100 * time.Millisecond)
	}
}

// Stop 优雅关闭 Executor
func (e *Executor) Stop() {
	log.Printf("Stopping executor %s", e.id)
	e.cancel()

	// 等待所有任务完成
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
