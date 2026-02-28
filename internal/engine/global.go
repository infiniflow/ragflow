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

package engine

import (
	"fmt"
	"sync"

	"go.uber.org/zap"

	"ragflow/internal/config"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/infinity"
	"ragflow/internal/logger"
)

var (
	globalEngine DocEngine
	once         sync.Once
)

// Init initializes document engine
func Init(cfg *config.DocEngineConfig) error {
	var initErr error
	once.Do(func() {
		var err error
		switch EngineType(cfg.Type) {
		case EngineElasticsearch:
			globalEngine, err = elasticsearch.NewEngine(cfg.ES)
		case EngineInfinity:
			globalEngine, err = infinity.NewEngine(cfg.Infinity)
		default:
			err = fmt.Errorf("unsupported doc engine type: %s", cfg.Type)
		}

		if err != nil {
			initErr = fmt.Errorf("failed to create doc engine: %w", err)
			return
		}
		logger.Info("Doc engine initialized", zap.String("type", string(cfg.Type)))
	})
	return initErr
}

// Get gets global document engine instance
func Get() DocEngine {
	return globalEngine
}

// Close closes document engine
func Close() error {
	if globalEngine != nil {
		return globalEngine.Close()
	}
	return nil
}
