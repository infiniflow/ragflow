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

	//// 方法1：使用 Sprint 返回带颜色的字符串
	//coloredPart := red.Sprint("错误")
	//normalPart := "：文件不存在，请检查路径"
	//
	//fmt.Println(coloredPart + normalPart)

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
			_, err = c.execute(test.Command, false)
			if err == nil {
				fmt.Printf("%s %s\n", green.Sprintf("✓ PASSED"), test.Case)
				passed++
			} else {
				fmt.Printf("%s %s\n", red.Sprintf("✗ FAILED"), test.Case)
				println(err.Error())
				failed++
			}
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
