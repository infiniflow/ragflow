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
	"context"
	"fmt"
	"sync"

	"ragflow/internal/common"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/infinity"
	"ragflow/internal/engine/nats"
	"ragflow/internal/engine/oceanbase"
	"ragflow/internal/engine/serenedb"
	"ragflow/internal/server"
	"ragflow/internal/tokenizer"

	"go.uber.org/zap"
)

var (
	globalEngine       DocEngine
	engineType         string
	messageQueueEngine MessageQueue
	once               sync.Once
)

// InitDocEngine initializes document engine
func InitDocEngine(ctx context.Context) error {

	var initErr error
	once.Do(func() {
		globalConfig := server.GetConfig()
		engineType = globalConfig.DocEngineType()
		tokenizer.SetEngineType(engineType)
		var err error
		switch engineType {
		case "elasticsearch":
			globalEngine, err = elasticsearch.NewEngine(ctx, globalConfig.GetElasticsearchConfig())
		case "infinity":
			globalEngine, err = infinity.NewEngine(ctx, globalConfig.GetInfinityConfig())
		case "oceanbase", "seekdb":
			connectionConfig, resolveErr := globalConfig.ResolveOceanBaseConnection(engineType)
			if resolveErr != nil {
				err = resolveErr
			} else {
				globalEngine, err = oceanbase.NewEngine(engineType, connectionConfig)
			}
		case "serenedb":
			globalEngine, err = serenedb.NewEngine(globalConfig.GetSereneDBConfig())
		default:
			err = fmt.Errorf("unsupported doc engine type: %s", engineType)
		}

		if err != nil {
			initErr = fmt.Errorf("failed to create doc engine: %w", err)
			return
		}
		common.Info("Doc engine initialized", zap.String("type", engineType))
	})
	return initErr
}

// GetEngineType returns the document engine type
func GetEngineType() string {
	return engineType
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

func GetMessageQueueEngine() MessageQueue {
	return messageQueueEngine
}

// SetMessageQueueEngine installs the global message-queue engine. It exists
// primarily as a test seam so callers can drive Start() without a real server
// config; production code uses InitMessageQueue.
func SetMessageQueueEngine(mq MessageQueue) {
	messageQueueEngine = mq
}

func InitMessageQueue() error {
	globalConfig := server.GetConfig()
	messageQueueType := globalConfig.QueueEngineType()
	switch messageQueueType {
	case "nats":
		natsConfig := globalConfig.GetNATSConfig()
		messageQueueEngine = nats.NewNatsEngine(natsConfig.Host, natsConfig.Port)
		err := messageQueueEngine.Init()
		if err != nil {
			return err
		}
	case "":
		return fmt.Errorf("message queue type is empty")
	default:
		return fmt.Errorf("unsupported message queue type: %s", messageQueueType)
	}
	return nil
}
