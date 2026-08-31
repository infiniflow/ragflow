//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	xquikSearchURL        = "https://xquik.com/api/v1/x/tweets/search"
	xquikDefaultPageSize  = 100
	xquikDefaultMaxPages  = 10
	xquikDefaultBatchSize = 32
	xquikMaxPageSize      = 10000
	xquikMaxPages         = 1000
)

type xquikConfig struct {
	apiKey       string
	query        string
	queryType    string
	pageSize     int
	maxPages     int
	batchSize    int
	requestDelay float64
}

// XquikConnector searches X posts through Xquik and maps them into RAGFlow documents.
type XquikConnector struct {
	cfg     xquikConfig
	baseURL string
}

// NewXquikConnector creates an Xquik connector from stored settings.
func NewXquikConnector(config map[string]any) (*XquikConnector, error) {
	credentials := configAnyMap(config["credentials"])
	apiKey := strings.TrimSpace(stringConfig(credentials["xquik_api_key"]))
	if apiKey == "" {
		return nil, &ConnectorMissingCredentialError{Message: "Xquik connector requires 'xquik_api_key' in credentials"}
	}

	query := strings.TrimSpace(stringConfig(config["query"]))
	if query == "" {
		return nil, &ConnectorValidationError{Message: "Xquik connector query is required"}
	}

	queryType, err := normalizeXquikQueryType(stringConfig(config["query_type"]))
	if err != nil {
		return nil, err
	}
	pageSize, err := xquikPositiveInt(config["page_size"], xquikDefaultPageSize, "page_size", xquikMaxPageSize)
	if err != nil {
		return nil, err
	}
	maxPages, err := xquikPositiveInt(config["max_pages"], xquikDefaultMaxPages, "max_pages", xquikMaxPages)
	if err != nil {
		return nil, err
	}
	batchSize, err := xquikPositiveInt(config["batch_size"], xquikDefaultBatchSize, "batch_size", 0)
	if err != nil {
		return nil, err
	}
	requestDelay := restAPIConfigFloat(config["request_delay"])
	if requestDelay < 0 {
		if config["request_delay"] != nil && strings.TrimSpace(stringConfig(config["request_delay"])) != "" {
			return nil, &ConnectorValidationError{Message: "Xquik connector request_delay must be a non-negative number"}
		}
		requestDelay = restAPIDefaultRequestDelay
	}

	return &XquikConnector{
		cfg: xquikConfig{
			apiKey:       apiKey,
			query:        query,
			queryType:    queryType,
			pageSize:     pageSize,
			maxPages:     maxPages,
			batchSize:    batchSize,
			requestDelay: requestDelay,
		},
		baseURL: xquikSearchURL,
	}, nil
}

func normalizeXquikQueryType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "latest":
		return "Latest", nil
	case "top":
		return "Top", nil
	default:
		return "", &ConnectorValidationError{Message: "Xquik connector query_type must be Latest or Top"}
	}
}

func xquikPositiveInt(value any, defaultValue int, field string, maximum int) (int, error) {
	if value == nil || strings.TrimSpace(stringConfig(value)) == "" {
		return defaultValue, nil
	}
	parsed, ok := restAPIConfigInt(value)
	if !ok || parsed <= 0 || maximum > 0 && parsed > maximum {
		rangeText := "a positive integer"
		if maximum > 0 {
			rangeText = fmt.Sprintf("from 1 to %d", maximum)
		}
		return 0, &ConnectorValidationError{Message: fmt.Sprintf("Xquik connector %s must be %s", field, rangeText)}
	}
	return parsed, nil
}

// Validate checks Xquik settings without network I/O.
func (c *XquikConnector) Validate(ctx context.Context) error {
	if c == nil {
		return &ConnectorValidationError{Message: "Xquik connector is nil"}
	}
	if c.cfg.apiKey == "" {
		return &ConnectorMissingCredentialError{Message: "Xquik connector requires 'xquik_api_key' in credentials"}
	}
	return nil
}

// ValidateConnectorSetting checks credentials with a one-post request.
func (c *XquikConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	delegate, err := c.restConnector(SyncRequest{}, true)
	if err != nil {
		return err
	}
	return delegate.ValidateLive(ctx)
}

// OpenSync opens a cursor-paginated Xquik search session.
func (c *XquikConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	delegate, err := c.restConnector(request, false)
	if err != nil {
		return nil, err
	}
	return delegate.OpenSync(ctx, request)
}

// OpenPrune reports that X search cannot enumerate a complete deletion snapshot.
func (c *XquikConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	return nil, ErrPruneUnsupported
}

func (c *XquikConnector) restConnector(request SyncRequest, validation bool) (*RestAPIConnector, error) {
	limit := c.cfg.pageSize
	paginationType := restAPIPaginationCursor
	maxPages := c.cfg.maxPages
	if validation {
		limit = 1
		paginationType = restAPIPaginationNone
		maxPages = 1
	}

	queryParams := map[string]any{
		"q":         c.cfg.query,
		"queryType": c.cfg.queryType,
		"limit":     limit,
	}
	if !validation {
		if !request.FromBeginning && request.WindowStart != nil {
			queryParams["sinceTime"] = request.WindowStart.UTC().Format(time.RFC3339Nano)
		}
		if !request.WindowEnd.IsZero() {
			queryParams["untilTime"] = request.WindowEnd.UTC().Format(time.RFC3339Nano)
		}
	}

	return NewRestAPIConnector(map[string]any{
		"url":          c.baseURL,
		"method":       "GET",
		"query_params": queryParams,
		"auth_type":    restAPIAuthAPIKeyHeader,
		"auth_config": map[string]any{
			"header_name": "x-api-key",
		},
		"credentials": map[string]any{
			"api_key": c.cfg.apiKey,
		},
		"items_path":     "$.tweets",
		"id_field":       "id",
		"content_fields": "text,author.username,createdAt,url",
		"metadata_fields": strings.Join([]string{
			"id", "createdAt", "url", "lang", "author.id", "author.username", "author.name",
			"author.verified", "likeCount", "replyCount", "retweetCount", "quoteCount",
			"viewCount", "bookmarkCount", "media[*].mediaUrl",
		}, ","),
		"pagination_type": paginationType,
		"pagination_config": map[string]any{
			"cursor_param":        "cursor",
			"next_cursor_field":   "next_cursor",
			"has_next_page_field": "has_next_page",
			"page_size":           c.cfg.pageSize,
		},
		"poll_timestamp_field": "createdAt",
		"batch_size":           c.cfg.batchSize,
		"max_pages":            maxPages,
		"request_delay":        c.cfg.requestDelay,
		"content_template":     "Author: @{author_username}\nPublished: {createdAt}\nURL: {url}\n\n{text}",
	})
}
