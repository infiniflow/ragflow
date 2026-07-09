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
	"errors"
	"fmt"
	"ragflow/internal/utility"
	"time"

	"ragflow/internal/dao"
	redisengine "ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	models "ragflow/internal/entity/models"
)

// ErrEmbedderNotWired is returned by QueueSaveToMemoryTask when
// the embedding-model call is reached. The Go runtime has no
// embedding model port yet; until one lands, callers see this
// error and know to fall back to the Python Canvas.
var ErrEmbedderNotWired = errors.New(
	"memory: embedder not wired in Go — " +
		"QueueSaveToMemoryTask runs the lookup + message construction " +
		"but cannot embed / save until internal/rag/llm/embedding_model " +
		"ships (Phase 8b follow-up)",
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
func (s *MemoryMessageService) QueueSaveToMemoryTask(
	ctx context.Context,
	memoryIDs []string,
	msg MemoryMessage,
) (*QueueSaveResult, error) {
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
		// (1) Look up the memory.
		mem, err := s.memories.GetMemoryConfig(memoryID)
		if err != nil {
			res.NotFound = append(res.NotFound, memoryID)
			continue
		}
		// (2) + (3) build the raw_message envelope. The Go port
		// keeps the same field set as Python:344-386 so the
		// downstream extractor (also still on the Python side)
		// can consume the row without schema changes.
		rawMessageID := generateRawMessageID()
		rawMessage := buildRawMessage(rawMessageID, memoryID, mem, msg)

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
		if err := queueMemoryTask(memoryID, mem.TenantID, rawMessageID, task, msg); err != nil {
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
func generateRawMessageID() int64 {
	if redisClient := redisengine.Get(); redisClient != nil {
		if id := redisClient.GenerateAutoIncrementID("id_generator", "memory", 1, nil); id > 0 {
			return id
		}
	}
	return time.Now().UnixNano()
}

// buildRawMessage constructs the raw_message envelope that gets
// passed to embed_and_save (and persisted in the message table
// for the async extractor to read).
func buildRawMessage(
	rawMessageID int64,
	memoryID string,
	mem *CreateMemoryResponse, // from MemoryService.GetMemoryConfig
	msg MemoryMessage,
) map[string]any {
	content := fmt.Sprintf("User Input: %s\nAgent Response: %s",
		msg.UserInput, msg.AgentResponse)
	out := map[string]any{
		"message_id":             rawMessageID,
		"message_type":           "raw",
		"message_type_kwd":       "raw",
		"source_id":              0,
		"memory_id":              memoryID,
		"user_id":                msg.UserID,
		"agent_id":               msg.AgentID,
		"session_id":             msg.SessionID,
		"content":                content,
		"content_ltks":           content,
		"tokenized_content_ltks": content,
		"valid_at":               time.Now().UTC().Format("2006-01-02 15:04:05"),
		"invalid_at":             nil,
		"forget_at":              nil,
		"status":                 true,
		"status_int":             1,
	}
	if mem != nil {
		// The embedder uses the memory's embd_id; keep the
		// pointer on the envelope so embed_and_save can
		// pick the right model when it lands.
		out["_memory_embd_id"] = mem.EmbdID
	}
	return out
}

// buildTaskRow constructs the Task row the async extractor polls.
func buildTaskRow(rawMessageID int64, memoryID string) map[string]any {
	return map[string]any{
		"id":        newUUIDString(),
		"doc_id":    memoryID,
		"task_type": "memory",
		"progress":  0.0,
		"begin_at":  time.Now(),
		"digest":    fmt.Sprintf("%d", rawMessageID),
	}
}

func (s *MemoryMessageService) embedAndSave(ctx context.Context, mem *CreateMemoryResponse, rawMessage map[string]any) error {
	if mem == nil {
		return errors.New("memory not found")
	}
	if s == nil || s.memories == nil || s.memories.docEngine == nil {
		return errors.New("message store is not initialized")
	}

	content, _ := rawMessage["content"].(string)
	driver, modelName, apiConfig, maxTokens, err := NewModelProviderService().GetModelConfigFromProviderInstance(mem.TenantID, entity.ModelTypeEmbedding, mem.EmbdID)
	if err != nil {
		return err
	}
	embeddingModel := models.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)
	embeddings, err := embeddingModel.ModelDriver.Embed(embeddingModel.ModelName, []string{content}, embeddingModel.APIConfig, &models.EmbeddingConfig{Dimension: 0})
	if err != nil {
		return err
	}
	if len(embeddings) == 0 || len(embeddings[0].Embedding) == 0 {
		return errors.New("embedding response is empty")
	}

	vector := embeddings[0].Embedding
	rawMessage[fmt.Sprintf("q_%d_vec", len(vector))] = vector
	rawMessage["id"] = fmt.Sprintf("%s_%d", rawMessage["memory_id"], rawMessage["message_id"])
	rawMessage["doc_id"] = rawMessage["memory_id"]

	indexName := memoryIndexName(mem.TenantID)
	exists, err := s.memories.docEngine.ChunkStoreExists(ctx, indexName, mem.ID)
	if err != nil {
		return fmt.Errorf("check message index: %w", err)
	}
	if !exists {
		if err := s.memories.docEngine.CreateChunkStore(ctx, indexName, mem.ID, len(vector), ""); err != nil {
			return fmt.Errorf("create message index: %w", err)
		}
	}
	if _, err := s.memories.docEngine.InsertChunks(ctx, []map[string]interface{}{mapStringAny(rawMessage)}, indexName, mem.ID); err != nil {
		return fmt.Errorf("insert message into memory: %w", err)
	}

	return nil
}

// embedAndSave is kept for older unit tests; production uses the method above.
func embedAndSave(_ context.Context, _ *CreateMemoryResponse, _ map[string]any) error {
	return ErrEmbedderNotWired
}

func (s *MemoryMessageService) insertTask(_ context.Context, row map[string]any) error {
	if s == nil {
		return errors.New("nil MemoryMessageService")
	}
	if s.taskDAO == nil {
		s.taskDAO = dao.NewTaskDAO()
	}
	return s.taskDAO.Create(taskFromRow(row))
}

// newUUIDString is a thin wrapper so we can swap in a real UUID
// generator later without changing call sites. Avoids an
// import-cycle with internal/uuid at the package boundary.
func newUUIDString() string {
	return utility.GenerateUUID()
}

func taskFromRow(row map[string]any) *entity.Task {
	digest := fmt.Sprint(row["digest"])
	beginAt, _ := row["begin_at"].(time.Time)
	if beginAt.IsZero() {
		now := time.Now()
		beginAt = now
	}
	return &entity.Task{
		ID:       fmt.Sprint(row["id"]),
		DocID:    fmt.Sprint(row["doc_id"]),
		TaskType: fmt.Sprint(row["task_type"]),
		Progress: 0,
		BeginAt:  &beginAt,
		Digest:   &digest,
	}
}

func queueMemoryTask(memoryID, tenantID string, rawMessageID int64, task map[string]any, msg MemoryMessage) error {
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
	if redisClient := redisengine.Get(); redisClient == nil || !redisClient.QueueProduct(memoryTaskQueueName(0), message) {
		return errors.New("Can't access Redis.")
	}
	return nil
}

func memoryTaskQueueName(priority int) string {
	return fmt.Sprintf("te.%d.common", priority)
}

func mapStringAny(in map[string]any) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
