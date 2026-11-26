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

package storage

import (
	"os/exec"
	"strings"
	"testing"
)

// Check the compiler's package closure, rather than the module requirements:
// the server still needs the cloud modules in go.mod.
func TestLocalStorageHasNoCloudSDKDependencies(t *testing.T) {
	command := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect local storage dependencies: %v\n%s", err, output)
	}
	for _, dependency := range strings.Fields(string(output)) {
		for _, sdk := range []string{
			"cloud.google.com/go/storage",
			"google.golang.org/api",
			"github.com/aws/aws-sdk-go-v2",
			"github.com/minio/minio-go/v7",
		} {
			if dependency == sdk || strings.HasPrefix(dependency, sdk+"/") {
				t.Errorf("local storage imports optional cloud SDK %s", dependency)
			}
		}
	}
}
