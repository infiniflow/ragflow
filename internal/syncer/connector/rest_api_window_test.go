package connector

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestRestAPIWindowCursorResume(t *testing.T) {
	withRestAPITestHooks(t)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	split := start.Add(30 * 24 * time.Hour)
	end := split.Add(24 * time.Hour)
	var mu sync.Mutex
	var requests []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		query := r.URL.Query()
		requests = append(requests, query)
		switch {
		case len(requests) == 1:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
		case query.Get("startTime") == start.Format(time.RFC3339) && !query.Has("cursor"):
			fmt.Fprint(w, `{"workOrders":[{"recordId":"a","title":"first","completedAt":"2026-10-01T08:00:00+08:00"},{"recordId":"b","title":"next window","completedAt":"2026-10-31T00:00:00Z"}],"nextCursor":"page-2"}`)
		case query.Get("startTime") == start.Format(time.RFC3339) && query.Get("cursor") == "page-2":
			fmt.Fprint(w, `{"workOrders":[],"nextCursor":null}`)
		case query.Get("startTime") == split.Format(time.RFC3339) && !query.Has("cursor"):
			fmt.Fprint(w, `{"workOrders":[{"recordId":"b","title":"second","completedAt":"2026-10-31T00:00:00Z"},{"recordId":"c","title":"third","completedAt":"2026-10-31T12:00:00Z"},{"recordId":"d","title":"cutoff","completedAt":"2026-11-01T00:00:00Z"}],"nextCursor":null}`)
		default:
			http.Error(w, "unexpected window or cursor", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	config := map[string]any{
		"url": server.URL + "?includeTranscript=false", "query_params": "limit=200",
		"items_path": "$.workOrders", "id_field": "recordId", "poll_timestamp_field": "completedAt",
		"content_fields": "title", "pagination_type": "cursor", "batch_size": 1, "request_delay": 0,
		"pagination_config": map[string]any{"next_cursor_path": "$.nextCursor"},
		"request_window": map[string]any{
			"start_param": "startTime", "end_param": "endTime", "initial_start": start.Format(time.RFC3339),
			"max_window_seconds": 2592000,
		},
	}
	c := mustRestAPIConnector(t, config)
	req := SyncRequest{FromBeginning: true, WindowEnd: end}
	session, err := c.OpenSync(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	first := nextRestAPIWindowTestBatch(t, session, "first")
	if !first.Documents[0].UpdatedAt.Equal(start) {
		t.Fatalf("offset timestamp did not map to the UTC boundary: %v", first.Documents[0].UpdatedAt)
	}
	second := nextRestAPIWindowTestBatch(t, session, "second")
	if second.Checkpoint == nil {
		t.Fatal("second window has no checkpoint")
	}

	req.Resume = second.Checkpoint
	session, err = c.OpenSync(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	nextRestAPIWindowTestBatch(t, session, "third")
	if _, err := session.NextBatch(t.Context()); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF at cutoff, got %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 5 {
		t.Fatalf("want five requests including rate-limit retry and resume, got %v", requests)
	}
	for i, query := range requests {
		wantStart, wantEnd, wantCursor := start, split, ""
		if i >= 3 {
			wantStart, wantEnd = split, end
		} else if i == 2 {
			wantCursor = "page-2"
		}
		if query.Get("startTime") != wantStart.Format(time.RFC3339) ||
			query.Get("endTime") != wantEnd.Format(time.RFC3339) ||
			query.Get("cursor") != wantCursor || query.Has("cursor") != (wantCursor != "") ||
			query.Get("limit") != "200" || query.Get("includeTranscript") != "false" {
			t.Errorf("request %d has unexpected query: %v", i, query)
		}
	}
}

func TestRestAPIWindowPageSettings(t *testing.T) {
	withRestAPITestHooks(t)
	start := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Millisecond)
	cutoff := start.Add(time.Minute)
	var mu sync.Mutex
	var requests []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		query := r.URL.Query()
		requests = append(requests, query)
		switch len(requests) {
		case 1, 4:
			fmt.Fprint(w, `{"payload":{"entries":[]},"hasNext":false}`)
		case 2:
			fmt.Fprintf(w, `{"payload":{"entries":[{"key":"1","text":"record 1","time":%d}]},"hasNext":true}`, start.UnixMilli())
		case 3:
			fmt.Fprintf(w, `{"payload":{"entries":[{"key":"2","text":"record 2","time":%d}]},"hasNext":false}`, cutoff.UnixMilli()-1)
		default:
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	c := mustRestAPIConnector(t, map[string]any{
		"url": server.URL, "items_path": "$.payload.entries[*]", "id_field": "key",
		"poll_timestamp_field": "time", "content_fields": "text", "request_delay": 0,
		"batch_size": 1, "pagination_type": "page",
		"pagination_config": map[string]any{
			"page_param": "p", "page_size_param": "size", "start_page": 0, "page_size": 50,
			"has_next_page_field": "hasNext",
		},
		"request_window": map[string]any{
			"start_param": "from", "end_param": "to", "initial_start": start.Format(time.RFC3339Nano),
			"time_format": "unix_milliseconds", "response_time_format": "unix_milliseconds",
			"max_window_seconds": "60",
		},
	})
	if err := c.ValidateConnectorSetting(t.Context(), nil); err != nil {
		t.Fatalf("test connection: %v", err)
	}
	session, err := c.OpenSync(t.Context(), SyncRequest{WindowEnd: cutoff})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	first := nextRestAPIWindowTestBatch(t, session, "record 1")
	if first.Checkpoint == nil || !first.Documents[0].UpdatedAt.Equal(start) {
		t.Fatalf("first page lost its checkpoint or timestamp: %+v", first)
	}
	nextRestAPIWindowTestBatch(t, session, "record 2")
	if _, err := session.NextBatch(t.Context()); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF after second page, got %v", err)
	}
	nextCutoff := cutoff.Add(time.Minute)
	session, err = c.OpenSync(t.Context(), SyncRequest{WindowStart: &cutoff, WindowEnd: nextCutoff})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.NextBatch(t.Context()); !errors.Is(err, io.EOF) {
		t.Fatalf("expected empty incremental window, got %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 4 {
		t.Fatalf("unexpected requests: %v", requests)
	}
	probeStart, errStart := strconv.ParseInt(requests[0].Get("from"), 10, 64)
	probeEnd, errEnd := strconv.ParseInt(requests[0].Get("to"), 10, 64)
	if errStart != nil || errEnd != nil || probeEnd-probeStart != 60000 || requests[0].Get("p") != "0" {
		t.Fatalf("expected one bounded first-page probe: %v", requests[0])
	}
	for i, query := range requests[1:] {
		wantStart, wantEnd, wantPage := start, cutoff, "0"
		if i == 1 {
			wantPage = "1"
		} else if i == 2 {
			wantStart, wantEnd = cutoff, nextCutoff
		}
		if query.Get("from") != strconv.FormatInt(wantStart.UnixMilli(), 10) ||
			query.Get("to") != strconv.FormatInt(wantEnd.UnixMilli(), 10) ||
			query.Get("p") != wantPage || query.Get("size") != "50" {
			t.Errorf("request %d has unexpected query: %v", i+1, query)
		}
	}
}

func TestRestAPIWindowRejectsInvalidPages(t *testing.T) {
	withRestAPITestHooks(t)
	for _, test := range []struct {
		name     string
		response string
	}{
		{"missing array", `{"error":"missing data"}`},
		{"invalid timestamp", `{"items":[{"id":"1","title":"record","updated":"invalid"}]}`},
		{"missing id", `{"items":[{"title":"record","updated":"2026-10-01T00:00:00Z"}]}`},
		{"page cap", `{"items":[],"hasNext":true}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(w, test.response)
			}))
			defer server.Close()
			c := mustRestAPIConnector(t, map[string]any{
				"url": server.URL, "items_path": "$.items", "id_field": "id", "content_fields": "title",
				"poll_timestamp_field": "updated", "max_pages": 1, "pagination_type": "page",
				"request_delay": 0, "pagination_config": map[string]any{},
				"request_window": map[string]any{
					"start_param": "from", "end_param": "to", "initial_start": "2026-10-01T00:00:00Z",
				},
			})
			if test.name == "page cap" {
				c.cfg.PaginationConfig["has_next_page_field"] = "hasNext"
			}
			session, err := c.OpenSync(t.Context(), SyncRequest{
				WindowEnd: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			var validationErr *ConnectorValidationError
			if _, err := session.NextBatch(t.Context()); !errors.As(err, &validationErr) {
				t.Fatalf("expected a validation error rather than a completed window, got %v", err)
			}
		})
	}
}

func TestRestAPIWindowRejectsConflictingParameters(t *testing.T) {
	for _, config := range []map[string]any{
		{"url": "https://example.com/records?from=2026"},
		{"query_params": "to=2026"},
		{"pagination_type": "page", "pagination_config": map[string]any{"page_param": "from"}},
		{"pagination_type": "cursor", "pagination_config": map[string]any{"initial_cursor": "old", "next_cursor_field": "next"}},
		{"method": "POST"},
	} {
		if config["url"] == nil {
			config["url"] = "https://example.com/records"
		}
		config["content_fields"] = "title"
		config["id_field"] = "id"
		config["poll_timestamp_field"] = "updated"
		config["request_window"] = map[string]any{
			"start_param": "from", "end_param": "to", "initial_start": "2026-10-01T00:00:00Z",
		}
		var validationErr *ConnectorValidationError
		if err := ValidateRestAPIRequestWindow(config); !errors.As(err, &validationErr) {
			t.Errorf("expected a validation error for %v, got %v", config, err)
		}
	}
}

func nextRestAPIWindowTestBatch(t *testing.T, session SyncSession, title string) SyncBatch {
	t.Helper()
	batch, err := session.NextBatch(t.Context())
	if err != nil || len(batch.Documents) != 1 || batch.Documents[0].SemanticIdentifier != title {
		t.Fatalf("expected one document named %q, got %v, %v", title, batch.Documents, err)
	}
	return batch
}
