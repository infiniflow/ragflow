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

package utility

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	htmlScriptStyleRE = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	htmlTagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	multiSpaceRE      = regexp.MustCompile(`[ \t]+`)
	multiNewlineRE    = regexp.MustCompile(`\n{3,}`)
)

// FetchRemoteFileSafely downloads rawURL with SSRF protection, connect/overall
// timeouts, and a hard size cap that rejects (rather than truncates) oversized
// bodies.
func FetchRemoteFileSafely(rawURL string, maxSize int64) ([]byte, http.Header, string, error) {
	currentURL := rawURL
	for redirects := 0; redirects < 10; redirects++ {
		hostname, resolvedIP, err := AssertURLSafe(currentURL)
		if err != nil {
			return nil, nil, "", err
		}
		client := PinnedHTTPClient(hostname, resolvedIP, 10*time.Second)
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}

		// codeql[go/request-forgery] False positive: the loop above
		resp, err := client.Get(currentURL) // #nosec G107
		if err != nil {
			return nil, nil, "", fmt.Errorf("failed to fetch URL: %w", err)
		}

		if resp.StatusCode == http.StatusMovedPermanently ||
			resp.StatusCode == http.StatusFound ||
			resp.StatusCode == http.StatusSeeOther ||
			resp.StatusCode == http.StatusTemporaryRedirect ||
			resp.StatusCode == http.StatusPermanentRedirect {
			location := resp.Header.Get("Location")
			resp.Body.Close()
			if location == "" {
				return nil, nil, "", fmt.Errorf("redirect response missing Location header")
			}
			baseURL, parseErr := url.Parse(currentURL)
			if parseErr != nil {
				return nil, nil, "", parseErr
			}
			nextURL, resolveErr := baseURL.Parse(location)
			if resolveErr != nil {
				return nil, nil, "", resolveErr
			}
			currentURL = nextURL.String()
			continue
		}

		if resp.StatusCode >= 400 {
			resp.Body.Close()
			return nil, nil, "", fmt.Errorf("remote URL returned HTTP %d", resp.StatusCode)
		}

		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
		resp.Body.Close()
		if readErr != nil {
			return nil, nil, "", fmt.Errorf("failed to read remote content: %w", readErr)
		}
		if int64(len(data)) > maxSize {
			return nil, nil, "", fmt.Errorf("remote file exceeds the maximum allowed size of %d bytes", maxSize)
		}
		return data, resp.Header.Clone(), currentURL, nil
	}
	return nil, nil, "", fmt.Errorf("stopped after too many redirects")
}

// NormalizeUploadInfoContent normalizes an uploaded file's filename, content
// type, and content bytes: detects PDF by magic bytes, converts HTML to
// readable markdown, and fixes the filename extension.
func NormalizeUploadInfoContent(filename, contentType string, data []byte) (string, string, []byte) {
	lowerCT := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if lowerCT == "" {
		lowerCT = http.DetectContentType(data)
	}

	if lowerCT == "application/pdf" || BytesLooksLikePDF(data) {
		if !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
			filename += ".pdf"
		}
		lowerCT = "application/pdf"
	}
	if lowerCT == "text/html" || lowerCT == "application/xhtml+xml" || LooksLikeHTML(data) {
		data = htmlToReadableMarkdown(data)
		if lowerCT == "" {
			lowerCT = "text/html"
		}
	}
	return filename, lowerCT, data
}

// BytesLooksLikePDF reports whether data starts with the PDF magic bytes.
func BytesLooksLikePDF(data []byte) bool {
	return len(data) >= 4 && string(data[:4]) == "%PDF"
}

// LooksLikeHTML reports whether data contains common HTML tag markers.
func LooksLikeHTML(data []byte) bool {
	snippet := strings.ToLower(string(data))
	return strings.Contains(snippet, "<html") || strings.Contains(snippet, "<body") || strings.Contains(snippet, "<div")
}

func htmlToReadableMarkdown(data []byte) []byte {
	text := string(data)
	text = htmlScriptStyleRE.ReplaceAllString(text, " ")
	text = strings.ReplaceAll(text, "<br>", "\n")
	text = strings.ReplaceAll(text, "<br/>", "\n")
	text = strings.ReplaceAll(text, "<br />", "\n")
	text = strings.ReplaceAll(text, "</p>", "\n\n")
	text = strings.ReplaceAll(text, "</div>", "\n")
	text = strings.ReplaceAll(text, "</li>", "\n")
	text = htmlTagRE.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	text = strings.ReplaceAll(text, "\r", "\n")
	text = multiSpaceRE.ReplaceAllString(text, " ")
	text = multiNewlineRE.ReplaceAllString(text, "\n\n")
	text = strings.TrimSpace(text)
	return []byte(text)
}
