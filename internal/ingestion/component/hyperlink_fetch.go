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
	if _, ok := hyperlinkTargetCollectors[family]; !ok {
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

	raw, warnings := collectHyperlinkTargets(family, binary)
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
	// Document-level deadline: per-URL timeouts alone allow maxURLs*timeout
	// worker occupancy (100 minutes at the configuration maxima). The overall
	// budget is derived from the same parameters and capped at five minutes.
	fetchCtx, cancel := context.WithTimeout(ctx, hyperlinkOverallDeadline(param))
	defer cancel()
	// total counts bytes consumed including rejected oversized responses, so
	// a document cannot download far past MaxTotalBytes through the reject
	// path. reading caps at remaining+1, so exhaustion can overshoot by one
	// byte before the loop stops.
	var total int64
	parsedFinal := map[string]bool{}
	out := parent
	for _, url := range urls {
		if fetchCtx.Err() != nil {
			warnings = append(warnings, "hyperlink: fetch cancelled; remaining links skipped")
			break
		}
		remaining := int64(param.MaxTotalBytes) - total
		if remaining <= 0 {
			warnings = append(warnings, "hyperlink: total fetch budget exhausted; remaining links skipped")
			break
		}
		readCap := int64(param.MaxBytesPerURL)
		if remaining < readCap {
			readCap = remaining
		}
		body, finalURL, consumed, warn := fetchHyperlinkPage(fetchCtx, client, url, param.TimeoutSeconds, readCap)
		total += consumed
		if warn != "" {
			warnings = append(warnings, warn)
			continue
		}
		// Distinct links can redirect to the same page; index it once.
		if parsedFinal[finalURL] {
			continue
		}
		parsedFinal[finalURL] = true
		sub := dispatchParse(ctx, utility.FileTypeHTML, finalURL, body, setups, progress.Window{})
		if sub.Err != nil {
			warnings = append(warnings, fmt.Sprintf("hyperlink: parse %s failed (%v)", finalURL, sub.Err))
			continue
		}
		for _, it := range sub.JSON {
			m := it.ToMap()
			// Provenance goes into the declared "extra" chunk column rather
			// than new top-level keys: Infinity drops undeclared columns, so
			// top-level tags would survive on Elasticsearch only.
			extra, _ := m["extra"].(map[string]any)
			if extra == nil {
				extra = map[string]any{}
			}
			extra[hyperlinkSourceURLKey] = finalURL
			extra[hyperlinkParentDocKey] = parentID
			m["extra"] = extra
			out = append(out, parser.DocItemFromMap(m))
		}
	}
	return out, warnings
}

// hyperlinkOverallDeadline bounds one document's linked fetching. It derives
// from the per-URL timeout and URL count so no extra config knob is needed.
func hyperlinkOverallDeadline(param schema.HyperlinkFetchParam) time.Duration {
	total := time.Duration(param.MaxURLs) * time.Duration(param.TimeoutSeconds) * time.Second
	if total > 5*time.Minute {
		total = 5 * time.Minute
	}
	return total
}

// hyperlinkTargetCollectors is the single list of families that support
// linked-content ingestion and the extractor for each. Check and the fetch
// post-step both read it, so a family cannot be accepted without an extractor.
var hyperlinkTargetCollectors = map[string]func([]byte) ([]string, []string){
	"pdf": func(binary []byte) ([]string, []string) {
		return extractLinksFromPDF(binary), nil
	},
	"docx": func(binary []byte) ([]string, []string) {
		links, err := extractLinksFromDocx(binary)
		if err != nil {
			return nil, []string{fmt.Sprintf("hyperlink: docx link extraction failed (%v)", err)}
		}
		return links, nil
	},
	"markdown": func(binary []byte) ([]string, []string) {
		return extractLinksFromMarkdownText(string(binary)), nil
	},
}

// collectHyperlinkTargets extracts raw link targets for the family from the
// source bytes. Markdown uses the raw document rather than the parsed items:
// the markdown parser resolves links to their label, so the URL is already
// gone by the time items exist.
func collectHyperlinkTargets(family string, binary []byte) ([]string, []string) {
	collector, ok := hyperlinkTargetCollectors[family]
	if !ok {
		return nil, nil
	}
	return collector(binary)
}

// fetchHyperlinkPage GETs one page through the guarded SSRF client with a
// per-URL timeout and a hard response cap (readCap, the smaller of the
// per-page and remaining document budgets). It returns the body, the final
// URL after redirects, the bytes consumed (charged to the document budget
// even when the response is rejected), or a warning (empty body on warning).
func fetchHyperlinkPage(ctx context.Context, client *http.Client, url string, timeoutSeconds int, readCap int64) (body []byte, finalURL string, consumed int64, warning string) {
	urlCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(urlCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", 0, fmt.Sprintf("hyperlink: skip %s (%v)", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", 0, fmt.Sprintf("hyperlink: fetch %s failed (%v)", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", 0, fmt.Sprintf("hyperlink: fetch %s returned status %s", url, resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	if !isFetchableContentType(contentType) {
		return nil, "", 0, fmt.Sprintf("hyperlink: skip %s (content type %q is not a page)", url, contentType)
	}
	capped, err := io.ReadAll(io.LimitReader(resp.Body, readCap+1))
	if err != nil {
		return nil, "", int64(len(capped)), fmt.Sprintf("hyperlink: read %s failed (%v)", url, err)
	}
	if int64(len(capped)) > readCap {
		return nil, "", int64(len(capped)), fmt.Sprintf("hyperlink: skip %s (over %d byte limit)", url, readCap)
	}
	if resp.Request != nil && resp.Request.URL != nil {
		return capped, resp.Request.URL.String(), int64(len(capped)), ""
	}
	return capped, url, int64(len(capped)), ""
}

func isFetchableContentType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if ct == "" {
		return true
	}
	return strings.HasPrefix(ct, "text/html") || strings.HasPrefix(ct, "application/xhtml+xml")
}
