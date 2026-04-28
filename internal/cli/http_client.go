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

package cli

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient handles HTTP requests to the RAGFlow server
type HTTPClient struct {
	Host           string
	Port           int
	APIVersion     string
	APIToken       string
	LoginToken     string
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	VerifySSL      bool
	client         *http.Client
	useAPIToken    bool
}

// NewHTTPClient creates a new HTTP client
func NewHTTPClient() *HTTPClient {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &HTTPClient{
		Host:           "127.0.0.1",
		Port:           9382,
		APIVersion:     "v1",
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    60 * time.Second,
		VerifySSL:      false,
		client: &http.Client{
			Transport: transport,
			Timeout:   60 * time.Second,
		},
	}
}

// APIBase returns the API base URL
func (c *HTTPClient) APIBase() string {
	return fmt.Sprintf("%s:%d/api/%s", c.Host, c.Port, c.APIVersion)
}

// NonAPIBase returns the non-API base URL
func (c *HTTPClient) NonAPIBase() string {
	return fmt.Sprintf("%s:%d/%s", c.Host, c.Port, c.APIVersion)
}

// BuildURL builds the full URL for a given path
func (c *HTTPClient) BuildURL(path string, useAPIBase bool) string {
	base := c.APIBase()
	if !useAPIBase {
		base = c.NonAPIBase()
	}
	if c.VerifySSL {
		return fmt.Sprintf("https://%s%s", base, path)
	}
	return fmt.Sprintf("http://%s%s", base, path)
}

// Headers builds the request headers
func (c *HTTPClient) Headers(authKind string, extra map[string]string) map[string]string {
	headers := make(map[string]string)

	switch authKind {
	case "api":
		if c.APIToken != "" {
			headers["Authorization"] = fmt.Sprintf("Bearer %s", c.APIToken)
		} else if c.LoginToken != "" {
			// Fallback to login token for API requests (user mode)
			headers["Authorization"] = fmt.Sprintf("Bearer %s", c.LoginToken)
		}
	case "web", "admin":
		if c.LoginToken != "" {
			headers["Authorization"] = c.LoginToken
		}
	}

	for k, v := range extra {
		headers[k] = v
	}
	return headers
}

// Response represents an HTTP response
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
	Duration   float64
}

// JSON parses the response body as JSON
func (r *Response) JSON() (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(r.Body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Request makes an HTTP request
func (c *HTTPClient) Request(method, path string, useAPIBase bool, authKind string, headers map[string]string, jsonBody map[string]interface{}) (*Response, error) {
	url := c.BuildURL(path, useAPIBase)
	mergedHeaders := c.Headers(authKind, headers)

	var body io.Reader
	if jsonBody != nil {
		jsonData, err := json.Marshal(jsonBody)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(jsonData)
		if mergedHeaders == nil {
			mergedHeaders = make(map[string]string)
		}
		mergedHeaders["Content-Type"] = "application/json"
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	for k, v := range mergedHeaders {
		req.Header.Set(k, v)
	}

	var resp *http.Response
	startTime := time.Now()
	resp, err = c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	duration := time.Since(startTime).Seconds()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header.Clone(),
		Duration:   duration,
	}, nil
}

// Request makes an HTTP request
func (c *HTTPClient) RequestWith2URL(method, webPath string, apiPath string, headers map[string]string, jsonBody map[string]interface{}) (*Response, error) {
	var path string
	var useAPIBase bool
	var authKind string
	if c.useAPIToken {
		path = apiPath
		useAPIBase = true
		authKind = "api"
	} else {
		path = webPath
		useAPIBase = false
		authKind = "web"
	}

	url := c.BuildURL(path, useAPIBase)
	mergedHeaders := c.Headers(authKind, headers)

	var body io.Reader
	if jsonBody != nil {
		jsonData, err := json.Marshal(jsonBody)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(jsonData)
		if mergedHeaders == nil {
			mergedHeaders = make(map[string]string)
		}
		mergedHeaders["Content-Type"] = "application/json"
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	for k, v := range mergedHeaders {
		req.Header.Set(k, v)
	}

	var resp *http.Response
	startTime := time.Now()
	resp, err = c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	duration := time.Since(startTime).Seconds()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header.Clone(),
		Duration:   duration,
	}, nil
}

// RequestWithIterations makes multiple HTTP requests for benchmarking
// Returns a map with "duration" (total time in seconds) and "response_list"
func (c *HTTPClient) RequestWithIterations(method, path string, useAPIBase bool, authKind string, headers map[string]string, jsonBody map[string]interface{}, iterations int) (*BenchmarkResponse, error) {
	response := new(BenchmarkResponse)

	if iterations <= 1 {
		start := time.Now()
		resp, err := c.Request(method, path, useAPIBase, authKind, headers, jsonBody)
		totalDuration := time.Since(start).Seconds()
		if err != nil {
			return nil, err
		}

		response.Code = resp.StatusCode
		response.Duration = totalDuration
		if response.Code == 0 {
			response.SuccessCount = 1
		} else {
			response.FailureCount = 1
		}
		return response, nil
	}

	url := c.BuildURL(path, useAPIBase)
	mergedHeaders := c.Headers(authKind, headers)

	var body io.Reader
	if jsonBody != nil {
		jsonData, err := json.Marshal(jsonBody)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(jsonData)
		if mergedHeaders == nil {
			mergedHeaders = make(map[string]string)
		}
		mergedHeaders["Content-Type"] = "application/json"
	}

	responseList := make([]*Response, 0, iterations)
	var totalDuration float64

	for i := 0; i < iterations; i++ {
		start := time.Now()

		var reqBody io.Reader
		if body != nil {
			// Need to create a new reader for each request
			jsonData, _ := json.Marshal(jsonBody)
			reqBody = bytes.NewReader(jsonData)
		}

		req, err := http.NewRequest(method, url, reqBody)
		if err != nil {
			return nil, err
		}

		for k, v := range mergedHeaders {
			req.Header.Set(k, v)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, err
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		responseList = append(responseList, &Response{
			StatusCode: resp.StatusCode,
			Body:       respBody,
			Headers:    resp.Header.Clone(),
		})

		totalDuration += time.Since(start).Seconds()
	}

	response.Code = 0
	response.Duration = totalDuration
	for _, resp := range responseList {
		if resp.StatusCode == 200 {
			response.SuccessCount++
		} else {
			response.FailureCount++
		}
	}

	return response, nil
}

// RequestJSON makes an HTTP request and returns JSON response
func (c *HTTPClient) RequestJSON(method, path string, useAPIBase bool, authKind string, headers map[string]string, jsonBody map[string]interface{}) (map[string]interface{}, error) {
	resp, err := c.Request(method, path, useAPIBase, authKind, headers, jsonBody)
	if err != nil {
		return nil, err
	}
	return resp.JSON()
}

// UploadMultipart uploads data using multipart/form-data
func (c *HTTPClient) UploadMultipart(path string, contentType string, body io.Reader) error {
	url := c.BuildURL(path, true)

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", contentType)
	if c.APIToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.APIToken))
	} else if c.LoginToken != "" {
		req.Header.Set("Authorization", c.LoginToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("upload failed: HTTP %d - %s", resp.StatusCode, string(respBody))
	}

	// Check response code
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && result.Code != 0 {
		return fmt.Errorf("upload failed: %s", result.Message)
	}

	return nil
}

// RequestStream makes an HTTP request for SSE streaming and returns the response body reader
func (c *HTTPClient) RequestStream(method, path string, useAPIBase bool, authKind string, headers map[string]string, jsonBody map[string]interface{}) (io.ReadCloser, error) {
	url := c.BuildURL(path, useAPIBase)
	mergedHeaders := c.Headers(authKind, headers)

	var body io.Reader
	if jsonBody != nil {
		jsonData, err := json.Marshal(jsonBody)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(jsonData)
		if mergedHeaders == nil {
			mergedHeaders = make(map[string]string)
		}
		mergedHeaders["Content-Type"] = "application/json"
	}
	// Add Accept header for SSE
	if mergedHeaders == nil {
		mergedHeaders = make(map[string]string)
	}
	mergedHeaders["Accept"] = "text/event-stream"

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	for k, v := range mergedHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return resp.Body, nil
}
