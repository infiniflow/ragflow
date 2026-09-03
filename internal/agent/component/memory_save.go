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

// Memory persistence scaffold. The Python Message component,
// when configured with memory_ids and memory_save=true, calls
// `queue_save_to_memory_task` from
// `api.db.joint_services.memory_message_service`. The Go runtime
// has a partial port (see internal/service/memory_message_service.go);
// this file defines the contract surface the Message component
// uses and a stub saver that returns ErrMemoryServiceMissing
// when no real implementation is wired.
//
// The interface (MemorySaver) is intentionally small: it takes a
// MemorySaveRequest and returns nil. A follow-up phase wires the
// real implementation, which will translate the request into the
// same DB row shape Python produces (api4conversation table per
// plan §2.11.6).

package component

import (
	"context"
	"errors"
	"sync"
)

// ErrMemoryServiceMissing is the deferred-state sentinel for
// memory persistence. The Message component wraps persistence
// calls in errors.Is checks so callers can detect the gap.
var ErrMemoryServiceMissing = errors.New(
	"component: memory persistence not yet wired in Go — " +
		"defer to Python Canvas or implement MemorySaver",
)

// MemorySaveRequest is the wire shape. It mirrors the Python
// `message_dict` built in message.py:_save_to_memory:
//
//	{
//	  "user_id":     ...,
//	  "agent_id":    ...,
//	  "session_id":  ...,
//	  "user_input":  ...,
//	  "agent_response": ...,
//	}
type MemorySaveRequest struct {
	MemoryIDs     []string // the canvas-declared memory_ids
	UserID        string
	AgentID       string
	SessionID     string
	UserInput     string
	AgentResponse string
}

// MemorySaver is the abstract interface for memory persistence.
// The default implementation returns ErrMemoryServiceMissing.
type MemorySaver interface {
	Save(ctx context.Context, req MemorySaveRequest) error
}

var (
	memSaverMu   sync.RWMutex
	memSaverImpl MemorySaver = stubMemorySaver{}
)

// SetMemorySaver installs a custom saver. Passing nil reverts to
// the default stub. Production code calls this at boot once
// internal/service/memory_message_service lands.
func SetMemorySaver(s MemorySaver) {
	memSaverMu.Lock()
	defer memSaverMu.Unlock()
	if s == nil {
		memSaverImpl = stubMemorySaver{}
		return
	}
	memSaverImpl = s
}

// GetMemorySaver returns the registered saver.
func GetMemorySaver() MemorySaver {
	memSaverMu.RLock()
	defer memSaverMu.RUnlock()
	return memSaverImpl
}

type stubMemorySaver struct{}

func (stubMemorySaver) Save(_ context.Context, _ MemorySaveRequest) error {
	return ErrMemoryServiceMissing
}
