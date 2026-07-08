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
	Server  string `yaml:"server"`
	Command string `yaml:"command"`
}

type TestBase struct {
	AdminIP   string `yaml:"admin_ip"`
	AdminPort int    `yaml:"admin_port"`
	APIIP     string `yaml:"api_ip"`
	APIPort   int    `yaml:"api_port"`
}

type TestSuite struct {
	TestBase
	Suite    string        `yaml:"suite"`
	Base     *string       `yaml:"base"`
	Tests    []TestCase    `yaml:"tests"`
	TearUp   []TestCommand `yaml:"tear_up"`
	TearDown []TestCommand `yaml:"tear_down"`
}

// RunTestSuite runs a test suite file
func (c *CLI) RunTestSuite(testSuite *TestSuite) error {
	fmt.Printf("=== Test Suite: %s ===\n", testSuite.Suite)

	// Add servers
	apiServerAddress := fmt.Sprintf("%s:%d", testSuite.APIIP, testSuite.APIPort)
	adminServerAddress := fmt.Sprintf("%s:%d", testSuite.AdminIP, testSuite.AdminPort)

	// Add API and admin server
	err := c.addServers(apiServerAddress, adminServerAddress)
	if err != nil {
		return err
	}

	// Run tear up commands
	fmt.Printf("=== tear up commands ===\n")
	for _, cmd := range testSuite.TearUp {
		// Switch server
		if cmd.Server == "api" {
			err = c.SwitchAPIServer("api")
			if err != nil {
				return err
			}
		} else if cmd.Server == "admin" {
			err = c.SwitchAdminServer()
			if err != nil {
				return err
			}
		}

		// Run command
		_, err = c.execute(cmd.Command, false)
		if err != nil {
			return fmt.Errorf("failed to run tear up command: %v", err)
		}
	}

	for _, test := range testSuite.Tests {
		fmt.Printf("=== Test: %s ===\n", test.Test)
		_, err = c.execute(test.Command, false)
		if err != nil {
			return err
		}
	}

	// Run test down commands
	fmt.Printf("=== tear down commands ===\n")
	for _, cmd := range testSuite.TearDown {
		_, err = c.execute(cmd.Command, false)
		if err != nil {
			return fmt.Errorf("failed to run tear down command: %v", err)
		}
	}

	return nil
}

func (c *CLI) SwitchAPIServer(serverName string) error {

	isSame, err := c.checkCurrentServer("api", serverName)
	if err != nil {
		return err
	}
	if isSame {
		return nil
	}

	command := fmt.Sprintf("use api '%s'", serverName)
	_, err = c.execute(command, false)
	if err != nil {
		return fmt.Errorf("failed to use API server: %v", err)
	}
	return nil
}

func (c *CLI) SwitchAdminServer() error {

	isSame, err := c.checkCurrentServer("admin", "")
	if err != nil {
		return err
	}
	if isSame {
		return nil
	}

	command := "use admin"
	_, err = c.execute(command, false)
	if err != nil {
		return fmt.Errorf("failed to use ADMIN server: %v", err)
	}
	return nil
}

func (c *CLI) addServers(apiHost, adminHost string) error {

	apiServerName := "api"
	command := fmt.Sprintf("add api '%s' host '%s'", apiServerName, apiHost)
	_, err := c.execute(command, false)
	if err != nil {
		return fmt.Errorf("failed to add API server: %v", err)
	}

	command = fmt.Sprintf("add admin host '%s'", adminHost)
	_, err = c.execute(command, false)
	if err != nil {
		return fmt.Errorf("failed to add ADMIN server: %v", err)
	}

	return nil
}

func (c *CLI) checkCurrentServer(serverMode, serverName string) (bool, error) {
	command := "show current"
	resp, err := c.execute(command, false)
	if err != nil {
		return false, fmt.Errorf("failed to show current server: %v", err)
	}

	commonDataResponse, ok := resp.(*CommonDataResponse)
	if !ok {
		return false, fmt.Errorf("failed to cast response to CommonDataResponse")
	}
	mode, ok := commonDataResponse.Data["mode"].(CommandLineMode)
	if !ok {
		return false, fmt.Errorf("failed to get mode from response")
	}
	if serverMode == "api" && mode == APIMode {
		// Check if current server is already API server
		var apiServer string
		apiServer, ok = commonDataResponse.Data["api_server"].(string)
		if !ok {
			return false, fmt.Errorf("failed to get API server from response")
		}
		if apiServer == serverName {
			// If current API server is serverName, then do nothing
			return true, nil
		}
	}

	if serverMode == "admin" && mode == AdminMode {
		// Check if current server is already ADMIN server
		return true, nil
	}
	return false, nil
}
