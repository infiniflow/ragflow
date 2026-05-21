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

	// In-memory task queue
	taskQueue chan *pendingTask

	// Notifies that an ingestor slot may have freed up
	slotFreed chan struct{}

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
	s := &IngestionManager{
		ingestionServers: make(map[string]*IngestionState),
		taskQueue:        make(chan *pendingTask, 10000),
		slotFreed:        make(chan struct{}, 100),
		ctx:              ctx,
		cancel:           cancel,
	}
	go s.dispatchLoop()
	return s
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

	log.Printf("Ingestor %s registered, max_concurrency=%d, supported_types=%v",
		*ingestionServerID, msg.RegisterInfo.MaxConcurrency, msg.RegisterInfo.SupportedDocTypes)
}

func (s *IngestionManager) handleHeartbeat(msg *common.IngestionMessage, ingestorID string, state *IngestionState) {
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

		log.Printf("Heartbeat from %s: %d active tasks", ingestorID, len(newTasks))
	}
}

func (s *IngestionManager) handleTaskResult(msg *common.IngestionMessage, ingestorID string, state *IngestionState) {
	if msg.TaskResult == nil {
		return
	}

	result := msg.TaskResult
	log.Printf("Task result from %s: task=%s, status=%s", ingestorID, result.TaskId, result.Status)

	// Remove from the ingestion_server's current task list
	if state != nil {
		delete(state.CurrentTasks, result.TaskId)
	}

	// Signal that a slot may have freed up for pending tasks
	select {
	case s.slotFreed <- struct{}{}:
	default:
	}
}

func (s *IngestionManager) handleTaskProgress(msg *common.IngestionMessage, ingestorID string, state *IngestionState) {
	if msg.TaskProgress == nil {
		return
	}

	progress := msg.TaskProgress
	log.Printf("Task progress from %s: task=%s, progress=%d%%, detail=%s",
		ingestorID, progress.TaskId, progress.Progress, progress.Info)
}

// SubmitTask is for API Server to call (non-gRPC, for testing only)
func (s *IngestionManager) SubmitTask(task *common.TaskAssignment) {
	s.taskQueue <- &pendingTask{
		Task:      task,
		CreatedAt: time.Now(),
	}
	log.Printf("Task %s submitted to queue", task.TaskId)

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

// tryAssign repeatedly tries to find an available ingestor and assign the task.
// Blocks until either the task is assigned or the context is canceled.
func (s *IngestionManager) tryAssign(task *common.TaskAssignment) {
	for {
		s.mu.RLock()
		var target *IngestionState
		for _, state := range s.ingestionServers {
			if state.Status == "active" && len(state.CurrentTasks) < int(state.Info.MaxConcurrency) {
				target = state
				break
			}
		}
		s.mu.RUnlock()

		if target != nil {
			s.assignToIngestor(task, target)
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

func (s *IngestionManager) assignToIngestor(task *common.TaskAssignment, state *IngestionState) {
	s.mu.Lock()
	state.CurrentTasks[task.TaskId] = true
	s.mu.Unlock()

	err := state.Stream.Send(&common.AdminMessage{
		MessageType:    "TASK_ASSIGNMENT",
		TaskAssignment: task,
	})
	if err != nil {
		log.Printf("Failed to assign task %s to ingestor %s: %v", task.TaskId, state.ID, err)
		s.mu.Lock()
		delete(state.CurrentTasks, task.TaskId)
		s.mu.Unlock()
		// Re-queue the task
		s.taskQueue <- &pendingTask{Task: task, CreatedAt: time.Now()}
		return
	}
	log.Printf("Assigned task %s to ingestion_server %s", task.TaskId, state.ID)
}

func (s *IngestionManager) cleanupIngestionServer(ingestorID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.ingestionServers[ingestorID]; exists {
		delete(s.ingestionServers, ingestorID)
		log.Printf("Ingestor %s cleaned up", ingestorID)
	}
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
