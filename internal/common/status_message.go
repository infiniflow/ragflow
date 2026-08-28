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
	"time"
)

type MessageType string

const (
	MessageHeartbeat MessageType = "heartbeat"
	MessageMetric    MessageType = "metric"
	MessageEvent     MessageType = "event"
)

type ServerType string

const (
	ServerTypeAPI        ServerType = "api_server"  // API server
	ServerTypeIngestion  ServerType = "ingestor"    // Ingestion server
	ServerTypeFileSyncer ServerType = "file_syncer" // File syncer server
)

type BaseMessage struct {
	MessageID   int64       `json:"report_id"`
	MessageType MessageType `json:"report_type"`
	ServerName  string      `json:"server_id"`
	ServerType  ServerType  `json:"server_type"`
	Host        string      `json:"host"`
	Port        int         `json:"port"`
	Version     string      `json:"version"`
	Timestamp   time.Time   `json:"timestamp"`
	Ext         interface{} `json:"ext,omitempty"`
}

type StartIngestionRequest struct {
	TaskID   string `json:"task_id" binding:"required"`
	TaskType string `json:"task_type" binding:"required"`
	From     string `json:"from" binding:"required"`
	UserID   string `json:"user_id" binding:"required"`
}
