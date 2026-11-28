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

package storage

import (
	"context"
	"go.uber.org/zap"
	"ragflow/internal/common"
	"time"
)

// MoveWithRollback implements the shared Move contract across backends: copy
// the object, then remove the source. If the source removal fails, the
// destination is removed best-effort under a detached 30s context derived via
// context.WithoutCancel — so cleanup survives a parent canceled during the
// source removal — and the move reports failure.
func MoveWithRollback(ctx context.Context, srcBucket, srcPath, destBucket, destPath string, copyFn func() bool, removeSrc, removeDest func(context.Context) error) bool {
	if !copyFn() {
		return false
	}
	if err := removeSrc(ctx); err != nil {
		common.Warn("Failed to remove source object after copy",
			zap.String("bucket", srcBucket), zap.String("key", srcPath), zap.Error(err))
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if rbErr := removeDest(rollbackCtx); rbErr != nil {
			common.Warn("Failed to roll back copied destination object",
				zap.String("bucket", destBucket), zap.String("key", destPath), zap.Error(rbErr))
		}
		return false
	}
	return true
}
