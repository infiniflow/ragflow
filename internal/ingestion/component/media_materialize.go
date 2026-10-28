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
func MaterializeMediaItem(ctx context.Context, m map[string]any, resolver media.MediaResolver, src media.MediaSource) error {
	if m == nil || resolver == nil || src == nil {
		return nil
	}
	if img, _ := m["image"].(string); img != "" {
		return nil
	}
	ref := entity.MediaRefFromAny(m["media_ref"])
	if ref == nil {
		if mediaRefPresent(m["media_ref"]) {
			return errors.New("media: media_ref is present but has no usable format/kind/digest")
		}
		return nil
	}
	data, err := resolver.Resolve(ctx, *ref, src)
	if err != nil {
		return err
	}
	m["image"] = encodeMediaDataURI(ref.MIME, data)
	return nil
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
