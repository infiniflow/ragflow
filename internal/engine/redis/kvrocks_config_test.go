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

// Package redis holds deployment-config regression guards for the
// Valkey->Kvrocks migration. These are plain unit tests (no external service):
// they assert that the deployment artifacts (CI workflow, compose file) encode
// the required fixes so a future edit cannot silently regress the migration.
package redis

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from this source file to the module root (go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root (go.mod) not found")
		}
		dir = parent
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestKvrocksDeploymentConfig guards the deployment artifacts that the parity
// integration tests cannot reach on their own: the CI workflow must forward the
// password into the container, and the compose healthcheck must authenticate
// now that requirepass is enforced.
func TestKvrocksDeploymentConfig(t *testing.T) {
	root := repoRoot(t)

	t.Run("WorkflowForwardsRedisPassword", func(t *testing.T) {
		wf := readFile(t, filepath.Join(root, ".github/workflows/kvrocks-parity.yml"))
		if !strings.Contains(wf, "-e REDIS_PASSWORD") {
			t.Fatal("kvrocks-parity.yml: the docker run must forward REDIS_PASSWORD into the container (-e REDIS_PASSWORD); without it Kvrocks starts unauthenticated and the parity tests fail to AUTH")
		}
	})

	t.Run("ComposeHealthcheckAuthenticates", func(t *testing.T) {
		compose := readFile(t, filepath.Join(root, "docker/docker-compose-base.yml"))
		// requirepass is enforced, so the healthcheck must authenticate. The
		// Kvrocks image ships redis-cli (but not nc), so the probe is
		// `redis-cli -a ${REDIS_PASSWORD} ping`. A bare PING would be rejected
		// and mark the service permanently unhealthy.
		if !strings.Contains(compose, "redis-cli") || !strings.Contains(compose, "${REDIS_PASSWORD}") {
			t.Fatal("docker-compose redis healthcheck must use `redis-cli -a ${REDIS_PASSWORD} ping` (the Kvrocks image has no nc); a bare PING fails now that requirepass is enforced")
		}
	})
}
