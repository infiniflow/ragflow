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
	"net/http"
	"net/http/httptest"
	"ragflow/internal/service/document"
	"ragflow/internal/utility"
	"strings"
	"testing"
	"time"
)

// TestHash128HexParity guards parity with Python api/utils/common.hash128
// (xxhash.xxh128 hexdigest), which backs deterministic connector document IDs.
func TestHash128HexParity(t *testing.T) {
	if got := document.Hash128Hex([]byte("kb:conn:rss:x")); got != "448df3288c2b8f167228e4e459156928" {
		t.Fatalf("Hash128Hex = %s, want 448df3288c2b8f167228e4e459156928", got)
	}
}

const testRSSFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example</title>
    <item>
      <guid>urn:1</guid>
      <link>https://example.com/a</link>
      <title>Alpha</title>
      <pubDate>Wed, 29 Jul 2026 10:00:00 GMT</pubDate>
      <description>&lt;p&gt;Hello &lt;b&gt;world&lt;/b&gt;&lt;/p&gt;</description>
      <author>alice</author>
      <category>news</category>
    </item>
    <item>
      <guid>urn:2</guid>
      <link>https://example.com/b</link>
      <title>Beta</title>
      <pubDate>Wed, 29 Jul 2026 12:00:00 GMT</pubDate>
      <content:encoded xmlns:content="http://purl.org/rss/1.0/modules/content/">&lt;p&gt;Full content&lt;/p&gt;</content:encoded>
    </item>
  </channel>
</rss>`

const testAtomFeed = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example Atom</title>
  <entry>
    <id>tag:example.com,2026:1</id>
    <title>Gamma</title>
    <link rel="alternate" href="https://example.com/c"/>
    <updated>2026-07-29T13:00:00Z</updated>
    <summary>Atom summary</summary>
    <author><name>bob</name></author>
    <category term="tech"/>
  </entry>
</feed>`

func serveFeed(t *testing.T, body string) (*httptest.Server, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(body))
	}))

	origAssert := utility.AssertURLSafe
	origPinned := utility.PinnedHTTPClient
	utility.AssertURLSafe = func(rawURL string) (string, string, error) {
		return "127.0.0.1", "127.0.0.1", nil
	}
	utility.PinnedHTTPClient = func(hostname, resolvedIP string, timeout time.Duration) *http.Client {
		return server.Client()
	}
	return server, func() {
		utility.AssertURLSafe = origAssert
		utility.PinnedHTTPClient = origPinned
		server.Close()
	}
}

func TestRSSLoadFull(t *testing.T) {
	server, cleanup := serveFeed(t, testRSSFeed)
	defer cleanup()

	connector := newRSSConnector(server.URL, 10)
	batches, err := connector.load(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(batches) != 1 || len(batches[0]) != 2 {
		t.Fatalf("expected 1 batch of 2 docs, got %+v", batches)
	}

	first := batches[0][0]
	wantID := "rss:" + md5Hex("urn:1")
	if first.ID != wantID {
		t.Fatalf("doc id = %s, want %s", first.ID, wantID)
	}
	if first.SemanticIdentifier != "Alpha" {
		t.Fatalf("semantic identifier = %q", first.SemanticIdentifier)
	}
	if got := string(first.Blob); got != "Alpha\n\nHello\nworld" {
		t.Fatalf("blob = %q", got)
	}
	if first.Metadata["link"] != "https://example.com/a" || first.Metadata["author"] != "alice" {
		t.Fatalf("metadata = %+v", first.Metadata)
	}
	if categories, ok := first.Metadata["categories"].([]string); !ok || len(categories) != 1 || categories[0] != "news" {
		t.Fatalf("categories = %+v", first.Metadata["categories"])
	}
	wantTime := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	if !first.UpdatedAt.Equal(wantTime) {
		t.Fatalf("updated at = %v, want %v", first.UpdatedAt, wantTime)
	}

	second := batches[0][1]
	if got := string(second.Blob); got != "Beta\n\nFull content" {
		t.Fatalf("second blob = %q", got)
	}
}

func TestRSSLoadPollWindow(t *testing.T) {
	server, cleanup := serveFeed(t, testRSSFeed)
	defer cleanup()

	connector := newRSSConnector(server.URL, 10)
	start := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC) // exactly entry 1's time — excluded (ts <= start)
	end := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)   // exactly entry 2's time — included
	batches, err := connector.load(context.Background(), &start, &end)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(batches) != 1 || len(batches[0]) != 1 {
		t.Fatalf("expected 1 doc in window, got %+v", batches)
	}
	if batches[0][0].SemanticIdentifier != "Beta" {
		t.Fatalf("in-window doc = %q, want Beta", batches[0][0].SemanticIdentifier)
	}

	after := time.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC)
	batches, err = connector.load(context.Background(), &after, nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(batches) != 0 {
		t.Fatalf("expected no docs after %v, got %+v", after, batches)
	}
}

func TestRSSLoadBatching(t *testing.T) {
	server, cleanup := serveFeed(t, testRSSFeed)
	defer cleanup()

	connector := newRSSConnector(server.URL, 1)
	batches, err := connector.load(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches with batch_size=1, got %d", len(batches))
	}
}

func TestAtomLoad(t *testing.T) {
	server, cleanup := serveFeed(t, testAtomFeed)
	defer cleanup()

	connector := newRSSConnector(server.URL, 10)
	batches, err := connector.load(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(batches) != 1 || len(batches[0]) != 1 {
		t.Fatalf("expected 1 atom doc, got %+v", batches)
	}
	doc := batches[0][0]
	if doc.ID != "rss:"+md5Hex("tag:example.com,2026:1") {
		t.Fatalf("doc id = %s", doc.ID)
	}
	if doc.Metadata["link"] != "https://example.com/c" || doc.Metadata["author"] != "bob" {
		t.Fatalf("metadata = %+v", doc.Metadata)
	}
	if got := string(doc.Blob); got != "Gamma\n\nAtom summary" {
		t.Fatalf("blob = %q", got)
	}
	wantTime := time.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC)
	if !doc.UpdatedAt.Equal(wantTime) {
		t.Fatalf("updated at = %v, want %v", doc.UpdatedAt, wantTime)
	}
}

func TestRSSListEntryIDs(t *testing.T) {
	server, cleanup := serveFeed(t, testRSSFeed)
	defer cleanup()

	connector := newRSSConnector(server.URL, 10)
	ids, err := connector.listEntryIDs(context.Background())
	if err != nil {
		t.Fatalf("listEntryIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != "rss:"+md5Hex("urn:1") || ids[1] != "rss:"+md5Hex("urn:2") {
		t.Fatalf("ids = %v", ids)
	}
}

func TestRSSInvalidFeed(t *testing.T) {
	server, cleanup := serveFeed(t, "this is not xml")
	defer cleanup()

	connector := newRSSConnector(server.URL, 10)
	if _, err := connector.load(context.Background(), nil, nil); err == nil || !strings.Contains(err.Error(), "failed to parse RSS feed") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestParseFeedTime(t *testing.T) {
	cases := map[string]time.Time{
		"Wed, 29 Jul 2026 10:00:00 GMT":   time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		"Wed, 29 Jul 2026 10:00:00 +0200": time.Date(2026, 7, 29, 10, 0, 0, 0, time.FixedZone("", 2*3600)),
		"2026-07-29T10:00:00Z":            time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		"2026-07-29T10:00:00+02:00":       time.Date(2026, 7, 29, 10, 0, 0, 0, time.FixedZone("", 2*3600)),
		"2026-07-29":                      time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC),
	}
	for raw, want := range cases {
		parsed, ok := parseFeedTime(raw)
		if !ok {
			t.Fatalf("parseFeedTime(%q) failed", raw)
		}
		if !parsed.Equal(want) {
			t.Fatalf("parseFeedTime(%q) = %v, want %v", raw, parsed, want)
		}
	}
	if _, ok := parseFeedTime("not a date"); ok {
		t.Fatalf("parseFeedTime should reject garbage")
	}
}

func TestNormalizeHTMLText(t *testing.T) {
	cases := map[string]string{
		"":                                   "",
		"  plain text  ":                     "plain text",
		"<p>Hello <b>world</b></p>":          "Hello\nworld",
		"<p>a</p><p>b</p>":                   "a\nb",
		"<script>var x=1;</script><p>ok</p>": "ok",
	}
	for input, want := range cases {
		if got := normalizeHTMLText(input); got != want {
			t.Fatalf("normalizeHTMLText(%q) = %q, want %q", input, got, want)
		}
	}
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
