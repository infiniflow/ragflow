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
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestIntentIsAdvertisedAsAProperty is a regression guard for a schema shape
// that unmarshals cleanly and therefore fails silently: an "intent" block placed
// as a sibling of "properties" instead of inside it. JSON stays valid, required
// still names it, and the model never receives the type or the description that
// tells it what to write — which is exactly the value the judge reads.
func TestIntentIsAdvertisedAsAProperty(t *testing.T) {
	tools := map[string]interface {
		Info(context.Context) (*schema.ToolInfo, error)
	}{
		"grep_chunks":            NewGrepChunksTool("tenant", []string{"kb1"}),
		"list_chunks":            NewListChunksTool("tenant", []string{"kb1"}),
		"search_bm25_chunks":     NewSearchBm25ChunksTool("tenant", []string{"kb1"}),
		"search_semantic_chunks": NewSearchSemanticChunksTool("tenant", []string{"kb1"}),
		"search_chunks":          NewSearchChunksTool("tenant", []string{"kb1"}),
	}
	for name, tool := range tools {
		t.Run(name, func(t *testing.T) {
			info, err := tool.Info(context.Background())
			if err != nil {
				t.Fatalf("Info: %v", err)
			}
			js, err := info.ParamsOneOf.ToJSONSchema()
			if err != nil {
				t.Fatalf("ToJSONSchema: %v", err)
			}
			intent, ok := js.Properties.Get("intent")
			if !ok {
				t.Fatalf("intent is not a property of %s (required lists %v, so a root-level "+
					"\"intent\" key would be silently dropped and the model would never see it)",
					name, js.Required)
			}
			if !strings.Contains(intent.Description, "one sentence") {
				t.Errorf("intent description = %q, want the guidance the model writes against", intent.Description)
			}
			required := false
			for _, r := range js.Required {
				if r == "intent" {
					required = true
				}
			}
			if !required {
				t.Errorf("intent is not in required: %v", js.Required)
			}
		})
	}
}
