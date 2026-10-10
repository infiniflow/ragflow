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
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/utility"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"go.uber.org/zap"
)

const (
	esSQLRequestTimeout = 2 * time.Second
	esSQLFetchSize      = 128
)

const esSQLRetryAttempts = 2
const esSQLRetryDelay = 3 * time.Second

// prepareSQL renders checked JSON field expressions as request-local keyword
// fields. Reading _source preserves strings that dynamic keyword mapping omits.
func prepareSQL(sqlText string) (string, map[string]interface{}, int, error) {
	tokens, err := utility.SQLScan(sqlText)
	if err != nil {
		return "", nil, 0, err
	}
	runtime := make(map[string]interface{})
	limit := esSQLFetchSize
	depth := 0
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token.IsPunct("(") {
			depth++
		}
		if token.IsPunct(")") {
			depth--
		}
		if depth == 0 && token.IsWord("limit") {
			if i+1 >= len(tokens) || tokens[i+1].Kind != utility.SQLNumber {
				return "", nil, 0, errors.New("LIMIT must be a non-negative integer")
			}
			limit, err = strconv.Atoi(tokens[i+1].Text)
			if err != nil || limit < 0 || limit > 1000 {
				return "", nil, 0, errors.New("LIMIT must be between 0 and 1000")
			}
		}
		switch {
		case token.IsWord("available_int"):
			// ES retrieval considers an omitted availability flag enabled.
			// Query-local defaulting also handles indices without its mapping.
			runtime["available_int"] = map[string]interface{}{
				"type":   "long",
				"script": "def value=params._source.available_int; emit(value == null ? 1L : ((Number)value).longValue());",
			}
		case token.IsWord("json_extract_string"), token.IsWord("json_extract"), token.IsWord("json_value"), token.IsWord("json_extract_isnull"):
			args, next, err := utility.SQLCallArguments(tokens, i)
			if err != nil || len(args) != 2 || len(args[0]) != 1 || !args[0][0].IsWord("chunk_data") || len(args[1]) != 1 || args[1][0].Kind != utility.SQLString {
				return "", nil, 0, errors.New("table JSON extraction requires chunk_data and a quoted column path")
			}
			key, ok := entity.TableDataKeyFromPath(args[1][0].Value)
			if !ok {
				return "", nil, 0, errors.New("table JSON extraction requires a canonical column key")
			}
			runtime[key] = map[string]interface{}{
				"type": "keyword",
				"script": map[string]interface{}{
					"source": "def cells=params._source.chunk_data; if (cells != null && cells[params.key] != null) emit(cells[params.key]);",
					"params": map[string]interface{}{"key": key},
				},
			}
			expression := strconv.Quote(key)
			if token.IsWord("json_extract_isnull") {
				expression = "( " + expression + " IS NULL )"
			}
			replacement, err := utility.SQLScan(expression)
			if err != nil {
				return "", nil, 0, err
			}
			tokens = append(append(append([]utility.SQLToken(nil), tokens[:i]...), replacement...), tokens[next:]...)
			i += len(replacement) - 1
		case token.IsPunct("=") && i+1 < len(tokens) && tokens[i+1].IsPunct("="):
			tokens = append(tokens[:i+1], tokens[i+2:]...)
		}
	}
	return utility.SQLRender(tokens, '"'), runtime, limit, nil
}

// RunSQL posts SQL to `/_sql`, translates the response into chunk-shaped maps.
// Returns (nil, nil) on empty rows; (nil, error) when retries exhausted.
func (e *Engine) RunSQL(ctx context.Context, tableName string, sqlText string, kbIDs []string, format string) ([]map[string]interface{}, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("Elasticsearch RunSQL: client not initialized")
	}
	if sqlText == "" {
		return nil, fmt.Errorf("Elasticsearch RunSQL: empty SQL")
	}

	common.Debug("ESConnection.sql get sql", zap.String("sql", sqlText))

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

func (e *Engine) runSQLOnce(ctx context.Context, sqlText string, format string) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, esSQLRequestTimeout)
	defer cancel()
	normalized, runtime, limit, err := prepareSQL(sqlText)
	if err != nil {
		return nil, err
	}
	if limit == 0 {
		return nil, nil
	}
	body := map[string]interface{}{"query": normalized, "fetch_size": min(esSQLFetchSize, limit)}
	if len(runtime) > 0 {
		body["runtime_mappings"] = runtime
	}
	cursor := ""
	defer func() {
		if cursor == "" {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), esSQLRequestTimeout)
		defer cancel()
		encoded, _ := json.Marshal(map[string]string{"cursor": cursor})
		request := esapi.SQLClearCursorRequest{Body: bytes.NewReader(encoded)}
		response, err := request.Do(cleanupCtx, e.client)
		if err != nil {
			common.Warn("ES SQL cursor cleanup failed", zap.Error(err))
			return
		}
		defer response.Body.Close()
		if response.IsError() {
			common.Warn("ES SQL cursor cleanup failed", zap.Int("status", response.StatusCode))
		}
	}()
	var names []string
	out := make([]map[string]interface{}, 0, min(esSQLFetchSize, limit))
	for {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		request := esapi.SQLQueryRequest{Body: bytes.NewReader(encoded), Format: format}
		response, err := request.Do(ctx, e.client)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		var page struct {
			Columns []struct {
				Name string `json:"name"`
			} `json:"columns"`
			Rows   [][]interface{} `json:"rows"`
			Cursor string          `json:"cursor"`
		}
		if response.IsError() {
			message, _ := io.ReadAll(response.Body)
			response.Body.Close()
			return nil, fmt.Errorf("status=%d body=%s", response.StatusCode, string(message))
		}
		err = json.NewDecoder(response.Body).Decode(&page)
		response.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
		cursor = page.Cursor
		if len(page.Columns) > 0 {
			names = names[:0]
			for _, column := range page.Columns {
				names = append(names, column.Name)
			}
		}
		for _, row := range page.Rows {
			chunk := make(map[string]interface{}, len(names))
			for i, name := range names {
				if i < len(row) {
					chunk[name] = row[i]
				}
			}
			out = append(out, chunk)
			if len(out) == limit {
				break
			}
		}
		if len(out) == limit || cursor == "" {
			break
		}
		body = map[string]interface{}{"cursor": cursor}
	}
	if len(out) == 0 {
		return nil, nil
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
