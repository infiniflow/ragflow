package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type restAPIWindowTimeFormat string

const (
	restAPIWindowRFC3339          restAPIWindowTimeFormat = "rfc3339"
	restAPIWindowUnixSeconds      restAPIWindowTimeFormat = "unix_seconds"
	restAPIWindowUnixMilliseconds restAPIWindowTimeFormat = "unix_milliseconds"
)

type restAPIRequestWindow struct {
	startParam     string
	endParam       string
	initialStart   time.Time
	requestFormat  restAPIWindowTimeFormat
	responseFormat restAPIWindowTimeFormat
	maxSpan        time.Duration
}

type restAPIWindowRange struct {
	start time.Time
	end   time.Time
}

// ValidateRestAPIRequestWindow checks opt-in window settings without network access.
func ValidateRestAPIRequestWindow(config map[string]any) error {
	if config["request_window"] == nil {
		return nil
	}
	_, err := parseRestAPIConfig(config)
	return err
}

// HasRestAPIWindowScopeChanged reports edits that require a fresh scan from initial_start.
func HasRestAPIWindowScopeChanged(before, after map[string]any) bool {
	if before["request_window"] == nil && after["request_window"] == nil {
		return false
	}
	for _, key := range []string{"request_window", "url", "method", "query_params",
		"items_path", "id_field", "poll_timestamp_field", "pagination_type", "pagination_config"} {
		if !reflect.DeepEqual(before[key], after[key]) {
			return true
		}
	}
	return false
}

func newRestAPIWindowError(format string, args ...any) error {
	return &ConnectorValidationError{Message: fmt.Sprintf("REST API request_window: "+format, args...)}
}

func parseRestAPIRequestWindow(cfg *restAPIConfig, raw any) (*restAPIRequestWindow, error) {
	if raw == nil {
		return nil, nil
	}
	config, ok := raw.(map[string]any)
	if !ok {
		return nil, newRestAPIWindowError("must be an object")
	}
	if cfg.Method != "GET" {
		return nil, newRestAPIWindowError("requires the GET method")
	}
	if cfg.IDField == "" || cfg.PollTimestampField == "" {
		return nil, newRestAPIWindowError("id_field and poll_timestamp_field are required")
	}

	window := &restAPIRequestWindow{
		startParam: strings.TrimSpace(stringConfig(config["start_param"])),
		endParam:   strings.TrimSpace(stringConfig(config["end_param"])),
	}
	if window.startParam == "" || window.endParam == "" || window.startParam == window.endParam {
		return nil, newRestAPIWindowError("start_param and end_param must be different, non-empty query parameters")
	}
	var err error
	if window.requestFormat, err = parseRestAPIWindowTimeFormat(config["time_format"]); err != nil {
		return nil, newRestAPIWindowError("invalid time_format")
	}
	if window.responseFormat, err = parseRestAPIWindowTimeFormat(config["response_time_format"]); err != nil {
		return nil, newRestAPIWindowError("invalid response_time_format")
	}
	initialStart, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(stringConfig(config["initial_start"])))
	if err != nil || initialStart.IsZero() {
		return nil, newRestAPIWindowError("initial_start must be an RFC3339 timestamp with a time zone")
	}
	window.initialStart = window.requestFormat.normalize(initialStart)

	rawSpan := config["max_window_seconds"]
	if text, ok := rawSpan.(string); ok {
		rawSpan = strings.TrimSpace(text)
	}
	if rawSpan != nil && rawSpan != "" {
		seconds, valid := restAPIConfigInt(rawSpan)
		if f, ok := rawSpan.(float64); ok && math.Trunc(f) != f {
			valid = false
		}
		if !valid || seconds <= 0 || int64(seconds) > math.MaxInt64/int64(time.Second) {
			return nil, newRestAPIWindowError("max_window_seconds must be a positive integer")
		}
		window.maxSpan = time.Duration(seconds) * time.Second
	}

	if err := window.validateQuery(cfg); err != nil {
		return nil, err
	}
	return window, nil
}

func (w *restAPIRequestWindow) validateQuery(cfg *restAPIConfig) error {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return newRestAPIWindowError("invalid URL")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return newRestAPIWindowError("invalid URL query")
	}

	pagination := cfg.PaginationConfig
	var paginationParams []string
	switch cfg.PaginationType {
	case restAPIPaginationPage:
		paginationParams = []string{
			firstNonEmpty(strings.TrimSpace(stringConfig(pagination["page_param"])), "page"),
			strings.TrimSpace(stringConfig(pagination["page_size_param"])),
		}
	case restAPIPaginationOffset:
		paginationParams = []string{
			firstNonEmpty(strings.TrimSpace(stringConfig(pagination["offset_param"])), "offset"),
			strings.TrimSpace(stringConfig(pagination["limit_param"])),
		}
	case restAPIPaginationCursor:
		cursorParam := firstNonEmpty(strings.TrimSpace(stringConfig(pagination["cursor_param"])), "cursor")
		paginationParams = []string{cursorParam}
		_, fixedCursor := cfg.QueryParams[cursorParam]
		if query.Has(cursorParam) || fixedCursor || strings.TrimSpace(stringConfig(pagination["initial_cursor"])) != "" {
			return newRestAPIWindowError("each window must start without a saved cursor")
		}
		if firstNonEmpty(stringConfig(pagination["next_cursor_path"]), stringConfig(pagination["next_cursor_field"])) == "" {
			return newRestAPIWindowError("cursor pagination requires next_cursor_path or next_cursor_field")
		}
	}
	for _, name := range []string{w.startParam, w.endParam} {
		if _, exists := cfg.QueryParams[name]; exists || query.Has(name) || strings.Contains(u.Path, "{"+name+"}") {
			return newRestAPIWindowError("parameter %q conflicts with the URL or fixed query parameters", name)
		}
		for _, param := range paginationParams {
			if name == param {
				return newRestAPIWindowError("parameter %q conflicts with pagination", name)
			}
		}
	}
	return nil
}

func parseRestAPIWindowTimeFormat(raw any) (restAPIWindowTimeFormat, error) {
	format := restAPIWindowTimeFormat(firstNonEmpty(strings.TrimSpace(stringConfig(raw)), string(restAPIWindowRFC3339)))
	switch format {
	case restAPIWindowRFC3339, restAPIWindowUnixSeconds, restAPIWindowUnixMilliseconds:
		return format, nil
	default:
		return "", newRestAPIWindowError("unsupported time format")
	}
}

func (f restAPIWindowTimeFormat) normalize(t time.Time) time.Time {
	switch f {
	case restAPIWindowUnixSeconds:
		return t.UTC().Truncate(time.Second)
	case restAPIWindowUnixMilliseconds:
		return t.UTC().Truncate(time.Millisecond)
	default:
		// Sync logs persist the cutoff at microsecond precision.
		return t.UTC().Truncate(time.Microsecond)
	}
}

func (f restAPIWindowTimeFormat) encode(t time.Time) string {
	switch f {
	case restAPIWindowUnixSeconds:
		return strconv.FormatInt(t.Unix(), 10)
	case restAPIWindowUnixMilliseconds:
		return strconv.FormatInt(t.UnixMilli(), 10)
	default:
		return t.Format(time.RFC3339Nano)
	}
}

func (f restAPIWindowTimeFormat) parse(value any) (time.Time, error) {
	if f == restAPIWindowRFC3339 {
		return time.Parse(time.RFC3339Nano, strings.TrimSpace(stringConfig(value)))
	}
	timestamp, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	if f == restAPIWindowUnixMilliseconds {
		return time.UnixMilli(timestamp), nil
	}
	return time.Unix(timestamp, 0), nil
}

func (w *restAPIRequestWindow) segment(start, end time.Time) restAPIWindowRange {
	if w.maxSpan > 0 {
		if limit := start.Add(w.maxSpan); limit.Before(end) {
			end = limit
		}
	}
	return restAPIWindowRange{start: start, end: end}
}

func (c *RestAPIConnector) validateRequestWindowLive(ctx context.Context) error {
	window := c.cfg.RequestWindow
	end := window.requestFormat.normalize(time.Now())
	start := end.Add(-time.Hour)
	if window.maxSpan > 0 && window.maxSpan < time.Hour {
		start = end.Add(-window.maxSpan)
	}
	if start.Before(window.initialStart) {
		start = window.initialStart
	}
	if !start.Before(end) {
		return newRestAPIWindowError("initial_start must be earlier than the test cutoff")
	}
	it := newRestAPIItemIterator(c)
	it.window = &restAPIWindowRange{start: start, end: end}
	items, err := it.nextPage(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if _, err := c.itemToDocument(item); err != nil {
			return err
		}
	}
	return nil
}

func extractRestAPIWindowItems(response any, itemsPath string) ([]map[string]any, error) {
	var items []any
	if itemsPath == "" {
		items, _ = response.([]any)
		if dict, ok := response.(map[string]any); ok {
			for _, key := range []string{"items", "results", "data", "records"} {
				if list, ok := dict[key].([]any); ok {
					items = list
					break
				}
			}
		}
	} else {
		path := strings.TrimSuffix(itemsPath, "[*]")
		matches, err := restAPIJSONPathMatches(response, path)
		if err == nil && len(matches) == 1 {
			items, _ = matches[0].([]any)
		}
	}
	if items == nil {
		return nil, newRestAPIWindowError("the response must contain an array at items_path")
	}
	records := make([]map[string]any, 0, len(items))
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			return nil, newRestAPIWindowError("each item must be a JSON object")
		}
		records = append(records, record)
	}
	return records, nil
}

func extractRestAPIWindowNextCursor(response any, pagination map[string]any) (string, error) {
	var value any
	if path := strings.TrimSpace(stringConfig(pagination["next_cursor_path"])); path != "" {
		matches, err := restAPIJSONPathMatches(response, path)
		if err != nil || len(matches) != 1 {
			return "", newRestAPIWindowError("next_cursor_path did not match a single value")
		}
		value = matches[0]
	} else {
		record, _ := response.(map[string]any)
		field := strings.TrimSpace(stringConfig(pagination["next_cursor_field"]))
		var ok bool
		if value, ok = record[field]; !ok {
			return "", newRestAPIWindowError("next_cursor_field is missing from the response")
		}
	}
	switch cursor := value.(type) {
	case nil:
		return "", nil
	case string:
		return cursor, nil
	case json.Number:
		return cursor.String(), nil
	default:
		return "", newRestAPIWindowError("the next cursor must be a string, number or null")
	}
}
