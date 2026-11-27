package chunker

import (
	"reflect"
	"strings"
	"testing"
)

// Regression coverage for #22874: a CRLF document with a custom (backtick)
// delimiter must split exactly like its LF twin because line endings are
// normalized before the delimiter is matched. Both the text path
// (invokeTextPayload -> chunkPerSegment) and the JSON path
// (invokeJSONPayload -> chunkFromItem) are exercised; their LF twin is the
// baseline for the expected chunk texts.

const (
	crlfCustomDelimDoc = "Alpha\r\nstill one paragraph\r\n\r\nBeta paragraph\r\n"
	lfCustomDelimDoc   = "Alpha\nstill one paragraph\n\nBeta paragraph\n"
	backtickBlankLine  = "`\n\n`"
)

func customCRLFTextParams() map[string]any {
	return map[string]any{
		"chunk_token_size": float64(128),
		"delimiters":       []string{backtickBlankLine},
	}
}

func crlfTextInput(text string) map[string]any {
	return map[string]any{"name": "t", "output_format": "text", "text": text}
}

func crlfJSONTextInput(text string) map[string]any {
	return map[string]any{
		"name":          "t",
		"output_format": "json",
		"json":          []map[string]any{{"text": text, "doc_type_kwd": "text"}},
	}
}

func TestCustomDelimiterTextCRLFNormalized(t *testing.T) {
	params := customCRLFTextParams()
	crlf := chunkTexts(invokeTokenChunks(t, params, crlfTextInput(crlfCustomDelimDoc)))
	lf := chunkTexts(invokeTokenChunks(t, params, crlfTextInput(lfCustomDelimDoc)))

	if len(crlf) != 2 {
		t.Fatalf("CRLF text path: want 2 chunks, got %d (%q)", len(crlf), crlf)
	}
	for i, s := range crlf {
		if strings.Contains(s, "\r") {
			t.Errorf("chunk[%d] carries carriage return: %q", i, s)
		}
	}
	if !reflect.DeepEqual(crlf, lf) {
		t.Errorf("CRLF/LF divergence:\n crlf=%q\n lf  =%q", crlf, lf)
	}
}

func TestCustomDelimiterJSONCRLFNormalized(t *testing.T) {
	params := customCRLFTextParams()
	crlf := chunkTexts(invokeTokenChunks(t, params, crlfJSONTextInput(crlfCustomDelimDoc)))
	lf := chunkTexts(invokeTokenChunks(t, params, crlfJSONTextInput(lfCustomDelimDoc)))

	if len(crlf) != 2 {
		t.Fatalf("CRLF JSON path: want 2 chunks, got %d (%q)", len(crlf), crlf)
	}
	for i, s := range crlf {
		if strings.Contains(s, "\r") {
			t.Errorf("chunk[%d] carries carriage return: %q", i, s)
		}
	}
	if !reflect.DeepEqual(crlf, lf) {
		t.Errorf("CRLF/LF divergence:\n crlf=%q\n lf  =%q", crlf, lf)
	}
}

// TestCustomDelimiterEntryCRLFNormalized covers the delimiter side of the same
// rule: input is normalized before matching, so a delimiter entry configured
// with CRLF must be normalized too. Otherwise "`\r\n\r\n`" would look for bytes
// the input no longer contains and the document would stay one chunk.
func TestCustomDelimiterEntryCRLFNormalized(t *testing.T) {
	crlfParams := map[string]any{
		"chunk_token_size": float64(128),
		"delimiters":       []string{"`\r\n\r\n`"},
	}
	crlf := chunkTexts(invokeTokenChunks(t, crlfParams, crlfTextInput(crlfCustomDelimDoc)))
	lf := chunkTexts(invokeTokenChunks(t, customCRLFTextParams(), crlfTextInput(lfCustomDelimDoc)))

	if len(crlf) != 2 {
		t.Fatalf("CRLF delimiter entry: want 2 chunks, got %d (%q)", len(crlf), crlf)
	}
	if !reflect.DeepEqual(crlf, lf) {
		t.Errorf("CRLF delimiter entry diverged from LF delimiter:\n crlf=%q\n lf  =%q", crlf, lf)
	}
}

// TestBareDelimiterEntryCRLFNormalized is the bare (non-backtick) counterpart:
// a CRLF bare delimiter must behave like its LF twin after normalization.
func TestBareDelimiterEntryCRLFNormalized(t *testing.T) {
	// A small target forces the token merge to keep paragraph boundaries, so a
	// delimiter that failed to match would produce different chunk texts.
	crlfParams := map[string]any{
		"chunk_token_size": float64(4),
		"delimiters":       []string{"\r\n\r\n"},
	}
	lfParams := map[string]any{
		"chunk_token_size": float64(4),
		"delimiters":       []string{"\n\n"},
	}
	crlf := chunkTexts(invokeTokenChunks(t, crlfParams, crlfTextInput(crlfCustomDelimDoc)))
	lf := chunkTexts(invokeTokenChunks(t, lfParams, crlfTextInput(lfCustomDelimDoc)))

	for i, s := range crlf {
		if strings.Contains(s, "\r") {
			t.Errorf("chunk[%d] carries carriage return: %q", i, s)
		}
	}
	if !reflect.DeepEqual(crlf, lf) {
		t.Errorf("CRLF bare delimiter entry diverged from LF delimiter:\n crlf=%q\n lf  =%q", crlf, lf)
	}
}

// TestCustomDelimiterSentenceFallbackCRLFNormalized covers the reviewer-flagged
// sibling path: the text-parser sentence fallback (file_type txt, bare "\n"
// primary delimiter) returns item text before chunkFromItem runs, so it must
// normalize line endings too. A custom CRLF child delimiter is compiled to LF,
// and without normalization it stops matching the raw "\r" item.
func TestCustomDelimiterSentenceFallbackCRLFNormalized(t *testing.T) {
	params := map[string]any{
		"chunk_token_size":    float64(128),
		"delimiters":          []string{"\n"},
		"children_delimiters": []string{"`\rX`"},
	}
	inputFor := func(text string) map[string]any {
		in := crlfJSONTextInput(text)
		in["file_type"] = "txt"
		return in
	}
	crlf := chunkTexts(invokeTokenChunks(t, params, inputFor("A\rXB")))
	lf := chunkTexts(invokeTokenChunks(t, params, inputFor("A\nXB")))

	if len(crlf) == 0 || len(lf) == 0 {
		t.Fatalf("sentence fallback returned no chunks: crlf=%q lf=%q", crlf, lf)
	}
	for i, s := range crlf {
		if strings.Contains(s, "\r") {
			t.Errorf("chunk[%d] carries carriage return: %q", i, s)
		}
	}
	if !reflect.DeepEqual(crlf, lf) {
		t.Errorf("sentence fallback CRLF/LF divergence:\n crlf=%q\n lf  =%q", crlf, lf)
	}
}

// TestCustomDelimiterJSONNonTextBytesUntouched locks the scope of the fix: only
// text items are normalized. A table/image item keeps its bytes verbatim.
func TestCustomDelimiterJSONNonTextBytesUntouched(t *testing.T) {
	params := customCRLFTextParams()
	for _, docType := range []string{"table", "image"} {
		t.Run(docType, func(t *testing.T) {
			chunks := invokeTokenChunks(t, params, map[string]any{
				"name":          "t",
				"output_format": "json",
				"json":          []map[string]any{{"text": crlfCustomDelimDoc, "doc_type_kwd": docType}},
			})
			if len(chunks) != 1 {
				t.Fatalf("want 1 chunk, got %d", len(chunks))
			}
			if got := chunks[0]["text"]; got != crlfCustomDelimDoc {
				t.Errorf("non-text text changed: want %q got %q", crlfCustomDelimDoc, got)
			}
		})
	}
}
