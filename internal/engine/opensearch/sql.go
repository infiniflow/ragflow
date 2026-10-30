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
	osSQLRetryAttempts  = 2
	osSQLRetryDelay     = 3 * time.Second
)

var whitespaceRe = regexp.MustCompile("[ `]+")
var lktksMatchRe = regexp.MustCompile(` ([a-z_]+_l?tks)( (?i:like) | ?= ?)'([^']+)'`)

// preprocessSQL normalizes SQL for OpenSearch: collapses whitespace/backticks,
// strips '%', and rewrites `<field>_l?tks like/= 'value'` into OpenSearch's
// named argument syntax: MATCH(fld, 'fine_tokenized', operator='OR', minimum_should_match='30%').
func preprocessSQL(sql string) string {
	sql = whitespaceRe.ReplaceAllString(sql, " ")
	sql = strings.ReplaceAll(sql, "%", "")

	// Collect replacements for <field>_l?tks like/= 'val'
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
			new: fmt.Sprintf(" MATCH(%s, '%s', operator='OR', minimum_should_match='30%%') ", fld, strings.ReplaceAll(fine, "'", "''")),
		})
	}
	for _, r := range replaces {
		sql = strings.Replace(sql, r.old, r.new, 1)
	}
	return sql
}

// RunSQL posts SQL to `/_plugins/_sql?format=jdbc`, translates the response into chunk-shaped maps.
// Returns (nil, nil) on empty rows (empty row sentinel protocol);
// returns (nil, error) when retries exhausted or non-timeout error occurs.
func (e *Engine) RunSQL(ctx context.Context, sqlText string, format string) ([]map[string]interface{}, error) {
	if e == nil || e.transportClient == nil {
		return nil, fmt.Errorf("OpenSearch RunSQL: client not initialized")
	}
	if strings.TrimSpace(sqlText) == "" {
		return nil, fmt.Errorf("OpenSearch RunSQL: empty SQL")
	}

	normFormat := strings.ToLower(strings.TrimSpace(format))
	if normFormat != "" && normFormat != "json" {
		return nil, fmt.Errorf("OpenSearch RunSQL: unsupported format %q, only json is supported", format)
	}

	common.Debug("OSConnection.sql get sql", zap.String("sql", sqlText))
	sqlText = preprocessSQL(sqlText)
	common.Debug("OSConnection.sql to opensearch", zap.String("sql", sqlText))

	var lastErr error
	for attempt := 0; attempt < osSQLRetryAttempts; attempt++ {
		rows, err := e.runSQLOnce(ctx, sqlText)
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
		common.Warn("OpenSearch request timeout",
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

func (e *Engine) runSQLOnce(ctx context.Context, sqlText string) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, osSQLRequestTimeout)
	defer cancel()

	body := map[string]interface{}{
		"query":      sqlText,
		"fetch_size": osSQLFetchSize,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	endpoint := "/_plugins/_sql?format=jdbc"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("create sql request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := e.transportClient.Stream(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sql request failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var osResp struct {
		Cursor string `json:"cursor"`
		Schema []struct {
			Name  string `json:"name"`
			Alias string `json:"alias"`
			Type  string `json:"type"`
		} `json:"schema"`
		Datarows [][]interface{} `json:"datarows"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&osResp); err != nil {
		return nil, fmt.Errorf("decode sql response: %w", err)
	}

	if osResp.Cursor != "" {
		defer e.closeSQLCursor(ctx, osResp.Cursor)
	}

	// Empty rows sentinel: return (nil, nil)
	if len(osResp.Datarows) == 0 {
		return nil, nil
	}

	if len(osResp.Schema) == 0 {
		return nil, fmt.Errorf("decode sql response: missing schema in JDBC response")
	}

	colNames := make([]string, len(osResp.Schema))
	for i, s := range osResp.Schema {
		if s.Alias != "" {
			colNames[i] = s.Alias
		} else {
			colNames[i] = s.Name
		}
	}

	out := make([]map[string]interface{}, 0, len(osResp.Datarows))
	for _, row := range osResp.Datarows {
		cm := make(map[string]interface{}, len(colNames))
		for i, name := range colNames {
			if i < len(row) {
				cm[name] = row[i]
			}
		}
		out = append(out, cm)
	}
	return out, nil
}

func (e *Engine) closeSQLCursor(ctx context.Context, cursor string) {
	if e == nil || e.transportClient == nil || cursor == "" {
		return
	}
	body := map[string]string{"cursor": cursor}
	buf, err := json.Marshal(body)
	if err != nil {
		return
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "/_plugins/_sql/close", bytes.NewReader(buf))
	if err != nil {
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := e.transportClient.Stream(httpReq)
	if err != nil {
		common.Debug("Failed to close OpenSearch SQL cursor", zap.Error(err))
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
}

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
