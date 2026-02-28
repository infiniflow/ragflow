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
	"fmt"
	"strings"
	"sync"
	"time"
)

// BenchmarkResult holds the result of a benchmark run
type BenchmarkResult struct {
	Duration      float64
	TotalCommands int
	SuccessCount  int
	FailureCount  int
	QPS           float64
	ResponseList  []*Response
}

// RunBenchmark runs a benchmark with the given concurrency and iterations
func (c *RAGFlowClient) RunBenchmark(cmd *Command) error {
	concurrency, ok := cmd.Params["concurrency"].(int)
	if !ok {
		concurrency = 1
	}

	iterations, ok := cmd.Params["iterations"].(int)
	if !ok {
		iterations = 1
	}

	nestedCmd, ok := cmd.Params["command"].(*Command)
	if !ok {
		return fmt.Errorf("benchmark command not found")
	}

	if concurrency < 1 {
		return fmt.Errorf("concurrency must be greater than 0")
	}

	// Add iterations to the nested command
	nestedCmd.Params["iterations"] = iterations

	if concurrency == 1 {
		return c.runBenchmarkSingle(concurrency, iterations, nestedCmd)
	}
	return c.runBenchmarkConcurrent(concurrency, iterations, nestedCmd)
}

// runBenchmarkSingle runs benchmark with single concurrency (sequential execution)
func (c *RAGFlowClient) runBenchmarkSingle(concurrency, iterations int, nestedCmd *Command) error {
	commandType := nestedCmd.Type

	startTime := time.Now()
	responseList := make([]*Response, 0, iterations)

	// For search_on_datasets, convert dataset names to IDs first
	if commandType == "search_on_datasets" && iterations > 1 {
		datasets, _ := nestedCmd.Params["datasets"].(string)
		datasetNames := strings.Split(datasets, ",")
		datasetIDs := make([]string, 0, len(datasetNames))
		for _, name := range datasetNames {
			name = strings.TrimSpace(name)
			id, err := c.getDatasetID(name)
			if err != nil {
				return err
			}
			datasetIDs = append(datasetIDs, id)
		}
		nestedCmd.Params["dataset_ids"] = datasetIDs
	}

	// Check if command supports native benchmark (iterations > 1)
	supportsNative := false
	if iterations > 1 {
		result, err := c.ExecuteCommand(nestedCmd)
		if err == nil && result != nil {
			// Command supports benchmark natively
			supportsNative = true
			duration, _ := result["duration"].(float64)
			respList, _ := result["response_list"].([]*Response)
			responseList = respList

			// Calculate and print results
			successCount := 0
			for _, resp := range responseList {
				if isSuccess(resp, commandType) {
					successCount++
				}
			}

			qps := float64(0)
			if duration > 0 {
				qps = float64(iterations) / duration
			}

			fmt.Printf("command: %s, Concurrency: %d, iterations: %d\n", commandType, concurrency, iterations)
			fmt.Printf("total duration: %.4fs, QPS: %.2f, COMMAND_COUNT: %d, SUCCESS: %d, FAILURE: %d\n",
				duration, qps, iterations, successCount, iterations-successCount)
			return nil
		}
	}

	// Manual execution: run iterations times
	if !supportsNative {
		// Remove iterations param to avoid native benchmark
		delete(nestedCmd.Params, "iterations")

		for i := 0; i < iterations; i++ {
			singleResult, err := c.ExecuteCommand(nestedCmd)
			if err != nil {
				// Command failed, add a failed response
				responseList = append(responseList, &Response{StatusCode: 0})
				continue
			}

			// For commands that return a single response (like ping with iterations=1)
			if singleResult != nil {
				if respList, ok := singleResult["response_list"].([]*Response); ok {
					responseList = append(responseList, respList...)
				}
			} else {
				// Command executed successfully but returned no data
				// Mark as success for now
				responseList = append(responseList, &Response{StatusCode: 200, Body: []byte("pong")})
			}
		}
	}

	duration := time.Since(startTime).Seconds()

	successCount := 0
	for _, resp := range responseList {
		if isSuccess(resp, commandType) {
			successCount++
		}
	}

	qps := float64(0)
	if duration > 0 {
		qps = float64(iterations) / duration
	}

	// Print results
	fmt.Printf("command: %s, Concurrency: %d, iterations: %d\n", commandType, concurrency, iterations)
	fmt.Printf("total duration: %.4fs, QPS: %.2f, COMMAND_COUNT: %d, SUCCESS: %d, FAILURE: %d\n",
		duration, qps, iterations, successCount, iterations-successCount)

	return nil
}

// runBenchmarkConcurrent runs benchmark with multiple concurrent workers
func (c *RAGFlowClient) runBenchmarkConcurrent(concurrency, iterations int, nestedCmd *Command) error {
	results := make([]map[string]interface{}, concurrency)
	var wg sync.WaitGroup

	// For search_on_datasets, convert dataset names to IDs first
	if nestedCmd.Type == "search_on_datasets" {
		datasets, _ := nestedCmd.Params["datasets"].(string)
		datasetNames := strings.Split(datasets, ",")
		datasetIDs := make([]string, 0, len(datasetNames))
		for _, name := range datasetNames {
			name = strings.TrimSpace(name)
			id, err := c.getDatasetID(name)
			if err != nil {
				return err
			}
			datasetIDs = append(datasetIDs, id)
		}
		nestedCmd.Params["dataset_ids"] = datasetIDs
	}

	startTime := time.Now()

	// Launch concurrent workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Create a new client for each goroutine to avoid race conditions
			workerClient := NewRAGFlowClient(c.ServerType)
			workerClient.HTTPClient = c.HTTPClient // Share the same HTTP client config

			// Execute benchmark silently (no output)
			responseList := workerClient.executeBenchmarkSilent(nestedCmd, iterations)

			results[idx] = map[string]interface{}{
				"duration":      0.0,
				"response_list": responseList,
			}
		}(i)
	}

	wg.Wait()
	endTime := time.Now()

	totalDuration := endTime.Sub(startTime).Seconds()
	successCount := 0
	commandType := nestedCmd.Type

	for _, result := range results {
		if result == nil {
			continue
		}
		responseList, _ := result["response_list"].([]*Response)
		for _, resp := range responseList {
			if isSuccess(resp, commandType) {
				successCount++
			}
		}
	}

	totalCommands := iterations * concurrency
	qps := float64(0)
	if totalDuration > 0 {
		qps = float64(totalCommands) / totalDuration
	}

	// Print results
	fmt.Printf("command: %s, Concurrency: %d, iterations: %d\n", commandType, concurrency, iterations)
	fmt.Printf("total duration: %.4fs, QPS: %.2f, COMMAND_COUNT: %d, SUCCESS: %d, FAILURE: %d\n",
		totalDuration, qps, totalCommands, successCount, totalCommands-successCount)

	return nil
}

// executeBenchmarkSilent executes a command for benchmark without printing output
func (c *RAGFlowClient) executeBenchmarkSilent(cmd *Command, iterations int) []*Response {
	responseList := make([]*Response, 0, iterations)

	for i := 0; i < iterations; i++ {
		var resp *Response
		var err error

		switch cmd.Type {
		case "ping_server":
			resp, err = c.HTTPClient.Request("GET", "/system/ping", false, "web", nil, nil)
		case "list_user_datasets":
			resp, err = c.HTTPClient.Request("POST", "/kb/list", false, "web", nil, nil)
		case "list_datasets":
			userName, _ := cmd.Params["user_name"].(string)
			resp, err = c.HTTPClient.Request("GET", fmt.Sprintf("/admin/users/%s/datasets", userName), true, "admin", nil, nil)
		case "search_on_datasets":
			question, _ := cmd.Params["question"].(string)
			datasetIDs, _ := cmd.Params["dataset_ids"].([]string)
			payload := map[string]interface{}{
				"kb_id":                    datasetIDs,
				"question":                 question,
				"similarity_threshold":     0.2,
				"vector_similarity_weight": 0.3,
			}
			resp, err = c.HTTPClient.Request("POST", "/chunk/retrieval_test", false, "web", nil, payload)
		default:
			// For other commands, we would need to add specific handling
			// For now, mark as failed
			resp = &Response{StatusCode: 0}
		}

		if err != nil {
			resp = &Response{StatusCode: 0}
		}

		responseList = append(responseList, resp)
	}

	return responseList
}

// isSuccess checks if a response is successful based on command type
func isSuccess(resp *Response, commandType string) bool {
	if resp == nil {
		return false
	}

	switch commandType {
	case "ping_server":
		return resp.StatusCode == 200 && string(resp.Body) == "pong"
	case "list_user_datasets", "list_datasets", "search_on_datasets":
		// Check status code and JSON response code for dataset commands
		if resp.StatusCode != 200 {
			return false
		}
		resJSON, err := resp.JSON()
		if err != nil {
			return false
		}
		code, ok := resJSON["code"].(float64)
		return ok && code == 0
	default:
		// For other commands, check status code and response code
		if resp.StatusCode != 200 {
			return false
		}
		resJSON, err := resp.JSON()
		if err != nil {
			return false
		}
		code, ok := resJSON["code"].(float64)
		return ok && code == 0
	}
}
