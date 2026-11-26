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

package tool

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/common"
)

const (
	jin10ToolName         = "jin10"
	jin10ToolDescription  = "Fetch financial flash updates, economic calendars, symbols, quotes, and news from the Jin10 REST API."
	jin10BaseURL          = "https://open-data-api.jin10.com/data-api"
	jin10MaxResponseBytes = 2 << 20
)

type jin10Params struct {
	Type             string `json:"type,omitempty" enum:"flash,calendar,symbols,news" doc:"Data to fetch. Defaults to flash."`
	Category         string `json:"category,omitempty" doc:"Flash category (1–5, default 1) or calendar category (cj, qh, hk, us, default cj)."`
	CalendarDataType string `json:"calendar_datatype,omitempty" enum:"data,event,holiday" doc:"Calendar data type. Defaults to data."`
	SymbolsDataType  string `json:"symbols_datatype,omitempty" enum:"symbols,quotes" doc:"Instrument list or quotes. Defaults to symbols."`
	SymbolsType      string `json:"symbols_type,omitempty" enum:"GOODS,FOREX,FUTURE,CRYPTO" doc:"Instrument market. Defaults to GOODS."`
	Codes            string `json:"codes,omitempty" doc:"Comma-separated instrument codes for quotes. Defaults to BTCUSD."`
	Contain          string `json:"contain,omitempty" doc:"Text to include in flash or news results."`
	Filter           string `json:"filter,omitempty" doc:"Text to exclude from flash or news results."`
}

type jin10Envelope struct {
	Items json.RawMessage `json:"items,omitempty"`
	Error string          `json:"_ERROR,omitempty"`
}

// Jin10Tool retrieves financial data from the Jin10 Open Data API.
type Jin10Tool struct {
	helper *HTTPHelper
}

// NewJin10Tool creates a Jin10 client that sends the secret-key header only
// to the configured API host.
func NewJin10Tool() *Jin10Tool {
	helper := NewHTTPHelper()
	helper.client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Jin10Tool{helper: helper}
}

// Info returns the tool's metadata for the chat model.
func (j *Jin10Tool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return BuildToolInfo(jin10ToolName, jin10ToolDescription, jin10Params{})
}

// InvokableRun dispatches a read-only Jin10 API call.
func (j *Jin10Tool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p jin10Params
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
			return jin10ErrJSON(fmt.Errorf("jin10: parse arguments: %w", err))
		}
	}
	apiKey := strings.TrimSpace(common.GetEnv(common.EnvJin10APIKey))
	if apiKey == "" {
		return jin10ErrJSON(fmt.Errorf("jin10: %s is not configured", common.EnvJin10APIKey))
	}
	endpoint, params, err := buildJin10Request(p)
	if err != nil {
		return jin10ErrJSON(err)
	}
	body, err := json.Marshal(params)
	if err != nil {
		return jin10ErrJSON(fmt.Errorf("jin10: encode request: %w", err))
	}

	resp, err := j.helper.Do(ctx, http.MethodGet, endpoint, string(body), "application/json", map[string]string{
		"Accept":     "application/json",
		"secret-key": apiKey,
	})
	if err != nil {
		if strings.Contains(err.Error(), apiKey) {
			err = errors.New(strings.ReplaceAll(err.Error(), apiKey, "[redacted]"))
		}
		return jin10ErrJSON(fmt.Errorf("jin10: request failed: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return jin10ErrJSON(fmt.Errorf("jin10: HTTP %d", resp.StatusCode))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, jin10MaxResponseBytes+1))
	if err != nil {
		return jin10ErrJSON(fmt.Errorf("jin10: read response: %w", err))
	}
	if len(raw) > jin10MaxResponseBytes {
		return jin10ErrJSON(errors.New("jin10: response too large"))
	}
	var result struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return jin10ErrJSON(fmt.Errorf("jin10: decode response: %w", err))
	}
	if len(result.Data) == 0 || string(result.Data) == "null" {
		return jin10ErrJSON(errors.New("jin10: response has no data"))
	}

	output, err := json.Marshal(jin10Envelope{Items: result.Data})
	if err != nil {
		return jin10ErrJSON(fmt.Errorf("jin10: encode result: %w", err))
	}
	return string(output), nil
}

func buildJin10Request(p jin10Params) (string, map[string]string, error) {
	query := url.Values{}
	params := make(map[string]string)
	var path string
	switch p.Type {
	case "", "flash":
		path = "/flash"
		if p.Category == "" || p.Category == "all" {
			p.Category = "1"
		}
		params["category"] = p.Category
		params["contain"], params["filter"] = p.Contain, p.Filter
		query.Set("category", p.Category)
	case "calendar":
		switch dataType := cmp.Or(p.CalendarDataType, "data"); dataType {
		case "data", "event", "holiday":
			path = "/calendar/" + dataType
		default:
			return "", nil, errors.New("jin10: calendar_datatype must be data, event, or holiday")
		}
		params["category"] = cmp.Or(p.Category, "cj")
		query.Set("category", params["category"])
	case "symbols":
		switch dataType := cmp.Or(p.SymbolsDataType, "symbols"); dataType {
		case "symbols":
			path = "/symbols"
		case "quotes":
			path = "/quotes"
			params["codes"] = cmp.Or(p.Codes, "BTCUSD")
		default:
			return "", nil, errors.New("jin10: symbols_datatype must be symbols or quotes")
		}
		params["type"] = cmp.Or(p.SymbolsType, "GOODS")
		query.Set("type", params["type"])
	case "news":
		path = "/news"
		params["contain"], params["filter"] = p.Contain, p.Filter
	default:
		return "", nil, errors.New("jin10: type must be flash, calendar, symbols, or news")
	}
	endpoint := jin10BaseURL + path
	if len(query) != 0 {
		endpoint += "?" + query.Encode()
	}
	return endpoint, params, nil
}

func jin10ErrJSON(err error) (string, error) {
	output, _ := json.Marshal(jin10Envelope{Error: err.Error()})
	return string(output), err
}
