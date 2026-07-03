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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/utility"

	"go.uber.org/zap"
)

const (
	psqlTimeout     = 10 * time.Second
	defaultPsqlPath = "/usr/bin/psql"
	defaultPsqlHost = "infinity"
	defaultPsqlPort = "5432"
)

var whitespaceRe = regexp.MustCompile("[ `]+")

var rowCountFooterRe = regexp.MustCompile(`^\(\d+ rows?`)

// fieldMappingEntry is one entry in infinity_mapping.json.
type fieldMappingEntry struct {
	Type    string `json:"type"`
	Comment string `json:"comment"`
}

// loadFieldMapping reads infinity_mapping.json and returns alias→actual
// and actual→firstAlias maps. Silently returns empty maps on missing file.
func loadFieldMapping(mappingFileName string) (aliasToActual map[string]string, actualToFirstAlias map[string]string, err error) {
	if mappingFileName == "" {
		mappingFileName = "infinity_mapping.json"
	}

	filePath, err := utility.FindConfFileInProject(mappingFileName)
	if err != nil {
		return nil, nil, err
	}

	data, err := os.ReadFile(*filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, map[string]string{}, nil
		}
		return nil, nil, fmt.Errorf("load field mapping %q: %w", *filePath, err)
	}

	fields := map[string]fieldMappingEntry{}
	if err = json.Unmarshal(data, &fields); err != nil {
		return nil, nil, fmt.Errorf("parse field mapping %q: %w", *filePath, err)
	}

	aliasToActual = make(map[string]string, len(fields)*2)
	actualToFirstAlias = make(map[string]string, len(fields))
	for actual, info := range fields {
		if info.Comment == "" {
			continue
		}
		var firstAlias string
		for _, raw := range strings.Split(info.Comment, ",") {
			alias := strings.TrimSpace(raw)
			if alias == "" {
				continue
			}
			aliasToActual[alias] = actual
			if firstAlias == "" {
				firstAlias = alias
			}
		}
		if firstAlias != "" {
			actualToFirstAlias[actual] = firstAlias
		}
	}
	return aliasToActual, actualToFirstAlias, nil
}

// preprocessSQL collapses spaces/backticks and strips '%'.
func preprocessSQL(sql string) string {
	sql = whitespaceRe.ReplaceAllString(sql, " ")
	sql = strings.ReplaceAll(sql, "%", "")
	return sql
}

// rewriteFieldAliases rewrites alias field names to actual stored names
// in SELECT, WHERE, ORDER BY, GROUP BY, and HAVING clauses.
func rewriteFieldAliases(sql string, aliasToActual map[string]string) string {
	if len(aliasToActual) == 0 {
		return sql
	}
	selectRe := regexp.MustCompile(`(?si)(select\s+)(.+?)(\s+from\b)`)
	sql = selectRe.ReplaceAllStringFunc(sql, func(m string) string {
		parts := selectRe.FindStringSubmatch(m)
		prefix, cols, suffix := parts[1], parts[2], parts[3]
		for alias, actual := range aliasToActual {
			pat := regexp.MustCompile(`(^|[,\s])` + regexp.QuoteMeta(alias) + `($|[,\s])`)
			cols = pat.ReplaceAllString(cols, "${1}"+actual+"${2}")
		}
		return prefix + cols + suffix
	})

	clauseAliases := func(sql, keyword string) string {
		return rewriteFirstAliasAfterKeyword(sql, keyword, aliasToActual)
	}
	sql = clauseAliases(sql, "where")
	sql = clauseAliases(sql, "order by")
	sql = clauseAliases(sql, "group by")
	sql = clauseAliases(sql, "having")
	return sql
}

func rewriteFirstAliasAfterKeyword(sql, keyword string, aliasToActual map[string]string) string {
	for alias, actual := range aliasToActual {
		aliasPat := regexp.MustCompile(`\b` + regexp.QuoteMeta(alias) + `\b`)
		kwIdx := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(keyword) + `\b`).FindStringIndex(sql)
		if kwIdx == nil {
			continue
		}
		tail := sql[kwIdx[1]:]
		aliasIdx := aliasPat.FindStringIndex(tail)
		if aliasIdx == nil {
			continue
		}
		absStart := kwIdx[1] + aliasIdx[0]
		absEnd := kwIdx[1] + aliasIdx[1]
		sql = sql[:absStart] + actual + sql[absEnd:]
	}
	return sql
}

// psqlResult is the structured parse of a psql table-format output.
type psqlResult struct {
	Columns []string
	Rows    [][]string
}

// runPsql shells out to psql and parses the table-format output.
func runPsql(ctx context.Context, host, port, sql string) (*psqlResult, error) {
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

	for _, raw := range strings.Split(lines[0], "|") {
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

// RunSQL implements the SQL retrieval path: preprocess, rewrite aliases,
// run psql subprocess, parse output.
func (e *infinityEngine) RunSQL(ctx context.Context, tableName string, sqlText string, kbIDs []string, _ string) ([]map[string]interface{}, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("infinity RunSQL: client not initialized")
	}
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return nil, fmt.Errorf("infinity RunSQL: empty SQL")
	}

	common.Debug("InfinityConnection.sql get sql", zap.String("sql", sqlText))

	sqlText = preprocessSQL(sqlText)

	aliasMap, _, err := loadFieldMapping(e.client.mappingFileName)
	if err != nil {
		return nil, fmt.Errorf("infinity RunSQL: %w", err)
	}
	sqlText = rewriteFieldAliases(sqlText, aliasMap)

	common.Debug("InfinityConnection.sql to execute", zap.String("sql", sqlText))

	host, port := resolvePsqlHostPort(e.client.hostURI, e.client.postgresPort)
	res, err := runPsql(ctx, host, port, sqlText)
	if err != nil {
		return nil, err
	}

	return toRowMaps(res), nil
}
