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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
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
		hostAddress = "http://127.0.0.1:9380"
	}

	zhipuAPIKey := os.Getenv("ZHIPU_AI_API_KEY")

	siliconFlowAPIKey := os.Getenv("SILICONFLOW_API_KEY")

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
		APIKey:                   os.Getenv("RAGFLOW_TEST_API_KEY"),
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
	return c.RequestJSONContext(context.Background(), method, path, body, params, extraHeaders)
}

func (c *ServerTestConfig) RequestJSONContext(ctx context.Context, method, path string, body any, params map[string]interface{}, extraHeaders map[string]string) (*http.Response, error) {
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

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header = c.buildHeaders(extraHeaders)
	return (&http.Client{Timeout: 90 * time.Second}).Do(req)
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

// A freshly started stack accepts TCP connections long before it can serve
// requests (admin-service bootstrap, database migrations, model init), so the
// session bootstrap retries "not ready yet" answers instead of failing the
// suite on the first attempt. This is what lets the workflow start the Go API
// tests right after `docker compose up -d` instead of running a separate
// readiness gate. Variables rather than constants so tests can shrink them.
var (
	bootstrapWait = 300 * time.Second
	bootstrapPoll = 2 * time.Second
)

// errServerStarting marks a reply that only means "not ready yet".
var errServerStarting = errors.New("server is not ready yet")

// bootstrapRetryable reports whether an attempt failed because the service is
// still starting rather than because the request itself was rejected.
func bootstrapRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errServerStarting) || errors.Is(err, io.EOF) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"admin server not connected",
		"connection refused",
		"connection reset",
		"no such host",
		"server closed idle connection",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

// Login bootstraps the test session: it registers this test user (a repeat
// registration is fine), signs in, and swaps the session token for the API key
// the REST calls authenticate with. Attempts that fail because the service is
// still starting are retried until the deadline.
func (c *ServerTestConfig) Login() error {
	deadline := time.Now().Add(bootstrapWait)
	for {
		err := c.loginOnce()
		if err == nil {
			return nil
		}
		if !bootstrapRetryable(err) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("service did not become ready within %s: %w", bootstrapWait, err)
		}
		fmt.Fprintf(os.Stderr, "Auth bootstrap: %v; retrying in %s\n", err, bootstrapPoll)
		time.Sleep(bootstrapPoll)
	}
}

func (c *ServerTestConfig) loginOnce() error {
	// Register (ignore already registered error)
	registerResp, err := c.PostJSON("/users", map[string]interface{}{
		"email":    c.Email,
		"nickname": "qa",
		"password": c.Password,
	}, nil)
	if err != nil {
		return fmt.Errorf("register request failed: %w", err)
	}
	defer registerResp.Body.Close()
	if registerResp.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: register status=%d", errServerStarting, registerResp.StatusCode)
	}
	registerPayload, err := c.decodeJSONBody(registerResp.Body)
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
	defer loginResp.Body.Close()
	if loginResp.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: login status=%d", errServerStarting, loginResp.StatusCode)
	}
	authToken := loginResp.Header.Get("Authorization")
	loginPayload, decodeErr := c.decodeJSONBody(loginResp.Body)
	if authToken == "" {
		if decodeErr == nil {
			return fmt.Errorf("login failed: code=%v, message=%v", loginPayload["code"], loginPayload["message"])
		}
		return fmt.Errorf("login failed: status=%d without an Authorization header", loginResp.StatusCode)
	}

	c.AuthToken = authToken

	// Get system token
	tokenResp, err := c.PostJSON("/system/tokens", nil, map[string]string{
		"Authorization": authToken,
	})
	if err != nil {
		return fmt.Errorf("system token request failed: %w", err)
	}
	defer tokenResp.Body.Close()
	if tokenResp.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: system token status=%d", errServerStarting, tokenResp.StatusCode)
	}
	tokenPayload, err := c.decodeJSONBody(tokenResp.Body)
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

// ---- Session preparation: providers, instances, tenant default models ----
// The REST suite addresses models as "<model>@<instance>@<provider>" (see
// memoryLLMModel / memoryEmbeddingModel), so it needs the same providers,
// instances, and tenant defaults that the deleted pytest tree used to set up.
// A fresh deployment ships the Builtin embedding model only, and GitHub CI has
// no ZHIPU/SILICONFLOW credentials, so the suite registers the rest itself.
const (
	defaultModelInstance  = "CI"
	defaultChatProvider   = "ZHIPU-AI"
	defaultChatModel      = "glm-4-flash"
	defaultEmbedProvider  = "Builtin"
	defaultEmbedInstance  = "Local"
	defaultEmbedModel     = "BAAI/bge-small-en-v1.5"
	defaultRerankProvider = "SILICONFLOW"
	defaultRerankModel    = "BAAI/bge-reranker-v2-m3"
)

// defaultModelSpec describes one provider the suite has to have available.
// bindOnly limits which catalog entries the provider instance binds: without
// it the server binds (and validates) the provider's entire catalog, including
// models the suite never uses.
type defaultModelSpec struct {
	provider string
	apiKey   func(*ServerTestConfig) string
	bindOnly []apiObject
}

func defaultModelSpecs() []defaultModelSpec {
	return []defaultModelSpec{
		{
			provider: defaultChatProvider,
			apiKey:   func(c *ServerTestConfig) string { return c.ZhipuAPIKey },
		},
		{
			provider: defaultRerankProvider,
			apiKey:   func(c *ServerTestConfig) string { return c.SiliconFlowAPIKey },
			bindOnly: []apiObject{
				{"model_type": []string{"rerank"}, "model_name": defaultRerankModel, "max_tokens": 8192},
				{"model_type": []string{"embedding"}, "model_name": "BAAI/bge-m3", "max_tokens": 8192},
				{"model_type": []string{"embedding"}, "model_name": "BAAI/bge-large-en-v1.5", "max_tokens": 512},
				{"model_type": []string{"embedding"}, "model_name": "BAAI/bge-large-zh-v1.5", "max_tokens": 512},
			},
		},
	}
}

// providerCredential reports the credential registered for a provider, empty
// when the environment has none and the fixture therefore leaves the provider
// and its defaults alone.
func providerCredential(provider string) string {
	for _, spec := range defaultModelSpecs() {
		if spec.provider == provider {
			return spec.apiKey(TestConfig)
		}
	}
	return ""
}

// defaultModelSelections are the tenant defaults the suite relies on: chat for
// the session/agent completion paths, embedding for datasets, rerank for the
// retrieval paths.
func defaultModelSelections() []apiObject {
	return []apiObject{
		{"model_provider": defaultChatProvider, "model_instance": defaultModelInstance, "model_type": "chat", "model_name": defaultChatModel},
		{"model_provider": defaultEmbedProvider, "model_instance": defaultEmbedInstance, "model_type": "embedding", "model_name": defaultEmbedModel},
		{"model_provider": defaultRerankProvider, "model_instance": defaultModelInstance, "model_type": "rerank", "model_name": defaultRerankModel},
	}
}

// apiCallError is a non-2xx application envelope returned by the server.
type apiCallError struct {
	method  string
	path    string
	code    float64
	message string
}

func (e *apiCallError) Error() string {
	return fmt.Sprintf("%s %s: code=%v message=%s", e.method, e.path, e.code, e.message)
}

// alreadyExists reports the "the thing you asked for is already there" family
// of errors, which are success for an idempotent setup step.
func alreadyExists(err error) bool {
	call, ok := asAPICallError(err)
	if !ok {
		return false
	}
	message := strings.ToLower(call.message)
	return strings.Contains(message, "duplicat") || strings.Contains(message, "already exist")
}

func asAPICallError(err error) (*apiCallError, bool) {
	for err != nil {
		if call, ok := err.(*apiCallError); ok {
			return call, true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return nil, false
		}
		err = unwrapper.Unwrap()
	}
	return nil, false
}

// callAPI performs a request as the configured tenant and returns the decoded
// envelope, failing on any non-zero application code.
func callAPI(method, path string, body apiObject) (apiObject, error) {
	response, err := TestConfig.RequestJSON(method, path, body, nil, nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var payload apiObject
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("%s %s: non-JSON response (status %d) %s", method, path, response.StatusCode, diagnosticBody(raw, TestConfig.APIKey, TestConfig.Password))
	}
	code, _ := payload["code"].(float64)
	if code != 0 {
		message, _ := payload["message"].(string)
		return payload, &apiCallError{method: method, path: path, code: code, message: message}
	}
	return payload, nil
}

// tenantProviders lists the provider and "<provider>/<instance>" pairs already
// added to the tenant, so the setup below only creates what is missing.
func tenantProviders() (map[string]bool, error) {
	payload, err := callAPI(http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	present := map[string]bool{}
	items, _ := payload["data"].([]any)
	for _, item := range items {
		model, ok := item.(map[string]any)
		if !ok {
			continue
		}
		provider, _ := model["provider_name"].(string)
		instance, _ := model["instance_name"].(string)
		if provider == "" {
			continue
		}
		present[strings.ToLower(provider)] = true
		if instance != "" {
			present[strings.ToLower(provider+"/"+instance)] = true
		}
	}
	return present, nil
}

// prepareDefaultModels mirrors the pytest session fixture
// (test/testcases/conftest.py::set_tenant_info) that used to run before this
// suite. It is best-effort by design: providers are registered without usable
// credentials in CI, so a failure here must not abort the run. Tests that
// genuinely need a model report their own error, which is how the Python
// fixture behaved too (it downgraded the same failures to warnings).
func prepareDefaultModels(logf func(format string, args ...any)) {
	present, err := tenantProviders()
	if err != nil {
		logf("default models: cannot list the tenant's models: %v", err)
		return
	}

	// Builtin (the TEI embedding model) needs no credential of its own.
	usable := map[string]bool{defaultEmbedProvider: true}
	created := make([]string, 0, 2)
	failures := 0

	for _, spec := range defaultModelSpecs() {
		// Without a credential for the provider the model can never serve a
		// request, so registering it and pointing the tenant's default at it
		// would only turn the suite's explicit "no default model" skips into
		// authentication failures. Leave that provider alone instead.
		if spec.apiKey(TestConfig) == "" {
			logf("default models: no %s credential in the environment; leaving %s defaults untouched", spec.provider, spec.provider)
			continue
		}
		usable[spec.provider] = true

		if !present[strings.ToLower(spec.provider)] {
			if _, err := callAPI(http.MethodPut, "/providers", apiObject{"provider_name": spec.provider}); err != nil && !alreadyExists(err) {
				logf("default models: register provider %s: %v", spec.provider, err)
				failures++
				continue
			}
			created = append(created, spec.provider)
		}
		if present[strings.ToLower(spec.provider+"/"+defaultModelInstance)] {
			continue
		}
		instance := apiObject{
			"instance_name": defaultModelInstance,
			"api_key":       spec.apiKey(TestConfig),
			"region":        "default",
			"base_url":      "",
		}
		if len(spec.bindOnly) > 0 {
			instance["model_info"] = spec.bindOnly
		}
		if _, err := callAPI(http.MethodPost, "/providers/"+spec.provider+"/instances", instance); err != nil && !alreadyExists(err) {
			logf("default models: create instance %s/%s: %v", spec.provider, defaultModelInstance, err)
			failures++
			continue
		}
		created = append(created, spec.provider+"/"+defaultModelInstance)
	}

	selected, skipped := 0, 0
	for _, selection := range defaultModelSelections() {
		provider, _ := selection["model_provider"].(string)
		if !usable[provider] {
			skipped++
			continue
		}
		if _, err := callAPI(http.MethodPatch, "/models/default", selection); err != nil {
			logf("default models: set default %v model (%s@%s@%s): %v",
				selection["model_type"], selection["model_name"], selection["model_instance"], selection["model_provider"], err)
			failures++
			continue
		}
		selected++
	}

	// One summary line so a CI log proves the fixture ran and shows whether
	// the defaults are actually in place.
	logf("default models: %d/%d defaults selected (%d skipped for lack of credentials), %d failure(s), created %v",
		selected, len(defaultModelSelections()), skipped, failures, created)
}
