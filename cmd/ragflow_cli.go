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
	// Parse command line arguments (skip program name)
	args, err := cli.ParseConnectionArgs(os.Args[1:])
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger with appropriate level
	logLevel := "warn" // Default to warn (quiet mode)
	if args.Verbose {
		logLevel = "info"
	}
	if err = common.Init(logLevel); err != nil {
		fmt.Printf("Warning: Failed to initialize logger: %v\n", err)
	}

	// Show help and exit
	if args.ShowHelp {
		cli.PrintUsage()
		os.Exit(0)
	}

	// Create CLI instance with parsed arguments
	cliApp, err := cli.NewCLIWithArgs(args)
	if err != nil {
		fmt.Printf("Failed to create CLI: %v\n", err)
		os.Exit(1)
	}

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cliApp.Cleanup()
		os.Exit(0)
	}()

	// Check if we have a single command to execute
	if args.Command != nil {
		// Single command mode
		if err = cliApp.RunSingleCommand(args.Command); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Interactive mode
		if err = cliApp.Run(); err != nil {
			fmt.Printf("CLI error: %v\n", err)
			os.Exit(1)
		}
	}
}
