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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/tablequery"

	"go.uber.org/zap"
)

const (
	psqlTimeout     = 10 * time.Second
	defaultPsqlPath = "/usr/bin/psql"
	defaultPsqlHost = "infinity"
	defaultPsqlPort = "5432"
)

var rowCountFooterRe = regexp.MustCompile(`^\(\d+ rows?`)

// psqlResult is the structured parse of a psql table-format output.
type psqlResult struct {
	Columns []string
	Rows    [][]string
}

// assertSafePsqlSelect enforces defense-in-depth before invoking psql: only a
// single-line SELECT statement without semicolons or psql meta-commands may be
// executed.
func assertSafePsqlSelect(sqlText string) error {
	trimmed := strings.TrimSpace(sqlText)
	if trimmed == "" {
		return fmt.Errorf("infinity ExecuteTableQuery: empty SQL")
	}
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT ") {
		return fmt.Errorf("infinity ExecuteTableQuery: only SELECT statements are allowed")
	}
	if strings.ContainsAny(trimmed, ";\r\n") {
		return fmt.Errorf("infinity ExecuteTableQuery: multi-statement or multiline SQL is not allowed")
	}
	if strings.HasPrefix(trimmed, `\`) {
		return fmt.Errorf("infinity ExecuteTableQuery: psql meta-commands are not allowed")
	}
	return nil
}

// runPsql shells out to psql and parses the table-format output.
func runPsql(ctx context.Context, host, port, sql string) (*psqlResult, error) {
	if err := assertSafePsqlSelect(sql); err != nil {
		return nil, err
	}
	psqlPath, err := findPsqlBinary()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, psqlTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, psqlPath, "-h", host, "-p", port, "-c", sql)
	common.Debug("executing psql",
		zap.String("path", psqlPath),
		zap.String("host", host),
		zap.String("port", port),
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("SQL timeout\n\nSQL: %s", sql)
		}
		return nil, fmt.Errorf("psql command failed: %s\nSQL: %s", strings.TrimSpace(stderr.String()), sql)
	}
	return parsePsqlTable(stdout.String()), nil
}

// findPsqlBinary checks PATH first, then falls back to defaultPsqlPath.
func findPsqlBinary() (string, error) {
	if path, err := exec.LookPath("psql"); err == nil {
		return path, nil
	}
	if _, err := os.Stat(defaultPsqlPath); err == nil {
		return defaultPsqlPath, nil
	}
	return "", fmt.Errorf("psql not found on PATH and not at %q", defaultPsqlPath)
}

// parsePsqlTable parses psql's pipe-delimited output:
//
//	col1 | col2
//	-----+-----
//	val1 | val2
func parsePsqlTable(output string) *psqlResult {
	res := &psqlResult{}
	out := strings.TrimSpace(output)
	if out == "" {
		return res
	}
	lines := strings.Split(out, "\n")
	if len(lines) == 0 {
		return res
	}

	for raw := range strings.SplitSeq(lines[0], "|") {
		if col := strings.TrimSpace(raw); col != "" {
			res.Columns = append(res.Columns, col)
		}
	}

	dataStart := 1
	if len(lines) >= 2 && strings.Contains(lines[1], "-") {
		dataStart = 2
	}
	for i := dataStart; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || rowCountFooterRe.MatchString(line) {
			continue
		}
		cells := strings.Split(line, "|")
		for j := range cells {
			cells[j] = strings.TrimSpace(cells[j])
		}
		switch {
		case len(cells) == len(res.Columns):
			res.Rows = append(res.Rows, cells)
		case len(cells) > len(res.Columns):
			res.Rows = append(res.Rows, cells[:len(res.Columns)])
		default:
			padded := make([]string, len(res.Columns))
			copy(padded, cells)
			for k := len(cells); k < len(res.Columns); k++ {
				padded[k] = ""
			}
			res.Rows = append(res.Rows, padded)
		}
	}
	return res
}

// toRowMaps converts psqlResult to a slice of column-keyed maps.
func toRowMaps(res *psqlResult) []map[string]interface{} {
	if res == nil || len(res.Rows) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(res.Rows))
	for _, row := range res.Rows {
		m := make(map[string]interface{}, len(res.Columns))
		for j, col := range res.Columns {
			if j < len(row) {
				m[col] = row[j]
			}
		}
		out = append(out, m)
	}
	return out
}

func resolvePsqlHostPort(hostURI string, postgresPort int) (host, port string) {
	host = defaultPsqlHost
	port = defaultPsqlPort
	if postgresPort > 0 {
		port = strconv.Itoa(postgresPort)
	}
	if hostURI != "" {
		if h, _, ok := strings.Cut(hostURI, ":"); ok && h != "" {
			host = h
		}
	}
	return host, port
}

// ExecuteTableQuery compiles a validated single-dataset TableQuery into
// Infinity SQL, verifies the single-SELECT psql guard, runs psql, and maps
// rows.
func (e *Engine) ExecuteTableQuery(ctx context.Context, query *tablequery.ValidatedQuery) ([]map[string]interface{}, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("infinity ExecuteTableQuery: client not initialized")
	}
	compiled, err := tablequery.Compile(query, tablequery.DialectInfinity)
	if err != nil {
		return nil, fmt.Errorf("infinity ExecuteTableQuery: %w", err)
	}
	if err := assertSafePsqlSelect(compiled.SQL); err != nil {
		return nil, err
	}

	common.Debug("InfinityConnection.sql to execute", zap.String("sql", compiled.SQL))

	host, port := resolvePsqlHostPort(e.client.hostURI, e.client.postgresPort)
	res, err := runPsql(ctx, host, port, compiled.SQL)
	if err != nil {
		return nil, err
	}

	return toRowMaps(res), nil
}
