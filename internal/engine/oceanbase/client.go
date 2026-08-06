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

// Package oceanbase implements the OceanBase and SeekDB document engines over
// their MySQL-compatible SQL protocol.
package oceanbase

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/server/config"

	mysql "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

const (
	connectionAttempts               = 2
	connectionRetryInterval          = 5 * time.Second
	defaultOperationTimeout          = 100 * time.Second
	minimumOceanBaseVersion          = "4.3.5.1"
	minimumHybridVersion             = "4.4.1.0"
	minimumSeekDBIndexRefreshVersion = "1.3.0.0"
)

var seekDBVersionPattern = regexp.MustCompile(`(?i)\bseekdb[-\s]v?(\d+\.\d+\.\d+(?:\.\d+)?)`)

type featureFlags struct {
	enableFullTextSearch         bool
	useFullTextHint              bool
	searchOriginalContent        bool
	enableHybridSearch           bool
	useFullTextFirstFusionSearch bool
}

// Engine is an OceanBase-family document engine backed by database/sql.
type Engine struct {
	db                  *sql.DB
	dbName              string
	engineType          string
	flags               featureFlags
	maxIdleConns        int
	hybridAvailable     atomic.Bool
	indexRefreshEnabled bool
}

// NewEngine creates an OceanBase or SeekDB document engine.
func NewEngine(engineType string, cfg config.OceanBaseConnectionConfig) (*Engine, error) {
	if engineType != "oceanbase" && engineType != "seekdb" {
		return nil, fmt.Errorf("invalid OceanBase-family engine type: %s", engineType)
	}
	if cfg.MaxConnections <= 0 {
		cfg.MaxConnections = 300
	}

	driverConfig := mysql.NewConfig()
	driverConfig.User = cfg.User
	driverConfig.Passwd = cfg.Password
	driverConfig.Net = "tcp"
	driverConfig.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	driverConfig.DBName = cfg.DBName
	driverConfig.ParseTime = true
	driverConfig.Collation = "utf8mb4_unicode_ci"
	driverConfig.Params = map[string]string{"charset": "utf8mb4"}
	driverConfig.Timeout = 30 * time.Second
	driverConfig.ReadTimeout = defaultOperationTimeout
	driverConfig.WriteTimeout = defaultOperationTimeout

	var db *sql.DB
	var lastErr error
	for attempt := 0; attempt < connectionAttempts; attempt++ {
		db, lastErr = sql.Open("mysql", driverConfig.FormatDSN())
		if lastErr == nil {
			maxOverflow := envInt("OB_MAX_OVERFLOW", max(cfg.MaxConnections/2, 10))
			db.SetMaxOpenConns(cfg.MaxConnections + maxOverflow)
			db.SetMaxIdleConns(cfg.MaxConnections)
			db.SetConnMaxLifetime(3600 * time.Second)
			pingCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			lastErr = db.PingContext(pingCtx)
			cancel()
			if lastErr == nil {
				break
			}
			_ = db.Close()
		}
		if attempt+1 < connectionAttempts {
			time.Sleep(connectionRetryInterval)
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("connect to %s %s:%d: %w", engineType, cfg.Host, cfg.Port, lastErr)
	}

	engine := newEngineWithDB(engineType, cfg.DBName, db)
	engine.maxIdleConns = cfg.MaxConnections
	if err := engine.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return engine, nil
}

func newEngineWithDB(engineType, dbName string, db *sql.DB) *Engine {
	flags := featureFlags{
		enableFullTextSearch:         envBool("ENABLE_FULLTEXT_SEARCH", true),
		useFullTextHint:              envBool("USE_FULLTEXT_HINT", true),
		searchOriginalContent:        envBool("SEARCH_ORIGINAL_CONTENT", true),
		enableHybridSearch:           envBool("ENABLE_HYBRID_SEARCH", false),
		useFullTextFirstFusionSearch: envBool("USE_FULLTEXT_FIRST_FUSION_SEARCH", true),
	}
	return &Engine{db: db, dbName: dbName, engineType: engineType, flags: flags}
}

func (e *Engine) initialize(ctx context.Context) error {
	var version string
	if err := e.db.QueryRowContext(ctx, "SELECT OB_VERSION()").Scan(&version); err != nil {
		return fmt.Errorf("get OceanBase version: %w", err)
	}
	if compareVersions(version, minimumOceanBaseVersion) < 0 {
		return fmt.Errorf("OceanBase version must be at least %s, current version is %s", minimumOceanBaseVersion, version)
	}
	if err := e.initializeIndexRefresh(ctx); err != nil {
		return err
	}

	e.ensureQueryTimeout(ctx)
	if e.flags.enableHybridSearch {
		available := e.engineType == "seekdb" || compareVersions(version, minimumHybridVersion) >= 0
		e.hybridAvailable.Store(available)
		if available {
			// The DBMS hybrid-search DSL uses the tokenized FTS fields. This
			// is the same switch made by the Python HybridSearch client path.
			e.flags.searchOriginalContent = false
		} else {
			common.Warn("OceanBase DBMS hybrid search is unavailable for this version",
				zap.String("version", version), zap.String("minimumVersion", minimumHybridVersion))
		}
	}
	return nil
}

func (e *Engine) initializeIndexRefresh(ctx context.Context) error {
	if e.engineType != "seekdb" {
		return nil
	}

	var serverVersion string
	if err := e.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&serverVersion); err != nil {
		return fmt.Errorf("get SeekDB version: %w", err)
	}
	seekDBVersion, ok := extractSeekDBVersion(serverVersion)
	if !ok {
		common.Warn("Could not parse SeekDB version; index refresh is disabled",
			zap.String("version", serverVersion))
		return nil
	}
	e.indexRefreshEnabled = compareVersions(seekDBVersion, minimumSeekDBIndexRefreshVersion) >= 0
	return nil
}

func extractSeekDBVersion(serverVersion string) (string, bool) {
	match := seekDBVersionPattern.FindStringSubmatch(serverVersion)
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

func (e *Engine) ensureQueryTimeout(ctx context.Context) {
	target := envInt64("OB_QUERY_TIMEOUT", 100_000_000)
	var name string
	var current int64
	if err := e.db.QueryRowContext(ctx, "SHOW VARIABLES LIKE 'ob_query_timeout'").Scan(&name, &current); err == nil && current >= target {
		return
	}
	if _, err := e.db.ExecContext(ctx, fmt.Sprintf("SET GLOBAL ob_query_timeout=%d", target)); err != nil {
		common.Warn("Failed to set OceanBase query timeout", zap.Error(err))
		return
	}
	// Existing sessions retain the old global value. Closing idle sessions
	// mirrors Python's engine.dispose() while keeping in-flight work alive.
	maxIdle := e.maxIdleConns
	if maxIdle <= 0 {
		maxIdle = e.db.Stats().MaxOpenConnections
	}
	e.db.SetMaxIdleConns(0)
	e.db.SetMaxIdleConns(maxIdle)
}

// Ping checks that the database connection is alive.
func (e *Engine) Ping(ctx context.Context) error {
	if e == nil || e.db == nil {
		return fmt.Errorf("OceanBase client is not initialized")
	}
	return e.db.PingContext(ctx)
}

// Close closes the SQL connection pool.
func (e *Engine) Close() error {
	if e == nil || e.db == nil {
		return nil
	}
	return e.db.Close()
}

// GetType returns oceanbase or seekdb, preserving the configured engine name.
func (e *Engine) GetType() string { return e.engineType }

// SupportsPageRank reports that OceanBase applies pagerank during ranking.
func (e *Engine) SupportsPageRank() bool { return true }

// AdjustChunkPagerank atomically updates the legacy pagerank_fea column.
func (e *Engine) AdjustChunkPagerank(ctx context.Context, tableName, chunkID, datasetID string, delta, minWeight, maxWeight float64) error {
	if err := validateIdentifier(tableName); err != nil {
		return err
	}
	_, err := e.db.ExecContext(ctx, "UPDATE "+quoteIdentifier(tableName)+
		" SET pagerank_fea = GREATEST(?, LEAST(?, COALESCE(pagerank_fea, 0) + ?)) WHERE id = ? AND kb_id = ?",
		minWeight, maxWeight, delta, chunkID, datasetID)
	return err
}

func envBool(name string, defaultValue bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if raw == "" {
		return defaultValue
	}
	return raw == "true" || raw == "1" || raw == "yes" || raw == "y"
}

func envInt64(name string, defaultValue int64) int64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}

func envInt(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return defaultValue
	}
	return value
}

func compareVersions(left, right string) int {
	parse := func(raw string) []int {
		parts := strings.FieldsFunc(raw, func(r rune) bool { return r < '0' || r > '9' })
		values := make([]int, 0, len(parts))
		for _, part := range parts {
			value, err := strconv.Atoi(part)
			if err == nil {
				values = append(values, value)
			}
		}
		return values
	}
	a, b := parse(left), parse(right)
	for i := 0; i < max(len(a), len(b)); i++ {
		var av, bv int
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}
