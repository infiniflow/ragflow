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

// InitIngestLog is the OSS no-op hook for ingestion lifecycle event sinks. The OSS
// build does not collect these events; the real sink lives in the EE build via the
// LogIngest seam. The signature is retained so the IngestionEventEmitter /
// SetEventEmitter wiring in the ingestion service keeps compiling.
func InitIngestLog(logType string, file FileOutput, serviceName string) error {
	return nil
}

type IngestionLog struct {
	EventTime      time.Time      `json:"event_time"`
	TenantID       string         `json:"tenant_id"`
	DatasetID      string         `json:"dataset_id"`
	DocID          string         `json:"doc_id"`
	UserID         string         `json:"user_id"`
	UserName       string         `json:"user_name,omitempty"`
	TenantName     string         `json:"tenant_name,omitempty"`
	Worker         string         `json:"worker"`
	TaskID         string         `json:"task_id"`
	EventType      string         `json:"event_type"`
	Status         string         `json:"status"`
	ErrorInfo      string         `json:"error_info,omitempty"`
	Filename       string         `json:"filename"`
	FileType       string         `json:"file_type"`
	FileSize       int64          `json:"file_size"`
	ParserID       string         `json:"parser_id"`
	UploadTime     time.Time      `json:"upload_time"`
	EnqueueTime    time.Time      `json:"enqueue_time"`
	BeginTime      time.Time      `json:"begin_time"`
	EndTime        time.Time      `json:"end_time"`
	DurationMS     int64          `json:"duration_ms"`
	WaitDurationS  float64        `json:"wait_duration_s"`
	ParseDurationS float64        `json:"parse_duration_s"`
	Progress       float64        `json:"progress"`
	RunCount       int            `json:"run_count"`
	Code           int            `json:"code"`
	Operation      map[string]any `json:"operation,omitempty"`
}

func SyncIngestLog() {

}

// LogIngest is the shared upstream seam. In the OSS build it is a no-op; the EE
// build provides the real sink.
func LogIngest(log *IngestionLog, fields ...zap.Field) {}
