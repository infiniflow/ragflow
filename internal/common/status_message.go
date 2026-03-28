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
	ServerTypeAPI       ServerType = "api_server"     // API server
	ServerTypeWorker    ServerType = "ingestor"       // Ingestion server
	ServerTypeScheduler ServerType = "data_collector" // Data collection server
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
