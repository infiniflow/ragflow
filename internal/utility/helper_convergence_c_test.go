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
	"strings"
	"testing"
)

// C-family convergence: exact inline forms replaced by canonical helpers.
// Variants with different semantics (e.g. compactStrings that trims but does
// not dedupe) are deliberately out of scope.
var helperConvergenceCPatterns = map[string]string{
	"../service/dataset_structure_graph.go":                        "out := make(map[string]interface{}, len(in))\n\tfor k, v := range in {",
	"../service/memory_message_service.go":                         "out := make(map[string]interface{}, len(in))\n\tfor k, v := range in {",
	"../agent/workflowx/loop.go":                                   "buf := make([]byte, len(v))\n\t\tcopy(buf, v)",
	"../dao/model_migration.go":                                    "sort.Strings(names)\n\treturn names",
	"../agent/runtime/template.go":                                 "seen := make(map[string]struct{}, len(matches))",
	"../tokenizer/embedlimit.go":                                   "func containsAnyMarker(msg string, markers []string) bool {\n\tfor _, marker := range markers {",
	"../agent/retrievalbridge/memory.go":                           "seen := make(map[string]struct{}, len(values))",
	"../ingestion/component/knowledge_compiler/structure/merge.go": "seen := map[string]bool{}\n\tvar out []string",
	"../ingestion/component/knowledge_compiler/mindmap/mindmap.go": "seen := make(map[string]struct{}, len(left)+len(right))",
	"../agent/component/llm.go":                                    "seen := make(map[string]struct{})\n\tvar out []string",
	"../agent/component/string_transform.go":                       "seen := make(map[string]struct{}, len(matches))",
	"../harness/graph/store/memory.go":                             "for k, v := range value {\n\t\tcopied[k] = v",
	"../harness/graph/types/scratchpad.go":                         "copied := make(map[string]interface{}, len(p.data))\n\tfor k, v := range p.data {\n\t\tcopied[k] = v",
	"../harness/graph/managed/managed.go":                          "result := make(map[string]interface{}, len(base))\n\tfor k, v := range base {",
	"../harness/graph/graph/state.go":                              "for k, v := range currentMap {\n\t\t\t\t\tresult[k] = v",
	"../harness/graph/channels/reducer.go":                         "for k, v := range aMap {\n\t\t\t\t\t\tresult[k] = v",
}

// extraCPatterns are additional legacy forms in files already covered above.
var helperConvergenceCExtraPatterns = map[string][]string{
	"../service/dataset_structure_graph.go": {
		"dedup := out[:0]",
	},
	"../harness/graph/types/scratchpad.go": {
		"copied := make(map[string]interface{}, len(p.metadata))\n\tfor k, v := range p.metadata {\n\t\tcopied[k] = v",
	},
	"../harness/graph/channels/reducer.go": {
		"for k, v := range currentMap {\n\t\t\t\t\tresult[k] = v",
	},
	"../tokenizer/embedlimit.go": {
		"func containsAnyCode(msg string, codes []string) bool {\n\tfor _, code := range codes {",
	},
}

func TestDeduplicateAndCloneConverged(t *testing.T) {
	leftovers := make([]string, 0)
	for path, pattern := range helperConvergenceCPatterns {
		if strings.Contains(readHelperConvergenceAFile(t, path), pattern) {
			leftovers = append(leftovers, path+": "+pattern)
		}
	}
	for path, patterns := range helperConvergenceCExtraPatterns {
		src := readHelperConvergenceAFile(t, path)
		for _, pattern := range patterns {
			if strings.Contains(src, pattern) {
				leftovers = append(leftovers, path+": "+pattern)
			}
		}
	}
	if len(leftovers) > 0 {
		t.Errorf("inline dedupe/clone forms still present:\n  %s", strings.Join(leftovers, "\n  "))
	}
}
