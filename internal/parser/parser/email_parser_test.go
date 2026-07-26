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
	"encoding/base64"
	"strings"
	"testing"
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

func TestEmailParser_MsgNotSupported(t *testing.T) {
	ctx := t.Context()
	p := NewEmailParser()
	result := p.ParseWithResult(ctx, "test.msg", []byte{})
	if result.Err == nil {
		t.Fatal("expected error for .msg file")
	}
	if !strings.Contains(result.Err.Error(), ".msg") {
		t.Errorf("error should mention .msg: %v", result.Err)
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

	atts, ok := item["attachments"].([]map[string]any)
	if !ok {
		t.Fatalf("attachments missing or wrong type: %T", item["attachments"])
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

	// Verify attachment is decoded from base64
	atts, ok := item["attachments"].([]map[string]any)
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
