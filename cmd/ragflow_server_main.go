//go:build !desktop

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
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"ragflow/internal/common"
)

// main is the server build's entry point: it connects to the external services
// the operator configured. The desktop build has its own main in
// ragflow_desktop.go, behind the desktop tag.
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer cancel()

	arguments, err := parseArgsFrom(os.Args)
	if err != nil {
		fmt.Printf("Failed to parse arguments: %v\n", err)
		os.Exit(1)
	}

	if *arguments.HelpFlag {
		printHelp(arguments)
		return
	}

	if *arguments.VersionFlag {
		fmt.Printf("RAGFlow version: %s\n", common.GetRAGFlowVersion())
		return
	}

	if err = runServer(ctx, cancel, arguments); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start RAGFlow: %v\n", err)
		os.Exit(1)
	}
}
