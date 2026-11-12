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

package agentic_rag

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
)

func TestDurationFromEnv(t *testing.T) {
	t.Setenv("AGENTIC_TEST_TIMEOUT", "5m")
	if got := durationFromEnv("AGENTIC_TEST_TIMEOUT", 90*time.Second); got != 5*time.Minute {
		t.Fatalf("override = %s, want 5m", got)
	}

	for name, raw := range map[string]string{"unset": "", "malformed": "soon", "zero": "0s", "negative": "-1m"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("AGENTIC_TEST_TIMEOUT", raw)
			if got := durationFromEnv("AGENTIC_TEST_TIMEOUT", 90*time.Second); got != 90*time.Second {
				t.Fatalf("override %q = %s, want the 90s default", raw, got)
			}
		})
	}

	if finalizeTimeout <= 0 || claimAnswerTimeout <= 0 {
		t.Fatalf("package budgets must stay positive: finalize=%s claim=%s", finalizeTimeout, claimAnswerTimeout)
	}
}

// TestDurationFromEnvWarnsOnStderrBeforeLoggerInit verifies the malformed
// override warning reaches stderr while the logger is still nil: the two
// package budgets are parsed during package initialization, before the server
// installs its logger, and WarnCtx returns without logging on a nil logger.
func TestDurationFromEnvWarnsOnStderrBeforeLoggerInit(t *testing.T) {
	t.Setenv("AGENTIC_TEST_TIMEOUT", "soon")
	saved := common.Logger
	common.Logger = nil
	t.Cleanup(func() { common.Logger = saved })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	savedStderr := os.Stderr
	os.Stderr = w
	got := durationFromEnv("AGENTIC_TEST_TIMEOUT", 90*time.Second)
	w.Close()
	os.Stderr = savedStderr

	if got != 90*time.Second {
		t.Fatalf("override = %s, want the 90s default", got)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	for _, want := range []string{"AGENTIC_TEST_TIMEOUT", `"soon"`, "1m30s"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("stderr warning missing %s: %q", want, string(out))
		}
	}
}
