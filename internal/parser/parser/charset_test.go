package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
)

// TestCharsetEncodingSharedLabelContract locks the single-label contract of
// charsetEncoding: every fallback label the HTML parser may walk (and the
// mail chain ships) resolves through the shared resolver, detector/WHATWG
// spellings fold onto the same encoder, and labels outside the shared
// family (e.g. windows-1252, handled by the HTML prescan layer, or utf-8,
// handled natively by decodeWithCharset) stay out so the two parsers cannot
// drift apart again.
func TestCharsetEncodingSharedLabelContract(t *testing.T) {
	mailLabels := []string{"gb2312", "gbk", "gb18030", "hz-gb-2312", "big5", "shift_jis", "sjis", "euc-kr", "iso-8859-1", "latin1"}
	for _, label := range append(mailLabels, fallbackCharsetLabels...) {
		if _, ok := charsetEncoding(label); !ok {
			t.Errorf("charsetEncoding(%q) = (_, false); the shared resolver must cover every mail-chain and HTML-fallback label", label)
		}
	}

	aliases := map[string]string{
		"GB-18030":   "gb18030",
		"GB2312":     "gb2312",
		"Shift_JIS":  "shift_jis",
		"Shift-JIS":  "shift_jis",
		"EUC-KR":     "euc-kr",
		"ISO-8859-1": "iso-8859-1",
		"ISO8859-1":  "iso-8859-1",
		"Latin1":     "latin1",
		" Big5 ":     "big5",
	}
	for alias, label := range aliases {
		aliasEnc, aliasOK := charsetEncoding(alias)
		labelEnc, labelOK := charsetEncoding(label)
		if !aliasOK || !labelOK || aliasEnc != labelEnc {
			t.Errorf("charsetEncoding(%q) = (%v, %v), charsetEncoding(%q) = (%v, %v); alias spelling must fold onto the same encoder", alias, aliasEnc, aliasOK, label, labelEnc, labelOK)
		}
	}

	for _, label := range []string{"windows-1252", "utf-8", "utf-16", "euc-jp", ""} {
		if _, ok := charsetEncoding(label); ok {
			t.Errorf("charsetEncoding(%q) = (_, true); labels handled elsewhere (prescan / native utf-8) must not enter the shared resolver", label)
		}
	}
}

// TestDecodeFirstCharsetMatchAcceptance pins the shared walker's contract:
// the first candidate whose decode produces no replacement runes wins and
// reports its own label; when every candidate is rejected the walker
// reports failure so callers apply their own terminal fallback. (utf-8 is
// the deterministic rejecting candidate: the Big5 bytes are invalid UTF-8.)
func TestDecodeFirstCharsetMatchAcceptance(t *testing.T) {
	big5Bytes := []byte{0xA4, 0xA4, 0xA4, 0xE5} // "中文" in Big5

	decoded, label, ok := decodeFirstCharsetMatch(big5Bytes, []string{"utf-8", "big5"})
	if !ok || label != "big5" || decoded != "中文" {
		t.Errorf("decodeFirstCharsetMatch(big5) = (%q, %q, %v); want (中文, big5, true)", decoded, label, ok)
	}

	if _, _, ok := decodeFirstCharsetMatch(big5Bytes, []string{"utf-8"}); ok {
		t.Error("decodeFirstCharsetMatch over a rejecting candidate list must report failure")
	}
}

func TestDecodeToUTF8(t *testing.T) {
	// 1. Empty input passes through
	out, label := DecodeToUTF8(nil, "")
	if len(out) != 0 || label != "utf-8" {
		t.Errorf("empty input: got (%d, %q), want (0, utf-8)", len(out), label)
	}

	// 2. Already UTF-8 passes through without alteration
	utf8Str := "你好，世界！This is valid UTF-8 text."
	out, label = DecodeToUTF8([]byte(utf8Str), "")
	if string(out) != utf8Str || label != "utf-8" {
		t.Errorf("UTF-8 input: got %q (%q), want %q (utf-8)", string(out), label, utf8Str)
	}

	// 3. GBK / GB2312 text
	chinese := "自然语言处理与知识图谱构建"
	gbkBytes, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(chinese))
	if err != nil {
		t.Fatalf("GBK encode: %v", err)
	}
	out, label = DecodeToUTF8(gbkBytes, "text/plain")
	if string(out) != chinese {
		t.Errorf("GBK decode: got %q, want %q", string(out), chinese)
	}
	if label != "gb18030" && label != "gbk" && label != "gb2312" {
		t.Errorf("GBK label: got %q, want gb18030/gbk/gb2312", label)
	}

	// 4. Big5 text
	traditional := "繁體中文測試資料"
	big5Bytes, err := traditionalchinese.Big5.NewEncoder().Bytes([]byte(traditional))
	if err != nil {
		t.Fatalf("Big5 encode: %v", err)
	}
	out, label = DecodeToUTF8(big5Bytes, "")
	if string(out) != traditional {
		t.Errorf("Big5 decode: got %q, want %q", string(out), traditional)
	}

	// 5. Shift-JIS Japanese text
	japaneseText := "日本語テキストのテストです"
	sjisBytes, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(japaneseText))
	if err != nil {
		t.Fatalf("Shift-JIS encode: %v", err)
	}
	out, _ = DecodeToUTF8(sjisBytes, "")
	if string(out) != japaneseText {
		t.Errorf("Shift-JIS decode: got %q, want %q", string(out), japaneseText)
	}

	// 6. EUC-KR Korean text
	koreanText := "한국어 시험: 이 문서는 EUC-KR로 인코딩되어 있습니다. 서울특별시."
	euckrBytes, err := korean.EUCKR.NewEncoder().Bytes([]byte(koreanText))
	if err != nil {
		t.Fatalf("EUC-KR encode: %v", err)
	}
	out, _ = DecodeToUTF8(euckrBytes, "")
	if string(out) != koreanText {
		t.Errorf("EUC-KR decode: got %q, want %q", string(out), koreanText)
	}

	// 7. XML declaration check
	xmlHeader := `<?xml version="1.0" encoding="GB2312"?><html><body><p>章节测试</p></body></html>`
	xmlGBK, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(xmlHeader))
	if err != nil {
		t.Fatalf("XML GBK encode: %v", err)
	}
	out, label = DecodeToUTF8(xmlGBK, "application/xhtml+xml")
	if !strings.Contains(string(out), "章节测试") {
		t.Errorf("XML declaration decode: missing expected text in %q", string(out))
	}
	if label != "gb2312" && label != "gb18030" && label != "gbk" {
		t.Errorf("XML declaration label: got %q", label)
	}
}

func TestCSVParser_GBK(t *testing.T) {
	csvText := "产品,分类,价格,备注\n笔记本电脑,电子,6999.00,\"特价销售，送鼠标\"\n无线耳机,配件,399.00,\"热销\"\n"
	gbkBytes, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(csvText))
	if err != nil {
		t.Fatalf("GBK encode: %v", err)
	}

	p := NewCSVParser()
	res := p.ParseWithResult(context.Background(), "products.csv", gbkBytes)
	if res.Err != nil {
		t.Fatalf("CSVParser.ParseWithResult failed: %v", res.Err)
	}
	if strings.ContainsRune(res.HTML, '\ufffd') {
		t.Errorf("CSV HTML output contains replacement runes: %s", res.HTML)
	}
	for _, want := range []string{"笔记本电脑", "电子", "特价销售，送鼠标", "无线耳机"} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("CSV HTML missing expected substring %q; got: %s", want, res.HTML)
		}
	}
	if enc, _ := res.File["encoding"].(string); enc != "gb18030" && enc != "gbk" && enc != "gb2312" {
		t.Errorf("CSV File.encoding = %q, want gbk/gb2312/gb18030", enc)
	}
}

func TestEPUBParser_GB2312(t *testing.T) {
	// Create an in-memory EPUB zip with GB2312-encoded chapter XHTML
	chapterContent := `<?xml version="1.0" encoding="GB2312"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>第一章</title></head>
<body><h1>第一章 破晓</h1><p>这是一段包含中文的EPUB正文内容。</p></body>
</html>`
	chapterGBK, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(chapterContent))
	if err != nil {
		t.Fatalf("GBK encode: %v", err)
	}

	containerXML := `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	contentOPF := `<?xml version="1.0" encoding="utf-8"?><package xmlns="http://www.idpf.org/2007/opf" unique-identifier="BookID" version="2.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>测试</dc:title></metadata><manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c1"/></spine></package>`

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("META-INF/container.xml")
	w.Write([]byte(containerXML))
	w, _ = zw.Create("OEBPS/content.opf")
	w.Write([]byte(contentOPF))
	w, _ = zw.Create("OEBPS/c1.xhtml")
	w.Write(chapterGBK)
	zw.Close()

	p := NewEPUBParser()
	res := p.ParseWithResult(context.Background(), "book.epub", buf.Bytes())
	if res.Err != nil {
		t.Fatalf("EPUBParser.ParseWithResult failed: %v", res.Err)
	}
	if len(res.JSON) == 0 {
		t.Fatal("want non-empty JSON items from EPUB")
	}
	text, _ := res.JSON[0]["text"].(string)
	if strings.ContainsRune(text, '\ufffd') {
		t.Errorf("EPUB text contains replacement runes: %s", text)
	}
	for _, want := range []string{"第一章 破晓", "这是一段包含中文的EPUB正文内容。"} {
		if !strings.Contains(text, want) {
			t.Errorf("EPUB text missing %q; got: %s", want, text)
		}
	}
	if enc, _ := res.File["encoding"].(string); enc != "gb2312" && enc != "gbk" && enc != "gb18030" {
		t.Errorf("EPUB File.encoding = %q, want gb2312/gbk/gb18030", enc)
	}
}

func TestDecodeToUTF8_HZGB2312Hint(t *testing.T) {
	// HZ-GB-2312 escape sequences (~{...~}) are pure 7-bit ASCII, hence valid UTF-8.
	// An explicit hint must decode via HZ-GB-2312 rather than returning bytes untouched.
	hz := []byte("~{<4Ky~}")
	out, label := DecodeToUTF8(hz, "hz-gb-2312")
	if label != "hzgb2312" && label != "hz-gb-2312" {
		t.Errorf("got label %q, want hzgb2312 or hz-gb-2312", label)
	}
	if string(out) == string(hz) {
		t.Fatal("hint ignored; utf8 fast-path swallowed HZ escapes")
	}
	if !utf8.Valid(out) {
		t.Fatal("decoded HZ output must be valid UTF-8")
	}
}

func TestDecodeToUTF8_XMLEncoding_HZGB2312(t *testing.T) {
	// XML encoding declaration for HZ-GB-2312 must be evaluated before the UTF-8 fast path.
	xml := []byte("<?xml version=\"1.0\" encoding=\"HZ-GB-2312\"?><html><body>~{<4Ky~}</body></html>")
	out, label := DecodeToUTF8(xml, "application/xhtml+xml")
	if label != "hzgb2312" && label != "hz-gb-2312" {
		t.Errorf("got label %q, want hzgb2312", label)
	}
	if string(out) == string(xml) {
		t.Fatal("XML HZ-GB-2312 declaration ignored; utf8 fast-path swallowed HZ escapes")
	}
}

func TestDecodeToUTF8_UnrecognizedHint_UTF8Payload(t *testing.T) {
	// Unrecognized charset labels (e.g. unknown-8bit from MIME) must not trigger
	// a bogus latin-1 decode or corrupt valid UTF-8 input.
	utf8Text := []byte("这是一段标准的UTF-8中文内容")
	out, label := DecodeToUTF8(utf8Text, "unknown-8bit")
	if label != "utf-8" {
		t.Errorf("got label %q, want utf-8", label)
	}
	if string(out) != string(utf8Text) {
		t.Errorf("unrecognized hint corrupted UTF-8 text; got %q, want %q", string(out), string(utf8Text))
	}
}

func TestDecodeToUTF8_BOMOverridesConflictingXMLDeclaration(t *testing.T) {
	// RFC 7303: UTF-8 BOM (\xef\xbb\xbf) takes precedence over a conflicting XML declaration (e.g. ISO-8859-1).
	conflictXML := append([]byte("\xef\xbb\xbf"), []byte(`<?xml version="1.0" encoding="ISO-8859-1"?><html><body>中文测试内容</body></html>`)...)
	out, label := DecodeToUTF8(conflictXML, "application/xhtml+xml")
	if label != "utf-8" {
		t.Errorf("got label %q, want utf-8", label)
	}
	if !strings.Contains(string(out), "中文测试内容") {
		t.Fatalf("UTF-8 BOM was overridden by XML ISO-8859-1; got: %s", string(out))
	}
}

func TestDecodeToUTF8_UTF16LEBOM(t *testing.T) {
	// UTF-16LE with BOM (\xff\xfe) must decode to valid UTF-8.
	src := "UTF-16LE 中文测试"
	utf16Bytes, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes([]byte(src))
	if err != nil {
		t.Fatalf("UTF-16 encode: %v", err)
	}
	out, label := DecodeToUTF8(utf16Bytes, "text/plain")
	if label != "utf-16le" {
		t.Errorf("got label %q, want utf-16le", label)
	}
	if !strings.Contains(string(out), src) {
		t.Errorf("got %q, want containing %q", string(out), src)
	}
}
