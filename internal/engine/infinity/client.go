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

package infinity

import (
	"context"
	"fmt"

	"ragflow/internal/config"
)

// Engine Infinity engine implementation
// Note: Infinity Go SDK is not yet available. This is a placeholder implementation.
type infinityEngine struct {
	config *config.InfinityConfig
}

// NewEngine creates an Infinity engine
// Note: This is a placeholder implementation waiting for official Infinity Go SDK
func NewEngine(cfg interface{}) (*infinityEngine, error) {
	infConfig, ok := cfg.(*config.InfinityConfig)
	if !ok {
		return nil, fmt.Errorf("invalid infinity config type, expected *config.InfinityConfig")
	}

	engine := &infinityEngine{
		config: infConfig,
	}

	return engine, nil
}

// Type returns the engine type
func (e *infinityEngine) Type() string {
	return "infinity"
}

// Ping health check
func (e *infinityEngine) Ping(ctx context.Context) error {
	return fmt.Errorf("infinity engine not implemented: waiting for official Go SDK")
}

// Close closes the connection
func (e *infinityEngine) Close() error {
	return nil
}
