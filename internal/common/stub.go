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
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
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

func SyncAudit() {
}

// ingestLogger receives the serialized ingest events. It defaults to a no-op so a
// binary that never calls InitIngestLog emits nothing and never panics.
var ingestLogger atomic.Pointer[zap.Logger]

func init() {
	ingestLogger.Store(zap.NewNop())
}

// InitIngestLog configures the sink for ingestion lifecycle events. It mirrors
// InitLogger: output always goes to stdout, and an optional rotated file is added
// when both Path and Filename are set.
func InitIngestLog(logType string, file FileOutput, serviceName string) error {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "service",
		CallerKey:      "caller",
		FunctionKey:    "",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000-07:00"),
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeName:     zapcore.FullNameEncoder,
	}
	maxSize := file.MaxSize
	if maxSize <= 0 {
		maxSize = DefaultLogMaxSizeMB
	}
	maxBackups := file.MaxBackups
	if maxBackups <= 0 {
		maxBackups = DefaultLogMaxBackups
	}
	maxAge := file.MaxAge
	if maxAge <= 0 {
		maxAge = DefaultLogMaxAgeDays
	}
	compress := DefaultLogCompress
	if file.Compress != nil {
		compress = *file.Compress
	}
	stdoutSyncer := zapcore.AddSync(os.Stdout)
	syncers := []zapcore.WriteSyncer{stdoutSyncer}
	// File output is opt-in via a non-empty Path. Path is treated as the log FILE
	// (matching conf/service_conf.yaml's "path is the file" wording): a stray
	// directory named after the file is never created. Only when Path is itself an
	// existing directory do we synthesize a per-service file name inside it.
	if file.Path != "" {
		filename := file.Path
		if fi, statErr := os.Stat(file.Path); statErr == nil && fi.IsDir() {
			name := file.Filename
			if name == "" {
				name = fmt.Sprintf("%s_ingest.log", serviceName)
			}
			filename = filepath.Join(file.Path, name)
		}
		ljLogger := &lumberjack.Logger{
			Filename:   filename,
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			MaxAge:     maxAge,
			Compress:   compress,
			LocalTime:  true,
		}
		syncers = append(syncers, zapcore.AddSync(ljLogger))
	}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), zapcore.NewMultiWriteSyncer(syncers...), zapcore.InfoLevel)
	oldLogger := ingestLogger.Swap(zap.New(core))
	if oldLogger != nil {
		_ = oldLogger.Sync()
	}
	return nil
}

type IngestionLog struct {
	EventTime time.Time `json:"event_time"`
	TenantID  string    `json:"tenant_id"`
	DatasetID string    `json:"dataset_id"`
	DocID     string    `json:"doc_id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name,omitempty"`
	Worker    string    `json:"worker"`
	TaskID    string    `json:"task_id"`
	EventType string    `json:"event_type"` // created|scheduled|running|completed|failed|stopped
	Status    string    `json:"status"`
	// ErrorInfo is the terminal reason as a plain string (not an error
	// interface) so the event serializes cleanly and portably to external
	// dashboards / ClickHouse.
	ErrorInfo      string    `json:"error_info,omitempty"`
	Filename       string    `json:"filename"`
	FileType       string    `json:"file_type"`
	FileSize       int64     `json:"file_size"`
	ParserID       string    `json:"parser_id"`
	UploadTime     time.Time `json:"upload_time"`
	EnqueueTime    time.Time `json:"enqueue_time"`
	BeginTime      time.Time `json:"begin_time"`
	EndTime        time.Time `json:"end_time"`
	DurationMS     int64     `json:"duration_ms"`
	WaitDurationS  float64   `json:"wait_duration_s"`
	ParseDurationS float64   `json:"parse_duration_s"`
	Progress       float64   `json:"progress"`
	RunCount       int       `json:"run_count"`
	// Code is a machine-readable category for grouping in dashboards. It
	// defaults to 0 when the caller does not set it.
	Code      int            `json:"code"`
	Operation map[string]any `json:"operation,omitempty"`
}

// SyncIngestLog flushes any buffered ingest output. Call it before process exit
// so no events are lost.
func SyncIngestLog() {
	_ = ingestLogger.Load().Sync()
}

// LogIngestEvent transmits one structured ingestion lifecycle event as a single,
// flat JSON line. In the OSS build it writes to the local zap sink and also
// forwards to the shared LogIngest seam (a no-op in OSS, the real sink in EE).
func LogIngestEvent(log IngestionLog) {
	ingestLogger.Load().With(log.zapFields()...).Info("ingest_log")
	LogIngest(&log)
}

// zapFields renders the event as flat zap fields (top-level keys, UTC times) so
// the emitted JSON line is directly consumable by ClickHouse / Grafana.
func (l IngestionLog) zapFields() []zap.Field {
	fields := []zap.Field{
		zap.Time("event_time", l.EventTime.UTC()),
		zap.String("tenant_id", l.TenantID),
		zap.String("dataset_id", l.DatasetID),
		zap.String("doc_id", l.DocID),
		zap.String("user_id", l.UserID),
		zap.String("worker", l.Worker),
		zap.String("task_id", l.TaskID),
		zap.String("event_type", l.EventType),
		zap.String("status", l.Status),
		zap.String("filename", l.Filename),
		zap.String("file_type", l.FileType),
		zap.Int64("file_size", l.FileSize),
		zap.String("parser_id", l.ParserID),
		skipZeroTime("upload_time", l.UploadTime),
		skipZeroTime("enqueue_time", l.EnqueueTime),
		skipZeroTime("begin_time", l.BeginTime),
		skipZeroTime("end_time", l.EndTime),
		zap.Int64("duration_ms", l.DurationMS),
		zap.Float64("wait_duration_s", l.WaitDurationS),
		zap.Float64("parse_duration_s", l.ParseDurationS),
		zap.Float64("progress", l.Progress),
		zap.Int("run_count", l.RunCount),
		zap.Int("code", l.Code),
	}
	if l.UserName != "" {
		fields = append(fields, zap.String("user_name", l.UserName))
	}
	if l.ErrorInfo != "" {
		fields = append(fields, zap.String("error_info", l.ErrorInfo))
	}
	if len(l.Operation) > 0 {
		fields = append(fields, zap.Any("operation", l.Operation))
	}
	return fields
}

// skipZeroTime omits a time field from the emitted JSON when it is the zero
// value, keeping the line compact and ClickHouse-friendly.
func skipZeroTime(key string, t time.Time) zap.Field {
	if t.IsZero() {
		return zap.Skip()
	}
	return zap.Time(key, t.UTC())
}

// LogIngest is the shared upstream seam. In the OSS build it is a no-op; the EE
// build provides the real sink. Our OSS LogIngestEvent forwards to it.
func LogIngest(log *IngestionLog, fields ...zap.Field) {}
