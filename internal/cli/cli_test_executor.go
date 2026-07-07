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
)

type TestCase struct {
	Test    string                 `yaml:"test"`
	Command string                 `yaml:"command"`
	Expect  map[string]interface{} `yaml:"expect"`
}

type TestCommand struct {
	Command string `yaml:"command"`
}

type TestSuite struct {
	Suite    string        `yaml:"suite"`
	Tests    []TestCase    `yaml:"tests"`
	TearUp   []TestCommand `yaml:"tear_up"`
	TearDown []TestCommand `yaml:"tear_down"`
}

// RunTestSuite runs a test suite file
func (c *CLI) RunTestSuite(testSuite *TestSuite) error {
	fmt.Printf("=== Test Suite: %s ===\n", testSuite.Suite)

	// Run tear up commands
	fmt.Printf("=== tear up commands ===\n")
	for _, cmd := range testSuite.TearUp {
		_, err := c.execute(cmd.Command, false)
		if err != nil {
			return fmt.Errorf("failed to run tear up command: %v", err)
		}
	}

	// Run test down commands
	fmt.Printf("=== tear down commands ===\n")
	for _, cmd := range testSuite.TearDown {
		_, err := c.execute(cmd.Command, false)
		if err != nil {
			return fmt.Errorf("failed to run tear down command: %v", err)
		}
	}

	return nil
}
