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
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// InvokeWithGroupFailover is the shared failover orchestration for every model
// invocation chain that resolves a gp- group reference: resolve → invoke, and
// when the invocation fails retryably, flip the currently picked group member
// inactive and retry on the next member (a fresh resolution). The cycle
// repeats: each retry member that also fails retryably is flipped too, until a
// member succeeds, the caller's context ends, an error fails canRetry, or the
// group runs out of members.
//
// resolve produces the model for the first attempt — a caller that already
// resolved the model earlier (e.g. because intermediate phases need it) returns
// that instance here. reresolve re-resolves the same reference after each flip;
// pass nil to reuse resolve when resolve always performs a fresh lookup. invoke
// performs the call; any state that must reset between attempts (streaming
// accumulators and the like) is reset inside it. canRetry gates which
// invocation errors qualify for failover (nil means every error does); the
// orchestration itself refuses to retry once ctx is done. Failover only
// applies to gp- references — non-group model IDs never flip and never retry.
//
// Error semantics: a success returns nil; a first-attempt or retry error that
// fails canRetry (or ends with the context) is returned as-is so it surfaces
// exactly as it would without failover. When no member is left to try — the
// flip fails, re-resolution finds no active member — an exhausted error
// wrapping every failed attempt (errors.Join) is returned, so errors.Is
// matches any of the member failures. Warn logs mark each failed transition.
// In the community edition MarkGroupMemberInactive reports false, so this
// collapses to a single resolve+invoke attempt.
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
	attempts := []error{}
	model, err := resolve(ctx)
	if err != nil {
		return err
	}
	invErr := invoke(ctx, model)
	if invErr == nil {
		return nil
	}
	attempts = append(attempts, invErr)
	if ctx.Err() != nil || (canRetry != nil && !canRetry(invErr)) {
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
	if reresolve == nil {
		reresolve = resolve
	}
	for ctx.Err() == nil {
		flipped, ferr := MarkGroupMemberInactive(ctx, db, strings.TrimSpace(tenantID), groupID, modelType)
		if ferr != nil {
			common.WarnCtx(ctx, "model group failover: failed to mark member inactive",
				zap.String("model_ref", modelRef), zap.Error(ferr))
			return exhaustedGroupError(groupID, attempts)
		}
		if !flipped {
			return exhaustedGroupError(groupID, attempts)
		}
		retryModel, rerr := reresolve(ctx)
		if rerr != nil {
			common.WarnCtx(ctx, "model group failover: re-resolve failed, no member left to try",
				zap.String("model_ref", modelRef), zap.Error(rerr))
			return exhaustedGroupError(groupID, attempts)
		}
		invErr = invoke(ctx, retryModel)
		if invErr == nil {
			return nil
		}
		attempts = append(attempts, invErr)
		if ctx.Err() != nil || (canRetry != nil && !canRetry(invErr)) {
			// The latest member's failure is terminal for this invocation:
			// surface it as-is rather than as an exhaustion.
			return invErr
		}
	}
	// The context ended mid-cascade: the last observed member failure is the
	// meaningful cause (it is what the caller's cancellation interrupted).
	return attempts[len(attempts)-1]
}

// exhaustedGroupError reports that every remaining member of a gp- group was
// tried (or the flip machinery failed) and the invocation still failed. It
// joins all member failures so errors.Is matches any one of them.
func exhaustedGroupError(groupID string, attempts []error) error {
	return fmt.Errorf("model group %s exhausted: %d member invocation(s) failed: %w", groupID, len(attempts), errors.Join(attempts...))
}
