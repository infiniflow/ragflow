package parser

import "testing"

// TestCharsetEncodingSharedLabelContract locks the single-label contract of
// charsetEncoding: every fallback label the HTML parser may walk (and the
// mail chain ships) resolves through the shared resolver, detector/WHATWG
// spellings fold onto the same encoder, and labels outside the shared
// family (e.g. windows-1252, handled by the HTML prescan layer, or utf-8,
// handled natively by decodeWithCharset) stay out so the two parsers cannot
// drift apart again.
func TestCharsetEncodingSharedLabelContract(t *testing.T) {
	mailLabels := []string{"gb2312", "gbk", "gb18030", "hz-gb-2312", "big5", "shift_jis", "sjis", "euc-kr", "iso-8859-1", "latin1"}
	for _, label := range append(mailLabels, htmlCharsetFallbackLabels...) {
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
