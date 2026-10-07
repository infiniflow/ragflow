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

// Package vastbase implements the Vastbase G100 document engine over its
// PostgreSQL-compatible wire protocol (database/sql + lib/pq). Chunks live in
// shared per-baseName tables with kb_id/memory_id as row-level discriminators;
// vector columns use the floatvector type with graph_index (HNSW) indexes.
package vastbase

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/server/config"

	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

const (
	connectionAttempts      = 24
	connectionRetryInterval = 5 * time.Second
	defaultPoolSize         = 50
	defaultMaxOverflow      = 100
	connMaxLifetime         = 30 * time.Minute
)

// Compatibility modes accepted for VastbaseConfig.DBCompatibility.
const (
	compatPG = "PG"
	compatB  = "B"
)

var (
	// lib/pq's parseOpts tolerates whitespace around "=" and treats a
	// backslash-escaped space as part of an unquoted value, so the value
	// alternation must cover both forms or the redaction leaks.
	dsnKeywordPwRe = regexp.MustCompile(`(password\s*=\s*)('(?:[^'\\]|\\.)*'|(?:\\.|[^\s\\])+)`)
	dsnURLPwRe     = regexp.MustCompile(`(://[^:/@\s]+:)[^@\s]+(@)`)
)

// redactDSN hides the password in both the lib/pq keyword form (password=...)
// and the URL form (scheme://user:password@host) used by VASTBASE_DSN.
func redactDSN(dsn string) string {
	dsn = dsnKeywordPwRe.ReplaceAllString(dsn, "${1}***")
	dsn = dsnURLPwRe.ReplaceAllString(dsn, "${1}***${2}")
	return dsn
}

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

// Engine is the Vastbase G100 document engine backed by database/sql.
type Engine struct {
	db              *sql.DB
	dbName          string
	engineType      string
	dbCompatibility string
	dsnSafe         string
}

// NewEngine constructs the engine from the Vastbase config. It retries the
// initial connection for up to two minutes (the Python connector contract for
// waiting on a freshly started container).
func NewEngine(cfg config.VastbaseConfig) (*Engine, error) {
	dsn := buildDSN(cfg)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("vastbase: open: %w", err)
	}
	poolSize := envInt("VB_POOL_SIZE", defaultPoolSize)
	maxOverflow := envInt("VB_MAX_OVERFLOW", defaultMaxOverflow)
	db.SetMaxOpenConns(poolSize + maxOverflow)
	db.SetMaxIdleConns(poolSize)
	db.SetConnMaxLifetime(connMaxLifetime)

	e := &Engine{
		db:              db,
		dbName:          cfg.DBName,
		engineType:      "vastbase",
		dbCompatibility: cfg.DBCompatibility,
		dsnSafe:         redactDSN(dsn),
	}

	var lastErr error
	for attempt := 0; attempt < connectionAttempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		lastErr = db.PingContext(pingCtx)
		cancel()
		if lastErr == nil {
			break
		}
		common.Warn("Vastbase is not healthy yet, retrying",
			zap.Int("attempt", attempt+1),
			zap.Int("max_attempts", connectionAttempts),
			zap.String("dsn", e.dsnSafe),
			zap.Error(lastErr))
		if attempt+1 < connectionAttempts {
			time.Sleep(connectionRetryInterval)
		}
	}
	if lastErr != nil {
		_ = db.Close()
		return nil, fmt.Errorf("vastbase: connect %s: %w", e.dsnSafe, lastErr)
	}

	if err := e.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	common.Info("Vastbase engine initialized",
		zap.String("dsn", e.dsnSafe),
		zap.String("dbcompatibility", e.dbCompatibility))
	return e, nil
}

func newEngineWithDB(cfg config.VastbaseConfig, db *sql.DB) *Engine {
	return &Engine{
		db:              db,
		dbName:          cfg.DBName,
		engineType:      "vastbase",
		dbCompatibility: cfg.DBCompatibility,
		dsnSafe:         "redacted",
	}
}

// buildDSN assembles a lib/pq keyword DSN. VASTBASE_DSN overrides everything;
// otherwise config values fill in. An unset ssl_mode follows the transport
// policy: plaintext (disable) only for loopback or unix-socket hosts, and
// certificate-verified TLS (verify-full) for every other TCP host — remote
// deployments must not silently fall back to cleartext. Unrecognized keywords
// such as statement_timeout are forwarded to the server as startup runtime
// parameters.
func buildDSN(cfg config.VastbaseConfig) string {
	if env := os.Getenv("VASTBASE_DSN"); env != "" {
		return env
	}
	host, port, user, password, dbName := "vastbase", 5432, "ragflow", "", "ragflow"
	sslMode := ""
	if cfg.Host != "" {
		host = cfg.Host
	}
	if cfg.Port != 0 {
		port = cfg.Port
	}
	if cfg.User != "" {
		user = cfg.User
	}
	password = cfg.Password
	if cfg.DBName != "" {
		dbName = cfg.DBName
	}
	if cfg.SSLMode != "" {
		sslMode = cfg.SSLMode
	}
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
	if password != "" {
		parts = append(parts, fmt.Sprintf("password=%s", quoteDSNValue(password)))
	}
	// No keepalive keywords here: lib/pq forwards keywords it does not
	// recognize to the server as startup parameters, and Vastbase (like
	// vanilla PostgreSQL) rejects `keepalives*` as unknown GUCs. TCP keepalive
	// stays on regardless — lib/pq dials through a zero-value net.Dialer,
	// which enables keepalives at the OS default interval.
	parts = append(parts,
		"connect_timeout=30",
		"statement_timeout=300000",
	)
	return strings.Join(parts, " ")
}

// initialize verifies the server answers and warns when the configured
// compatibility mode does not match the instance's datcompatibility. A B-mode
// distance operator issued against a PG instance fails at query time, so the
// early warning saves a debugging round trip.
func (e *Engine) initialize(ctx context.Context) error {
	var version string
	if err := e.db.QueryRowContext(ctx, "SELECT vb_version()").Scan(&version); err != nil {
		return fmt.Errorf("vastbase: get version: %w", err)
	}
	common.Info("Vastbase server version", zap.String("version", version))

	var datCompat string
	if err := e.db.QueryRowContext(ctx,
		"SELECT datcompatibility FROM pg_database WHERE datname = current_database()").Scan(&datCompat); err == nil {
		expected := "PG"
		if e.dbCompatibility == compatB {
			expected = "B"
		}
		if !strings.EqualFold(strings.TrimSpace(datCompat), expected) {
			common.Warn("Vastbase dbcompatibility mismatch: instance reports " +
				datCompat + ", engine configured for " + e.dbCompatibility +
				"; distance operators and full-text indexes may fail")
		}
	}
	return nil
}

// distanceOp returns the cosine-distance operator for the compatibility mode.
func (e *Engine) distanceOp() string {
	if e.dbCompatibility == compatB {
		return "<+>"
	}
	return "<=>"
}

// GetType returns the engine type string.
func (e *Engine) GetType() string {
	return e.engineType
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
	if err := validateIdentifier(indexName); err != nil {
		return err
	}
	_, err := e.db.ExecContext(ctx, fmt.Sprintf(
		"UPDATE %s SET pagerank_fea = GREATEST($1, LEAST($2, COALESCE(pagerank_fea, 0) + $3)) WHERE id = $4 AND kb_id = $5",
		quoteIdent(indexName)), minWeight, maxWeight, delta, chunkID, kbID)
	if err != nil {
		return fmt.Errorf("vastbase: adjust pagerank: %w", err)
	}
	return nil
}

func envInt(name string, fallback int) int {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}
