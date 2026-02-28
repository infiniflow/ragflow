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

package elasticsearch

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"ragflow/internal/config"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// Engine Elasticsearch engine implementation
type elasticsearchEngine struct {
	client *elasticsearch.Client
	config *config.ElasticsearchConfig
}

// NewEngine creates an Elasticsearch engine
func NewEngine(cfg interface{}) (*elasticsearchEngine, error) {
	esConfig, ok := cfg.(*config.ElasticsearchConfig)
	if !ok {
		return nil, fmt.Errorf("invalid Elasticsearch config type, expected *config.ElasticsearchConfig")
	}

	// Create ES client
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{esConfig.Hosts},
		Username:  esConfig.Username,
		Password:  esConfig.Password,
		Transport: &http.Transport{
			MaxIdleConnsPerHost:   10,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}

	// Check connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := esapi.InfoRequest{}
	res, err := req.Do(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to ping Elasticsearch: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("Elasticsearch returned error: %s", res.Status())
	}

	engine := &elasticsearchEngine{
		client: client,
		config: esConfig,
	}

	return engine, nil
}

// Type returns the engine type
func (e *elasticsearchEngine) Type() string {
	return "elasticsearch"
}

// Ping health check
func (e *elasticsearchEngine) Ping(ctx context.Context) error {
	req := esapi.InfoRequest{}
	res, err := req.Do(ctx, e.client)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch ping failed: %s", res.Status())
	}
	return nil
}

// Close closes the connection
func (e *elasticsearchEngine) Close() error {
	// Go-elasticsearch client doesn't have a Close method, connection is managed by the transport
	return nil
}
