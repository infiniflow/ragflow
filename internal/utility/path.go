/*
Copyright 2026 The InfiniFlow Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utility

import (
	"os"
	"path/filepath"
)

// GetProjectRoot returns the project root directory
func GetProjectRoot() string {
	// Try environment variable first
	if d := os.Getenv("RAG_PROJECT_BASE"); d != "" {
		return d
	}
	if d := os.Getenv("RAG_DEPLOY_BASE"); d != "" {
		return d
	}

	// The binary is always at <project_root>/bin/, so going up 2 levels from
	// the executable path gives the project root.
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(filepath.Dir(exe))
}
