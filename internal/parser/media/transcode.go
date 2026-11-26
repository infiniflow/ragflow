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
	"strings"
	"sync"
)

// ImageTranscoder converts an image payload of one MIME type into another
// (typically a raster format consumable by vision models and image viewers).
type ImageTranscoder func(ctx context.Context, data []byte) (outMIME string, outData []byte, err error)

var (
	imageTranscodersMu sync.RWMutex
	imageTranscoders   = map[string]ImageTranscoder{}
)

// RegisterImageTranscoder installs (or clears when fn is nil) a transcoder for
// mime and returns the previously registered transcoder, if any.
func RegisterImageTranscoder(mime string, fn ImageTranscoder) ImageTranscoder {
	key := normalizeTranscodeMIME(mime)
	if key == "" {
		return nil
	}
	imageTranscodersMu.Lock()
	defer imageTranscodersMu.Unlock()
	prev := imageTranscoders[key]
	if fn == nil {
		delete(imageTranscoders, key)
		return prev
	}
	imageTranscoders[key] = fn
	return prev
}

// TranscodeImage converts data using the transcoder registered for mime, or
// for a metafile signature when mime has no registered transcoder.
// It reports ok=false with a nil error when no transcoder is registered.
func TranscodeImage(ctx context.Context, mime string, data []byte) (outMIME string, outData []byte, ok bool, err error) {
	fn := lookupImageTranscoder(normalizeTranscodeMIME(mime))
	if fn == nil {
		fn = lookupImageTranscoder(MetafileMIME(data))
	}
	if fn == nil {
		return "", nil, false, nil
	}
	outMIME, outData, err = fn(ctx, data)
	if err != nil {
		return "", nil, true, err
	}
	return normalizeTranscodeMIME(outMIME), outData, true, nil
}

func lookupImageTranscoder(mime string) ImageTranscoder {
	imageTranscodersMu.RLock()
	defer imageTranscodersMu.RUnlock()
	return imageTranscoders[mime]
}

func normalizeTranscodeMIME(mime string) string {
	mime = strings.TrimSpace(mime)
	if semi := strings.IndexByte(mime, ';'); semi >= 0 {
		mime = strings.TrimSpace(mime[:semi])
	}
	return strings.ToLower(mime)
}
