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

package component

import (
	"context"
	"encoding/base64"
	"errors"

	"ragflow/internal/entity"
	"ragflow/internal/parser/media"
)

// MaterializeMediaItem fills m["image"] from m["media_ref"] when the item
// carries a reference and no inline bytes. The item map is the parser/chunker
// wire shape, so the upload pass and vision enhancement share one seam.
func MaterializeMediaItem(ctx context.Context, m map[string]any) error {
	if m == nil {
		return nil
	}
	if img, _ := m["image"].(string); img != "" {
		return nil
	}
	data, ref, err := ResolveMediaRef(ctx, m)
	if err != nil || ref == nil {
		return err
	}
	m["image"] = encodeMediaDataURI(ref.MIME, data)
	return nil
}

// ResolveMediaRef reads the item's run-scoped media reference from the cache.
// It returns (nil, nil, nil) when no reference is present so callers can keep
// their existing inline-image path. A malformed reference, unsupported source
// kind, cache failure, or payload integrity failure is returned to the caller.
func ResolveMediaRef(ctx context.Context, m map[string]any) ([]byte, *entity.MediaRef, error) {
	if m == nil {
		return nil, nil, nil
	}
	ref := entity.MediaRefFromAny(m["media_ref"])
	if ref == nil {
		if mediaRefPresent(m["media_ref"]) {
			return nil, nil, errors.New("media: media_ref is present but has no usable format/kind/digest")
		}
		return nil, nil, nil
	}
	if ref.Kind != entity.MediaKindKV {
		return nil, nil, errors.New("media: only run-cache media references are supported")
	}
	cache := media.CacheFromContext(ctx)
	if cache == nil {
		return nil, nil, errors.New("media: reference present but no run cache")
	}
	data, err := cache.Get(ctx, *ref)
	if err != nil {
		return nil, nil, err
	}
	return data, ref, nil
}

// mediaRefPresent reports whether a media_ref value claims to be a reference.
// A nil pointer, zero value, empty string, or empty map is treated as absent;
// anything else that did not decode into a usable MediaRef is malformed.
func mediaRefPresent(v any) bool {
	switch ref := v.(type) {
	case nil:
		return false
	case *entity.MediaRef:
		return !ref.IsZero()
	case entity.MediaRef:
		return !ref.IsZero()
	case string:
		return ref != ""
	case map[string]any:
		return len(ref) > 0
	default:
		return true
	}
}

// encodeMediaDataURI renders image bytes in the form the chunker expects for
// upload and vision materialization. An empty MIME falls back to
// application/octet-stream; image references always carry a concrete type.
func encodeMediaDataURI(mime string, data []byte) string {
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}
