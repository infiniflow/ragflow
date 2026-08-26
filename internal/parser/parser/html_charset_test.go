package parser

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
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
	if !utf8Valid([]byte(src)) {
		t.Fatal("test source must be valid UTF-8 before re-encoding")
	}
	out, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatalf("GBK encode: %v", err)
	}
	if utf8Valid(out) {
		t.Fatal("GBK fixture unexpectedly decodes as UTF-8; fixture is ineffective")
	}
	return out
}

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
	if utf8Valid(raw) {
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
