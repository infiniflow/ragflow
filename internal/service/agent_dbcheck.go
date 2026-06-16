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
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"

	"ragflow/internal/common"
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

// AssertHostIsSafe returns the first resolved public IP for host, or an
// error when the host resolves to any non-public address. The check
// mirrors the SSRF guard in the Python implementation so external
// service calls cannot pivot to internal network ranges.
func AssertHostIsSafe(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("Host must not be empty.")
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		zap.L().Warn("SSRF guard could not resolve host",
			zap.String("host", host),
			zap.Error(err),
		)
		return "", fmt.Errorf("Could not resolve host %q: %w", host, err)
	}
	if len(ips) == 0 {
		zap.L().Warn("SSRF guard blocked host: resolved to no addresses",
			zap.String("host", host),
		)
		return "", fmt.Errorf("Host %q resolved to no addresses.", host)
	}

	var resolvedIP string
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return "", fmt.Errorf("invalid resolved IP %q for host %q", ip.String(), host)
		}
		// Normalize IPv4-mapped IPv6, equivalent to Python _effective_ip().
		addr = addr.Unmap()

		if !isPublicAddr(addr) {
			zap.L().Warn("SSRF guard blocked host",
				zap.String("host", host),
				zap.String("resolved_ip", addr.String()),
			)
			return "", fmt.Errorf("Host resolves to a non-public address (%s), which is not allowed.", addr.String())
		}
		if resolvedIP == "" {
			resolvedIP = addr.String()
		}
	}
	if resolvedIP == "" {
		return "", fmt.Errorf("Host %q resolved to no addresses.", host)
	}
	return resolvedIP, nil
}

func isPublicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()

	if !addr.IsValid() {
		return false
	}
	if !addr.IsGlobalUnicast() {
		return false
	}
	if addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return false
	}
	return !isSpecialUseAddr(addr)
}

func isSpecialUseAddr(addr netip.Addr) bool {
	addr = addr.Unmap()

	specialCIDRs := []string{
		// IPv4 special-use / documentation / reserved ranges.
		"0.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",

		// IPv6 special-use / documentation / local ranges.
		"::/128",
		"::1/128",
		"64:ff9b:1::/48",
		"100::/64",
		"2001::/23",
		"2001:2::/48",
		"fc00::/7",
		"fe80::/10",
		"ff00::/8",
		"2001:db8::/32",
		"2002::/16",
	}
	for _, cidr := range specialCIDRs {
		prefix := netip.MustParsePrefix(cidr)
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
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
func (s *AgentService) TestDBConnection(userID string, req *TestDBConnectionRequest) (common.ErrorCode, error) {
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
		db, err := sql.Open("mysql", config.FormatDSN())
		if err != nil {
			return common.CodeExceptionError, err
		}
		defer db.Close()

		ctx, cancel := context.WithTimeout(context.Background(), dbProbeTimeout)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			return common.CodeExceptionError, err
		}
		if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
			return common.CodeExceptionError, err
		}
	default:
		return common.CodeExceptionError, errors.New("Unsupported database type.")
	}

	return common.CodeSuccess, nil
}
