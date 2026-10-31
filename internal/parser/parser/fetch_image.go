//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ragflow/internal/parser/media"
	"ragflow/internal/utility"
)

// isRemoteImageSource reports whether src is an HTTP(S) image source.
func isRemoteImageSource(src string) bool {
	src = strings.TrimSpace(src)
	return strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")
}

// remoteImageMIME labels downloaded bytes for the run media cache. A response
// that does not identify as an image keeps an opaque label so the image is
// still cached and uploaded rather than silently dropped.
func remoteImageMIME(data []byte) string {
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		mime = "image/octet-stream"
	}
	return mime
}

// fetchImage fetches a bounded HTTP(S) image payload through the shared
// SSRF-guarded downloader. That downloader validates every hop (private,
// loopback, link-local, CGNAT and metadata addresses are rejected) and pins
// the resolved address for the connection, so redirects cannot reach an
// internal host either.
func fetchImage(ctx context.Context, rawURL string) ([]byte, error) {
	rawURL = strings.TrimSpace(rawURL)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("image fetch: invalid image URL: %w", err)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("image fetch: image URL must not carry credentials")
	}

	data, _, _, err := utility.FetchRemoteFileSafelyWithTimeout(ctx, rawURL, int64(media.ImageMaxBytes()), 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("image fetch: fetch image %s: %w", rawURL, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("image fetch: image %s is empty", rawURL)
	}
	return data, nil
}
