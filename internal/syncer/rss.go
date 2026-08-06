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

package syncer

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"ragflow/internal/utility"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// maxRSSFeedSize caps a single feed document held in memory.
const maxRSSFeedSize = 64 * 1024 * 1024

// rssDocument is one feed entry materialized as an importable document,
// mirroring the Python Document model produced by RSSConnector.
type rssDocument struct {
	ID                 string
	SemanticIdentifier string
	Blob               []byte
	UpdatedAt          time.Time
	Metadata           map[string]interface{}
}

// feedEntry is the normalized union of an RSS <item> and an Atom <entry>.
type feedEntry struct {
	ID         string
	Link       string
	Title      string
	Updated    string
	Published  string
	Content    string
	Summary    string
	Author     string
	Categories []string
}

type rssXMLItem struct {
	GUID           string   `xml:"guid"`
	Link           string   `xml:"link"`
	Title          string   `xml:"title"`
	PubDate        string   `xml:"pubDate"`
	DCDate         string   `xml:"date"`
	Description    string   `xml:"description"`
	ContentEncoded string   `xml:"encoded"`
	Author         string   `xml:"author"`
	Categories     []string `xml:"category"`
}

type atomXMLLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

type atomXMLCategory struct {
	Term string `xml:"term,attr"`
}

type atomXMLEntry struct {
	ID        string        `xml:"id"`
	Title     string        `xml:"title"`
	Links     []atomXMLLink `xml:"link"`
	Updated   string        `xml:"updated"`
	Published string        `xml:"published"`
	Summary   string        `xml:"summary"`
	Content   string        `xml:"content"`
	Author    struct {
		Name string `xml:"name"`
	} `xml:"author"`
	Categories []atomXMLCategory `xml:"category"`
}

type feedXML struct {
	XMLName xml.Name
	Channel struct {
		Items []rssXMLItem `xml:"item"`
	} `xml:"channel"`
	Entries []atomXMLEntry `xml:"entry"`
}

// rssConnector fetches and parses an RSS/Atom feed, mirroring the Python
// RSSConnector (common/data_source/rss_connector.py).
type rssConnector struct {
	feedURL   string
	batchSize int
}

func newRSSConnector(feedURL string, batchSize int) *rssConnector {
	if batchSize < 1 {
		batchSize = 1
	}
	return &rssConnector{feedURL: strings.TrimSpace(feedURL), batchSize: batchSize}
}

// load returns the feed entries whose entry time falls inside (start, end]
// (nil bounds are open), batched by batchSize. It mirrors the Python
// load_from_state (nil bounds) and poll_source (bounded) paths.
func (c *rssConnector) load(ctx context.Context, start, end *time.Time) ([][]rssDocument, error) {
	entries, err := c.readFeed(ctx)
	if err != nil {
		return nil, err
	}

	var batches [][]rssDocument
	batch := make([]rssDocument, 0, c.batchSize)
	for _, entry := range entries {
		updatedAt := resolveEntryTime(entry)
		if start != nil && !updatedAt.After(*start) {
			continue
		}
		if end != nil && updatedAt.After(*end) {
			continue
		}
		batch = append(batch, c.buildDocument(entry, updatedAt))
		if len(batch) >= c.batchSize {
			batches = append(batches, batch)
			batch = make([]rssDocument, 0, c.batchSize)
		}
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}
	return batches, nil
}

// listEntryIDs returns the deterministic connector-level document ID of every
// entry in the feed. Used by prune to compute the retain set.
func (c *rssConnector) listEntryIDs(ctx context.Context) ([]string, error) {
	entries, err := c.readFeed(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, c.buildDocumentID(entry))
	}
	return ids, nil
}

func (c *rssConnector) readFeed(ctx context.Context) ([]feedEntry, error) {
	if c.feedURL == "" {
		return nil, fmt.Errorf("feed_url is required")
	}

	body, _, _, err := utility.FetchRemoteFileSafely(ctx, c.feedURL, maxRSSFeedSize)
	if err != nil {
		return nil, err
	}

	var parsed feedXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed: %w", err)
	}

	entries := make([]feedEntry, 0, len(parsed.Channel.Items)+len(parsed.Entries))
	for _, item := range parsed.Channel.Items {
		entries = append(entries, feedEntry{
			ID:         strings.TrimSpace(item.GUID),
			Link:       strings.TrimSpace(item.Link),
			Title:      strings.TrimSpace(item.Title),
			Updated:    strings.TrimSpace(item.DCDate),
			Published:  strings.TrimSpace(item.PubDate),
			Content:    item.ContentEncoded,
			Summary:    item.Description,
			Author:     strings.TrimSpace(item.Author),
			Categories: item.Categories,
		})
	}
	for _, atom := range parsed.Entries {
		entry := feedEntry{
			ID:        strings.TrimSpace(atom.ID),
			Title:     strings.TrimSpace(atom.Title),
			Updated:   strings.TrimSpace(atom.Updated),
			Published: strings.TrimSpace(atom.Published),
			Content:   atom.Content,
			Summary:   atom.Summary,
			Author:    strings.TrimSpace(atom.Author.Name),
		}
		for _, link := range atom.Links {
			if link.Href == "" {
				continue
			}
			if entry.Link == "" || link.Rel == "alternate" {
				entry.Link = strings.TrimSpace(link.Href)
			}
		}
		for _, category := range atom.Categories {
			if category.Term != "" {
				entry.Categories = append(entry.Categories, category.Term)
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (c *rssConnector) buildDocument(entry feedEntry, updatedAt time.Time) rssDocument {
	link := strings.TrimSpace(entry.Link)
	title := strings.TrimSpace(entry.Title)
	stableKey := c.resolveStableKey(entry)
	semanticIdentifier := title
	if semanticIdentifier == "" {
		semanticIdentifier = link
	}
	if semanticIdentifier == "" {
		semanticIdentifier = stableKey
	}
	blob := []byte(buildEntryContent(entry, semanticIdentifier))

	metadata := map[string]interface{}{"feed_url": c.feedURL}
	if link != "" {
		metadata["link"] = link
	}
	if entry.Author != "" {
		metadata["author"] = entry.Author
	}
	if len(entry.Categories) > 0 {
		categories := make([]string, 0, len(entry.Categories))
		for _, category := range entry.Categories {
			if trimmed := strings.TrimSpace(category); trimmed != "" {
				categories = append(categories, trimmed)
			}
		}
		if len(categories) > 0 {
			metadata["categories"] = categories
		}
	}

	return rssDocument{
		ID:                 c.buildDocumentID(entry),
		SemanticIdentifier: semanticIdentifier,
		Blob:               blob,
		UpdatedAt:          updatedAt,
		Metadata:           metadata,
	}
}

func (c *rssConnector) buildDocumentID(entry feedEntry) string {
	sum := md5.Sum([]byte(c.resolveStableKey(entry)))
	return "rss:" + hex.EncodeToString(sum[:])
}

func (c *rssConnector) resolveStableKey(entry feedEntry) string {
	for _, candidate := range []string{entry.ID, entry.Link, entry.Title, c.feedURL} {
		if trimmed := strings.TrimSpace(candidate); trimmed != "" {
			return trimmed
		}
	}
	return c.feedURL
}

// buildEntryContent mirrors Python _build_content: the semantic identifier
// followed by the normalized entry content (or summary/description fallback).
func buildEntryContent(entry feedEntry, semanticIdentifier string) string {
	parts := []string{semanticIdentifier}
	if normalized := normalizeHTMLText(entry.Content); normalized != "" {
		parts = append(parts, normalized)
	}
	if len(parts) == 1 {
		fallback := entry.Summary
		if normalized := normalizeHTMLText(fallback); normalized != "" {
			parts = append(parts, normalized)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

// resolveEntryTime mirrors Python _resolve_entry_time: prefer updated, then
// published, falling back to now.
func resolveEntryTime(entry feedEntry) time.Time {
	for _, raw := range []string{entry.Updated, entry.Published} {
		if parsed, ok := parseFeedTime(raw); ok {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}

var feedTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	time.RFC1123Z,
	time.RFC1123,
	time.RFC850,
	time.RFC822Z,
	time.RFC822,
	time.ANSIC,
	"2006-01-02T15:04:05Z0700",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

func parseFeedTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range feedTimeLayouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// normalizeHTMLText extracts plain text from an HTML fragment, mirroring
// bs4 get_text("\n", strip=True). Non-HTML input passes through trimmed.
func normalizeHTMLText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "<") {
		return value
	}
	node, err := html.Parse(strings.NewReader(value))
	if err != nil {
		return value
	}
	var segments []string
	collectHTMLText(node, &segments)
	return strings.Join(segments, "\n")
}

func collectHTMLText(node *html.Node, segments *[]string) {
	if node.Type == html.ElementNode {
		switch strings.ToLower(node.Data) {
		case "script", "style", "noscript":
			return
		}
	}
	if node.Type == html.TextNode {
		if text := strings.TrimSpace(node.Data); text != "" {
			*segments = append(*segments, text)
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectHTMLText(child, segments)
	}
}
