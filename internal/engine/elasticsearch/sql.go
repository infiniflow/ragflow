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

package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"go.uber.org/zap"
)

const (
	esSQLRequestTimeout = 2 * time.Second
	esSQLFetchSize      = 128
)

const esSQLRetryAttempts = 2
const esSQLRetryDelay = 3 * time.Second

var whitespaceRe = regexp.MustCompile("[ `]+")
var lktksMatchRe = regexp.MustCompile(` ([a-z_]+_l?tks)( like | ?= ?)'([^']+)'`)

// Preprocess normalizes SQL for ES: collapses whitespace/backticks,
// strips '%', and rewrites `<field>_l?tks like/= 'value'` into a
// tokenized MATCH() call.
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

// RunSQL posts SQL to `/_sql`, translates the response into chunk-shaped maps.
// Returns (nil, nil) on empty rows; (nil, error) when retries exhausted.
func (e *elasticsearchEngine) RunSQL(ctx context.Context, tableName string, sqlText string, kbIDs []string, format string) ([]map[string]interface{}, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("Elasticsearch RunSQL: client not initialized")
	}
	if sqlText == "" {
		return nil, fmt.Errorf("Elasticsearch RunSQL: empty SQL")
	}

	common.Debug("ESConnection.sql get sql", zap.String("sql", sqlText))
	sqlText = Preprocess(sqlText)
	common.Debug("ESConnection.sql to es", zap.String("sql", sqlText))

	var lastErr error
	for attempt := 0; attempt < esSQLRetryAttempts; attempt++ {
		rows, err := e.runSQLOnce(ctx, sqlText, format)
		if err == nil {
			return rows, nil
		}
		lastErr = err
		if !isTimeoutError(err) {
			common.Warn("ESConnection.sql got exception",
				zap.String("sql", sqlText),
				zap.Error(err))
			return nil, fmt.Errorf("SQL error: %w\n\nSQL: %s", err, sqlText)
		}
		common.Warn("ES request timeout",
			zap.String("sql", sqlText),
			zap.Int("attempt", attempt+1),
			zap.Error(err))
		if attempt < esSQLRetryAttempts-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(esSQLRetryDelay):
			}
		}
	}
	common.Error(fmt.Sprintf("ESConnection.sql timeout after %d attempts. SQL: %s", esSQLRetryAttempts, sqlText), lastErr)
	return nil, fmt.Errorf("Elasticsearch RunSQL: timeout after %d attempts: %w", esSQLRetryAttempts, lastErr)
}

func (e *elasticsearchEngine) runSQLOnce(ctx context.Context, sqlText string, format string) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, esSQLRequestTimeout)
	defer cancel()

	body := map[string]interface{}{
		"query":      sqlText,
		"fetch_size": esSQLFetchSize,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	req := esapi.SQLQueryRequest{
		Body:   bytes.NewReader(buf),
		Format: format,
	}
	res, err := req.Do(ctx, e.client)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		errBody, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("status=%d body=%s", res.StatusCode, string(errBody))
	}

	// Parse the SQL response.
	var resp struct {
		Columns []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"columns"`
		Rows [][]interface{} `json:"rows"`
	}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(resp.Rows) == 0 {
		return nil, nil
	}

	// Convert to chunk-shaped maps. Column names map 1:1 to JSON keys.
	out := make([]map[string]interface{}, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		cm := make(map[string]interface{}, len(resp.Columns))
		for i, col := range resp.Columns {
			if i < len(row) {
				cm[col.Name] = row[i]
			}
		}
		out = append(out, cm)
	}
	return out, nil
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
