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
	"context"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// InvokeWithGroupFailover is the shared failover orchestration for every model
// invocation chain that resolves a gp- group reference: resolve → invoke, and
// when the invocation fails retryably, flip the currently picked group member
// inactive and retry once on the next member (a fresh resolution).
//
// resolve produces the model for the first attempt — a caller that already
// resolved the model earlier (e.g. because intermediate phases need it) returns
// that instance here. reresolve re-resolves the same reference after the flip;
// pass nil to reuse resolve when resolve always performs a fresh lookup. invoke
// performs the call; any state that must reset between attempts (streaming
// accumulators and the like) is reset inside it. canRetry gates which
// invocation errors qualify for failover (nil means every error does); the
// orchestration itself refuses to retry once ctx is done. Failover only
// applies to gp- references — non-group model IDs never flip and never retry.
//
// On a failed flip, re-resolution, or retry the original invocation error is
// returned so it surfaces exactly as it would without failover; only Warn logs
// mark the outcome. In the community edition MarkGroupMemberInactive reports
// false, so this collapses to a single resolve+invoke attempt.
func InvokeWithGroupFailover[T any](
	ctx context.Context,
	db *gorm.DB,
	tenantID string,
	modelType entity.ModelType,
	modelRef string,
	resolve func(context.Context) (T, error),
	reresolve func(context.Context) (T, error),
	invoke func(context.Context, T) error,
	canRetry func(error) bool,
) error {
	model, err := resolve(ctx)
	if err != nil {
		return err
	}
	invErr := invoke(ctx, model)
	if invErr == nil || ctx.Err() != nil || (canRetry != nil && !canRetry(invErr)) {
		return invErr
	}
	groupID, ok := common.ParseGroupModelRef(modelRef)
	if !ok {
		return invErr
	}
	// A nil db means the caller was assembled without a database handle
	// (unit tests, degraded wiring): failover is best-effort, so skip the
	// flip and surface the original invocation error.
	if db == nil {
		return invErr
	}
	flipped, ferr := MarkGroupMemberInactive(ctx, db, strings.TrimSpace(tenantID), groupID, modelType)
	if ferr != nil {
		common.WarnCtx(ctx, "model group failover: failed to mark member inactive",
			zap.String("model_ref", modelRef), zap.Error(ferr))
		return invErr
	}
	if !flipped {
		return invErr
	}
	if reresolve == nil {
		reresolve = resolve
	}
	retryModel, rerr := reresolve(ctx)
	if rerr != nil {
		common.WarnCtx(ctx, "model group failover: re-resolve failed",
			zap.String("model_ref", modelRef), zap.Error(rerr))
		return invErr
	}
	if retryErr := invoke(ctx, retryModel); retryErr != nil {
		common.WarnCtx(ctx, "model group failover: retry failed",
			zap.String("model_ref", modelRef), zap.Error(retryErr))
		return invErr
	}
	return nil
}
