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

package handler

import (
	"crypto/tls"
	"fmt"
	"net/mail"
	"net/smtp"
	"ragflow/internal/server"
	"strings"
)

var sendTenantInviteEmail = deliverTenantInviteEmail

func deliverTenantInviteEmail(toEmail, recipientEmail, tenantID, inviter string) error {
	cfg := server.GetConfig()
	if cfg == nil {
		return fmt.Errorf("server config is not initialized")
	}

	smtpCfg := cfg.SMTP
	if smtpCfg.MailServer == "" || smtpCfg.MailPort == 0 {
		return fmt.Errorf("smtp config is incomplete")
	}
	if len(smtpCfg.MailDefaultSender) < 2 {
		return fmt.Errorf("mail_default_sender must include display name and email address")
	}

	from := mail.Address{
		Name:    smtpCfg.MailDefaultSender[0],
		Address: smtpCfg.MailDefaultSender[1],
	}
	to := mail.Address{Address: toEmail}
	subject := "RAGFlow Invitation"
	body := fmt.Sprintf(
		"Hi %s,\n%s has invited you to join their team (ID: %s).\nClick the link below to complete your registration:\n%s\nIf you did not request this, please ignore this email.\n",
		recipientEmail,
		inviter,
		tenantID,
		smtpCfg.MailFrontendURL,
	)
	message := strings.Join([]string{
		fmt.Sprintf("From: %s", from.String()),
		fmt.Sprintf("To: %s", to.String()),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	address := fmt.Sprintf("%s:%d", smtpCfg.MailServer, smtpCfg.MailPort)
	var auth smtp.Auth
	if smtpCfg.MailUsername != "" || smtpCfg.MailPassword != "" {
		auth = smtp.PlainAuth("", smtpCfg.MailUsername, smtpCfg.MailPassword, smtpCfg.MailServer)
	}

	if smtpCfg.MailUseSSL {
		return sendMailWithTLS(address, smtpCfg.MailServer, auth, from.Address, []string{to.Address}, []byte(message))
	}

	client, err := smtp.Dial(address)
	if err != nil {
		return err
	}
	defer client.Close()

	if smtpCfg.MailUseTLS {
		tlsConfig := &tls.Config{ServerName: smtpCfg.MailServer}
		if err := client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return err
	}
	if err := client.Rcpt(to.Address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte(message)); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func sendMailWithTLS(addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(msg); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
