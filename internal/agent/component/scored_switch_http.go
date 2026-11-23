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
