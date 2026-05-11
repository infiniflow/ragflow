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
	"encoding/json"
	"fmt"
	"net/http"
	"ragflow/internal/server"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// Engine Elasticsearch engine implementation
type elasticsearchEngine struct {
	client *elasticsearch.Client
	config *server.ElasticsearchConfig
}

// NewEngine creates an Elasticsearch engine
func NewEngine(cfg interface{}) (*elasticsearchEngine, error) {
	esConfig, ok := cfg.(*server.ElasticsearchConfig)
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

// GetClusterStats gets Elasticsearch cluster statistics
// Reference: curl -XGET "http://{es_host}/_cluster/stats" -H "kbn-xsrf: reporting"
func (e *elasticsearchEngine) GetClusterStats() (map[string]interface{}, error) {
	req := esapi.ClusterStatsRequest{}
	res, err := req.Do(context.Background(), e.client)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster stats: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch cluster stats returned error: %s", res.Status())
	}

	var rawStats map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&rawStats); err != nil {
		return nil, fmt.Errorf("failed to decode cluster stats: %w", err)
	}

	result := make(map[string]interface{})

	// Basic cluster info
	if clusterName, ok := rawStats["cluster_name"].(string); ok {
		result["cluster_name"] = clusterName
	}
	if status, ok := rawStats["status"].(string); ok {
		result["status"] = status
	}

	// Indices info
	if indices, ok := rawStats["indices"].(map[string]interface{}); ok {
		if count, ok := indices["count"].(float64); ok {
			result["indices"] = int(count)
		}
		if shards, ok := indices["shards"].(map[string]interface{}); ok {
			if total, ok := shards["total"].(float64); ok {
				result["indices_shards"] = int(total)
			}
		}
		if docs, ok := indices["docs"].(map[string]interface{}); ok {
			if docCount, ok := docs["count"].(float64); ok {
				result["docs"] = int64(docCount)
			}
			if deleted, ok := docs["deleted"].(float64); ok {
				result["docs_deleted"] = int64(deleted)
			}
		}
		if store, ok := indices["store"].(map[string]interface{}); ok {
			if sizeInBytes, ok := store["size_in_bytes"].(float64); ok {
				result["store_size"] = convertBytes(int64(sizeInBytes))
			}
			if totalDataSetSize, ok := store["total_data_set_size_in_bytes"].(float64); ok {
				result["total_dataset_size"] = convertBytes(int64(totalDataSetSize))
			}
		}
		if mappings, ok := indices["mappings"].(map[string]interface{}); ok {
			if fieldCount, ok := mappings["total_field_count"].(float64); ok {
				result["mappings_fields"] = int(fieldCount)
			}
			if dedupFieldCount, ok := mappings["total_deduplicated_field_count"].(float64); ok {
				result["mappings_deduplicated_fields"] = int(dedupFieldCount)
			}
			if dedupSize, ok := mappings["total_deduplicated_mapping_size_in_bytes"].(float64); ok {
				result["mappings_deduplicated_size"] = convertBytes(int64(dedupSize))
			}
		}
	}

	// Nodes info
	if nodes, ok := rawStats["nodes"].(map[string]interface{}); ok {
		if count, ok := nodes["count"].(map[string]interface{}); ok {
			if total, ok := count["total"].(float64); ok {
				result["nodes"] = int(total)
			}
		}
		if versions, ok := nodes["versions"].([]interface{}); ok {
			result["nodes_version"] = versions
		}
		if os, ok := nodes["os"].(map[string]interface{}); ok {
			if mem, ok := os["mem"].(map[string]interface{}); ok {
				if totalInBytes, ok := mem["total_in_bytes"].(float64); ok {
					result["os_mem"] = convertBytes(int64(totalInBytes))
				}
				if usedInBytes, ok := mem["used_in_bytes"].(float64); ok {
					result["os_mem_used"] = convertBytes(int64(usedInBytes))
				}
				if usedPercent, ok := mem["used_percent"].(float64); ok {
					result["os_mem_used_percent"] = usedPercent
				}
			}
		}
		if jvm, ok := nodes["jvm"].(map[string]interface{}); ok {
			if versions, ok := jvm["versions"].([]interface{}); ok && len(versions) > 0 {
				if version0, ok := versions[0].(map[string]interface{}); ok {
					if vmVersion, ok := version0["vm_version"].(string); ok {
						result["jvm_versions"] = vmVersion
					}
				}
			}
			if mem, ok := jvm["mem"].(map[string]interface{}); ok {
				if heapUsed, ok := mem["heap_used_in_bytes"].(float64); ok {
					result["jvm_heap_used"] = convertBytes(int64(heapUsed))
				}
				if heapMax, ok := mem["heap_max_in_bytes"].(float64); ok {
					result["jvm_heap_max"] = convertBytes(int64(heapMax))
				}
			}
		}
	}

	return result, nil
}

// convertBytes converts bytes to human readable format
func convertBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
		PB = 1024 * TB
	)

	if bytes >= PB {
		return fmt.Sprintf("%.2f pb", float64(bytes)/float64(PB))
	}
	if bytes >= TB {
		return fmt.Sprintf("%.2f tb", float64(bytes)/float64(TB))
	}
	if bytes >= GB {
		return fmt.Sprintf("%.2f gb", float64(bytes)/float64(GB))
	}
	if bytes >= MB {
		return fmt.Sprintf("%.2f mb", float64(bytes)/float64(MB))
	}
	if bytes >= KB {
		return fmt.Sprintf("%.2f kb", float64(bytes)/float64(KB))
	}
	return fmt.Sprintf("%d b", bytes)
}
