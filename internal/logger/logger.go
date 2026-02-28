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

package logger

import (
	"fmt"
	"runtime"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Logger *zap.Logger
	Sugar  *zap.SugaredLogger
)

// Init initializes the global logger
// Note: This requires zap to be installed: go get go.uber.org/zap
func Init(level string) error {
	// Parse log level
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

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
		Level:            zap.NewAtomicLevelAt(zapLevel),
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
