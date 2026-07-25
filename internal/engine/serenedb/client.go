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

package serenedb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/server/config"

	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

const (
	defaultHost     = "serenedb"
	defaultPort     = 7890
	defaultUser     = "postgres"
	defaultDBName   = "postgres"
	poolMaxOpen     = 8
	poolMaxIdle     = 2
	connMaxLifetime = 30 * time.Minute
)

var dsnPasswordRe = regexp.MustCompile(`(password=)[^ ]+`)

// serenedbEngine implements engine.DocEngine backed by SereneDB.
type serenedbEngine struct {
	db      *sql.DB
	dsnSafe string
}

// NewEngine constructs the engine from the SereneDB config, mirroring the
// elasticsearch/infinity factories in engine/global.go.
func NewEngine(cfg config.SereneDBConfig) (*serenedbEngine, error) {
	dsn := buildDSN(cfg)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("serenedb: open: %w", err)
	}
	db.SetMaxOpenConns(poolMaxOpen)
	db.SetMaxIdleConns(poolMaxIdle)
	db.SetConnMaxLifetime(connMaxLifetime)

	e := &serenedbEngine{db: db, dsnSafe: dsnPasswordRe.ReplaceAllString(dsn, "${1}***")}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("serenedb: ping %s: %w", e.dsnSafe, err)
	}
	common.Info("SereneDB engine initialized", zap.String("dsn", e.dsnSafe))
	return e, nil
}

// buildDSN assembles a lib/pq keyword DSN. SERENEDB_DSN overrides everything
// (matching the Python connector); otherwise config values fill in, then the
// documented defaults.
func buildDSN(cfg config.SereneDBConfig) string {
	if env := os.Getenv("SERENEDB_DSN"); env != "" {
		return env
	}
	host, port, user, password, dbName := defaultHost, defaultPort, defaultUser, "", defaultDBName
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
	parts := []string{
		fmt.Sprintf("host=%s", host),
		fmt.Sprintf("port=%d", port),
		fmt.Sprintf("user=%s", user),
		fmt.Sprintf("dbname=%s", dbName),
		"sslmode=disable",
	}
	if password != "" {
		parts = append(parts, fmt.Sprintf("password=%s", password))
	}
	return strings.Join(parts, " ")
}

// GetType returns the engine type string.
func (e *serenedbEngine) GetType() string {
	return "serenedb"
}

// SupportsPageRank reports dataset-level pagerank support. Like Elasticsearch,
// SereneDB folds pagerank_fea into every scored query and stores it in a real
// column that UpdateChunks can set, so the dataset-level toggle is supported.
func (e *serenedbEngine) SupportsPageRank() bool {
	return true
}

// Ping verifies connectivity.
func (e *serenedbEngine) Ping(ctx context.Context) error {
	return e.db.PingContext(ctx)
}

// Close releases the connection pool.
func (e *serenedbEngine) Close() error {
	if e.db == nil {
		return nil
	}
	return e.db.Close()
}

// exec runs a statement that returns no rows.
func (e *serenedbEngine) exec(ctx context.Context, query string, args ...interface{}) error {
	_, err := e.db.ExecContext(ctx, query, args...)
	return err
}

// queryMaps runs a query and shapes each row into a map keyed by column name,
// decoding JSON and array columns to structured values.
func (e *serenedbEngine) queryMaps(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		entity := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			v := decodeValue(col, vals[i])
			if v == nil {
				continue
			}
			entity[col] = v
		}
		out = append(out, entity)
	}
	return out, rows.Err()
}

// tableExists reports whether a relation exists by probing it.
func (e *serenedbEngine) tableExists(ctx context.Context, tableName string) bool {
	if err := e.exec(ctx, fmt.Sprintf("SELECT 1 FROM %s LIMIT 0", tableName)); err != nil {
		return false
	}
	return true
}
