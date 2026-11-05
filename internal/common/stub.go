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

//go:build !ee

package common

import (
	"time"

	"go.uber.org/zap"
)

var (
	OpenSourceVersion = "open source"
	EnterpriseEdition = "enterprise edition"
)

// GetRAGFlowType reports the edition this binary was built for.
func GetRAGFlowType() string {
	return OpenSourceVersion
}

func InitAudit(logType string, file FileOutput, serviceName string) error {
	return nil
}

type OperationLog struct {
	EventTime     time.Time      `json:"event_time"`
	UserID        string         `json:"user_id"`
	IPAddress     string         `json:"ip_address"`
	Operation     string         `json:"operation"`
	APIPath       string         `json:"api_path"`
	HTTPMethod    string         `json:"http_method"`
	RequestParams string         `json:"request_params"`
	Resources     map[string]any `json:"resources"`
	Code          int            `json:"code"`
	Error         error          `json:"error"`
	DurationMS    int64          `json:"duration_ms"`
}

func Audit(operationLog *OperationLog, fields ...zap.Field) {
}

// SyncAudit is the OSS no-op flush for the audit logger. The real sink lives in
// the EE build; the OSS binary keeps only the instrumentation seam so the
// server entrypoint's deferred flush (cmd/ragflow_server.go) compiles and runs.
func SyncAudit() {
}
