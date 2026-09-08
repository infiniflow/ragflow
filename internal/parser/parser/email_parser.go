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
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"path/filepath"
	"strings"
	"time"

	"github.com/AkmalOt/gomsg"
	"golang.org/x/net/html"
	"golang.org/x/text/transform"

	"ragflow/internal/utility"
)

// EmailParser parses .eml (RFC 5322 email) and .msg (Outlook OLE2) files
// into structured JSON or plain-text output. Mirrors Python's _email()
// method in rag/flow/parser/parser.py, whose .msg branch uses extract_msg.
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

func (p *EmailParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	return p.parseEmail(ctx, filename, data, 0)
}

// parseEmail is ParseWithResult with a re-chunk depth so nested email
// attachments are parsed (and their body made retrievable) without unbounded
// recursion into attachments-of-attachments.
func (p *EmailParser) parseEmail(ctx context.Context, filename string, data []byte, depth int) ParseResult {
	ext := strings.ToLower(filepath.Ext(filename))

	var content map[string]any
	if ext == ".msg" {
		var (
			msg map[string]any
			err error
		)
		// gomsg.Decode parses untrusted OLE2/CFB input; guard against a
		// panic from a malformed .msg so one bad email can't take down the
		// ingestion worker.
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Log so a genuine bug (e.g. a nil deref in parseMSG)
					// is not silently masked as a "decode panicked" error.
					log.Printf("email: .msg decode panicked for %q; skipping: %v", filename, r)
					err = fmt.Errorf("email: .msg decode panicked: %v", r)
				}
			}()
			msg, err = parseMSG(data, p.fields)
		}()
		if err != nil {
			return ParseResult{Err: fmt.Errorf("email: .msg: %w", err)}
		}
		content = msg
	} else {
		content = parseEML(bytes.NewReader(data), p.fields)
	}

	outputFormat := p.outputFormat
	if outputFormat == "" {
		outputFormat = "text"
	}

	// Re-chunk attachments so their content becomes retrievable within the
	// same document (user-oriented; mirrors Python legacy rag/app/email.py
	// naive_chunk). Each attachment is re-parsed by its file extension via
	// the shared parser registry; a single unparseable/skipped attachment
	// never breaks the whole email.
	extraItems, attachmentText := p.rechunkEmailAttachments(ctx, content, depth)

	// attachments has been consumed by rechunkEmailAttachments (which
	// re-parses each attachment by extension to make its content
	// retrievable). It is otherwise dead weight: jsonItemsToPages copies
	// every key into a schema.Page, but buildPagesFromBytes keeps only
	// text+doc_type_kwd, so carrying the full attachment payloads through to
	// the chunker would only bloat the intermediate pages before being
	// discarded. Drop it from the result content.
	delete(content, "attachments")

	if outputFormat == "json" {
		content["doc_type_kwd"] = "text"
		items := []map[string]any{content}
		items = append(items, extraItems...)
		return ParseResult{
			OutputFormat: "json",
			File:         map[string]any{"name": filename},
			JSON:         items,
		}
	}

	// Text output: flatten fields into a single string.
	var sb strings.Builder
	for k, v := range content {
		// The metadata map (every non-basic header: Received chains,
		// DKIM/ARC signatures, …) stays available in the JSON output, but
		// flattening it into chunkable text buries the message under
		// transport noise, so it is excluded from the text output.
		if k == "metadata" {
			continue
		}
		switch val := v.(type) {
		case string:
			if k == "text_html" {
				// Text output feeds the chunker directly; emit the
				// visible text of the HTML part instead of raw markup,
				// which otherwise lands in chunks as tag soup.
				val = htmlBodyToText(val)
			}
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
		case []string:
			sb.WriteString(strings.Join(val, "\n"))
		}
	}
	// Attachment text (re-parsed by extension) replaces the old crude
	// "filename:payload" flatten, so binary attachments no longer leak
	// mojibake into the searchable (indexed) text. The raw attachment
	// payloads themselves are dropped after rechunk (see parseEmail), so the
	// JSON output path carries only the re-parsed attachment text, never the
	// original payload bytes.
	if attachmentText != "" {
		sb.WriteString(attachmentText)
		sb.WriteString("\n")
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

// -- header decoding (RFC 2047) --

// rfc2047Decoder decodes RFC 2047 encoded-words (e.g. "=?utf-8?B?...?=") in
// header values. utf-8 is handled natively by mime; the CharsetReader covers
// the non-UTF-8 charsets via charsetEncoding.
var rfc2047Decoder = &mime.WordDecoder{
	CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
		if enc, ok := charsetEncoding(charset); ok {
			return transform.NewReader(input, enc.NewDecoder()), nil
		}
		return nil, fmt.Errorf("email: unsupported header charset %q", charset)
	},
}

// decodeHeaderWord decodes any RFC 2047 encoded-words in a header value,
// leaving undecodable values untouched.
func decodeHeaderWord(val string) string {
	decoded, err := rfc2047Decoder.DecodeHeader(val)
	if err != nil {
		return val
	}
	return decoded
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

	// Headers. net/mail does not decode RFC 2047 encoded-words, so decode
	// each value explicitly; otherwise a non-ASCII subject/from arrives as
	// an unreadable "=?utf-8?B?...?=" blob that chunk delimiters then shred
	// at the "?" characters.
	meta := map[string]any{}
	for key, vals := range msg.Header {
		keyLower := strings.ToLower(key)
		decoded := make([]string, len(vals))
		for i, v := range vals {
			decoded[i] = decodeHeaderWord(v)
		}
		val := strings.Join(decoded, ", ")
		switch keyLower {
		case "from", "to", "cc", "bcc", "date", "subject":
			if target[keyLower] {
				content[keyLower] = val
			}
		default:
			meta[keyLower] = val
		}
	}
	// Always emit metadata to match the Python flow parser contract
	// (rag/flow/parser/parser.py:_email), which unconditionally builds a
	// metadata dict and collects every non-basic header into it regardless
	// of whether "metadata" is present in the configured fields.
	content["metadata"] = meta

	// Body and attachments — readMailBody walks all multipart parts
	// and collects text/html bodies alongside attachment parts whose
	// Content-Disposition starts with "attachment".
	needAttachments := target["attachments"]
	// Attachments are extracted independently of "body" (mirrors the Python
	// flow parser _email, whose attachment block is separate from the body
	// block). Walk the body whenever either is requested, then gate each key
	// by its own field so text/text_html stay absent when "body" is not in
	// fields while attachments are still extracted when "attachments" is.
	if target["body"] || needAttachments {
		contentType := msg.Header.Get("Content-Type")
		bodyText, bodyHTML, attachments := readMailBody(msg.Body, contentType, needAttachments)
		// Always emit text/text_html when "body" is requested, to match the
		// Python flow parser contract (rag/flow/parser/parser.py:_email),
		// which sets both unconditionally (empty string for a missing part)
		// rather than omitting the key.
		if target["body"] {
			content["text"] = bodyText
			content["text_html"] = bodyHTML
		}
		if needAttachments {
			content["attachments"] = attachments
		}
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
				// raw preserves the byte-exact decoded-CTE bytes so the
				// re-chunk step re-parses the original attachment instead of
				// the charset-decoded string (which can differ when the
				// attachment declares a CJK charset). rechunkEmailAttachments
				// prefers "raw" and falls back to "payload".
				"raw": string(raw),
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
		// Real-world MIME base64 is often line-wrapped (~76 chars per line).
		// Remove all whitespace before decoding; base64.StdEncoding.DecodeString
		// strictly rejects interior whitespace.
		cleanRaw := bytes.ReplaceAll(raw, []byte("\n"), nil)
		cleanRaw = bytes.ReplaceAll(cleanRaw, []byte("\r"), nil)
		d, err := base64.StdEncoding.DecodeString(string(cleanRaw))
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

// -- HTML body → visible text (text output only) --

// htmlBodyToText flattens an email HTML body to its visible text. Unlike the
// structured HTML parser (which keeps table markup for the dedicated HTML
// chunk method), email bodies use tables for layout, so this walker descends
// into tables and emits cell text instead. <head>/<script>/<style> subtrees
// are skipped; whitespace folding is shared with the standalone HTML parser
// via leafWriter.
func htmlBodyToText(body string) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return body
	}
	var b bytes.Buffer
	w := &leafWriter{b: &b, lineStart: true}
	walkHTMLBodyText(doc, w)
	return strings.TrimSpace(b.String())
}

// isEmailBlockTag reports whether tag delimits a line in flattened email
// body text: the standard block tags plus table tags, since email uses
// tables for layout and their cells/rows must not fuse either.
func isEmailBlockTag(tag string) bool {
	return isBlockTag(tag) || tag == "table" || tag == "td" || tag == "th"
}

func walkHTMLBodyText(n *html.Node, w *leafWriter) {
	switch n.Type {
	case html.TextNode:
		w.writeText(n.Data)
	case html.DocumentNode:
		// html.Parse always wraps the fragment in a Document node holding an
		// <html><body> skeleton; the walker relies on descending through those
		// transparent containers (head/script/style subtrees are skipped by the
		// ElementNode case), so without descending here the whole body would
		// be dropped.
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walkHTMLBodyText(child, w)
		}
	case html.ElementNode:
		switch n.Data {
		case "head", "script", "style", "noscript":
			return
		case "br":
			w.hardBreak()
			return
		}
		// A nested block starts on a new line when text already precedes
		// it, so outer text and inner block text stay separate
		// ("<div>a<p>b</p></div>" flattens to "a\nb", not "ab").
		if isEmailBlockTag(n.Data) && w.b.Len() > 0 && !w.endsNL {
			w.hardBreak()
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walkHTMLBodyText(child, w)
		}
		// Block-level tags and table cells/rows end the current line so
		// adjacent block/cell text does not fuse together.
		if isEmailBlockTag(n.Data) && w.b.Len() > 0 && !w.endsNL {
			w.hardBreak()
		}
	}
}

// decodeMailPayload attempts charset decoding using the unified DecodeToUTF8 helper.
func decodeMailPayload(payload []byte, charset string) string {
	if len(payload) == 0 {
		return ""
	}
	decoded, _ := DecodeToUTF8(payload, charset)
	return string(decoded)
}

// parseMSG parses an Outlook .msg (OLE2 compound document) file using the
// gomsg library, mirroring the Python flow parser's _email() .msg branch
// (rag/flow/parser/parser.py), which parses via extract_msg. The output map
// shares the same field shape as parseEML so the downstream json/text
// assembly in ParseWithResult is reused unchanged.
func parseMSG(data []byte, fields []string) (map[string]any, error) {
	target := targetFieldsSet(fields)

	msg, err := gomsg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	content := map[string]any{}

	if target["from"] {
		content["from"] = formatSender(msg)
	}
	if target["to"] {
		content["to"] = formatRecipient(msg.DisplayTo)
	}
	if target["cc"] {
		content["cc"] = formatRecipient(msg.DisplayCC)
	}
	if target["bcc"] {
		content["bcc"] = formatRecipient(msg.DisplayBCC)
	}
	if target["date"] {
		content["date"] = formatMsgDate(msg.Date)
	}
	if target["subject"] {
		content["subject"] = msg.Subject
	}

	// Always emit metadata to match the Python flow parser contract, which
	// unconditionally builds a {message_id, in_reply_to} metadata dict for
	// .msg files regardless of whether "metadata" is in the configured fields.
	// Empty values are emitted as nil (JSON null) to match extract_msg's None.
	content["metadata"] = map[string]any{
		"message_id":  orNil(msg.MessageID),
		"in_reply_to": orNil(msg.InReplyTo),
	}

	if target["body"] {
		// Mirror Python: prefer the plain body, fall back to the HTML body
		// when the plain body is empty. The .msg branch emits only "text"
		// (never "text_html"), matching the Python _email .msg contract exactly.
		text := msg.Body
		if strings.TrimSpace(text) == "" && len(msg.BodyHTML) > 0 {
			text = string(msg.BodyHTML)
		}
		content["text"] = text
	}

	if target["attachments"] {
		// Flatten attachments, recursing into embedded .msg files. gomsg
		// parses an embedded Message via Attachment.EmbeddedMessage even
		// though it exposes no raw bytes for it; the embedded message body
		// is surfaced as a retrievable text attachment (see msgAttachments).
		content["attachments"] = msgAttachments(msg)
	}

	return content, nil
}

// msgAttachments flattens a gomsg.Message's attachments into the
// {filename, payload} shape rechunkEmailAttachments consumes. Embedded .msg
// attachments expose no raw bytes via gomsg, but gomsg does parse the embedded
// Message; we surface its body as a retrievable text attachment (named .txt so
// the re-chunk step re-parses it as plain text) and recurse into its own
// attachments, so an embedded email is no longer silently dropped.
func msgAttachments(msg *gomsg.Message) []map[string]any {
	out := make([]map[string]any, 0, len(msg.Attachments))
	for _, a := range msg.Attachments {
		if a.IsEmbeddedMessage() {
			if em := a.EmbeddedMessage(); em != nil {
				body := em.Body
				if strings.TrimSpace(body) == "" {
					body = string(em.BodyHTML)
				}
				if body != "" {
					out = append(out, map[string]any{
						"filename": a.DisplayName() + ".txt",
						"payload":  body,
					})
				}
				out = append(out, msgAttachments(em)...)
				continue
			}
		}
		out = append(out, map[string]any{
			"filename": a.DisplayName(),
			"payload":  string(a.Data()),
		})
	}
	return out
}

// primarySenderEmail prefers the SMTP address, falling back to the raw email
// address when SMTP is unavailable (e.g. an EX address type).
func primarySenderEmail(msg *gomsg.Message) string {
	if msg.SenderSMTP != "" {
		return msg.SenderSMTP
	}
	return msg.SenderEmail
}

// formatSender renders the .msg sender the way extract_msg's "sender" string
// is displayed: "Display Name <email>" when a distinct display name is present,
// otherwise "<email>" (matching extract_msg's angular-bracket form for an
// address with no display name).
func formatSender(msg *gomsg.Message) string {
	email := primarySenderEmail(msg)
	if msg.SenderName != "" && msg.SenderName != email {
		return msg.SenderName + " <" + email + ">"
	}
	if email != "" {
		return "<" + email + ">"
	}
	return msg.SenderName
}

// formatRecipient renders a display recipient string the way extract_msg does:
// a bare single email address is wrapped in angle brackets, while a string
// that already contains an address form (display name, or multiple recipients)
// is returned unchanged.
func formatRecipient(display string) string {
	if display == "" {
		return ""
	}
	if strings.Contains(display, "<") {
		return display
	}
	if !strings.ContainsAny(display, ",;") && strings.Contains(display, "@") {
		return "<" + display + ">"
	}
	return display
}

// orNil maps an empty string to nil so it serializes as JSON null, matching
// extract_msg's None for absent properties.
func orNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// formatMsgDate renders an Outlook .msg date the way extract_msg does:
// strftime("%Y-%m-%d %H:%M:%S%z"), which emits the zone without a colon
// (e.g. "2018-03-24 00:06:29+0800"). Go's -0700 layout reproduces that
// (the -07:00 layout would wrongly insert a colon). A zero time (date
// missing from the .msg) maps to nil so it serializes as JSON null,
// matching extract_msg's None, instead of a bogus sentinel such as
// "0001-01-01 00:00:00+0000".
func formatMsgDate(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format("2006-01-02 15:04:05-0700")
}

// recoverParse runs fn, converting a panic from an untrusted attachment
// parser (e.g. a corrupt PDF/DOCX hitting a native CGO backend) into a zero
// ParseResult with panicked=true, so the caller can skip that one attachment
// instead of failing the whole email. This mirrors the recover around the
// .msg parseMSG call in parseEmail.
func recoverParse(fn func() ParseResult) (res ParseResult, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	return fn(), false
}

// rechunkEmailAttachments re-parses each email attachment by its file
// extension and folds the extracted text back into the same document so
// attachment content becomes retrievable. This mirrors the user-oriented
// behaviour of Python's legacy rag/app/email.py, which re-chunks every
// attachment via naive_chunk and merges the resulting chunks into the same
// document.
//
// KNOWN LIMITATION: re-chunk uses the default-config parser returned by
// GetParser(ft) (and, for a nested email, only the top-level p.fields — no
// tenant/setup language, parse_method, or OCR/VLM model). The extracted
// attachment text may therefore differ from ingesting the same file as a
// standalone document through the pipeline's tenant/setup-configured
// parser. This is a deliberate, best-effort approximation for making
// attachment content retrievable (the same simplification Python's
// email.py naive_chunk makes), not a correctness bug; threading the full
// tenant/setup config into re-chunk is intentionally out of scope here.
//
// Attachments whose extension has no text-oriented parser (images, audio,
// video) are skipped: they carry no plain text in this pipeline and would
// require vision/speech models that are out of scope here. A single
// unparseable, empty, unsupported, or panicking attachment is skipped
// without failing the whole email (mirrors email.py's per-attachment
// try/except).
//
// The function returns both forms the caller needs:
//   - extraItems: structured JSON items (one per non-empty text segment),
//     each carrying only "text" and "doc_type_kwd", for the JSON output path.
//   - text:       the concatenated attachment text, for the text output path.
//
// Re-chunking stops at one level of nesting: a nested email is parsed (so its
// body becomes retrievable) but its own attachments are not re-chunked, to
// avoid unbounded recursion through attachments-of-attachments.
const maxRechunkPayloadBytes = 32 << 20 // 32 MiB

func (p *EmailParser) rechunkEmailAttachments(ctx context.Context, content map[string]any, depth int) ([]map[string]any, string) {
	if depth > 0 {
		return nil, ""
	}
	// Honor task cancellation so a long attachment list does not keep
	// re-parsing after the ingestion task has been stopped/aborted.
	if ctx.Err() != nil {
		return nil, ""
	}

	raw, ok := content["attachments"].([]map[string]any)
	if !ok || len(raw) == 0 {
		return nil, ""
	}

	var extra []map[string]any
	var sb strings.Builder
	for _, att := range raw {
		fn, _ := att["filename"].(string)
		payload, _ := att["payload"].(string)
		// Prefer the byte-exact raw bytes (when present) so a re-parsed
		// attachment is not silently corrupted by a prior charset decode.
		if rawPayload, ok := att["raw"].(string); ok && rawPayload != "" {
			payload = rawPayload
		}
		if fn == "" || payload == "" {
			// Empty payload (e.g. an embedded .msg with no exposed bytes) or
			// a missing filename — nothing to re-parse.
			continue
		}
		if len(payload) > maxRechunkPayloadBytes {
			// Too large to re-parse inline; re-chunking re-runs the heavy
			// parsers (PDF/OCR/...), which would otherwise dominate or stall
			// ingestion on a single huge attachment. The attachment remains
			// referenced in metadata.
			continue
		}
		ft := utility.GetFileType(fn)
		switch ft {
		case utility.FileTypeOTHER, utility.FileTypeVISUAL,
			utility.FileTypeAURAL, utility.FileTypeVIDEO, utility.FileTypeFOLDER:
			// No plain-text parser in this pipeline; skip rather than call
			// a vision/speech model that is out of scope.
			continue
		}
		var res ParseResult
		var panicked bool
		if ft == utility.FileTypeEMAIL {
			// Reuse the top-level field configuration (including "body") so a
			// nested .eml/.msg is parsed for its body instead of with a fresh,
			// unconfigured parser that would only emit metadata and index
			// garbage. We always request JSON output for the nested parse so
			// the extracted "text" field is returned clean (the text output
			// path would otherwise also flatten metadata into the result).
			ep := NewEmailParser()
			ep.ConfigureFromSetup(map[string]any{
				"fields":        p.fields,
				"output_format": "json",
			})
			res, panicked = recoverParse(func() ParseResult {
				return ep.parseEmail(ctx, fn, []byte(payload), depth+1)
			})
		} else {
			np, err := GetParser(ft)
			if err != nil {
				continue
			}
			res, panicked = recoverParse(func() ParseResult {
				return np.ParseWithResult(ctx, fn, []byte(payload))
			})
		}
		if panicked {
			// An untrusted attachment parser (e.g. a corrupt PDF/DOCX hitting
			// a native CGO backend) panicked. Skip just this attachment so one
			// bad file can't fail the whole email — mirrors the .msg
			// parseMSG recover.
			log.Printf("email: attachment %q re-parse panicked; skipping", fn)
		}
		if panicked || res.Err != nil {
			continue
		}

		var texts []string
		if res.OutputFormat == "json" && len(res.JSON) > 0 {
			for _, it := range res.JSON {
				if t, ok := it["text"].(string); ok {
					if t = strings.TrimSpace(t); t != "" {
						texts = append(texts, t)
					}
				}
			}
		} else if res.Text != "" {
			if t := strings.TrimSpace(res.Text); t != "" {
				texts = append(texts, t)
			}
		}

		for _, t := range texts {
			extra = append(extra, map[string]any{
				"text":         t,
				"doc_type_kwd": "text",
			})
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(t)
		}
	}
	return extra, sb.String()
}
