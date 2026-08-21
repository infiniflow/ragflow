package parser

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// buildChineseEML constructs a realistic Chinese-client (QQ/Foxmail-style)
// eml: RFC 2047 encoded-word headers (GBK/base64), a multipart/alternative
// body (text/plain + text/html, GBK + base64), and an RFC2047-encoded
// attachment filename.
func buildChineseEML(t *testing.T) []byte {
	t.Helper()
	gbk := func(s string) string {
		out, _, err := transform.String(simplifiedchinese.GBK.NewEncoder(), s)
		if err != nil {
			t.Fatalf("gbk encode: %v", err)
		}
		return out
	}
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	wrap76 := func(s string) []string {
		var out []string
		for len(s) > 76 {
			out = append(out, s[:76])
			s = s[76:]
		}
		if len(s) > 0 {
			out = append(out, s)
		}
		return out
	}

	plainBody := "你好，这是一封测试邮件的正文。\n第二行内容。"
	htmlBody := "<html><head><meta charset=\"gbk\"></head><body><div>你好，这是一封测试邮件的正文（HTML 版）。</div><p>第二行内容。</p></body></html>"
	attachTxt := "这是附件里的文本内容，附件文件名是中文的。"
	encName := "=?gbk?B?" + b64(gbk("测试附件.txt")) + "?="

	var sb strings.Builder
	w := func(s string) { sb.WriteString(s); sb.WriteString("\r\n") }

	w("From: =?gbk?B?" + b64(gbk("张三")) + "?= <zhangsan@example.com>")
	w("To: =?gbk?B?" + b64(gbk("李四")) + "?= <lisi@example.com>")
	w("Subject: =?gbk?B?" + b64(gbk("测试主题：项目周报")) + "?=")
	w("Date: Thu, 20 Aug 2026 10:00:00 +0800")
	w("MIME-Version: 1.0")
	w(`Content-Type: multipart/mixed; boundary="----=_Part_001"`)
	w("")
	w("------=_Part_001")
	w(`Content-Type: multipart/alternative; boundary="----=_Part_002"`)
	w("")
	w("------=_Part_002")
	w("Content-Type: text/plain; charset=gbk")
	w("Content-Transfer-Encoding: base64")
	w("")
	for _, line := range wrap76(b64(gbk(plainBody))) {
		w(line)
	}
	w("")
	w("------=_Part_002")
	w("Content-Type: text/html; charset=gbk")
	w("Content-Transfer-Encoding: base64")
	w("")
	for _, line := range wrap76(b64(gbk(htmlBody))) {
		w(line)
	}
	w("")
	w("------=_Part_002--")
	w("")
	w("------=_Part_001")
	w(`Content-Type: application/octet-stream; name="` + encName + `"`)
	w("Content-Transfer-Encoding: base64")
	w(`Content-Disposition: attachment; filename="` + encName + `"`)
	w("")
	for _, line := range wrap76(b64(gbk(attachTxt))) {
		w(line)
	}
	w("")
	w("------=_Part_001--")
	return []byte(sb.String())
}

func newKBEmailParser(format string) *EmailParser {
	p := NewEmailParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": format,
		"fields":        []string{"from", "to", "cc", "bcc", "date", "subject", "body", "attachments"},
	})
	return p
}

// TestEmailParser_ChineseEMLRFC2047Text covers the KB Email chunk method's
// text output for a Chinese eml with an attachment. It mirrors Python
// rag/app/email.py + email.policy.default:
//   - RFC2047 encoded-word headers must decode (张三/李四/测试主题：项目周报)
//   - the HTML body must surface as visible text, not raw markup
//   - the RFC2047 attachment filename must decode so re-chunking indexes
//     the attachment content
//   - the output must be deterministic across parses
func TestEmailParser_ChineseEMLRFC2047Text(t *testing.T) {
	eml := buildChineseEML(t)
	p := newKBEmailParser("text")

	res := p.ParseWithResult(context.Background(), "带附件的email.eml", eml)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	text := res.Text

	for _, want := range []string{"张三", "李四", "测试主题：项目周报", "你好，这是一封测试邮件的正文。"} {
		if !strings.Contains(text, want) {
			t.Errorf("text output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "gbk?B?") {
		t.Errorf("text output contains undecoded RFC2047 encoded-words:\n%s", text)
	}
	if strings.Contains(text, "<html>") || strings.Contains(text, "</div>") {
		t.Errorf("text output leaks raw HTML markup:\n%s", text)
	}
	if !strings.Contains(text, "你好，这是一封测试邮件的正文（HTML 版）。") {
		t.Errorf("text output missing HTML-body visible text:\n%s", text)
	}
	if !strings.Contains(text, "这是附件里的文本内容") {
		t.Errorf("attachment content missing from text output (filename/type detection):\n%s", text)
	}

	res2 := p.ParseWithResult(context.Background(), "带附件的email.eml", eml)
	if res2.Text != text {
		t.Errorf("text output not deterministic:\nrun1:\n%s\nrun2:\n%s", text, res2.Text)
	}
}

// TestEmailParser_ChineseEMLRFC2047JSON verifies the JSON path keeps the
// raw text_html field (Python flow-parser _email() contract) while header
// values decode and the re-chunked attachment text is appended as an item.
func TestEmailParser_ChineseEMLRFC2047JSON(t *testing.T) {
	eml := buildChineseEML(t)
	p := newKBEmailParser("json")

	res := p.ParseWithResult(context.Background(), "带附件的email.eml", eml)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Fatalf("expected output_format json, got %q", res.OutputFormat)
	}
	if len(res.JSON) != 2 {
		t.Fatalf("expected 2 JSON items (email + attachment), got %d", len(res.JSON))
	}
	head := res.JSON[0]
	if v, _ := head["subject"].(string); v != "测试主题：项目周报" {
		t.Errorf("subject = %q, want decoded 测试主题：项目周报", v)
	}
	if v, _ := head["from"].(string); !strings.Contains(v, "张三") {
		t.Errorf("from = %q, want decoded 张三", v)
	}
	if v, _ := head["text_html"].(string); !strings.Contains(v, "<div>") {
		t.Errorf("JSON text_html should keep the raw HTML body, got %q", v)
	}
	att := res.JSON[1]
	if v, _ := att["text"].(string); !strings.Contains(v, "这是附件里的文本内容") {
		t.Errorf("attachment item text = %q", v)
	}
}

// TestEmailParser_SinglePartBase64CTE verifies a non-multipart message with
// Content-Transfer-Encoding: base64 decodes its body (mirrors Python's
// msg.get_payload(decode=True) on single-part messages).
func TestEmailParser_SinglePartBase64CTE(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("Hello from a base64 single-part body."))
	raw := strings.Join([]string{
		"From: sender@test.com",
		"To: recipient@test.com",
		"Subject: Single part",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: base64",
		"",
		encoded,
	}, "\r\n")

	p := newKBEmailParser("text")
	res := p.ParseWithResult(context.Background(), "single.eml", []byte(raw))
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(res.Text, "Hello from a base64 single-part body.") {
		t.Errorf("single-part base64 body not decoded:\n%s", res.Text)
	}
}
