package esoscommon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ScanChunks reads a snapshot through a scroll cursor. It works for historical
// graph rows whose logical IDs are present only in hit metadata.
func ScanChunks(ctx context.Context, perform func(*http.Request) (*http.Response, error), indexes []string, query map[string]interface{}) ([]map[string]interface{}, error) {
	const keepAlive = "1m"
	const path = "/_search/scroll"
	type page struct {
		ID       string `json:"_scroll_id"`
		TimedOut bool   `json:"timed_out"`
		Shards   struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
		Hits struct {
			Rows []struct {
				ID     string                 `json:"_id"`
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	request := func(ctx context.Context, method, endpoint string, data interface{}) (page, int, error) {
		var result page
		payload, err := json.Marshal(data)
		if err != nil {
			return result, 0, err
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
		if err != nil {
			return result, 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := perform(req)
		if err != nil {
			return result, 0, err
		}
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return result, resp.StatusCode, err
		}
		return result, resp.StatusCode, nil
	}
	var scrollID string
	defer func() {
		if scrollID != "" {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			defer cancel()
			_, _, _ = request(cleanup, http.MethodDelete, path, map[string][]string{"scroll_id": {scrollID}})
		}
	}()

	escaped := make([]string, len(indexes))
	for i, index := range indexes {
		escaped[i] = url.PathEscape(index)
	}
	body := CopyFields(query)
	delete(body, "from")
	body["size"] = 1000
	body["sort"] = []string{"_doc"}
	result, status, err := request(ctx, http.MethodPost, "/"+strings.Join(escaped, ",")+"/_search?scroll="+keepAlive, body)
	if err == nil && status == http.StatusNotFound && result.Error.Type == "index_not_found_exception" {
		return nil, nil
	}
	var rows []map[string]interface{}
	for {
		if result.ID != "" {
			scrollID = result.ID
		}
		if err != nil {
			return nil, err
		}
		if status >= 300 || result.TimedOut || result.Shards.Failed > 0 {
			return nil, fmt.Errorf("chunk scroll failed (HTTP %d, timed out: %t, failed shards: %d, error: %s)", status, result.TimedOut, result.Shards.Failed, result.Error.Type)
		}
		if len(result.Hits.Rows) == 0 {
			return rows, nil
		}
		if result.ID == "" {
			return nil, fmt.Errorf("chunk scroll response is missing its cursor")
		}
		for _, hit := range result.Hits.Rows {
			row := hit.Source
			if row == nil {
				row = make(map[string]interface{}, 1)
			}
			row["id"] = hit.ID
			rows = append(rows, row)
		}
		result, status, err = request(ctx, http.MethodPost, path, map[string]string{"scroll": keepAlive, "scroll_id": scrollID})
	}
}
