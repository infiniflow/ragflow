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
	"os"
	"runtime"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger      *zap.Logger
	Sugar       *zap.SugaredLogger
	levelMu     sync.RWMutex
	atomicLevel zap.AtomicLevel
	pkgLevels   map[string]string
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

func initPackageLogLevels(rootLevel zapcore.Level) {
	levels := make(map[string]string)
	for _, item := range strings.Split(os.Getenv("LOG_LEVELS"), ",") {
		terms := strings.SplitN(item, "=", 2)
		if len(terms) != 2 {
			continue
		}
		pkgName := strings.TrimSpace(terms[0])
		if pkgName == "" {
			continue
		}
		level, err := parseZapLevel(terms[1])
		if err != nil {
			level = zapcore.InfoLevel
		}
		levels[pkgName] = logLevelName(level)
	}
	// I set it to align with python for now, we shall change it later before ragflow 1.0
	if _, ok := levels["peewee"]; !ok {
		levels["peewee"] = logLevelName(zapcore.WarnLevel)
	}
	if _, ok := levels["pdfminer"]; !ok {
		levels["pdfminer"] = logLevelName(zapcore.WarnLevel)
	}
	if _, ok := levels["root"]; !ok {
		levels["root"] = logLevelName(rootLevel)
	}
	pkgLevels = levels
}

// Init initializes the global logger
// Note: This requires zap to be installed: go get go.uber.org/zap
func Init(level string) error {
	zapLevel, err := parseZapLevel(level)
	if err != nil {
		zapLevel = zapcore.InfoLevel
	}

	// Create atomic level for dynamic updates
	atomicLevel = zap.NewAtomicLevelAt(zapLevel)

	// Custom encoder config to control output format
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "", // Disable caller/line number
		FunctionKey:    "",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"), // Human-readable time format
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder, // Not used since CallerKey is empty
	}

	// Configure zap
	config := zap.Config{
		Level:            atomicLevel,
		Development:      false,
		Encoding:         "console",
		EncoderConfig:    encoderConfig,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	// Build logger
	logger, err := config.Build(zap.AddCallerSkip(1))
	if err != nil {
		return err
	}

	Logger = logger
	Sugar = logger.Sugar()

	levelMu.Lock()
	initPackageLogLevels(zapLevel)
	levelMu.Unlock()

	return nil
}

// Sync flushes any buffered log entries
func Sync() {
	if Logger != nil {
		_ = Logger.Sync()
	}
}

// Fatal logs a fatal message using zap with caller info
func Fatal(msg string, fields ...zap.Field) {
	if Logger == nil {
		panic("logger not initialized")
	}
	// Get caller info (skip this function to get the actual caller)
	_, file, line, ok := runtime.Caller(1)
	if ok {
		fields = append(fields, zap.String("caller", fmt.Sprintf("%s:%d", file, line)))
	}
	Logger.Fatal(msg, fields...)
}

// Info logs an info message using zap or standard logger
func Info(msg string, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	Logger.Info(msg, fields...)
}

// Error logs an error message using zap or standard logger
func Error(msg string, err error) {
	if Logger == nil {
		return
	}
	Logger.Error(msg, zap.Error(err))
}

// Debug logs a debug message using zap or standard logger
func Debug(msg string, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	Logger.Debug(msg, fields...)
}

// Warn logs a warning message using zap or standard logger
func Warn(msg string, fields ...zap.Field) {
	if Logger == nil {
		return
	}
	Logger.Warn(msg, fields...)
}

// IsDebugEnabled returns true if debug logging is enabled
func IsDebugEnabled() bool {
	return atomicLevel.Enabled(zapcore.DebugLevel)
}

// GetLevel returns the current log level
func GetLevel() string {
	levelMu.RLock()
	defer levelMu.RUnlock()
	return atomicLevel.String()
}

// GetLogLevels returns Python-compatible package log levels.
func GetLogLevels() map[string]string {
	levelMu.RLock()
	defer levelMu.RUnlock()

	levels := make(map[string]string, len(pkgLevels))
	for pkgName, level := range pkgLevels {
		levels[pkgName] = level
	}
	return levels
}

// SetLevel sets the log level at runtime
func SetLevel(level string) error {
	levelMu.Lock()
	defer levelMu.Unlock()

	zapLevel, err := parseZapLevel(level)
	if err != nil {
		return err
	}
	atomicLevel.SetLevel(zapLevel)
	if pkgLevels == nil {
		pkgLevels = make(map[string]string)
	}
	pkgLevels["root"] = logLevelName(zapLevel)
	return nil
}

// SetPackageLogLevel sets a Python-compatible package log level at runtime.
func SetPackageLogLevel(pkgName, level string) error {
	zapLevel, err := parseZapLevel(level)
	if err != nil {
		return err
	}

	levelMu.Lock()
	defer levelMu.Unlock()

	if pkgLevels == nil {
		pkgLevels = make(map[string]string)
	}
	pkgLevels[pkgName] = logLevelName(zapLevel)
	if pkgName == "root" {
		atomicLevel.SetLevel(zapLevel)
	}
	return nil
}
