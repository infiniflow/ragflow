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

import "testing"

func TestRunBenchmarkConcurrentPropagatesWorkerError(t *testing.T) {
	cli := &CLI{Config: &CommandLineConfig{CLIMode: APIMode}}
	command := &Command{Type: "benchmark", Params: map[string]interface{}{
		"concurrency": 2,
		"iterations":  1,
		"command": &Command{
			Type:   "not_a_command",
			Params: map[string]interface{}{},
		},
	}}

	response, err := cli.RunBenchmark(1, command)
	if err == nil {
		t.Fatal("RunBenchmark() error = nil, want worker command error")
	}
	if response != nil {
		t.Fatalf("RunBenchmark() response = %#v, want nil on worker error", response)
	}
}

func TestRunBenchmarkRejectsNonpositiveIterations(t *testing.T) {
	client := &CLI{Config: &CommandLineConfig{CLIMode: APIMode}}
	for _, iterations := range []int{0, -1} {
		command := &Command{Type: "benchmark", Params: map[string]interface{}{
			"iterations": iterations,
			"command":    &Command{Type: "not_a_command", Params: map[string]interface{}{}},
		}}

		response, err := client.RunBenchmark(1, command)
		if response != nil || err == nil || err.Error() != "iterations must be greater than 0" {
			t.Errorf("iterations %d: got response %v, error %v; want iteration validation error", iterations, response, err)
		}
	}
}
