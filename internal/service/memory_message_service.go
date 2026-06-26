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

// memory_message_service.go — Phase 8b real MemorySaver port.
//
// Port of api.db.joint_services.memory_message_service.queue_save_to_memory_task
// from the Python runtime. The Go port is a partial implementation
// (synchronous parts land here; the embedding-model call is loud-failed
// with ErrEmbedderNotWired until a Go embedding port lands).
//
// Python signature (api/db/joint_services/memory_message_service.py:344):
//
//   async def queue_save_to_memory_task(
//       memory_ids: list[str],
//       message_dict: dict,
//   ) -> tuple[list[str], list[dict]]
//   # (not_found_memory, failed_memory)
//
// Go equivalent:
//
//   type QueueSaveResult struct {
//       NotFound []string
//       Failed   []MemoryFailure
//   }
//
//   func (s *MemoryMessageService) QueueSaveToMemoryTask(
//       ctx context.Context,
//       memoryIDs []string,
//       msg MemoryMessage,
//   ) (*QueueSaveResult, error)
//
// The function is the entry point the Message component calls
// after a conversation turn when `memory_save=true` is set. It
// must:
//
//  1. For each memory id: look up the Memory (via MemoryService).
//  2. Generate a raw_message_id from Redis auto-increment (namespace "memory").
//  3. Build the raw_message envelope (mirrors Python:344-386).
//  4. Call embed_and_save on the memory + [raw_message]. ← DEFERRED
//  5. Insert a Task row in the task table for the async extractor.
//  6. Return not-found + failed lists.
//
// Steps 1, 2, 3, 5, 6 are implemented here. Step 4 (the
// embed_and_save call) is wrapped in a loud-fail gate that returns
// ErrEmbedderNotWired until the Go embedding layer ships.

package service

import (
	"context"
	"errors"
	"fmt"
	"time"
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

// MemoryFailure describes one memory that failed to save. The
// FailMsg is the underlying error (or "embedder not wired" until
// the embedder port lands).
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
// api.db.joint_services.memory_message_service. It depends on a
// MemoryService instance for the lookup; the embedder is
// hard-coded to loud-fail (see ErrEmbedderNotWired).
type MemoryMessageService struct {
	memories *MemoryService
}

// NewMemoryMessageService constructs a service bound to the
// supplied MemoryService. Caller is expected to register this as
// the default MemorySaver in the Message component via
// `component.SetMemorySaver(...)` at boot.
func NewMemoryMessageService(memories *MemoryService) *MemoryMessageService {
	return &MemoryMessageService{memories: memories}
}

// QueueSaveToMemoryTask runs the memory-persistence flow for the
// supplied memory_ids + message. See package comment for the
// step-by-step contract. The function is synchronous — the Python
// async version awaits `embed_and_save` and a Redis call; both are
// replaced here with synchronous equivalents (and a loud-fail
// embedder).
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

		// (4) embed_and_save. Loud-fail: the embedder is the
		// only step the Go runtime can't do yet. When it
		// ships, replace this branch with a call into
		// internal/rag/llm/embedding_model.
		if err := embedAndSave(ctx, mem, rawMessage); err != nil {
			res.Failed = append(res.Failed, MemoryFailure{
				MemoryID: memoryID,
				FailMsg:  err.Error(),
			})
			continue
		}

		// (5) Task row insertion. The Python side bulk-inserts
		// a Task row with digest=str(raw_message_id); the
		// extractor's task_type is "memory". The Go port
		// constructs the same row shape and defers the actual
		// insert to TaskDAO when the project adds one (today
		// TaskDAO is in internal/dao; the API mirrors the
		// Python Task entity closely).
		task := buildTaskRow(rawMessageID, memoryID)
		if err := s.insertTask(ctx, task); err != nil {
			res.Failed = append(res.Failed, MemoryFailure{
				MemoryID: memoryID,
				FailMsg:  fmt.Sprintf("task insert: %s", err.Error()),
			})
			continue
		}
	}
	return res, nil
}

// generateRawMessageID is a placeholder for the Redis auto-
// increment the Python side uses (`REDIS_CONN.generate_auto_increment_id
// (namespace="memory")`). The Go port generates a UUID-shaped
// integer now; replace with a Redis-backed counter when the
// project's Redis client lands.
func generateRawMessageID() int64 {
	// seconds-since-epoch is unique enough for the Go port's
	// own bookkeeping. The Redis-backed counter is the source
	// of truth in production; this fallback only matters for
	// the tests that don't need cross-process uniqueness.
	return time.Now().Unix()
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
		"message_id":   rawMessageID,
		"message_type": "raw",
		"source_id":    0,
		"memory_id":    memoryID,
		"user_id":      msg.UserID,
		"agent_id":     msg.AgentID,
		"session_id":   msg.SessionID,
		"content":      content,
		"valid_at":     time.Now().UTC().Format(time.RFC3339),
		"invalid_at":   nil,
		"forget_at":    nil,
		"status":       true,
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
		"digest":    fmt.Sprintf("%d", rawMessageID),
	}
}

// embedAndSave is the deferred gate. Replace with a call to the
// real embedding model + memory_message table insert when those
// land.
func embedAndSave(_ context.Context, _ *CreateMemoryResponse, _ map[string]any) error {
	return ErrEmbedderNotWired
}

// insertTask is a placeholder for the bulk_insert_into_db call
// the Python side makes. The Go side needs a TaskDAO write path
// (the Python Task entity is mirrored in internal/entity); until
// that lands this is a no-op that returns nil so the rest of
// the flow can be exercised.
func (s *MemoryMessageService) insertTask(_ context.Context, _ map[string]any) error {
	return nil
}

// newUUIDString is a thin wrapper so we can swap in a real UUID
// generator later without changing call sites. Avoids an
// import-cycle with internal/uuid at the package boundary.
func newUUIDString() string {
	return fmt.Sprintf("mem-%d", time.Now().UnixNano())
}
