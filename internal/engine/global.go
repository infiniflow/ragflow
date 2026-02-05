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
