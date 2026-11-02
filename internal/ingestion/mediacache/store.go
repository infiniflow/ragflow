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

// Package mediacache stores decoded parser media for the short-lived,
// run-scoped handoff between Parser, vision enhancement and Chunker. It uses
// an independent one-day TTL and media manifest; persistence success purges
// media through that manifest.
package mediacache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/chunkcache"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/parser/media"
)

const (
	keyNamespace = "mc"
	TTL          = 24 * time.Hour
)

// Store is a run-scoped media cache backed by a chunkcache.Store.
type Store struct {
	backend chunkcache.Store
	scope   string

	mu sync.Mutex
}

// New returns a cache bound to one ingestion run. A nil backend is a valid
// value so callers can surface configured-cache failures only when media is
// actually encountered.
func New(backend chunkcache.Store, scope string) *Store {
	return &Store{
		backend: backend,
		scope:   strings.TrimSpace(scope),
	}
}

// Put validates and stores one decoded image, then records the key on the
// current task manifest. Any write or lifecycle failure is returned.
func (s *Store) Put(ctx context.Context, format, mime string, data []byte) (entity.MediaRef, error) {
	if s == nil || s.backend == nil {
		return entity.MediaRef{}, errors.New("mediacache: cache is not configured")
	}
	if s.scope == "" {
		return entity.MediaRef{}, errors.New("mediacache: run scope is empty")
	}
	if taskID := globals.TaskID(ctx); taskID != "" && taskID != s.scope {
		return entity.MediaRef{}, fmt.Errorf("mediacache: run scope %q does not match task %q", s.scope, taskID)
	}
	if strings.TrimSpace(format) == "" {
		return entity.MediaRef{}, errors.New("mediacache: media format is empty")
	}
	if strings.TrimSpace(mime) == "" {
		return entity.MediaRef{}, errors.New("mediacache: media type is empty")
	}
	if len(data) == 0 || len(data) > media.HTMLImageMaxBytes() {
		return entity.MediaRef{}, fmt.Errorf("mediacache: image size %d is outside the 1..%d-byte limit", len(data), media.HTMLImageMaxBytes())
	}

	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	ref := entity.MediaRef{
		Format: format,
		Kind:   entity.MediaKindKV,
		Scope:  s.scope,
		SHA256: digest,
		MIME:   strings.ToLower(strings.TrimSpace(mime)),
		Size:   int64(len(data)),
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.key(digest)
	if !s.backend.Set(ctx, key, string(data), TTL) {
		return entity.MediaRef{}, fmt.Errorf("mediacache: write image %s", mediaShortDigest(digest))
	}
	if err := s.track(ctx, key); err != nil {
		if !s.backend.Delete(ctx, key) {
			return entity.MediaRef{}, fmt.Errorf("mediacache: track image %s: %w (rollback failed)", mediaShortDigest(digest), err)
		}
		return entity.MediaRef{}, fmt.Errorf("mediacache: track image %s: %w", mediaShortDigest(digest), err)
	}
	return ref, nil
}

// Get resolves a reference and validates its scope, digest and encoded size.
// A cache miss is media.ErrMediaNotFound; backend and corruption failures are
// distinct errors so callers cannot silently turn them into a skipped image.
func (s *Store) Get(ctx context.Context, ref entity.MediaRef) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.backend == nil {
		return nil, errors.New("mediacache: cache is not configured")
	}
	if s.scope == "" {
		return nil, errors.New("mediacache: run scope is empty")
	}
	if taskID := globals.TaskID(ctx); taskID != "" && taskID != s.scope {
		return nil, fmt.Errorf("mediacache: run scope %q does not match task %q", s.scope, taskID)
	}
	if !ref.Valid() || ref.Kind != entity.MediaKindKV {
		return nil, errors.New("mediacache: invalid kvrocks media reference")
	}
	if ref.Scope != s.scope {
		return nil, fmt.Errorf("mediacache: reference scope %q does not match run scope %q", ref.Scope, s.scope)
	}
	if ref.Size > int64(media.HTMLImageMaxBytes()) {
		return nil, fmt.Errorf("mediacache: image %s exceeds the %d-byte limit", mediaShortDigest(ref.SHA256), media.HTMLImageMaxBytes())
	}

	value, err := s.backend.Get(ctx, s.key(ref.SHA256))
	if err != nil {
		return nil, fmt.Errorf("mediacache: read image %s: %w", mediaShortDigest(ref.SHA256), err)
	}
	if value == "" {
		return nil, fmt.Errorf("%w: image %s", media.ErrMediaNotFound, mediaShortDigest(ref.SHA256))
	}
	data, err := media.ValidatePayload([]byte(value), ref)
	if err != nil {
		return nil, fmt.Errorf("mediacache: validate image %s: %w", mediaShortDigest(ref.SHA256), err)
	}
	return data, nil
}

func (s *Store) key(digest string) string {
	return fmt.Sprintf("%s:media:%x:%s", keyNamespace, xxhash.Sum64String(s.scope), digest)
}

func mediaShortDigest(digest string) string {
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12]
}

func manifestKey(scope string) string {
	return fmt.Sprintf("%s:manifest:%x", keyNamespace, xxhash.Sum64String(scope))
}

func (s *Store) track(ctx context.Context, key string) error {
	// Taskless preview runs have no persist-time cleanup and rely on TTL.
	if globals.TaskID(ctx) == "" {
		return nil
	}
	mk := manifestKey(s.scope)
	if !s.backend.SAdd(ctx, mk, key) {
		return fmt.Errorf("mediacache: record manifest %s", mk)
	}
	if !s.backend.Expire(ctx, mk, TTL) {
		s.backend.Delete(ctx, mk)
		return fmt.Errorf("mediacache: expire manifest %s", mk)
	}
	return nil
}

// PurgeTask removes media after durable persistence. Failed runs rely on TTL.
func PurgeTask(ctx context.Context, backend chunkcache.Store, taskID string) error {
	if backend == nil || taskID == "" {
		return nil
	}
	mk := manifestKey(taskID)
	keys, err := backend.SMembers(ctx, mk)
	if err != nil {
		return fmt.Errorf("mediacache: read manifest: %w", err)
	}
	for _, key := range keys {
		if !backend.Delete(ctx, key) {
			return fmt.Errorf("mediacache: delete %s", key)
		}
	}
	if !backend.Delete(ctx, mk) {
		return fmt.Errorf("mediacache: delete manifest %s", mk)
	}
	return nil
}
