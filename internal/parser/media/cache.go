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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"ragflow/internal/entity"
)

// ErrMediaNotFound reports that a valid reference has no cached payload.
// Cache transport, scope, size and digest failures use other errors and must
// be returned to the task rather than treated as a missing image.
var ErrMediaNotFound = errors.New("media: reference not found")

// Cache stores decoded image bytes for materialization by later ingestion
// components. Implementations own key scoping, lifecycle bookkeeping and
// validation of the payload size and digest before returning bytes from Get.
type Cache interface {
	Put(ctx context.Context, format, mime string, data []byte) (entity.MediaRef, error)
	Get(ctx context.Context, ref entity.MediaRef) ([]byte, error)
}

type cacheContextKey struct{}

// WithCache attaches the media cache used by parser-emitted references.
func WithCache(ctx context.Context, cache Cache) context.Context {
	return context.WithValue(ctx, cacheContextKey{}, cache)
}

// CacheFromContext returns the run-scoped media cache, or nil when the caller
// did not attach one. HTML images must fail rather than silently drop when it
// is absent.
func CacheFromContext(ctx context.Context) Cache {
	cache, _ := ctx.Value(cacheContextKey{}).(Cache)
	return cache
}

// ValidatePayload performs the shared cache integrity and size checks.
func ValidatePayload(data []byte, ref entity.MediaRef) ([]byte, error) {
	if len(data) > HTMLImageMaxBytes() {
		return nil, fmt.Errorf("media: image %s exceeds the %d-byte limit", shortMediaDigest(ref.SHA256), HTMLImageMaxBytes())
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != ref.SHA256 {
		return nil, fmt.Errorf("media: image digest mismatch: reference %s, cache %s", shortMediaDigest(ref.SHA256), shortMediaDigest(got))
	}
	if ref.Size > 0 && int64(len(data)) != ref.Size {
		return nil, fmt.Errorf("media: image size mismatch: reference %d bytes, cache %d bytes", ref.Size, len(data))
	}
	return data, nil
}

func shortMediaDigest(digest string) string {
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12]
}
