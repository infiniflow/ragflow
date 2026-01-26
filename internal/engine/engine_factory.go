package engine

import (
	"fmt"

	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/infinity"
)

// DocEngineConfig 文档引擎配置
type DocEngineConfig struct {
	Type     EngineType       `mapstructure:"type"`
	ES       interface{}      `mapstructure:"es"`
	Infinity interface{}      `mapstructure:"infinity"`
}

// CreateDocEngine 根据配置创建文档引擎
func CreateDocEngine(cfg *DocEngineConfig) (DocEngine, error) {
	switch cfg.Type {
	case EngineElasticsearch:
		return elasticsearch.NewEngine(cfg.ES)
	case EngineInfinity:
		return infinity.NewEngine(cfg.Infinity)
	default:
		return nil, fmt.Errorf("unsupported doc engine type: %s", cfg.Type)
	}
}
