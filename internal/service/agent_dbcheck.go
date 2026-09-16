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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/utility"
)

// TestDBConnectionRequest is the request body for AgentService.TestDBConnection.
type TestDBConnectionRequest struct {
	DBType   string      `json:"db_type"`
	Database string      `json:"database"`
	Username string      `json:"username"`
	Host     string      `json:"host"`
	Port     interface{} `json:"port"`
	Password string      `json:"password"`
}

// AllowAnyHostForTest mirrors utility.AllowAnyHostForTest: a test-only
// override that disables the SSRF guard in AssertHostIsSafe. Production
// code MUST leave this at false. Tests that need to talk to a local
// httptest server or stub-resolved DB host flip it on and reset it
// in t.Cleanup.
//
// The previous form was an env-var check (ALLOW_ANY_HOST=1) which was
// a live runtime toggle any operator could flip to disable the SSRF
// guard globally. PR review round 6, Major #3: this is a process-
// memory boolean only — no env var, no deployment flag, no path to
// bypass from outside the test binary.
var AllowAnyHostForTest = false

func allowAnyHost() bool {
	return AllowAnyHostForTest
}

// AssertHostIsSafe returns the first resolved public IP for host, or an error
// when the host resolves to any non-public address. It delegates to
// utility.AssertHostSafe (the shared host-type SSRF guard) so external DB
// probes cannot pivot to internal network ranges; the check mirrors the SSRF
// guard in the Python implementation.
func AssertHostIsSafe(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("host must not be empty")
	}
	if allowAnyHost() {
		zap.L().Warn("SSRF guard bypass enabled via AllowAnyHostForTest; allowing host without validation",
			zap.String("host", host),
		)
		return host, nil
	}

	resolvedIP, err := utility.AssertHostSafe(host)
	if err != nil {
		zap.L().Warn("SSRF guard blocked host",
			zap.String("host", host),
			zap.Error(err),
		)
		return "", err
	}
	return resolvedIP, nil
}

func missingDBConnectionFields(req *TestDBConnectionRequest) []string {
	missing := make([]string, 0, 6)
	if req == nil || strings.TrimSpace(req.DBType) == "" {
		missing = append(missing, "db_type")
	}
	if req == nil || strings.TrimSpace(req.Database) == "" {
		missing = append(missing, "database")
	}
	if req == nil || strings.TrimSpace(req.Username) == "" {
		missing = append(missing, "username")
	}
	if req == nil || strings.TrimSpace(req.Host) == "" {
		missing = append(missing, "host")
	}
	if req == nil || dbConnectionPort(req.Port) == "" {
		missing = append(missing, "port")
	}
	if req == nil || req.Password == "" {
		missing = append(missing, "password")
	}
	return missing
}

func dbConnectionPort(port interface{}) string {
	switch value := port.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case float64:
		return strconv.Itoa(int(value))
	case float32:
		return strconv.Itoa(int(value))
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case json.Number:
		return value.String()
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

// TestDBConnection validates input and performs a probe connect against
// the requested database. The probe enforces an SSRF allow-list and a
// short timeout to keep the API responsive when targets are unreachable.
// The "required argument are missing" message has a trailing semicolon
// and space to stay byte-identical with the Python implementation.
func (s *AgentService) TestDBConnection(ctx context.Context, userID string, req *TestDBConnectionRequest) (common.ErrorCode, error) {
	if missing := missingDBConnectionFields(req); len(missing) > 0 {
		return common.CodeArgumentError, fmt.Errorf("required argument are missing: %s; ", strings.Join(missing, ","))
	}

	safeHost, err := AssertHostIsSafe(req.Host)
	if err != nil {
		zap.L().Warn(
			"Rejected test_db_connection: unsafe host",
			zap.String("host", req.Host),
			zap.String("db_type", req.DBType),
			zap.String("user", userID),
			zap.Error(err),
		)
		return common.CodeDataError, err
	}

	switch req.DBType {
	case "mysql", "mariadb", "oceanbase":
		port := dbConnectionPort(req.Port)
		dbProbeTimeout := 5 * time.Second
		config := mysql.Config{
			User:                 req.Username,
			Passwd:               req.Password,
			Net:                  "tcp",
			Addr:                 net.JoinHostPort(safeHost, port),
			DBName:               req.Database,
			Timeout:              dbProbeTimeout,
			AllowNativePasswords: true,
		}
		var db *sql.DB
		db, err = sql.Open("mysql", config.FormatDSN())
		if err != nil {
			return common.CodeExceptionError, err
		}
		defer db.Close()

		newCtx, cancel := context.WithTimeout(ctx, dbProbeTimeout)
		defer cancel()

		if err = db.PingContext(newCtx); err != nil {
			return common.CodeExceptionError, err
		}
		if _, err = db.ExecContext(newCtx, "SELECT 1"); err != nil {
			return common.CodeExceptionError, err
		}
	default:
		return common.CodeExceptionError, errors.New("unsupported database type")
	}

	return common.CodeSuccess, nil
}
