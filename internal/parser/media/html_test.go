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

package media

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
)

func TestScanHTMLMediaSpansHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	spans, _ := scanHTMLMediaSpans(ctx, []byte(`<img src="data:image/png;base64,aGVsbG8=">`))
	if len(spans) != 0 {
		t.Fatalf("cancelled scan returned %d spans", len(spans))
	}
}

func TestScanHTMLMediaSpansHandlesCaseQuotesAndPadding(t *testing.T) {
	input := `<img src='DATA:IMAGE/PNG;BASE64,aGVsbG8'>`
	spans, _ := scanHTMLMediaSpans(context.Background(), []byte(input))
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Ref.MIME != "image/png" {
		t.Errorf("mime = %q, want image/png", spans[0].Ref.MIME)
	}
	data, sum, err := decodeHTMLImagePayload([]byte(input)[spans[0].TagStart:spans[0].TagEnd])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("data = %q, want hello", data)
	}
	if hex.EncodeToString(sum[:]) != spans[0].Ref.SHA256 {
		t.Errorf("scan digest and decode digest differ")
	}
}

// TestHTMLParser_RefModeKeepsResultIndependentOfImageBytes pins the core
// goal: the parse result no longer scales with image payloads.
func TestScanHTMLMediaSpansDecodesCharacterReferences(t *testing.T) {
	// aGVsbG8+ decodes to "hello>"; &#43; is the HTML-escaped '+'.
	input := `<img src="data:image/png;base64,aGVsbG8&#43;">`
	spans, _ := scanHTMLMediaSpans(context.Background(), []byte(input))
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	data, sum, err := decodeHTMLImagePayload([]byte(input)[spans[0].TagStart:spans[0].TagEnd])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(data) != "hello>" {
		t.Fatalf("data = %q, want hello>", data)
	}
	if hex.EncodeToString(sum[:]) != spans[0].Ref.SHA256 {
		t.Fatal("scan and decode digests differ")
	}
}

func TestScanHTMLMediaSpansSkipsHeadersBeyondTheParserLimit(t *testing.T) {
	// The parser rejects headers longer than 128 bytes; the scan must not
	// look past that window for a comma, and must still find the next image.
	input := `<img src="data:image/png;` + strings.Repeat("x", 200) + `">` +
		`<img src="data:image/png;base64,aGVsbG8=">`
	spans, _ := scanHTMLMediaSpans(context.Background(), []byte(input))
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want the valid image only", len(spans))
	}
}
