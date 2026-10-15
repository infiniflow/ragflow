//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package chunker

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// testImagePayload renders a w x h gradient image encoded the way the Markdown
// parser emits images: bare base64 of the container bytes. An alpha below 0xff
// makes every other pixel of a PNG semi-transparent.
func testImagePayload(tb testing.TB, format string, w, h int, seed, alpha uint8) string {
	tb.Helper()
	var buf bytes.Buffer
	switch format {
	case "png":
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				a := uint8(0xff)
				if alpha < 0xff && (x+y)%2 == 0 {
					a = alpha
				}
				img.SetNRGBA(x, y, color.NRGBA{R: uint8(x) + seed, G: uint8(y) ^ seed, B: uint8(x*y) + seed, A: a})
			}
		}
		if err := png.Encode(&buf, img); err != nil {
			tb.Fatal(err)
		}
	case "jpeg":
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetRGBA(x, y, color.RGBA{R: uint8(x) + seed, G: uint8(y) ^ seed, B: uint8(x*y) + seed, A: 0xff})
			}
		}
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
			tb.Fatal(err)
		}
	default:
		tb.Fatalf("unknown image format %q", format)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// pairwiseComposite is the reference rendering: mergeMarkdownImagesChecked
// applied one payload at a time, which is how mergeMarkdownUnits composed a
// chunk's images before they were stacked once per chunk.
func pairwiseComposite(tb testing.TB, payloads ...string) string {
	tb.Helper()
	acc := ""
	for i, p := range payloads {
		var ok bool
		if acc, ok = mergeMarkdownImagesChecked(acc, p); !ok {
			tb.Fatalf("pairwise merge refused payload %d", i)
		}
	}
	return acc
}

// imageUnits builds the split markdown units for a text block followed by one
// image unit per payload, each carrying a short alt text.
func imageUnits(payloads []string) markdownSplit {
	units := []schema.ChunkDoc{{Text: "intro paragraph\n", DocType: "text", CKType: "text"}}
	for i, p := range payloads {
		units = append(units, schema.ChunkDoc{Text: fmt.Sprintf("figure %d\n", i), DocType: "image", Image: p})
	}
	units = append(units, schema.ChunkDoc{Text: "outro paragraph", DocType: "text", CKType: "text"})
	return splitMarkdownUnits(units, nil, true)
}

func TestMergeMarkdownUnitsStacksImagesLikePairwise(t *testing.T) {
	payloads := []string{
		testImagePayload(t, "png", 120, 40, 1, 0xff),
		testImagePayload(t, "jpeg", 90, 60, 2, 0xff),
		testImagePayload(t, "png", 150, 30, 3, 0xff),
		testImagePayload(t, "jpeg", 60, 60, 4, 0xff),
	}
	merged := mergeMarkdownUnits(imageUnits(payloads), 512, 0)
	if len(merged) != 1 {
		t.Fatalf("got %d chunks, want 1", len(merged))
	}
	if want := pairwiseComposite(t, payloads...); merged[0].Image != want {
		t.Fatalf("composite differs from the pairwise rendering (%d vs %d bytes)", len(merged[0].Image), len(want))
	}
}

func TestMergeMarkdownUnitsSemiTransparentImagesMatchPairwise(t *testing.T) {
	payloads := []string{
		testImagePayload(t, "png", 100, 40, 5, 0x80),
		testImagePayload(t, "jpeg", 80, 50, 6, 0xff),
		testImagePayload(t, "png", 100, 20, 7, 0x40),
	}
	merged := mergeMarkdownUnits(imageUnits(payloads), 512, 0)
	if len(merged) != 1 {
		t.Fatalf("got %d chunks, want 1", len(merged))
	}
	if want := pairwiseComposite(t, payloads...); merged[0].Image != want {
		t.Fatal("composite with semi-transparent sources differs from the pairwise rendering")
	}
}

func TestMergeMarkdownUnitsKeepsSingleImagePayloadVerbatim(t *testing.T) {
	a := testImagePayload(t, "jpeg", 64, 64, 8, 0xff)
	b := testImagePayload(t, "jpeg", 64, 64, 9, 0xff)
	// A budget that fits one image unit forces every image into its own chunk.
	merged := mergeMarkdownUnits(imageUnits([]string{a, b}), 4, 0)
	var images []string
	for _, ck := range merged {
		if ck.Image != "" {
			images = append(images, ck.Image)
		}
	}
	if len(images) != 2 || images[0] != a || images[1] != b {
		t.Fatalf("single-image chunks must carry the source payload untouched, got %d images", len(images))
	}
}

func TestMergeMarkdownUnitsDropsDuplicateAndRefusesUndecodableImages(t *testing.T) {
	a := testImagePayload(t, "png", 50, 50, 10, 0xff)
	b := testImagePayload(t, "jpeg", 50, 50, 11, 0xff)
	broken := base64.StdEncoding.EncodeToString([]byte("not an image"))
	merged := mergeMarkdownUnits(imageUnits([]string{a, a, b, broken}), 512, 0)
	if len(merged) != 2 {
		t.Fatalf("got %d chunks, want 2 (the undecodable image starts a new chunk)", len(merged))
	}
	if want := pairwiseComposite(t, a, b); merged[0].Image != want {
		t.Fatal("duplicate image must be dropped and the rest composed like the pairwise rendering")
	}
	if merged[1].Image != broken {
		t.Fatal("undecodable image must be carried verbatim on its own chunk")
	}
}

func TestMergeMarkdownUnitsHeadingTableComposite(t *testing.T) {
	headingImage := testImagePayload(t, "png", 70, 30, 12, 0xff)
	tableImage := testImagePayload(t, "jpeg", 90, 40, 13, 0xff)
	units := []schema.ChunkDoc{
		{Text: "## Results", DocType: "text", CKType: "heading", Image: headingImage},
		{Text: "<table><tr><td>1</td></tr></table>", DocType: "table", Image: tableImage},
	}
	merged := mergeMarkdownUnits(splitMarkdownUnits(units, nil, true), 512, 0)
	if len(merged) != 1 || merged[0].CKType != "table" {
		t.Fatalf("heading and table must merge into one table chunk, got %+v", merged)
	}
	if !strings.HasPrefix(merged[0].Text, "## Results") {
		t.Fatalf("table chunk must keep the heading text, got %q", merged[0].Text)
	}
	if want := pairwiseComposite(t, headingImage, tableImage); merged[0].Image != want {
		t.Fatal("heading+table composite differs from the pairwise rendering")
	}
}

// BenchmarkMergeMarkdownUnitsImages merges six 1600x900 screenshots into one
// chunk: the shape of a how-to page whose steps are illustrated.
func BenchmarkMergeMarkdownUnitsImages(b *testing.B) {
	payloads := make([]string, 6)
	for i := range payloads {
		payloads[i] = testImagePayload(b, "jpeg", 1600, 900, uint8(i*7), 0xff)
	}
	parts := imageUnits(payloads)
	var merged []schema.ChunkDoc
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		merged = mergeMarkdownUnits(parts, 512, 0)
	}
	b.ReportMetric(float64(len(merged)), "chunks")
}
