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

package common

// SMTPConfig is the SMTP block from conf/service_conf.yaml, used by the
// forgot-password OTP flow and any other transactional email path.
//
// It lives in internal/common rather than internal/server so that
// internal/utility (which renders/sends the email) can reference the
// type without importing internal/server. internal/server already
// imports internal/utility (via variable.go), so the reverse import
// would close an import cycle.
type SMTPConfig struct {
	MailServer      string `mapstructure:"mail_server"`
	MailPort        int    `mapstructure:"mail_port"`
	MailUseSSL      bool   `mapstructure:"mail_use_ssl"`
	MailUseTLS      bool   `mapstructure:"mail_use_tls"`
	MailUsername    string `mapstructure:"mail_username"`
	MailPassword    string `mapstructure:"mail_password"`
	MailFromName    string `mapstructure:"mail_from_name"`
	MailFromAddress string `mapstructure:"mail_from_address"`
	MailFrontendURL string `mapstructure:"mail_frontend_url"`
}
