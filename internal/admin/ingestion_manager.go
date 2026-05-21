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
	"log"
	"net"
	"sync"
	"time"

	"ragflow/internal/common"

	"google.golang.org/grpc"
)

type IngestionManager struct {
	common.UnimplementedIngestionManagerServer
	mu sync.RWMutex

	// Registered ingestion servers
	ingestionServers map[string]*IngestionState

	// Pull request queue of executors waiting for tasks
	pendingPulls map[string][]chan *common.TaskAssignment

	// In-memory task queue
	taskQueue chan *pendingTask

	grpcServer *grpc.Server // gRPC server instance for graceful shutdown via Stop()

	ctx    context.Context
	cancel context.CancelFunc
}

type IngestionState struct {
	ID            string
	Info          *common.RegisterInfo
	LastHeartbeat time.Time
	CurrentTasks  map[string]bool // task_id -> whether the task is currently running
	Stream        common.IngestionManager_ActionServer
	Status        string // active, draining
}

type pendingTask struct {
	Task      *common.TaskAssignment
	CreatedAt time.Time
}

func NewAdminServer() *IngestionManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &IngestionManager{
		ingestionServers: make(map[string]*IngestionState),
		pendingPulls:     make(map[string][]chan *common.TaskAssignment),
		taskQueue:        make(chan *pendingTask, 10000),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Action handles the bidirectional streaming RPC from ingestion servers
func (s *IngestionManager) Action(stream common.IngestionManager_ActionServer) error {
	var ingestionServerID string
	var state *IngestionState

	log.Println("New ingestion_server connection")

	// Start receive goroutine
	recvErr := make(chan error, 1)
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				recvErr <- err
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
	case err := <-recvErr:
		// Connection dropped, clean up
		s.cleanupIngestionServer(ingestionServerID)
		return err
	case <-sendDone:
		return nil
	}
}

func (s *IngestionManager) handleMessage(
	stream common.IngestionManager_ActionServer,
	msg *common.IngestionMessage,
	ingestionServerID *string,
	state **IngestionState,
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

	case "PULL_REQUEST":
		s.handlePullRequest(stream, msg, *ingestionServerID, *state)

	default:
		log.Printf("Unknown message type: %s", msg.MessageType)
		stream.Send(&common.AdminMessage{
			MessageType:  "ERROR",
			ErrorMessage: "unknown message type",
		})
	}
}

func (s *IngestionManager) handleRegister(
	stream common.IngestionManager_ActionServer,
	msg *common.IngestionMessage,
	ingestionServerID *string,
	state **IngestionState,
) {
	if msg.RegisterInfo == nil {
		stream.Send(&common.AdminMessage{
			MessageType:  "ERROR",
			ErrorMessage: "missing register info",
		})
		return
	}

	*ingestionServerID = msg.IngestionServerId
	*state = &IngestionState{
		ID:            msg.IngestionServerId,
		Info:          msg.RegisterInfo,
		LastHeartbeat: time.Now(),
		CurrentTasks:  make(map[string]bool),
		Stream:        stream,
		Status:        "active",
	}

	s.mu.Lock()
	s.ingestionServers[*ingestionServerID] = *state
	s.mu.Unlock()

	stream.Send(&common.AdminMessage{
		MessageType: "ACK",
		AckInfo: &common.AckInfo{
			TaskId:  "",
			Success: true,
			Message: "registered successfully",
		},
	})

	log.Printf("Executor %s registered, max_concurrency=%d, supported_types=%v",
		*ingestionServerID, msg.RegisterInfo.MaxConcurrency, msg.RegisterInfo.SupportedDocTypes)
}

func (s *IngestionManager) handleHeartbeat(msg *common.IngestionMessage, executorID string, state *IngestionState) {
	if state == nil {
		return
	}

	state.LastHeartbeat = time.Now()

	if msg.HeartbeatInfo != nil {
		// Update current task list
		newTasks := make(map[string]bool)
		for _, tid := range msg.HeartbeatInfo.CurrentTaskIds {
			newTasks[tid] = true
		}
		state.CurrentTasks = newTasks

		log.Printf("Heartbeat from %s: %d active tasks", executorID, len(newTasks))
	}
}

func (s *IngestionManager) handleTaskResult(msg *common.IngestionMessage, executorID string, state *IngestionState) {
	if msg.TaskResult == nil {
		return
	}

	result := msg.TaskResult
	log.Printf("Task result from %s: task=%s, status=%s", executorID, result.TaskId, result.Status)

	// Remove from the ingestion_server's current task list
	if state != nil {
		delete(state.CurrentTasks, result.TaskId)
	}

	// Could trigger callback or notify clients waiting for results
	// In production, could notify API Server that the task is complete
}

func (s *IngestionManager) handleTaskProgress(msg *common.IngestionMessage, executorID string, state *IngestionState) {
	if msg.TaskProgress == nil {
		return
	}

	progress := msg.TaskProgress
	log.Printf("Task progress from %s: task=%s, progress=%d%%, detail=%s",
		executorID, progress.TaskId, progress.Progress, progress.Info)
}

func (s *IngestionManager) handlePullRequest(
	stream common.IngestionManager_ActionServer,
	msg *common.IngestionMessage,
	executorID string,
	state *IngestionState,
) {
	if state == nil {
		stream.Send(&common.AdminMessage{
			MessageType:  "ERROR",
			ErrorMessage: "not registered",
		})
		return
	}

	// Check if there is an available slot
	if len(state.CurrentTasks) >= int(state.Info.MaxConcurrency) {
		// No available slot, return empty (non-blocking)
		stream.Send(&common.AdminMessage{
			MessageType:    "TASK_ASSIGNMENT",
			TaskAssignment: nil, // nil means no task
		})
		return
	}

	// Dequeue a task
	select {
	case pending := <-s.taskQueue:
		// Task available, assign to this ingestion_server
		state.CurrentTasks[pending.Task.TaskId] = true

		stream.Send(&common.AdminMessage{
			MessageType:    "TASK_ASSIGNMENT",
			TaskAssignment: pending.Task,
		})
		log.Printf("Assigned task %s to ingestion_server %s", pending.Task.TaskId, executorID)

	default:
		// No task available, return empty
		stream.Send(&common.AdminMessage{
			MessageType:    "TASK_ASSIGNMENT",
			TaskAssignment: nil,
		})
	}
}

func (s *IngestionManager) cleanupIngestionServer(executorID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.ingestionServers[executorID]; exists {
		delete(s.ingestionServers, executorID)
		log.Printf("Executor %s cleaned up", executorID)
	}
}

// SubmitTask is for API Server to call (non-gRPC, for testing only)
func (s *IngestionManager) SubmitTask(task *common.TaskAssignment) {
	s.taskQueue <- &pendingTask{
		Task:      task,
		CreatedAt: time.Now(),
	}
	log.Printf("Task %s submitted to queue", task.TaskId)
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

//func main() {
//	server := NewAdminServer()
//
//	// 模拟一个提交任务的 goroutine（实际应由 API Server 调用）
//	go func() {
//		taskID := 1
//		for {
//			time.Sleep(3 * time.Second)
//			server.SubmitTask(&common.TaskAssignment{
//				TaskId:         string(rune(taskID)),
//				DocType:        "pdf",
//				DocUrl:         "http://example.com/doc.pdf",
//				Config:         `{"ocr": true}`,
//				AssignToken:    "token-" + string(rune(taskID)),
//				TimeoutSeconds: 300,
//			})
//			taskID++
//		}
//	}()
//
//	if err := server.Start(":50051"); err != nil {
//		log.Fatal(err)
//	}
//}
