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
	"fmt"
	"io"
	"net/http"
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
	Token             string

	InvalidAPIKey            string
	InvalidID                string
	DatasetNameLimit         int
	DocumentNameLimit        int
	ChatAssistantNameLimit   int
	SessionWithChatNameLimit int

	DefaultDataSetParser map[string]interface{}
}

func InitServerTestConfig() *ServerTestConfig {
	fmt.Fprintf(os.Stderr, "InitServerTestConfig start\n")
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
		Email:                    "test@example.com",
		Password:                 "testpassword",
		InvalidAPIKey:            "invalidapikey",
		InvalidID:                invalidID,
		DatasetNameLimit:         100,
		DocumentNameLimit:        100,
		ChatAssistantNameLimit:   100,
		SessionWithChatNameLimit: 100,
		DefaultDataSetParser:     defaultParserConfig,
	}

	fmt.Fprintf(os.Stderr, "InitServerTestConfig init successfully\n")
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
	if c.Token != "" && h.Get("Authorization") == "" {
		h.Set("Authorization", "Bearer "+c.Token)
	}
	return h
}

func (c *ServerTestConfig) Request(method, path string, body io.Reader, extraHeaders map[string]string) (*http.Response, error) {
	normalizedPath := "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequest(method, c.apiBase()+normalizedPath, body)
	if err != nil {
		return nil, err
	}
	req.Header = c.buildHeaders(extraHeaders)
	return http.DefaultClient.Do(req)
}

func (c *ServerTestConfig) Get(path string, extraHeaders map[string]string) (*http.Response, error) {
	return c.Request(http.MethodGet, path, nil, extraHeaders)
}

func (c *ServerTestConfig) Post(path string, body io.Reader, extraHeaders map[string]string) (*http.Response, error) {
	return c.Request(http.MethodPost, path, body, extraHeaders)
}

func (c *ServerTestConfig) Delete(path string, extraHeaders map[string]string) (*http.Response, error) {
	return c.Request(http.MethodDelete, path, nil, extraHeaders)
}

func (c *ServerTestConfig) Put(path string, body io.Reader, extraHeaders map[string]string) (*http.Response, error) {
	return c.Request(http.MethodPut, path, body, extraHeaders)
}

func (c *ServerTestConfig) Patch(path string, body io.Reader, extraHeaders map[string]string) (*http.Response, error) {
	return c.Request(http.MethodPatch, path, body, extraHeaders)
}
