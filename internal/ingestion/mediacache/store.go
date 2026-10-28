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
// the same Kvrocks instance, TTL and task manifest as chunkcache; persistence
// success purges both through one manifest.
package mediacache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/cespare/xxhash/v2"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/chunkcache"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/parser/media"
)

const (
	keyNamespace = "mc"
	// MaxRunBytes bounds the decoded image bytes one parser run may retain.
	// This restores the HTML parser's former 64 MiB document budget while
	// keeping every image individually bounded by media.MaxImagePayloadBytes.
	MaxRunBytes = 64 << 20
)

// Store is a run-scoped media cache backed by a chunkcache.Store.
type Store struct {
	backend chunkcache.Store
	scope   string

	mu       sync.Mutex
	maxItems int
	maxBytes int64
	used     int64
	seen     map[string]int64
}

// New returns a cache bound to one ingestion run. A nil backend is a valid
// value so callers can surface configured-cache failures only when media is
// actually encountered.
func New(backend chunkcache.Store, scope string) *Store {
	return &Store{
		backend:  backend,
		scope:    strings.TrimSpace(scope),
		maxBytes: MaxRunBytes,
		maxItems: 256,
		seen:     make(map[string]int64),
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
	if len(data) == 0 || len(data) > media.MaxImagePayloadBytes {
		return entity.MediaRef{}, fmt.Errorf("mediacache: image size %d is outside the 1..%d-byte limit", len(data), media.MaxImagePayloadBytes)
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

	existingSize, exists := s.seen[digest]
	if exists && existingSize != int64(len(data)) {
		return entity.MediaRef{}, fmt.Errorf("mediacache: digest %s changed size from %d to %d", mediaShortDigest(digest), existingSize, len(data))
	}
	if exists {
		return ref, nil
	}
	if len(s.seen) >= s.maxItems {
		return entity.MediaRef{}, errors.New("mediacache: run image limit exceeded")
	}
	if s.used+int64(len(data)) > s.maxBytes {
		return entity.MediaRef{}, fmt.Errorf(
			"mediacache: run cache limit exceeded: %d + %d > %d bytes",
			s.used,
			len(data),
			s.maxBytes,
		)
	}

	key := s.key(digest)
	if !s.backend.Set(ctx, key, string(data), chunkcache.TTL) {
		return entity.MediaRef{}, fmt.Errorf("mediacache: write image %s", mediaShortDigest(digest))
	}
	if err := chunkcache.Track(ctx, s.backend, key); err != nil {
		if !s.backend.Delete(ctx, key) {
			return entity.MediaRef{}, fmt.Errorf("mediacache: track image %s: %w (rollback failed)", mediaShortDigest(digest), err)
		}
		return entity.MediaRef{}, fmt.Errorf("mediacache: track image %s: %w", mediaShortDigest(digest), err)
	}
	s.seen[digest] = int64(len(data))
	s.used += int64(len(data))
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
	if ref.Size > media.MaxImagePayloadBytes {
		return nil, fmt.Errorf("mediacache: image %s exceeds the %d-byte limit", mediaShortDigest(ref.SHA256), media.MaxImagePayloadBytes)
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
