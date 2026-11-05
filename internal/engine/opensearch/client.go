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

package opensearch

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"ragflow/internal/server/config"
	"ragflow/internal/utility"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

// Engine is the OpenSearch engine implementation.
type Engine struct {
	transportClient *opensearch.Client
	apiClient       *opensearchapi.Client
	httpTransport   *http.Transport
	version         string
	pipelineMgr     *pipelineManager
}

// parseAddresses validates and extracts addresses from a comma-separated hosts string.
// Every host address must explicitly include a supported scheme (http:// or https://).
func parseAddresses(rawHosts string) ([]string, error) {
	rawHosts = strings.TrimSpace(rawHosts)
	if rawHosts == "" {
		return nil, fmt.Errorf("opensearch hosts configuration is required")
	}

	var addresses []string
	for _, h := range strings.Split(rawHosts, ",") {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if !strings.HasPrefix(h, "http://") && !strings.HasPrefix(h, "https://") {
			return nil, fmt.Errorf("invalid opensearch host %q: missing or unsupported scheme (must start with http:// or https://)", h)
		}
		u, err := url.Parse(h)
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("invalid opensearch host %q: missing host in URL", h)
		}
		addresses = append(addresses, h)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("opensearch hosts configuration is required")
	}
	return addresses, nil
}

// NewEngine creates an OpenSearch engine and initializes required templates.
func NewEngine(ctx context.Context, osConfig config.OpenSearchConfig) (*Engine, error) {
	addresses, err := parseAddresses(osConfig.Hosts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse opensearch hosts: %w", err)
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: osConfig.InsecureSkipVerify,
	}

	if osConfig.CACertPath != "" {
		caCert, err := os.ReadFile(osConfig.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate from %s: %w", osConfig.CACertPath, err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to append CA certificate to pool")
		}
		tlsConfig.RootCAs = caCertPool
	}

	if (osConfig.ClientCertPath != "") != (osConfig.ClientKeyPath != "") {
		return nil, fmt.Errorf("both client_cert_path and client_key_path must be provided for mTLS")
	}

	if osConfig.ClientCertPath != "" && osConfig.ClientKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(osConfig.ClientCertPath, osConfig.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   10 * time.Second,
		MaxIdleConns:          1000,
		MaxIdleConnsPerHost:   300,
		MaxConnsPerHost:       500,
		IdleConnTimeout:       60 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		TLSClientConfig:       tlsConfig,
		ForceAttemptHTTP2:     true,
	}

	clientConfig := opensearch.Config{
		Addresses: addresses,
		Username:  osConfig.Username,
		Password:  osConfig.Password,
		Transport: transport,
	}

	apiClient, err := opensearchapi.NewClient(opensearchapi.Config{
		Client: clientConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenSearch client: %w", err)
	}

	newCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	infoResp, err := apiClient.Info(newCtx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to ping OpenSearch: %w", err)
	}
	version := infoResp.Version.Number

	pm, err := newPipelineManager(version)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OpenSearch: %w", err)
	}

	engine := &Engine{
		transportClient: apiClient.Client,
		apiClient:       apiClient,
		httpTransport:   transport,
		version:         version,
		pipelineMgr:     pm,
	}

	// 1. Template for chunk indices (ragflow_*) - priority 1
	if err = engine.CreateIndexTemplate(newCtx, "ragflow_os_mapping", "ragflow_*", "os_mapping.json", 1); err != nil {
		return nil, fmt.Errorf("failed to create chunk index template: %w", err)
	}
	// 2. Template for doc_meta indices (ragflow_doc_meta_*) - priority 2
	if err = engine.CreateIndexTemplate(newCtx, "ragflow_doc_meta_mapping", "ragflow_doc_meta_*", "doc_meta_es_mapping.json", 2); err != nil {
		return nil, fmt.Errorf("failed to create doc_meta index template: %w", err)
	}
	// 3. Template for memory indices (memory_*) - priority 3
	if err = engine.CreateIndexTemplate(newCtx, "ragflow_memory_mapping", "memory_*", "os_mapping.json", 3); err != nil {
		return nil, fmt.Errorf("failed to create memory index template: %w", err)
	}

	return engine, nil
}

// GetType returns "opensearch".
func (e *Engine) GetType() string {
	return "opensearch"
}

// SupportsPageRank returns true because OpenSearch supports PageRank field updates.
func (e *Engine) SupportsPageRank() bool {
	return true
}

// Version returns the detected OpenSearch server version.
func (e *Engine) Version() string {
	return e.version
}

// Ping checks if OpenSearch is accessible.
func (e *Engine) Ping(ctx context.Context) error {
	infoResp, err := e.apiClient.Info(ctx, nil)
	if err != nil {
		return err
	}
	if infoResp == nil || (infoResp.Inspect().Response != nil && infoResp.Inspect().Response.StatusCode >= 400) {
		return fmt.Errorf("opensearch ping returned status error")
	}
	return nil
}

// Close closes the engine and releases idle HTTP transport connections.
func (e *Engine) Close() error {
	if e.httpTransport != nil {
		e.httpTransport.CloseIdleConnections()
	}
	return nil
}

// CreateIndexTemplate creates an index template with specified mapping.
func (e *Engine) CreateIndexTemplate(ctx context.Context, templateName, indexPattern, mappingFileName string, priority ...int) error {
	if templateName == "" || indexPattern == "" {
		return fmt.Errorf("template name and index pattern cannot be empty")
	}

	p := 1
	if len(priority) > 0 {
		p = priority[0]
	}

	if mappingFileName == "" {
		mappingFileName = "os_mapping.json"
	}

	mappingPath, err := utility.FindConfFileInProject(mappingFileName)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(*mappingPath)
	if err != nil {
		return fmt.Errorf("failed to read mapping file %q: %w", *mappingPath, err)
	}

	var mapping map[string]interface{}
	if err = json.Unmarshal(data, &mapping); err != nil {
		return fmt.Errorf("failed to parse mapping file %q: %w", *mappingPath, err)
	}

	// In OpenSearch, top-level "dynamic": "runtime" is invalid; rewrite to true.
	if m, ok := mapping["mappings"].(map[string]interface{}); ok {
		if dyn, ok := m["dynamic"].(string); ok && dyn == "runtime" {
			m["dynamic"] = true
		}
	}

	templateSettings := mapping["settings"]
	templateMappings := mapping["mappings"]

	// Try modern Composable Index Template first (PUT /_index_template/{name})
	templateBody := map[string]interface{}{
		"index_patterns": []string{indexPattern},
		"priority":       p,
		"template": map[string]interface{}{
			"settings": templateSettings,
			"mappings": templateMappings,
		},
	}

	templateBytes, err := json.Marshal(templateBody)
	if err != nil {
		return fmt.Errorf("failed to marshal template: %w", err)
	}

	resp, err := e.apiClient.IndexTemplate.Create(ctx, opensearchapi.IndexTemplateCreateReq{
		IndexTemplate: templateName,
		Body:          bytes.NewReader(templateBytes),
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Inspect().Response.Body)
			reason := extractErrorReason(bodyBytes)
			if reason != "" {
				return fmt.Errorf("index template creation failed: %s", reason)
			}
			return fmt.Errorf("index template creation failed: %s, body: %s",
				resp.Inspect().Response.Status(), string(bodyBytes))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to create index template: %w", err)
	}

	return nil
}
