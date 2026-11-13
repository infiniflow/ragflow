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
func (c *CLI) RunBenchmark(commandCount int, cmd *Command) (ResponseIf, error) {
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
		response, err := c.runBenchmarkSingle(iterations, nestedCmd)
		if err != nil {
			return nil, err
		}
		return response, nil
	}
	response, err := c.runBenchmarkConcurrent(concurrency, iterations, nestedCmd)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// runBenchmarkSingle runs benchmark with single concurrency (sequential execution)
func (c *CLI) runBenchmarkSingle(iterations int, nestedCmd *Command) (*BenchmarkResponse, error) {
	commandType := nestedCmd.Type

	// For search_on_datasets, convert dataset names to IDs first
	if commandType == "search_on_datasets" && iterations > 1 {
		datasets, _ := nestedCmd.Params["datasets"].(string)
		datasetNames := strings.Split(datasets, ",")
		datasetIDs := make([]string, 0, len(datasetNames))
		for _, name := range datasetNames {
			name = strings.TrimSpace(name)
			id, err := c.getDatasetIDByName(name)
			if err != nil {
				return nil, err
			}
			datasetIDs = append(datasetIDs, id)
		}
		nestedCmd.Params["dataset_ids"] = datasetIDs
	}

	// Check if command supports native benchmark (iterations > 1)
	result, err := c.ExecuteCommand(iterations, nestedCmd)
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
func (c *CLI) runBenchmarkConcurrent(concurrency, iterations int, nestedCmd *Command) (*BenchmarkResponse, error) {
	type workerResult struct {
		response *BenchmarkResponse
		err      error
	}
	results := make([]workerResult, concurrency)
	var wg sync.WaitGroup

	// For search_on_datasets, convert dataset names to IDs first
	if nestedCmd.Type == "search_on_datasets" {
		datasets, _ := nestedCmd.Params["datasets"].(string)
		datasetNames := strings.Split(datasets, ",")
		datasetIDs := make([]string, 0, len(datasetNames))
		for _, name := range datasetNames {
			name = strings.TrimSpace(name)
			id, err := c.getDatasetIDByName(name)
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

			workerCmd := *nestedCmd
			workerCmd.Params = make(map[string]interface{}, len(nestedCmd.Params))
			for key, value := range nestedCmd.Params {
				workerCmd.Params[key] = value
			}
			results[idx].response, results[idx].err = c.runBenchmarkSingle(iterations, &workerCmd)
		}(i)
	}

	wg.Wait()
	endTime := time.Now()

	totalDuration := endTime.Sub(startTime).Seconds()
	successCount := 0
	failureCount := 0

	for i, result := range results {
		if result.err != nil {
			return nil, fmt.Errorf("benchmark worker %d: %w", i+1, result.err)
		}
		if result.response == nil {
			return nil, fmt.Errorf("benchmark worker %d returned no response", i+1)
		}
		successCount += result.response.SuccessCount
		failureCount += result.response.FailureCount
	}

	//totalCommands := iterations * concurrency

	var benchmarkResponse BenchmarkResponse
	benchmarkResponse.Duration = totalDuration
	benchmarkResponse.Code = 0
	benchmarkResponse.SuccessCount = successCount
	benchmarkResponse.FailureCount = failureCount
	benchmarkResponse.Concurrency = concurrency

	return &benchmarkResponse, nil
}
