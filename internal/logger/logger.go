package logger

import (
	"log"

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
	// Check if zap is available, if not, use standard logger
	defer func() {
		if Logger == nil {
			log.Printf("Logger initialized (standard library)")
		}
	}()

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

	// Configure zap
	config := zap.Config{
		Level:            zap.NewAtomicLevelAt(zapLevel),
		Development:      false,
		Encoding:         "json",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
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

// Fatal logs a fatal message using zap or standard logger
func Fatal(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Fatal(msg, fields...)
	} else {
		log.Fatalf(msg)
	}
}

// Info logs an info message using zap or standard logger
func Info(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Info(msg, fields...)
	} else {
		log.Println(msg)
	}
}

// Error logs an error message using zap or standard logger
func Error(msg string, err error) {
	if Logger != nil {
		Logger.Error(msg, zap.Error(err))
	} else {
		log.Printf("%s: %v", msg, err)
	}
}

// Debug logs a debug message using zap or standard logger
func Debug(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Debug(msg, fields...)
	} else {
		log.Printf("[DEBUG] %s", msg)
	}
}

// Warn logs a warning message using zap or standard logger
func Warn(msg string, fields ...zap.Field) {
	if Logger != nil {
		Logger.Warn(msg, fields...)
	} else {
		log.Printf("[WARN] %s", msg)
	}
}
