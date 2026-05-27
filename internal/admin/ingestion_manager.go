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

package admin

import (
	"context"
	"fmt"
	"net"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"sync"
	"time"

	"ragflow/internal/common"

	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
)

const heartbeatTimeout = 30 * time.Second

type IngestionManager struct {
	common.UnimplementedIngestionManagerServer
	mu sync.RWMutex

	// Registered ingestion servers
	ingestionServers map[string]*IngestorState // ingestor id -> ingestor id

	taskStates map[string]*TaskState // task_id -> task state

	// In-memory task queue
	taskQueue chan *pendingTask

	// Notifies that an ingestor slot may have freed up
	slotFreed chan struct{}

	grpcServer *grpc.Server // gRPC server instance for graceful shutdown via Stop()

	ctx    context.Context
	cancel context.CancelFunc

	ingestionTaskDAO    *dao.IngestionTaskDAO
	ingestionTaskLogDAO *dao.IngestionTaskLogDAO
}

type TaskState struct {
	taskID                 string // same as task_id in database
	status                 string // created, assigned, processing, completed, failed
	comeFrom               string // api server id
	assignTo               string // ingestor id
	lastUpdate             time.Time
	startTime              *time.Time
	estimatedRemainingTime time.Duration // estimated cost in seconds to complete the task
	errorMessage           string
}

type IngestorState struct {
	ID            string
	Info          *common.RegisterInfo
	LastHeartbeat time.Time
	Stream        common.IngestionManager_ActionServer
	Status        string // active, draining
	Address       string
	ProcessID     int64
	cpuUsage      float64
	vmsUsage      float64
	rssUsage      float64
}

type pendingTask struct {
	Task      *common.TaskAssignment
	CreatedAt time.Time
}

var ingestionManager *IngestionManager

func GetIngestionManager() *IngestionManager {
	return ingestionManager
}

func NewAdminServer() *IngestionManager {
	ctx, cancel := context.WithCancel(context.Background())
	ingestionManager = &IngestionManager{
		taskStates:          make(map[string]*TaskState),
		ingestionServers:    make(map[string]*IngestorState),
		taskQueue:           make(chan *pendingTask, 10000),
		slotFreed:           make(chan struct{}, 100),
		ctx:                 ctx,
		cancel:              cancel,
		ingestionTaskDAO:    dao.NewIngestionTaskDAO(),
		ingestionTaskLogDAO: dao.NewIngestionTaskLogDAO(),
	}
	go ingestionManager.dispatchLoop()
	//go ingestionManager.heartbeatCheckLoop() no need to check heartbeat timeout
	return ingestionManager
}

func (s *IngestionManager) RestoreTasks() error {
	ingestionTasks, err := ingestionManager.ingestionTaskDAO.GetAllTasks(0, 0)
	if err != nil {
		return err
	}
	for _, ingestionTask := range ingestionTasks {
		switch ingestionTask.Status {
		case "CREATED":
		case "CANCELLING":
			{
				var log *entity.IngestionTaskLog
				log, err = s.ingestionTaskLogDAO.LatestLogByTaskID(ingestionTask.ID)

				if log == nil {
					// no log means not assigned
					task := &common.TaskAssignment{
						TaskId:   ingestionTask.ID,
						UserId:   ingestionTask.UserID,
						ComeFrom: "Admin",
						TaskType: ingestionTask.Status,
					}
					s.tryAssign(task)
				}
			}
		default:
			// COMPLETED / FAILED / CANCELED, ignore
			continue
		}
	}

	// created no log, select an ingestor and send
	// cancelling no log, select an ingestor and send

	// created has log, send to the ingestor that ingestor will restore it
	// cancelling has log, send to the ingestor that ingestor will restore it
	// running with log, send to the ingestor that ingestor will restore it

	// status with log, but ingestor name can't be found. Administrator will be responsible to reassign it to other ingestor

	// completed / failed/ canceled, ignore
	return nil
}

// Action handles the bidirectional streaming RPC from ingestion servers
func (s *IngestionManager) Action(stream common.IngestionManager_ActionServer) error {
	var ingestionServerID string
	var state *IngestorState

	common.Info("New ingestion_server connection")

	// Start receive goroutine
	receiveErrorCH := make(chan error, 1)
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				receiveErrorCH <- err
				return
			}
			s.handleMessage(stream, msg, &ingestionServerID, &state)
		}
	}()

	// Start send goroutine: send tasks immediately when assigned to this ingestion_server
	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		for {
			select {
			case <-stream.Context().Done():
				return
			case <-s.ctx.Done():
				return
			}
		}
	}()

	select {
	case err := <-receiveErrorCH:
		// Connection dropped, clean up
		s.cleanupIngestionServer(ingestionServerID)
		return err
	case <-sendDone:
		// Stream context canceled (client disconnect or server shutdown)
		s.cleanupIngestionServer(ingestionServerID)
		return nil
	}
}

func (s *IngestionManager) handleMessage(
	stream common.IngestionManager_ActionServer,
	msg *common.IngestionMessage,
	ingestionServerID *string,
	state **IngestorState,
) {
	switch msg.MessageType {
	case "REGISTER":
		s.handleRegister(stream, msg, ingestionServerID, state)

	case "HEARTBEAT":
		s.handleHeartbeat(msg, *ingestionServerID, *state)

	case "TASK_RESULT":
		s.handleTaskResult(msg, *ingestionServerID, *state)

	case "TASK_PROGRESS":
		s.handleTaskProgress(msg, *ingestionServerID, *state)

	default:
		common.Info(fmt.Sprintf("Unknown message type: %s", msg.MessageType))
		err := stream.Send(&common.AdminMessage{
			MessageType:  "ERROR",
			ErrorMessage: "unknown message type",
		})
		if err != nil {
			common.Error("Fail to send unknown message", err)
			return
		}
	}
}

func (s *IngestionManager) handleRegister(
	stream common.IngestionManager_ActionServer,
	msg *common.IngestionMessage,
	ingestionServerID *string,
	state **IngestorState,
) {
	if msg.RegisterInfo == nil {
		err := stream.Send(&common.AdminMessage{
			MessageType:  "ERROR",
			ErrorMessage: "missing register info",
		})
		if err != nil {
			common.Error("Fail to send missing register info", err)
			return
		}
		return
	}

	peerHost, ok := peer.FromContext(stream.Context())
	if !ok {
		err := stream.Send(&common.AdminMessage{
			MessageType:  "ERROR",
			ErrorMessage: "peer not found in context",
		})
		if err != nil {
			common.Error("Fail to send 'peer not found' message", err)
			return
		}
		return
	}
	clientAddr := peerHost.Addr.String()

	*ingestionServerID = msg.IngestorId
	*state = &IngestorState{
		ID:            msg.IngestorId,
		Info:          msg.RegisterInfo,
		LastHeartbeat: time.Now().Truncate(time.Second),
		Stream:        stream,
		Status:        "active",
		Address:       clientAddr,
	}

	s.mu.Lock()
	s.ingestionServers[*ingestionServerID] = *state
	s.mu.Unlock()

	err := stream.Send(&common.AdminMessage{
		MessageType: "ACK",
		AckInfo: &common.AckInfo{
			TaskId:  "",
			Success: true,
			Message: "registered successfully",
		},
	})
	if err != nil {
		common.Error("Fail to send ACK message", err)
		return
	}

	common.Info(fmt.Sprintf("Ingestor %s registered, max_concurrency=%d, supported_types=%v",
		*ingestionServerID, msg.RegisterInfo.MaxConcurrency, msg.RegisterInfo.SupportedDocTypes))
}

func (s *IngestionManager) handleHeartbeat(msg *common.IngestionMessage, ingestorID string, state *IngestorState) {
	if state == nil {
		return
	}

	state.LastHeartbeat = time.Now().Truncate(time.Second)

	if msg.HeartbeatInfo != nil {

		lastUpdateTime := time.Now().Truncate(time.Second)
		s.mu.Lock()
		ingestorState := s.ingestionServers[msg.IngestorId]
		ingestorState.LastHeartbeat = lastUpdateTime
		if ingestorState.Status == "timeout" {
			ingestorState.Status = "active"
			common.Info(fmt.Sprintf("Ingestor %s recovered from timeout, status set to active", msg.IngestorId))
		}
		ingestorState.ProcessID = msg.HeartbeatInfo.ProcessId
		ingestorState.cpuUsage = float64(msg.HeartbeatInfo.CpuUsage)
		ingestorState.vmsUsage = float64(msg.HeartbeatInfo.VmsUsage) / 1024 / 1024 // in MB
		ingestorState.rssUsage = float64(msg.HeartbeatInfo.RssUsage) / 1024 / 1024 // in MB

		// Delete expired terminal tasks from currentTasks
		for _, taskID := range msg.HeartbeatInfo.DeleteTaskIds {
			delete(s.taskStates, taskID)
		}

		for _, ingestorTaskState := range msg.HeartbeatInfo.TaskStates {
			localTaskState := s.taskStates[ingestorTaskState.TaskId]
			if localTaskState == nil {
				startTime := time.Unix(0, ingestorTaskState.StartTime)
				localTaskState = &TaskState{
					taskID:    ingestorTaskState.TaskId,
					comeFrom:  ingestorTaskState.ComeFrom,
					startTime: &startTime,
				}
			}
			localTaskState.estimatedRemainingTime = time.Duration(ingestorTaskState.EstimatedRemainingTimeSeconds)
			localTaskState.lastUpdate = lastUpdateTime
			localTaskState.status = ingestorTaskState.Status
			localTaskState.errorMessage = ingestorTaskState.ErrorMessage
			localTaskState.assignTo = msg.IngestorId
		}
		s.mu.Unlock()

		common.Debug(fmt.Sprintf("Heartbeat from %s", ingestorID))
	}
}

func (s *IngestionManager) handleTaskResult(msg *common.IngestionMessage, ingestorID string, state *IngestorState) {
	if msg.TaskResult == nil {
		return
	}

	result := msg.TaskResult
	common.Info(fmt.Sprintf("Task result from %s: task=%s, status=%s, message=%s", ingestorID, result.TaskId, result.Status, result.ErrorMessage))

	// Signal that a slot may have freed up for pending tasks
	select {
	case s.slotFreed <- struct{}{}:
	default:
	}
}

func (s *IngestionManager) handleTaskProgress(msg *common.IngestionMessage, ingestorID string, state *IngestorState) {
	if msg.TaskProgress == nil {
		return
	}

	progress := msg.TaskProgress
	common.Info(fmt.Sprintf("Task progress from %s: task=%s, progress=%d%%, detail=%s",
		ingestorID, progress.TaskId, progress.Progress, progress.Info))
}

// SubmitTask is for API Server to call (non-gRPC, for testing only)
func (s *IngestionManager) SubmitTask(task *common.TaskAssignment) {
	s.taskQueue <- &pendingTask{
		Task:      task,
		CreatedAt: time.Now().Truncate(time.Second),
	}
	common.Info(fmt.Sprintf("Task %s submitted to queue", task.TaskId))

	// Wake up dispatchLoop if it's blocked waiting for a slot
	select {
	case s.slotFreed <- struct{}{}:
	default:
	}
}

// dispatchLoop pulls tasks from the queue and assigns them to available ingestors.
// Runs in a background goroutine.
func (s *IngestionManager) dispatchLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case pending := <-s.taskQueue:
			go s.tryAssign(pending.Task)
		}
	}
}

// heartbeatCheckLoop periodically checks all registered ingestors for heartbeat timeout.
// If an ingestor's LastHeartbeat is older than heartbeatTimeout, its status is set to "timeout".
func (s *IngestionManager) heartbeatCheckLoop() {
	ticker := time.NewTicker(heartbeatTimeout / 3)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.checkHeartbeats()
		}
	}
}

func (s *IngestionManager) checkHeartbeats() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Truncate(time.Second)
	for id, state := range s.ingestionServers {
		if now.Sub(state.LastHeartbeat) > heartbeatTimeout {
			if state.Status != "timeout" {
				state.Status = "timeout"
				common.Info(fmt.Sprintf("Ingestor %s heartbeat timeout, marked as timeout", id))
			}
		}
	}
}

func (s *IngestionManager) SelectIngestorForTask(task *common.TaskAssignment) (*IngestorState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch task.TaskType {
	case "CREATED":
		if s.taskStates[task.TaskId] == nil {
			// already dispatched or running
			return nil, fmt.Errorf("task: %s is already dispatched or running", task.TaskId)
		}
		for _, ingestor := range s.ingestionServers {
			if ingestor.Status == "active" {
				s.taskStates[task.TaskId] = &TaskState{
					taskID:     task.TaskId,
					status:     "DISPATCHED",
					comeFrom:   task.ComeFrom,
					startTime:  nil,
					lastUpdate: time.Now().Truncate(time.Second),
					assignTo:   ingestor.ID,
				}
				return ingestor, nil
			}
		}
		// no ingestor  available
	case "CANCELING":
		return s.ingestionServers[task.AssignedTo], nil
	case "SHUTDOWN":
		return s.ingestionServers[task.AssignedTo], nil
	}

	return nil, fmt.Errorf("no ingestor available")
}

// tryAssign repeatedly tries to find an available ingestor and assign the task.
// Blocks until either the task is assigned or the context is canceled.
func (s *IngestionManager) tryAssign(task *common.TaskAssignment) {
	for {

		target, err := s.SelectIngestorForTask(task)
		if err != nil {
			common.Info(err.Error())
			return
		}

		if target != nil {
			// CREATED, has available ingestor
			// CANCELLING and SHUTDOWN, has available ingestor
			task.AssignedTo = target.ID
			s.assignToIngestor(task, target)
			return
		}

		// for CREATED, no available ingestor, loop until one ingestor is available
		// for CANCELLING, corresponding ingestor isn't started.
		// - ingestion task hasn't started in ingestor
		// - corresponding ingestor is shutdown, manually assign the cancelling task to another ingestor
		// for SHUTDOWN, corresponding ingestor isn't started, return since the ingestor isn't started from admin point of view

		if task.TaskType == "start_ingestion_task" {
			// Receives a start ingestion task, save and change the states
			s.mu.Lock()
			s.taskStates[task.TaskId] = &TaskState{
				taskID:     task.TaskId,
				status:     "pending",
				comeFrom:   task.ComeFrom,
				lastUpdate: time.Now().Truncate(time.Second),
				startTime:  nil,
			}
			s.mu.Unlock()
		} else {
			// shutdown ingestor or cancel task
			common.Info("Task is completed, canceled, or ingestor is shutdown")
			return
		}

		// No ingestor available, wait for a slot to free up
		select {
		case <-s.ctx.Done():
			return
		case <-s.slotFreed:
			// A slot might be free, retry
		case <-time.After(2 * time.Second):
			// Periodic retry as fallback
		}
	}
}

func (s *IngestionManager) assignToIngestor(task *common.TaskAssignment, state *IngestorState) {

	err := state.Stream.Send(&common.AdminMessage{
		MessageType:    "TASK_ASSIGNMENT",
		TaskAssignment: task, // CREATED, CANCELLING, SHUTDOWN
	})
	if err != nil {
		common.Info(fmt.Sprintf("Failed to assign task %s to ingestor %s: %v", task.TaskId, state.ID, err))
		// Re-queue the task
		s.taskQueue <- &pendingTask{Task: task, CreatedAt: time.Now().Truncate(time.Second)}
		return
	}
	common.Info(fmt.Sprintf("Assigned task %s to ingestion_server %s", task.TaskId, state.ID))
}

func (s *IngestionManager) cleanupIngestionServer(ingestorID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ingestorID == "" {
		// Client disconnected before REGISTER completed — nothing to clean up
		common.Info("Unregistered ingestion server disconnected")
		return
	}

	if _, exists := s.ingestionServers[ingestorID]; exists {
		delete(s.ingestionServers, ingestorID)
		common.Info(fmt.Sprintf("Ingestor %s cleaned up", ingestorID))

		// Clean the tasks handled by this ingestor
		var tasksToDelete []string
		for _, taskState := range s.taskStates {
			if taskState.assignTo == ingestorID {
				tasksToDelete = append(tasksToDelete, taskState.taskID)
			}
		}
		for _, taskID := range tasksToDelete {
			delete(s.taskStates, taskID)
		}
	}
}

func (s *IngestionManager) ListIngestors() ([]map[string]interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []map[string]interface{}
	for ingestorID, state := range s.ingestionServers {

		var taskCount int64
		for _, task := range s.taskStates {
			if task.assignTo == ingestorID {
				taskCount++
			}
		}

		result = append(result, map[string]interface{}{
			"id":             ingestorID,
			"name":           state.Info.Name,
			"address":        state.Address,
			"last_heartbeat": state.LastHeartbeat,
			"task_count":     taskCount,
			"status":         state.Status,
			"cpu_usage":      state.cpuUsage,
			"rss_usage":      state.rssUsage,
			"vms_usage":      state.vmsUsage,
			"process_id":     state.ProcessID,
		})
	}
	return result, nil
}

func (s *IngestionManager) ListIngestionTasks() ([]map[string]interface{}, error) {

	var result []map[string]interface{}

	ingestionTasks, err := s.ingestionTaskDAO.GetAllTasks(0, 0)
	if err != nil {
		return nil, err
	}

	//ID         string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	//UserID     string  `gorm:"column:user_id;size:32;not null" json:"user_id"`
	//DocumentID string  `gorm:"column:document_id;size:32;not null;index" json:"document_id"`
	//DatasetID  string  `gorm:"column:dataset_id;size:32;not null" json:"dataset_id"`
	//Schema     JSONMap `gorm:"column:schema;type:longtext" json:"schema"`
	//Status     string  `gorm:"column:status;size:32;not null;" json:"status"`
	//CreateTime *int64     `gorm:"column:create_time;index" json:"create_time,omitempty"`
	//CreateDate *time.Time `gorm:"column:create_date;index" json:"create_date,omitempty"`
	//UpdateTime *int64     `gorm:"column:update_time;index" json:"update_time,omitempty"`
	//UpdateDate *time.Time `gorm:"column:update_date;index" json:"update_date,omitempty"`
	var taskID2index = make(map[string]int)
	for idx, task := range ingestionTasks {
		result = append(result, map[string]interface{}{
			"id":          task.ID,
			"user_id":     task.UserID,
			"dataset_id":  task.DatasetID,
			"document_id": task.DocumentID,
			"schema":      task.Schema,
			"status":      task.Status,
			"create_time": task.CreateTime,
			"from":        "",
			"assign_to":   "",
			"ETA":         time.Duration(0),
			"error":       "",
			"last_update": time.Time{},
			"phase":       "",
		})
		taskID2index[task.ID] = idx
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for index, taskState := range s.taskStates {
		common.Info(fmt.Sprintf("Task %s: %s", index, taskState.taskID))
		taskResult := result[taskID2index[taskState.taskID]]
		taskResult["phase"] = taskState.status
		taskResult["from"] = taskState.comeFrom
		taskResult["assign_to"] = taskState.assignTo
		taskResult["last_update"] = taskState.lastUpdate
		taskResult["error"] = taskState.errorMessage
		taskResult["ETA"] = taskState.estimatedRemainingTime
	}

	return result, nil
}

// Start starts the admin service
func (s *IngestionManager) Start(port string) error {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		return err
	}

	s.grpcServer = grpc.NewServer()
	common.RegisterIngestionManagerServer(s.grpcServer, s)

	return s.grpcServer.Serve(lis)
}

// Stop gracefully shuts down the admin service
func (s *IngestionManager) Stop() {
	common.Info("Stopping RAGFlow ingestion manager...")

	// Notify all goroutines to exit
	s.cancel()

	// Gracefully stop gRPC server (stop accepting new connections, wait for in-flight requests)
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	// Close the task queue
	s.mu.Lock()
	close(s.taskQueue)
	s.mu.Unlock()

	common.Info("RAGFlow ingestion manager stopped")
}
