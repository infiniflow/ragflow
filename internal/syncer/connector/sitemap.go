//
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
//

package connector

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"ragflow/internal/utility"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"golang.org/x/net/html"
)

const (
	defaultSitemapBatchSize = 10
	defaultSitemapUserAgent = "RAGFlow-SitemapConnector/1.0"
	maxSitemapDepth         = 5
	maxSitemapFetches       = 1000
	maxSitemapRedirects     = 10
	maxSitemapFileSize      = 64 * 1024 * 1024
	sitemapFetchTimeout     = 60 * time.Second
)

// sitemapFetchFunc downloads one URL and returns its body and Content-Type.
type sitemapFetchFunc func(ctx context.Context, rawURL string) ([]byte, string, error)

// sitemapAssertURLSafe is the SSRF guard, indirected so tests can stub DNS resolution.
var sitemapAssertURLSafe = utility.AssertURLSafe

// SitemapConnector ingests the web pages listed in a sitemap.xml.
//
// It supports plain urlset sitemaps and sitemapindex files (recursively, up to
// maxSitemapDepth levels), incremental polling via <lastmod>, HTML to Markdown
// extraction, PDF documents served directly, and optional discovery of PDF
// links inside HTML pages. Sitemaps are a public XML standard (sitemaps.org),
// so there is no vendor SDK: every request is a plain HTTP GET guarded by the
// shared SSRF checks.
type SitemapConnector struct {
	sitemapURL          string
	batchSize           int
	userAgent           string
	urlFilter           *regexp.Regexp
	followPDFLinks      bool
	restrictPDFToDomain bool

	fetch sitemapFetchFunc
}

// NewSitemapConnector creates a sitemap connector from Python-compatible config.
func NewSitemapConnector(config map[string]any) (*SitemapConnector, error) {
	sitemapURL := strings.TrimSpace(stringConfig(config["sitemap_url"]))
	userAgent := strings.TrimSpace(stringConfig(config["user_agent"]))
	if userAgent == "" {
		userAgent = defaultSitemapUserAgent
	}

	var urlFilter *regexp.Regexp
	if pattern := strings.TrimSpace(stringConfig(config["url_filter"])); pattern != "" {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("url_filter is not a valid regex: %w", err)
		}
		urlFilter = compiled
	}

	connector := &SitemapConnector{
		sitemapURL:          sitemapURL,
		batchSize:           configInt(config["batch_size"], defaultSitemapBatchSize),
		userAgent:           userAgent,
		urlFilter:           urlFilter,
		followPDFLinks:      configBoolDefault(config["follow_pdf_links"], false),
		restrictPDFToDomain: configBoolDefault(config["restrict_pdf_to_domain"], true),
	}
	connector.fetch = connector.fetchRemote
	return connector, nil
}

// Validate validates sitemap settings and checks that the sitemap lists at
// least one URL accepted by the filter.
func (c *SitemapConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("sitemap connector is nil")
	}
	if c.sitemapURL == "" {
		return fmt.Errorf("sitemap URL is required")
	}
	if err := validateSitemapURL(c.sitemapURL); err != nil {
		return err
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}

	entries, err := c.listEntries(ctx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if c.urlMatches(entry.loc) {
			return nil
		}
	}
	return fmt.Errorf("sitemap contains no URLs matching the filter")
}

// ValidateConnectorSetting validates sitemap settings from an unsaved config.
func (c *SitemapConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one sitemap sync session.
//
// Page bodies are fetched lazily, batch by batch, inside NextBatch so that a
// large sitemap never has to be materialized in memory. Documents carry a
// content fingerprint, so unchanged pages are skipped by the runner before
// being re-indexed even when the sitemap exposes no usable <lastmod>.
func (c *SitemapConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	entries, err := c.listEntries(ctx)
	if err != nil {
		return nil, err
	}

	incremental := !request.FromBeginning && request.WindowStart != nil
	pending := make([]sitemapEntry, 0, len(entries))
	for _, entry := range entries {
		if incremental {
			// Pages without <lastmod> cannot be attributed to a window; they are
			// only picked up by a full sync, exactly like the Python connector.
			if entry.lastmod.IsZero() {
				continue
			}
			if beforeOrAtWindowStart(entry.lastmod, request.WindowStart) || afterWindowEnd(entry.lastmod, request.WindowEnd) {
				continue
			}
		}
		pending = append(pending, entry)
	}

	session := &sitemapSyncSession{
		connector: c,
		pending:   pending,
		seen:      make(map[string]struct{}, len(entries)),
	}
	for _, entry := range entries {
		session.seen[entry.loc] = struct{}{}
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete sitemap prune snapshot session.
//
// When PDF link discovery is enabled the HTML pages are fetched so that the
// discovered PDFs are part of the retained set; otherwise they would be pruned
// as stale right after being indexed.
func (c *SitemapConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	entries, err := c.listEntries(ctx)
	if err != nil {
		return nil, err
	}

	documents := make([]SlimDocument, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		seen[entry.loc] = struct{}{}
		if !c.urlMatches(entry.loc) {
			continue
		}
		documents = append(documents, SlimDocument{SourceID: sitemapSourceID(entry.loc)})
	}

	if c.followPDFLinks {
		for _, entry := range entries {
			if !c.urlMatches(entry.loc) {
				continue
			}
			body, contentType, err := c.fetch(ctx, entry.loc)
			if err != nil {
				slog.Warn("sitemap prune: failed to fetch page for PDF discovery", "url", entry.loc, "error", err)
				continue
			}
			if isPDFResponse(contentType, body) {
				continue
			}
			for _, pdfURL := range c.discoverPDFLinks(body, entry.loc) {
				if _, exists := seen[pdfURL]; exists {
					continue
				}
				seen[pdfURL] = struct{}{}
				documents = append(documents, SlimDocument{SourceID: sitemapSourceID(pdfURL)})
			}
		}
	}

	return &sitemapPruneSession{documents: documents, batchSize: c.batchSize}, nil
}

// listEntries walks the sitemap (and nested sitemap indexes) and returns the
// deduplicated entries in document order. url_filter is applied by callers.
//
// Every sitemap URL is fetched at most once (a sitemapindex that references
// itself or an ancestor is skipped) and the walk stops after maxSitemapFetches
// sitemap documents, so one connector configuration cannot trigger unbounded
// egress.
func (c *SitemapConnector) listEntries(ctx context.Context) ([]sitemapEntry, error) {
	var (
		entries []sitemapEntry
		seen    = make(map[string]struct{})
		visited = make(map[string]struct{})
		fetched int
	)
	var walk func(sitemapURL string, depth int) error
	walk = func(sitemapURL string, depth int) error {
		if depth > maxSitemapDepth {
			slog.Warn("sitemap: max depth reached, stopping", "url", sitemapURL)
			return nil
		}
		if _, done := visited[sitemapURL]; done {
			slog.Warn("sitemap: nested sitemap already visited, skipping", "url", sitemapURL)
			return nil
		}
		visited[sitemapURL] = struct{}{}
		if fetched >= maxSitemapFetches {
			slog.Warn("sitemap: maximum number of sitemap fetches reached, stopping", "url", sitemapURL, "max", maxSitemapFetches)
			return nil
		}
		if err := validateSitemapURL(sitemapURL); err != nil {
			if depth == 0 {
				return err
			}
			slog.Warn("sitemap: skipping invalid nested sitemap URL", "url", sitemapURL, "error", err)
			return nil
		}
		body, _, err := c.fetch(ctx, sitemapURL)
		if err != nil {
			if depth == 0 {
				return fmt.Errorf("failed to fetch sitemap %s: %w", sitemapURL, err)
			}
			slog.Warn("sitemap: failed to fetch nested sitemap", "url", sitemapURL, "error", err)
			return nil
		}
		fetched++

		document, err := parseSitemapXML(body)
		if err != nil {
			return fmt.Errorf("failed to parse sitemap XML from %s: %w", sitemapURL, err)
		}
		if document.isIndex {
			for _, child := range document.sitemaps {
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		for _, entry := range document.entries {
			if _, exists := seen[entry.loc]; exists {
				continue
			}
			seen[entry.loc] = struct{}{}
			entries = append(entries, entry)
		}
		return nil
	}
	if err := walk(c.sitemapURL, 0); err != nil {
		return nil, err
	}
	if fetched == 0 {
		return nil, fmt.Errorf("failed to fetch sitemap %s", c.sitemapURL)
	}
	return entries, nil
}

// urlMatches reports whether a URL passes the optional url_filter regex.
func (c *SitemapConnector) urlMatches(rawURL string) bool {
	return c.urlFilter == nil || c.urlFilter.MatchString(rawURL)
}

// buildDocument downloads one URL and converts it into a source document.
// It returns nil when the page must be skipped (fetch error or empty content).
func (c *SitemapConnector) buildDocument(ctx context.Context, rawURL string, lastmod time.Time, parentURL string) (*SourceDocument, []string) {
	if err := validateSitemapURL(rawURL); err != nil {
		slog.Warn("sitemap: skipping invalid page URL", "url", rawURL, "error", err)
		return nil, nil
	}
	body, contentType, err := c.fetch(ctx, rawURL)
	if err != nil {
		slog.Warn("sitemap: failed to fetch page", "url", rawURL, "error", err)
		return nil, nil
	}

	updatedAt := lastmod
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	var (
		blob      []byte
		extension string
		pdfLinks  []string
	)
	if isPDFResponse(contentType, body) {
		if len(body) == 0 {
			slog.Debug("sitemap: empty PDF, skipping", "url", rawURL)
			return nil, nil
		}
		blob = body
		extension = ".pdf"
	} else {
		if c.followPDFLinks {
			pdfLinks = c.discoverPDFLinks(body, rawURL)
		}
		text, err := sitemapHTMLToMarkdown(body)
		if err != nil {
			slog.Warn("sitemap: failed to parse page", "url", rawURL, "error", err)
			return nil, pdfLinks
		}
		if strings.TrimSpace(text) == "" {
			slog.Debug("sitemap: empty content, skipping", "url", rawURL)
			return nil, pdfLinks
		}
		blob = []byte(text)
		extension = ".md"
	}

	metadata := map[string]any{
		"sitemap_url": c.sitemapURL,
		"url":         rawURL,
	}
	if parentURL != "" {
		metadata["parent_url"] = parentURL
	}
	return &SourceDocument{
		SourceID:           sitemapSourceID(rawURL),
		SemanticIdentifier: sitemapSemanticIdentifier(rawURL),
		Extension:          extension,
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
		Fingerprint:        contentFingerprint(blob),
	}, pdfLinks
}

// discoverPDFLinks returns the absolute PDF links found in an HTML page that
// pass the optional same-domain restriction.
func (c *SitemapConnector) discoverPDFLinks(body []byte, pageURL string) []string {
	links := extractPDFLinks(body, pageURL)
	if !c.restrictPDFToDomain {
		return links
	}
	sitemapHost := hostOf(c.sitemapURL)
	filtered := links[:0]
	for _, link := range links {
		if hostOf(link) == sitemapHost {
			filtered = append(filtered, link)
		}
	}
	return filtered
}

// fetchRemote downloads rawURL with SSRF protection, manual redirect handling
// that re-validates every hop, the configured User-Agent, and a hard size cap.
func (c *SitemapConnector) fetchRemote(ctx context.Context, rawURL string) ([]byte, string, error) {
	currentURL := rawURL
	for redirects := 0; redirects <= maxSitemapRedirects; redirects++ {
		hostname, resolvedIP, err := sitemapAssertURLSafe(currentURL)
		if err != nil {
			return nil, "", err
		}
		client := utility.PinnedHTTPClient(hostname, resolvedIP, sitemapFetchTimeout)
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, currentURL, nil)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("User-Agent", c.userAgent)
		resp, err := client.Do(req) // `#nosec` G107
		if err != nil {
			return nil, "", fmt.Errorf("failed to fetch URL: %w", err)
		}

		switch resp.StatusCode {
		case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
			location := resp.Header.Get("Location")
			resp.Body.Close()
			if location == "" {
				return nil, "", fmt.Errorf("redirect response missing Location header")
			}
			base, parseErr := url.Parse(currentURL)
			if parseErr != nil {
				return nil, "", parseErr
			}
			next, resolveErr := base.Parse(location)
			if resolveErr != nil {
				return nil, "", resolveErr
			}
			currentURL = next.String()
			continue
		}

		if resp.StatusCode >= 400 {
			resp.Body.Close()
			return nil, "", fmt.Errorf("remote URL returned HTTP %d", resp.StatusCode)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxSitemapFileSize+1))
		contentType := strings.ToLower(resp.Header.Get("Content-Type"))
		resp.Body.Close()
		if readErr != nil {
			return nil, "", fmt.Errorf("failed to read remote content: %w", readErr)
		}
		if int64(len(data)) > maxSitemapFileSize {
			return nil, "", fmt.Errorf("remote file exceeds the maximum allowed size of %d bytes", maxSitemapFileSize)
		}
		return data, contentType, nil
	}
	return nil, "", fmt.Errorf("exceeded %d redirects fetching %s", maxSitemapRedirects, rawURL)
}

// sitemapSyncSession streams sitemap pages and then the PDFs discovered in them.
type sitemapSyncSession struct {
	connector *SitemapConnector

	pending []sitemapEntry // sitemap URLs still to fetch
	index   int

	seen        map[string]struct{}
	pendingPDFs []sitemapPDFLink // discovered PDFs, emitted after the sitemap pass
	pdfIndex    int
}

// sitemapPDFLink is a PDF discovered inside an HTML page.
type sitemapPDFLink struct {
	url       string
	parentURL string
}

// NextBatch fetches and returns the next batch of documents.
func (s *sitemapSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	batchSize := s.connector.batchSize
	documents := make([]SourceDocument, 0, batchSize)

	for s.index < len(s.pending) && len(documents) < batchSize {
		if err := ctx.Err(); err != nil {
			return SyncBatch{}, err
		}
		entry := s.pending[s.index]
		s.index++
		if !s.connector.urlMatches(entry.loc) {
			continue
		}
		doc, pdfLinks := s.connector.buildDocument(ctx, entry.loc, entry.lastmod, "")
		for _, pdfURL := range pdfLinks {
			if _, exists := s.seen[pdfURL]; exists {
				continue
			}
			s.seen[pdfURL] = struct{}{}
			s.pendingPDFs = append(s.pendingPDFs, sitemapPDFLink{url: pdfURL, parentURL: entry.loc})
		}
		if doc != nil {
			documents = append(documents, *doc)
		}
	}

	if len(documents) == 0 {
		for s.pdfIndex < len(s.pendingPDFs) && len(documents) < batchSize {
			if err := ctx.Err(); err != nil {
				return SyncBatch{}, err
			}
			link := s.pendingPDFs[s.pdfIndex]
			s.pdfIndex++
			doc, _ := s.connector.buildDocument(ctx, link.url, time.Time{}, link.parentURL)
			if doc != nil {
				documents = append(documents, *doc)
			}
		}
	}

	if len(documents) == 0 {
		return SyncBatch{}, io.EOF
	}
	return SyncBatch{Documents: documents, Checkpoint: sitemapSyncCheckpoint(documents[len(documents)-1])}, nil
}

// Close closes the sitemap sync session.
func (s *sitemapSyncSession) Close() error {
	return nil
}

// applyResume advances past the last committed sitemap page when retrying a task.
//
// Only sitemap-listed URLs can anchor a partial resume. A checkpoint taken
// during the PDF pass anchors on the PDF itself (see sitemapSyncCheckpoint);
// that anchor is not a sitemap-listed URL, so ErrSyncResumeInvalid is returned
// here and the runner restarts the whole window. Skipping past the parent page
// instead would silently drop any of its PDFs that were not committed yet.
func (s *sitemapSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if sourceID == "" {
		return fmt.Errorf("sitemap sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	for index, entry := range s.pending {
		if sitemapSourceID(entry.loc) == sourceID {
			s.index = index + 1
			return nil
		}
	}
	return fmt.Errorf("sitemap resume anchor %q was not found in the current sitemap: %w", sourceID, ErrSyncResumeInvalid)
}

// sitemapSyncCheckpoint returns a resume point after a committed document.
//
// The anchor is the document's own SourceID, never the parent page. A
// checkpoint taken during the PDF pass therefore anchors on the PDF itself;
// that anchor is not a sitemap-listed URL, so applyResume reports
// ErrSyncResumeInvalid and the runner restarts the window instead of skipping
// the parent page and silently dropping the PDFs that had not been committed
// yet (those PDFs are only known again once the parent page is re-fetched).
func sitemapSyncCheckpoint(doc SourceDocument) *SyncCheckpoint {
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    doc.SourceID,
		SourceID:  doc.SourceID,
		UpdatedAt: &updatedAt,
	}
}

type sitemapPruneSession struct {
	documents []SlimDocument
	batchSize int
	index     int
}

// NextBatch returns the next sitemap prune snapshot batch.
func (s *sitemapPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.index >= len(s.documents) {
		return PruneBatch{}, io.EOF
	}
	end := s.index + s.batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	batch := PruneBatch{Documents: s.documents[s.index:end]}
	s.index = end
	return batch, nil
}

// Close closes the sitemap prune session.
func (s *sitemapPruneSession) Close() error {
	return nil
}

// sitemapEntry is one <url> element of a urlset.
type sitemapEntry struct {
	loc     string
	lastmod time.Time
}

// sitemapDocument is a parsed sitemap or sitemap index.
type sitemapDocument struct {
	isIndex  bool
	sitemaps []string
	entries  []sitemapEntry
}

type sitemapXMLRoot struct {
	XMLName  xml.Name
	Sitemaps []sitemapXMLLoc `xml:"sitemap"`
	URLs     []sitemapXMLURL `xml:"url"`
}

type sitemapXMLLoc struct {
	Loc string `xml:"loc"`
}

type sitemapXMLURL struct {
	Loc     string `xml:"loc"`
	Lastmod string `xml:"lastmod"`
}

// parseSitemapXML parses a urlset or sitemapindex payload.
func parseSitemapXML(data []byte) (*sitemapDocument, error) {
	var root sitemapXMLRoot
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = false
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	document := &sitemapDocument{}
	switch strings.ToLower(root.XMLName.Local) {
	case "sitemapindex":
		document.isIndex = true
		for _, item := range root.Sitemaps {
			if loc := strings.TrimSpace(item.Loc); loc != "" {
				document.sitemaps = append(document.sitemaps, loc)
			}
		}
	case "urlset":
		for _, item := range root.URLs {
			loc := strings.TrimSpace(item.Loc)
			if loc == "" {
				continue
			}
			document.entries = append(document.entries, sitemapEntry{loc: loc, lastmod: parseSitemapLastmod(item.Lastmod)})
		}
	default:
		return nil, fmt.Errorf("unexpected root element <%s>", root.XMLName.Local)
	}
	return document, nil
}

// parseSitemapLastmod parses W3C Datetime values used by <lastmod>.
func parseSitemapLastmod(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04Z07:00",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	slog.Debug("sitemap: unrecognised lastmod format", "value", value)
	return time.Time{}
}

// validateSitemapURL checks scheme/host shape and the SSRF guard.
func validateSitemapURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("URL must be a valid http or https URL: %q", rawURL)
	}
	if _, _, err := sitemapAssertURLSafe(rawURL); err != nil {
		return err
	}
	return nil
}

// isPDFResponse reports whether a response is a PDF by Content-Type or magic bytes.
func isPDFResponse(contentType string, body []byte) bool {
	return strings.Contains(strings.ToLower(contentType), "application/pdf") || utility.BytesLooksLikePDF(body)
}

// sitemapSourceID returns the Python-compatible sitemap document ID.
func sitemapSourceID(rawURL string) string {
	sum := md5.Sum([]byte(rawURL))
	return "sitemap:" + hex.EncodeToString(sum[:])
}

// sitemapSemanticIdentifier strips the scheme so the name is a valid storage key.
func sitemapSemanticIdentifier(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	identifier := parsed.Host + parsed.Path
	if parsed.RawQuery != "" {
		identifier += "?" + parsed.RawQuery
	}
	if parsed.Fragment != "" {
		identifier += "#" + parsed.Fragment
	}
	identifier = strings.Trim(identifier, "/")
	if identifier == "" {
		return rawURL
	}
	return identifier
}

// hostOf returns the lower-cased host of a URL, or "" when unparsable.
func hostOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Host)
}

// sitemapHTMLToMarkdown converts a page to Markdown, dropping navigation chrome.
func sitemapHTMLToMarkdown(body []byte) (string, error) {
	converter := md.NewConverter("", true, &md.Options{EmDelimiter: "*"})
	converter.Remove("script", "style", "noscript", "nav", "header", "footer", "aside", "form", "iframe", "svg")
	out, err := converter.ConvertString(string(body))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// extractPDFLinks returns absolute <a href> links whose path ends in .pdf
// (query strings and fragments are ignored, so "/manual.pdf?download=1" counts).
func extractPDFLinks(body []byte, pageURL string) []string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	var links []string
	tokenizer := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return links
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if token.Data != "a" {
				continue
			}
			for _, attr := range token.Attr {
				if attr.Key != "href" {
					continue
				}
				resolved, err := base.Parse(strings.TrimSpace(attr.Val))
				if err != nil || !strings.HasSuffix(strings.ToLower(resolved.Path), ".pdf") {
					continue
				}
				links = append(links, resolved.String())
			}
		}
	}
}
