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
//

package parser

// Shared charset-label resolution and decode-acceptance helpers. Both
// email_parser.go (RFC 2047 headers, mail bodies) and html_parser.go
// (decodeHTMLToUTF8) build on these, so a label resolves to the same
// decoder everywhere and the "first candidate that decodes without
// replacement runes" walk exists exactly once.

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/saintfish/chardet"
	htmlcharset "golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

// canonicalCharsetLabel canonicalizes a charset label for comparison and
// lookup: ASCII lower-casing with surrounding whitespace plus '-' and '_'
// removed, so detector outputs and WHATWG spellings such as "GB-18030",
// "Shift_JIS" or "EUC-KR" fold onto one canonical form.
func canonicalCharsetLabel(label string) string {
	return strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(label)))
}

// charsetEncoding maps a charset label to its decoder, covering the
// charsets common in real-world mail and legacy HTML beyond utf-8: the
// Chinese families (gb2312/gbk/gb18030, big5), shift_jis, euc-kr, and
// latin-1. It is the single label resolver shared by the RFC 2047 header
// decoder, decodeMailPayload (mail bodies), and decodeHTMLToUTF8 (HTML
// bytes) so every caller resolves a charset label identically.
//
// The "gb2312" label maps to GBK, not HZGB2312: mail labeled gb2312 carries
// plain 8-bit GB2312 bytes, while HZGB2312 only decodes the 7-bit HZ escape
// form used under the distinct "hz-gb-2312" label.
func charsetEncoding(charset string) (encoding.Encoding, bool) {
	switch canonicalCharsetLabel(charset) {
	case "gb2312", "gbk":
		return simplifiedchinese.GBK, true
	case "gb18030":
		return simplifiedchinese.GB18030, true
	case "hzgb2312":
		return simplifiedchinese.HZGB2312, true
	case "big5":
		return traditionalchinese.Big5, true
	case "shiftjis", "sjis":
		return japanese.ShiftJIS, true
	case "euckr":
		return korean.EUCKR, true
	case "iso88591", "latin1":
		return charmap.ISO8859_1, true
	}
	return nil, false
}

// decodeWithCharset decodes payload under one charset label, rejecting the
// result when it produces replacement runes (U+FFFD). An empty label is
// treated as utf-8.
func decodeWithCharset(payload []byte, charset string) (string, error) {
	switch canonicalCharsetLabel(charset) {
	case "utf8", "":
		s := string(payload)
		if !strings.ContainsRune(s, '\ufffd') {
			return s, nil
		}
		return "", fmt.Errorf("invalid utf-8")
	}
	if enc, ok := charsetEncoding(charset); ok {
		return decodeTransform(payload, enc.NewDecoder())
	}
	if enc, _ := htmlcharset.Lookup(charset); enc != nil {
		return decodeTransform(payload, enc.NewDecoder())
	}
	return "", fmt.Errorf("unsupported charset: %s", charset)
}

// isRecognizedCharset reports whether charset maps to a known decoder
// either via our shared charsetEncoding resolver or htmlcharset.Lookup.
func isRecognizedCharset(charset string) bool {
	switch canonicalCharsetLabel(charset) {
	case "utf8", "":
		return true
	}
	if _, ok := charsetEncoding(charset); ok {
		return true
	}
	if enc, _ := htmlcharset.Lookup(charset); enc != nil {
		return true
	}
	return false
}

// decodeTransform decodes payload through decoder, rejecting results that
// contain replacement runes.
func decodeTransform(payload []byte, decoder *encoding.Decoder) (string, error) {
	reader := transform.NewReader(bytes.NewReader(payload), decoder)
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	if !strings.ContainsRune(string(decoded), '\ufffd') {
		return string(decoded), nil
	}
	return "", fmt.Errorf("decode produced replacement characters")
}

// decodeFirstCharsetMatch walks the candidate labels in order and returns
// the first decode that succeeds without replacement runes, along with the
// label that produced it. It is the shared fallback-chain walker behind
// both decodeMailPayload and decodeHTMLToUTF8.
func decodeFirstCharsetMatch(data []byte, labels []string) (string, string, bool) {
	for _, label := range labels {
		if decoded, err := decodeWithCharset(data, label); err == nil {
			return decoded, label, true
		}
	}
	return "", "", false
}

// fallbackCharsetLabels is the ordered brute-force decode loop for bytes
// that are neither valid UTF-8 nor carrying a BOM / explicit charset declaration.
// GB18030 (the GBK superset) leads for CJK documents, followed by Big5, Shift-JIS,
// EUC-KR, and terminal ISO-8859-1.
var fallbackCharsetLabels = []string{"gb18030", "big5", "shift_jis", "euc-kr", "iso-8859-1"}

// charsetDetectConfidence is the minimum statistical-detection confidence
// (on chardet's 0-100 scale) allowed to override the fallback chain order.
const charsetDetectConfidence = 90

func detectedCharset(data []byte) string {
	sample := data
	if len(sample) > 1024 {
		sample = sample[:1024]
	}
	res, err := chardet.NewTextDetector().DetectBest(sample)
	if err != nil || res == nil || res.Charset == "" || res.Confidence < charsetDetectConfidence {
		return ""
	}
	detected := canonicalCharsetLabel(res.Charset)
	// The detector reports the GBK family as GB2312 / GBK; GB18030 is the
	// superset we ship, so both map onto the gb18030 candidate.
	if detected == "gb2312" || detected == "gbk" {
		detected = "gb18030"
	}
	for _, label := range fallbackCharsetLabels {
		if canonicalCharsetLabel(label) == detected {
			return label
		}
	}
	return ""
}

var (
	htmlMetaCharsetRe = regexp.MustCompile(`(?i)<meta[^>]*charset\s*=\s*["']?\s*([a-z0-9._+\-]+)`)
	xmlEncodingRe     = regexp.MustCompile(`(?i)<\?xml[^>]*encoding\s*=\s*["']?\s*([a-z0-9._+\-]+)`)
)

func declaresWindows1252(data []byte) bool {
	if len(data) > 1024 {
		data = data[:1024]
	}
	for _, m := range htmlMetaCharsetRe.FindAllSubmatch(data, -1) {
		if _, name := htmlcharset.Lookup(string(m[1])); name == "windows-1252" {
			return true
		}
	}
	for _, m := range xmlEncodingRe.FindAllSubmatch(data, -1) {
		if _, name := htmlcharset.Lookup(string(m[1])); name == "windows-1252" {
			return true
		}
	}
	return false
}

func declaredXMLEncoding(data []byte) string {
	sample := data
	if len(sample) > 1024 {
		sample = sample[:1024]
	}
	m := xmlEncodingRe.FindSubmatch(sample)
	if len(m) > 1 {
		return string(m[1])
	}
	return ""
}

// extractCharsetHint extracts any explicit charset label from hint,
// whether hint is a bare charset ("gb2312") or a media type parameter ("text/plain; charset=gb2312").
func extractCharsetHint(hint string) string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return ""
	}
	if !strings.Contains(hint, "/") {
		return hint
	}
	if idx := strings.Index(strings.ToLower(hint), "charset="); idx != -1 {
		cs := hint[idx+len("charset="):]
		cs = strings.Trim(cs, `"' `)
		if end := strings.IndexAny(cs, "; \t\r\n"); end != -1 {
			cs = cs[:end]
		}
		return cs
	}
	return ""
}

// DecodeToUTF8 converts arbitrary data bytes into UTF-8.
// If data is already valid UTF-8, it returns data untouched with encoding "utf-8".
// hint can be:
//   - An explicit charset label (e.g. "gbk", "gb2312", "big5", from MIME headers)
//   - A MIME Content-Type (e.g. "text/html", "application/xhtml+xml", "text/csv", "text/plain")
//   - Empty string ""
//
// It returns the decoded UTF-8 bytes and the canonical encoding label used.
func DecodeToUTF8(data []byte, hint string) ([]byte, string) {
	if len(data) == 0 {
		return data, "utf-8"
	}

	// 1. Explicit charset declaration from hint (e.g. email MIME headers).
	// If the caller explicitly declared a non-UTF8 charset, prioritize trying it first.
	// This correctly handles 7-bit ASCII escape encodings like HZ-GB-2312, where bytes are
	// valid ASCII (thus valid UTF-8) but represent encoded Chinese escape sequences.
	// We verify the charset is recognized so unrecognized labels (e.g. "unknown-8bit")
	// do not corrupt UTF-8 text or bypass validation.
	if cs := extractCharsetHint(hint); cs != "" && canonicalCharsetLabel(cs) != "utf8" && isRecognizedCharset(cs) {
		if decoded, err := decodeWithCharset(data, cs); err == nil {
			return []byte(decoded), canonicalCharsetLabel(cs)
		}
	}

	// 2. BOM prescan (Byte Order Mark).
	// RFC 7303 §3.2 and XML specs define BOM as the authoritative physical encoding signature
	// that overrides conflicting in-band text declarations (e.g. UTF-8 BOM with <?xml encoding="ISO-8859-1"?>).
	// When contentType is "", DetermineEncoding reports certain=true strictly when a physical BOM is present.
	if enc, name, certain := htmlcharset.DetermineEncoding(data, ""); certain && enc != nil {
		if decoded, err := decodeTransform(data, enc.NewDecoder()); err == nil {
			return []byte(decoded), name
		}
	}

	// 3. XML declaration check (e.g. <?xml version="1.0" encoding="GB2312"?> in EPUB XHTML).
	// Evaluated before the UTF-8 fast-path so that 7-bit ASCII-compatible escape encodings
	// like HZ-GB-2312 declared in XML are decoded rather than treated as valid ASCII UTF-8.
	if xmlEnc := declaredXMLEncoding(data); xmlEnc != "" && canonicalCharsetLabel(xmlEnc) != "utf8" && isRecognizedCharset(xmlEnc) {
		if decoded, err := decodeWithCharset(data, xmlEnc); err == nil {
			return []byte(decoded), canonicalCharsetLabel(xmlEnc)
		}
	}

	// 4. Fast-path: already valid UTF-8.
	if utf8.Valid(data) {
		return data, "utf-8"
	}

	// 5. Document prescan via DetermineEncoding (handles HTML meta charset, and Content-Type)
	contentType := ""
	if strings.Contains(hint, "/") {
		contentType = hint
	}
	if enc, name, _ := htmlcharset.DetermineEncoding(data, contentType); enc != nil &&
		(name != "windows-1252" || declaresWindows1252(data)) {
		if decoded, err := decodeTransform(data, enc.NewDecoder()); err == nil {
			return []byte(decoded), name
		}
	}

	// 6. Statistical detection with chardet (confidence >= 90)
	if label := detectedCharset(data); label != "" {
		if decoded, err := decodeWithCharset(data, label); err == nil {
			return []byte(decoded), label
		}
	}

	// 7. Fallback chain: gb18030 -> big5 -> shift_jis -> euc-kr -> iso-8859-1
	if decoded, label, ok := decodeFirstCharsetMatch(data, fallbackCharsetLabels); ok {
		return []byte(decoded), label
	}

	// Defensive fallback: unreachable for non-empty input as terminal ISO-8859-1
	// accepts every byte sequence without producing replacement runes.
	return data, ""
}
