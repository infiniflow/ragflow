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
	"path/filepath"

	"ragflow/internal/desktop"
	"ragflow/internal/server/bootstrap"
)

// defaultDesktopDataDir is created under the user's home when neither
// --data-dir nor RAGFLOW_DESKTOP_DATA_DIR is set.
const defaultDesktopDataDir = ".ragflow/desktop"

// startDesktopBackends boots the in-process document engine, cache, message
// queue and object store under one data directory, repoints the parsed
// configuration at them and returns the function that stops them.
//
// The desktop build has no separate entry point: ragflow_server.go's main runs
// it here, before the configuration is read, so the tag decides which backends
// exist rather than how the server starts.
func startDesktopBackends(ctx context.Context, arguments *bootstrap.ServerArgs) (func(), error) {
	dataDir, err := desktopDataDir(arguments.DataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve the data directory: %w", err)
	}

	runtime, err := desktop.Start(ctx, dataDir)
	if err != nil {
		return nil, fmt.Errorf("start the backends in %s: %w", dataDir, err)
	}

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

	return func() {
		if closeErr := runtime.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to stop the desktop backends: %v\n", closeErr)
		}
	}, nil
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
