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

package component

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/progress"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

// maybeFetchHyperlinkContents is the Parser post-step for the opt-in
// linked-content ingestion (#21032). The parent result is final; fetched
// pages are parsed as plain HTML (no nested fetching — depth is one by
// construction, mirroring Python's is_root=False) and appended after the
// parent items with source_url/parent_doc_id tags.
//
// Failures never fail the run: every skip is reported as a warning so the
// parent document still indexes.
func maybeFetchHyperlinkContents(ctx context.Context, fileType utility.FileType, filename string, binary []byte, parent []parser.DocItem, setups map[string]schema.ParserSetup, docID string) ([]parser.DocItem, []string) {
	family := resolveParserFamily(fileType)
	if !hyperlinkFamilies[family] {
		return parent, nil
	}
	setup, ok := setups[family]
	if !ok {
		return parent, nil
	}
	param, err := schema.HyperlinkFetchParamFromSetup(setup)
	if err != nil {
		return parent, []string{fmt.Sprintf("hyperlink: invalid %s config (%v); linked content skipped", family, err)}
	}
	if !param.FetchLinks {
		return parent, nil
	}
	if family == "markdown" && !param.CollectMarkdownLinks {
		return parent, nil
	}

	raw, warnings := collectHyperlinkTargets(family, binary, parent)
	urls, truncated := normalizeAndDedupeLinks(raw, param.MaxURLs)
	if truncated {
		warnings = append(warnings, fmt.Sprintf("hyperlink: %d links found, fetching first %d", len(raw), param.MaxURLs))
	}
	if len(urls) == 0 {
		return parent, warnings
	}

	parentID := docID
	if parentID == "" {
		parentID = filename
	}
	client := common.GetSSRFHTTPClient()
	// total counts fetched bytes after the fact, so exhaustion can overshoot
	// by at most one already-fetched page (bounded by MaxBytesPerURL).
	var total int64
	out := parent
	for _, url := range urls {
		if ctx.Err() != nil {
			warnings = append(warnings, "hyperlink: fetch cancelled; remaining links skipped")
			break
		}
		if total >= int64(param.MaxTotalBytes) {
			warnings = append(warnings, "hyperlink: total fetch budget exhausted; remaining links skipped")
			break
		}
		body, finalURL, warn := fetchHyperlinkPage(ctx, client, url, param)
		if warn != "" {
			warnings = append(warnings, warn)
			continue
		}
		total += int64(len(body))
		sub := dispatchParse(ctx, utility.FileTypeHTML, finalURL, body, setups, progress.Window{})
		if sub.Err != nil {
			warnings = append(warnings, fmt.Sprintf("hyperlink: parse %s failed (%v)", finalURL, sub.Err))
			continue
		}
		for _, it := range sub.JSON {
			m := it.ToMap()
			m[hyperlinkSourceURLKey] = finalURL
			m[hyperlinkParentDocKey] = parentID
			out = append(out, parser.DocItemFromMap(m))
		}
	}
	return out, warnings
}

// collectHyperlinkTargets extracts raw link targets for the family:
// document bytes for pdf/docx, parent item text for markdown.
func collectHyperlinkTargets(family string, binary []byte, parent []parser.DocItem) ([]string, []string) {
	switch family {
	case "pdf":
		return extractLinksFromPDF(binary), nil
	case "docx":
		links, err := extractLinksFromDocx(binary)
		if err != nil {
			return nil, []string{fmt.Sprintf("hyperlink: docx link extraction failed (%v)", err)}
		}
		return links, nil
	case "markdown":
		var raw []string
		for _, it := range parent {
			text, _ := it.ToMap()["text"].(string)
			raw = append(raw, extractLinksFromMarkdownText(text)...)
		}
		return raw, nil
	default:
		return nil, nil
	}
}

// fetchHyperlinkPage GETs one page through the guarded SSRF client with a
// per-URL timeout and a hard response cap. It returns the body, the final
// URL after redirects, or a warning (empty body on warning).
func fetchHyperlinkPage(ctx context.Context, client *http.Client, url string, param schema.HyperlinkFetchParam) (body []byte, finalURL string, warning string) {
	urlCtx, cancel := context.WithTimeout(ctx, time.Duration(param.TimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(urlCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Sprintf("hyperlink: skip %s (%v)", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Sprintf("hyperlink: fetch %s failed (%v)", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Sprintf("hyperlink: fetch %s returned status %s", url, resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	if !isFetchableContentType(contentType) {
		return nil, "", fmt.Sprintf("hyperlink: skip %s (content type %q is not a page)", url, contentType)
	}
	capped, err := io.ReadAll(io.LimitReader(resp.Body, int64(param.MaxBytesPerURL)+1))
	if err != nil {
		return nil, "", fmt.Sprintf("hyperlink: read %s failed (%v)", url, err)
	}
	if len(capped) > param.MaxBytesPerURL {
		return nil, "", fmt.Sprintf("hyperlink: skip %s (over %d byte limit)", url, param.MaxBytesPerURL)
	}
	if resp.Request != nil && resp.Request.URL != nil {
		return capped, resp.Request.URL.String(), ""
	}
	return capped, url, ""
}

func isFetchableContentType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "" {
		return true
	}
	return strings.HasPrefix(ct, "text/html") || strings.HasPrefix(ct, "application/xhtml+xml")
}
