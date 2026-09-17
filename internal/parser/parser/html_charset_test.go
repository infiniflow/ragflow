package parser

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
)

// gbkHTML is a Sina-finance-style balance-sheet page encoded in GBK with a
// GB2312 meta declaration, the shape reported by the "agent data pipeline
// parses HTML into mojibake" issue. GBK bytes double as GB18030 input.
func gbkHTML(t *testing.T, withMeta bool) []byte {
	t.Helper()
	head := "<html><head><title>贵州茅台(600519)资产负债表</title></head>"
	if withMeta {
		head = `<html><head><meta http-equiv="Content-Type" content="text/html; charset=GB2312">` +
			"<title>贵州茅台(600519)资产负债表</title></head>"
	}
	body := "<body><h1>贵州茅台(600519)资产负债表</h1>" +
		"<table><tr><th>项目</th><th>20251231</th></tr>" +
		"<tr><td>货币资金</td><td>56000000000.00</td></tr>" +
		"<tr><td>应收账款</td><td>1200000000.00</td></tr></table>" +
		"<p>单位：元</p></body></html>"
	src := head + body
	if !utf8.Valid([]byte(src)) {
		t.Fatal("test source must be valid UTF-8 before re-encoding")
	}
	out, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatalf("GBK encode: %v", err)
	}
	if utf8.Valid(out) {
		t.Fatal("GBK fixture unexpectedly decodes as UTF-8; fixture is ineffective")
	}
	return out
}

// htmlEncodedFixture re-encodes src with enc and guards that the fixture is
// effective: it must not itself be valid UTF-8, otherwise a decode test would
// trivially pass through the fast path and prove nothing.
func htmlEncodedFixture(t *testing.T, src string, enc encoding.Encoding) []byte {
	t.Helper()
	raw, err := enc.NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatalf("fixture encode: %v", err)
	}
	if utf8.Valid(raw) {
		t.Fatal("fixture unexpectedly decodes as UTF-8; fixture is ineffective")
	}
	return raw
}

// itemsText concatenates the text of every item in res, newline separated.
func itemsText(res ParseResult) string {
	var sb strings.Builder
	for _, it := range res.JSON {
		if s, ok := it["text"].(string); ok {
			sb.WriteString(s)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// assertDecodedChinese fails res on a parse error, any U+FFFD replacement
// rune, or missing expected Chinese substrings.
func assertDecodedChinese(t *testing.T, res ParseResult) {
	t.Helper()
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	got := itemsText(res)
	if strings.ContainsRune(got, '\ufffd') {
		t.Errorf("output contains U+FFFD replacement runes: %q", got)
	}
	for _, want := range []string{"\u8d35\u5dde\u8305\u53f0", "\u8d44\u4ea7\u8d1f\u503a\u8868", "\u8d27\u5e01\u8d44\u91d1", "\u5e94\u6536\u8d26\u6b3e"} {
		if !strings.Contains(got, want) {
			t.Errorf("decoded text missing %q; got: %q", want, got)
		}
	}
}

// TestHTMLParser_DecodesGBKMetaDeclared reproduces the reported issue: a
// GBK-encoded HTML file parsed by the data pipeline surfaced as mojibake
// because x/net/html assumes UTF-8. The meta charset declaration must drive
// the decode, mirroring Python's find_codec path.
func TestHTMLParser_DecodesGBKMetaDeclared(t *testing.T) {
	res := NewHTMLParser().ParseWithResult(context.Background(), "balance.html", gbkHTML(t, true))
	assertDecodedChinese(t, res)
	if enc, _ := res.File["encoding"].(string); enc == "" || enc == "utf-8" {
		t.Errorf("File.encoding = %q, want the detected non-UTF-8 label", enc)
	}
}

// TestHTMLParser_DecodesGBKWithoutDeclaration covers GBK bytes with no meta
// declaration: the GB18030 fallback chain candidate must decode them.
func TestHTMLParser_DecodesGBKWithoutDeclaration(t *testing.T) {
	res := NewHTMLParser().ParseWithResult(context.Background(), "balance.html", gbkHTML(t, false))
	assertDecodedChinese(t, res)
}

// TestHTMLParser_DecodesBig5MetaDeclared covers a Big5 page with its meta
// declaration.
func TestHTMLParser_DecodesBig5MetaDeclared(t *testing.T) {
	src := `<html><head><meta charset="big5"></head><body><h1>` +
		"\u53f0\u7063\u5927\u5b78" + `</h1><p>` + "\u8cc7\u6599\u5eab" + `</p></body></html>`
	raw, err := traditionalchinese.Big5.NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatalf("Big5 encode: %v", err)
	}
	if utf8.Valid(raw) {
		t.Fatal("Big5 fixture unexpectedly decodes as UTF-8; fixture is ineffective")
	}
	res := NewHTMLParser().ParseWithResult(context.Background(), "tw.html", raw)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	got := itemsText(res)
	if strings.ContainsRune(got, '\ufffd') {
		t.Errorf("output contains U+FFFD replacement runes: %q", got)
	}
	for _, want := range []string{"\u53f0\u7063\u5927\u5b78", "\u8cc7\u6599\u5eab"} {
		if !strings.Contains(got, want) {
			t.Errorf("decoded text missing %q; got: %q", want, got)
		}
	}
}

// TestHTMLParser_UTF8Passthrough ensures valid UTF-8 input is untouched and
// still reported as utf-8.
func TestHTMLParser_UTF8Passthrough(t *testing.T) {
	src := "<html><body><h1>\u8d35\u5dde\u8305\u53f0</h1><p>plain utf-8</p></body></html>"
	res := NewHTMLParser().ParseWithResult(context.Background(), "u.html", []byte(src))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	got := itemsText(res)
	if !strings.Contains(got, "plain utf-8") || !strings.Contains(got, "\u8d35\u5dde\u8305\u53f0") {
		t.Errorf("utf-8 passthrough lost content; got: %q", got)
	}
	if enc, _ := res.File["encoding"].(string); enc != "utf-8" {
		t.Errorf("File.encoding = %q, want utf-8", enc)
	}
}

// TestHTMLParser_DecodeTableSurvivesGBK verifies the structured table item
// (doc_type_kwd:"table") is emitted from the decoded document with its
// Chinese cell text intact.
func TestHTMLParser_DecodeTableSurvivesGBK(t *testing.T) {
	res := NewHTMLParser().ParseWithResult(context.Background(), "balance.html", gbkHTML(t, true))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	var tableText string
	for _, it := range res.JSON {
		if it["doc_type_kwd"] == "table" {
			tableText, _ = it["text"].(string)
		}
	}
	if tableText == "" {
		t.Fatalf("no structured table item emitted; items: %#v", res.JSON)
	}
	if !strings.Contains(tableText, "\u8d27\u5e01\u8d44\u91d1") {
		t.Errorf("table item missing decoded cell text; got: %q", tableText)
	}
}

// assertUndeclaredDecode parses an undeclared (no <meta charset>) document
// encoded with enc and asserts the original text round-trips: no U+FFFD and
// every want substring survives, proving the statistical detector steered the
// decode instead of the GB18030-first chain order hijacking the bytes.
func assertUndeclaredDecode(t *testing.T, raw []byte, wantLabel string, wants []string) {
	t.Helper()
	res := NewHTMLParser().ParseWithResult(context.Background(), "doc.html", raw)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	got := itemsText(res)
	if strings.ContainsRune(got, '\ufffd') {
		t.Errorf("output contains U+FFFD replacement runes: %q", got)
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("decoded text missing %q; got: %q", want, got)
		}
	}
	if enc, _ := res.File["encoding"].(string); enc != wantLabel {
		t.Errorf("File.encoding = %q, want %q", enc, wantLabel)
	}
}

// TestHTMLParser_DecodesBig5WithoutDeclaration pins the undeclared-Big5 risk
// the review flagged: every Big5 double-byte pair is also a valid GB18030
// pair, so the ordered chain alone silently decoded Big5 pages as wrong
// Chinese text. The statistical detector must pick Big5 first (mirroring
// Python find_codec's chardet step) and round-trip the original text.
func TestHTMLParser_DecodesBig5WithoutDeclaration(t *testing.T) {
	src := "<html><body><h1>\u53f0\u7063\u5927\u5b78\u8cc7\u6599\u5eab</h1>" +
		"<p>\u9019\u662f\u4e00\u4efd\u7e41\u9ad4\u4e2d\u6587\u6e2c\u8a66\u6587\u4ef6\uff0c\u7528\u65bc\u9a57\u8b49\u5b57\u5143\u7de8\u78bc\u5075\u6e2c\u3002</p></body></html>"
	assertUndeclaredDecode(t, htmlEncodedFixture(t, src, traditionalchinese.Big5), "big5",
		[]string{"\u53f0\u7063\u5927\u5b78\u8cc7\u6599\u5eab", "\u7e41\u9ad4\u4e2d\u6587"})
}

// TestHTMLParser_DecodesShiftJISWithoutDeclaration covers an undeclared
// Shift-JIS page: the detector must steer the decode to Shift-JIS rather
// than letting GB18030 absorb the bytes as mojibake.
func TestHTMLParser_DecodesShiftJISWithoutDeclaration(t *testing.T) {
	src := "<html><body><h1>\u65e5\u672c\u8a9e\u306e\u30c6\u30b9\u30c8</h1>" +
		"<p>\u3053\u308c\u306f\u30b7\u30d5\u30c8JIS\u3067\u30a8\u30f3\u30b3\u30fc\u30c9\u3055\u308c\u305f\u6587\u66f8\u3067\u3059\u3002\u6771\u4eac\u90fd\u5e81\u306e\u8cc7\u6599\u3002</p></body></html>"
	assertUndeclaredDecode(t, htmlEncodedFixture(t, src, japanese.ShiftJIS), "shift_jis",
		[]string{"\u65e5\u672c\u8a9e\u306e\u30c6\u30b9\u30c8", "\u6771\u4eac\u90fd\u5e81"})
}

// TestHTMLParser_DecodesEUCKRWithoutDeclaration covers an undeclared EUC-KR
// page: detection picks the EUC-KR candidate before the chain order runs.
func TestHTMLParser_DecodesEUCKRWithoutDeclaration(t *testing.T) {
	src := "<html><body><h1>\ud55c\uad6d\uc5b4 \uc2dc\ud5d8</h1>" +
		"<p>\uc774 \ubb38\uc11c\ub294 EUC-KR\ub85c \uc778\ucf54\ub529\ub418\uc5b4 \uc788\uc2b5\ub2c8\ub2e4. \uc11c\uc6b8\ud2b9\ubcc4\uc2dc.</p></body></html>"
	assertUndeclaredDecode(t, htmlEncodedFixture(t, src, korean.EUCKR), "euc-kr",
		[]string{"\ud55c\uad6d\uc5b4 \uc2dc\ud5d8", "\uc11c\uc6b8\ud2b9\ubcc4\uc2dc"})
}

// TestHTMLParser_Latin1TerminalFallback asserts the ISO-8859-1 terminal
// guarantee: an undeclared Western page round-trips through ISO-8859-1
// (either by detection or as the chain's any-byte-decodes terminal), never
// as U+FFFD soup.
func TestHTMLParser_Latin1TerminalFallback(t *testing.T) {
	src := "<html><body><h1>Caf\u00e9 r\u00e9sum\u00e9</h1><p>Na\u00efve fa\u00e7ade \u00e0 gogo.</p></body></html>"
	assertUndeclaredDecode(t, htmlEncodedFixture(t, src, charmap.ISO8859_1), "iso-8859-1",
		[]string{"Caf\u00e9 r\u00e9sum\u00e9", "Na\u00efve fa\u00e7ade"})
}

// TestHTMLParser_DecodesUTF16BOM covers the BOM path of the HTML5 prescan
// (charset.DetermineEncoding): a UTF-16LE document with its BOM decodes even
// though no <meta charset> is declared — a net-new improvement over Python's
// find_codec, which has no BOM handling.
func TestHTMLParser_DecodesUTF16BOM(t *testing.T) {
	src := "<html><body><h1>\u8cb4\u5dde\u8305\u81fa</h1><p>utf16 bom doc</p></body></html>"
	raw, err := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatalf("UTF-16LE encode: %v", err)
	}
	raw = append([]byte{0xff, 0xfe}, raw...)
	res := NewHTMLParser().ParseWithResult(context.Background(), "u16.html", raw)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	got := itemsText(res)
	if !strings.Contains(got, "\u8cb4\u5dde\u8305\u81fa") || !strings.Contains(got, "utf16 bom doc") {
		t.Errorf("UTF-16 BOM decode lost content; got: %q", got)
	}
}

// TestHTMLParser_EmptyInputNoPanic proves the never-worse-than-before claim
// at the degenerate boundary: empty input parses without error.
func TestHTMLParser_EmptyInputNoPanic(t *testing.T) {
	res := NewHTMLParser().ParseWithResult(context.Background(), "empty.html", nil)
	if res.Err != nil {
		t.Fatalf("ParseWithResult(empty): %v", res.Err)
	}
}

// TestDecodeHTMLToUTF8_NeverWorseThanBefore feeds hostile bytes that no
// multibyte candidate decodes cleanly: whatever accepts them (in practice
// the ISO-8859-1 terminal) must produce replacement-free output with a
// non-empty label, and empty input passes through untouched.
func TestDecodeHTMLToUTF8_NeverWorseThanBefore(t *testing.T) {
	out, label := decodeHTMLToUTF8(nil)
	if len(out) != 0 || label != "utf-8" {
		t.Errorf("empty input: got (%d bytes, %q), want (0, utf-8)", len(out), label)
	}
	raw := []byte{0x81, 0x20, 0xfe, 0x21, 0x8f, 0x39, 0x41, 0x7e}
	out, label = decodeHTMLToUTF8(raw)
	if len(out) == 0 || label == "" {
		t.Errorf("hostile bytes: got (%d bytes, %q), want non-empty decode and label", len(out), label)
	}
	if strings.ContainsRune(string(out), '\ufffd') {
		t.Errorf("hostile bytes decoded to replacement runes: %q", out)
	}
}

// TestHTMLParser_DecodesDeclaredWindows1252 pins the declared-windows-1252
// path: DetermineEncoding returns windows-1252 (certain=false) both for a
// real declaration and as its "nothing declared" sentinel, so the declaration
// must be confirmed explicitly. The 0x80-0x9F range (smart quotes, €, ™, —)
// differs from the ISO-8859-1 terminal, which decodes those bytes as C1
// control characters instead of punctuation.
func TestHTMLParser_DecodesDeclaredWindows1252(t *testing.T) {
	src := `<html><head><meta charset="windows-1252"></head><body>` +
		"<h1>\u201cSmart\u201d pricing \u2014 20% off</h1>" +
		"<p>Paste\u2122 caf\u00e9 \u20ac3.50</p></body></html>"
	raw := htmlEncodedFixture(t, src, charmap.Windows1252)
	res := NewHTMLParser().ParseWithResult(context.Background(), "win1252.html", raw)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if enc, _ := res.File["encoding"].(string); enc != "windows-1252" {
		t.Errorf("File.encoding = %q, want windows-1252", enc)
	}
	got := itemsText(res)
	if strings.ContainsRune(got, '\ufffd') {
		t.Errorf("output contains U+FFFD replacement runes: %q", got)
	}
	for _, want := range []string{"\u201cSmart\u201d", "\u2014", "\u2122", "\u20ac"} {
		if !strings.Contains(got, want) {
			t.Errorf("decoded text missing %q; got: %q", want, got)
		}
	}
}

// TestHTMLParser_DecodesDeclaredLatin1AliasAsWindows1252 covers the
// http-equiv pragma form declaring iso-8859-1: WHATWG folds that label (and
// us-ascii) onto windows-1252, so the page must decode with windows-1252
// semantics — smart quotes survive instead of becoming C1 controls.
func TestHTMLParser_DecodesDeclaredLatin1AliasAsWindows1252(t *testing.T) {
	src := `<html><head><meta http-equiv="Content-Type" content="text/html; charset=iso-8859-1"></head><body>` +
		"<p>He said \u201cna\u00efve\u201d \u2014 caf\u00e9 \u2122</p></body></html>"
	raw := htmlEncodedFixture(t, src, charmap.Windows1252)
	res := NewHTMLParser().ParseWithResult(context.Background(), "latin1.html", raw)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if enc, _ := res.File["encoding"].(string); enc != "windows-1252" {
		t.Errorf("File.encoding = %q, want windows-1252", enc)
	}
	if got := itemsText(res); !strings.Contains(got, "\u201cna\u00efve\u201d") {
		t.Errorf("decoded text lost windows-1252 punctuation; got: %q", got)
	}
}
