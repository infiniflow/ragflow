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
	"ragflow/internal/common"
	"runtime"
)

// GetProjectRoot returns the project root directory
func GetProjectRoot() string {
	// Try environment variable first
	if confDir := common.GetEnv(common.EnvRAGFlowConfDir); confDir != "" {
		return confDir
	}
	if d := common.GetEnv(common.EnvRAGProjectBase); d != "" {
		return d
	}
	if d := common.GetEnv(common.EnvRAGDeployBase); d != "" {
		return d
	}

	// Find project root by looking for go.mod from this source file.
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

// ConfDir returns the directory holding the bundled configuration: models/,
// all_models.json, system_settings.json, llm_factories.json.
//
// Every candidate but the last-resort one is absolute, because a packaged app
// does not run from the source tree and has no useful working directory — a
// window launched from Finder starts in "/". The app bundle keeps the directory
// at Contents/Resources/conf, two levels above Contents/Resources/binaries/<engine>;
// a plain install keeps it beside the binary; the source tree is still found
// through the executable's ancestors so `bin/` and `go test` behave as before.
//
// RAGFLOW_CONF_DIR is accepted either as the conf directory itself or as the
// project root containing it, matching how GetProjectRoot reads it.
func ConfDir() string {
	if dir := common.GetEnv(common.EnvRAGFlowConfDir); dir != "" {
		if hasModelsDir(dir) {
			return dir
		}
		if joined := filepath.Join(dir, "conf"); hasModelsDir(joined) {
			return joined
		}
	}
	for _, base := range []string{
		common.GetEnv(common.EnvRAGProjectBase),
		common.GetEnv(common.EnvRAGDeployBase),
	} {
		if base == "" {
			continue
		}
		if joined := filepath.Join(base, "conf"); hasModelsDir(joined) {
			return joined
		}
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		for _, candidate := range []string{
			filepath.Join(exeDir, "conf"),
			filepath.Join(exeDir, "..", "conf"),
			filepath.Join(exeDir, "..", "..", "conf"),
		} {
			if hasModelsDir(candidate) {
				return candidate
			}
		}
	}
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for depth := 0; depth < 4; depth++ {
			if joined := filepath.Join(dir, "conf"); hasModelsDir(joined) {
				return joined
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "conf"
}

// hasModelsDir marks a complete conf directory. models/ is what the engine reads
// on every start, so its presence separates a real bundle from a stray path.
func hasModelsDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "models"))
	return err == nil && info.IsDir()
}

func FindConfFileInProject(fileName string) (*string, error) {

	var filePath string
	if projDir := common.GetEnv(common.EnvRAGProjectBase); projDir != "" {
		filePath = filepath.Join(projDir, "conf", fileName)
		if _, err := os.Stat(filePath); err == nil {
			return &filePath, nil
		}
	}

	if projDir := common.GetEnv(common.EnvRAGDeployBase); projDir != "" {
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
