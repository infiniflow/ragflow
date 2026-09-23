// Package component — composite llm_id parsing tests.
//
// The canvas llm_id is right-anchored on the provider (Python
// split_model_name uses rsplit("@", 2)): the last '@'-separated segment is
// the provider, the second-to-last the instance, and everything to the left
// is the model name — which may itself contain '@' (e.g. LM Studio
// quant-suffixed ids like "text-embedding-nomic-embed-text-v1.5@q8_0",
// producing composites like "...@q8_0@lmstudio@LM-Studio").
package component

import "testing"

func TestParseLLMIDPartsRightAnchored(t *testing.T) {
	cases := []struct {
		in           string
		wantModel    string
		wantInstance string
		wantProvider string
	}{
		{"gpt-4o", "gpt-4o", "", ""},
		{"gpt-4o@OpenAI", "gpt-4o", "default", "OpenAI"},
		{"Qwen/Qwen3-8B@default@SILICONFLOW", "Qwen/Qwen3-8B", "default", "SILICONFLOW"},
		// Model name containing '@' — the real-world shape Python's
		// split_model_name is written for.
		{"text-embedding-nomic-embed-text-v1.5@q8_0@lmstudio@LM-Studio",
			"text-embedding-nomic-embed-text-v1.5@q8_0", "lmstudio", "LM-Studio"},
		{"a@b@c@d@e", "a@b@c", "d", "e"},
	}
	for _, tc := range cases {
		gotModel, gotInstance, gotProvider := parseLLMIDParts(tc.in)
		if gotModel != tc.wantModel || gotInstance != tc.wantInstance || gotProvider != tc.wantProvider {
			t.Errorf("parseLLMIDParts(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tc.in, gotModel, gotInstance, gotProvider, tc.wantModel, tc.wantInstance, tc.wantProvider)
		}
	}
}

func TestSplitCompositeLLMIDEmbeddedAt(t *testing.T) {
	// With a '@' inside the model name the provider must still be the last
	// segment — the old left-anchored split returned hasDriver=false here, so
	// no driver was resolved and the whole composite was passed upstream as
	// the model name.
	model, driver, ok := splitCompositeLLMID("text-embedding-nomic-embed-text-v1.5@q8_0@lmstudio@LM-Studio")
	if !ok || model != "text-embedding-nomic-embed-text-v1.5@q8_0" || driver != "LM-Studio" {
		t.Fatalf("splitCompositeLLMID = (%q, %q, %v), want (%q, %q, true)",
			model, driver, ok, "text-embedding-nomic-embed-text-v1.5@q8_0", "LM-Studio")
	}
}
