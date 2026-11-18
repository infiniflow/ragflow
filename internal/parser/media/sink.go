// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package media

import (
	"context"
	"errors"
	"fmt"

	"ragflow/internal/entity"
)

// Sink admits and caches media sources for one serial parser invocation.
// Source identity and payload extraction belong to the caller; storage scope,
// atomic quotas and lifecycle belong to Cache.
type Sink struct {
	ctx        context.Context
	cache      Cache
	format     string
	budget     *Budget
	refs       map[string]*entity.MediaRef
	limitSkips int
}

// NewSink binds a parser invocation to its injected cache and shared budget.
// Both ctx and budget must be non-nil.
func NewSink(ctx context.Context, format string, budget *Budget) *Sink {
	return &Sink{ctx: ctx, cache: CacheFromContext(ctx), format: format, budget: budget, refs: make(map[string]*entity.MediaRef)}
}

// Resolve checks the declared size before loading a new source. A false load
// result records a source omission; the caller reports its format-specific
// warning. Limits also return nil without an error. Every cache failure other
// than ErrMediaLimitExceeded is returned to the caller.
// Repeated sources reuse both accepted references and recorded omissions.
func (s *Sink) Resolve(source string, declared Size, load func() (mime string, data []byte, ok bool)) (*entity.MediaRef, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	if ref, seen := s.refs[source]; seen {
		return ref, nil
	}
	if s.cache == nil {
		return nil, fmt.Errorf("%s media cache is not configured", s.format)
	}
	s.refs[source] = nil
	reservation, admission := s.budget.Reserve(declared)
	if admission != Admitted {
		return nil, nil
	}
	mime, data, ok := load()
	if !ok || !reservation.Commit(Size(len(data))) {
		return nil, nil
	}
	ref, err := s.cache.Put(s.ctx, s.format, mime, data)
	if errors.Is(err, ErrMediaLimitExceeded) {
		s.limitSkips++
		return nil, nil
	}
	if err != nil {
		delete(s.refs, source) // A cache failure is not a memoized source omission.
		return nil, fmt.Errorf("%s media cache write: %w", s.format, err)
	}
	s.refs[source] = &ref
	return &ref, nil
}

func (s *Sink) Warnings() []string {
	warnings := s.budget.Warnings()
	if s.limitSkips > 0 {
		warnings = append(warnings, fmt.Sprintf("%s parser skipped %d image(s) after the run media cache limit", s.format, s.limitSkips))
	}
	return warnings
}

// Lookup lets parsers avoid revalidating source metadata on repeated references.
func (s *Sink) Lookup(source string) (*entity.MediaRef, bool) {
	ref, seen := s.refs[source]
	return ref, seen
}

// Skip records an unreadable or invalid source whose warning the parser owns.
func (s *Sink) Skip(source string) { s.refs[source] = nil }
