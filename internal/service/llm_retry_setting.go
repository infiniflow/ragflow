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
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// llmMaxRetriesSyncInterval bounds how long a service keeps using an old retry
// count after an administrator changes the setting.
const llmMaxRetriesSyncInterval = 15 * time.Second

// ApplyLLMMaxRetriesSetting sets the process-wide provider-call retry count from
// the llm.max_retries system setting. An unset or empty setting means the
// administrator has not overridden the deployment default, so the configured
// count (llm.max_retries in service_conf.yaml or LLM_MAX_RETRIES) applies.
func ApplyLLMMaxRetriesSetting(ctx context.Context, db *gorm.DB, configured int) error {
	settings, err := dao.NewSystemSettingsDAO().GetByName(ctx, db, common.SettingLLMMaxRetries)
	if err != nil {
		return err
	}

	retries := configured
	if len(settings) == 1 {
		if value := strings.TrimSpace(settings[0].Value); value != "" {
			if retries, err = strconv.Atoi(value); err != nil {
				return fmt.Errorf("invalid %s %q: %w", common.SettingLLMMaxRetries, value, err)
			}
		}
	}
	return common.SetLLMMaxRetries(retries)
}

// SyncLLMMaxRetries applies the llm.max_retries system setting now and then
// again every llmMaxRetriesSyncInterval until ctx ends, so a change saved in the
// admin console reaches every service without a restart. A failed read or an
// unusable stored value keeps the count already in effect.
func SyncLLMMaxRetries(ctx context.Context, db *gorm.DB, configured int) {
	apply := func() {
		if err := ApplyLLMMaxRetriesSetting(ctx, db, configured); err != nil && ctx.Err() == nil {
			common.Warn("Failed to apply the llm.max_retries setting", zap.Error(err))
		}
	}
	apply()

	ticker := time.NewTicker(llmMaxRetriesSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			apply()
		}
	}
}
