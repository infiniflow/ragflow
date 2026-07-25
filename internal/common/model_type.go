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

import (
	"fmt"
	"time"
)

type ModelUsage struct {
	UserID         string    `json:"user_id"`
	UserEmail      string    `json:"user_email"`
	TenantID       string    `json:"tenant_id"`
	TenantEmail    string    `json:"tenant_email"`
	GroupID        string    `json:"group_id"`
	GroupName      string    `json:"group_name"`
	AppID          string    `json:"app_id"`
	AppName        string    `json:"app_name"`
	SessionID      string    `json:"session_id"`
	ProviderName   string    `json:"provider_name"`
	InstanceID     string    `json:"instance_id"`
	APIKey         string    `json:"api_key"`
	ModelName      string    `json:"model_name"`
	Type           string    `json:"type"`
	RequestID      string    `json:"request_id"`
	StartAt        time.Time `json:"start_at"`
	ResponseTimeMS int64     `json:"response_time_ms"`
	ErrorMessage   string    `json:"error_message"`

	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (m *ModelUsage) String() string {
	return fmt.Sprintf("%#v", m)
}

type OperationLog struct {
	EventTime     time.Time `json:"event_time"`
	UserID        string    `json:"user_id"`
	IPAddress     string    `json:"ip_address"`
	Operation     string    `json:"operation"`
	APIPath       string    `json:"api_path"`
	HTTPMethod    string    `json:"http_method"`
	ResourceType  string    `json:"resource_type"`
	ResourceID    string    `json:"resource_id"`
	ResourceName  string    `json:"resource_name"`
	RequestParams string    `json:"request_params"`
	ErrorCode     uint16    `json:"error_code"`
	Message       string    `json:"message"`
	DurationMS    int64     `json:"duration_ms"`
}
