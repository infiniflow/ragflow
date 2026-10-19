package connector

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"testing"
	"time"
)

// TestRSSConnectorOpenSyncFullAndIncremental verifies full and windowed sync.
func TestRSSConnectorOpenSyncFullAndIncremental(t *testing.T) {
	connector, err := NewRSSConnector(map[string]any{"feed_url": "https://example.com/feed.xml", "batch_size": 1})
	if err != nil {
		t.Fatalf("NewRSSConnector failed: %v", err)
	}
	connector.fetchFeed = staticRSSFeed

	fullSession, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-05T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync full failed: %v", err)
	}
	first, err := fullSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch first failed: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != expectedRSSSourceID("entry-old") {
		t.Fatalf("unexpected first full batch: %+v", first.Documents)
	}
	second, err := fullSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch second failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != expectedRSSSourceID("entry-new") {
		t.Fatalf("unexpected second full batch: %+v", second.Documents)
	}
	if _, err = fullSession.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}

	start := mustTime(t, "2026-01-02T00:00:00Z")
	incrementalSession, err := connector.OpenSync(t.Context(), SyncRequest{WindowStart: &start, WindowEnd: mustTime(t, "2026-01-04T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync incremental failed: %v", err)
	}
	batch, err := incrementalSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch incremental failed: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("incremental batch len = %d, want 1", len(batch.Documents))
	}
	doc := batch.Documents[0]
	if doc.SourceID != expectedRSSSourceID("entry-new") {
		t.Fatalf("incremental source id = %s", doc.SourceID)
	}
	if doc.SemanticIdentifier != "New title" {
		t.Fatalf("semantic identifier = %q", doc.SemanticIdentifier)
	}
	if string(doc.Blob) != "New title\n\nNew body" {
		t.Fatalf("blob = %q", string(doc.Blob))
	}
	if doc.Metadata["link"] != "https://example.com/new" {
		t.Fatalf("link metadata = %v", doc.Metadata["link"])
	}
	if doc.Fingerprint == "" {
		t.Fatalf("fingerprint is empty")
	}
}

// TestRSSConnectorOpenPrune verifies complete slim snapshot generation.
func TestRSSConnectorOpenPrune(t *testing.T) {
	connector, err := NewRSSConnector(map[string]any{"feed_url": "https://example.com/feed.xml", "batch_size": 10})
	if err != nil {
		t.Fatalf("NewRSSConnector failed: %v", err)
	}
	connector.fetchFeed = staticRSSFeed
	session, err := connector.OpenPrune(t.Context(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 3 {
		t.Fatalf("prune snapshot len = %d, want 3", len(batch.Documents))
	}
	if batch.Documents[0].SourceID != expectedRSSSourceID("entry-old") ||
		batch.Documents[1].SourceID != expectedRSSSourceID("entry-new") ||
		batch.Documents[2].SourceID != expectedRSSSourceID("entry-future") {
		t.Fatalf("unexpected prune ids: %+v", batch.Documents)
	}
}

// TestRSSConnectorOpenSyncResumesAfterCheckpoint verifies retry resumes after committed entries.
func TestRSSConnectorOpenSyncResumesAfterCheckpoint(t *testing.T) {
	connector, err := NewRSSConnector(map[string]any{"feed_url": "https://example.com/feed.xml", "batch_size": 2})
	if err != nil {
		t.Fatalf("NewRSSConnector failed: %v", err)
	}
	connector.fetchFeed = staticRSSFeed

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-07T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch first failed: %v", err)
	}
	if len(first.Documents) != 2 {
		t.Fatalf("first batch len = %d, want 2", len(first.Documents))
	}
	if first.Checkpoint == nil || first.Checkpoint.SourceID != expectedRSSSourceID("entry-new") {
		t.Fatalf("first checkpoint = %+v, want entry-new", first.Checkpoint)
	}

	resumed, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-07T00:00:00Z"), Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resume NextBatch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != expectedRSSSourceID("entry-future") {
		t.Fatalf("resume documents = %+v, want entry-future", second.Documents)
	}
	if second.Checkpoint == nil || second.Checkpoint.SourceID != expectedRSSSourceID("entry-future") {
		t.Fatalf("resume checkpoint = %+v, want entry-future", second.Checkpoint)
	}
	if _, err = resumed.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("resume EOF = %v", err)
	}
}

func TestRSSConnectorOpenSyncResumeRejectsMissingCheckpoint(t *testing.T) {
	connector, err := NewRSSConnector(map[string]any{"feed_url": "https://example.com/feed.xml", "batch_size": 2})
	if err != nil {
		t.Fatalf("NewRSSConnector failed: %v", err)
	}
	connector.fetchFeed = staticRSSFeed

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{}})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestRSSConnectorValidateConnectorSetting(t *testing.T) {
	connector, err := NewRSSConnector(map[string]any{"feed_url": "https://example.com/feed.xml"})
	if err != nil {
		t.Fatalf("NewRSSConnector failed: %v", err)
	}
	connector.fetchFeed = staticRSSFeed
	if err := connector.ValidateConnectorSetting(t.Context(), nil); err != nil {
		t.Fatalf("ValidateConnectorSetting failed: %v", err)
	}
}

// TestRSSConnectorReadsAtomTextByType verifies that Atom title, summary and
// content follow their type: html (escaped or CDATA) and xhtml become plain
// text, and text stays literal.
func TestRSSConnectorReadsAtomTextByType(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		want  string
	}{
		{"html content", `<title>Title</title><content type="html">&lt;p&gt;Fish &amp;amp; chips, 2 &amp;lt; 3&lt;/p&gt;&lt;script&gt;track()&lt;/script&gt;</content>`, "Title\n\nFish & chips, 2 < 3"},
		{"html CDATA content", `<title>Title</title><content type="html"><![CDATA[<p>Body</p><style>p { color: red }</style>]]></content>`, "Title\n\nBody"},
		{"html CDATA summary", `<title>Title</title><summary type="html"><![CDATA[<p>Summary only</p>]]></summary>`, "Title\n\nSummary only"},
		{"text/html content", `<title>Title</title><content type="text/html">&lt;p&gt;Body&lt;/p&gt;</content>`, "Title\n\nBody"},
		{"padded upper case html content", `<title>Title</title><content type=" HTML ">&lt;p&gt;Body&lt;/p&gt;</content>`, "Title\n\nBody"},
		{"xhtml content", `<title>Title</title><content type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml"><p>Inline body</p></div></content>`, "Title\n\nInline body"},
		{"XML media content", `<title>Title</title><content type="application/xhtml+xml"><div xmlns="http://www.w3.org/1999/xhtml"><p>Visible XML body</p></div></content>`, "Title\n\nVisible XML body"},
		{"html content with child elements", `<title>Title</title><content type="html"><p>raw</p></content>`, "Title\n\nraw"},
		{"untyped content", `<title>Title</title><content>Use &lt;token&gt; and 2 &lt; 3 &gt; 1</content>`, "Title\n\nUse <token> and 2 < 3 > 1"},
		{"text content", `<title>Title</title><content type="text">Use &lt;token&gt; here</content>`, "Title\n\nUse <token> here"},
		{"text content with entity", `<title>Title</title><content type="text">Write &amp;lt;tag&amp;gt; literally</content>`, "Title\n\nWrite &lt;tag&gt; literally"},
		{"text/plain content", `<title>Title</title><content type="text/plain">Use &lt;token&gt; here</content>`, "Title\n\nUse <token> here"},
		{"text CDATA content", `<title>Title</title><content type="text"><![CDATA[Use <token> here]]></content>`, "Title\n\nUse <token> here"},
		{"text summary", `<title>Title</title><summary>Write &amp;lt;tag&amp;gt; and &lt;token&gt;</summary>`, "Title\n\nWrite &lt;tag&gt; and <token>"},
		{"html title", `<title type="html"><![CDATA[Fish &amp; chips]]></title>`, "Fish & chips"},
		{"xhtml title", `<title type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml">XHTML title</div></title>`, "XHTML title"},
		{"text title", `<title>Use &lt;token&gt; &amp;amp; more</title>`, "Use <token> &amp; more"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector, err := NewRSSConnector(map[string]any{"feed_url": "https://example.com/atom.xml"})
			if err != nil {
				t.Fatalf("NewRSSConnector failed: %v", err)
			}
			connector.fetchFeed = func(ctx context.Context, feedURL string) ([]byte, error) {
				return []byte(`<feed xmlns="http://www.w3.org/2005/Atom"><entry><id>entry</id>` + tt.entry + `</entry></feed>`), nil
			}
			session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
			if err != nil {
				t.Fatalf("OpenSync failed: %v", err)
			}
			batch, err := session.NextBatch(context.Background())
			if err != nil {
				t.Fatalf("NextBatch failed: %v", err)
			}
			if len(batch.Documents) != 1 {
				t.Fatalf("documents = %d, want 1", len(batch.Documents))
			}
			if got := string(batch.Documents[0].Blob); got != tt.want {
				t.Fatalf("blob = %q, want %q", got, tt.want)
			}
		})
	}
}

// staticRSSFeed returns a fixed RSS feed for tests.
func staticRSSFeed(ctx context.Context, feedURL string) ([]byte, error) {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Feed</title>
    <item>
      <guid>entry-old</guid>
      <title>Old title</title>
      <link>https://example.com/old</link>
      <pubDate>Thu, 01 Jan 2026 00:00:00 +0000</pubDate>
      <description><![CDATA[<p>Old body</p>]]></description>
    </item>
    <item>
      <guid>entry-new</guid>
      <title>New title</title>
      <link>https://example.com/new</link>
      <pubDate>Sat, 03 Jan 2026 00:00:00 +0000</pubDate>
      <description><![CDATA[<p>New body</p>]]></description>
      <category>news</category>
    </item>
    <item>
      <guid>entry-future</guid>
      <title>Future title</title>
      <link>https://example.com/future</link>
      <pubDate>Tue, 06 Jan 2026 00:00:00 +0000</pubDate>
      <description><![CDATA[<p>Future body</p>]]></description>
    </item>
  </channel>
</rss>`), nil
}

// expectedRSSSourceID returns Python-compatible RSS source ID.
func expectedRSSSourceID(value string) string {
	sum := md5.Sum([]byte(value))
	return "rss:" + hex.EncodeToString(sum[:])
}

// mustTime parses a test timestamp.
func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}
