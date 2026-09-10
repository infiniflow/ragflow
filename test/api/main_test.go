////go:build integration

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
	fmt.Fprintln(os.Stderr, "Login")

	err := TestConfig.Login()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to login: %v\n", err)
		os.Exit(1)
	}

	// Run the test cases
	code := m.Run()

	err = TestConfig.Logout()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to logout: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "Logout")
	os.Exit(code)
}
