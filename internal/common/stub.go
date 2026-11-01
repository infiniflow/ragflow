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
	EventTime     time.Time `json:"event_time"`
	UserID        string    `json:"user_id"`
	UserName      string    `json:"user_name"`
	IPAddress     string    `json:"ip_address"`
	Operation     string    `json:"operation"`
	APIPath       string    `json:"api_path"`
	HTTPMethod    string    `json:"http_method"`
	ResourceType  string    `json:"resource_type"`
	ResourceID    string    `json:"resource_id"`
	ResourceName  string    `json:"resource_name"`
	RequestParams string    `json:"request_params"`
	Code          int       `json:"code"`
	Error         error     `json:"error"`
	DurationMS    int64     `json:"duration_ms"`
}

func Audit(operationLog *OperationLog, fields ...zap.Field) {
}

func InitIngestLog(logType string, file FileOutput, serviceName string) error {
	return nil
}

type IngestionLog struct {
	EventTime  time.Time      `json:"event_time"`
	UserID     string         `json:"user_id"`
	UserName   string         `json:"user_name"`
	DocumentID string         `json:"document_id"`
	Worker     string         `json:"worker"`
	TaskID     string         `json:"task_id"`
	TaskType   string         `json:"task_type"`
	Filename   string         `json:"filename"`
	Code       int            `json:"code"`
	Error      error          `json:"error"`
	DurationMS int64          `json:"duration_ms"`
	Operation  map[string]any `json:"operation"`
}

func LogIngest(log *IngestionLog, fields ...zap.Field) {}
