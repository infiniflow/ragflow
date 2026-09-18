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
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	Logger      *zap.Logger
	Sugar       *zap.SugaredLogger
	atomicLevel zap.AtomicLevel

	stdLogOnce sync.Once
	stdLogger  *log.Logger
)

// FileOutput describes the rotated log file destination.
//
// Path is required to enable file output; empty disables the file destination
// (stdout only). When Path is set, the file is written under ./logs/<Path>
// and rotated by lumberjack according to MaxSize / MaxBackups / MaxAge / Compress.
//
// Numeric zero values (MaxSize/MaxBackups/MaxAge) are replaced with defaults
// (100 MB / 10 / 30 days) inside Init. Compress is a *bool so that "not set"
// (nil) can be distinguished from "explicitly false"; when nil it defaults to
// DefaultLogCompress (true).
type FileOutput struct {
	Filename   string
	Path       string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   *bool
}

const (
	cyanLogMarker  = "[[RAGFLOW_CYAN_LOG]]"
	greenLogMarker = "[[RAGFLOW_GREEN_LOG]]"
	redLogMarker   = "[[RAGFLOW_RED_LOG]]"
	resetLogMarker = "[[RAGFLOW_RESET_LOG]]"
	ansiBrightCyan = "\x1b[96m"
	ansiGreen      = "\x1b[32m"
	ansiRed        = "\x1b[31m"
	ansiReset      = "\x1b[0m"
	// DefaultLogMaxSizeMB is the default rotation threshold (lumberjack
	// MaxSize is in MB, not bytes).
	DefaultLogMaxSizeMB = 100
	// DefaultLogMaxBackups is the default number of rotated files retained.
	DefaultLogMaxBackups = 10
	// DefaultLogMaxAgeDays is the default retention window for rotated files.
	DefaultLogMaxAgeDays = 30
	// DefaultLogCompress is the project default for gzipping rotated files.
	DefaultLogCompress = true
)

type coloredLineWriteSyncer struct {
	zapcore.WriteSyncer
	color bool
}

func (s coloredLineWriteSyncer) Write(p []byte) (int, error) {
	if !bytes.Contains(p, []byte(cyanLogMarker)) && !bytes.Contains(p, []byte(greenLogMarker)) && !bytes.Contains(p, []byte(redLogMarker)) {
		return s.WriteSyncer.Write(p)
	}

	line := bytes.Clone(p)
	if s.color {
		line = bytes.ReplaceAll(line, []byte(cyanLogMarker), []byte(ansiBrightCyan))
		line = bytes.ReplaceAll(line, []byte(greenLogMarker), []byte(ansiGreen))
		line = bytes.ReplaceAll(line, []byte(redLogMarker), []byte(ansiRed))
		line = bytes.ReplaceAll(line, []byte(resetLogMarker), []byte(ansiReset))
	} else {
		line = bytes.ReplaceAll(line, []byte(cyanLogMarker), nil)
		line = bytes.ReplaceAll(line, []byte(greenLogMarker), nil)
		line = bytes.ReplaceAll(line, []byte(redLogMarker), nil)
		line = bytes.ReplaceAll(line, []byte(resetLogMarker), nil)
	}

	n, err := s.WriteSyncer.Write(line)
	if err != nil {
		return 0, err
	}
	if n != len(line) {
		return 0, io.ErrShortWrite
	}
	return len(p), nil
}

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

// InitLogger initializes the global logger. stdout is always written. If file.Path
// is non-empty, a rotated file is also written via lumberjack.
//
// Callers should pass a non-empty Path so that file logging is preserved
// (each binary's hardcoded default goes through this parameter). The empty
// path case is reserved for CLI mode where stdout is the only output.
//
// Numeric fields (MaxSize, MaxBackups, MaxAge) are defaulted to 100/10/30
// when zero. Compress is taken as supplied.
func InitLogger(level string, file FileOutput, serviceName string) error {
	zapLevel, err := parseZapLevel(level)
	if err != nil {
		zapLevel = zapcore.InfoLevel
	}

	atomicLevel = zap.NewAtomicLevelAt(zapLevel)

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:       "timestamp",
		LevelKey:      "level",
		NameKey:       "service",
		CallerKey:     "caller",
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
	// File sink only when a destination is actually configured. The cmd/*
	// entry points init the logger TWICE: a pre-config temporary logger
	// (before the port is known, e.g. "api_server" → logs/api_server.log)
	// and the real one after server.Init renames it (e.g.
	// "api_server_9384" → logs/api_server_9384.log). The temporary pass now
	// passes an empty FileOutput and stays stdout-only, so the pre-config
	// startup window no longer litters a second, orphaned log file next to
	// the real one (and ragflow-cli no longer drops a lumberjack file into
	// os.TempDir()).
	if file.Path != "" && file.Filename != "" {
		ljLogger := &lumberjack.Logger{
			Filename:   filepath.Join(file.Path, file.Filename),
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			MaxAge:     maxAge,
			Compress:   compress,
			LocalTime:  true,
		}
		syncers = append(syncers, zapcore.AddSync(ljLogger))
	}

	var core zapcore.Core
	if IsLLMDebugEnabled() {
		cores := []zapcore.Core{
			zapcore.NewCore(
				zapcore.NewConsoleEncoder(encoderConfig),
				coloredLineWriteSyncer{WriteSyncer: stdoutSyncer, color: true},
				atomicLevel,
			),
		}
		if len(syncers) > 1 {
			cores = append(cores, zapcore.NewCore(
				zapcore.NewConsoleEncoder(encoderConfig),
				coloredLineWriteSyncer{WriteSyncer: syncers[1]},
				atomicLevel,
			))
		}
		core = zapcore.NewTee(cores...)
	} else {
		core = zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderConfig),
			zap.CombineWriteSyncers(syncers...),
			atomicLevel,
		)
	}

	if serviceName != "" {
		Logger = zap.New(core,
			zap.Fields(zap.Int("pid", os.Getpid())),
			zap.AddCallerSkip(1),
		).Named(serviceName)
	} else {
		Logger = zap.New(core,
			zap.Fields(zap.Int("pid", os.Getpid())),
			zap.AddCallerSkip(1),
		)
	}
	Sugar = Logger.Sugar()

	return nil
}

// SyncLog flushes any buffered log entries.
func SyncLog() {
	if Logger != nil {
		_ = Logger.Sync()
	}
}

func Fatal(msg string, fields ...zap.Field) {
	if Logger == nil {
		panic("logger not initialized")
	}
	Logger.Fatal(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	Logger.Info(msg, fields...)
}

// LogRequestResponseInfo writes the request portion in bright cyan. The
// response portion is green for success and red for failure on stdout. File
// output remains uncolored.
func LogRequestResponseInfo(request, response string, responseSucceeded bool) {
	if Logger == nil {
		return
	}
	responseMarker := redLogMarker
	if responseSucceeded {
		responseMarker = greenLogMarker
	}
	Logger.Info(cyanLogMarker + request + responseMarker + " " + response + resetLogMarker)
}

func Error(msg string, err error, fields ...zap.Field) {
	if Logger == nil {
		return
	}

	if IsDebugEnabled() {
		Logger.Error(fmt.Sprintf("%s, %+v", msg, err), fields...)
	} else {
		Logger.Error(fmt.Sprintf("%s, %v", msg, err), fields...)
	}
}

func Debug(msg string, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	Logger.Debug(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	Logger.Warn(msg, fields...)
}

// StdLogger returns a *log.Logger that routes writes through the global zap
// logger, so call sites that keep a *log.Logger facade still land in the
// project's structured logs. When the project logger has not been initialized
// yet (e.g. before InitLogger runs or in standalone tests) it falls back to the
// standard-library default. The returned logger writes at Info level.
//
// The returned *log.Logger resolves the write target LAZILY on every write.
// Package-level variables like `var _LOG = common.StdLogger()` are evaluated
// during package init, long before InitLogger runs; a logger captured eagerly
// at that moment would be log.Default() forever and its output would vanish
// into stderr, never reaching the structured log files.
func StdLogger() *log.Logger {
	stdLogOnce.Do(func() {
		stdLogger = log.New(stdLogRouter{}, "", 0)
	})
	return stdLogger
}

// stdLogRouter dispatches *log.Logger writes to the current global logger on
// every write (see StdLogger).
type stdLogRouter struct{}

func (stdLogRouter) Write(p []byte) (int, error) {
	if Logger == nil {
		return os.Stderr.Write(p)
	}
	Logger.Info(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

// --- per-request log correlation -------------------------------------------
//
// One conversation turn (a chat completion request) fans out into dozens of
// log lines across packages — agent, delivery gate, auditor, tools, retrieval,
// token usage — and concurrent benchmark questions interleave them. Attaching
// the session id to the request context and reading it back in the Ctx-variant
// log helpers below lets a single `grep session_id=<id>` reconstruct one
// turn's full trail (the q71 postmortem had to reconstruct it from timestamps).

type ctxKey int

const sessionIDCtxKey ctxKey = iota

// WithSessionID returns a context that tags every Ctx-variant log call with
// the conversation turn's session id. Empty ids are a no-op so callers do not
// need to guard.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	if strings.TrimSpace(sessionID) == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionIDCtxKey, strings.TrimSpace(sessionID))
}

// SessionIDFromContext extracts the correlation id ("" when absent).
func SessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(sessionIDCtxKey).(string)
	return id
}

func sessionFields(ctx context.Context) []zap.Field {
	if id := SessionIDFromContext(ctx); id != "" {
		return []zap.Field{zap.String("session_id", id)}
	}
	return nil
}

// InfoCtx is Info plus the session_id correlation field when ctx carries one.
func InfoCtx(ctx context.Context, msg string, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	Logger.Info(msg, append(sessionFields(ctx), fields...)...)
}

// WarnCtx is Warn plus the session_id correlation field when ctx carries one.
func WarnCtx(ctx context.Context, msg string, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	Logger.Warn(msg, append(sessionFields(ctx), fields...)...)
}

// DebugCtx is Debug plus the session_id correlation field when ctx carries one.
func DebugCtx(ctx context.Context, msg string, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	Logger.Debug(msg, append(sessionFields(ctx), fields...)...)
}

// ErrorCtx is Error plus the session_id correlation field when ctx carries one.
func ErrorCtx(ctx context.Context, msg string, err error, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	detail := fmt.Sprintf("%s, %v", msg, err)
	if IsDebugEnabled() {
		detail = fmt.Sprintf("%s, %+v", msg, err)
	}
	Logger.Error(detail, append(sessionFields(ctx), fields...)...)
}

// IsDebugEnabled returns true if debug logging is enabled.
func IsDebugEnabled() bool {
	return atomicLevel.Enabled(zapcore.DebugLevel)
}

// GetLogLevel returns the current log level.
func GetLogLevel() string {
	return atomicLevel.String()
}

// SetLogLevel sets the log level at runtime.
func SetLogLevel(level string) error {
	zapLevel, err := parseZapLevel(level)
	if err != nil {
		return err
	}
	atomicLevel.SetLevel(zapLevel)
	return nil
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
				ginErr = err5xxNoError
			}
			Error(msg, ginErr, fields...)
		case status >= 400:
			Warn(msg, fields...)
		default:
			Info(msg, fields...)
		}
	}
}

var err5xxNoError = errors.New("5xx response with no handler error attached")
