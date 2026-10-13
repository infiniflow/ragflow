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
	"strings"

	"ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

// memorySaverAdapter bridges the component.MemorySaver interface to the
// agent-internal memory queue. It is installed via component.SetMemorySaver
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

// Save implements component.MemorySaver. It checks the runtime user against
// each configured memory before queueing the message.
func (a *memorySaverAdapter) Save(ctx context.Context, req component.MemorySaveRequest) error {
	if a == nil || a.svc == nil {
		return fmt.Errorf("memory: saver adapter not initialised")
	}
	authorizationUserID := req.UserID
	if len(req.MemoryIDs) > 0 {
		state, stateErr := runtime.GetStateFromContext(ctx)
		if stateErr != nil || state == nil {
			return fmt.Errorf("%w: canvas runtime state is required to use memory from an agent", permission.ErrInvalidPermission)
		}
		canvasID, _ := state.Sys["canvas_id"].(string)
		userID, _ := state.Sys["user_id"].(string)
		canvasID = strings.TrimSpace(canvasID)
		userID = strings.TrimSpace(userID)
		authorizationUserID = userID
		if canvasID == "" {
			return fmt.Errorf("%w: canvas id is required to use memory from an agent", permission.ErrInvalidPermission)
		}
		if userID == "" {
			return permission.ErrUnauthenticated
		}
		checker := permission.NewDatabaseChecker(dao.DB)
		entry := permission.ResourceRef{Kind: permission.ResourceKindCanvas, ID: canvasID}
		for _, memoryID := range req.MemoryIDs {
			memoryID = strings.TrimSpace(memoryID)
			if memoryID == "" {
				continue
			}
			if err := checker.CheckDependency(ctx, permission.Subject{UserID: userID}, entry,
				permission.ResourceRef{Kind: permission.ResourceKindMemory, ID: memoryID},
				permission.OperationRun, permission.OperationUse); err != nil {
				return fmt.Errorf("memory %q: %w", memoryID, err)
			}
		}
	}
	msg := MemoryMessage{
		UserID:        req.UserID,
		AgentID:       req.AgentID,
		SessionID:     req.SessionID,
		UserInput:     req.UserInput,
		AgentResponse: req.AgentResponse,
	}
	ok, detail, err := a.svc.saveAgentMessage(ctx, authorizationUserID, req.MemoryIDs, msg)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("memory: %s", detail)
	}
	return nil
}
