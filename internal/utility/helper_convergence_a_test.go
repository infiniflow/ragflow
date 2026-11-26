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

package utility

import (
	"os"
	"strings"
	"testing"
)

// Paths are relative to this test file's directory (internal/utility).
var helperConvergenceAFiles = []string{
	"../service/metadata_filter.go",
	"../agent/component/switch.go",
	"../agent/canvas/scheduler.go",
	"../harness/core/middlewares/dynamictool/toolsearch/toolsearch.go",
	"../dao/model_migration.go",
	"../service/chat_session.go",
	"../service/agent.go",
	"../service/ingestion_task_service.go",
	"../agent/component/docs_generator.go",
	"../agent/sandbox/probe.go",
	"../agent/tool/searxng.go",
	"../agentic_rag/decision_judge.go",
}

// legacyContainsFoldPatterns are the exact inline forms converged to
// common.ContainsFold. Needle-already-normalized callers elsewhere are
// deliberately out of scope.
var legacyContainsFoldPatterns = map[string]string{
	"../service/metadata_filter.go":                                    "strings.Contains(strings.ToLower(val), strings.ToLower(strValue))",
	"../agent/component/switch.go":                                     "strings.Contains(strings.ToLower(ls), strings.ToLower(rs))",
	"../agent/canvas/scheduler.go":                                     "strings.Contains(strings.ToLower(text), strings.ToLower(cpnID)+\"@\")",
	"../harness/core/middlewares/dynamictool/toolsearch/toolsearch.go": "strings.Contains(strings.ToLower(part), kw)",
	"../dao/model_migration.go":                                        "strings.Contains(strings.ToLower(columnType.DatabaseTypeName()), \"int\")",
}

// legacyTruncateRunesPatterns are the exact inline rune-slicing forms
// converged to utility.TruncateRunes. Variants with separate ellipsis or
// byte-limit logic are intentionally out of scope.
var legacyTruncateRunesPatterns = map[string]string{
	"../service/chat_session.go":           "string(nameRunes[:255])",
	"../service/agent.go":                  "return string(runes)",
	"../service/ingestion_task_service.go": "string([]rune(marker)[:limits.MaxMessageChars])",
	"../agent/component/docs_generator.go": "string(runes[:maxBase])",
	"../agent/sandbox/probe.go":            "string(runes[:min(200, len(runes))])",
	"../agent/tool/searxng.go":             "string(runes[:10000])",
	"../agentic_rag/decision_judge.go":     "string([]rune(text)[:b.maxChars])",
}

func readHelperConvergenceAFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestContainsFoldConverged(t *testing.T) {
	leftovers := make([]string, 0)
	for path, pattern := range legacyContainsFoldPatterns {
		if strings.Contains(readHelperConvergenceAFile(t, path), pattern) {
			leftovers = append(leftovers, path+": "+pattern)
		}
	}
	if len(leftovers) > 0 {
		t.Errorf("inline case-folded containment still present:\n  %s", strings.Join(leftovers, "\n  "))
	}
}

func TestTruncateRunesConverged(t *testing.T) {
	leftovers := make([]string, 0)
	for path, pattern := range legacyTruncateRunesPatterns {
		if strings.Contains(readHelperConvergenceAFile(t, path), pattern) {
			leftovers = append(leftovers, path+": "+pattern)
		}
	}
	if len(leftovers) > 0 {
		t.Errorf("inline rune truncation still present:\n  %s", strings.Join(leftovers, "\n  "))
	}
}

func TestCanonicalCallsPresent(t *testing.T) {
	containsFoldCalls, truncateCalls := 0, 0
	for _, path := range helperConvergenceAFiles {
		src := readHelperConvergenceAFile(t, path)
		containsFoldCalls += strings.Count(src, "common.ContainsFold(")
		truncateCalls += strings.Count(src, "utility.TruncateRunes(")
	}
	if containsFoldCalls < 7 {
		t.Errorf("expected at least 7 common.ContainsFold call sites, found %d", containsFoldCalls)
	}
	if truncateCalls < 9 {
		t.Errorf("expected at least 9 utility.TruncateRunes call sites, found %d", truncateCalls)
	}
}
