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

package infinity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"ragflow/internal/common"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/apache/thrift/lib/go/thrift"

	"ragflow/internal/server"

	infinity "github.com/infiniflow/infinity-go-sdk"
	"go.uber.org/zap"
)

type infinityClient struct {
	pool        *infinity.ConnectionPool
	poolMaxOpen int
	dbName      string

	getDatabaseSeq      atomic.Uint64
	getDatabaseInflight atomic.Int64

	// Original URI from config, used by RunSQL to extract the host.
	hostURI string

	// Port for psql wire-protocol listener (default 5432).
	postgresPort int

	// JSON file (under conf/) with the field-name alias map.
	mappingFileName string
}

// defaultPoolMaxSize is the hard upper bound for the Go Infinity connection
// pool. Unlike the Python pool (where INFINITY_POOL_MAX_SIZE is a non-binding
// pre-warm value), this is a true cap enforced by the SDK's GetContext.
const defaultPoolMaxSize = 32
const defaultWarmConnections = 4
const defaultOperationTimeout = 30 * time.Second
const defaultSocketTimeout = 30 * time.Second

// resolvePoolMaxSize reads INFINITY_POOL_MAX_SIZE_GO (a Go-only knob, separate
// from the Python env var) and falls back to defaultPoolMaxSize. A value < 1 is
// rejected so the pool never becomes unbounded.
func resolvePoolMaxSize() int {
	raw := os.Getenv("INFINITY_POOL_MAX_SIZE_GO")
	if raw == "" {
		return defaultPoolMaxSize
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		common.Warn("INFINITY_POOL_MAX_SIZE_GO must be a positive integer; using default",
			zap.String("value", raw), zap.Int("default", defaultPoolMaxSize))
		return defaultPoolMaxSize
	}
	return v
}

// NewInfinityClient creates a new Infinity client using the SDK
type poolExhaustedError struct {
	caller string
}

func (e *poolExhaustedError) Error() string {
	return fmt.Sprintf("Infinity connection pool exhausted while %s", e.caller)
}

func (e *poolExhaustedError) Code() common.ErrorCode {
	return common.CodeConnectionPoolExhausted
}

func (e *poolExhaustedError) Message() string {
	return e.Error()
}

func ensureDeadline(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func NewInfinityClient(cfg *server.InfinityConfig) (*infinityClient, error) {
	// Parse URI like "localhost:23817" to get IP and port
	host := "127.0.0.1"
	port := 23817

	if cfg.URI != "" {
		parts := strings.Split(cfg.URI, ":")
		if len(parts) == 2 {
			host = parts[0]
			if p, err := strconv.Atoi(parts[1]); err == nil {
				port = p
			}
		}
	}

	// Retry connecting for up to 120 seconds (24 attempts * 5 seconds)
	common.Info("Connecting to Infinity")
	uri := infinity.NetworkAddress{IP: host, Port: port}
	maxOpen := resolvePoolMaxSize()
	var pool *infinity.ConnectionPool
	var err error
	for i := 0; i < 24; i++ {
		warmSize := maxOpen
		if warmSize > defaultWarmConnections {
			warmSize = defaultWarmConnections
		}
		poolCfg := infinity.DefaultConnectionPoolConfig(uri)
		poolCfg.InitialSize = warmSize
		poolCfg.MaxOpen = maxOpen
		poolCfg.MaxIdle = warmSize
		pool, err = infinity.NewConnectionPool(poolCfg, func(u infinity.URI) (*infinity.InfinityConnection, error) {
			networkAddress, ok := u.(infinity.NetworkAddress)
			if !ok {
				return nil, fmt.Errorf("unexpected URI type: %T", u)
			}
			return infinity.NewInfinityConnectionWithConfig(networkAddress, &thrift.TConfiguration{
				ConnectTimeout: server.DefaultConnectTimeout,
				SocketTimeout:  defaultSocketTimeout,
			})
		})
		if err == nil {
			break
		}
		if i < 23 {
			time.Sleep(5 * time.Second)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("Failed to connect to Infinity after 120s: %w", err)
	}

	client := &infinityClient{
		pool:            pool,
		poolMaxOpen:     maxOpen,
		dbName:          cfg.DBName,
		hostURI:         cfg.URI,
		postgresPort:    cfg.PostgresPort,
		mappingFileName: cfg.MappingFileName,
	}

	return client, nil
}

// WaitForHealthy blocks until Infinity is healthy or timeout
func (c *infinityClient) WaitForHealthy(ctx context.Context, timeout time.Duration) error {
	common.Info("Waiting for Infinity to be healthy")
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, release, err := c.checkoutConn(ctx, "WaitForHealthy")
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		res, err := conn.ShowCurrentNode()
		release()
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		// Use reflection to access ErrorCode and ServerStatus fields
		// since ShowCurrentNodeResponse is in an internal package
		v := reflect.ValueOf(res)
		if v.Kind() != reflect.Ptr {
			common.Warn("Infinity health check returned unexpected response kind",
				zap.String("type", fmt.Sprintf("%T", res)),
				zap.String("kind", v.Kind().String()),
			)
			time.Sleep(5 * time.Second)
			continue
		}
		v = v.Elem()
		errorCode := v.FieldByName("ErrorCode")
		serverStatus := v.FieldByName("ServerStatus")
		if !errorCode.IsValid() || !serverStatus.IsValid() {
			common.Warn("Infinity health check response missing expected fields",
				zap.String("type", fmt.Sprintf("%T", res)),
				zap.Bool("has_error_code", errorCode.IsValid()),
				zap.Bool("has_server_status", serverStatus.IsValid()),
			)
			time.Sleep(5 * time.Second)
			continue
		}
		// ErrorCode 0 means OK, ServerStatus "started" or "alive" means healthy
		if errorCode.Int() == 0 {
			status := serverStatus.String()
			if status == "started" || status == "alive" {
				common.Info("Infinity is healthy")
				return nil
			}
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("Infinity not healthy after %v", timeout)
}

func (c *infinityClient) checkoutConn(ctx context.Context, caller string) (*infinity.InfinityConnection, func(), error) {
	if c == nil || c.pool == nil {
		return nil, nil, fmt.Errorf("Infinity client not initialized")
	}
	ctx, cancel := ensureDeadline(ctx, defaultOperationTimeout)
	conn, err := c.pool.GetContext(ctx)
	if err != nil {
		cancel()
		if (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) && c.poolMaxOpen > 0 {
			stats := c.pool.Stats()
			if stats.TotalConnections >= c.poolMaxOpen && stats.AvailableConnections == 0 {
				return nil, nil, &poolExhaustedError{caller: caller}
			}
		}
		return nil, nil, err
	}
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		defer cancel()
		if err := c.pool.Put(conn); err != nil {
			common.Warn("Infinity connection release failed",
				zap.String("caller", caller),
				zap.String("conn_ptr", fmt.Sprintf("%p", conn)),
				zap.Error(err),
			)
		}
	}
	return conn, release, nil
}

func (c *infinityClient) checkoutDatabase(ctx context.Context, caller string) (*infinity.Database, func(), error) {
	conn, release, err := c.checkoutConn(ctx, caller)
	if err != nil {
		return nil, nil, err
	}
	reqID := c.getDatabaseSeq.Add(1)
	inflight := c.getDatabaseInflight.Add(1)
	start := time.Now()
	stats := c.pool.Stats()
	common.Info("Infinity GetDatabase begin",
		zap.Uint64("req_id", reqID),
		zap.String("caller", caller),
		zap.String("db_name", c.dbName),
		zap.Bool("is_connected", conn.IsConnected()),
		zap.Int64("inflight", inflight),
		zap.String("conn_ptr", fmt.Sprintf("%p", conn)),
		zap.Int("pool_in_use", stats.InUseConnections),
		zap.Int("pool_available", stats.AvailableConnections),
	)
	db, err := conn.GetDatabase(c.dbName)
	if err != nil {
		inflight = c.getDatabaseInflight.Add(-1)
		elapsed := time.Since(start)
		common.Warn("Infinity GetDatabase failed",
			zap.Uint64("req_id", reqID),
			zap.String("caller", caller),
			zap.String("db_name", c.dbName),
			zap.Bool("is_connected", conn.IsConnected()),
			zap.Int64("inflight", inflight),
			zap.Duration("elapsed", elapsed),
			zap.Error(err),
		)
		release()
		return nil, nil, err
	}
	wrappedRelease := func() {
		inflight := c.getDatabaseInflight.Add(-1)
		elapsed := time.Since(start)
		common.Info("Infinity GetDatabase done",
			zap.Uint64("req_id", reqID),
			zap.String("caller", caller),
			zap.String("db_name", c.dbName),
			zap.Bool("is_connected", conn.IsConnected()),
			zap.Int64("inflight", inflight),
			zap.Duration("elapsed", elapsed),
		)
		release()
	}
	return db, wrappedRelease, nil
}

// Engine Infinity engine implementation using Go SDK
type infinityEngine struct {
	config                 *server.InfinityConfig
	client                 *infinityClient
	mappingFileName        string
	docMetaMappingFileName string
}

// NewEngine creates an Infinity engine
func NewEngine(cfg interface{}) (*infinityEngine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("infinity config is nil, please check your configuration file for 'doc_engine.infinity' settings")
	}
	infConfig, ok := cfg.(*server.InfinityConfig)
	if !ok {
		return nil, fmt.Errorf("invalid infinity config type, expected *config.InfinityConfig")
	}
	if infConfig == nil {
		return nil, fmt.Errorf("infinity config is nil, please check your configuration file for 'doc_engine.infinity' settings")
	}

	client, err := NewInfinityClient(infConfig)
	if err != nil {
		return nil, err
	}

	mappingFileName := infConfig.MappingFileName
	if mappingFileName == "" {
		mappingFileName = "infinity_mapping.json"
	}
	docMetaMappingFileName := infConfig.DocMetaMappingFileName
	if docMetaMappingFileName == "" {
		docMetaMappingFileName = "doc_meta_infinity_mapping.json"
	}

	engine := &infinityEngine{
		config:                 infConfig,
		client:                 client,
		mappingFileName:        mappingFileName,
		docMetaMappingFileName: docMetaMappingFileName,
	}

	// Wait for Infinity to be healthy
	if err := client.WaitForHealthy(context.Background(), 120*time.Second); err != nil {
		return nil, fmt.Errorf("Infinity not healthy: %w", err)
	}

	// MigrateDB creates the database if it doesn't exist
	if err := engine.MigrateDB(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return engine, nil
}

// GetType returns the engine type
func (e *infinityEngine) GetType() string {
	return "infinity"
}

// SupportsPageRank returns false because Infinity does not support pagerank.
func (e *infinityEngine) SupportsPageRank() bool {
	return false
}

// Ping checks if Infinity is accessible
func (e *infinityEngine) Ping(ctx context.Context) error {
	if e.client == nil || e.client.pool == nil {
		return fmt.Errorf("Infinity client not initialized")
	}
	conn, release, err := e.client.checkoutConn(ctx, "Ping")
	if err != nil {
		return err
	}
	defer release()
	if !conn.IsConnected() {
		return fmt.Errorf("Infinity not connected")
	}
	return nil
}

// Close closes the Infinity connection
func (e *infinityEngine) Close() error {
	if e.client != nil && e.client.pool != nil {
		return e.client.pool.Close()
	}
	return nil
}

// MigrateDB creates the database if it doesn't exist
func (e *infinityEngine) MigrateDB(ctx context.Context) error {
	conn, release, err := e.client.checkoutConn(ctx, "MigrateDB")
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}
	defer release()
	_, err = conn.CreateDatabase(e.client.dbName, infinity.ConflictTypeIgnore, "")
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	return nil
}
