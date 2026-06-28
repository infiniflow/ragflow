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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	Logger      *zap.Logger
	Sugar       *zap.SugaredLogger
	atomicLevel zap.AtomicLevel
)

// FileOutput describes the rotated log file destination.
//
// Path is required to enable file output; empty disables the file destination
// (stdout only). When Path is set, the file is written under ./logs/<Path>
// and rotated by lumberjack according to MaxSize / MaxBackups / MaxAge / Compress.
//
// Numeric zero values are replaced with defaults (100 MB / 10 / 30 days) inside
// Init. Compress is left as the caller-provided value; the project default is
// applied by callers (see resolveCompress) so that "not set" can be distinguished
// from "explicitly false" via the *bool LogConfig.Compress field.
type FileOutput struct {
	Path       string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
}

const (
	defaultMaxSizeMB  = 100
	defaultMaxBackups = 10
	defaultMaxAgeDays = 30
)

func parseZapLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "fatal":
		return zapcore.FatalLevel, nil
	case "panic":
		return zapcore.PanicLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unknown log level: %s", level)
	}
}

func logLevelName(level zapcore.Level) string {
	if level == zapcore.WarnLevel {
		return "WARNING"
	}
	return strings.ToUpper(level.String())
}

// Init initializes the global logger. stdout is always written. If file.Path
// is non-empty, a rotated file is also written via lumberjack.
//
// Callers should pass a non-empty Path so that file logging is preserved
// (each binary's hardcoded default goes through this parameter). The empty
// path case is reserved for CLI mode where stdout is the only output.
//
// Numeric fields (MaxSize, MaxBackups, MaxAge) are defaulted to 100/10/30
// when zero. Compress is taken as supplied.
func Init(level string, file FileOutput) error {
	zapLevel, err := parseZapLevel(level)
	if err != nil {
		zapLevel = zapcore.InfoLevel
	}

	atomicLevel = zap.NewAtomicLevelAt(zapLevel)

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:       "timestamp",
		LevelKey:      "level",
		NameKey:       "logger",
		CallerKey:     "",
		FunctionKey:   "",
		MessageKey:    "msg",
		StacktraceKey: "stacktrace",
		LineEnding:    zapcore.DefaultLineEnding,
		EncodeLevel:   zapcore.LowercaseLevelEncoder,
		// RFC 3339 with fixed-width millisecond precision and explicit
		// timezone offset (UTC rendered as "Z", other zones as "+HH:MM"
		// / "-HH:MM"). Easier to ingest than the default "2006-01-02
		// 15:04:05" layout — which had no ms and no zone — and avoids
		// the variable-width output of RFC3339Nano.
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02T15:04:05.000Z07:00"),
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	maxSize := file.MaxSize
	if maxSize <= 0 {
		maxSize = defaultMaxSizeMB
	}
	maxBackups := file.MaxBackups
	if maxBackups <= 0 {
		maxBackups = defaultMaxBackups
	}
	maxAge := file.MaxAge
	if maxAge <= 0 {
		maxAge = defaultMaxAgeDays
	}

	syncers := []zapcore.WriteSyncer{zapcore.AddSync(os.Stdout)}
	if file.Path != "" {
		ljLogger := &lumberjack.Logger{
			Filename:   filepath.Join("logs", file.Path),
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			MaxAge:     maxAge,
			Compress:   file.Compress,
			LocalTime:  true,
		}
		syncers = append(syncers, zapcore.AddSync(ljLogger))
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zap.CombineWriteSyncers(syncers...),
		atomicLevel,
	)

	Logger = zap.New(core, zap.AddCallerSkip(1))
	Sugar = Logger.Sugar()

	return nil
}

// Sync flushes any buffered log entries.
func Sync() {
	if Logger != nil {
		_ = Logger.Sync()
	}
}

// Fatal logs a fatal message using zap with caller info, then calls os.Exit(1).
func Fatal(msg string, fields ...zap.Field) {
	if Logger == nil {
		panic("logger not initialized")
	}
	_, file, line, ok := runtime.Caller(1)
	if ok {
		fields = append(fields, zap.String("caller", fmt.Sprintf("%s:%d", file, line)))
	}
	Logger.Fatal(msg, fields...)
}

// Info logs an info message.
func Info(msg string, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	Logger.Info(msg, fields...)
}

// Error logs an error message. err may be nil; if non-nil it is appended as
// a zap.Error field. Additional fields follow.
func Error(msg string, err error, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	Logger.Error(msg, fields...)
}

// Debug logs a debug message.
func Debug(msg string, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	Logger.Debug(msg, fields...)
}

// Warn logs a warning message.
func Warn(msg string, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	Logger.Warn(msg, fields...)
}

// IsDebugEnabled returns true if debug logging is enabled.
func IsDebugEnabled() bool {
	return atomicLevel.Enabled(zapcore.DebugLevel)
}

// GetLevel returns the current log level.
func GetLevel() string {
	return atomicLevel.String()
}

// SetLevel sets the log level at runtime.
func SetLevel(level string) error {
	zapLevel, err := parseZapLevel(level)
	if err != nil {
		return err
	}
	atomicLevel.SetLevel(zapLevel)
	return nil
}

// ResolveCompress applies the project default (true) when the config-level
// Compress is nil. When non-nil, the operator's choice is used as-is.
//
// The project default is compression on; operators can opt out by setting
// log.compress: false in service_conf.yaml. Because Go's bool zero value is
// false and would otherwise be indistinguishable from "not set", the YAML
// struct uses *bool and this helper resolves the defaulting at the cmd/
// boundary. The *bool does not live in this file because FileOutput itself
// takes a plain bool (the caller has already resolved the default by then).
func ResolveCompress(c *bool) bool {
	if c == nil {
		return true
	}
	return *c
}

// GinLogger returns a gin middleware that emits one log line per request
// through Logger. Level is chosen by status:
//
//	5xx → Error (with err from c.Errors, or sentinel if none)
//	4xx → Warn
//	else → Info
//
// c.Errors content is always included as a zap.String("error", ...) field
// when present, regardless of level. This is the project-standard HTTP
// access log; it replaces gin.Logger() so every request line lands in the
// same log file as the rest of the application.
//
// The raw query string is intentionally NOT logged — the path field
// carries only the URL path. Query parameters frequently carry secrets
// (OAuth codes, SAML responses, signed state, API keys in callback
// URLs, etc.) and there is no way to redact them generically. The
// presence and length of a query string are recorded instead so
// operators can still see that one was sent.
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery
		c.Next()
		latency := time.Since(start)
		status := c.Writer.Status()

		fields := []zap.Field{
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("size", c.Writer.Size()),
			zap.Bool("has_query", raw != ""),
			zap.Int("query_len", len(raw)),
		}

		var ginErr error
		if len(c.Errors) > 0 {
			last := c.Errors.Last()
			// Only emit the string error field for non-5xx paths. The 5xx
			// branch below routes ginErr through common.Error(), which
			// already adds a structured zap.Error field; logging both
			// creates two "error" fields in the same record and confuses
			// log aggregation. 4xx / 2xx-3xx paths use Warn/Info which do
			// not take an error arg, so the string form is their only
			// way to surface c.Errors content.
			if status < 500 {
				fields = append(fields, zap.String("error", last.Error()))
			}
			ginErr = last.Err
		}

		msg := "HTTP request"
		switch {
		case status >= 500:
			if ginErr == nil {
				// Likely a panic recovered by gin.Recovery() with no c.Error attached.
				// Use a sentinel so the err field is non-empty; operators can
				// grep for this string in logs.
				ginErr = errors.New("5xx response with no handler error attached")
			}
			Error(msg, ginErr, fields...)
		case status >= 400:
			Warn(msg, fields...)
		default:
			Info(msg, fields...)
		}
	}
}
