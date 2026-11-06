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
	"time"

	"ragflow/internal/common"

	"go.uber.org/zap"
)

// retryObjectOp runs op up to attempts times, sleeping one second between
// attempts and invoking onRetry (e.g. a client reconnect) before each sleep.
// It returns nil on the first success and the last error once attempts are
// exhausted. A canceled context short-circuits before an attempt and wins
// over the backoff sleep, so cancellation always surfaces promptly instead
// of costing a sleep window — the contract pinned by s3_cancel_test.go.
func retryObjectOp(ctx context.Context, attempts int, onRetry func(), op func() error) error {
	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := op(); err != nil {
			lastErr = err
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			if onRetry != nil {
				onRetry()
			}
			if serr := sleepOrAbort(ctx, time.Second); serr != nil {
				return serr
			}
			continue
		}
		return nil
	}
	return lastErr
}

// moveWithRollback implements the shared Move contract across backends: copy
// the object, then remove the source. If the source removal fails, the
// destination is removed best-effort under a detached 30s context derived via
// context.WithoutCancel — so cleanup survives a parent canceled during the
// source removal — and the move reports failure.
func moveWithRollback(ctx context.Context, srcBucket, srcPath, destBucket, destPath string, copyFn func() bool, removeSrc, removeDest func(context.Context) error) bool {
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
