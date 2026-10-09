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
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/infinity"
	"ragflow/internal/engine/nats"
	"ragflow/internal/engine/oceanbase"
	"ragflow/internal/engine/serenedb"
	"ragflow/internal/engine/vastbase"
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

		// Configuration errors are permanent; only connection failures are
		// worth waiting out.
		switch engineType {
		case "elasticsearch", "infinity", "oceanbase", "seekdb", "serenedb", "vastbase":
		default:
			initErr = fmt.Errorf("unsupported doc engine type: %s", engineType)
			return
		}

		// The doc engine container boots in parallel with this process, so the
		// first connection attempt usually fails while it is still starting.
		// Wait instead of failing fast: a compose healthcheck gate would
		// serialize the whole stack behind the engine's boot.
		initErr = common.WaitForReady(ctx, "doc engine ("+engineType+")", 3*time.Minute, func(waitCtx context.Context) error {
			var err error
			switch engineType {
			case "elasticsearch":
				globalEngine, err = elasticsearch.NewEngine(waitCtx, globalConfig.GetElasticsearchConfig())
			case "infinity":
				globalEngine, err = infinity.NewEngine(waitCtx, globalConfig.GetInfinityConfig())
			case "oceanbase", "seekdb":
				connectionConfig, resolveErr := globalConfig.ResolveOceanBaseConnection(engineType)
				if resolveErr != nil {
					err = resolveErr
				} else {
					globalEngine, err = oceanbase.NewEngine(engineType, connectionConfig)
				}
			case "serenedb":
				globalEngine, err = serenedb.NewEngine(globalConfig.GetSereneDBConfig())
			case "vastbase":
				globalEngine, err = vastbase.NewEngine(globalConfig.GetVastbaseConfig())
			default:
				err = fmt.Errorf("unsupported doc engine type: %s", engineType)
			}
			return err
		})
		if initErr != nil {
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

func InitMessageQueue(ctx context.Context) error {
	globalConfig := server.GetConfig()
	messageQueueType := globalConfig.QueueEngineType()
	switch messageQueueType {
	case "nats":
		natsConfig := globalConfig.GetNATSConfig()
		messageQueueEngine = nats.NewNatsEngine(
			natsConfig.Host,
			natsConfig.Port,
		)
		// NATS boots in parallel with this process; wait for it instead of
		// failing fast on the first refused connection.
		return common.WaitForReady(ctx, "message queue (nats)", 2*time.Minute, func(waitCtx context.Context) error {
			return messageQueueEngine.Init()
		})
	case "":
		return fmt.Errorf("message queue type is empty")
	default:
		return fmt.Errorf("unsupported message queue type: %s", messageQueueType)
	}
}
