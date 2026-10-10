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

package dao

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/entity/models"
)

var modelProviderManager *models.ProviderManager
var modelProviderManagerMu sync.Mutex

// GetModelProviderManager returns the process-wide model provider catalog.
// The catalog is parsed from the provider JSON files under conf/models, so it
// has no SQL backend and no connection of its own; InitDB seeds it at startup
// and the fallback below covers callers that run without InitDB.
func GetModelProviderManager() *models.ProviderManager {
	if modelProviderManager != nil {
		return modelProviderManager
	}

	modelProviderManagerMu.Lock()
	defer modelProviderManagerMu.Unlock()
	if modelProviderManager != nil {
		return modelProviderManager
	}
	if existing := models.GetProviderManager(); existing != nil {
		modelProviderManager = existing
		return modelProviderManager
	}
	modelConfigDir, err := findModelConfigDir()
	if err != nil {
		common.Fatal("Failed to locate model providers", zap.Error(err))
	}
	if err = models.InitProviderManager(modelConfigDir); err != nil {
		common.Fatal("Failed to load model providers", zap.Error(err))
	}
	modelProviderManager = models.GetProviderManager()
	return modelProviderManager
}

func findModelConfigDir() (string, error) {
	candidates := []string{
		"conf/models",
		filepath.Join("..", "..", "conf", "models"),
		filepath.Join("..", "..", "..", "conf", "models"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("conf/models not found")
}
