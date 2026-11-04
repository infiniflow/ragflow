package component

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const scoringResponseLimit = 8 << 20

var scoringHTTPClient = &http.Client{Timeout: 120 * time.Second}

type scoringPoster interface {
	Post(ctx context.Context, call scoringCall) (body []byte, status int, err error)
}

type httpScoringPoster struct{}

func (httpScoringPoster) Post(ctx context.Context, call scoringCall) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, call.Endpoint, bytes.NewReader(call.Body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if call.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+call.APIKey)
	}
	for key, value := range call.Headers {
		req.Header.Set(key, value)
	}
	resp, err := scoringHTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, scoringResponseLimit))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func (c *ScoredSwitchComponent) posterOrDefault() scoringPoster {
	if c != nil && c.poster != nil {
		return c.poster
	}
	return httpScoringPoster{}
}

func statusError(status int, body []byte, apiKey string) error {
	snippet := redactSecret(truncateUTF8(body, 300), apiKey)
	return fmt.Errorf("model request failed with status %d: %s", status, snippet)
}
