//
//  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

package utility

import (
	"fmt"
	"testing"
)

func TestGetRAGFlowVersion(t *testing.T) {
	version := GetRAGFlowVersion()
	fmt.Printf("RAGFlow Version: %s\n", version)
	if version == "" {
		t.Error("GetRAGFlowVersion returned empty string")
	}
	if version == "unknown" {
		t.Log("Warning: GetRAGFlowVersion returned 'unknown', VERSION file not found and git command failed")
	}
}

func TestGetClosestTagAndCount(t *testing.T) {
	version := getClosestTagAndCount()
	fmt.Printf("Git Version: %s\n", version)
	// This test just prints the version, no strict assertion
}
