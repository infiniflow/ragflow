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

//go:build !ee

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"ragflow/internal/server/bootstrap"
)

func registerFlags(_ *flag.FlagSet, _ *bootstrap.ServerArgs) func() error {
	return func() error { return nil }
}

func printHelp(_ *bootstrap.ServerArgs) {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "RAGFlow Server - runs admin, API, ingestor, syncer and chatter in one process.")
	fmt.Fprintln(os.Stderr, "Database migration and superuser initialization run automatically.")
	fmt.Fprintln(os.Stderr, "Default ports: admin 9381, API 9380.")
	fmt.Fprintln(os.Stderr)
	printCommonHelp()
}

func validateServerMode(_ *bootstrap.ServerArgs) error { return nil }

func runEEServices(ctx context.Context, cancel context.CancelFunc, args *bootstrap.ServerArgs, services *bootstrap.AgentServices) error {
	return runServices(ctx, cancel, args, services)
}

// runDeepDoc reports that the EE DeepDoc inference server is an enterprise
// capability.
func runDeepDoc(ctx context.Context, serverName string, args *bootstrap.ServerArgs) error {
	return errors.New("the DeepDoc inference server is only available in the enterprise edition")
}
