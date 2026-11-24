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

package common

import (
	"os"
	"path/filepath"
)

// SystemResourceDir is where the Infinity resource bundle is installed on a
// host that unpacks it system-wide (the Docker images and the .deb do).
const SystemResourceDir = "/usr/share/infinity/resource"

// resourceMarker is the file every complete resource tree contains. A candidate
// directory that lacks it is either empty or partially copied, and must not
// shadow one that is complete.
var resourceMarker = filepath.Join("rag", "huqie.trie")

// ResourceDir returns the directory holding the analyzer dictionaries, WordNet
// and the spaCy models — the `resource/` tree the desktop app ships.
//
// One resolver for both entry points: the packaged app finds the copy inside its
// bundle, and a CLI run from bin/ finds the tree beside the binary, without
// either needing RAGFLOW_DICT_PATH set. The order is
//
//  1. RAGFLOW_DICT_PATH — an explicit choice, returned as is (even if it is
//     wrong: the caller's error message names the path it was given);
//  2. resource/ next to the executable, ../resource, and ../Resources/resource
//     (the macOS bundle layout, where the binary is Contents/MacOS/<name>);
//  3. ~/.ragflow/resource, for a per-user install;
//  4. SystemResourceDir.
func ResourceDir() string {
	if dir := GetEnv(EnvRAGFlowDictPath); dir != "" {
		return dir
	}
	for _, dir := range resourceCandidates() {
		if hasResourceMarker(dir) {
			return dir
		}
	}
	return SystemResourceDir
}

func resourceCandidates() []string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		dirs = append(dirs,
			filepath.Join(exeDir, "resource"),
			filepath.Join(exeDir, "..", "resource"),
			filepath.Join(exeDir, "..", "Resources", "resource"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".ragflow", "resource"))
	}
	return dirs
}

func hasResourceMarker(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, resourceMarker))
	return err == nil && !info.IsDir()
}

// confFilePath resolves a file inside the configuration directory. It reads the
// same RAGFLOW_CONF_DIR override as internal/utility.ConfDir — either the conf
// directory itself or the project root containing it — but lives here because
// utility depends on this package, not the other way round. The desktop shell
// always exports the variable, so a packaged app finds its copy; a source or
// deploy tree keeps the working-directory-relative default.
func confFilePath(name string) string {
	dir := GetEnv(EnvRAGFlowConfDir)
	if dir == "" {
		return filepath.Join("conf", name)
	}
	for _, candidate := range []string{filepath.Join(dir, name), filepath.Join(dir, "conf", name)} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join(dir, name)
}
