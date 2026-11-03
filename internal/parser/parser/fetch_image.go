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

// normalizeRemoteImageSource reports whether src is an HTTP(S) image source
// and returns it in fetchable form. Scheme matching is case-insensitive, and a
// protocol-relative source resolves against HTTPS, matching how a browser
// loads one from an HTTPS page.
func normalizeRemoteImageSource(src string) (string, bool) {
	src = strings.TrimSpace(src)
	if strings.HasPrefix(src, "//") {
		src = "https:" + src
	}
	parsed, err := url.Parse(src)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return src, true
	}
	return "", false
}

// isRemoteImageSource reports whether src is an HTTP(S) image source.
func isRemoteImageSource(src string) bool {
	_, ok := normalizeRemoteImageSource(src)
	return ok
}

// unsupportedRemoteImageSource reports whether src is an absolute URL that
// names a remote resource over a scheme the parser cannot fetch. Local
// references, data URIs and non-hierarchical schemes such as cid: are not
// remote image sources and stay silent.
func unsupportedRemoteImageSource(src string) bool {
	src = strings.TrimSpace(src)
	parsed, err := url.Parse(src)
	if err != nil || parsed.Scheme == "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "data":
		return false
	}
	// Require an authority so a local path (including a Windows drive path
	// such as C:\images\a.png) is not mistaken for a remote source.
	return len(parsed.Scheme) > 1 && parsed.Host != ""
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
