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
	"mime/multipart"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEmailParser_EmlJSON(t *testing.T) {
	ctx := t.Context()
	raw := strings.Join([]string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Cc: cc@example.com",
		"Date: Mon, 07 Jul 2025 10:00:00 +0000",
		"Subject: Test Email",
		"Content-Type: text/plain; charset=utf-8",
		"X-Custom-Header: custom-value",
		"",
		"This is the body of the test email.",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"fields":        []string{"from", "to", "cc", "date", "subject", "body", "metadata"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.OutputFormat != "json" {
		t.Fatalf("expected output_format json, got %q", result.OutputFormat)
	}
	if len(result.JSON) != 1 {
		t.Fatalf("expected 1 JSON item, got %d", len(result.JSON))
	}
	item := result.JSON[0]

	if v, ok := item["from"].(string); !ok || v != "sender@example.com" {
		t.Errorf("from: got %q", v)
	}
	if v, ok := item["to"].(string); !ok || v != "recipient@example.com" {
		t.Errorf("to: got %q", v)
	}
	if v, ok := item["subject"].(string); !ok || v != "Test Email" {
		t.Errorf("subject: got %q", v)
	}
	if v, ok := item["text"].(string); !ok || !strings.Contains(v, "body of the test email") {
		t.Errorf("text: got %q", v)
	}
	if meta, ok := item["metadata"].(map[string]any); ok {
		if v, ok := meta["x-custom-header"].(string); !ok || v != "custom-value" {
			t.Errorf("metadata x-custom-header: got %q", v)
		}
	} else {
		t.Error("metadata missing or wrong type")
	}
	if v, ok := item["doc_type_kwd"].(string); !ok || v != "text" {
		t.Errorf("doc_type_kwd: got %q", v)
	}
}

func TestEmailParser_EmlText(t *testing.T) {
	ctx := t.Context()
	raw := strings.Join([]string{
		"From: sender@test.com",
		"To: recipient@test.com",
		"Subject: Hello",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hello, world!",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "text",
		"fields":        []string{"from", "to", "subject", "body"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.OutputFormat != "text" {
		t.Fatalf("expected output_format text, got %q", result.OutputFormat)
	}
	if !strings.Contains(result.Text, "Hello, world!") {
		t.Errorf("text missing body: %q", result.Text)
	}
	if !strings.Contains(result.Text, "sender@test.com") {
		t.Errorf("text missing from: %q", result.Text)
	}
}

// TestEmailParser_EmlTextChunkText covers the KB chunk-quality bug behind
// Feishu issue om_x100b675dcd7a18a4c2cbfff662a404e: the text output of an
// .eml with an RFC 2047 encoded subject, transport headers (DKIM/ARC/
// Received) and an HTML part used to carry (1) the flattened metadata dump,
// (2) the raw "=?utf-8?B?...?=" subject blob and (3) shredded HTML markup —
// all of which ended up in chunks. The text output must instead skip the
// metadata dump, decode the subject, and flatten the HTML part to its
// visible text. The JSON output keeps metadata and raw text_html unchanged.
func TestEmailParser_EmlTextChunkText(t *testing.T) {
	ctx := t.Context()
	// "=?utf-8?B?5bim6ZmE5Lu255qEZW1haWw=?=" decodes to "带附件的email";
	// "=?utf-8?B?5a6J?=" decodes to "安".
	raw := strings.Join([]string{
		"From: =?utf-8?B?5a6J?= <asiro@qq.com>",
		"To: someone@test.com",
		"Subject: =?utf-8?B?5bim6ZmE5Lu255qEZW1haWw=?=",
		"DKIM-Signature: v=1; a=rsa-sha256; b=abc123",
		"ARC-Seal: i=1; cv=none",
		"Received: from mail.example.com by mx.example.com",
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=altbound",
		"",
		"--altbound",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"带附件的email",
		"--altbound",
		"Content-Type: text/html; charset=utf-8",
		"",
		`<div><img style="width:100%" src="http://example.com/x.png"><div>带附件的email</div></div>`,
		"--altbound--",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "text",
		"fields":        []string{"from", "to", "subject", "body"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// (1) No transport-header dump.
	for _, noise := range []string{"metadata:", "dkim-signature", "arc-seal", "received:"} {
		if strings.Contains(strings.ToLower(result.Text), noise) {
			t.Errorf("text output contains header noise %q: %q", noise, result.Text)
		}
	}
	// (2) Subject/from decoded from RFC 2047.
	if !strings.Contains(result.Text, "subject:带附件的email") {
		t.Errorf("subject not decoded: %q", result.Text)
	}
	if !strings.Contains(result.Text, "from:安 <asiro@qq.com>") {
		t.Errorf("from not decoded: %q", result.Text)
	}
	if strings.Contains(result.Text, "=?utf-8?") {
		t.Errorf("raw encoded-word leaked into text: %q", result.Text)
	}
	// (3) HTML part flattened to visible text, no markup.
	for _, tag := range []string{"<div", "<img", "style="} {
		if strings.Contains(result.Text, tag) {
			t.Errorf("raw HTML markup %q leaked into text: %q", tag, result.Text)
		}
	}
	// HTML part must actually contribute its visible text: subject + plain
	// body + html body = exactly 3 occurrences of the body string.
	if c := strings.Count(result.Text, "带附件的email"); c != 3 {
		t.Errorf("expected subject+plain+html body text, got %d occurrences in: %q", c, result.Text)
	}
	if !strings.Contains(result.Text, "text_html:带附件的email") {
		t.Errorf("html body text missing from text_html: %q", result.Text)
	}

	// JSON output keeps the full metadata and raw text_html.
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"fields":        []string{"from", "to", "subject", "body"},
	})
	jres := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if jres.Err != nil {
		t.Fatalf("unexpected error: %v", jres.Err)
	}
	item := jres.JSON[0]
	if _, ok := item["metadata"].(map[string]any); !ok {
		t.Errorf("json output must keep metadata: %#v", item)
	}
	if v, _ := item["text_html"].(string); !strings.Contains(v, "<img") {
		t.Errorf("json output must keep raw text_html: %q", v)
	}
	if v, _ := item["subject"].(string); v != "带附件的email" {
		t.Errorf("json subject not decoded: %q", v)
	}
}

// TestEmailParser_MsgSupported parses a real Outlook .msg fixture and verifies
// the Go output aligns with the Python flow parser _email() .msg branch
// (rag/flow/parser/parser.py). Replaces the old "MsgNotSupported" test now that
// .msg is supported via gomsg.
func TestEmailParser_MsgSupported(t *testing.T) {
	ctx := t.Context()
	data, err := os.ReadFile("testdata/sample.msg")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"fields":        []string{"from", "to", "cc", "bcc", "date", "subject", "body", "attachments", "metadata"},
	})

	result := p.ParseWithResult(ctx, "sample.msg", data)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if len(result.JSON) != 1 {
		t.Fatalf("expected 1 JSON item, got %d", len(result.JSON))
	}
	item := result.JSON[0]

	if v, ok := item["from"].(string); !ok || v != "<christoph@freiraum.xyz>" {
		t.Errorf("from: got %q", v)
	}
	if v, ok := item["to"].(string); !ok || v != "<christoph@freiraum.xyz>" {
		t.Errorf("to: got %q", v)
	}
	if v, ok := item["subject"].(string); !ok || v != "asdf" {
		t.Errorf("subject: got %q", v)
	}
	// The rendered offset follows the host timezone, so assert the layout and the
	// instant instead of one host's spelling.
	if v, ok := item["date"].(string); !ok {
		t.Errorf("date: got %T, want string", item["date"])
	} else if got, err := time.Parse("2006-01-02 15:04:05-0700", v); err != nil {
		t.Errorf("date %q: %v", v, err)
	} else if want := time.Date(2018, 3, 23, 16, 6, 29, 0, time.UTC); !got.Equal(want) {
		t.Errorf("date: got %q = %v, want %v", v, got.UTC(), want)
	}
	if v, ok := item["text"].(string); !ok || v != " \r\n\r\n" {
		t.Errorf("text: got %q", v)
	}
	// The .msg branch must NOT emit text_html (matches Python _email .msg branch).
	if _, ok := item["text_html"]; ok {
		t.Error("text_html must be absent for .msg")
	}
	meta, ok := item["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata missing or wrong type: %T", item["metadata"])
	}
	if v, ok := meta["message_id"].(string); !ok || v == "" {
		t.Errorf("metadata message_id: got %q", v)
	}
	// Empty in_reply_to mirrors extract_msg's None -> JSON null.
	if v, ok := meta["in_reply_to"]; ok && v != nil {
		t.Errorf("metadata in_reply_to: got %v, want nil", v)
	}
	if _, ok := meta["in_reply_to"]; !ok {
		t.Error("metadata in_reply_to key must be present")
	}
	// attachments are extracted by the .msg branch but deliberately dropped
	// from the final ParseResult (consumed by rechunkEmailAttachments;
	// buildPagesFromBytes keeps only text+doc_type_kwd). Verify the
	// high-level result no longer carries the heavy payload...
	if _, ok := item["attachments"]; ok {
		t.Error("attachments must be dropped from the final ParseResult")
	}
	// ...and verify the .msg branch still extracts them at the parse level.
	msgContent, err := parseMSG(data, []string{"from", "to", "cc", "bcc", "date", "subject", "body", "attachments", "metadata"})
	if err != nil {
		t.Fatalf("parseMSG: %v", err)
	}
	atts, ok := msgContent["attachments"].([]map[string]any)
	if !ok {
		t.Fatalf("parseMSG attachments missing or wrong type: %T", msgContent["attachments"])
	}
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if fn, _ := atts[0]["filename"].(string); fn != "5AAoPFgV-nJ965R7o-98C38840-4454-4750-9AEF-F53DB3E37548.jpg" {
		t.Errorf("filename = %q", fn)
	}
	if pl, _ := atts[0]["payload"].(string); len(pl) != 122784 {
		t.Errorf("payload length = %d, want 122784", len(pl))
	}
}

// TestEmailParser_MsgMetadataAlwaysPresent verifies the .msg branch emits
// metadata unconditionally (matching the Python contract) even when "metadata"
// is omitted from the configured fields.
func TestEmailParser_MsgMetadataAlwaysPresent(t *testing.T) {
	ctx := t.Context()
	data, err := os.ReadFile("testdata/sample.msg")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"fields":        []string{"from", "subject"}, // "metadata" intentionally absent
	})

	result := p.ParseWithResult(ctx, "sample.msg", data)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	item := result.JSON[0]

	if _, ok := item["metadata"].(map[string]any); !ok {
		t.Fatalf("metadata must always be present for .msg, got %T", item["metadata"])
	}
	// Basic fields not in fields are dropped.
	for _, dropped := range []string{"to", "date", "body", "attachments"} {
		if _, ok := item[dropped]; ok {
			t.Errorf("%s should be dropped when not in fields", dropped)
		}
	}
}

// TestEmailParser_MsgTextOutputExcludesMetadata verifies the .msg text
// output: .msg content flows through the same flatten loop as .eml, so the
// unconditionally emitted metadata map stays out of the chunkable text
// (while remaining available in the JSON output) and the basic fields are
// still flattened.
func TestEmailParser_MsgTextOutputExcludesMetadata(t *testing.T) {
	ctx := t.Context()
	data, err := os.ReadFile("testdata/sample.msg")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	p := NewEmailParser() // text output; default fields
	p.ConfigureFromSetup(map[string]any{
		"fields": []string{"from", "to", "cc", "bcc", "date", "subject", "body", "attachments", "metadata"},
	})
	result := p.ParseWithResult(ctx, "sample.msg", data)
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.OutputFormat != "text" {
		t.Fatalf("output format = %q, want text", result.OutputFormat)
	}

	if !strings.Contains(result.Text, "subject:asdf") {
		t.Errorf("text output missing subject, got %q", result.Text)
	}
	if !strings.Contains(result.Text, "from:<christoph@freiraum.xyz>") {
		t.Errorf("text output missing from, got %q", result.Text)
	}
	for _, leaked := range []string{"metadata", "message_id", "in_reply_to"} {
		if strings.Contains(result.Text, leaked) {
			t.Errorf("text output must not contain %q, got %q", leaked, result.Text)
		}
	}
}

func TestEmailParser_Base64Attachment(t *testing.T) {
	ctx := t.Context()
	attachmentContent := "Hello! This is the decoded content of the attachment."
	encoded := base64.StdEncoding.EncodeToString([]byte(attachmentContent))
	// Simulate MIME line-wrapping (typically 76 chars per line).
	if len(encoded) > 20 {
		encoded = encoded[:20] + "\r\n" + encoded[20:]
	}

	boundary := "attachboundary"
	raw := strings.Join([]string{
		"From: sender@test.com",
		"To: receiver@test.com",
		"Subject: Base64 Attachment Test",
		"Content-Type: multipart/mixed; boundary=" + boundary,
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Body text here.",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"Content-Disposition: attachment; filename=\"test.txt\"",
		"Content-Transfer-Encoding: base64",
		"",
		encoded,
		"--" + boundary + "--",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"fields":        []string{"from", "body", "attachments"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	item := result.JSON[0]

	// attachments are dropped from the final ParseResult (consumed by
	// rechunk, then deleted); verify the high-level result is clean...
	if _, ok := item["attachments"]; ok {
		t.Error("attachments must be dropped from the final ParseResult")
	}
	// ...and verify the .eml branch still decodes the base64 attachment.
	eml := parseEML(bytes.NewReader([]byte(raw)), []string{"from", "body", "attachments"})
	atts, ok := eml["attachments"].([]map[string]any)
	if !ok {
		t.Fatalf("attachments missing or wrong type: %T", eml["attachments"])
	}
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	payload, ok := atts[0]["payload"].(string)
	if !ok {
		t.Fatalf("payload missing or wrong type: %T", atts[0]["payload"])
	}
	if payload != attachmentContent {
		t.Errorf("attachment payload = %q, want %q (should be decoded from base64, not raw base64)", payload, attachmentContent)
	}
	if fn, _ := atts[0]["filename"].(string); fn != "test.txt" {
		t.Errorf("filename = %q, want test.txt", fn)
	}
}

func TestEmailParser_Base64AttachmentInMixedMultipart(t *testing.T) {
	ctx := t.Context()
	// Simulates the original test email structure:
	// multipart/mixed → multipart/alternative (text/plain + text/html) + base64 attachment
	innerBoundary := "inneralt"
	outerBoundary := "outermixed"
	attachmentContent := "<html><body><h1>Bookmarks</h1><p>Test data</p></body></html>"
	encoded := base64.StdEncoding.EncodeToString([]byte(attachmentContent))
	// Simulate MIME line-wrapping (typically 76 chars per line).
	if len(encoded) > 20 {
		encoded = encoded[:20] + "\r\n" + encoded[20:]
	}

	raw := strings.Join([]string{
		"From: sender@test.com",
		"To: receiver@test.com",
		"Subject: Mixed Multipart Test",
		"Content-Type: multipart/mixed; boundary=" + outerBoundary,
		"",
		"--" + outerBoundary,
		"Content-Type: multipart/alternative; boundary=" + innerBoundary,
		"",
		"--" + innerBoundary,
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Plain text body.",
		"--" + innerBoundary,
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>HTML body.</p>",
		"--" + innerBoundary + "--",
		"--" + outerBoundary,
		"Content-Type: text/html; charset=utf-8",
		"Content-Disposition: attachment; filename=\"bookmarks.html\"",
		"Content-Transfer-Encoding: base64",
		"",
		encoded,
		"--" + outerBoundary + "--",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"fields":        []string{"from", "body", "attachments"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	item := result.JSON[0]

	// Verify body text was extracted from nested multipart/alternative
	if v, ok := item["text"].(string); !ok || !strings.Contains(v, "Plain text body") {
		t.Errorf("text: got %q, want to contain 'Plain text body'", v)
	}
	if v, ok := item["text_html"].(string); !ok || !strings.Contains(v, "HTML body") {
		t.Errorf("text_html: got %q, want to contain 'HTML body'", v)
	}

	// attachments are dropped from the final ParseResult; verify the
	// high-level result is clean...
	if _, ok := item["attachments"]; ok {
		t.Error("attachments must be dropped from the final ParseResult")
	}
	// ...and verify the .eml branch still decodes the base64 attachment.
	eml := parseEML(bytes.NewReader([]byte(raw)), []string{"from", "body", "attachments"})
	atts, ok := eml["attachments"].([]map[string]any)
	if !ok || len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	payload, _ := atts[0]["payload"].(string)
	if payload != attachmentContent {
		t.Errorf("attachment payload = %q, want %q (should be decoded from base64)", payload, attachmentContent)
	}
}

func TestEmailParser_Multipart(t *testing.T) {
	ctx := t.Context()
	boundary := "boundary123"
	raw := strings.Join([]string{
		"From: multipart@test.com",
		"To: receiver@test.com",
		"Subject: Multipart Test",
		"Content-Type: multipart/alternative; boundary=" + boundary,
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Plain text body.",
		"--" + boundary,
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>HTML body.</p>",
		"--" + boundary + "--",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"fields":        []string{"from", "body"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	item := result.JSON[0]
	if v, ok := item["text"].(string); !ok || !strings.Contains(v, "Plain text body") {
		t.Errorf("text: got %q", v)
	}
	if v, ok := item["text_html"].(string); !ok || !strings.Contains(v, "HTML body") {
		t.Errorf("text_html: got %q", v)
	}
}

// TestEmailParser_MetadataAlwaysPresent aligns Go with the Python flow parser
// contract: metadata is emitted unconditionally and every non-basic header is
// collected into it, even when "metadata" is NOT listed in fields.
func TestEmailParser_MetadataAlwaysPresent(t *testing.T) {
	ctx := t.Context()
	raw := strings.Join([]string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Cc: cc@example.com",
		"Date: Mon, 07 Jul 2025 10:00:00 +0000",
		"Subject: Test Email",
		"Message-ID: <abc@def.example>",
		"X-Custom-Header: custom-value",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"This is the body of the test email.",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		// Note: "metadata" is intentionally NOT in fields.
		"fields": []string{"from", "subject", "body"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	item := result.JSON[0]

	// metadata must exist even though it is not in fields (Python contract).
	meta, ok := item["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata missing or wrong type (must always be present): %T", item["metadata"])
	}
	if v, ok := meta["x-custom-header"].(string); !ok || v != "custom-value" {
		t.Errorf("metadata x-custom-header: got %q", v)
	}
	if v, ok := meta["message-id"].(string); !ok || v != "<abc@def.example>" {
		t.Errorf("metadata message-id: got %q", v)
	}
	if v, ok := meta["content-type"].(string); !ok || !strings.Contains(v, "text/plain") {
		t.Errorf("metadata content-type: got %q", v)
	}

	// Basic fields requested in fields appear at top level.
	if v, ok := item["from"].(string); !ok || v != "sender@example.com" {
		t.Errorf("from: got %q", v)
	}
	if v, ok := item["subject"].(string); !ok || v != "Test Email" {
		t.Errorf("subject: got %q", v)
	}

	// Basic fields NOT in fields are dropped (neither top-level nor metadata).
	for _, dropped := range []string{"to", "cc", "date"} {
		if _, ok := item[dropped]; ok {
			t.Errorf("%s should be dropped when not in fields, but was present", dropped)
		}
		if _, ok := meta[dropped]; ok {
			t.Errorf("%s should not leak into metadata when not in fields", dropped)
		}
	}
}

// TestEmailParser_TextHTMLAlwaysPresent aligns Go with the Python flow parser
// contract: text and text_html are always emitted (empty string when the part
// is missing), not omitted when empty.
func TestEmailParser_TextHTMLAlwaysPresent(t *testing.T) {
	ctx := t.Context()
	raw := strings.Join([]string{
		"From: sender@example.com",
		"To: recipient@example.com",
		"Subject: Plain only",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Plain body, no html part.",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"fields":        []string{"from", "body"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	item := result.JSON[0]

	if v, ok := item["text"].(string); !ok || !strings.Contains(v, "Plain body") {
		t.Errorf("text: got %q", v)
	}
	// text_html must be present (empty) for a plain-text email.
	v, ok := item["text_html"].(string)
	if !ok {
		t.Fatalf("text_html should always be present (even empty), got %T", item["text_html"])
	}
	if v != "" {
		t.Errorf("text_html: expected empty for plain-text email, got %q", v)
	}
}

// TestEmailParser_AttachmentsWithoutBody aligns Go with the Python flow
// parser contract: attachments are extracted whenever "attachments" is in
// fields, independently of whether "body" is requested. The Python _email
// attachment block is separate from the body block, so a config that selects
// only attachments must still yield them. Previously Go silently returned an
// empty list because attachment extraction was coupled to the body branch
// (the "else if needAttachments" fallback set an empty slice instead of
// walking the message).
// TestEmailParser_AttachmentSearchableJSON verifies the user-oriented
// behaviour: an email attachment is re-parsed by its file extension and its
// content becomes a retrievable chunk in the SAME document (mirrors Python
// legacy rag/app/email.py naive_chunk). The attachment text must appear as a
// separate JSON item, while the email body stays on the main item.
func TestEmailParser_AttachmentSearchableJSON(t *testing.T) {
	ctx := t.Context()
	attachmentContent := "QUOTE: the quick brown fox jumps over the lazy dog."
	encoded := base64.StdEncoding.EncodeToString([]byte(attachmentContent))
	boundary := "attachboundary"
	raw := strings.Join([]string{
		"From: sender@test.com",
		"To: receiver@test.com",
		"Subject: Attachment Test",
		"Content-Type: multipart/mixed; boundary=" + boundary,
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Email body text.",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"Content-Disposition: attachment; filename=\"note.txt\"",
		"Content-Transfer-Encoding: base64",
		"",
		encoded,
		"--" + boundary + "--",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"fields":        []string{"from", "body", "attachments"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Body must remain on the main item.
	if v, ok := result.JSON[0]["text"].(string); !ok || !strings.Contains(v, "Email body text") {
		t.Errorf("body missing on main item: %v", result.JSON[0]["text"])
	}

	// Attachment text must appear as a separate retrievable JSON item.
	found := false
	for _, it := range result.JSON {
		if txt, ok := it["text"].(string); ok && strings.Contains(txt, "quick brown fox") {
			found = true
		}
	}
	if !found {
		t.Errorf("attachment text not found in JSON output:\n%#v", result.JSON)
	}
}

// TestEmailParser_AttachmentSearchableText is the text-output equivalent:
// the re-parsed attachment text must be present in result.Text.
func TestEmailParser_AttachmentSearchableText(t *testing.T) {
	ctx := t.Context()
	attachmentContent := "QUOTE: the quick brown fox jumps over the lazy dog."
	encoded := base64.StdEncoding.EncodeToString([]byte(attachmentContent))
	boundary := "attachboundary"
	raw := strings.Join([]string{
		"From: sender@test.com",
		"To: receiver@test.com",
		"Subject: Attachment Test",
		"Content-Type: multipart/mixed; boundary=" + boundary,
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Email body text.",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"Content-Disposition: attachment; filename=\"note.txt\"",
		"Content-Transfer-Encoding: base64",
		"",
		encoded,
		"--" + boundary + "--",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "text",
		"fields":        []string{"from", "body", "attachments"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if !strings.Contains(result.Text, "quick brown fox") {
		t.Errorf("attachment text missing from text output: %q", result.Text)
	}
	if !strings.Contains(result.Text, "Email body text") {
		t.Errorf("email body missing from text output: %q", result.Text)
	}
}

// TestEmailParser_AttachmentEmptyPayloadSkipped verifies error isolation:
// an attachment with an empty payload (the legacy .msg nested-attachment
// case, where gomsg exposes no raw bytes) is skipped without breaking the
// email, and a valid sibling attachment is still re-chunked.
func TestEmailParser_AttachmentEmptyPayloadSkipped(t *testing.T) {
	ctx := t.Context()
	validContent := "VALID attachment payload"
	validEncoded := base64.StdEncoding.EncodeToString([]byte(validContent))
	boundary := "mixedbound"
	raw := strings.Join([]string{
		"From: sender@test.com",
		"To: receiver@test.com",
		"Subject: Empty Payload",
		"Content-Type: multipart/mixed; boundary=" + boundary,
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hello body.",
		"--" + boundary,
		"Content-Type: application/octet-stream; name=\"empty.bin\"",
		"Content-Disposition: attachment; filename=\"empty.bin\"",
		"Content-Transfer-Encoding: base64",
		"",
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"Content-Disposition: attachment; filename=\"valid.txt\"",
		"Content-Transfer-Encoding: base64",
		"",
		validEncoded,
		"--" + boundary + "--",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"fields":        []string{"from", "body", "attachments"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("email must not fail on empty-payload attachment: %v", result.Err)
	}

	found := false
	for _, it := range result.JSON {
		if txt, ok := it["text"].(string); ok && strings.Contains(txt, "VALID attachment payload") {
			found = true
		}
	}
	if !found {
		t.Errorf("valid attachment text not found despite empty-payload sibling: %#v", result.JSON)
	}
}

// TestEmailParser_AttachmentUnknownExtSkipped verifies that an attachment
// with an unrecognized extension is skipped (no parser available) while a
// valid sibling attachment is still re-chunked. Mirrors the legacy email.py
// behaviour, which only re-chunks known file types.
func TestEmailParser_AttachmentUnknownExtSkipped(t *testing.T) {
	ctx := t.Context()
	validContent := "KNOWN attachment payload"
	validEncoded := base64.StdEncoding.EncodeToString([]byte(validContent))
	boundary := "mixedbound"
	raw := strings.Join([]string{
		"From: sender@test.com",
		"To: receiver@test.com",
		"Subject: Unknown Ext",
		"Content-Type: multipart/mixed; boundary=" + boundary,
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hello body.",
		"--" + boundary,
		"Content-Type: application/octet-stream; name=\"data.xyz\"",
		"Content-Disposition: attachment; filename=\"data.xyz\"",
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString([]byte("opaque bytes")),
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"Content-Disposition: attachment; filename=\"known.txt\"",
		"Content-Transfer-Encoding: base64",
		"",
		validEncoded,
		"--" + boundary + "--",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"fields":        []string{"from", "body", "attachments"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("email must not fail on unknown-extension attachment: %v", result.Err)
	}

	found := false
	for _, it := range result.JSON {
		if txt, ok := it["text"].(string); ok && strings.Contains(txt, "KNOWN attachment payload") {
			found = true
		}
	}
	if !found {
		t.Errorf("known attachment text not found despite unknown-ext sibling: %#v", result.JSON)
	}
}

func TestEmailParser_AttachmentsWithoutBody(t *testing.T) {
	ctx := t.Context()
	attachmentContent := "SECRET attachment payload"
	encoded := base64.StdEncoding.EncodeToString([]byte(attachmentContent))
	boundary := "mixedbound"
	raw := strings.Join([]string{
		"From: sender@test.com",
		"To: receiver@test.com",
		"Subject: Attach Test",
		"Content-Type: multipart/mixed; boundary=" + boundary,
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hello body.",
		"--" + boundary,
		"Content-Type: application/octet-stream; name=\"a.txt\"",
		"Content-Disposition: attachment; filename=\"a.txt\"",
		"Content-Transfer-Encoding: base64",
		"",
		encoded,
		"--" + boundary + "--",
	}, "\r\n")

	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		// NOTE: "body" is intentionally NOT in fields.
		"fields": []string{"from", "attachments"},
	})

	result := p.ParseWithResult(ctx, "test.eml", []byte(raw))
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	item := result.JSON[0]

	// text/text_html must be absent (gated by "body"), matching Python.
	if _, ok := item["text"]; ok {
		t.Error("text should be absent when body not in fields")
	}
	if _, ok := item["text_html"]; ok {
		t.Error("text_html should be absent when body not in fields")
	}

	// attachments are dropped from the final ParseResult (consumed by
	// rechunk, then deleted); verify the high-level result is clean...
	if _, ok := item["attachments"]; ok {
		t.Error("attachments must be dropped from the final ParseResult")
	}
	// ...and verify the .eml branch still extracts them even without body.
	eml := parseEML(bytes.NewReader([]byte(raw)), []string{"from", "attachments"})
	atts, ok := eml["attachments"].([]map[string]any)
	if !ok {
		t.Fatalf("attachments missing or wrong type: %T", eml["attachments"])
	}
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment without body, got %d (bug: attachments silently dropped)", len(atts))
	}
	if fn, _ := atts[0]["filename"].(string); fn != "a.txt" {
		t.Errorf("filename = %q, want a.txt", fn)
	}
	if pl, _ := atts[0]["payload"].(string); pl != attachmentContent {
		t.Errorf("payload = %q, want %q", pl, attachmentContent)
	}
}

// TestEmailParser_NestedEMLAttachmentRechunk locks the regression where a
// nested .eml attachment was re-chunked with an UNCONFIGURED EmailParser
// (fields == nil), so parseEML emitted only metadata and the text path indexed
// "metadata:{...}" garbage. The nested email must instead be parsed with the
// top-level field configuration (including "body") so its body becomes the
// indexed text. The outer email's own metadata flattening is expected; the
// nested email must NOT contribute a second "metadata:{" segment.
func TestEmailParser_NestedEMLAttachmentRechunk(t *testing.T) {
	ctx := t.Context()

	innerRaw := strings.Join([]string{
		"From: inner@x.com",
		"To: outer@y.com",
		"Subject: inner",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"INNER BODY SECRET",
	}, "\r\n")

	boundary := "outerbound"
	raw := strings.Join([]string{
		"From: outer@y.com",
		"To: someone@z.com",
		"Subject: outer",
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=" + boundary,
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=utf-8",
		"",
		"OUTER BODY VISIBLE",
		"--" + boundary,
		"Content-Type: message/rfc822",
		"Content-Disposition: attachment; filename=\"inner.eml\"",
		"",
		innerRaw,
		"--" + boundary + "--",
	}, "\r\n")

	for _, format := range []string{"json", "text"} {
		p := NewEmailParser()
		p.ConfigureFromSetup(map[string]any{
			"output_format": format,
			"fields":        []string{"from", "to", "subject", "body", "attachments", "metadata"},
		})

		result := p.ParseWithResult(ctx, "outer.eml", []byte(raw))
		if result.Err != nil {
			t.Fatalf("[%s] unexpected error: %v", format, result.Err)
		}

		// Collect all indexed text for this output mode (JSON populates
		// result.JSON; text populates result.Text).
		var indexed strings.Builder
		if format == "json" {
			for _, it := range result.JSON {
				if txt, ok := it["text"].(string); ok {
					indexed.WriteString(txt)
					indexed.WriteString("\n")
				}
			}
		} else {
			indexed.WriteString(result.Text)
		}

		// The nested email body must be retrievable.
		if !strings.Contains(indexed.String(), "INNER BODY SECRET") {
			t.Errorf("[%s] nested body not indexed: %q", format, indexed.String())
		}
		if !strings.Contains(indexed.String(), "OUTER BODY VISIBLE") {
			t.Errorf("[%s] outer body missing: %q", format, indexed.String())
		}

		if format == "json" {
			// The nested email must be a distinct, clean item, carrying no
			// metadata key (the old bug leaked a "metadata:{...}" string as
			// its text).
			found := false
			for _, it := range result.JSON {
				if txt, ok := it["text"].(string); ok && strings.Contains(txt, "INNER BODY SECRET") {
					found = true
					if _, hasMeta := it["metadata"]; hasMeta {
						t.Errorf("[json] nested item must not carry metadata key: %#v", it)
					}
				}
			}
			if !found {
				t.Errorf("[json] nested body not found as separate item: %#v", result.JSON)
			}
		} else {
			// Text mode: metadata (transport headers) is never flattened
			// into chunkable text — not for the outer email, and not for
			// the nested re-parse (which always requests JSON output).
			if strings.Contains(result.Text, "metadata:") {
				t.Errorf("[text] metadata must not leak into text output: %q", result.Text)
			}
		}
	}
}

// TestEmailParser_RechunkAttachmentTextPath exercises rechunkEmailAttachments
// directly with the {filename, payload} attachment shape that both .eml and
// .msg parsing feed into it. It verifies text attachments are indexed while
// binary (VISUAL) attachments are skipped, and that binary payloads never leak
// into the indexed text.
func TestEmailParser_RechunkAttachmentTextPath(t *testing.T) {
	ctx := t.Context()
	content := map[string]any{
		"attachments": []map[string]any{
			{"filename": "note.txt", "payload": "hello from attachment"},
			{"filename": "pic.jpg", "payload": "<binary bytes>"},
		},
	}

	p := NewEmailParser()
	extra, text := p.rechunkEmailAttachments(ctx, content, 0)

	if len(extra) != 1 {
		t.Fatalf("expected 1 indexed attachment, got %d: %#v", len(extra), extra)
	}
	if v, _ := extra[0]["text"].(string); v != "hello from attachment" {
		t.Errorf("text = %q, want hello from attachment", v)
	}
	if !strings.Contains(text, "hello from attachment") {
		t.Errorf("indexed text missing attachment: %q", text)
	}
	if strings.Contains(text, "<binary bytes>") {
		t.Errorf("binary payload leaked into indexed text: %q", text)
	}
}

// TestFormatMsgDate verifies the .msg date rendering: a zero time (date
// missing from the .msg) maps to nil (JSON null, matching extract_msg's
// None) instead of a bogus sentinel like "0001-01-01 00:00:00+0000".
func TestFormatMsgDate(t *testing.T) {
	if got := formatMsgDate(time.Time{}); got != nil {
		t.Errorf("zero date should map to nil, got %#v", got)
	}
	got := formatMsgDate(time.Date(2018, 3, 24, 0, 6, 29, 0, time.FixedZone("CST", 8*3600)))
	if got != "2018-03-24 00:06:29+0800" {
		t.Errorf("formatted date = %q, want 2018-03-24 00:06:29+0800", got)
	}
}

// TestRechunkEmailAttachments_ContextCancelled verifies that an already
// cancelled context short-circuits re-chunking instead of re-parsing
// attachments (mirrors the cancellation check CodeRabbit flagged).
func TestRechunkEmailAttachments_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // task already aborted
	content := map[string]any{
		"attachments": []map[string]any{
			{"filename": "note.txt", "payload": "must not be re-chunked"},
		},
	}
	extra, text := NewEmailParser().rechunkEmailAttachments(ctx, content, 0)
	if len(extra) != 0 || text != "" {
		t.Errorf("cancelled context should short-circuit re-chunk: extra=%#v text=%q", extra, text)
	}
}

// TestRecoverParse_IsolatesPanic verifies that a panic from an untrusted
// attachment parser is converted into a zero ParseResult with panicked=true
// and does NOT propagate out of recoverParse (so a single bad attachment can
// be skipped instead of failing the whole email).
func TestRecoverParse_IsolatesPanic(t *testing.T) {
	res, panicked := recoverParse(func() ParseResult {
		panic("boom")
	})
	if !panicked {
		t.Error("expected panicked=true")
	}
	if res.Err != nil || res.JSON != nil || res.Text != "" {
		t.Errorf("panic should yield a zero ParseResult, got %#v", res)
	}
}

// TestRecoverParse_PassesThrough verifies a normal parser result is returned
// unchanged with panicked=false.
func TestRecoverParse_PassesThrough(t *testing.T) {
	want := ParseResult{OutputFormat: "text", Text: "hello"}
	res, panicked := recoverParse(func() ParseResult {
		return want
	})
	if panicked {
		t.Error("expected panicked=false for a normal result")
	}
	if res.Text != "hello" {
		t.Errorf("result not passed through: %#v", res)
	}
}

// TestEmailParser_RechunkPrefersRaw verifies that rechunkEmailAttachments
// re-parses the byte-exact "raw" bytes when present, rather than the
// charset-decoded "payload". The fallback (no "raw") still uses "payload".
func TestEmailParser_RechunkPrefersRaw(t *testing.T) {
	ctx := t.Context()
	p := NewEmailParser()

	withRaw, textRaw := p.rechunkEmailAttachments(ctx, map[string]any{
		"attachments": []map[string]any{
			{"filename": "note.txt", "payload": "WORLD", "raw": "HELLO"},
		},
	}, 0)
	if len(withRaw) != 1 || withRaw[0]["text"] != "HELLO" {
		t.Fatalf("raw not preferred: extra=%#v text=%q", withRaw, textRaw)
	}
	if strings.Contains(textRaw, "WORLD") {
		t.Errorf("re-chunk used payload instead of raw: %q", textRaw)
	}

	fallback, textFallback := p.rechunkEmailAttachments(ctx, map[string]any{
		"attachments": []map[string]any{
			{"filename": "note.txt", "payload": "WORLD"},
		},
	}, 0)
	if len(fallback) != 1 || fallback[0]["text"] != "WORLD" {
		t.Fatalf("payload fallback broken: extra=%#v text=%q", fallback, textFallback)
	}
}

// TestReadMailBody_AttachmentPreservesRaw verifies that .eml attachment
// collection stores the byte-exact decoded-CTE bytes under "raw" alongside the
// charset-decoded "payload". Declaring charset=gbk makes decodeMailPayload
// decode GBK bytes to a UTF-8 string, so the two differ — re-chunk relies on
// "raw" to avoid silently corrupting the attachment.
func TestReadMailBody_AttachmentPreservesRaw(t *testing.T) {
	// GBK-encoded "中文" (中=0xD6D0, 文=0xCEC4).
	gbk := []byte{0xD6, 0xD0, 0xCE, 0xC4}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	bp, _ := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain"}})
	bp.Write([]byte("body"))
	ap, _ := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"application/octet-stream; charset=gbk"},
		"Content-Disposition":       {`attachment; filename="x.bin"`},
		"Content-Transfer-Encoding": {"base64"},
	})
	ap.Write([]byte(base64.StdEncoding.EncodeToString(gbk)))
	mw.Close()

	_, _, attachments := readMailBody(strings.NewReader(buf.String()), "multipart/mixed; boundary="+mw.Boundary(), true)
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d: %#v", len(attachments), attachments)
	}
	att := attachments[0]
	if raw, _ := att["raw"].(string); raw != string(gbk) {
		t.Errorf("raw = %q, want byte-exact %q", raw, string(gbk))
	}
	if payload, _ := att["payload"].(string); payload != "中文" {
		t.Errorf("payload = %q, want 中文 (charset-decoded)", payload)
	}
}

// TestDecodeHeaderWord_Charsets verifies the charset routing behind RFC 2047
// encoded-word decoding. The "gb2312" label must decode plain 8-bit
// GB2312/GBK bytes — not HZ escape sequences — because real-world mail
// labeled gb2312 carries plain bytes. HZ decoding stays available under its
// own "hz-gb-2312" label. An unsupported charset leaves the value untouched.
func TestDecodeHeaderWord_Charsets(t *testing.T) {
	encodedWord := func(charset string, raw []byte) string {
		return "=?" + charset + "?B?" + base64.StdEncoding.EncodeToString(raw) + "?="
	}

	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"gb2312 label decodes plain GB2312 bytes", encodedWord("gb2312", []byte{0xD6, 0xD0, 0xCE, 0xC4}), "中文"},
		{"gbk", encodedWord("gbk", []byte{0xD6, 0xD0, 0xCE, 0xC4}), "中文"},
		{"gb18030", encodedWord("gb18030", []byte{0xD6, 0xD0, 0xCE, 0xC4}), "中文"},
		{"big5", encodedWord("big5", []byte{0xA4, 0xA4, 0xA4, 0xE5}), "中文"},
		{"shift_jis", encodedWord("shift_jis", []byte{0x93, 0xFA, 0x96, 0x7B}), "日本"},
		{"euc-kr", encodedWord("euc-kr", []byte{0xC7, 0xD1, 0xB1, 0xB9}), "한국"},
		{"iso-8859-1", encodedWord("iso-8859-1", []byte{'c', 'a', 'f', 0xE9}), "café"},
		{"hz-gb-2312 keeps HZ escape decoding", encodedWord("hz-gb-2312", []byte("~{VPND~}")), "中文"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decodeHeaderWord(tt.header); got != tt.want {
				t.Errorf("decodeHeaderWord(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}

	unsupported := encodedWord("x-unsupported-charset", []byte{0x01, 0x02})
	if got := decodeHeaderWord(unsupported); got != unsupported {
		t.Errorf("unsupported charset: decodeHeaderWord(%q) = %q, want value untouched", unsupported, got)
	}
}

// TestParseEML_GB2312EncodedSubject is the end-to-end regression for the
// charset fix: a subject labeled gb2312 carrying plain 8-bit bytes must
// arrive decoded, not as the raw "=?gb2312?B?...?=" blob.
func TestParseEML_GB2312EncodedSubject(t *testing.T) {
	raw := strings.Join([]string{
		"From: sender@example.com",
		"Subject: =?gb2312?B?" + base64.StdEncoding.EncodeToString([]byte{0xD6, 0xD0, 0xCE, 0xC4}) + "?=",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"body",
	}, "\r\n")

	content := parseEML(strings.NewReader(raw), []string{"subject"})
	if content["subject"] != "中文" {
		t.Errorf("subject = %q, want 中文", content["subject"])
	}
}

// TestDecodeMailPayload_DeclaredCharsets verifies the body-side charset
// resolution: a gb2312-labeled body carries plain 8-bit bytes (routed to
// GBK like the header path, not HZGB2312), HZ-escaped content decodes only
// under its own "hz-gb-2312" label, and a declared charset beyond the
// fallback chain (big5) decodes via charsetEncoding instead of degrading to
// latin-1 mojibake.
func TestDecodeMailPayload_DeclaredCharsets(t *testing.T) {
	if got := decodeMailPayload([]byte{0xD6, 0xD0, 0xCE, 0xC4}, "gb2312"); got != "中文" {
		t.Errorf("gb2312 body = %q, want 中文", got)
	}
	if got := decodeMailPayload([]byte("~{VPND~}"), "hz-gb-2312"); got != "中文" {
		t.Errorf("hz-gb-2312 body = %q, want 中文", got)
	}
	if got := decodeMailPayload([]byte{0xA4, 0xA4, 0xA4, 0xE5}, "big5"); got != "中文" {
		t.Errorf("big5 body = %q, want 中文", got)
	}
}

// TestReadMailBody_DeclaredBodyCharsets locks the body-path charset routing
// exercised end to end by readMailBody (readMailBody → decodeMailPayload →
// decodeWithCharset), beyond the direct decodeMailPayload unit above: a
// gb2312-declared body part decodes its plain 8-bit bytes, and HZ-escaped
// content decodes under its own "hz-gb-2312" label.
func TestReadMailBody_DeclaredBodyCharsets(t *testing.T) {
	tests := []struct {
		name    string
		charset string
		body    []byte
	}{
		{"gb2312 body decodes plain bytes", "gb2312", []byte{0xD6, 0xD0, 0xCE, 0xC4}},
		{"hz-gb-2312 body decodes HZ escapes", "hz-gb-2312", []byte("~{VPND~}")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, _, _ := readMailBody(strings.NewReader(string(tt.body)), "text/plain; charset="+tt.charset, false)
			if text != "中文" {
				t.Errorf("readMailBody(charset=%s) = %q, want 中文", tt.charset, text)
			}
		})
	}
}

// TestHTMLBodyToText verifies the visible-text flattening of email HTML
// bodies, above all that text preceding a nested block-level element starts
// a new line instead of fusing with the block's own text.
func TestHTMLBodyToText(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"text before nested block", `<div>prefix<p>paragraph</p></div>`, "prefix\nparagraph"},
		{"nested divs", `<div>outer<div>inner</div></div>`, "outer\ninner"},
		{"sibling blocks", `<p>one</p><p>two</p>`, "one\ntwo"},
		{"layout table cells", `<table><tr><td>a</td><td>b</td></tr></table>`, "a\nb"},
		{"multi-row layout table stays line-separated", `<p>intro</p><table><tr><td>a</td><td>b</td></tr><tr><td>c</td></tr></table><p>outro</p>`, "intro\na\nb\nc\noutro"},
		{"inline tags stay on one line", `<p>Hello <b>world</b></p>`, "Hello world"},
		{"br forces a break", `one<br>two`, "one\ntwo"},
		{"head script style skipped", `<html><head><title>t</title><style>.x{color:red}</style></head><body><p>visible</p><script>var x=1;</script></body></html>`, "visible"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := htmlBodyToText(tt.body); got != tt.want {
				t.Errorf("htmlBodyToText(%q) = %q, want %q", tt.body, got, tt.want)
			}
		})
	}
}
