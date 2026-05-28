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
	"ragflow/internal/cache"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/server"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"time"
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
		redisClient := cache.Get()
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
