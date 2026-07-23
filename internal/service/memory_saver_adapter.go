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

package service

import (
	"context"
	"fmt"

	"ragflow/internal/agent/component"
)

// memorySaverAdapter bridges the component.MemorySaver interface to
// MemoryService.AddMessage. It is installed via component.SetMemorySaver
// at boot time so that the Message component can persist conversation
// turns to memory stores declared in the canvas DSL.
type memorySaverAdapter struct {
	svc *MemoryService
}

// NewMemorySaverAdapter returns a component.MemorySaver backed by the
// given MemoryService.
func NewMemorySaverAdapter(svc *MemoryService) component.MemorySaver {
	return &memorySaverAdapter{svc: svc}
}

// Save implements component.MemorySaver. It delegates to
// MemoryService.AddMessage which handles access filtering, message
// construction, embedding, and async task queueing — the same pipeline
// used by the REST API add_message endpoint.
func (a *memorySaverAdapter) Save(ctx context.Context, req component.MemorySaveRequest) error {
	if a == nil || a.svc == nil {
		return fmt.Errorf("memory: saver adapter not initialised")
	}
	msg := MemoryMessage{
		UserID:        req.UserID,
		AgentID:       req.AgentID,
		SessionID:     req.SessionID,
		UserInput:     req.UserInput,
		AgentResponse: req.AgentResponse,
	}
	ok, detail, err := a.svc.AddMessage(ctx, req.UserID, req.MemoryIDs, msg)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("memory: %s", detail)
	}
	return nil
}
