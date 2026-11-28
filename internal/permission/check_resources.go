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

package permission

import "context"

// ResourceChecker is the batch-capable permission surface shared by callers
// that authorize a set of resources with one load.
type ResourceChecker interface {
	FilterResources(ctx context.Context, refs []ResourceRef, op Operation) ([]ResourceRef, error)
	CheckResource(ctx context.Context, ref ResourceRef, op Operation) error
}

// CheckResources authorizes every ref with one batch resource load instead of
// one authorization round-trip per resource. When the batch fails or omits a
// ref, only the unanswered refs are re-checked individually, in request order:
// callers normalize the first error into a public code, so a later ref's
// failure must not mask it. Omission is not denial — an omitted ref still gets
// a real single-resource check.
func CheckResources(ctx context.Context, checker ResourceChecker, refs []ResourceRef, op Operation) error {
	if len(refs) == 0 {
		return nil
	}
	accessible, err := checker.FilterResources(ctx, refs, op)
	if err != nil {
		for _, ref := range refs {
			if err := checker.CheckResource(ctx, ref, op); err != nil {
				return err
			}
		}
		return nil
	}
	allowed := make(map[ResourceRef]bool, len(accessible))
	for _, ref := range accessible {
		allowed[ref] = true
	}
	if len(allowed) == len(refs) {
		return nil
	}
	for _, ref := range refs {
		if allowed[ref] {
			continue
		}
		if err := checker.CheckResource(ctx, ref, op); err != nil {
			return err
		}
	}
	return nil
}
