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

func Audit(operationLog *OperationLog, fields ...zap.Field) {
}

// ingestLogger receives the serialized ingest events. It defaults to a no-op so a
// binary that never calls InitIngestLog emits nothing and never panics.
var ingestLogger = zap.NewNop()

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
	// File output is opt-in: only when a directory is configured. A missing
	// Filename falls back to "<service>_ingest.log" so operators only set path.
	if file.Path != "" {
		filename := file.Filename
		if filename == "" {
			filename = fmt.Sprintf("%s_ingest.log", serviceName)
		}
		ljLogger := &lumberjack.Logger{
			Filename:   filepath.Join(file.Path, filename),
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			MaxAge:     maxAge,
			Compress:   compress,
			LocalTime:  true,
		}
		syncers = append(syncers, zapcore.AddSync(ljLogger))
	}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), zapcore.NewMultiWriteSyncer(syncers...), zapcore.InfoLevel)
	ingestLogger = zap.New(core)
	return nil
}

// SyncIngestLog flushes any buffered ingest output. Call it before process exit
// so no events are lost.
func SyncIngestLog() {
	_ = ingestLogger.Sync()
}

// LogIngest transmits one structured ingestion lifecycle event as a single, flat
// JSON object. It is best-effort: a nil/closed sink never panics or fails the
// caller's pipeline.
func LogIngest(log IngestLog) {
	ingestLogger.With(log.zapFields()...).Info("ingest_log")
}

// zapFields renders the event as flat zap fields (top-level keys, UTC times) so
// the emitted JSON line is directly consumable by ClickHouse / Grafana.
func (l IngestLog) zapFields() []zap.Field {
	fields := []zap.Field{
		zap.Time("event_time", l.EventTime.UTC()),
		zap.String("tenant_id", l.TenantID),
		zap.String("dataset_id", l.DatasetID),
		zap.String("doc_id", l.DocID),
		zap.String("user_id", l.UserID),
		zap.String("user_name", l.UserName),
		zap.String("worker", l.Worker),
		zap.String("task_id", l.TaskID),
		zap.String("event_type", l.EventType),
		zap.String("status", l.Status),
		zap.Int("code", l.Code),
		zap.String("filename", l.Filename),
		zap.String("file_type", l.FileType),
		zap.Int64("file_size", l.FileSize),
		zap.String("parser_id", l.ParserID),
		zap.Time("upload_time", l.UploadTime.UTC()),
		zap.Time("enqueue_time", l.EnqueueTime.UTC()),
		zap.Time("begin_time", l.BeginTime.UTC()),
		zap.Time("end_time", l.EndTime.UTC()),
		zap.Int64("duration_ms", l.DurationMS),
		zap.Float64("wait_duration_s", l.WaitDurationS),
		zap.Float64("parse_duration_s", l.ParseDurationS),
		zap.Float64("progress", l.Progress),
		zap.Int("run_count", l.RunCount),
	}
	if l.ErrorInfo != "" {
		fields = append(fields, zap.String("error_info", l.ErrorInfo))
	}
	if len(l.Operation) > 0 {
		fields = append(fields, zap.Any("operation", l.Operation))
	}
	return fields
}
