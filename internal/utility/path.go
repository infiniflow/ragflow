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
	"runtime"
)

// GetProjectRoot returns the project root directory
func GetProjectRoot() string {
	// Try environment variable first
	if confDir := os.Getenv("RAGFLOW_CONF_DIR"); confDir != "" {
		return confDir
	}
	if d := os.Getenv("RAG_PROJECT_BASE"); d != "" {
		return d
	}
	if d := os.Getenv("RAG_DEPLOY_BASE"); d != "" {
		return d
	}

	// Find project root by looking for go.mod from this source file. This is
	// stable when IDEs run a temporary binary outside the repository.
	_, curFile, _, ok := runtime.Caller(0)
	if ok {
		dir := filepath.Dir(curFile)
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// Deployment binaries are normally at <project_root>/bin/.
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(filepath.Dir(exe))
}
