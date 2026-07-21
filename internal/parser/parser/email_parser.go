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

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"path/filepath"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// EmailParser parses .eml (RFC 5322 email) files into structured
// JSON or plain-text output. Mirrors Python's _email() method in
// rag/flow/parser/parser.py.
//
// .msg (Outlook) files are not supported in the Go path; callers
// receive a clear error.
type EmailParser struct {
	fields       []string
	outputFormat string
}

func NewEmailParser() *EmailParser {
	return &EmailParser{}
}

func (p *EmailParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if v, ok := setup["output_format"].(string); ok && v != "" {
		p.outputFormat = v
	}
	if raw, ok := setup["fields"]; ok {
		switch list := raw.(type) {
		case []string:
			p.fields = list
		case []any:
			p.fields = make([]string, 0, len(list))
			for _, item := range list {
				if s, ok := item.(string); ok {
					p.fields = append(p.fields, s)
				}
			}
		}
	}
	if len(p.fields) == 0 {
		p.fields = []string{"from", "to", "cc", "bcc", "date", "subject", "body", "attachments", "metadata"}
	}
}

func (p *EmailParser) ParseWithResult(filename string, data []byte) ParseResult {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".msg" {
		return ParseResult{
			Err: fmt.Errorf("email: .msg (Outlook) files are not supported in the Go parser; use .eml format"),
		}
	}

	emailContent := parseEML(bytes.NewReader(data), p.fields)

	outputFormat := p.outputFormat
	if outputFormat == "" {
		outputFormat = "text"
	}

	if outputFormat == "json" {
		emailContent["doc_type_kwd"] = "text"
		return ParseResult{
			OutputFormat: "json",
			File:         map[string]any{"name": filename},
			JSON:         []map[string]any{emailContent},
		}
	}

	// Text output: flatten fields into a single string.
	var sb strings.Builder
	for k, v := range emailContent {
		switch val := v.(type) {
		case string:
			sb.WriteString(k)
			sb.WriteString(":")
			sb.WriteString(val)
			sb.WriteString("\n")
		case map[string]any:
			sb.WriteString(k)
			sb.WriteString(":{")
			for mk, mv := range val {
				if ms, ok := mv.(string); ok {
					sb.WriteString(mk)
					sb.WriteString(":")
					sb.WriteString(ms)
					sb.WriteString(", ")
				}
			}
			sb.WriteString("}\n")
		case []map[string]any:
			for _, att := range val {
				fn, _ := att["filename"].(string)
				pl, _ := att["payload"].(string)
				sb.WriteString(fn)
				sb.WriteString(":")
				sb.WriteString(pl)
				sb.WriteString("\n")
			}
		case []string:
			sb.WriteString(strings.Join(val, "\n"))
		}
	}
	return ParseResult{
		OutputFormat: "text",
		File:         map[string]any{"name": filename},
		Text:         sb.String(),
	}
}

// -- field set helpers --

func targetFieldsSet(fields []string) map[string]bool {
	m := make(map[string]bool, len(fields))
	for _, f := range fields {
		m[strings.ToLower(strings.TrimSpace(f))] = true
	}
	return m
}

// -- .eml parsing (RFC 5322 with multipart support) --

func parseEML(r io.Reader, fields []string) map[string]any {
	target := targetFieldsSet(fields)
	content := map[string]any{}

	msg, err := mail.ReadMessage(r)
	if err != nil {
		content["error"] = fmt.Sprintf("email: parse error: %v", err)
		return content
	}

	// Headers.
	meta := map[string]any{}
	for key, vals := range msg.Header {
		keyLower := strings.ToLower(key)
		val := strings.Join(vals, ", ")
		switch keyLower {
		case "from", "to", "cc", "bcc", "date", "subject":
			if target[keyLower] {
				content[keyLower] = val
			}
		default:
			meta[keyLower] = val
		}
	}
	if target["metadata"] {
		content["metadata"] = meta
	}

	// Body and attachments — readMailBody walks all multipart parts
	// and collects text/html bodies alongside attachment parts whose
	// Content-Disposition starts with "attachment".
	needAttachments := target["attachments"]
	if target["body"] {
		contentType := msg.Header.Get("Content-Type")
		bodyText, bodyHTML, attachments := readMailBody(msg.Body, contentType, needAttachments)
		if bodyText != "" {
			content["text"] = bodyText
		}
		if bodyHTML != "" {
			content["text_html"] = bodyHTML
		}
		if needAttachments {
			content["attachments"] = attachments
		}
	} else if needAttachments {
		content["attachments"] = []map[string]any{}
	}

	return content
}

// readMailBody reads the body of an email message, handling
// multipart/alternative, multipart/mixed, and single-part content
// types. Returns (textBody, htmlBody, attachments).
// When collectAttachments is true, non-text parts with Content-Disposition
// starting with "attachment" are collected.
func readMailBody(body io.Reader, contentType string, collectAttachments bool) (string, string, []map[string]any) {
	var attachments []map[string]any

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain"
	}

	if !strings.HasPrefix(mediaType, "multipart/") {
		raw, _ := io.ReadAll(body)
		decoded := decodeMailPayload(raw, params["charset"])
		if mediaType == "text/html" {
			return "", decoded, attachments
		}
		return decoded, "", attachments
	}

	boundary := params["boundary"]
	if boundary == "" {
		raw, _ := io.ReadAll(body)
		return decodeMailPayload(raw, ""), "", attachments
	}

	mr := multipart.NewReader(body, boundary)
	var textParts, htmlParts []string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		partCT := part.Header.Get("Content-Type")
		partMedia, partParams, _ := mime.ParseMediaType(partCT)

		if strings.HasPrefix(partMedia, "multipart/") {
			t, h, nestedAttachments := readMailBody(part, partCT, collectAttachments)
			if t != "" {
				textParts = append(textParts, t)
			}
			if h != "" {
				htmlParts = append(htmlParts, h)
			}
			attachments = append(attachments, nestedAttachments...)
			continue
		}

		// Check if this part is an attachment.
		if collectAttachments && isAttachmentPart(part) {
			raw, _ := io.ReadAll(part)
			raw = decodeCTE(raw, part.Header.Get("Content-Transfer-Encoding"))

			attachments = append(attachments, map[string]any{
				"filename": attachmentFilename(part),
				"payload":  decodeMailPayload(raw, partParams["charset"]),
			})
			continue
		}

		raw, _ := io.ReadAll(part)
		raw = decodeCTE(raw, part.Header.Get("Content-Transfer-Encoding"))
		decoded := decodeMailPayload(raw, partParams["charset"])

		switch partMedia {
		case "text/plain":
			textParts = append(textParts, decoded)
		case "text/html":
			htmlParts = append(htmlParts, decoded)
		}
	}
	return strings.Join(textParts, "\n"), strings.Join(htmlParts, "\n"), attachments
}

// isAttachmentPart checks whether a multipart part should be treated as
// an attachment (Content-Disposition starts with "attachment"). Mirrors
// Python's check in _email().
// decodeCTE decodes Content-Transfer-Encoding (base64, quoted-printable, etc.).
// Mirrors Python part.get_payload(decode=True).
func decodeCTE(raw []byte, cte string) []byte {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "base64":
		d, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return raw
		}
		return d
	case "quoted-printable":
		r := quotedprintable.NewReader(bytes.NewReader(raw))
		d, err := io.ReadAll(r)
		if err != nil {
			return raw
		}
		return d
	default:
		return raw
	}
}

func isAttachmentPart(part *multipart.Part) bool {
	disp := part.Header.Get("Content-Disposition")
	if disp == "" {
		return false
	}
	dispType, _, err := mime.ParseMediaType(disp)
	if err != nil {
		return false
	}
	return strings.EqualFold(dispType, "attachment")
}

// attachmentFilename extracts a filename from the part's
// Content-Disposition or Content-Type headers.
func attachmentFilename(part *multipart.Part) string {
	if fn := part.FileName(); fn != "" {
		return fn
	}
	ct := part.Header.Get("Content-Type")
	if ct != "" {
		_, params, _ := mime.ParseMediaType(ct)
		if name, ok := params["name"]; ok {
			return name
		}
	}
	return "attachment.bin"
}

// decodeMailPayload attempts multiple charset decodings.
// Mirrors Python's _decode_payload with fallback chain:
// utf-8 → gb2312 → gbk → gb18030 → latin1 → utf-8 (ignore).
func decodeMailPayload(payload []byte, charset string) string {
	if len(payload) == 0 {
		return ""
	}
	charsets := buildCharsetChain(charset)
	for _, enc := range charsets {
		if enc == "" {
			// raw bytes → already fallthrough
			return string(payload)
		}
		decoded, err := decodeWithCharset(payload, enc)
		if err == nil {
			return decoded
		}
	}
	return string(payload)
}

func buildCharsetChain(declared string) []string {
	chain := make([]string, 0, 7)
	if declared != "" {
		chain = append(chain, declared)
	}
	chain = append(chain, "utf-8", "gb2312", "gbk", "gb18030", "latin1")
	return chain
}

func decodeWithCharset(payload []byte, charset string) (string, error) {
	charset = strings.ToLower(strings.TrimSpace(charset))
	switch charset {
	case "utf-8", "utf8", "":
		s := string(payload)
		if !strings.ContainsRune(s, '\ufffd') {
			return s, nil
		}
		return "", fmt.Errorf("invalid utf-8")
	case "latin1", "iso-8859-1", "iso8859-1":
		runes := make([]rune, len(payload))
		for i, b := range payload {
			runes[i] = rune(b)
		}
		return string(runes), nil
	case "gb2312":
		return decodeTransform(payload, simplifiedchinese.HZGB2312.NewDecoder())
	case "gbk":
		return decodeTransform(payload, simplifiedchinese.GBK.NewDecoder())
	case "gb18030":
		return decodeTransform(payload, simplifiedchinese.GB18030.NewDecoder())
	}
	// Unknown charset: treat as latin-1.
	runes := make([]rune, len(payload))
	for i, b := range payload {
		runes[i] = rune(b)
	}
	return string(runes), nil
}

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
