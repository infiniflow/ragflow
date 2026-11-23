//go:build desktop

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
	"path/filepath"
	"syscall"

	"ragflow/internal/common"
	"ragflow/internal/desktop"
	"ragflow/internal/server/bootstrap"
)

// defaultDesktopDataDir is created under the user's home when neither
// --data-dir nor RAGFLOW_DESKTOP_DATA_DIR is set.
const defaultDesktopDataDir = ".ragflow/desktop"

// main is the desktop build's entry point. It boots the in-process backends
// (pebbis, JetStream, the S3 gateway) under one data directory, repoints the
// parsed configuration at them, and then runs the same server the server build
// runs. The server build's main lives in ragflow_server.go behind !desktop.
func main() {
	os.Exit(runDesktop())
}

func runDesktop() int {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer cancel()

	arguments, err := parseArgsFrom(os.Args)
	if err != nil {
		fmt.Printf("Failed to parse arguments: %v\n", err)
		return 1
	}

	if *arguments.HelpFlag {
		printHelp(arguments)
		return 0
	}

	if *arguments.VersionFlag {
		fmt.Printf("RAGFlow version: %s\n", common.GetRAGFlowVersion())
		return 0
	}

	dataDir, err := desktopDataDir(arguments.DataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to resolve the desktop data directory: %v\n", err)
		return 1
	}

	runtime, err := desktop.Start(ctx, dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start the desktop backends: %v\n", err)
		return 1
	}
	defer func() {
		if closeErr := runtime.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to stop the desktop backends: %v\n", closeErr)
		}
	}()

	bootstrap.SetDesktopEndpoints(bootstrap.DesktopEndpoints{
		KvrocksAddr:     runtime.KvrocksAddr,
		KvrocksPassword: runtime.KvrocksPassword,
		NATSHost:        runtime.NATSHost,
		NATSPort:        runtime.NATSPort,
		S3Endpoint:      runtime.S3Endpoint,
		S3AccessKey:     runtime.S3AccessKey,
		S3SecretKey:     runtime.S3SecretKey,
		InfinityAddr:    runtime.InfinityAddr,
		DataDir:         runtime.DataDir,
	})

	// A desktop installation keeps its configuration next to its data unless
	// -f points elsewhere.
	if arguments.ConfigPath == nil {
		cfg := filepath.Join(dataDir, "service_conf.yaml")
		if _, statErr := os.Stat(cfg); statErr == nil {
			arguments.ConfigPath = &cfg
		}
	}

	if err = runServer(ctx, cancel, arguments); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start RAGFlow: %v\n", err)
		return 1
	}
	return 0
}

// desktopDataDir resolves where the in-process backends keep their state:
// --data-dir first, then RAGFLOW_DESKTOP_DATA_DIR, then ~/.ragflow/desktop.
func desktopDataDir(flagValue *string) (string, error) {
	if flagValue != nil && *flagValue != "" {
		return *flagValue, nil
	}
	if env := os.Getenv("RAGFLOW_DESKTOP_DATA_DIR"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultDesktopDataDir), nil
}
