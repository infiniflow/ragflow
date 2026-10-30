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

package parser

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/media"
	"ragflow/internal/utility"
)

const markdownMediaDataURI = "data:image/png;base64,aGVsbG8="

func markdownIngestionParser(t *testing.T) ParseResultProducer {
	t.Helper()
	p, err := GetParser(utility.FileTypeMarkdown)
	if err != nil {
		t.Fatalf("GetParser(FileTypeMarkdown): %v", err)
	}
	return p
}

func markdownImageItems(t *testing.T, res ParseResult) []map[string]any {
	t.Helper()
	var images []map[string]any
	for _, item := range ItemsToMaps(res.JSON) {
		if item["doc_type_kwd"] == DocTypeImage {
			images = append(images, item)
		}
	}
	return images
}

func markdownText(res ParseResult) string {
	var texts []string
	for _, item := range ItemsToMaps(res.JSON) {
		if text, ok := item["text"].(string); ok {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

func TestMarkdownParserIngestionStoresBlockDataURI(t *testing.T) {
	ctx, cache := recordingMediaContext()
	res := markdownIngestionParser(t).ParseWithResult(ctx, "doc.md", []byte("Before\n\n![figure]("+markdownMediaDataURI+")\n\nAfter\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(cache.puts) != 1 || cache.puts[0].format != "markdown" || cache.puts[0].mime != "image/png" || string(cache.puts[0].data) != "hello" {
		t.Fatalf("cache puts = %+v", cache.puts)
	}
	images := markdownImageItems(t, res)
	if len(images) != 1 {
		t.Fatalf("image items = %+v, want one", ItemsToMaps(res.JSON))
	}
	item := images[0]
	if _, ok := item["image"]; ok {
		t.Fatalf("image bytes retained on the item: %+v", item)
	}
	ref := entity.MediaRefFromAny(item["media_ref"])
	if ref == nil || ref.Format != "markdown" || ref.Kind != entity.MediaKindKV || ref.MIME != "image/png" || ref.Size != int64(len("hello")) {
		t.Fatalf("media_ref = %+v", ref)
	}
	sum := sha256.Sum256([]byte("hello"))
	if want := hex.EncodeToString(sum[:]); ref.SHA256 != want {
		t.Fatalf("sha256 = %s, want %s", ref.SHA256, want)
	}
	if _, err := cache.Get(ctx, *ref); err != nil {
		t.Fatalf("resolve media_ref: %v", err)
	}
	if texts := markdownText(res); !strings.Contains(texts, "Before") || !strings.Contains(texts, "After") {
		t.Fatalf("surrounding text lost: %q", texts)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", res.Warnings)
	}
}

func TestMarkdownParserIngestionStoresTableDataURI(t *testing.T) {
	ctx, cache := recordingMediaContext()
	md := "| Field | Value |\n|---|---|\n| Logo | ![logo](" + markdownMediaDataURI + ") |\n"
	res := markdownIngestionParser(t).ParseWithResult(ctx, "table.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(cache.puts) != 1 || string(cache.puts[0].data) != "hello" {
		t.Fatalf("cache puts = %+v", cache.puts)
	}
	images := markdownImageItems(t, res)
	if len(images) != 1 {
		t.Fatalf("image items = %+v, want one", ItemsToMaps(res.JSON))
	}
	item := images[0]
	if entity.MediaRefFromAny(item["media_ref"]) == nil {
		t.Fatalf("table image without media_ref: %+v", item)
	}
	if item["parent_table_id"] == nil {
		t.Fatalf("table image lost its parent table: %+v", item)
	}
	if _, ok := item["image"]; ok {
		t.Fatalf("image bytes retained on the item: %+v", item)
	}
}

func TestMarkdownParserIngestionFetchesRemoteImage(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	const body = "\x89PNG\r\n\x1a\nremote-bytes"
	server := imageTestServer(t, body, &hits)
	ctx, cache := recordingMediaContext()

	res := markdownIngestionParser(t).ParseWithResult(ctx, "remote.md", []byte("![remote]("+server.URL+"/image.png)\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetches = %d, want 1", hits.Load())
	}
	if len(cache.puts) != 1 || cache.puts[0].format != "markdown" || cache.puts[0].mime != "image/png" || string(cache.puts[0].data) != body {
		t.Fatalf("cache puts = %+v", cache.puts)
	}
	images := markdownImageItems(t, res)
	if len(images) != 1 {
		t.Fatalf("image items = %+v, want one", ItemsToMaps(res.JSON))
	}
	ref := entity.MediaRefFromAny(images[0]["media_ref"])
	if ref == nil || ref.Format != "markdown" || ref.Size != int64(len(body)) {
		t.Fatalf("media_ref = %+v", ref)
	}
	if _, ok := images[0]["image"]; ok {
		t.Fatalf("image bytes retained on the item: %+v", images[0])
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", res.Warnings)
	}
}

func TestMarkdownParserIngestionSharesRepeatedSources(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	server := imageTestServer(t, "\x89PNG\r\n\x1a\nshared", &hits)
	ctx, cache := recordingMediaContext()

	md := "![a](" + markdownMediaDataURI + ")\n\n![b](" + markdownMediaDataURI + ")\n\n" +
		"![c](" + server.URL + "/same.png)\n\n![d](" + server.URL + "/same.png)\n"
	res := markdownIngestionParser(t).ParseWithResult(ctx, "repeat.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetches = %d, want 1 for a repeated URL", hits.Load())
	}
	if len(cache.puts) != 2 {
		t.Fatalf("cache puts = %d, want one data URI and one remote source", len(cache.puts))
	}
	images := markdownImageItems(t, res)
	if len(images) != 4 {
		t.Fatalf("image items = %d, want 4", len(images))
	}
	ref := entity.MediaRefFromAny(images[0]["media_ref"])
	if ref == nil {
		t.Fatalf("first image without media_ref: %+v", images[0])
	}
	for i, item := range images[1:] {
		other := entity.MediaRefFromAny(item["media_ref"])
		if other == nil {
			t.Fatalf("image %d without media_ref: %+v", i+1, item)
		}
	}
}

func TestMarkdownParserIngestionSkipsFailedRemoteDownload(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	ctx, cache := recordingMediaContext()

	res := markdownIngestionParser(t).ParseWithResult(ctx, "missing.md", []byte("Before\n\n![gone]("+server.URL+"/gone.png)\n\nAfter\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetches = %d, want 1", hits.Load())
	}
	if len(cache.puts) != 0 {
		t.Fatalf("cache puts = %+v, want none", cache.puts)
	}
	if images := markdownImageItems(t, res); len(images) != 0 {
		t.Fatalf("image items = %+v, want none", images)
	}
	texts := markdownText(res)
	if !strings.Contains(texts, "Before") || !strings.Contains(texts, "After") || !strings.Contains(texts, "gone") {
		t.Fatalf("text item lost the failed image or its neighbours: %q", texts)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "remote image(s) that could not be downloaded") {
		t.Fatalf("missing remote download warning: %v", res.Warnings)
	}
}

func TestMarkdownParserIngestionRejectsInternalAddress(t *testing.T) {
	ctx, cache := recordingMediaContext()
	for _, source := range []string{"http://10.0.0.1/image.png", "http://127.0.0.1/image.png"} {
		res := markdownIngestionParser(t).ParseWithResult(ctx, "ssrf.md", []byte("![blocked]("+source+")\n"))
		if res.Err != nil {
			t.Fatalf("ParseWithResult(%s): %v", source, res.Err)
		}
		if len(cache.puts) != 0 {
			t.Fatalf("cache puts = %+v, want none", cache.puts)
		}
		if !strings.Contains(strings.Join(res.Warnings, "\n"), "could not be downloaded") {
			t.Fatalf("missing SSRF rejection warning for %s: %v", source, res.Warnings)
		}
	}
}

func TestMarkdownParserIngestionRejectsRedirectToPrivateAddress(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "http://10.0.0.1/image.png", http.StatusFound)
	}))
	defer server.Close()
	ctx, cache := recordingMediaContext()

	res := markdownIngestionParser(t).ParseWithResult(ctx, "redirect.md", []byte("![redirect]("+server.URL+"/redirect.png)\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("first-hop fetches = %d, want 1", hits.Load())
	}
	if len(cache.puts) != 0 {
		t.Fatalf("cache puts = %+v, want none", cache.puts)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "could not be downloaded") {
		t.Fatalf("missing redirect rejection warning: %v", res.Warnings)
	}
}

func TestMarkdownParserIngestionSkipsInvalidDataURI(t *testing.T) {
	ctx, cache := recordingMediaContext()
	res := markdownIngestionParser(t).ParseWithResult(ctx, "bad.md", []byte("Before\n\n![broken](data:image/png;base64,!!!)\n\nAfter\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(cache.puts) != 0 {
		t.Fatalf("cache puts = %+v, want none", cache.puts)
	}
	if images := markdownImageItems(t, res); len(images) != 0 {
		t.Fatalf("image items = %+v, want none", images)
	}
	texts := markdownText(res)
	if !strings.Contains(texts, "Before") || !strings.Contains(texts, "After") || !strings.Contains(texts, "broken") {
		t.Fatalf("text item lost the failed image or its neighbours: %q", texts)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "unresolved image reference") {
		t.Fatalf("missing invalid-data-URI warning: %v", res.Warnings)
	}
}

func TestMarkdownParserIngestionNeverReadsLocalPaths(t *testing.T) {
	ctx, cache := recordingMediaContext()
	res := markdownIngestionParser(t).ParseWithResult(ctx, "local.md", []byte("![figure](images/fig-1.png)\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(cache.puts) != 0 {
		t.Fatalf("cache puts = %+v, want none", cache.puts)
	}
	if images := markdownImageItems(t, res); len(images) != 0 {
		t.Fatalf("image items = %+v, want none", images)
	}
	if texts := markdownText(res); !strings.Contains(texts, "![figure](images/fig-1.png)") {
		t.Fatalf("local image reference lost from text: %q", texts)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "unresolved image reference") {
		t.Fatalf("missing unresolved-source warning: %v", res.Warnings)
	}
}

func TestMarkdownParserIngestionFailsWhenCacheWriteFails(t *testing.T) {
	writeErr := errors.New("kvrocks unavailable")
	cache := &recordingMediaCache{putErr: writeErr}
	ctx := media.WithCache(context.Background(), cache)

	res := markdownIngestionParser(t).ParseWithResult(ctx, "failed.md", []byte("![figure]("+markdownMediaDataURI+")\n"))
	if !errors.Is(res.Err, writeErr) {
		t.Fatalf("ParseWithResult error = %v, want %v", res.Err, writeErr)
	}
}

func TestMarkdownParserIngestionFailsWithoutMediaCache(t *testing.T) {
	res := markdownIngestionParser(t).ParseWithResult(context.Background(), "nocache.md", []byte("![figure]("+markdownMediaDataURI+")\n"))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "media cache is not configured") {
		t.Fatalf("ParseWithResult error = %v, want a missing media cache error", res.Err)
	}
}

func TestMarkdownParserIngestionSkipsImagesAfterRunCacheLimit(t *testing.T) {
	cache := &recordingMediaCache{putErr: fmt.Errorf("mediacache: run image count limit 1 exceeded: %w", media.ErrMediaLimitExceeded)}
	ctx := media.WithCache(context.Background(), cache)

	res := markdownIngestionParser(t).ParseWithResult(ctx, "limit.md", []byte("Before\n\n![one]("+markdownMediaDataURI+")\n\n![two](data:image/png;base64,YQ==)\n\nAfter\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if images := markdownImageItems(t, res); len(images) != 0 {
		t.Fatalf("image items = %+v, want none", images)
	}
	texts := markdownText(res)
	if !strings.Contains(texts, "Before") || !strings.Contains(texts, "After") {
		t.Fatalf("text after skipped images lost: %q", texts)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "run media cache limit") {
		t.Fatalf("missing run-limit warning: %v", res.Warnings)
	}
}

func TestMarkdownParserIngestionCancellationFailsTask(t *testing.T) {
	ctx, cancel := context.WithCancel(media.WithCache(context.Background(), &recordingMediaCache{}))
	cancel()
	res := markdownIngestionParser(t).ParseWithResult(ctx, "canceled.md", []byte("![figure]("+markdownMediaDataURI+")\n"))
	if !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("ParseWithResult error = %v, want context.Canceled", res.Err)
	}
}

// TestPDFMarkdownKeepsInlineDataURIImage pins that the PDF backend Markdown
// normalization path still inlines data-URI images: media-ref mode is an
// ingestion-only opt-in, and the PDF path must not change.
func TestPDFMarkdownKeepsInlineDataURIImage(t *testing.T) {
	res := parsePDFMarkdown(context.Background(), "ocr.pdf", "![figure]("+markdownMediaDataURI+")\n")
	if res.Err != nil {
		t.Fatalf("parsePDFMarkdown: %v", res.Err)
	}
	images := markdownImageItems(t, res)
	if len(images) != 1 {
		t.Fatalf("image items = %+v, want one inline image", ItemsToMaps(res.JSON))
	}
	if images[0]["image"] != "aGVsbG8=" {
		t.Fatalf("image payload = %v, want the inline base64 payload", images[0]["image"])
	}
	if _, ok := images[0]["media_ref"]; ok {
		t.Fatalf("PDF Markdown normalization emitted a media_ref: %+v", images[0])
	}
}

func TestMarkdownParserIngestionFlattenKeepsImageAsText(t *testing.T) {
	ctx, cache := recordingMediaContext()
	p := markdownIngestionParser(t)
	markdown, ok := p.(*MarkdownParser)
	if !ok {
		t.Fatalf("GetParser(FileTypeMarkdown) = %T", p)
	}
	markdown.FlattenMediaToText = true

	res := p.ParseWithResult(ctx, "flatten.md", []byte("![figure]("+markdownMediaDataURI+")\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(cache.puts) != 1 || string(cache.puts[0].data) != "hello" {
		t.Fatalf("cache puts = %+v", cache.puts)
	}
	items := ItemsToMaps(res.JSON)
	if len(items) != 1 || items[0]["doc_type_kwd"] != DocTypeText {
		t.Fatalf("flattened items = %+v, want one text item", items)
	}
	if entity.MediaRefFromAny(items[0]["media_ref"]) == nil {
		t.Fatalf("flattened image lost its media_ref: %+v", items[0])
	}
}

// TestMarkdownParserIngestionCountsInvalidDataURIOnce pins the shared source
// key between the Markdown resolver and the walker's ref-only data-URI
// fallback: one broken data URI must produce one warning, not an
// invalid-payload warning plus an unresolved-source warning.
func TestMarkdownParserIngestionCountsInvalidDataURIOnce(t *testing.T) {
	ctx, cache := recordingMediaContext()
	res := markdownIngestionParser(t).ParseWithResult(ctx, "bad.md", []byte("![broken](data:image/png;base64,!!!)\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(cache.puts) != 0 {
		t.Fatalf("cache puts = %+v, want none", cache.puts)
	}
	warnings := strings.Join(res.Warnings, "\n")
	if strings.Contains(warnings, "invalid or oversized inline image") {
		t.Fatalf("broken data URI was resolved twice: %v", res.Warnings)
	}
	if got := strings.Count(warnings, "unresolved image reference"); got != 1 {
		t.Fatalf("unresolved warnings = %d, want 1: %v", got, res.Warnings)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one", res.Warnings)
	}
}

// TestMarkdownParserIngestionCacheLimitCountedOnce pins that a data URI the
// run cache rejects is attempted once and reported once.
func TestMarkdownParserIngestionCacheLimitCountedOnce(t *testing.T) {
	cache := &recordingMediaCache{putErr: fmt.Errorf("mediacache: run image count limit 1 exceeded: %w", media.ErrMediaLimitExceeded)}
	ctx := media.WithCache(context.Background(), cache)

	res := markdownIngestionParser(t).ParseWithResult(ctx, "limit.md", []byte("![one]("+markdownMediaDataURI+")\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if cache.putCalls != 1 {
		t.Fatalf("cache Put calls = %d, want 1", cache.putCalls)
	}
	warnings := strings.Join(res.Warnings, "\n")
	if !strings.Contains(warnings, "skipped 1 image(s) after the run media cache limit") {
		t.Fatalf("cache-limit warning count is not 1: %v", res.Warnings)
	}
	if strings.Contains(warnings, "skipped 2 image(s)") {
		t.Fatalf("cache-limit warning double counted: %v", res.Warnings)
	}
}

// TestMarkdownParserIngestionBudgetRejectionCountedOnce pins that a data URI
// rejected by the run byte budget is reserved once, not once per fallback.
func TestMarkdownParserIngestionBudgetRejectionCountedOnce(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", "1")
	ctx, cache := recordingMediaContext()
	res := markdownIngestionParser(t).ParseWithResult(ctx, "budget.md", []byte("![one]("+markdownMediaDataURI+")\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(cache.puts) != 0 {
		t.Fatalf("cache puts = %+v, want none", cache.puts)
	}
	warnings := strings.Join(res.Warnings, "\n")
	if !strings.Contains(warnings, "omitted 1 embedded image payload(s) after reaching the 1-byte run image budget") {
		t.Fatalf("budget warning count is not 1: %v", res.Warnings)
	}
}

// TestMarkdownParserIngestionStopsRemoteDownloadsAfterByteBudget pins that an
// unknown-size download rejected by the run byte budget stops later downloads
// before they reach the network.
func TestMarkdownParserIngestionStopsRemoteDownloadsAfterByteBudget(t *testing.T) {
	allowLoopbackImages(t)
	t.Setenv("RAGFLOW_MEDIA_MAX_BYTES_PER_RUN", "1")
	var hits atomic.Int32
	server := imageTestServer(t, "\x89PNG\r\n\x1a\nremote", &hits)
	ctx, cache := recordingMediaContext()

	md := "![a](" + server.URL + "/a.png)\n\n![b](" + server.URL + "/b.png)\n"
	res := markdownIngestionParser(t).ParseWithResult(ctx, "budget.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if hits.Load() != 1 {
		t.Fatalf("remote fetches = %d, want 1 once the byte budget is spent", hits.Load())
	}
	if len(cache.puts) != 0 {
		t.Fatalf("cache puts = %+v, want none", cache.puts)
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "after reaching the 1-byte run image budget") {
		t.Fatalf("missing byte-budget warning: %v", res.Warnings)
	}
}

// TestMarkdownTextParserSkipsImages pins the text-only contract: preview, CLI
// and attachment parsing must not download remote images, decode data URIs or
// emit image payloads, while keeping the alt text and surrounding prose.
func TestMarkdownTextParserSkipsImages(t *testing.T) {
	allowLoopbackImages(t)
	var hits atomic.Int32
	server := imageTestServer(t, "\x89PNG\r\n\x1a\nremote", &hits)

	p, err := GetTextOnlyParser(utility.FileTypeMarkdown)
	if err != nil {
		t.Fatalf("GetTextOnlyParser(FileTypeMarkdown): %v", err)
	}
	markdown, ok := p.(*MarkdownParser)
	if !ok || !markdown.textOnly || markdown.mediaRefs {
		t.Fatalf("GetTextOnlyParser(FileTypeMarkdown) = %#v, want the text-only parser", p)
	}

	input := "Before\n\n![alt text](" + server.URL + "/image.png)\n\n" +
		"![inline](data:image/png;base64,aGVsbG8=)\n\n" +
		"![broken](data:image/png;base64,!!!)\n\n" +
		"| A | B |\n|---|---|\n| x | ![cell alt](" + server.URL + "/cell.png) |\n\n" +
		"After\n"
	// No media cache is attached: a text-only parse must not need one.
	res := p.ParseWithResult(context.Background(), "text.md", []byte(input))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if hits.Load() != 0 {
		t.Fatalf("text-only Markdown reached the network %d time(s)", hits.Load())
	}
	var texts []string
	for _, item := range ItemsToMaps(res.JSON) {
		if _, ok := item["image"]; ok {
			t.Fatalf("text-only Markdown retained inline image bytes: %+v", item)
		}
		if _, ok := item["media_ref"]; ok {
			t.Fatalf("text-only Markdown emitted a media_ref: %+v", item)
		}
		if text, ok := item["text"].(string); ok && text != "" {
			texts = append(texts, text)
		}
		if table, ok := item["table"].(*entity.TableData); ok {
			for _, row := range table.CellTexts() {
				texts = append(texts, strings.Join(row, " "))
			}
		}
	}
	joined := strings.Join(texts, "\n")
	for _, want := range []string{"Before", "After", "alt text", "inline", "broken", "cell alt"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("text-only Markdown lost %q: %q", want, joined)
		}
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none for a text-only parse", res.Warnings)
	}
}
