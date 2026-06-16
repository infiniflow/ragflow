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
	"encoding/json"
	"fmt"
	"ragflow/internal/engine/redis"
	"strings"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/server"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
)

// SystemService system service
type SystemService struct{}

// NewSystemService create system service
func NewSystemService() *SystemService {
	return &SystemService{}
}

// ConfigResponse system configuration response
type ConfigResponse struct {
	RegisterEnabled      int  `json:"registerEnabled"`
	DisablePasswordLogin bool `json:"disablePasswordLogin"`
}

// GetConfig get system configuration
func (s *SystemService) GetConfig() (*ConfigResponse, error) {
	cfg := server.GetConfig()
	registerEnabled := 1
	if !cfg.Authentication.RegisterEnabled {
		registerEnabled = 0
	}
	return &ConfigResponse{
		RegisterEnabled:      registerEnabled,
		DisablePasswordLogin: cfg.Authentication.DisablePasswordLogin,
	}, nil
}

// VersionResponse version response
type VersionResponse struct {
	Version string `json:"version"`
}

type HealthzMeta struct {
	Elapsed string `json:"elapsed"`
	Error   string `json:"error,omitempty"`
}

type HealthzResponse struct {
	DB        string                 `json:"db"`
	Redis     string                 `json:"redis"`
	DocEngine string                 `json:"doc_engine"`
	Storage   string                 `json:"storage"`
	Status    string                 `json:"status"`
	Meta      map[string]HealthzMeta `json:"_meta,omitempty"`
}

// GetVersion get RAGFlow version
func (s *SystemService) GetVersion() (*VersionResponse, error) {
	version := utility.GetRAGFlowVersion()
	return &VersionResponse{
		Version: version,
	}, nil
}

// ComponentStatus describes one dependency health check.
type ComponentStatus map[string]interface{}

// StatusResponse system status response.
type StatusResponse struct {
	DocEngine              ComponentStatus          `json:"doc_engine"`
	Storage                ComponentStatus          `json:"storage"`
	Database               ComponentStatus          `json:"database"`
	Redis                  ComponentStatus          `json:"redis"`
	TaskExecutorHeartbeats map[string][]interface{} `json:"task_executor_heartbeats"`
}

// GetStatus gets health status for core system dependencies.
func (s *SystemService) GetStatus() (*StatusResponse, error) {
	return &StatusResponse{
		DocEngine:              s.getDocEngineStatus(),
		Storage:                s.getStorageStatus(),
		Database:               s.getDatabaseStatus(),
		Redis:                  s.getRedisStatus(),
		TaskExecutorHeartbeats: s.getTaskExecutorHeartbeats(),
	}, nil
}

func (s *SystemService) getDocEngineStatus() ComponentStatus {
	cfg := server.GetConfig()
	docEngineType := ""
	if cfg != nil {
		docEngineType = strings.ToLower(string(cfg.DocEngine.Type))
	}

	startedAt := time.Now()
	docEngine := engine.Get()
	if docEngine == nil {
		return ComponentStatus{
			"type":    docEngineType,
			"status":  "red",
			"elapsed": elapsedMilliseconds(startedAt),
			"error":   "doc engine not initialized",
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := docEngine.Ping(ctx); err != nil {
		return ComponentStatus{
			"type":    docEngine.GetType(),
			"status":  "red",
			"elapsed": elapsedMilliseconds(startedAt),
			"error":   err.Error(),
		}
	}

	return ComponentStatus{
		"type":    docEngine.GetType(),
		"status":  "green",
		"elapsed": elapsedMilliseconds(startedAt),
	}
}

func (s *SystemService) getStorageStatus() ComponentStatus {
	cfg := server.GetConfig()
	storageType := ""
	if cfg != nil {
		storageType = strings.ToLower(string(cfg.StorageEngine.Type))
	}

	startedAt := time.Now()
	factory := storage.GetStorageFactory().GetStorage()
	if factory == nil {
		return ComponentStatus{
			"type":    storageType,
			"status":  "red",
			"elapsed": elapsedMilliseconds(startedAt),
			"error":   "storage not initialized",
		}
	}

	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !factory.Health() {
		return ComponentStatus{
			"type":    storageType,
			"status":  "red",
			"elapsed": elapsedMilliseconds(startedAt),
			"error":   "storage health check failed",
		}
	}

	return ComponentStatus{
		"type":    storageType,
		"status":  "green",
		"elapsed": elapsedMilliseconds(startedAt),
	}
}

func (s *SystemService) getDatabaseStatus() ComponentStatus {
	cfg := server.GetConfig()
	databaseType := ""
	if cfg != nil {
		databaseType = cfg.Database.Driver
	}

	startedAt := time.Now()
	if dao.DB == nil {
		return ComponentStatus{
			"type":    databaseType,
			"status":  "red",
			"elapsed": elapsedMilliseconds(startedAt),
			"error":   "database not initialized",
		}
	}

	sqlDB, err := dao.GetDB().DB()
	if err != nil {
		return ComponentStatus{
			"type":    databaseType,
			"status":  "red",
			"elapsed": elapsedMilliseconds(startedAt),
			"error":   err.Error(),
		}
	}

	if err = sqlDB.Ping(); err != nil {
		return ComponentStatus{
			"type":    databaseType,
			"status":  "red",
			"elapsed": elapsedMilliseconds(startedAt),
			"error":   err.Error(),
		}
	}

	return ComponentStatus{
		"type":    databaseType,
		"status":  "green",
		"elapsed": elapsedMilliseconds(startedAt),
	}
}

func (s *SystemService) getRedisStatus() ComponentStatus {
	startedAt := time.Now()
	redisClient := redis.Get()
	if redisClient == nil {
		return ComponentStatus{
			"status":  "red",
			"elapsed": elapsedMilliseconds(startedAt),
			"error":   "redis not initialized",
		}
	}
	if !redisClient.Health() {
		return ComponentStatus{
			"status":  "red",
			"elapsed": elapsedMilliseconds(startedAt),
			"error":   "Lost connection!",
		}
	}

	return ComponentStatus{
		"status":  "green",
		"elapsed": elapsedMilliseconds(startedAt),
	}
}

func (s *SystemService) getTaskExecutorHeartbeats() map[string][]interface{} {
	heartbeatsByExecutor := map[string][]interface{}{}
	redisClient := redis.Get()
	if redisClient == nil {
		return heartbeatsByExecutor
	}

	taskExecutorIDs, err := redisClient.SMembers("TASKEXE")
	if err != nil {
		return heartbeatsByExecutor
	}

	now := float64(time.Now().Unix())
	for _, taskExecutorID := range taskExecutorIDs {
		rawHeartbeats, err := redisClient.ZRangeByScore(taskExecutorID, now-60*30, now)
		if err != nil {
			continue
		}

		heartbeats := make([]interface{}, 0, len(rawHeartbeats))
		for _, rawHeartbeat := range rawHeartbeats {
			var heartbeat interface{}
			if err := json.Unmarshal([]byte(rawHeartbeat), &heartbeat); err != nil {
				heartbeats = append(heartbeats, rawHeartbeat)
				continue
			}
			heartbeats = append(heartbeats, heartbeat)
		}
		heartbeatsByExecutor[taskExecutorID] = heartbeats
	}

	return heartbeatsByExecutor
}

func elapsedMilliseconds(startedAt time.Time) string {
	return fmt.Sprintf("%.1f", float64(time.Since(startedAt).Microseconds())/1000.0)
}

func okNok(ok bool) string {
	if ok {
		return "ok"
	}
	return "nok"
}

func timedHealthCheck(check func() error) (bool, HealthzMeta) {
	start := time.Now()
	err := check()
	meta := HealthzMeta{
		Elapsed: fmt.Sprintf("%.1f", float64(time.Since(start).Microseconds())/1000.0),
	}
	if err != nil {
		meta.Error = err.Error()
		return false, meta
	}
	return true, meta
}

// Healthz runs lightweight dependency checks for /api/v1/system/healthz.
func (s *SystemService) Healthz(ctx context.Context) (*HealthzResponse, bool) {
	meta := map[string]HealthzMeta{}

	dbOK, dbMeta := timedHealthCheck(func() error {
		if dao.DB == nil {
			return fmt.Errorf("database is not initialized")
		}
		sqlDB, err := dao.DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.PingContext(ctx)
	})
	if !dbOK {
		meta["db"] = dbMeta
	}

	redisOK, redisMeta := timedHealthCheck(func() error {
		redisClient := redis.Get()
		if redisClient == nil || !redisClient.Health() {
			return fmt.Errorf("redis is not healthy")
		}
		return nil
	})
	if !redisOK {
		meta["redis"] = redisMeta
	}

	docOK, docMeta := timedHealthCheck(func() error {
		docEngine := engine.Get()
		if docEngine == nil {
			return fmt.Errorf("document engine is not initialized")
		}
		return docEngine.Ping(ctx)
	})
	if !docOK {
		meta["doc_engine"] = docMeta
	}

	storageOK, storageMeta := timedHealthCheck(func() error {
		store := storage.GetStorageFactory().GetStorage()
		if store == nil || !store.Health() {
			return fmt.Errorf("storage is not healthy")
		}
		return nil
	})
	if !storageOK {
		meta["storage"] = storageMeta
	}

	allOK := dbOK && redisOK && docOK && storageOK
	result := &HealthzResponse{
		DB:        okNok(dbOK),
		Redis:     okNok(redisOK),
		DocEngine: okNok(docOK),
		Storage:   okNok(storageOK),
		Status:    okNok(allOK),
	}
	if len(meta) > 0 {
		result.Meta = meta
	}
	return result, allOK
}
