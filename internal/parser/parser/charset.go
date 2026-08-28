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
	"strings"

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
	// Unknown charset: treat as latin-1.
	runes := make([]rune, len(payload))
	for i, b := range payload {
		runes[i] = rune(b)
	}
	return string(runes), nil
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
