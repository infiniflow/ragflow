//go:build integration

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

package api

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	logf := func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }

	// An externally supplied API key means the caller owns the tenant; only a
	// self-registered session is logged out again below.
	loggedIn := false
	if TestConfig.APIKey == "" {
		fmt.Fprintln(os.Stderr, "Login")
		if err := TestConfig.Login(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to login: %v\n", err)
			os.Exit(1)
		}
		loggedIn = true
	}

	// Register the providers, instances, and tenant defaults the suite needs.
	// This used to be the job of the pytest session fixture, which is why the
	// Go suite had to run after the Python tests in CI.
	prepareDefaultModels(logf)

	// Run the test cases
	code := m.Run()

	if !loggedIn {
		os.Exit(code)
	}

	if err := TestConfig.Logout(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to logout: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "Logout")
	os.Exit(code)
}
