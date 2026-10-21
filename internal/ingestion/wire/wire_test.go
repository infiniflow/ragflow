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

package wire

import (
	"testing"

	"ragflow/internal/agent/runtime"
)

// TestBlankImportRegistersIngestionComponents pins the registration contract
// this package exists for: blank-importing wire must make every ingestion
// component resolvable in the runtime registry. cmd entries rely on that
// import alone, so a removed or renamed blank import has to fail here rather
// than surface as "unknown component" at run time.
//
// The names are string literals on purpose: importing the component packages
// directly would register them via the test's own imports and hide a broken
// wire.go.
func TestBlankImportRegistersIngestionComponents(t *testing.T) {
	for _, name := range []string{
		"File", "Parser", "Tokenizer", "Extractor",
		"TokenChunker", "GeneralChunker", "PageChunker", "TableChunker",
	} {
		factory, category, _, ok := runtime.DefaultRegistry.Lookup(name)
		if !ok {
			t.Errorf("component %q is not registered; the blank imports in wire.go must trigger its init()", name)
			continue
		}
		if factory == nil {
			t.Errorf("component %q registered with a nil factory", name)
		}
		if category != runtime.CategoryIngestion {
			t.Errorf("component %q category = %q, want %q", name, category, runtime.CategoryIngestion)
		}
	}
}
