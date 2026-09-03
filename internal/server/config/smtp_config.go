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

package config

import (
	"ragflow/internal/common"

	"github.com/spf13/viper"
)

func (c *Config) ParseSMTPConfig(v *viper.Viper) error {
	// Default SMTP config
	c.smtp.MailServer = ""
	c.smtp.MailPort = 465
	c.smtp.MailUseSSL = true
	c.smtp.MailUseTLS = false
	c.smtp.MailUsername = ""
	c.smtp.MailPassword = ""
	c.smtp.MailFromName = "RAGFlow"
	c.smtp.MailFromAddress = ""
	c.smtp.MailFrontendURL = "https://your-frontend.example.com"

	if !v.IsSet("smtp") {
		return nil
	}
	sub := v.Sub("smtp")
	if sub == nil {
		return nil
	}

	if sub.IsSet("mail_server") {
		c.smtp.MailServer = sub.GetString("mail_server")
	}

	if sub.IsSet("mail_port") {
		c.smtp.MailPort = sub.GetInt("mail_port")
	}

	if sub.IsSet("mail_use_ssl") {
		c.smtp.MailUseSSL = sub.GetBool("mail_use_ssl")
	}

	if sub.IsSet("mail_use_tls") {
		c.smtp.MailUseTLS = sub.GetBool("mail_use_tls")
	}

	if sub.IsSet("mail_username") {
		c.smtp.MailUsername = sub.GetString("mail_username")
	}

	if sub.IsSet("mail_password") {
		c.smtp.MailPassword = sub.GetString("mail_password")
	}

	if sub.IsSet("mail_from_name") {
		c.smtp.MailFromName = sub.GetString("mail_from_name")
	}

	if sub.IsSet("mail_from_address") {
		c.smtp.MailFromAddress = sub.GetString("mail_from_address")
	}

	if sub.IsSet("mail_frontend_url") {
		c.smtp.MailFrontendURL = sub.GetString("mail_frontend_url")
	}

	return nil
}

func (c *Config) GetSMTPConfig() common.SMTPConfig {
	return c.smtp
}
