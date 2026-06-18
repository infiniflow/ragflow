//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package ingestion

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"sync"
	"time"

	"ragflow/internal/common"

	"google.golang.org/grpc"
)

type Ingestor struct {
	id          string
	name        string
	serverAddr  string
	conn        *grpc.ClientConn
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

	ingestionTaskDAO       *dao.IngestionTaskDAO
	ingestionTaskLogDAO    *dao.IngestionTaskLogDAO
	ingestionTaskletDAO    *dao.IngestionTaskletDAO
	ingestionTaskletLogDAO *dao.IngestionTaskletLogDAO
}

type TaskLog struct {
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time"`
	Description string                 `json:"description"`
	Details     map[string]interface{} `json:"details"`
}

type TaskContext struct {
	Ctx        context.Context
	CancelFunc context.CancelFunc
	// if tasklet is nil, this context is belonged to a task
	// if task and tasklet are both not nil, this context is belonged to a tasklet, the task is the parent task of the tasklet
	Task                   *entity.IngestionTask
	Tasklet                *entity.IngestionTasklet
	Logs                   []*TaskLog
	estimatedRemainingTime time.Duration // estimated cost in seconds to complete the task
	Progress               int32
	ErrorMessage           string
	TaskHandle             common.TaskHandle
}

func NewIngestor(name string, maxConcurrency int32, supportedTypes []string) *Ingestor {
	ctx, cancel := context.WithCancel(context.Background())
	id := common.GenerateUUID()
	return &Ingestor{
		id:                     id,
		name:                   name,
		ctx:                    ctx,
		cancel:                 cancel,
		maxConcurrency:         maxConcurrency,
		supportedDocTypes:      supportedTypes,
		version:                "1.0.0",
		currentTasks:           make(map[string]*TaskContext),
		taskChan:               make(chan *TaskContext, maxConcurrency*2),
		ShutdownCh:             make(chan struct{}, 1),
		ingestionTaskDAO:       dao.NewIngestionTaskDAO(),
		ingestionTaskLogDAO:    dao.NewIngestionTaskLogDAO(),
		ingestionTaskletDAO:    dao.NewIngestionTaskletDAO(),
		ingestionTaskletLogDAO: dao.NewIngestionTaskletLogDAO(),
	}
}

func (e *Ingestor) ID() string {
	return e.id
}

func (e *Ingestor) Start() error {
	common.Info(fmt.Sprintf("Ingestor %s initialized", e.id))
	msgQueueEngine := engine.GetMessageQueueEngine()
	err := msgQueueEngine.InitConsumer("tasks.RAGFLOW")
	if err != nil {
		return err
	}

	// Ensure worker pool is started on first task
	go e.startWorkerPool()

	for {
		var taskHandles []common.TaskHandle
		taskHandles, err = msgQueueEngine.GetMessages(4)
		if err != nil {
			common.Error("error consuming message", err)
			continue
		}
		for _, taskHandle := range taskHandles {
			taskMessage := taskHandle.GetMessage()
			common.Info(fmt.Sprintf("Received task id: %s, type: %s", taskMessage.TaskID, taskMessage.TaskType))
			if taskMessage.TaskType != common.TaskTypeIngestionTask {
				common.Info(fmt.Sprintf("task %s is not an ingestion task", taskMessage.TaskID))
				err = taskHandle.Ack()
				if err != nil {
					common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), err)
					return err
				}
				continue
			}
			var task *entity.IngestionTask
			task, err = e.ingestionTaskDAO.SetRunningByIngestor(taskMessage.TaskID)
			if err != nil {
				if errors.Is(err, common.ErrTaskNotFound) {
					common.Warn(fmt.Sprintf("task %s not found, skipping", taskMessage.TaskID))
					err = taskHandle.Ack()
					if err != nil {
						common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), err)
						return err
					}
					continue
				} else {
					common.Error(fmt.Sprintf("error setting task %s to running", taskMessage.TaskID), err)
					return err
				}
			}
			if task == nil {
				common.Info(fmt.Sprintf("task %s is already removed", taskMessage.TaskID))
				err = taskHandle.Ack()
				if err != nil {
					return err
				}
				continue
			}

			switch task.Status {
			case common.COMPLETED, common.STOPPED, common.FAILED:
				common.Info(fmt.Sprintf("task %s is already %s", taskMessage.TaskID, task.Status))
				err = taskHandle.Ack()
				if err != nil {
					common.Error(fmt.Sprintf("error nack task %s", taskMessage.TaskID), err)
					return err
				}
				continue
			case common.STOPPING, common.CREATED:
				err = fmt.Errorf("task %s is in unexpected status %s", taskMessage.TaskID, task.Status)
				return err
			case common.RUNNING:
			}

			// Construct TaskContext with a cancellable context
			ctx, cancel := context.WithCancel(e.ctx)
			taskCtx := &TaskContext{
				Ctx:        ctx,
				CancelFunc: cancel,
				Task:       task,
				TaskHandle: taskHandle,
			}

			// Register in currentTasks immediately so heartbeat sees PENDING state
			//e.tasksMu.Lock()
			//e.currentTasks[task.ID] = taskCtx
			//e.tasksMu.Unlock()

			// Push to task channel; if full, reject the task (backpressure)
			select {
			case e.taskChan <- taskCtx:
				common.Info(fmt.Sprintf("Task %s queued (channel: %d/%d)", task.ID, len(e.taskChan), cap(e.taskChan)))
			default:
				common.Info(fmt.Sprintf("No available slot for task %s, failed", task.ID))

				//e.tasksMu.Lock()
				//delete(e.currentTasks, task.ID)
				//e.tasksMu.Unlock()

				err = taskHandle.Nack()
				if err != nil {
					common.Error(fmt.Sprintf("error nack task %s", taskMessage.TaskID), err)
					return err
				}
			}
		}
	}
}

//// Connect connects to the admin and establishes a bidirectional stream
//func (e *Ingestor) Connect(serverAddr string) error {
//	e.serverAddr = serverAddr
//	conn, err := grpc.Dial(serverAddr,
//		grpc.WithTransportCredentials(insecure.NewCredentials()),
//		grpc.WithBlock(),
//		grpc.WithTimeout(5*time.Second),
//	)
//	if err != nil {
//		return fmt.Errorf("fail to connect admin server: %s", err.Error())
//	}
//	e.conn = conn
//
//	e.client = common.NewIngestionManagerClient(conn)
//
//	stream, err := e.client.Action(e.ctx)
//	if err != nil {
//		conn.Close()
//		return err
//	}
//	e.stream = stream
//
//	common.Info(fmt.Sprintf("Ingestor %s connected to admin", e.id))
//
//	// 1. Send registration message
//	if err = e.sendRegister(); err != nil {
//		conn.Close()
//		return err
//	}
//
//	// Ensure worker pool is started on first task
//	e.startWorkerPool()
//
//	// 2. Start receive loop
//	go e.receiveLoop()
//
//	// 3. Start heartbeat loop
//	go e.heartbeatLoop()
//
//	return nil
//}

//func (e *Ingestor) sendRegister() error {
//	msg := &common.IngestionMessage{
//		IngestorId:  e.id,
//		MessageType: "REGISTER",
//		RegisterInfo: &common.RegisterInfo{
//			MaxConcurrency:    e.maxConcurrency,
//			SupportedDocTypes: e.supportedDocTypes,
//			Version:           e.version,
//			Name:              e.name,
//		},
//	}
//	return e.stream.Send(msg)
//}
//
//func (e *Ingestor) sendHeartbeat() error {
//	e.tasksMu.RLock()
//
//	cutoff := time.Now().Add(-10 * time.Minute)
//	var toDeleteTask []string
//	taskStates := make([]*common.TaskState, 0, len(e.currentTasks))
//
//	for tid, tc := range e.currentTasks {
//		// Check if task is in a terminal state and expired beyond 10 minutes
//		if (tc.Status == "CANCELED" || tc.Status == "COMPLETED" || tc.Status == "REJECTED") &&
//			!tc.EndTime.IsZero() && tc.EndTime.Before(cutoff) {
//			toDeleteTask = append(toDeleteTask, tid)
//		} else {
//			taskStates = append(taskStates, &common.TaskState{
//				TaskId:                        tid,
//				Status:                        tc.Status,
//				EstimatedRemainingTimeSeconds: int64(tc.estimatedRemainingTime),
//				ErrorMessage:                  tc.ErrorMessage,
//				StartTime:                     tc.StartTime.UnixNano(),
//				ComeFrom:                      tc.Task.ComeFrom,
//			})
//		}
//	}
//	e.tasksMu.RUnlock()
//
//	// Delete expired terminal tasks from currentTasks
//	if len(toDeleteTask) > 0 {
//		e.tasksMu.Lock()
//		for _, id := range toDeleteTask {
//			delete(e.currentTasks, id)
//		}
//		e.tasksMu.Unlock()
//	}
//
//	var pid = int64(os.Getpid())
//	p, err := process.NewProcess(int32(pid))
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	var cpuPercent float64
//	cpuPercent, err = p.Percent(100 * time.Millisecond)
//	if err != nil {
//		cpuPercent = math.NaN()
//		common.Info(fmt.Sprintf("Fail to read CPU usage: %v", err))
//	}
//
//	RssUsage := math.NaN()
//	VmsUsage := math.NaN()
//	memInfo, err := p.MemoryInfo()
//	if err == nil {
//		RssUsage = float64(memInfo.RSS)
//		VmsUsage = float64(memInfo.VMS)
//	} else {
//		common.Info(fmt.Sprintf("Fail to read memory usage: %v", err))
//	}
//	msg := &common.IngestionMessage{
//		IngestorId:  e.id,
//		MessageType: "HEARTBEAT",
//		HeartbeatInfo: &common.HeartbeatInfo{
//			TaskStates:    taskStates,
//			DeleteTaskIds: toDeleteTask,
//			CpuUsage:      float32(cpuPercent),
//			VmsUsage:      float32(VmsUsage),
//			RssUsage:      float32(RssUsage),
//			ProcessId:     pid,
//		},
//	}
//	return e.stream.Send(msg)
//}
//
//func (e *Ingestor) sendTaskResult(taskID, status, errorMsg string) error {
//	msg := &common.IngestionMessage{
//		IngestorId:  e.id,
//		MessageType: "TASK_RESULT",
//		TaskResult: &common.TaskResult{
//			TaskId:       taskID,
//			Status:       status,
//			ErrorMessage: errorMsg,
//		},
//	}
//	return e.stream.Send(msg)
//}
//
//func (e *Ingestor) sendTaskProgress(taskID string, progress int32, info string) error {
//	msg := &common.IngestionMessage{
//		IngestorId:  e.id,
//		MessageType: "TASK_PROGRESS",
//		TaskProgress: &common.TaskProgress{
//			TaskId:   taskID,
//			Progress: progress,
//			Info:     info,
//		},
//	}
//	return e.stream.Send(msg)
//}
//
//func (e *Ingestor) receiveLoop() {
//	for {
//		msg, err := e.stream.Recv()
//		if err != nil {
//			if e.ctx.Err() != nil {
//				common.Info(fmt.Sprintf("Ingestor %s context cancelled, receive loop exiting", e.id))
//				return
//			}
//			common.Info(fmt.Sprintf("Receive error: %v", err))
//			common.Info("Admin connection lost, attempting to reconnect")
//			e.reconnect()
//			return
//		}
//
//		switch msg.MessageType {
//		case "TASK_ASSIGNMENT":
//			e.handleTaskAssignment(msg.TaskAssignment)
//
//		case "ACK":
//			common.Info(fmt.Sprintf("Received ACK: task=%s, success=%v, msg=%s",
//				msg.AckInfo.TaskId, msg.AckInfo.Success, msg.AckInfo.Message))
//
//		case "ERROR":
//			common.Info(fmt.Sprintf("Received error from admin: %s", msg.ErrorMessage))
//
//		default:
//			common.Info(fmt.Sprintf("Unknown admin message type: %s", msg.MessageType))
//		}
//	}
//}
//
//func (e *Ingestor) handleTaskAssignment(task *common.TaskAssignment) {
//	if task == nil {
//		return
//	}
//
//	common.Info(fmt.Sprintf("Received task: %s, task_type=%s", task.TaskId, task.TaskType))
//
//	switch task.TaskType {
//	case "shutdown_ingestor":
//		if e.id == task.AssignedTo {
//			e.handleShutdownIngestor()
//			return
//		}
//
//		common.Error("unmatched ingestor id", fmt.Errorf("attempt to shutdown ingestor: %s, current ingestor: %s, mismatched", task.AssignedTo, e.id))
//		return
//	case "cancel_ingestion_task":
//		e.handleCancelTask(task.TaskId)
//		return
//	case "start_ingestion_task":
//		// create ingestion task log
//		err := e.ingestionTaskLogDAO.Create(&entity.IngestionTaskLog{
//			TaskID: task.TaskId,
//			Action: "CREATED",
//		})
//		if err != nil {
//			common.Fatal(fmt.Sprintf("Failed to create ingestion task log for task %s: %v", task.TaskId, err))
//			return
//		}
//	}
//
//	// Construct TaskContext with a cancellable context
//	ctx, cancel := context.WithCancel(e.ctx)
//	taskCtx := &TaskContext{
//		Ctx:        ctx,
//		CancelFunc: cancel,
//		Task:       task,
//		Status:     "QUEUED",
//	}
//
//	// Register in currentTasks immediately so heartbeat sees PENDING state
//	e.tasksMu.Lock()
//	e.currentTasks[task.TaskId] = taskCtx
//	e.tasksMu.Unlock()
//
//	common.Info("wait for 10 seconds")
//	time.Sleep(time.Second * 10)
//	// Push to task channel; if full, reject the task (backpressure)
//	select {
//	case e.taskChan <- taskCtx:
//		common.Info(fmt.Sprintf("Task %s queued (channel: %d/%d)", task.TaskId, len(e.taskChan), cap(e.taskChan)))
//	default:
//		common.Info(fmt.Sprintf("No available slot for task %s, rejecting", task.TaskId))
//		//e.tasksMu.Lock()
//		//delete(e.currentTasks, task.TaskId)
//		//e.tasksMu.Unlock()
//		taskCtx.Status = "REJECTED"
//		taskCtx.EndTime = time.Now()
//		e.sendTaskResult(taskCtx.Task.TaskId, "REJECTED", "task rejected before execution")
//	}
//}
//
//func (e *Ingestor) handleCancelTask(taskID string) {
//	e.tasksMu.Lock()
//	taskCtx, exists := e.currentTasks[taskID]
//	e.tasksMu.Unlock()
//
//	if !exists {
//		common.Info(fmt.Sprintf("Cancel request for unknown task %s, ignoring", taskID))
//		return
//	}
//
//	common.Info(fmt.Sprintf("Cancelling task %s (current status: %s)", taskID, taskCtx.Status))
//	taskCtx.CancelFunc()
//}
//
//func (e *Ingestor) handleShutdownIngestor() {
//	common.Info(fmt.Sprintf("Shutdown task received, initiating graceful shutdown of ingestor %s", e.id))
//	select {
//	case e.ShutdownCh <- struct{}{}:
//	default:
//	}
//	return
//}

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
			if taskCtx.Tasklet != nil {
				e.executeTasklet(taskCtx)
			} else {
				e.executeTask(taskCtx)
			}
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
	common.Info(fmt.Sprintf("Starting task %s", task.ID))

	latestLog, err := e.ingestionTaskLogDAO.LatestLogByTaskID(task.ID)
	if err != nil {
		latestLog = &entity.IngestionTaskLog{
			ID:     0,
			TaskID: task.ID,
			Checkpoint: entity.JSONMap{
				"current_step": 1,
				"total_step":   5,
			},
		}
		err = e.ingestionTaskLogDAO.Create(latestLog)
		if err != nil {
			common.Error(fmt.Sprintf("Failed to create task log for task %s", task.ID), err)
			return
		}
	}

	var checkpointMap map[string]interface{}
	checkpointMap = latestLog.Checkpoint
	currentStep, ok := common.GetInt(checkpointMap["current_step"])
	if !ok {
		common.Fatal(fmt.Sprintf("Failed to get current step from task log for task %s", task.ID))
		return
	}
	totalStep, ok := common.GetInt(checkpointMap["total_step"])
	if !ok {
		common.Fatal(fmt.Sprintf("Failed to get current step from task log for task %s", task.ID))
		return
	}
	for i := currentStep; i < totalStep; i++ {
		select {
		case <-ctx.Done():
			// Task canceled
			common.Info(fmt.Sprintf("Task %s stopped", task.ID))
			return
		case <-time.After(5000 * time.Millisecond):
			common.Info(fmt.Sprintf("Task %s is running step %d", task.ID, i))
			checkpointMap["current_step"] = i + 1
			latestLog.Checkpoint = checkpointMap
			latestLog.ID++
			err = latestLog.UpdateCreateDateAndTime()
			if err != nil {
				common.Error(fmt.Sprintf("Failed to update date and time of task log for task %s", task.ID), err)
				return
			}

			err = e.ingestionTaskLogDAO.Create(latestLog)
			if err != nil {
				common.Error(fmt.Sprintf("Failed to create task log for task %s", task.ID), err)
				return
			}
		}
	}

	err = e.ingestionTaskDAO.UpdateStatus(task.ID, common.COMPLETED)
	if err != nil {
		common.Error(fmt.Sprintf("Task %s update status failed", task.ID), err)
		return
	}

	common.Info(fmt.Sprintf("Task %s completed", task.ID))
}

func (e *Ingestor) executeTasklet(taskCtx *TaskContext) {
	ctx := taskCtx.Ctx
	tasklet := taskCtx.Tasklet
	common.Info(fmt.Sprintf("Starting tasklet %s", tasklet.ID))

	latestLog, err := e.ingestionTaskletLogDAO.LatestLogByTaskletID(tasklet.ID)
	if err != nil {
		latestLog = &entity.IngestionTaskletLog{
			TaskletID: tasklet.ID,
			Checkpoint: entity.JSONMap{
				"current_step": 0,
				"total_step":   3,
			},
		}
		err = e.ingestionTaskletLogDAO.Create(latestLog)
		if err != nil {
			common.Error(fmt.Sprintf("Failed to create task log for tasklet %s", tasklet.ID), err)
			return
		}
	}

	var checkpointMap map[string]interface{}
	checkpointMap = latestLog.Checkpoint
	currentStep := checkpointMap["current_step"].(int)
	totalStep := checkpointMap["total_step"].(int)
	for i := currentStep; i < totalStep; i++ {
		select {
		case <-ctx.Done():
			// Task canceled
			common.Info(fmt.Sprintf("Tasklet %s stopped", tasklet.ID))
			return
		case <-time.After(3000 * time.Millisecond):
			common.Info(fmt.Sprintf("Tasklet %s is running step %d", tasklet.ID, i))
			checkpointMap["current_step"] = i + 1
			latestLog.Checkpoint = checkpointMap
			err = e.ingestionTaskletLogDAO.Create(latestLog)
			if err != nil {
				common.Error(fmt.Sprintf("Failed to update task log for tasklet %s", tasklet.ID), err)
				return
			}
		}
	}

	err = e.ingestionTaskletDAO.UpdateStatus(tasklet.ID, common.STOPPED)
	if err != nil {
		common.Error(fmt.Sprintf("Tasklet %s update status failed", tasklet.ID), err)
		return
	}

	common.Info(fmt.Sprintf("Tasklet %s completed", tasklet.ID))
}

//
//func (e *Ingestor) heartbeatLoop() {
//	ticker := time.NewTicker(5 * time.Second)
//	defer ticker.Stop()
//
//	for {
//		select {
//		case <-e.ctx.Done():
//			return
//		case <-ticker.C:
//			if err := e.sendHeartbeat(); err != nil {
//				common.Info(fmt.Sprintf("Failed to send heartbeat: %v", err))
//				if e.ctx.Err() != nil {
//					common.Info(fmt.Sprintf("Ingestor %s context cancelled, heartbeat loop exiting", e.id))
//					return
//				}
//				common.Info(fmt.Sprintf("Admin connection lost, attempting to reconnect"))
//				e.reconnect()
//				return
//			}
//		}
//	}
//}
//
//// reconnect closes the old connection and establishes a new one with exponential backoff.
//// Only one reconnection attempt runs at a time; concurrent callers return immediately.
//func (e *Ingestor) reconnect() {
//	if e.ctx.Err() != nil {
//		common.Info(fmt.Sprintf("Ingestor %s is shutting down, skipping reconnection", e.id))
//		return
//	}
//
//	if !e.reconnectMu.TryLock() {
//		return
//	}
//	defer e.reconnectMu.Unlock()
//
//	common.Info(fmt.Sprintf("Ingestor %s attempting to reconnect to admin at %s", e.id, e.serverAddr))
//
//	// Close old stream and connection
//	if e.stream != nil {
//		e.stream.CloseSend()
//	}
//	if e.conn != nil {
//		e.conn.Close()
//	}
//
//	backoff := 1 * time.Second
//	maxBackoff := 30 * time.Second
//
//	for {
//		conn, err := grpc.Dial(e.serverAddr,
//			grpc.WithTransportCredentials(insecure.NewCredentials()),
//			grpc.WithBlock(),
//			grpc.WithTimeout(5*time.Second),
//		)
//		if err != nil {
//			common.Info(fmt.Sprintf("Reconnect dial failed: %v, retrying in %v", err, backoff))
//			time.Sleep(backoff)
//			backoff *= 2
//			if backoff > maxBackoff {
//				backoff = maxBackoff
//			}
//			continue
//		}
//		e.conn = conn
//		e.client = common.NewIngestionManagerClient(conn)
//
//		stream, err := e.client.Action(e.ctx)
//		if err != nil {
//			conn.Close()
//			common.Info(fmt.Sprintf("Reconnect create stream failed: %v, retrying in %v", err, backoff))
//			time.Sleep(backoff)
//			backoff *= 2
//			if backoff > maxBackoff {
//				backoff = maxBackoff
//			}
//			continue
//		}
//		e.stream = stream
//
//		if err = e.sendRegister(); err != nil {
//			stream.CloseSend()
//			conn.Close()
//			common.Info(fmt.Sprintf("Reconnect register failed: %v, retrying in %v", err, backoff))
//			time.Sleep(backoff)
//			backoff *= 2
//			if backoff > maxBackoff {
//				backoff = maxBackoff
//			}
//			continue
//		}
//
//		common.Info(fmt.Sprintf("Ingestor %s reconnected to admin", e.id))
//		break
//	}
//
//	// Restart the loops on the new stream
//	go e.receiveLoop()
//	go e.heartbeatLoop()
//}

// Stop gracefully shuts down the ingestor
func (e *Ingestor) Stop() {
	common.Info(fmt.Sprintf("Stopping ingestor %s", e.id))
	e.cancel()

	// Wait for all workers to finish (they exit on ctx.Done())
	e.workerWg.Wait()
	common.Info("All tasks completed")
}
