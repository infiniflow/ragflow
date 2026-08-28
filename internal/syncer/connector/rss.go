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
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/utility"

	"github.com/zeebo/xxh3"
)

const (
	defaultRSSBatchSize = 32
	maxRSSFeedSize      = 32 * 1024 * 1024
	rssFetchTimeout     = 60 * time.Second
	rssFetchRetryCount  = 3
)

var (
	rssHTMLScriptStyleRE = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	rssHTMLTagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	rssWhitespaceRE      = regexp.MustCompile(`[ \t]+`)
	rssNewlineRE         = regexp.MustCompile(`\n{3,}`)
)

// RSSConnector reads RSS and Atom feeds.
type RSSConnector struct {
	feedURL   string
	batchSize int
	fetchFeed func(ctx context.Context, feedURL string) ([]byte, error)
	entries   []rssEntry
}

// NewRSSConnector creates an RSS connector from Python-compatible config.
func NewRSSConnector(config map[string]any) (*RSSConnector, error) {
	feedURL, _ := config["feed_url"].(string)
	batchSize := configInt(config["batch_size"], defaultRSSBatchSize)
	return &RSSConnector{feedURL: strings.TrimSpace(feedURL), batchSize: batchSize}, nil
}

// Validate validates RSS connector settings and feed readability.
func (c *RSSConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("rss connector is nil")
	}
	if c.feedURL == "" {
		return fmt.Errorf("RSS feed URL is required")
	}

	parsed, err := url.Parse(c.feedURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid RSS feed URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("RSS feed URL must use HTTP or HTTPS")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}

	entries, err := c.loadEntries(ctx)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("RSS feed contains no entries")
	}

	c.entries = entries
	return nil
}

// OpenSync opens one RSS sync session.
func (c *RSSConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	entries, err := c.loadEntries(ctx)
	if err != nil {
		return nil, err
	}

	documents := make([]SourceDocument, 0, len(entries))
	for _, entry := range entries {
		if !request.WindowEnd.IsZero() && entry.updatedAt.After(request.WindowEnd) {
			continue
		}
		if !request.FromBeginning && request.WindowStart != nil { // not reindex
			if !entry.updatedAt.After(*request.WindowStart) {
				continue
			}
		}
		documents = append(documents, entry.toSourceDocument(c.feedURL))
	}
	session := &rssSyncSession{documents: documents, batchSize: c.batchSize}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete RSS prune snapshot session.
func (c *RSSConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	entries, err := c.loadEntries(ctx)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(entries))
	for _, entry := range entries {
		documents = append(documents, SlimDocument{SourceID: entry.sourceID()})
	}
	return &rssPruneSession{documents: documents, batchSize: c.batchSize}, nil
}

// ValidateConnectorSetting validates RSS settings from an unsaved config.
func (c *RSSConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// loadEntries fetches and parses the configured feed.
func (c *RSSConnector) loadEntries(ctx context.Context) ([]rssEntry, error) {
	if c.entries != nil {
		return c.entries, nil
	}
	if c.fetchFeed != nil {
		data, err := c.fetchFeed(ctx, c.feedURL)
		if err != nil {
			return nil, err
		}
		return parseRSSFeed(data)
	}
	data, _, _, err := fetchRSSFeedWithRetry(ctx, c.feedURL)
	if err != nil {
		return nil, err
	}
	entries, err := parseRSSFeed(data)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// fetchRSSFeedWithRetry fetches a feed with bounded retries for transient slowness.
func fetchRSSFeedWithRetry(ctx context.Context, feedURL string) ([]byte, http.Header, string, error) {
	var lastErr error
	for attempt := 1; attempt <= rssFetchRetryCount; attempt++ {
		data, headers, finalURL, err := utility.FetchRemoteFileSafelyWithTimeout(ctx, feedURL, maxRSSFeedSize, rssFetchTimeout)
		if err == nil {
			return data, headers, finalURL, nil
		}
		lastErr = err
		if attempt == rssFetchRetryCount {
			break
		}
		select {
		case <-ctx.Done():
			return nil, nil, "", ctx.Err()
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}
	return nil, nil, "", lastErr
}

type rssSyncSession struct {
	documents []SourceDocument
	batchSize int
	index     int
}

// NextBatch returns the next RSS document batch.
func (s *rssSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if s.index >= len(s.documents) {
		return SyncBatch{}, io.EOF
	}
	end := s.index + s.batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	batchDocuments := s.documents[s.index:end]
	batch := SyncBatch{Documents: batchDocuments, Checkpoint: rssSyncCheckpoint(batchDocuments[len(batchDocuments)-1])}
	s.index = end
	return batch, nil
}

// Close closes the RSS sync session.
func (s *rssSyncSession) Close() error {
	return nil
}

// applyResume advances past the last committed RSS document when retrying a task.
func (s *rssSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if sourceID == "" {
		return fmt.Errorf("rss sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	for index, doc := range s.documents {
		if doc.SourceID == sourceID {
			s.index = index + 1
			return nil
		}
	}
	return fmt.Errorf("rss resume anchor %q was not found in the current feed: %w", sourceID, ErrSyncResumeInvalid)
}

// rssSyncCheckpoint returns a resume point after a committed RSS document.
func rssSyncCheckpoint(doc SourceDocument) *SyncCheckpoint {
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    doc.SourceID,
		SourceID:  doc.SourceID,
		UpdatedAt: &updatedAt,
	}
}

type rssPruneSession struct {
	documents []SlimDocument
	batchSize int
	index     int
}

// NextBatch returns the next RSS prune snapshot batch.
func (s *rssPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
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

// Close closes the RSS prune session.
func (s *rssPruneSession) Close() error {
	return nil
}

type rssEntry struct {
	id         string
	link       string
	title      string
	author     string
	categories []string
	content    string
	summary    string
	updatedAt  time.Time
}

// sourceID returns the RSS source document ID.
func (e rssEntry) sourceID() string {
	stable := firstNonEmpty(e.id, e.link, e.title)
	sum := md5.Sum([]byte(stable))
	return "rss:" + hex.EncodeToString(sum[:])
}

// semanticIdentifier returns the human-readable document name seed.
func (e rssEntry) semanticIdentifier() string {
	return firstNonEmpty(e.title, e.link, e.id, "rss-entry")
}

// toSourceDocument converts one feed entry into the syncer model.
func (e rssEntry) toSourceDocument(feedURL string) SourceDocument {
	body := e.renderText()
	fingerprint := rssFingerprint(body)
	metadata := map[string]any{"feed_url": feedURL}
	if e.link != "" {
		metadata["link"] = e.link
	}
	if e.author != "" {
		metadata["author"] = e.author
	}
	if len(e.categories) > 0 {
		metadata["categories"] = e.categories
	}
	return SourceDocument{
		SourceID:           e.sourceID(),
		SemanticIdentifier: e.semanticIdentifier(),
		Extension:          ".txt",
		Blob:               []byte(body),
		UpdatedAt:          e.updatedAt,
		SizeBytes:          int64(len(body)),
		Metadata:           metadata,
		Fingerprint:        fingerprint,
	}
}

// renderText builds the plain-text RSS document body.
func (e rssEntry) renderText() string {
	parts := []string{e.semanticIdentifier()}
	body := htmlToText(firstNonEmpty(e.content, e.summary))
	if body != "" {
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n\n")
}

type rssRoot struct {
	XMLName xml.Name
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssXMLItem `xml:"item"`
}

type rssXMLItem struct {
	GUID        string        `xml:"guid"`
	Title       string        `xml:"title"`
	Link        string        `xml:"link"`
	Author      string        `xml:"author"`
	Creator     string        `xml:"creator"`
	PubDate     string        `xml:"pubDate"`
	Published   string        `xml:"published"`
	Updated     string        `xml:"updated"`
	Description string        `xml:"description"`
	Summary     string        `xml:"summary"`
	Encoded     string        `xml:"encoded"`
	Categories  []rssCategory `xml:"category"`
}

type rssCategory struct {
	Term  string `xml:"term,attr"`
	Label string `xml:"label,attr"`
	Text  string `xml:",chardata"`
}

type atomFeed struct {
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID         string         `xml:"id"`
	Title      string         `xml:"title"`
	Updated    string         `xml:"updated"`
	Published  string         `xml:"published"`
	Summary    atomText       `xml:"summary"`
	Content    atomText       `xml:"content"`
	Links      []atomLink     `xml:"link"`
	Author     atomPerson     `xml:"author"`
	Categories []atomCategory `xml:"category"`
}

type atomText struct {
	Text string `xml:",innerxml"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomPerson struct {
	Name string `xml:"name"`
}

type atomCategory struct {
	Term  string `xml:"term,attr"`
	Label string `xml:"label,attr"`
}

// parseRSSFeed parses RSS 2.0 and Atom payloads.
func parseRSSFeed(data []byte) ([]rssEntry, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
		}
		if start, ok := token.(xml.StartElement); ok {
			switch strings.ToLower(start.Name.Local) {
			case "rss", "rdf":
				return parseRSSXML(data)
			case "feed":
				return parseAtomXML(data)
			default:
				return nil, fmt.Errorf("failed to parse RSS feed")
			}
		}
	}
}

// parseRSSXML parses an RSS-style feed document.
func parseRSSXML(data []byte) ([]rssEntry, error) {
	var root rssRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
	}
	entries := make([]rssEntry, 0, len(root.Channel.Items))
	for _, item := range root.Channel.Items {
		entry := rssEntry{
			id:         strings.TrimSpace(item.GUID),
			link:       strings.TrimSpace(item.Link),
			title:      strings.TrimSpace(item.Title),
			author:     firstNonEmpty(item.Author, item.Creator),
			categories: rssCategories(item.Categories),
			content:    firstNonEmpty(item.Encoded, item.Description),
			summary:    firstNonEmpty(item.Summary, item.Description),
			updatedAt:  parseFeedTime(firstNonEmpty(item.Updated, item.Published, item.PubDate)),
		}
		entries = append(entries, entry.withDefaults())
	}
	return entries, nil
}

// parseAtomXML parses an Atom feed document.
func parseAtomXML(data []byte) ([]rssEntry, error) {
	var feed atomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
	}
	entries := make([]rssEntry, 0, len(feed.Entries))
	for _, item := range feed.Entries {
		entry := rssEntry{
			id:         strings.TrimSpace(item.ID),
			link:       atomEntryLink(item.Links),
			title:      htmlToText(item.Title),
			author:     strings.TrimSpace(item.Author.Name),
			categories: atomCategories(item.Categories),
			content:    item.Content.Text,
			summary:    item.Summary.Text,
			updatedAt:  parseFeedTime(firstNonEmpty(item.Updated, item.Published)),
		}
		entries = append(entries, entry.withDefaults())
	}
	return entries, nil
}

// withDefaults fills missing stable fields with deterministic fallbacks.
func (e rssEntry) withDefaults() rssEntry {
	return e
}

// rssFingerprint returns a deterministic body fingerprint.
func rssFingerprint(body string) string {
	sum := xxh3.Hash128([]byte(body)).Bytes()
	return hex.EncodeToString(sum[:])
}

// rssCategories returns RSS category labels.
func rssCategories(categories []rssCategory) []string {
	out := make([]string, 0, len(categories))
	for _, category := range categories {
		value := firstNonEmpty(category.Term, category.Label, category.Text)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

// atomCategories returns Atom category labels.
func atomCategories(categories []atomCategory) []string {
	out := make([]string, 0, len(categories))
	for _, category := range categories {
		value := firstNonEmpty(category.Label, category.Term)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

// atomEntryLink returns the preferred Atom link.
func atomEntryLink(links []atomLink) string {
	for _, link := range links {
		if link.Href != "" && (link.Rel == "" || link.Rel == "alternate") {
			return strings.TrimSpace(link.Href)
		}
	}
	if len(links) > 0 {
		return strings.TrimSpace(links[0].Href)
	}
	return ""
}

// htmlToText removes markup and normalizes text spacing.
func htmlToText(value string) string {
	value = rssHTMLScriptStyleRE.ReplaceAllString(value, "")
	value = rssHTMLTagRE.ReplaceAllString(value, "\n")
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(rssWhitespaceRE.ReplaceAllString(line, " "))
	}
	value = strings.Join(lines, "\n")
	value = rssNewlineRE.ReplaceAllString(value, "\n\n")
	return strings.TrimSpace(value)
}

// parseFeedTime parses common RSS and Atom time formats.
func parseFeedTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339Nano,
		time.RFC3339,
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

// firstNonEmpty returns the first non-empty trimmed string.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// configInt reads a positive integer-like config value.
func configInt(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case int64:
		if typed > 0 {
			return int(typed)
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}
