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

// Minimal SMTP sender for transactional email (forgot-password OTP, etc).
// Mirrors api/utils/web_utils.py:send_email_html on the Python side and
// uses the same conf/service_conf.yaml `smtp` block so a single config
// powers both backends.
//
// The config is passed in as a parameter rather than read via
// server.GetConfig() — internal/server already imports internal/utility
// (via variable.go), so importing server from here would close an
// import cycle. The SMTPConfig type lives in internal/common for the
// same reason.
package utility

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"

	"ragflow/internal/common"

	"go.uber.org/zap"
)

// SMTPNotConfiguredError is returned when an SMTP send is attempted but the
// active config has no mail server. Lets the caller distinguish a config
// problem from a transient delivery failure.
type SMTPNotConfiguredError struct{}

func (SMTPNotConfiguredError) Error() string {
	return "smtp is not configured"
}

// SMTPInsecureAuthError is returned when authentication is requested over
// an unencrypted SMTP connection (neither MailUseSSL nor MailUseTLS set).
// Sending credentials in the clear is refused on principle.
type SMTPInsecureAuthError struct{}

func (SMTPInsecureAuthError) Error() string {
	return "smtp authentication refused over plaintext connection (set mail_use_ssl or mail_use_tls)"
}

// SendResetCodeEmail delivers the password-reset OTP email. It is the Go
// analogue of:
//
//	await send_email_html(
//	    subject="Your Password Reset Code",
//	    to_email=email,
//	    template_key="reset_code",
//	    code=otp,
//	    ttl_min=ttl_min,
//	)
//
// — same subject, same plaintext body shape (see RESET_CODE_EMAIL_TMPL in
// api/utils/email_templates.py).
func SendResetCodeEmail(cfg common.SMTPConfig, toEmail, otp string, ttlMinutes int) error {
	if cfg.MailServer == "" || cfg.MailPort == 0 {
		return SMTPNotConfiguredError{}
	}

	subject := "Your Password Reset Code"
	body := fmt.Sprintf(
		"Hello,\nYour password reset code is: %s\nThis code will expire in %d minutes.\n",
		otp, ttlMinutes,
	)

	fromAddr := cfg.MailFromAddress
	if fromAddr == "" {
		fromAddr = cfg.MailUsername
	}
	fromName := cfg.MailFromName
	if fromName == "" {
		fromName = "RAGFlow"
	}
	fromHeader := fmt.Sprintf("%s <%s>", fromName, fromAddr)

	msg := buildPlainEmail(fromHeader, toEmail, subject, body)
	if err := sendMail(cfg, fromAddr, toEmail, msg); err != nil {
		common.Warn("smtp send failed",
			zap.String("to", toEmail),
			zap.String("server", cfg.MailServer),
			zap.Int("port", cfg.MailPort),
			zap.Error(err),
		)
		return err
	}
	return nil
}

// buildPlainEmail composes an RFC 5322 plain-text message. CRLF line
// endings are required by the SMTP DATA spec.
func buildPlainEmail(from, to, subject, body string) []byte {
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: 8bit",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body)
}

// sendMail dispatches the message over implicit TLS, STARTTLS, or plain
// — matching how the Python aiosmtplib client is configured by the
// `mail_use_ssl` / `mail_use_tls` flags.
//
// Authentication is only attempted over an encrypted session. If the
// caller asks for auth (MailUsername set) on a plaintext connection,
// SMTPInsecureAuthError is returned before any credential is written.
func sendMail(cfg common.SMTPConfig, from, to string, msg []byte) error {
	if cfg.MailUsername != "" && !cfg.MailUseSSL && !cfg.MailUseTLS {
		return SMTPInsecureAuthError{}
	}

	addr := net.JoinHostPort(cfg.MailServer, fmt.Sprintf("%d", cfg.MailPort))
	auth := smtp.PlainAuth("", cfg.MailUsername, cfg.MailPassword, cfg.MailServer)

	if cfg.MailUseSSL {
		// Implicit TLS (typical port 465). Dial TLS first, then SMTP.
		tlsCfg := &tls.Config{
			ServerName: cfg.MailServer,
			MinVersion: tls.VersionTLS12,
		}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("smtp tls dial: %w", err)
		}
		client, err := smtp.NewClient(conn, cfg.MailServer)
		if err != nil {
			conn.Close()
			return fmt.Errorf("smtp client init: %w", err)
		}
		defer client.Quit()
		if cfg.MailUsername != "" {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
		return deliverMail(client, from, to, msg)
	}

	// STARTTLS (typical port 587) or plain (auth refused above).
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer client.Quit()
	if cfg.MailUseTLS {
		tlsCfg := &tls.Config{
			ServerName: cfg.MailServer,
			MinVersion: tls.VersionTLS12,
		}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
		if cfg.MailUsername != "" {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
	}
	// Plaintext: no auth performed (refused at the top of the function).
	return deliverMail(client, from, to, msg)
}

func deliverMail(client *smtp.Client, from, to string, msg []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail-from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt-to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		w.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return nil
}
