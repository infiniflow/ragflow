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

// memory_message_service.go — real MemorySaver port.
//
// Port of api.db.joint_services.memory_message_service.queue_save_to_memory_task
// from the Python runtime.
//
// Python signature (api/db/joint_services/memory_message_service.py:344):
//
//	async def queue_save_to_memory_task(
//	    memory_ids: list[str],
//	    message_dict: dict,
//	) -> tuple[list[str], list[dict]]
//	# (not_found_memory, failed_memory)
//
// Go equivalent:
//
//	type QueueSaveResult struct {
//	    NotFound []string
//	    Failed   []MemoryFailure
//	}
//
//	func (s *MemoryMessageService) QueueSaveToMemoryTask(
//	    ctx context.Context,
//	    memoryIDs []string,
//	    msg MemoryMessage,
//	) (*QueueSaveResult, error)
//
// The function is the entry point the Message component calls
// after a conversation turn when `memory_save=true` is set. It
// must:
//
//  1. For each memory id: look up the Memory (via MemoryService).
//  2. Generate a raw_message_id from Redis auto-increment (namespace "memory").
//  3. Build the raw_message envelope (mirrors Python:344-386).
//  4. Call embed_and_save on the memory + [raw_message].
//  5. Insert a Task row in the task table for the async extractor.
//  6. Return not-found + failed lists.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	redisengine "ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	models "ragflow/internal/entity/models"
	"ragflow/internal/utility"
)

// MemoryMessage is the wire shape for QueueSaveToMemoryTask. It
// mirrors the Python `message_dict` built in
// agent/component/message.py:_save_to_memory:
//
//	{
//	  "user_id":        str,
//	  "agent_id":       str,
//	  "session_id":     str,
//	  "user_input":     str,
//	  "agent_response": str,
//	}
type MemoryMessage struct {
	UserID        string
	AgentID       string
	SessionID     string
	UserInput     string
	AgentResponse string
}

// MemoryFailure describes one memory that failed to save.
type MemoryFailure struct {
	MemoryID string
	FailMsg  string
}

// QueueSaveResult is the return value. NotFound / Failed mirror
// the Python `not_found_memory` / `failed_memory` lists.
type QueueSaveResult struct {
	NotFound []string
	Failed   []MemoryFailure
}

// MemoryMessageService is the Go port of
// api.db.joint_services.memory_message_service.
type MemoryMessageService struct {
	memories *MemoryService
	taskDAO  *dao.TaskDAO
}

// NewMemoryMessageService constructs a service bound to the
// supplied MemoryService. Caller is expected to register this as
// the default MemorySaver in the Message component via
// `component.SetMemorySaver(...)` at boot.
func NewMemoryMessageService(memories *MemoryService) *MemoryMessageService {
	return &MemoryMessageService{
		memories: memories,
		taskDAO:  dao.NewTaskDAO(),
	}
}

// QueueSaveToMemoryTask runs the memory-persistence flow for the
// supplied memory_ids + message. See package comment for the
// step-by-step contract. The function is synchronous — the Python
// async version awaits `embed_and_save` and Redis calls; this Go port does the
// same work synchronously from the HTTP request path.
//
// Returned QueueSaveResult has NotFound / Failed populated for
// the per-memory outcomes. The outer error is reserved for
// call-level failures (e.g. invalid input); per-memory failures
// go into Failed, mirroring the Python tuple shape.
func (s *MemoryMessageService) QueueSaveToMemoryTask(ctx context.Context, memoryIDs []string, msg MemoryMessage) (*QueueSaveResult, error) {
	if len(memoryIDs) == 0 {
		return &QueueSaveResult{}, nil
	}
	if msg.AgentID == "" {
		return nil, errors.New("memory: message.AgentID is required")
	}
	if s == nil || s.memories == nil {
		return nil, errors.New("memory: nil MemoryMessageService or memory dependency")
	}

	res := &QueueSaveResult{}
	for _, memoryID := range memoryIDs {
		// (1) Look up the memory (no access control — trusted internal queue processing).
		mem, err := s.memories.getMemoryConfig(ctx, memoryID)
		if err != nil {
			res.NotFound = append(res.NotFound, memoryID)
			continue
		}
		// (2) + (3) build the raw_message envelope. The Go port
		// keeps the same field set as Python:344-386 so the
		// downstream extractor can consume the row without
		// schema changes.
		rawMessageID := generateRawMessageID(ctx)
		rawMessage := buildRawMessage(rawMessageID, memoryID, msg)

		if err := s.embedAndSave(ctx, mem, rawMessage); err != nil {
			res.Failed = append(res.Failed, MemoryFailure{
				MemoryID: memoryID,
				FailMsg:  err.Error(),
			})
			continue
		}

		task := buildTaskRow(rawMessageID, memoryID)
		if err := s.insertTask(ctx, task); err != nil {
			res.Failed = append(res.Failed, MemoryFailure{
				MemoryID: memoryID,
				FailMsg:  fmt.Sprintf("task insert: %s", err.Error()),
			})
			continue
		}
		if err = queueMemoryTask(ctx, memoryID, mem.TenantID, rawMessageID, task, msg); err != nil {
			res.Failed = append(res.Failed, MemoryFailure{
				MemoryID: memoryID,
				FailMsg:  err.Error(),
			})
		}
	}
	return res, nil
}

// generateRawMessageID returns the Redis auto-increment id used by the Python
// side (`REDIS_CONN.generate_auto_increment_id(namespace="memory")`).
func generateRawMessageID(ctx context.Context) int64 {
	if redisClient := redisengine.Get(); redisClient != nil {
		if id := redisClient.GenerateAutoIncrementID(ctx, "id_generator", "memory", 1, nil); id > 0 {
			return id
		}
	}
	return time.Now().UnixNano()
}

// buildRawMessage constructs the raw_message envelope that gets
// passed to embed_and_save (and persisted in the message table
// for the async extractor to read). Only logical message fields
// are set here, mirroring Python queue_save_to_memory_task; the
// doc engine maps them to storage fields at insert time
// (Elasticsearch tokenizes content before write, Infinity
// tokenizes on save).
func buildRawMessage(
	rawMessageID int64,
	memoryID string,
	msg MemoryMessage,
) map[string]any {
	content := fmt.Sprintf("User Input: %s\nAgent Response: %s",
		msg.UserInput, msg.AgentResponse)
	return map[string]any{
		"message_id":   rawMessageID,
		"message_type": "raw",
		"source_id":    0,
		"memory_id":    memoryID,
		"user_id":      msg.UserID,
		"agent_id":     msg.AgentID,
		"session_id":   msg.SessionID,
		"content":      content,
		"valid_at":     time.Now().UTC().Format("2006-01-02 15:04:05"),
		"invalid_at":   nil,
		"forget_at":    nil,
		"status":       true,
	}
}

// buildTaskRow constructs the Task row the async extractor polls.
func buildTaskRow(rawMessageID int64, memoryID string) map[string]any {
	return map[string]any{
		"id":           newUUIDString(),
		"doc_id":       memoryID,
		"task_type":    "memory",
		"progress":     0.0,
		"progress_msg": "",
		"begin_at":     time.Now(),
		"digest":       fmt.Sprintf("%d", rawMessageID),
	}
}

func (s *MemoryMessageService) embedAndSave(ctx context.Context, mem *CreateMemoryResponse, rawMessage map[string]any) error {
	return s.embedAndSaveMessages(ctx, mem, []map[string]any{rawMessage})
}

// embedAndSaveMessages embeds every message's content with the memory's
// embedding model and inserts the batch into the memory's chunk store,
// creating the index on first use. Mirrors Python embed_and_save.
func (s *MemoryMessageService) embedAndSaveMessages(ctx context.Context, mem *CreateMemoryResponse, messages []map[string]any) error {
	if mem == nil {
		return errors.New("memory not found")
	}
	if s == nil || s.memories == nil || s.memories.docEngine == nil {
		return errors.New("message store is not initialized")
	}
	if len(messages) == 0 {
		return nil
	}

	contents := make([]string, len(messages))
	for i, message := range messages {
		contents[i], _ = message["content"].(string)
	}
	driver, modelName, apiConfig, maxTokens, err := NewModelProviderService().ResolveModelConfig(ctx, mem.TenantID, entity.ModelTypeEmbedding, mem.EmbdID)
	if err != nil {
		return err
	}
	embeddingModel := models.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)
	embeddings, err := embeddingModel.ModelDriver.Embed(ctx, embeddingModel.ModelName, contents, embeddingModel.APIConfig, &models.EmbeddingConfig{Dimension: 0}, nil)
	if err != nil {
		return err
	}
	if len(embeddings) != len(messages) {
		return fmt.Errorf("embedding response count %d does not match message count %d", len(embeddings), len(messages))
	}

	vectorDim := 0
	for i, message := range messages {
		vector := embeddings[i].Embedding
		if len(vector) == 0 {
			return errors.New("embedding response is empty")
		}
		vectorDim = len(vector)
		message[fmt.Sprintf("q_%d_vec", len(vector))] = vector
		message["id"] = fmt.Sprintf("%s_%v", message["memory_id"], message["message_id"])
		message["doc_id"] = message["memory_id"]
	}

	indexName := memoryIndexName(mem.TenantID)
	exists, err := s.memories.docEngine.ChunkStoreExists(ctx, indexName, mem.ID)
	if err != nil {
		return fmt.Errorf("check message index: %w", err)
	}
	if !exists {
		if err := s.memories.docEngine.CreateChunkStore(ctx, indexName, mem.ID, vectorDim, ""); err != nil {
			return fmt.Errorf("create message index: %w", err)
		}
	}
	docs := make([]map[string]interface{}, len(messages))
	for i, message := range messages {
		docs[i] = mapStringAny(message)
	}
	if _, err := s.memories.docEngine.InsertChunks(ctx, docs, indexName, mem.ID); err != nil {
		return fmt.Errorf("insert message into memory: %w", err)
	}

	return nil
}

func (s *MemoryMessageService) insertTask(ctx context.Context, row map[string]any) error {
	if s == nil {
		return errors.New("nil MemoryMessageService")
	}
	if s.taskDAO == nil {
		s.taskDAO = dao.NewTaskDAO()
	}
	return s.taskDAO.Create(ctx, dao.DB, taskFromRow(row))
}

// newUUIDString is a thin wrapper so we can swap in a real UUID
// generator later without changing call sites. Avoids an
// import-cycle with internal/uuid at the package boundary.
func newUUIDString() string {
	return utility.GenerateUUID()
}

func taskFromRow(row map[string]any) *entity.Task {
	digest := fmt.Sprint(row["digest"])
	progressMsg := fmt.Sprint(row["progress_msg"])
	beginAt, _ := row["begin_at"].(time.Time)
	if beginAt.IsZero() {
		now := time.Now()
		beginAt = now
	}
	return &entity.Task{
		ID:          fmt.Sprint(row["id"]),
		DocID:       fmt.Sprint(row["doc_id"]),
		TaskType:    fmt.Sprint(row["task_type"]),
		Progress:    0,
		ProgressMsg: &progressMsg,
		BeginAt:     &beginAt,
		Digest:      &digest,
	}
}

func queueMemoryTask(ctx context.Context, memoryID, tenantID string, rawMessageID int64, task map[string]any, msg MemoryMessage) error {
	taskID := fmt.Sprint(task["id"])
	message := map[string]any{
		"id":        taskID,
		"task_id":   taskID,
		"task_type": task["task_type"],
		"memory_id": memoryID,
		"tenant_id": tenantID,
		"source_id": rawMessageID,
		"message_dict": map[string]any{
			"user_id":        msg.UserID,
			"agent_id":       msg.AgentID,
			"session_id":     msg.SessionID,
			"user_input":     msg.UserInput,
			"agent_response": msg.AgentResponse,
		},
	}
	// Publish the memory-extraction task to NATS (tasks.RAGFLOW) so it is
	// consumed by the Ingestor's shared consumer + worker pool, dispatched by
	// TaskType=="memory" in processMessage. This keeps Go out of the Python
	// te.*.common Redis stream entirely, removing the cross-consumer
	// contention that previously stole Python dataflow tasks.
	mq := engine.GetMessageQueueEngine()
	if mq == nil {
		return errors.New("can't access message queue engine")
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal memory task payload: %w", err)
	}
	taskMessage := common.TaskMessage{
		TaskID:   taskID,
		TaskType: common.TaskTypeMemory,
		Payload:  payload,
	}
	tmPayload, err := json.Marshal(taskMessage)
	if err != nil {
		return fmt.Errorf("marshal memory task message: %w", err)
	}
	if err := mq.PublishTask(common.TaskSubject, tmPayload); err != nil {
		return fmt.Errorf("publish memory task %s: %w", taskID, err)
	}
	return nil
}

func mapStringAny(in map[string]any) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
