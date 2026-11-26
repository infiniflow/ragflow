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

// Package opengauss implements the RAGFlow document engine for openGauss DataVec.
package opengauss

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/server/config"

	pq "gitcode.com/opengauss/openGauss-connector-go-pq"
	"go.uber.org/zap"
)

const (
	connectionAttempts      = 24
	connectionRetryInterval = 5 * time.Second
	defaultPoolSize         = 50
	defaultMaxOverflow      = 100
	connMaxLifetime         = 30 * time.Minute
)

const connectionTimeout = 2 * time.Minute
const initializeTimeout = 30 * time.Second

// quoteDSNValue wraps a lib/pq keyword-DSN value in single quotes, escaping
// backslashes and quotes, so hosts/users/passwords with spaces or special
// characters do not break the DSN.
func quoteDSNValue(v string) string {
	r := strings.ReplaceAll(v, `\`, `\\`)
	r = strings.ReplaceAll(r, `'`, `\'`)
	return "'" + r + "'"
}

// isLocalDBHost reports whether the host is loopback or a unix-socket path —
// the only addresses allowed to default to plaintext transport.
func isLocalDBHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	return strings.HasPrefix(host, "/")
}

// Engine stores documents and their vector and BM25 indexes in openGauss.
type Engine struct {
	db *sql.DB
}

// NewEngine opens the pool and checks the schema and required DataVec capabilities.
func NewEngine(cfg config.OpenGaussConfig) (*Engine, error) {
	if err := validateIdentifier(cfg.Schema); err != nil {
		return nil, fmt.Errorf("opengauss schema: %w", err)
	}
	db, err := openPool(cfg)
	if err != nil {
		return nil, err
	}
	poolSize := envInt("OPENGAUSS_POOL_SIZE", defaultPoolSize)
	maxOverflow := envInt("OPENGAUSS_MAX_OVERFLOW", defaultMaxOverflow)
	db.SetMaxOpenConns(poolSize + maxOverflow)
	db.SetMaxIdleConns(poolSize)
	db.SetConnMaxLifetime(connMaxLifetime)

	e := &Engine{db: db}

	var lastErr error
	connectCtx, cancelConnect := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancelConnect()
	for attempt := 0; attempt < connectionAttempts; attempt++ {
		if connectCtx.Err() != nil {
			break
		}
		pingCtx, cancel := context.WithTimeout(connectCtx, 10*time.Second)
		lastErr = db.PingContext(pingCtx)
		cancel()
		if lastErr == nil {
			break
		}
		common.Warn("openGauss is not healthy yet, retrying",
			zap.Int("attempt", attempt+1),
			zap.Int("max_attempts", connectionAttempts),
			zap.String("database", cfg.Database),
			zap.Error(lastErr))
		if attempt+1 < connectionAttempts {
			select {
			case <-time.After(connectionRetryInterval):
			case <-connectCtx.Done():
			}
		}
	}
	if lastErr != nil {
		_ = db.Close()
		return nil, fmt.Errorf("opengauss: connect to %s: %w", cfg.Database, lastErr)
	}

	initCtx, cancelInit := context.WithTimeout(context.Background(), initializeTimeout)
	defer cancelInit()
	if err := e.checkDataVec(initCtx); err != nil {
		_ = db.Close()
		return nil, err
	}
	common.Info("openGauss engine initialized",
		zap.String("database", cfg.Database),
		zap.String("schema", cfg.Schema))
	return e, nil
}

// The connector clears the socket deadline after authentication when connect_timeout is set.
func openPool(cfg config.OpenGaussConfig) (*sql.DB, error) {
	dsn := connectionString(cfg.Host, cfg.Port, cfg.User, cfg.Database, cfg.SSLMode)
	options, err := pq.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("opengauss connection configuration: %w", err)
	}
	options.Password = cfg.Password
	options.RuntimeParams["search_path"] = quoteIdent(cfg.Schema)
	dial := options.DialFunc
	options.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		deadline := time.Now().Add(options.ConnectTimeout)
		if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
			deadline = end
		}
		conn, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		if err := conn.SetDeadline(deadline); err != nil {
			conn.Close()
			return nil, err
		}
		return conn, nil
	}
	connector, err := pq.NewConnectorConfig(options)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(connector), nil
}

func connectionString(host string, port int, user, dbName, sslMode string) string {
	if sslMode == "" {
		if isLocalDBHost(host) {
			sslMode = "disable"
		} else {
			sslMode = "verify-full"
		}
	}
	parts := []string{
		fmt.Sprintf("host=%s", quoteDSNValue(host)),
		fmt.Sprintf("port=%d", port),
		fmt.Sprintf("user=%s", quoteDSNValue(user)),
		fmt.Sprintf("dbname=%s", quoteDSNValue(dbName)),
		fmt.Sprintf("sslmode=%s", quoteDSNValue(sslMode)),
	}
	parts = append(parts,
		"connect_timeout=30",
		"statement_timeout=300000",
	)
	return strings.Join(parts, " ")
}

// GetType returns the engine type string.
func (e *Engine) GetType() string {
	return "opengauss"
}

// SupportsPageRank reports dataset-level pagerank support: pagerank_fea is a
// real column folded into every scored query and adjusted atomically by
// AdjustChunkPagerank.
func (e *Engine) SupportsPageRank() bool {
	return true
}

// Ping verifies connectivity.
func (e *Engine) Ping(ctx context.Context) error {
	return e.db.PingContext(ctx)
}

// Close releases the connection pool.
func (e *Engine) Close() error {
	if e.db == nil {
		return nil
	}
	return e.db.Close()
}

// AdjustChunkPagerank atomically clamps pagerank_fea into [minWeight,
// maxWeight] while applying delta, scoped to the dataset's rows in the shared
// chunk table.
func (e *Engine) AdjustChunkPagerank(ctx context.Context, indexName, chunkID, kbID string, delta, minWeight, maxWeight float64) error {
	indexName = physicalTableName(indexName)
	if err := validateIdentifier(indexName); err != nil {
		return err
	}
	_, err := e.db.ExecContext(ctx, fmt.Sprintf(
		"UPDATE %s SET pagerank_fea = GREATEST($1, LEAST($2, COALESCE(pagerank_fea, 0) + $3)) WHERE id = $4 AND kb_id = $5",
		quoteIdent(indexName)), minWeight, maxWeight, delta, chunkID, kbID)
	if err != nil {
		return fmt.Errorf("opengauss: adjust pagerank: %w", err)
	}
	return nil
}

// envInt reads a positive integer environment variable, falling back to the
// default when unset, empty, or invalid.
func envInt(name string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func (e *Engine) checkDataVec(ctx context.Context) error {
	var schema sql.NullString
	var mode, encoding string
	var vector, bm25, hnsw bool
	err := e.db.QueryRowContext(ctx, `SELECT current_schema(), datcompatibility, getdatabaseencoding(),
 EXISTS (SELECT 1 FROM pg_type WHERE typname='vector'),
 EXISTS (SELECT 1 FROM pg_am WHERE amname='bm25'),
 EXISTS (SELECT 1 FROM pg_am WHERE amname='hnsw')
 FROM pg_database WHERE datname=current_database()`).Scan(&schema, &mode, &encoding, &vector, &bm25, &hnsw)
	if err != nil {
		return fmt.Errorf("opengauss: checking DataVec: %w", err)
	}
	if !schema.Valid || schema.String == "" {
		return fmt.Errorf("opengauss: configured schema is missing or inaccessible")
	}
	if encoding != "UTF8" {
		return fmt.Errorf("opengauss requires a UTF8 database (encoding=%s)", encoding)
	}
	if mode != "A" || !vector || !bm25 || !hnsw {
		return fmt.Errorf("opengauss requires an A-mode DataVec database (mode=%s vector=%t bm25=%t hnsw=%t)", mode, vector, bm25, hnsw)
	}
	return nil
}
