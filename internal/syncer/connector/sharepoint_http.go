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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

var errSharePointResponseTooLarge = errors.New("SharePoint API response exceeds maximum size")

func (c *SharePointConnector) graphRequestAttempt(ctx context.Context, apiURL, token string, maxBody int64, backoff time.Duration) ([]byte, int, string, error) {
	if err := c.limiter.acquire(ctx); err != nil {
		return nil, 0, "", err
	}
	defer c.limiter.release()
	ctx, cancel := context.WithTimeout(ctx, sharepointRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close()
	retryAfter := resp.Header.Get("Retry-After")
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		c.limiter.postpone(sharePointRetryDelay(retryAfter, backoff, time.Now()))
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, retryAfter, sharePointGraphError(resp.StatusCode, body)
	}
	if int64(len(body)) > maxBody {
		return nil, resp.StatusCode, "", fmt.Errorf("%w of %d bytes", errSharePointResponseTooLarge, maxBody)
	}
	return body, resp.StatusCode, retryAfter, readErr
}

func sharePointRequestError(apiURL string, err error) error {
	if err == nil {
		return nil
	}
	// Redirect targets can contain download credentials. Keep only the original
	// Graph path and the underlying error, never query strings or signed URLs.
	var urlErr *url.Error
	for errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	endpoint, parseErr := url.Parse(apiURL)
	if parseErr != nil {
		return fmt.Errorf("SharePoint request: %w", err)
	}
	return fmt.Errorf("SharePoint GET %s: %w", endpoint.EscapedPath(), err)
}
