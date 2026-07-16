//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package tool

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"ragflow/internal/agent/runtime"
)

const emailToolName = "email"

const emailToolDescription = "Send an email via SMTP. Returns success/failure status."

const (
	emailDialTimeout    = 10 * time.Second
	emailSessionTimeout = 30 * time.Second
)

// emailParams contains both the Python model-call inputs and the Canvas node
// configuration used to deliver the message. Info exposes only the former.
type emailParams struct {
	SMTPServer   string `json:"smtp_server"`
	SMTPPort     int    `json:"smtp_port"`
	Email        string `json:"email"`
	SMTPUsername string `json:"smtp_username"`
	Password     string `json:"password"`
	SenderName   string `json:"sender_name"`
	ToEmail      string `json:"to_email"`
	CCEmail      string `json:"cc_email"`
	Content      string `json:"content"`
	Subject      string `json:"subject"`
}

// emailEnvelope is what the model sees.
type emailEnvelope struct {
	OK    bool   `json:"ok"`
	Error string `json:"_ERROR,omitempty"`
}

// EmailTool is the SMTP email
// sender tool. It composes an
// RFC 822 message and submits it via the stdlib net/smtp client. All
// authentication modes supported by net/smtp.Auth are available
// (PLAIN, LOGIN, CRAM-MD5) by selecting the appropriate creds.
type EmailTool struct {
	defaults emailParams
}

var _ ToolComponent = (*EmailTool)(nil)

// NewEmailTool returns an EmailTool. There is no shared HTTPHelper
// (SMTP is not HTTP), so the constructor is the simplest possible.
func NewEmailTool() *EmailTool {
	return newEmailTool(emailParams{SMTPPort: 465})
}

func newEmailTool(defaults emailParams) *EmailTool {
	if defaults.SMTPPort == 0 {
		defaults.SMTPPort = 465
	}
	return &EmailTool{defaults: defaults}
}

// ToolMeta returns the tool's metadata for the chat model.
func (e *EmailTool) ToolMeta() ToolMeta {
	return ToolMeta{
		Name:        emailToolName,
		Description: emailToolDescription,
		Parameters: map[string]ParameterInfo{
			"to_email": {
				Type:        ParamTypeString,
				Description: "Recipient email address list.",
				Required:    true,
			},
			"cc_email": {
				Type:        ParamTypeString,
				Description: "Optional CC recipient list.",
				Required:    false,
			},
			"subject": {
				Type:        ParamTypeString,
				Description: "Email subject line.",
				Required:    true,
			},
			"content": {
				Type:        ParamTypeString,
				Description: "Email body (plain text).",
				Required:    true,
			},
		},
	}
}

func (e *EmailTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"to_email": "Recipient email address list.",
			"cc_email": "Optional CC recipient list.",
			"content":  "Email body.",
			"subject":  "Email subject.",
		},
		Outputs: map[string]string{
			"success": "Whether the email was sent successfully.",
		},
		InputForm: map[string]any{
			"to_email": map[string]any{"name": "To ", "type": "line"},
			"subject":  map[string]any{"name": "Subject", "type": "line", "optional": true},
			"cc_email": map[string]any{"name": "CC To", "type": "line", "optional": true},
		},
	}
}

// buildEmailMessage composes the RFC 822 wire format: headers + blank
// line + body. Extracted so tests can verify subject / recipient
// inclusion without opening a real socket.
func buildEmailMessage(from, senderName string, to, cc []string, subject, body string) []byte {
	var b strings.Builder
	fromHeader := (&mail.Address{
		Name:    stripEmailHeaderLineBreaks(senderName),
		Address: stripEmailHeaderLineBreaks(from),
	}).String()
	b.WriteString("From: " + fromHeader + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	if len(cc) > 0 {
		b.WriteString("Cc: " + strings.Join(cc, ", ") + "\r\n")
	}
	b.WriteString("Subject: " + stripEmailHeaderLineBreaks(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	return []byte(b.String())
}

func stripEmailHeaderLineBreaks(value string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(value)
}

type emailSender func(ctx context.Context, p emailParams, msg []byte) error

var sendEmail = sendEmailSMTP

// InvokableRun sends the email. We delegate to smtp.SendMail which
// handles EHLO, STARTTLS, and AUTH transparently when an *smtp.Auth is
// supplied; with nil auth it sends unauthenticated.
func (e *EmailTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
	var p emailParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return emailErrJSON(fmt.Errorf("email: parse arguments: %w", err)),
			fmt.Errorf("email: parse arguments: %w", err)
	}
	// Apply defaults for fields not present in runtime args.
	if p.SMTPServer == "" {
		p.SMTPServer = e.defaults.SMTPServer
	}
	if p.SMTPPort == 0 {
		p.SMTPPort = e.defaults.SMTPPort
	}
	if p.Email == "" {
		p.Email = e.defaults.Email
	}
	if p.Password == "" {
		p.Password = e.defaults.Password
	}
	if p.SenderName == "" {
		p.SenderName = e.defaults.SenderName
	}
	state, _, _ := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	p.ToEmail = runtime.ResolveTemplateForDisplay(p.ToEmail, state)
	p.CCEmail = runtime.ResolveTemplateForDisplay(p.CCEmail, state)
	p.Subject = stripEmailHeaderLineBreaks(runtime.ResolveTemplateForDisplay(p.Subject, state))
	p.Content = runtime.ResolveTemplateForDisplay(p.Content, state)
	if err := validateEmailParams(&p); err != nil {
		return emailErrJSON(err), err
	}

	toRecipients := splitEmailList(p.ToEmail)
	ccRecipients := splitEmailList(p.CCEmail)
	if len(toRecipients) == 0 {
		err := fmt.Errorf("email: to_email is required")
		return emailErrJSON(err), err
	}
	subject := p.Subject
	if subject == "" {
		subject = "No Subject"
	}
	content := p.Content
	if content == "" {
		content = "No content provided"
	}
	msg := buildEmailMessage(p.Email, p.SenderName, toRecipients, ccRecipients, subject, content)
	if err := sendEmail(ctx, p, msg); err != nil {
		return emailErrJSON(fmt.Errorf("email: send: %w", err)),
			fmt.Errorf("email: send: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return emailErrJSON(err), err
	}
	return emailJSON(emailEnvelope{OK: true}), nil
}

func (e *EmailTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	ok, _ := envelope["ok"].(bool)
	return map[string]any{"success": ok}
}

func sendEmailSMTP(ctx context.Context, p emailParams, msg []byte) error {
	if p.SMTPPort == 465 {
		return sendEmailSMTPS(ctx, p, msg)
	}
	return sendEmailSTARTTLS(ctx, p, msg)
}

func sendEmailSTARTTLS(ctx context.Context, p emailParams, msg []byte) error {
	addr := fmt.Sprintf("%s:%d", p.SMTPServer, p.SMTPPort)
	dialer := &net.Dialer{Timeout: emailDialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	setEmailDeadline(ctx, conn)
	stopWatch := watchEmailContext(ctx, conn)
	defer stopWatch()

	client, err := smtp.NewClient(conn, p.SMTPServer)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Hello("localhost"); err != nil {
		return err
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return fmt.Errorf("email: SMTP server does not advertise STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{ServerName: p.SMTPServer, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	return submitEmail(ctx, client, p, msg)
}

func sendEmailSMTPS(ctx context.Context, p emailParams, msg []byte) error {
	addr := fmt.Sprintf("%s:%d", p.SMTPServer, p.SMTPPort)
	dialer := &net.Dialer{Timeout: emailDialTimeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	setEmailDeadline(ctx, rawConn)
	stopWatch := watchEmailContext(ctx, rawConn)
	defer stopWatch()

	conn := tls.Client(rawConn, &tls.Config{ServerName: p.SMTPServer, MinVersion: tls.VersionTLS12})
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, p.SMTPServer)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Hello("localhost"); err != nil {
		return err
	}
	return submitEmail(ctx, client, p, msg)
}

func submitEmail(ctx context.Context, client *smtp.Client, p emailParams, msg []byte) error {
	username := p.SMTPUsername
	if username == "" {
		username = p.Email
	}
	if username != "" && p.Password != "" {
		if err := client.Auth(smtp.PlainAuth("", username, p.Password, p.SMTPServer)); err != nil {
			return err
		}
	}
	if err := client.Mail(p.Email); err != nil {
		return err
	}
	for _, addr := range emailRecipients(p.ToEmail, p.CCEmail) {
		if err := client.Rcpt(addr); err != nil {
			return err
		}
	}
	wc, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		_ = wc.Close()
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := client.Quit(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func setEmailDeadline(ctx context.Context, conn net.Conn) {
	deadline := time.Now().Add(emailSessionTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
}

func watchEmailContext(ctx context.Context, conn net.Conn) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-done:
		}
	}()
	return func() { close(done) }
}

// validateEmailParams guards against obviously broken inputs. The
// upstream SMTP server will give a better error for malformed
// addresses, but the common case (empty / missing) is caught here.
func validateEmailParams(p *emailParams) error {
	switch {
	case p.SMTPServer == "":
		return fmt.Errorf("email: smtp_server is required")
	case p.SMTPPort <= 0 || p.SMTPPort > 65535:
		return fmt.Errorf("email: smtp_port must be in [1, 65535]")
	case p.Email == "":
		return fmt.Errorf("email: email is required")
	}
	return nil
}

func emailRecipients(toEmail, ccEmail string) []string {
	recipients := splitEmailList(toEmail)
	return append(recipients, splitEmailList(ccEmail)...)
}

func splitEmailList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if recipient := strings.TrimSpace(part); recipient != "" {
			out = append(out, recipient)
		}
	}
	return out
}

func emailJSON(env emailEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"email: marshal result: %s"}`, err)
	}
	return string(b)
}

func emailErrJSON(err error) string {
	return emailJSON(emailEnvelope{Error: err.Error()})
}
