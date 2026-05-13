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

package utility

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// HTTPClient is a configurable HTTP client
type HTTPClient struct {
	host       string
	port       int
	useSSL     bool
	timeout    time.Duration
	headers    map[string]string
	httpClient *http.Client
}

// HTTPClientBuilder is a builder for HTTPClient
type HTTPClientBuilder struct {
	client *HTTPClient
}

// NewHTTPClientBuilder creates a new HTTPClientBuilder with default values
func NewHTTPClientBuilder() *HTTPClientBuilder {
	return &HTTPClientBuilder{
		client: &HTTPClient{
			host:    "localhost",
			port:    80,
			useSSL:  false,
			timeout: 30 * time.Second,
			headers: make(map[string]string),
		},
	}
}

// WithHost sets the host
func (b *HTTPClientBuilder) WithHost(host string) *HTTPClientBuilder {
	b.client.host = host
	return b
}

// WithPort sets the port
func (b *HTTPClientBuilder) WithPort(port int) *HTTPClientBuilder {
	b.client.port = port
	return b
}

// WithSSL enables or disables SSL
func (b *HTTPClientBuilder) WithSSL(useSSL bool) *HTTPClientBuilder {
	b.client.useSSL = useSSL
	return b
}

// WithTimeout sets the timeout duration
func (b *HTTPClientBuilder) WithTimeout(timeout time.Duration) *HTTPClientBuilder {
	b.client.timeout = timeout
	return b
}

// WithHeader adds a single header
func (b *HTTPClientBuilder) WithHeader(key, value string) *HTTPClientBuilder {
	b.client.headers[key] = value
	return b
}

// WithHeaders sets multiple headers
func (b *HTTPClientBuilder) WithHeaders(headers map[string]string) *HTTPClientBuilder {
	for key, value := range headers {
		b.client.headers[key] = value
	}
	return b
}

// Build creates the HTTPClient
func (b *HTTPClientBuilder) Build() *HTTPClient {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}

	// If SSL is disabled, allow insecure connections
	if !b.client.useSSL {
		transport.TLSClientConfig.InsecureSkipVerify = true
	}

	b.client.httpClient = &http.Client{
		Timeout:   b.client.timeout,
		Transport: transport,
	}

	return b.client
}

// SetHost sets the host
func (c *HTTPClient) SetHost(host string) {
	c.host = host
}

// SetPort sets the port
func (c *HTTPClient) SetPort(port int) {
	c.port = port
}

// SetSSL enables or disables SSL
func (c *HTTPClient) SetSSL(useSSL bool) {
	c.useSSL = useSSL
}

// SetTimeout sets the timeout duration
func (c *HTTPClient) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
	c.httpClient.Timeout = timeout
}

// SetHeader sets a single header
func (c *HTTPClient) SetHeader(key, value string) {
	c.headers[key] = value
}

// SetHeaders sets multiple headers
func (c *HTTPClient) SetHeaders(headers map[string]string) {
	c.headers = headers
}

// AddHeader adds a header without removing existing ones
func (c *HTTPClient) AddHeader(key, value string) {
	c.headers[key] = value
}

// GetHeaders returns a copy of all headers
func (c *HTTPClient) GetHeaders() map[string]string {
	headersCopy := make(map[string]string)
	for k, v := range c.headers {
		headersCopy[k] = v
	}
	return headersCopy
}

// GetBaseURL returns the base URL
func (c *HTTPClient) GetBaseURL() string {
	scheme := "http"
	if c.useSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, c.host, c.port)
}

// GetFullURL returns the full URL for a given path
func (c *HTTPClient) GetFullURL(path string) string {
	baseURL := c.GetBaseURL()
	// Ensure path starts with /
	if path != "" && path[0] != '/' {
		path = "/" + path
	}
	return baseURL + path
}

// prepareRequest creates an HTTP request with configured headers
func (c *HTTPClient) prepareRequest(method, urlStr string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, err
	}

	// Add configured headers
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	return req, nil
}

// Get performs a GET request
func (c *HTTPClient) Get(path string) (*http.Response, error) {
	urlStr := c.GetFullURL(path)
	req, err := c.prepareRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

// GetWithParams performs a GET request with query parameters
func (c *HTTPClient) GetWithParams(path string, params map[string]string) (*http.Response, error) {
	urlStr := c.GetFullURL(path)
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	u.RawQuery = query.Encode()

	req, err := c.prepareRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

// Post performs a POST request
func (c *HTTPClient) Post(path string, body []byte) (*http.Response, error) {
	urlStr := c.GetFullURL(path)
	req, err := c.prepareRequest(http.MethodPost, urlStr, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

// PostJSON performs a POST request with JSON content type
func (c *HTTPClient) PostJSON(path string, body []byte) (*http.Response, error) {
	c.SetHeader("Content-Type", "application/json")
	return c.Post(path, body)
}

// Put performs a PUT request
func (c *HTTPClient) Put(path string, body []byte) (*http.Response, error) {
	urlStr := c.GetFullURL(path)
	req, err := c.prepareRequest(http.MethodPut, urlStr, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

// Delete performs a DELETE request
func (c *HTTPClient) Delete(path string) (*http.Response, error) {
	urlStr := c.GetFullURL(path)
	req, err := c.prepareRequest(http.MethodDelete, urlStr, nil)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}

// Do performs a request with the given method
func (c *HTTPClient) Do(method, path string, body []byte) (*http.Response, error) {
	urlStr := c.GetFullURL(path)
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := c.prepareRequest(method, urlStr, bodyReader)
	if err != nil {
		return nil, err
	}
	return c.httpClient.Do(req)
}
