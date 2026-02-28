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

package utility

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	ragflowVersionInfo = "unknown"
	versionOnce        sync.Once
)

// GetRAGFlowVersion gets the RAGFlow version information
// It reads from VERSION file or falls back to git describe command
func GetRAGFlowVersion() string {
	versionOnce.Do(func() {
		ragflowVersionInfo = getRAGFlowVersionInternal()
	})
	return ragflowVersionInfo
}

// getRAGFlowVersionInternal internal function to get version
func getRAGFlowVersionInternal() string {
	// Get the path to VERSION file
	// Assuming this file is in internal/utility, VERSION is in project root
	exePath, err := os.Executable()
	if err != nil {
		return getClosestTagAndCount()
	}

	// Try to find VERSION file in project root
	// Start from executable directory and go up
	dir := filepath.Dir(exePath)
	for i := 0; i < 5; i++ { // Try up to 5 levels up
		versionPath := filepath.Join(dir, "VERSION")
		if data, err := os.ReadFile(versionPath); err == nil {
			return strings.TrimSpace(string(data))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Fallback to git command
	return getClosestTagAndCount()
}

// getClosestTagAndCount gets version info from git describe command
func getClosestTagAndCount() string {
	cmd := exec.Command("git", "describe", "--tags", "--match=v*", "--first-parent", "--always")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}
