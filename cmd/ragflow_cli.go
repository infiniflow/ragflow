//go:build ignore
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

package main

import (
	"fmt"
	"os"
	"os/signal"
	"ragflow/internal/common"
	"syscall"

	"ragflow/internal/cli"
)

func main() {

	parseArgs, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		return
	}

	if parseArgs.ShowHelp {
		cli.PrintUsage()
		return
	}

	//parseArgs.Print()
	logLevel := "warn" // Default to warn (quiet mode)
	if parseArgs.Verbose {
		logLevel = "info"
	}

	if err = common.Init(logLevel, ""); err != nil {
		fmt.Printf("Warning: Failed to initialize logger: %v\n", err)
	}

	client, err := cli.NewCLIWithConfig(parseArgs)
	if err != nil {
		fmt.Printf("Failed to create CLI: %v\n", err)
		os.Exit(1)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		client.Cleanup()
		os.Exit(0)
	}()

	if parseArgs.Command != nil {
		if err = client.RunSingleCommand(parseArgs.Command); err != nil {
			fmt.Printf("Command execution failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		if err = client.Run(); err != nil {
			fmt.Printf("CLI error: %v\n", err)
			os.Exit(1)
		}
	}

	return
}
