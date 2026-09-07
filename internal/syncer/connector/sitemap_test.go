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
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// sitemapFixture is an in-memory site: URL -> (Content-Type, body).
type sitemapFixture map[string][2]string

func (f sitemapFixture) fetch(ctx context.Context, rawURL string) ([]byte, string, error) {
	entry, ok := f[rawURL]
	if !ok {
		return nil, "", fmt.Errorf("not found: %s", rawURL)
	}
	return []byte(entry[1]), entry[0], nil
}

const sitemapTestIndex = `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>https://example.com/sitemap-pages.xml</loc></sitemap>
  <sitemap><loc>https://example.com/sitemap-missing.xml</loc></sitemap>
</sitemapindex>`

const sitemapTestURLSet = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/old</loc><lastmod>2026-01-01</lastmod></url>
  <url><loc>https://example.com/new</loc><lastmod>2026-01-03T10:00:00.123Z</lastmod></url>
  <url><loc>https://example.com/future</loc><lastmod>2026-01-06T00:00:00+00:00</lastmod></url>
  <url><loc>https://example.com/undated</loc></url>
  <url><loc>https://example.com/brochure.pdf</loc><lastmod>2026-01-02</lastmod></url>
  <url><loc>https://example.com/new</loc></url>
</urlset>`

func newSitemapTestConnector(t *testing.T, config map[string]any, fixture sitemapFixture) *SitemapConnector {
	t.Helper()
	restore := sitemapAssertURLSafe
	sitemapAssertURLSafe = func(rawURL string) (string, string, error) { return "example.com", "203.0.113.10", nil }
	t.Cleanup(func() { sitemapAssertURLSafe = restore })

	if _, ok := config["sitemap_url"]; !ok {
		config["sitemap_url"] = "https://example.com/sitemap.xml"
	}
	connector, err := NewSitemapConnector(config)
	if err != nil {
		t.Fatalf("NewSitemapConnector failed: %v", err)
	}
	connector.fetch = fixture.fetch
	return connector
}

func defaultSitemapFixture() sitemapFixture {
	return sitemapFixture{
		"https://example.com/sitemap.xml":       {"application/xml", sitemapTestIndex},
		"https://example.com/sitemap-pages.xml": {"application/xml", sitemapTestURLSet},
		"https://example.com/old":               {"text/html; charset=utf-8", `<html><head><title>Old</title></head><body><nav><a href="/">Home</a></nav><main><h1>Old page</h1><p>Old body</p></main><footer>Footer</footer></body></html>`},
		"https://example.com/new":               {"text/html", `<html><body><main><h1>New page</h1><p>New body with <a href="/docs/guide.pdf">a guide</a> and <a href="https://cdn.other.org/ext.pdf">an external one</a>.</p></main></body></html>`},
		"https://example.com/future":            {"text/html", `<html><body><p>Future body</p></body></html>`},
		"https://example.com/undated":           {"text/html", `<html><body><p>Undated body</p></body></html>`},
		"https://example.com/brochure.pdf":      {"application/pdf", "%PDF-1.4 brochure"},
		"https://example.com/docs/guide.pdf":    {"application/octet-stream", "%PDF-1.7 guide"},
		"https://cdn.other.org/ext.pdf":         {"application/pdf", "%PDF-1.7 external"},
	}
}

func drainSitemapSession(t *testing.T, session SyncSession) ([]SyncBatch, []SourceDocument) {
	t.Helper()
	var (
		batches   []SyncBatch
		documents []SourceDocument
	)
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			return batches, documents
		}
		if err != nil {
			t.Fatalf("NextBatch failed: %v", err)
		}
		batches = append(batches, batch)
		documents = append(documents, batch.Documents...)
	}
}

func sitemapDocIDs(documents []SourceDocument) []string {
	ids := make([]string, 0, len(documents))
	for _, doc := range documents {
		ids = append(ids, doc.Metadata["url"].(string))
	}
	return ids
}

func expectedSitemapSourceID(rawURL string) string {
	sum := md5.Sum([]byte(rawURL))
	return "sitemap:" + hex.EncodeToString(sum[:])
}

// TestSitemapConnectorFullSync verifies index recursion, deduplication,
// HTML to Markdown conversion, PDF detection, and batch checkpoints.
func TestSitemapConnectorFullSync(t *testing.T) {
	connector := newSitemapTestConnector(t, map[string]any{"batch_size": 2}, defaultSitemapFixture())

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, WindowEnd: mustTime(t, "2026-01-05T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batches, documents := drainSitemapSession(t, session)

	want := []string{"https://example.com/old", "https://example.com/new", "https://example.com/future", "https://example.com/undated", "https://example.com/brochure.pdf"}
	if got := sitemapDocIDs(documents); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("documents = %v, want %v", got, want)
	}
	if len(batches) != 3 {
		t.Fatalf("batches = %d, want 3", len(batches))
	}
	if batches[0].Checkpoint == nil || batches[0].Checkpoint.SourceID != expectedSitemapSourceID("https://example.com/new") {
		t.Fatalf("first checkpoint = %+v", batches[0].Checkpoint)
	}

	old := documents[0]
	if old.SourceID != expectedSitemapSourceID("https://example.com/old") {
		t.Fatalf("source id = %s", old.SourceID)
	}
	if old.SemanticIdentifier != "example.com/old" {
		t.Fatalf("semantic identifier = %q", old.SemanticIdentifier)
	}
	if old.Extension != ".md" {
		t.Fatalf("extension = %q, want .md", old.Extension)
	}
	if body := string(old.Blob); !strings.Contains(body, "# Old page") || !strings.Contains(body, "Old body") || strings.Contains(body, "Home") || strings.Contains(body, "Footer") {
		t.Fatalf("markdown body = %q", body)
	}
	if old.UpdatedAt != mustTime(t, "2026-01-01T00:00:00Z") {
		t.Fatalf("updated at = %v", old.UpdatedAt)
	}
	if old.Fingerprint == "" || old.Metadata["sitemap_url"] != "https://example.com/sitemap.xml" {
		t.Fatalf("fingerprint/metadata = %q / %v", old.Fingerprint, old.Metadata)
	}

	pdf := documents[4]
	if pdf.Extension != ".pdf" || string(pdf.Blob) != "%PDF-1.4 brochure" {
		t.Fatalf("pdf document = %+v", pdf)
	}
	if pdf.SemanticIdentifier != "example.com/brochure.pdf" {
		t.Fatalf("pdf semantic identifier = %q", pdf.SemanticIdentifier)
	}
}

// TestSitemapConnectorIncrementalSync verifies <lastmod> window filtering.
func TestSitemapConnectorIncrementalSync(t *testing.T) {
	connector := newSitemapTestConnector(t, map[string]any{}, defaultSitemapFixture())

	start := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{WindowStart: &start, WindowEnd: mustTime(t, "2026-01-04T00:00:00Z")})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	_, documents := drainSitemapSession(t, session)

	want := []string{"https://example.com/new"}
	if got := sitemapDocIDs(documents); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("incremental documents = %v, want %v", got, want)
	}
	if documents[0].UpdatedAt != mustTime(t, "2026-01-03T10:00:00.123Z") {
		t.Fatalf("fractional lastmod not preserved: %v", documents[0].UpdatedAt)
	}
}

// TestSitemapConnectorURLFilter verifies the url_filter regex and its validation.
func TestSitemapConnectorURLFilter(t *testing.T) {
	connector := newSitemapTestConnector(t, map[string]any{"url_filter": `/(old|new)$`}, defaultSitemapFixture())
	if err := connector.Validate(t.Context()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	_, documents := drainSitemapSession(t, session)
	want := []string{"https://example.com/old", "https://example.com/new"}
	if got := sitemapDocIDs(documents); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("filtered documents = %v, want %v", got, want)
	}

	if _, err := NewSitemapConnector(map[string]any{"sitemap_url": "https://example.com/sitemap.xml", "url_filter": "("}); err == nil {
		t.Fatalf("invalid regex accepted")
	}

	noMatch := newSitemapTestConnector(t, map[string]any{"url_filter": `/nothing-here$`}, defaultSitemapFixture())
	if err := noMatch.Validate(t.Context()); err == nil || !strings.Contains(err.Error(), "no URLs matching") {
		t.Fatalf("Validate with non-matching filter = %v", err)
	}
}

// TestSitemapConnectorFollowPDFLinks verifies PDF discovery, domain restriction,
// deduplication, and the parent-anchored checkpoint of the PDF pass.
func TestSitemapConnectorFollowPDFLinks(t *testing.T) {
	restricted := newSitemapTestConnector(t, map[string]any{"follow_pdf_links": true, "url_filter": "/new$"}, defaultSitemapFixture())
	session, err := restricted.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batches, documents := drainSitemapSession(t, session)
	want := []string{"https://example.com/new", "https://example.com/docs/guide.pdf"}
	if got := sitemapDocIDs(documents); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("restricted documents = %v, want %v", got, want)
	}
	guide := documents[1]
	if guide.Extension != ".pdf" || guide.Metadata["parent_url"] != "https://example.com/new" {
		t.Fatalf("discovered pdf = %+v", guide)
	}
	last := batches[len(batches)-1]
	if last.Checkpoint == nil || last.Checkpoint.SourceID != expectedSitemapSourceID("https://example.com/new") {
		t.Fatalf("pdf pass checkpoint = %+v, want parent page anchor", last.Checkpoint)
	}

	open := newSitemapTestConnector(t, map[string]any{"follow_pdf_links": "true", "restrict_pdf_to_domain": false, "url_filter": "/new$"}, defaultSitemapFixture())
	session, err = open.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	_, documents = drainSitemapSession(t, session)
	want = []string{"https://example.com/new", "https://example.com/docs/guide.pdf", "https://cdn.other.org/ext.pdf"}
	if got := sitemapDocIDs(documents); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unrestricted documents = %v, want %v", got, want)
	}
}

// TestSitemapConnectorOpenPrune verifies the slim snapshot, including discovered PDFs.
func TestSitemapConnectorOpenPrune(t *testing.T) {
	connector := newSitemapTestConnector(t, map[string]any{"batch_size": 10}, defaultSitemapFixture())
	session, err := connector.OpenPrune(t.Context(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 5 || batch.Documents[0].SourceID != expectedSitemapSourceID("https://example.com/old") {
		t.Fatalf("prune snapshot = %+v", batch.Documents)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("prune EOF = %v", err)
	}

	withPDFs := newSitemapTestConnector(t, map[string]any{"batch_size": 10, "follow_pdf_links": true}, defaultSitemapFixture())
	session, err = withPDFs.OpenPrune(t.Context(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err = session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 6 || batch.Documents[5].SourceID != expectedSitemapSourceID("https://example.com/docs/guide.pdf") {
		t.Fatalf("prune snapshot with pdf discovery = %+v", batch.Documents)
	}
}

// TestSitemapConnectorResume verifies retry resumes after the committed anchor.
func TestSitemapConnectorResume(t *testing.T) {
	connector := newSitemapTestConnector(t, map[string]any{"batch_size": 2}, defaultSitemapFixture())
	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}

	resumed, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	_, documents := drainSitemapSession(t, resumed)
	want := []string{"https://example.com/future", "https://example.com/undated", "https://example.com/brochure.pdf"}
	if got := sitemapDocIDs(documents); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("resumed documents = %v, want %v", got, want)
	}

	if _, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{}}); !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("empty resume anchor error = %v", err)
	}
	if _, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{SourceID: "sitemap:gone"}}); !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("unknown resume anchor error = %v", err)
	}
}

// TestSitemapConnectorValidate verifies configuration and reachability checks.
func TestSitemapConnectorValidate(t *testing.T) {
	fixture := defaultSitemapFixture()
	connector := newSitemapTestConnector(t, map[string]any{}, fixture)
	if err := connector.ValidateConnectorSetting(t.Context(), nil); err != nil {
		t.Fatalf("ValidateConnectorSetting failed: %v", err)
	}

	cases := map[string]map[string]any{
		"missing url":  {"sitemap_url": ""},
		"ftp scheme":   {"sitemap_url": "ftp://example.com/sitemap.xml"},
		"unreachable":  {"sitemap_url": "https://example.com/missing.xml"},
		"empty urlset": {"sitemap_url": "https://example.com/empty.xml"},
		"not xml":      {"sitemap_url": "https://example.com/not-xml"},
	}
	fixture["https://example.com/empty.xml"] = [2]string{"application/xml", `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"></urlset>`}
	fixture["https://example.com/not-xml"] = [2]string{"text/html", `<html><body>nope</body></html>`}
	for name, config := range cases {
		t.Run(name, func(t *testing.T) {
			connector := newSitemapTestConnector(t, config, fixture)
			if err := connector.Validate(t.Context()); err == nil {
				t.Fatalf("Validate accepted %v", config)
			}
		})
	}
}

// TestSitemapConnectorSkipsBrokenPages verifies fetch failures and empty pages
// are skipped without aborting the sync.
func TestSitemapConnectorSkipsBrokenPages(t *testing.T) {
	fixture := defaultSitemapFixture()
	delete(fixture, "https://example.com/old")
	fixture["https://example.com/new"] = [2]string{"text/html", `<html><body><nav>only chrome</nav></body></html>`}
	connector := newSitemapTestConnector(t, map[string]any{}, fixture)

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	_, documents := drainSitemapSession(t, session)
	want := []string{"https://example.com/future", "https://example.com/undated", "https://example.com/brochure.pdf"}
	if got := sitemapDocIDs(documents); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("documents = %v, want %v", got, want)
	}
}

// TestParseSitemapLastmod verifies the accepted W3C Datetime shapes.
func TestParseSitemapLastmod(t *testing.T) {
	cases := map[string]string{
		"2026-01-02":                      "2026-01-02T00:00:00Z",
		"2026-01-02T03:04:05Z":            "2026-01-02T03:04:05Z",
		"2026-01-02T03:04:05.789Z":        "2026-01-02T03:04:05.789Z",
		"2026-01-02T03:04:05+02:00":       "2026-01-02T01:04:05Z",
		"2026-01-02T03:04:05.123456+0200": "",
		"2026-01-02T03:04":                "2026-01-02T03:04:00Z",
		"yesterday":                       "",
	}
	for input, want := range cases {
		got := parseSitemapLastmod(input)
		if want == "" {
			if !got.IsZero() {
				t.Fatalf("parseSitemapLastmod(%q) = %v, want zero", input, got)
			}
			continue
		}
		if got.IsZero() || got.Format("2006-01-02T15:04:05.999Z07:00") != want {
			t.Fatalf("parseSitemapLastmod(%q) = %v, want %s", input, got, want)
		}
	}
}

// TestSitemapSemanticIdentifier verifies query and fragment are preserved.
func TestSitemapSemanticIdentifier(t *testing.T) {
	cases := map[string]string{
		"https://example.com/":               "example.com",
		"https://example.com/a/b/":           "example.com/a/b",
		"https://example.com/a?page=2#top":   "example.com/a?page=2#top",
		"https://example.com/a?page=2":       "example.com/a?page=2",
		"https://example.com/docs/guide.pdf": "example.com/docs/guide.pdf",
	}
	for input, want := range cases {
		if got := sitemapSemanticIdentifier(input); got != want {
			t.Fatalf("sitemapSemanticIdentifier(%q) = %q, want %q", input, got, want)
		}
	}
}
