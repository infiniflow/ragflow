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
func (c *RAGFlowClient) RunBenchmark(cmd *Command) (ResponseIf, error) {
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
		return nil, fmt.Errorf("benchmark command not found")
	}

	if concurrency < 1 {
		return nil, fmt.Errorf("concurrency must be greater than 0")
	}

	// Add iterations to the nested command
	nestedCmd.Params["iterations"] = iterations

	if concurrency == 1 {
		return c.runBenchmarkSingle(iterations, nestedCmd)
	}
	return c.runBenchmarkConcurrent(concurrency, iterations, nestedCmd)
}

// runBenchmarkSingle runs benchmark with single concurrency (sequential execution)
func (c *RAGFlowClient) runBenchmarkSingle(iterations int, nestedCmd *Command) (*BenchmarkResponse, error) {
	commandType := nestedCmd.Type

	// For search_on_datasets, convert dataset names to IDs first
	if commandType == "search_on_datasets" && iterations > 1 {
		datasets, _ := nestedCmd.Params["datasets"].(string)
		datasetNames := strings.Split(datasets, ",")
		datasetIDs := make([]string, 0, len(datasetNames))
		for _, name := range datasetNames {
			name = strings.TrimSpace(name)
			id, err := c.getDatasetID(name)
			if err != nil {
				return nil, err
			}
			datasetIDs = append(datasetIDs, id)
		}
		nestedCmd.Params["dataset_ids"] = datasetIDs
	}

	// Check if command supports native benchmark (iterations > 1)
	if iterations > 1 {
		result, err := c.ExecuteCommand(nestedCmd)
		// convert result to BenchmarkResponse
		benchmarkResponse := result.(*BenchmarkResponse)
		benchmarkResponse.Concurrency = 1
		return benchmarkResponse, err
	}

	result, err := c.ExecuteCommand(nestedCmd)
	if err != nil {
		fmt.Printf("fail to execute: %s", commandType)
		return nil, err
	}

	var benchmarkResponse BenchmarkResponse
	switch result.Type() {
	case "common":
		commonResponse := result.(*CommonResponse)
		benchmarkResponse.Code = commonResponse.Code
		benchmarkResponse.Duration = commonResponse.Duration
		if commonResponse.Code == 0 {
			benchmarkResponse.SuccessCount = 1
		} else {
			benchmarkResponse.FailureCount = 1
		}
	case "simple":
		simpleResponse := result.(*SimpleResponse)
		benchmarkResponse.Code = simpleResponse.Code
		benchmarkResponse.Duration = simpleResponse.Duration
		if simpleResponse.Code == 0 {
			benchmarkResponse.SuccessCount = 1
		} else {
			benchmarkResponse.FailureCount = 1
		}
	case "show":
		dataResponse := result.(*CommonDataResponse)
		benchmarkResponse.Code = dataResponse.Code
		benchmarkResponse.Duration = dataResponse.Duration
		if dataResponse.Code == 0 {
			benchmarkResponse.SuccessCount = 1
		} else {
			benchmarkResponse.FailureCount = 1
		}
	case "data":
		kvResponse := result.(*KeyValueResponse)
		benchmarkResponse.Code = kvResponse.Code
		benchmarkResponse.Duration = kvResponse.Duration
		if kvResponse.Code == 0 {
			benchmarkResponse.SuccessCount = 1
		} else {
			benchmarkResponse.FailureCount = 1
		}
	default:
		return nil, fmt.Errorf("unsupported command type: %s", result.Type())
	}
	benchmarkResponse.Concurrency = 1
	return &benchmarkResponse, nil
}

// runBenchmarkConcurrent runs benchmark with multiple concurrent workers
func (c *RAGFlowClient) runBenchmarkConcurrent(concurrency, iterations int, nestedCmd *Command) (*BenchmarkResponse, error) {
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
				return nil, err
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

	var benchmarkResponse BenchmarkResponse
	benchmarkResponse.Duration = totalDuration
	benchmarkResponse.Code = 0
	benchmarkResponse.SuccessCount = successCount
	benchmarkResponse.FailureCount = totalCommands - successCount
	benchmarkResponse.Concurrency = concurrency

	return &benchmarkResponse, nil
}

// executeBenchmarkSilent executes a command for benchmark without printing output
func (c *RAGFlowClient) executeBenchmarkSilent(cmd *Command, iterations int) []*Response {
	responseList := make([]*Response, 0, iterations)

	for i := 0; i < iterations; i++ {
		var resp *Response
		var err error

		switch cmd.Type {
		case "ping":
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
	case "ping":
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
