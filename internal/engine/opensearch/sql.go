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

package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"

	"go.uber.org/zap"
)

const (
	osSQLRequestTimeout = 2 * time.Second
	osSQLFetchSize      = 128
	// osSQLMaxPages bounds cursor pagination so a misbehaving cluster cannot
	// keep the caller in an endless fetch loop.
	osSQLMaxPages = 1000
)

const osSQLRetryAttempts = 2
const osSQLRetryDelay = 3 * time.Second

var whitespaceRe = regexp.MustCompile("[ `]+")
var lktksMatchRe = regexp.MustCompile(` ([a-z_]+_l?tks)( like | ?= ?)'([^']+)'`)

// Preprocess normalizes SQL for OpenSearch: collapses whitespace/backticks,
// strips '%', and rewrites `<field>_l?tks like/= 'value'` into a tokenized
// MATCH() call. MATCH(field, tokens, 'operator=OR;minimum_should_match=30%')
// is the OpenSearch SQL dialect (ES SQL has no such three-argument form),
// mirroring Python's opensearch_conn.py sql().
func Preprocess(sql string) string {
	sql = whitespaceRe.ReplaceAllString(sql, " ")
	sql = strings.ReplaceAll(sql, "%", "")

	// Collect replacements so we don't re-scan tokens we've already rewritten
	type replacement struct {
		old, new string
	}
	var replaces []replacement
	for _, m := range lktksMatchRe.FindAllStringSubmatchIndex(sql, -1) {
		match := sql[m[0]:m[1]]
		fld := sql[m[2]:m[3]]
		val := sql[m[6]:m[7]]
		tokenized, err := tokenizer.Tokenize(val)
		if err != nil {
			continue
		}
		fine, err := tokenizer.FineGrainedTokenize(tokenized)
		if err != nil {
			continue
		}
		replaces = append(replaces, replacement{
			old: match,
			// fine comes from tokenizer.FineGrainedTokenize, which strips
			// non-alphanumerics; defense-in-depth escape just in case a
			// future tokenizer change reintroduces a quote.
			new: fmt.Sprintf(" MATCH(%s, '%s', 'operator=OR;minimum_should_match=30%%') ", fld, strings.ReplaceAll(fine, "'", "''")),
		})
	}
	for _, r := range replaces {
		sql = strings.Replace(sql, r.old, r.new, 1)
	}
	return sql
}

// RunSQL posts SQL to `/_plugins/_sql`, translates the response into
// chunk-shaped maps. Returns (nil, nil) on empty rows; (nil, error) when
// retries exhausted.
func (e *Engine) RunSQL(ctx context.Context, tableName string, sqlText string, kbIDs []string, format string) ([]map[string]interface{}, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("OpenSearch RunSQL: client not initialized")
	}
	if sqlText == "" {
		return nil, fmt.Errorf("OpenSearch RunSQL: empty SQL")
	}

	common.Debug("OSConnection.sql get sql", zap.String("sql", sqlText))
	sqlText = Preprocess(sqlText)
	common.Debug("OSConnection.sql to os", zap.String("sql", sqlText))

	var lastErr error
	for attempt := 0; attempt < osSQLRetryAttempts; attempt++ {
		rows, err := e.runSQLOnce(ctx, sqlText, format)
		if err == nil {
			return rows, nil
		}
		lastErr = err
		if !isTimeoutError(err) {
			common.Warn("OSConnection.sql got exception",
				zap.String("sql", sqlText),
				zap.Error(err))
			return nil, fmt.Errorf("SQL error: %w\n\nSQL: %s", err, sqlText)
		}
		common.Warn("OS request timeout",
			zap.String("sql", sqlText),
			zap.Int("attempt", attempt+1),
			zap.Error(err))
		if attempt < osSQLRetryAttempts-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(osSQLRetryDelay):
			}
		}
	}
	common.Error(fmt.Sprintf("OSConnection.sql timeout after %d attempts. SQL: %s", osSQLRetryAttempts, sqlText), lastErr)
	return nil, fmt.Errorf("OpenSearch RunSQL: timeout after %d attempts: %w", osSQLRetryAttempts, lastErr)
}

func (e *Engine) runSQLOnce(ctx context.Context, sqlText string, format string) ([]map[string]interface{}, error) {
	var (
		out    []map[string]interface{}
		cursor string
	)
	for page := 0; page < osSQLMaxPages; page++ {
		rows, next, err := e.runSQLPage(ctx, sqlText, cursor, format)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
		if next == "" {
			return out, nil
		}
		cursor = next
	}
	return out, nil
}

// runSQLPage executes one `/_plugins/_sql` round trip. The first page sends the
// query with fetch_size; later pages send the returned cursor. Each page gets
// its own timeout so a slow page does not consume the whole query budget.
func (e *Engine) runSQLPage(ctx context.Context, sqlText string, cursor string, format string) ([]map[string]interface{}, string, error) {
	pageCtx, cancel := context.WithTimeout(ctx, osSQLRequestTimeout)
	defer cancel()

	body := map[string]interface{}{}
	if cursor == "" {
		body["query"] = sqlText
		body["fetch_size"] = osSQLFetchSize
	} else {
		body["cursor"] = cursor
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("marshal body: %w", err)
	}

	// OpenSearch SQL endpoint (ES uses /_sql; OpenSearch uses /_plugins/_sql).
	req, err := http.NewRequestWithContext(pageCtx, http.MethodPost, "/_plugins/_sql", bytes.NewReader(buf))
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if format != "" {
		q := req.URL.Query()
		q.Set("format", format)
		req.URL.RawQuery = q.Encode()
	}

	res, err := e.client.Perform(req)
	if err != nil {
		return nil, "", fmt.Errorf("request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		errBody, _ := io.ReadAll(res.Body)
		return nil, "", fmt.Errorf("status=%d body=%s", res.StatusCode, string(errBody))
	}

	// Parse the OpenSearch SQL response: schema + datarows (ES uses columns +
	// rows, a different shape). Cursor is present while more pages remain.
	var resp struct {
		Schema []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"schema"`
		DataRows [][]interface{} `json:"datarows"`
		Cursor   string          `json:"cursor"`
	}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return nil, "", fmt.Errorf("decode response: %w", err)
	}

	if len(resp.DataRows) == 0 {
		return nil, resp.Cursor, nil
	}

	// Convert to chunk-shaped maps. Column names map 1:1 to JSON keys.
	out := make([]map[string]interface{}, 0, len(resp.DataRows))
	for _, row := range resp.DataRows {
		cm := make(map[string]interface{}, len(resp.Schema))
		for i, col := range resp.Schema {
			if i < len(row) {
				cm[col.Name] = row[i]
			}
		}
		out = append(out, cm)
	}
	return out, resp.Cursor, nil
}

// isTimeoutError detects connection-level and per-attempt timeouts
// via context.DeadlineExceeded, net.Error.Timeout(), and substring
// matches (for SDKs that wrap without typed errors).
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := err.Error()
	for _, sub := range []string{"i/o timeout", "deadline exceeded", "connection timeout", "context deadline"} {
		if strings.Contains(msg, sub) {
			return true
		}
	}
	return false
}
