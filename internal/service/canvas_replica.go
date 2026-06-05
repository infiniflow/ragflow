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
	"fmt"
	"time"

	"ragflow/internal/cache"
	"ragflow/internal/entity"
)

const canvasReplicaTTL = 3 * time.Hour

// CanvasReplicaPayload represents the structure saved in Redis.
type CanvasReplicaPayload struct {
	CanvasID       string         `json:"canvas_id"`
	TenantID       string         `json:"tenant_id"`
	RuntimeUserID  string         `json:"runtime_user_id"`
	Title          string         `json:"title"`
	CanvasCategory string         `json:"canvas_category"`
	DSL            entity.JSONMap `json:"dsl"`
	UpdatedAt      int64          `json:"updated_at"`
}

// CanvasReplicaService handles saving and retrieving canvas snapshots in Redis.
type CanvasReplicaService struct{}

func NewCanvasReplicaService() *CanvasReplicaService {
	return &CanvasReplicaService{}
}

// Bootstrap creates a replica if absent and keeps existing runtime state.
func (s *CanvasReplicaService) Bootstrap(canvasID, tenantID, runtimeUserID string, dsl entity.JSONMap, canvasCategory, title string) (*CanvasReplicaPayload, error) {
	redisClient := cache.Get()
	if redisClient == nil || !redisClient.IsAlive() {
		return nil, fmt.Errorf("redis client not initialized or unavailable")
	}

	if dsl == nil {
		dsl = entity.JSONMap{}
	}
	key := fmt.Sprintf("canvas:replica:%s:%s:%s", canvasID, tenantID, runtimeUserID)

	var payload CanvasReplicaPayload
	if ok := redisClient.GetObj(key, &payload); ok {
		payload.DSL = NormalizeChunkerDSL(payload.DSL)
		return &payload, nil
	}

	if canvasCategory == "" {
		canvasCategory = "agent_canvas"
	}

	payload = CanvasReplicaPayload{
		CanvasID:       canvasID,
		TenantID:       tenantID,
		RuntimeUserID:  runtimeUserID,
		Title:          title,
		CanvasCategory: canvasCategory,
		DSL:            NormalizeChunkerDSL(dsl),
		UpdatedAt:      time.Now().Unix(),
	}

	if ok := redisClient.SetObj(key, payload, canvasReplicaTTL); !ok {
		return nil, fmt.Errorf("failed to save canvas replica to redis")
	}

	return &payload, nil
}
