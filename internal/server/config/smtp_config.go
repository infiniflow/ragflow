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

import "github.com/spf13/viper"

//type SMTPConfig struct {
//	MailServer      string `mapstructure:"mail_server"`
//	MailPort        int    `mapstructure:"mail_port"`
//	MailUseSSL      bool   `mapstructure:"mail_use_ssl"`
//	MailUseTLS      bool   `mapstructure:"mail_use_tls"`
//	MailUsername    string `mapstructure:"mail_username"`
//	MailPassword    string `mapstructure:"mail_password"`
//	MailFromName    string `mapstructure:"mail_from_name"`
//	MailFromAddress string `mapstructure:"mail_from_address"`
//	MailFrontendURL string `mapstructure:"mail_frontend_url"`
//}

func ParseSMTPConfig(config *Config, v *viper.Viper) error {
	// Default SMTP config
	config.SMTP.MailServer = ""
	config.SMTP.MailPort = 465
	config.SMTP.MailUseSSL = true
	config.SMTP.MailUseTLS = false
	config.SMTP.MailUsername = ""
	config.SMTP.MailPassword = ""
	config.SMTP.MailFromName = "RAGFlow"
	config.SMTP.MailFromAddress = ""
	config.SMTP.MailFrontendURL = "https://your-frontend.example.com"

	if !v.IsSet("smtp") {
		return nil
	}
	sub := v.Sub("smtp")
	if sub == nil {
		return nil
	}

	if sub.IsSet("mail_server") {
		config.SMTP.MailServer = sub.GetString("mail_server")
	}

	if sub.IsSet("mail_port") {
		config.SMTP.MailPort = sub.GetInt("mail_port")
	}

	if sub.IsSet("mail_use_ssl") {
		config.SMTP.MailUseSSL = sub.GetBool("mail_use_ssl")
	}

	if sub.IsSet("mail_use_tls") {
		config.SMTP.MailUseTLS = sub.GetBool("mail_use_tls")
	}

	if sub.IsSet("mail_username") {
		config.SMTP.MailUsername = sub.GetString("mail_username")
	}

	if sub.IsSet("mail_password") {
		config.SMTP.MailPassword = sub.GetString("mail_password")
	}

	if sub.IsSet("mail_from_name") {
		config.SMTP.MailFromName = sub.GetString("mail_from_name")
	}

	if sub.IsSet("mail_from_address") {
		config.SMTP.MailFromAddress = sub.GetString("mail_from_address")
	}

	if sub.IsSet("mail_frontend_url") {
		config.SMTP.MailFrontendURL = sub.GetString("mail_frontend_url")
	}

	return nil
}
