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
	"ragflow/internal/common"
	"ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/server"
	"ragflow/internal/storage"
)

// SystemService system service
type SystemService struct {
	systemSettingsDAO *dao.SystemSettingsDAO
}

// NewSystemService create system service
func NewSystemService() *SystemService {
	return &SystemService{
		systemSettingsDAO: dao.NewSystemSettingsDAO(),
	}
}

// ConfigResponse system configuration response
type ConfigResponse struct {
	EnableRegister       bool `json:"registerEnabled"`
	DisablePasswordLogin bool `json:"disablePasswordLogin"`
}

// GetConfig get system configuration
func (s *SystemService) GetConfig() (*ConfigResponse, error) {
	cfg := server.GetConfig()
	return &ConfigResponse{
		EnableRegister:       cfg.EnableRegister(),
		DisablePasswordLogin: cfg.DisablePasswordLogin(),
	}, nil
}

// VersionResponse version response
type VersionResponse struct {
	Version string `json:"version"`
	Type    string `json:"type"`
}

type HealthzMeta struct {
	Elapsed string `json:"elapsed"`
	Error   string `json:"error,omitempty"`
}

type HealthzResponse struct {
	DB           string                 `json:"db"`
	Redis        string                 `json:"redis"`
	DocEngine    string                 `json:"doc_engine"`
	Storage      string                 `json:"storage"`
	MessageQueue string                 `json:"message_queue"`
	Status       string                 `json:"status"`
	Meta         map[string]HealthzMeta `json:"_meta,omitempty"`
}

// GetVersion get RAGFlow version
func (s *SystemService) GetVersion() (*VersionResponse, error) {
	version := common.GetRAGFlowVersion()
	versionType := common.GetRAGFlowType()
	return &VersionResponse{
		Version: version,
		Type:    versionType,
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
func (s *SystemService) GetStatus(ctx context.Context) (*StatusResponse, error) {
	return &StatusResponse{
		DocEngine:              s.getDocEngineStatus(ctx),
		Storage:                s.getStorageStatus(ctx),
		Database:               s.getDatabaseStatus(ctx),
		Redis:                  s.getRedisStatus(ctx),
		TaskExecutorHeartbeats: s.getTaskExecutorHeartbeats(ctx),
	}, nil
}

func (s *SystemService) getDocEngineStatus(ctx context.Context) ComponentStatus {
	cfg := server.GetConfig()
	docEngineType := ""
	if cfg != nil {
		docEngineType = cfg.DocEngineType()
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

	timeOutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := docEngine.Ping(timeOutCtx); err != nil {
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

func (s *SystemService) getStorageStatus(ctx context.Context) ComponentStatus {
	cfg := server.GetConfig()
	storageType := ""
	if cfg != nil {
		storageType = cfg.StorageEngineType()
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

	timeOutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if !factory.Health(timeOutCtx) {
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

func (s *SystemService) getDatabaseStatus(ctx context.Context) ComponentStatus {
	cfg := server.GetConfig()
	databaseType := ""
	if cfg != nil {
		databaseType = cfg.DatabaseType()
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

	if err = sqlDB.PingContext(ctx); err != nil {
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

func (s *SystemService) getRedisStatus(ctx context.Context) ComponentStatus {
	startedAt := time.Now()
	redisClient := redis.Get()
	if redisClient == nil {
		return ComponentStatus{
			"status":  "red",
			"elapsed": elapsedMilliseconds(startedAt),
			"error":   "redis not initialized",
		}
	}
	if !redisClient.Health(ctx) {
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

func (s *SystemService) getTaskExecutorHeartbeats(ctx context.Context) map[string][]interface{} {
	heartbeatsByExecutor := map[string][]interface{}{}
	redisClient := redis.Get()
	if redisClient == nil {
		return heartbeatsByExecutor
	}

	taskExecutorIDs, err := redisClient.SMembers(ctx, "TASKEXE")
	if err != nil {
		return heartbeatsByExecutor
	}

	now := float64(time.Now().Unix())
	for _, taskExecutorID := range taskExecutorIDs {
		rawHeartbeats, err := redisClient.ZRangeByScore(ctx, taskExecutorID, now-60*30, now)
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

func GetComponentsHealthz(ctx context.Context) (*HealthzResponse, bool) {
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
		if redisClient == nil || !redisClient.Health(ctx) {
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
		if store == nil || !store.Health(ctx) {
			return fmt.Errorf("storage is not healthy")
		}
		return nil
	})
	if !storageOK {
		meta["storage"] = storageMeta
	}

	messageQueueOK, messageQueueMeta := timedHealthCheck(func() error {

		msgQueueEngine := engine.GetMessageQueueEngine()
		if msgQueueEngine == nil {
			return fmt.Errorf("message queue is not initialized")
		}

		status := msgQueueEngine.CheckStatus()

		if msgQueueEngine == nil || status != "CONNECTED" {
			return fmt.Errorf("message queue is not healthy")
		}
		return nil
	})
	if !messageQueueOK {
		meta["message_queue"] = messageQueueMeta
	}

	allOK := dbOK && redisOK && docOK && storageOK && messageQueueOK
	result := &HealthzResponse{
		DB:           okNok(dbOK),
		Redis:        okNok(redisOK),
		DocEngine:    okNok(docOK),
		Storage:      okNok(storageOK),
		MessageQueue: okNok(messageQueueOK),
		Status:       okNok(allOK),
	}
	if len(meta) > 0 {
		result.Meta = meta
	}
	return result, allOK
}

// Healthz runs lightweight dependency checks for /api/v1/system/healthz.
func (s *SystemService) Healthz(ctx context.Context) (*HealthzResponse, bool) {
	return GetComponentsHealthz(ctx)
}

// ListAllVariables list all variables
// Returns all system settings from database
func (s *SystemService) ListAllVariables(ctx context.Context) ([]map[string]interface{}, error) {
	settings, err := s.systemSettingsDAO.GetAll(ctx, dao.DB)
	if err != nil {
		return nil, err
	}

	return common.FormatSystemSettings(settings), nil
}

func (s *SystemService) ShowVariable(ctx context.Context, varName string) ([]map[string]interface{}, error) {
	settings, err := s.systemSettingsDAO.GetByName(ctx, dao.DB, varName)
	if err != nil {
		return nil, err
	}

	if len(settings) == 0 {
		settings, err = s.systemSettingsDAO.GetByNamePrefix(ctx, dao.DB, varName)
		if err != nil {
			return nil, err
		}
		if len(settings) == 0 {
			return nil, fmt.Errorf("can't get setting: %s", varName)
		}
	}
	return common.FormatSystemSettings(settings), nil
}

// SetVariable set variable
// Creates or updates a system setting
// If the setting exists, updates it; otherwise creates a new one
func (s *SystemService) SetVariable(ctx context.Context, varName, varValue string) error {
	settings, err := s.systemSettingsDAO.GetByName(ctx, dao.DB, varName)
	if err != nil {
		return err
	}

	if len(settings) == 1 {
		setting := &settings[0]
		if err = common.ValidateSystemSettingValue(*setting, varValue); err != nil {
			return err
		}
		setting.Value = varValue
		return s.systemSettingsDAO.UpdateByName(ctx, dao.DB, varName, setting)
	} else if len(settings) > 1 {
		return fmt.Errorf("can't update more than 1 setting: %s", varName)
	}

	dataType := common.InferSystemSettingDataType(varName)
	newSetting := &entity.SystemSettings{
		Name:     varName,
		Value:    varValue,
		Source:   "admin",
		DataType: dataType,
	}
	if err = common.ValidateSystemSettingValue(*newSetting, varValue); err != nil {
		return err
	}
	return s.systemSettingsDAO.Create(ctx, dao.DB, newSetting)
}

// Config methods

// ListAllConfigs list all configs
// Returns all service configurations from the config file
func (s *SystemService) ListAllConfigs() ([]map[string]interface{}, error) {
	result, err := server.GetAllConfigs()
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Environment methods

// ListEnvironments list all environments
func (s *SystemService) ListEnvironments() ([]map[string]interface{}, error) {
	result := make([]map[string]interface{}, 0)

	globalConfig := server.GetConfig()

	// DOC_ENGINE
	docEngine := globalConfig.GetEnvDocumentEngineType()
	if docEngine == "" {
		docEngine = "elasticsearch"
	}
	result = append(result, map[string]interface{}{
		"env":   "DOC_ENGINE",
		"value": docEngine,
	})

	// DEFAULT_SUPERUSER_EMAIL
	defaultSuperuserEmail := common.GetEnvSmall(common.EnvDefaultSuperuserEmail)
	if defaultSuperuserEmail == "" {
		defaultSuperuserEmail = "admin@ragflow.io"
	}
	result = append(result, map[string]interface{}{
		"env":   "DEFAULT_SUPERUSER_EMAIL",
		"value": defaultSuperuserEmail,
	})

	// DB_TYPE
	dbType := common.GetEnvSmall(common.EnvDBType)
	if dbType == "" {
		dbType = "mysql"
	}
	result = append(result, map[string]interface{}{
		"env":   "DB_TYPE",
		"value": dbType,
	})

	// DEVICE
	device := common.GetEnvSmall(common.EnvDevice)
	if device == "" {
		device = "cpu"
	}
	result = append(result, map[string]interface{}{
		"env":   "DEVICE",
		"value": device,
	})

	// STORAGE_IMPL
	storageImpl := common.GetEnvSmall(common.EnvStorageImpl)
	if storageImpl == "" {
		storageImpl = "MINIO"
	}
	result = append(result, map[string]interface{}{
		"env":   "STORAGE_IMPL",
		"value": storageImpl,
	})

	return result, nil
}
