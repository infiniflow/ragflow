//go:build integration

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

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var TestConfig *ServerTestConfig

func init() {
	TestConfig = InitServerTestConfig()
	if TestConfig == nil {
		panic("TestConfig is nil")
	}
}

type ServerTestConfig struct {
	Priority          int
	HostAddress       string
	APIVersion        string
	ZhipuAPIKey       string
	SiliconFlowAPIKey string
	Email             string
	Password          string
	AuthToken         string
	APIKey            string

	InvalidAPIKey            string
	InvalidID                string
	DatasetNameLimit         int
	DocumentNameLimit        int
	ChatAssistantNameLimit   int
	SessionWithChatNameLimit int

	DefaultDataSetParser map[string]interface{}
}

func InitServerTestConfig() *ServerTestConfig {
	//fmt.Fprintf(os.Stderr, "InitServerTestConfig start\n")
	var priority int
	var err error
	testPriorityStr := os.Getenv("TEST_PRIORITY")
	if testPriorityStr == "" {
		priority = 0
	} else {
		priority, err = strconv.Atoi(testPriorityStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "TEST_PRIORITY is not a valid integer: %v\n", err)
			return nil
		}
	}

	hostAddress := os.Getenv("HOST_ADDRESS")
	if hostAddress == "" {
		hostAddress = "http://127.0.0.1:9384"
	}

	zhipuAPIKey := os.Getenv("ZHIPU_AI_API_KEY")
	if zhipuAPIKey == "" {
		fmt.Fprintf(os.Stderr, "ZHIPU_AI_API_KEY is not set\n")
		return nil
	}

	siliconFlowAPIKey := os.Getenv("SILICONFLOW_API_KEY")
	if siliconFlowAPIKey == "" {
		fmt.Fprintf(os.Stderr, "SILICONFLOW_API_KEY is not set\n")
		return nil
	}

	email := os.Getenv("RAGFLOW_TEST_EMAIL")
	if email == "" {
		email = "qa@infiniflow.org"
	}

	password := os.Getenv("RAGFLOW_TEST_PASSWORD")
	if password == "" {
		password = "ctAseGvejiaSWWZ88T/m4FQVOpQyUvP+x7sXtdv3feqZACiQleuewkUi35E16wSd5C5QcnkkcV9cYc8TKPTRZlxappDuirxghxoOvFcJxFU4ixLsD\nfN33jCHRoDUW81IH9zjij/vaw8IbVyb6vuwg6MX6inOEBRRzVbRYxXOu1wkWY6SsI8X70oF9aeLFp/PzQpjoe/YbSqpTq8qqrmHzn9vO+yvyYyvmDsphXe\nX8f7fp9c7vUsfOCkM+gHY3PadG+QHa7KI7mzTKgUTZImK6BZtfRBATDTthEUbbaTewY4H0MnWiCeeDhcbeQao6cFy1To8pE3RpmxnGnS8BsBn8w=="
	}

	invalidID := "00000000000000000000000000000000"

	defaultParserConfig := map[string]interface{}{
		"layout_recognize":   "DeepDOC",
		"chunk_token_num":    512,
		"delimiter":          "\n",
		"auto_keywords":      0,
		"auto_questions":     0,
		"html4excel":         false,
		"image_context_size": 0,
		"table_context_size": 0,
		"topn_tags":          3,
		"llm_id":             "glm-4-flash@CI@ZHIPU-AI",
		"parent_child": map[string]interface{}{
			"use_parent_child":   false,
			"children_delimiter": "\n",
		},
		"children_delimiter": "",
	}

	serverTestConfig := ServerTestConfig{
		Priority:                 priority,
		HostAddress:              hostAddress,
		APIVersion:               "v1",
		ZhipuAPIKey:              zhipuAPIKey,
		SiliconFlowAPIKey:        siliconFlowAPIKey,
		Email:                    email,
		Password:                 password,
		InvalidAPIKey:            "invalidapikey",
		InvalidID:                invalidID,
		DatasetNameLimit:         100,
		DocumentNameLimit:        100,
		ChatAssistantNameLimit:   100,
		SessionWithChatNameLimit: 100,
		DefaultDataSetParser:     defaultParserConfig,
	}

	//fmt.Fprintf(os.Stderr, "InitServerTestConfig init successfully\n")
	return &serverTestConfig
}

func (c *ServerTestConfig) apiBase() string {
	return fmt.Sprintf("%s/api/%s", c.HostAddress, c.APIVersion)
}

func (c *ServerTestConfig) buildHeaders(extra map[string]string) http.Header {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	for k, v := range extra {
		h.Set(k, v)
	}
	if c.AuthToken != "" && h.Get("Authorization") == "" {
		h.Set("Authorization", "Bearer "+c.AuthToken)
	} else {
		if c.APIKey != "" && h.Get("Authorization") == "" {
			h.Set("Authorization", "Bearer "+c.APIKey)
		}
	}

	return h
}

func (c *ServerTestConfig) RequestJSON(method, path string, body, params map[string]interface{}, extraHeaders map[string]string) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}

	u, err := url.Parse(c.apiBase() + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, fmt.Sprint(v))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(method, u.String(), bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header = c.buildHeaders(extraHeaders)
	return http.DefaultClient.Do(req)
}

func (c *ServerTestConfig) GetJSON(path string, params map[string]interface{}, extraHeaders map[string]string) (*http.Response, error) {
	return c.RequestJSON(http.MethodGet, path, nil, params, extraHeaders)
}

func (c *ServerTestConfig) PostJSON(path string, body map[string]interface{}, extraHeaders map[string]string) (*http.Response, error) {
	return c.RequestJSON(http.MethodPost, path, body, nil, extraHeaders)
}

func (c *ServerTestConfig) DeleteJSON(path string, body map[string]interface{}, extraHeaders map[string]string) (*http.Response, error) {
	return c.RequestJSON(http.MethodDelete, path, body, nil, extraHeaders)
}

func (c *ServerTestConfig) PutJSON(path string, body map[string]interface{}, extraHeaders map[string]string) (*http.Response, error) {
	return c.RequestJSON(http.MethodPut, path, body, nil, extraHeaders)
}

func (c *ServerTestConfig) PatchJSON(path string, body map[string]interface{}, extraHeaders map[string]string) (*http.Response, error) {
	return c.RequestJSON(http.MethodPatch, path, body, nil, extraHeaders)
}

func (c *ServerTestConfig) decodeJSONBody(body io.Reader) (map[string]interface{}, error) {
	b, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	if err = json.Unmarshal(b, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *ServerTestConfig) Login() error {
	// Register (ignore already registered error)
	registerResp, err := c.PostJSON("/users", map[string]interface{}{
		"email":    c.Email,
		"nickname": "qa",
		"password": c.Password,
	}, nil)
	if err != nil {
		return fmt.Errorf("register request failed: %w", err)
	}
	registerPayload, err := c.decodeJSONBody(registerResp.Body)
	registerResp.Body.Close()
	if err != nil {
		return fmt.Errorf("failed to decode register response: %w", err)
	}
	if registerResp.StatusCode != http.StatusOK {
		return fmt.Errorf("register failed: status=%d, payload=%v", registerResp.StatusCode, registerPayload)
	}
	registerCode, _ := registerPayload["code"].(float64)
	registerMessage, _ := registerPayload["message"].(string)
	if int(registerCode) != 0 && !strings.Contains(registerMessage, "has already registered") {
		return fmt.Errorf("register failed: code=%v, message=%s", registerPayload["code"], registerMessage)
	}

	// Login
	loginResp, err := c.PostJSON("/auth/login", map[string]interface{}{
		"email":    c.Email,
		"password": c.Password,
	}, nil)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	authToken := loginResp.Header.Get("Authorization")
	loginResp.Body.Close()
	if authToken == "" {
		return fmt.Errorf("login response missing Authorization header")
	}

	c.AuthToken = authToken

	// Get system token
	tokenResp, err := c.PostJSON("/system/tokens", nil, map[string]string{
		"Authorization": authToken,
	})
	if err != nil {
		return fmt.Errorf("system token request failed: %w", err)
	}
	tokenPayload, err := c.decodeJSONBody(tokenResp.Body)
	tokenResp.Body.Close()
	if err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}
	if tokenResp.StatusCode != http.StatusOK {
		return fmt.Errorf("system token failed: status=%d, payload=%v", tokenResp.StatusCode, tokenPayload)
	}
	tokenData, ok := tokenPayload["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("system token response data is not an object: %v", tokenPayload)
	}
	apiKey, ok := tokenData["token"].(string)
	if !ok || apiKey == "" {
		return fmt.Errorf("system token response data.token is not a valid string: %v", tokenData)
	}

	c.APIKey = apiKey
	return nil
}

func (c *ServerTestConfig) Logout() error {
	resp, err := c.PostJSON("/auth/logout", nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read logout response: %w", err)
	}
	var payload map[string]interface{}
	if err = json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("failed to decode logout response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("logout failed: status=%d, payload=%v", resp.StatusCode, payload)
	}
	code, _ := payload["code"].(float64)
	if int(code) != 0 {
		return fmt.Errorf("logout failed: code=%v, message=%v", payload["code"], payload["message"])
	}

	c.APIKey = ""
	c.AuthToken = ""
	return nil
}
