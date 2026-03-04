# Logger Package

This package provides structured logging using Uber's Zap library.

## Installation

Install zap dependency:

```bash
go get go.uber.org/zap
```

## Usage

The logger is initialized in `cmd/server_main.go` and is available throughout the application.

### Basic Usage

```go
import (
    "ragflow/internal/logger"
    "go.uber.org/zap"
)

// Log with structured fields
logger.Info("User login", zap.String("user_id", userID), zap.String("ip", clientIP))

// Log error
logger.Error("Failed to connect database", err)

// Log fatal (exits application)
logger.Fatal("Failed to start server", err)

// Debug level
logger.Debug("Processing request", zap.String("request_id", reqID))

// Warning level
logger.Warn("Slow query", zap.Duration("duration", duration))
```

### Access Logger Directly

If you need the underlying Zap logger:

```go
logger.Logger.Info("Message", zap.String("key", "value"))
```

Or use the SugaredLogger for more flexible API:

```go
logger.Sugar.Infow("Message", "key", "value")
```

## Fallback to Standard Logger

If zap is not installed or fails to initialize, the logger will fallback to the standard library `log` package, ensuring the application continues to work.

## Log Levels

The logger supports the following levels:
- `debug` - Detailed information for debugging
- `info` - General informational messages
- `warn` - Warning messages
- `error` - Error messages
- `fatal` - Fatal errors that stop the application

The log level is configured via the server mode in the configuration:
- `debug` mode uses `debug` level
- `release` mode uses `info` level
