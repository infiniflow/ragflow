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

package service

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"time"

	"ragflow/internal/server"

	"github.com/go-sql-driver/mysql"
)

// oceanbaseConn holds the OceanBase connection params. OceanBase (and its
// lite variant SeekDB) speak the MySQL wire protocol, so we reach it with the
// standard database/sql MySQL driver — no pyobvector equivalent needed.
type oceanbaseConn struct {
	host, user, password, dbName string
	port                         int
}

// oceanbaseConfigFromViper reads the `oceanbase` section of service_conf.yaml.
// Returns ok=false when OceanBase isn't configured, which the caller maps to
// the Python-parity "OceanBase is not in use." path.
func oceanbaseConfigFromViper() (*oceanbaseConn, bool) {
	v := server.GetGlobalViperConfig()
	if v == nil || !v.IsSet("oceanbase.config") {
		return nil, false
	}
	c := &oceanbaseConn{
		host:     v.GetString("oceanbase.config.host"),
		user:     v.GetString("oceanbase.config.user"),
		password: v.GetString("oceanbase.config.password"),
		dbName:   v.GetString("oceanbase.config.db_name"),
		port:     v.GetInt("oceanbase.config.port"),
	}
	if c.host == "" || c.port == 0 {
		return nil, false
	}
	return c, true
}

func (c *oceanbaseConn) uri() string { return net.JoinHostPort(c.host, strconv.Itoa(c.port)) }

func (c *oceanbaseConn) open() (*sql.DB, error) {
	cfg := mysql.Config{
		User:   c.user,
		Passwd: c.password,
		Net:    "tcp",
		Addr:   c.uri(),
		// Connect to the server (not a specific schema) so the probe works
		// even before the doc DB is created; the storage query names the
		// schema explicitly.
		Timeout:              5 * time.Second,
		ReadTimeout:          5 * time.Second,
		AllowNativePasswords: true,
	}
	return sql.Open("mysql", cfg.FormatDSN())
}

// probeOceanbase gathers health + performance metrics over plain MySQL-wire
// SQL, mirroring Python OBConnection.health() + get_performance_metrics().
// Each metric is isolated so one missing OB-specific table/variable doesn't
// fail the whole probe (Python wraps each in try/except with fallbacks).
func probeOceanbase(c *oceanbaseConn) (health, performance ComponentStatus, err error) {
	db, err := c.open()
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	// latency_ms: time a trivial round-trip (also the connectivity check).
	start := time.Now()
	if err := db.PingContext(ctx); err != nil {
		return nil, nil, err
	}
	var one int
	_ = db.QueryRowContext(ctx, "SELECT 1").Scan(&one)
	latencyMs := float64(time.Since(start).Microseconds()) / 1000.0

	getVar := func(name string) string {
		var k, val string
		// SHOW VARIABLES doesn't accept a bound parameter for the LIKE clause
		// on MySQL/OceanBase, so interpolate the name directly. `name` is only
		// ever a fixed internal constant below, so this is injection-safe.
		q := fmt.Sprintf("SHOW VARIABLES LIKE '%s'", name)
		if e := db.QueryRowContext(ctx, q).Scan(&k, &val); e != nil {
			return "unknown"
		}
		return val
	}

	versionComment := getVar("version_comment")
	maxConns := getVar("max_connections")

	activeConns := 0
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.processlist").Scan(&activeConns)

	slowQueries := 0
	_ = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.processlist WHERE time > 1 AND command != 'Sleep'").Scan(&slowQueries)

	storageUsed := "N/A"
	var sizeMB sql.NullFloat64
	if e := db.QueryRowContext(ctx,
		"SELECT ROUND(SUM(data_length+index_length)/1024/1024,2) FROM information_schema.tables WHERE table_schema = ?",
		c.dbName).Scan(&sizeMB); e == nil && sizeMB.Valid {
		storageUsed = fmt.Sprintf("%.2fMB", sizeMB.Float64)
	}

	health = ComponentStatus{
		"uri":             c.uri(),
		"version_comment": versionComment,
	}
	performance = ComponentStatus{
		"connection":         "connected",
		"latency_ms":         latencyMs,
		"version":            versionComment,
		"active_connections": activeConns,
		"max_connections":    maxConns,
		"slow_queries":       slowQueries,
		"storage_used":       storageUsed,
		"storage_total":      "N/A",
	}
	return health, performance, nil
}
