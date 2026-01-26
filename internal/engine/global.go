package engine

import (
	"fmt"
	"log"
	"sync"

	"ragflow/internal/config"
)

var (
	globalEngine DocEngine
	once         sync.Once
)

// Init initializes document engine
func Init(cfg *config.DocEngineConfig) error {
	var initErr error
	once.Do(func() {
		engineCfg := &DocEngineConfig{
			Type:     EngineType(cfg.Type),
			ES:       cfg.ES,
			Infinity: cfg.Infinity,
		}
		var err error
		globalEngine, err = CreateDocEngine(engineCfg)
		if err != nil {
			initErr = fmt.Errorf("failed to create doc engine: %w", err)
			return
		}
		log.Printf("Doc engine initialized: %s", cfg.Type)
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
