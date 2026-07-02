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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func FindConfFileInProject(fileName string) (*string, error) {

	var filePath string
	if projDir := os.Getenv("RAG_PROJECT_BASE"); projDir != "" {
		filePath = filepath.Join(projDir, "conf", fileName)
		if _, err := os.Stat(filePath); err == nil {
			return &filePath, nil
		}
	}

	if projDir := os.Getenv("RAG_DEPLOY_BASE"); projDir != "" {
		filePath = filepath.Join(projDir, "conf", fileName)
		if _, err := os.Stat(filePath); err == nil {
			return &filePath, nil
		}
	}

	exeFilePath, err := os.Executable()
	if err == nil {
		projDir := filepath.Dir(filepath.Dir(exeFilePath))
		filePath = filepath.Join(projDir, "conf", fileName)
		if _, err = os.Stat(filePath); err == nil {
			return &filePath, nil
		}
	}

	_, curFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(curFile)
	for {
		if _, err = os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			filePath = filepath.Join(dir, "conf", fileName)
			if _, err = os.Stat(filePath); err == nil {
				return &filePath, nil
			}
			return nil, fmt.Errorf("conf file %s not found in %s", fileName, dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			projDir := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(curFile))))
			filePath = filepath.Join(projDir, "conf", fileName)
			if _, err = os.Stat(filePath); err == nil {
				return &filePath, nil
			}

			return nil, fmt.Errorf("conf file %s not found in %s", fileName, projDir)
		}
		dir = parent
	}
}
