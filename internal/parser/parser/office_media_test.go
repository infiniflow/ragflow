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

package parser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/media"
	"ragflow/internal/utility"
)

// officeTestPNG returns a small deterministic PNG. Distinct seeds produce
// distinct bytes, so tests can use the digest as the image identity.
func officeTestPNG(t *testing.T, seed int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 17, 17))
	img.SetGray(seed%17, (seed/17)%17, color.Gray{Y: 1 + uint8(seed%255)})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return buf.Bytes()
}

// officeTestImageB64 is the base64 encoding the Office IR uses for image data.
func officeTestImageB64(t *testing.T, seed int) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(officeTestPNG(t, seed))
}

// officeMediaTestContext returns a context carrying a recording media cache.
func officeMediaTestContext() (context.Context, *recordingMediaCache) {
	cache := &recordingMediaCache{}
	return media.WithCache(context.Background(), cache), cache
}

// officeMediaTestSink builds a sink over a recording media cache.
func officeMediaTestSink(t *testing.T, format string) (*officeMediaSink, *recordingMediaCache) {
	t.Helper()
	ctx, cache := officeMediaTestContext()
	return newOfficeMediaSink(ctx, format), cache
}

// officeMediaTestSinkWithCache builds a sink over the given cache.
func officeMediaTestSinkWithCache(format string, cache media.Cache) *officeMediaSink {
	return newOfficeMediaSink(media.WithCache(context.Background(), cache), format)
}

// requireOfficeMediaRef asserts that an item carries a valid media_ref and no
// legacy base64 image payload.
func requireOfficeMediaRef(t *testing.T, item map[string]any) *entity.MediaRef {
	t.Helper()
	if _, ok := item["image"]; ok {
		t.Fatalf("legacy image payload retained: %#v", item)
	}
	ref := entity.MediaRefFromAny(item["media_ref"])
	if ref == nil || !ref.Valid() {
		t.Fatalf("item has no valid media_ref: %#v", item)
	}
	return ref
}

// dedupingMediaCache mirrors the run media cache's digest dedupe: repeated
// writes of one payload reach the backend once and share its reference.
type dedupingMediaCache struct {
	recordingMediaCache
	backendWrites int
	refs          map[string]entity.MediaRef
}

func (c *dedupingMediaCache) Put(ctx context.Context, format, mime string, data []byte) (entity.MediaRef, error) {
	sum := sha256.Sum256(data)
	key := hex.EncodeToString(sum[:])
	if ref, ok := c.refs[key]; ok {
		return ref, nil
	}
	ref, err := c.recordingMediaCache.Put(ctx, format, mime, data)
	if err != nil {
		return entity.MediaRef{}, err
	}
	if c.refs == nil {
		c.refs = make(map[string]entity.MediaRef)
	}
	c.refs[key] = ref
	c.backendWrites++
	return ref, nil
}

func TestOfficeMediaSinkWritesImageToRunCache(t *testing.T) {
	sink, cache := officeMediaTestSink(t, officeMediaFormatDOCX)
	imageData := officeTestPNG(t, 3)
	ref, ok := sink.put(imageData, "")
	if !ok || ref == nil {
		t.Fatalf("put = (%+v, %v), want an accepted image", ref, ok)
	}
	if sink.err != nil {
		t.Fatalf("sink error: %v", sink.err)
	}
	if len(cache.puts) != 1 {
		t.Fatalf("cache writes = %d, want 1", len(cache.puts))
	}
	put := cache.puts[0]
	if put.format != officeMediaFormatDOCX || put.mime != "image/png" || !bytes.Equal(put.data, imageData) {
		t.Fatalf("cache write = %+v, want docx image/png with the image bytes", put)
	}
	sum := sha256.Sum256(imageData)
	if ref.Kind != entity.MediaKindKV || ref.Scope != "run-1" ||
		ref.SHA256 != hex.EncodeToString(sum[:]) || ref.MIME != "image/png" ||
		ref.Size != int64(len(imageData)) {
		t.Fatalf("media_ref = %+v", ref)
	}
	if warnings := sink.warnings(); len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

// TestOfficeMediaSinkReusesOneBackendWriteForRepeatedImage pins that repeating
// one image does not charge the document budget twice and reaches the media
// cache backend once, while every occurrence still gets the same reference.
func TestOfficeMediaSinkReusesOneBackendWriteForRepeatedImage(t *testing.T) {
	cache := &dedupingMediaCache{}
	sink := officeMediaTestSinkWithCache(officeMediaFormatPPTX, cache)
	imageData := officeTestPNG(t, 4)

	first, ok := sink.put(imageData, "image/png")
	if !ok {
		t.Fatal("first put rejected")
	}
	second, ok := sink.put(imageData, "image/png")
	if !ok {
		t.Fatal("repeated put rejected")
	}
	if *first != *second {
		t.Fatalf("references differ: %+v vs %+v", *first, *second)
	}
	if cache.backendWrites != 1 {
		t.Fatalf("backend writes = %d, want 1", cache.backendWrites)
	}
	if warnings := sink.warnings(); len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
}

func TestOfficeMediaSinkSkipsInvalidImages(t *testing.T) {
	sink, cache := officeMediaTestSink(t, officeMediaFormatDOCX)
	for _, data := range [][]byte{nil, []byte("not an image")} {
		if ref, ok := sink.put(data, ""); ok || ref != nil {
			t.Fatalf("put(%q) = (%+v, %v), want skipped", data, ref, ok)
		}
	}
	if sink.err != nil {
		t.Fatalf("sink error: %v", sink.err)
	}
	if cache.putCalls != 0 {
		t.Fatalf("cache writes = %d, want none for invalid images", cache.putCalls)
	}
	warnings := strings.Join(sink.warnings(), "\n")
	if !strings.Contains(warnings, "skipped 2 invalid or unsupported embedded image(s)") {
		t.Fatalf("warnings = %q, want invalid-image warning", warnings)
	}
}

func TestOfficeMediaSinkFailsWithoutCache(t *testing.T) {
	sink := newOfficeMediaSink(context.Background(), officeMediaFormatDOCX)
	ref, ok := sink.put(officeTestPNG(t, 5), "")
	if ok || ref != nil {
		t.Fatalf("put = (%+v, %v), want failure without a cache", ref, ok)
	}
	if sink.err == nil || !strings.Contains(sink.err.Error(), "media cache is not configured") {
		t.Fatalf("sink error = %v, want missing-cache failure", sink.err)
	}
}

func TestOfficeMediaSinkFailsOnCacheWriteError(t *testing.T) {
	writeErr := errors.New("kvrocks unavailable")
	cache := &recordingMediaCache{putErr: writeErr}
	sink := officeMediaTestSinkWithCache(officeMediaFormatDOCX, cache)

	if ref, ok := sink.put(officeTestPNG(t, 6), ""); ok || ref != nil {
		t.Fatalf("put = (%+v, %v), want failure", ref, ok)
	}
	if !errors.Is(sink.err, writeErr) {
		t.Fatalf("sink error = %v, want %v", sink.err, writeErr)
	}
	if cache.putCalls != 1 {
		t.Fatalf("cache Put calls = %d, want 1", cache.putCalls)
	}
	if warnings := sink.warnings(); len(warnings) != 0 {
		t.Fatalf("warnings = %v, want the failure reported as an error, not a skip", warnings)
	}
	if ref, ok := sink.put(officeTestPNG(t, 7), ""); ok || ref != nil {
		t.Fatalf("put after failure = (%+v, %v), want skipped", ref, ok)
	}
	if cache.putCalls != 1 {
		t.Fatalf("cache Put calls = %d, want traversal to stop after the failure", cache.putCalls)
	}
}

func TestOfficeMediaSinkSkipsImageAtRunCacheLimit(t *testing.T) {
	cache := &recordingMediaCache{putErr: fmt.Errorf(
		"mediacache: run image count limit 1 exceeded: %w", media.ErrMediaLimitExceeded)}
	sink := officeMediaTestSinkWithCache(officeMediaFormatDOCX, cache)

	if ref, ok := sink.put(officeTestPNG(t, 8), ""); ok || ref != nil {
		t.Fatalf("put = (%+v, %v), want skipped at the run limit", ref, ok)
	}
	if sink.err != nil {
		t.Fatalf("sink error = %v, want a warning, not a parse failure", sink.err)
	}
	warnings := strings.Join(sink.warnings(), "\n")
	if !strings.Contains(warnings, "skipped 1 image(s) after the run media cache limit") {
		t.Fatalf("warnings = %q, want run-limit warning", warnings)
	}
}

func TestOfficeMediaSinkSkipsOversizedImage(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_IMAGE_MAX_BYTES", "16")
	sink, cache := officeMediaTestSink(t, officeMediaFormatDOCX)
	if ref, ok := sink.put(officeTestPNG(t, 9), ""); ok || ref != nil {
		t.Fatalf("put = (%+v, %v), want skipped for exceeding the per-image limit", ref, ok)
	}
	if sink.err != nil {
		t.Fatalf("sink error = %v, want a warning", sink.err)
	}
	if cache.putCalls != 0 {
		t.Fatalf("cache writes = %d, want none for an oversized image", cache.putCalls)
	}
	warnings := strings.Join(sink.warnings(), "\n")
	if !strings.Contains(warnings, "larger than the 16-byte per-image limit") {
		t.Fatalf("warnings = %q, want oversized-image warning", warnings)
	}
}

// TestOfficeMediaSinkCapsDistinctImages pins the run source limit: the first
// 256 distinct images are admitted and later sources are skipped with a
// warning instead of failing the parse.
func TestOfficeMediaSinkCapsDistinctImages(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "256")
	sink, cache := officeMediaTestSink(t, officeMediaFormatXLSX)
	emitted := 0
	for i := 0; i <= 256; i++ {
		if _, ok := sink.put(officeTestPNG(t, i), ""); ok {
			emitted++
		}
	}
	if emitted != 256 {
		t.Fatalf("admitted images = %d, want 256", emitted)
	}
	if cache.putCalls != 256 {
		t.Fatalf("cache writes = %d, want 256", cache.putCalls)
	}
	if sink.err != nil {
		t.Fatalf("sink error = %v, want a warning", sink.err)
	}
	warnings := strings.Join(sink.warnings(), "\n")
	if !strings.Contains(warnings, "stopped extracting embedded images after the 256-item run limit") {
		t.Fatalf("warnings = %q, want run-limit warning", warnings)
	}
}

// TestBuildDOCXJSONSections_KeepsTextWhenRunLimitSkipsImage verifies that a
// cache run-limit skip removes only the image item and warns; surrounding text
// and the rest of the document survive.
func TestBuildDOCXJSONSections_KeepsTextWhenRunLimitSkipsImage(t *testing.T) {
	cache := &recordingMediaCache{putErr: fmt.Errorf(
		"mediacache: run image count limit 1 exceeded: %w", media.ErrMediaLimitExceeded)}
	sink := officeMediaTestSinkWithCache(officeMediaFormatDOCX, cache)
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"before"}]},
		{"type":"image","data":"` + officeTestImageB64(t, 10) + `"},
		{"type":"paragraph","content":[{"type":"text","text":"after"}]}
	]}]}`

	sections := buildDOCXJSONSections(irJSON, sink)
	if sink.err != nil {
		t.Fatalf("sink error: %v", sink.err)
	}
	if len(sections) != 2 || sections[0]["text"] != "before" || sections[1]["text"] != "after" {
		t.Fatalf("sections = %+v, want the surrounding text without the skipped image", sections)
	}
	if !strings.Contains(strings.Join(sink.warnings(), "\n"), "after the run media cache limit") {
		t.Fatalf("warnings = %v, want run-limit warning", sink.warnings())
	}
}

func TestOfficeImageMIME(t *testing.T) {
	emf := make([]byte, 44)
	binary.LittleEndian.PutUint32(emf, 1)
	copy(emf[40:44], " EMF")

	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "png", data: officeTestPNG(t, 0), want: "image/png"},
		{name: "jpeg", data: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}, want: "image/jpeg"},
		{name: "gif", data: []byte("GIF89a\x01\x00\x01\x00"), want: "image/gif"},
		{name: "bmp", data: []byte("BM\x00\x00\x00\x00"), want: "image/bmp"},
		{name: "tiff little endian", data: []byte("II*\x00\x08\x00\x00\x00"), want: "image/tiff"},
		{name: "tiff big endian", data: []byte("MM\x00*\x00\x00\x00\x08"), want: "image/tiff"},
		{name: "emf", data: emf, want: "image/x-emf"},
		{name: "wmf placeable", data: []byte{0xD7, 0xCD, 0xC6, 0x9A, 0x00, 0x00}, want: "image/x-wmf"},
		{name: "wmf standard", data: []byte{1, 0, 9, 0, 0, 3}, want: "image/x-wmf"},
		{name: "svg", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), want: "image/svg+xml"},
		{name: "xml svg", data: []byte("\xef\xbb\xbf<?xml version=\"1.0\"?><svg xmlns=\"http://www.w3.org/2000/svg\"/>"), want: "image/svg+xml"},
		{name: "unknown", data: []byte("not an image"), want: ""},
		{name: "empty", data: nil, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := officeImageMIME(tc.data); got != tc.want {
				t.Errorf("officeImageMIME = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDOCXParserFailsWhenMediaCacheIsMissing pins that the normal DOCX parser
// fails the parse instead of silently dropping an embedded image when no run
// media cache is attached.
func TestDOCXParserFailsWhenMediaCacheIsMissing(t *testing.T) {
	orig := docxExtract
	t.Cleanup(func() { docxExtract = orig })
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"keep"}]},
		{"type":"image","data":"` + officeTestImageB64(t, 11) + `"}
	]}]}`
	docxExtract = func([]byte, string) (string, string, string, error, error) {
		return irJSON, "", "", nil, nil
	}

	res := NewDOCXParser().ParseWithResult(context.Background(), "missing.docx", []byte("PK\x03\x04"))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "media cache is not configured") {
		t.Fatalf("ParseWithResult error = %v, want missing-cache failure", res.Err)
	}
}

func TestDOCXTextParserSkipsImagesWithoutCache(t *testing.T) {
	orig := docxExtract
	t.Cleanup(func() { docxExtract = orig })
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"keep"}]},
		{"type":"image","data":"` + officeTestImageB64(t, 12) + `"}
	]}]}`
	docxExtract = func([]byte, string) (string, string, string, error, error) {
		return irJSON, "", "", nil, nil
	}

	p, err := GetTextOnlyParser(utility.FileTypeDOCX)
	if err != nil {
		t.Fatal(err)
	}
	res := p.ParseWithResult(context.Background(), "text.docx", []byte("PK\x03\x04"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	items := ItemsToMaps(res.JSON)
	if len(items) != 1 || items[0]["text"] != "keep" {
		t.Fatalf("items = %+v, want the text without the skipped image", items)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none for a text-only parse", res.Warnings)
	}
}

func TestPPTXParserFailsWhenMediaCacheIsMissing(t *testing.T) {
	orig := pptxExtract
	t.Cleanup(func() { pptxExtract = orig })
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"keep"}]},
		{"type":"image","data":"` + officeTestImageB64(t, 13) + `"}
	]}]}`
	pptxExtract = func([]byte, string) (string, string, string, error, error) {
		return irJSON, "", "", nil, nil
	}

	res := NewPPTXParser().ParseWithResult(context.Background(), "missing.pptx", []byte("PK\x03\x04"))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "media cache is not configured") {
		t.Fatalf("ParseWithResult error = %v, want missing-cache failure", res.Err)
	}
}

func TestPPTXTextParserSkipsImagesWithoutCache(t *testing.T) {
	orig := pptxExtract
	t.Cleanup(func() { pptxExtract = orig })
	irJSON := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"keep"}]},
		{"type":"image","data":"` + officeTestImageB64(t, 14) + `"}
	]}]}`
	pptxExtract = func([]byte, string) (string, string, string, error, error) {
		return irJSON, "", "", nil, nil
	}

	p, err := GetTextOnlyParser(utility.FileTypePPTX)
	if err != nil {
		t.Fatal(err)
	}
	res := p.ParseWithResult(context.Background(), "text.pptx", []byte("PK\x03\x04"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	items := ItemsToMaps(res.JSON)
	if len(items) != 1 || items[0]["text"] != "keep" {
		t.Fatalf("items = %+v, want the text without the skipped image", items)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none for a text-only parse", res.Warnings)
	}
}

// SVG sniffing must inspect a bounded prefix without copying a large payload.
func TestOfficeSVGSniffDoesNotCopyPayload(t *testing.T) {
	data := append([]byte(`<?xml version="1.0"?><SVG xmlns="http://www.w3.org/2000/svg">`), bytes.Repeat([]byte("X"), 1<<20)...)
	if !isSVGImage(data) {
		t.Fatal("SVG not recognized")
	}
	if allocations := testing.AllocsPerRun(10, func() { _ = isSVGImage(data) }); allocations != 0 {
		t.Fatalf("SVG sniff allocated %v times; want no payload copy", allocations)
	}
}

func TestOfficeMediaSinkCountsDistinctInvalidPayloadsWithoutSpendingBudget(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "1")
	sink, cache := officeMediaTestSink(t, officeMediaFormatDOCX)
	for _, data := range [][]byte{nil, {}, []byte("unrecognized-a"), []byte("unrecognized-a"), []byte("unrecognized-b"), []byte("unrecognized-b")} {
		if ref, ok := sink.put(data, ""); ok || ref != nil {
			t.Fatalf("invalid payload accepted: %q", data)
		}
	}
	if cache.putCalls != 0 {
		t.Fatalf("invalid payload cache writes = %d, want 0", cache.putCalls)
	}
	if ref, ok := sink.put(officeTestPNG(t, 2), ""); !ok || ref == nil {
		t.Fatal("invalid payloads consumed the only image slot")
	}
	if sink.err != nil {
		t.Fatalf("sink error: %v", sink.err)
	}
	warnings := sink.warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "skipped 3 invalid or unsupported embedded image(s)") {
		t.Fatalf("warnings = %v, want one warning for three distinct invalid payloads", warnings)
	}
	if cache.putCalls != 1 {
		t.Fatalf("cache writes = %d, want only the valid image", cache.putCalls)
	}
}
