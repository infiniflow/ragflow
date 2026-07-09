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

	"github.com/fatih/color"
)

type TestCase struct {
	Server  *string                `yaml:"server"`
	Case    string                 `yaml:"case"`
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
	Suite string     `yaml:"suite"`
	Base  *string    `yaml:"base"`
	Tests []TestCase `yaml:"tests"`
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

func (c *CLI) verifySimpleResponse(simpleResponse *SimpleResponse, expect map[string]interface{}) error {
	if simpleResponse == nil {
		return fmt.Errorf("response is nil")
	}

	status, ok := expect["status"].(string)
	if !ok {
		return fmt.Errorf("no status in result")
	}

	if simpleResponse.Code == 0 && strings.ToLower(status) == "success" {
		return nil
	}

	if strings.ToLower(status) == strings.ToLower(simpleResponse.Message) {
		return nil
	}

	return fmt.Errorf("status: %s, expected: %s", simpleResponse.Message, status)
}

func (c *CLI) verifyCommonDataResponse(commonDataResponse *CommonDataResponse, expect map[string]interface{}) error {
	if commonDataResponse == nil {
		return fmt.Errorf("response is nil")
	}

	status, ok := expect["status"].(string)
	if !ok {
		return fmt.Errorf("no status in result")
	}

	if commonDataResponse.Code == 0 && strings.ToLower(status) == "success" {
		return nil
	}

	if strings.ToLower(status) == strings.ToLower(commonDataResponse.Message) {
		return nil
	}

	return fmt.Errorf("status: %s, expected: %s", commonDataResponse.Message, status)
}

func (c *CLI) verifyCommonResponse(commonResponse *CommonResponse, expect map[string]interface{}) error {
	if commonResponse == nil {
		return fmt.Errorf("response is nil")
	}

	status, ok := expect["status"].(string)
	if !ok {
		return fmt.Errorf("no status in result")
	}

	if commonResponse.Code == 0 && strings.ToLower(status) == "success" {
		return nil
	}

	if strings.ToLower(status) == strings.ToLower(commonResponse.Message) {
		return nil
	}

	return fmt.Errorf("status: %s, expected: %s", commonResponse.Message, status)
}

func (c *CLI) verifyResult(resp ResponseIf, expect map[string]interface{}) error {
	if resp == nil {
		return fmt.Errorf("response is nil")
	}

	switch v := resp.(type) {
	case *SimpleResponse:
		return c.verifySimpleResponse(v, expect)
	case *CommonDataResponse:

		return c.verifyCommonDataResponse(v, expect)
	case *CommonResponse:
		return c.verifyCommonResponse(v, expect)
	default:
		return fmt.Errorf("response type is not supported")
	}
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

	color.NoColor = false
	red := color.New(color.FgRed)
	green := color.New(color.FgGreen)

	var passed int
	var failed int

	for _, test := range testSuite.Tests {
		if test.Server != nil {
			serverName := *test.Server
			if serverName == "api" {
				err = c.SwitchAPIServer("api")
				if err != nil {
					return err
				}
			} else if serverName == "admin" {
				err = c.SwitchAdminServer()
				if err != nil {
					return err
				}
			}
			color.Green("SWITCH %s", serverName)
		} else {
			var result ResponseIf
			result, err = c.execute(test.Command, false)
			if err == nil {
				err = c.verifyResult(result, test.Expect)
				if err == nil {
					fmt.Printf("%s %s\n", green.Sprintf("✓ PASSED"), test.Case)
					passed++
					continue
				}
			}

			fmt.Printf("%s %s\n", red.Sprintf("✗ FAILED"), test.Case)
			println(err.Error())
			failed++
		}
	}

	fmt.Printf("Test cases %d, passed: %d, failed: %d\n", passed+failed, passed, failed)

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
