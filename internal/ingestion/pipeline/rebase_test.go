package pipeline

import (
	"encoding/json"
	"testing"
)

func rebaseDSL(chunkTokenSize int) []byte {
	dsl := map[string]any{
		"components": map[string]any{
			"GeneralChunker:SixApplesFall": map[string]any{
				"downstream": []any{"Tokenizer:LegalReadersDecide"},
				"obj": map[string]any{
					"component_name": "GeneralChunker",
					"params": map[string]any{
						"chunk_token_size": chunkTokenSize,
						"delimiters":       []any{"\n", "!", "?", ";"},
					},
				},
			},
			"Tokenizer:LegalReadersDecide": map[string]any{
				"downstream": []any{},
				"obj": map[string]any{
					"component_name": "Tokenizer",
					"params": map[string]any{
						"fields": "text",
					},
				},
			},
		},
	}
	raw, err := json.Marshal(dsl)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestRebaseStoredParamsOnPipelineEditDropsSeededDefaults(t *testing.T) {
	stored := map[string]any{
		"GeneralChunker:SixApplesFall": map[string]any{
			"chunk_token_size":   512.0,
			"delimiters":         []any{"\n", "!", "?", ";"},
			"overlapped_percent": 0.5,
		},
		"Tokenizer:LegalReadersDecide": map[string]any{
			"fields": "text",
		},
	}
	rebased := RebaseStoredParamsOnPipelineEdit(stored, rebaseDSL(512), rebaseDSL(64))
	chunker, ok := rebased["GeneralChunker:SixApplesFall"].(map[string]any)
	if !ok {
		t.Fatalf("expected chunker entry to survive, got %v", rebased)
	}
	if _, exists := chunker["chunk_token_size"]; exists {
		t.Errorf("seeded default chunk_token_size must be dropped so the edited DSL wins")
	}
	if _, exists := chunker["delimiters"]; exists {
		t.Errorf("seeded default delimiters must be dropped so the edited DSL wins")
	}
	if got := chunker["overlapped_percent"]; got != 0.5 {
		t.Errorf("user override overlapped_percent must survive, got %v", got)
	}
	if _, exists := rebased["Tokenizer:LegalReadersDecide"]; exists {
		t.Errorf("component with only seeded defaults must vanish entirely")
	}
}

func TestRebaseStoredParamsOnPipelineEditKeepsDatasetScopedKeys(t *testing.T) {
	stored := map[string]any{
		"metadata": map[string]any{"enabled": true},
		"GeneralChunker:SixApplesFall": map[string]any{
			"chunk_token_size": 512.0,
		},
	}
	rebased := RebaseStoredParamsOnPipelineEdit(stored, rebaseDSL(512), rebaseDSL(512))
	if _, exists := rebased["metadata"]; !exists {
		t.Errorf("dataset-scoped keys must be preserved")
	}
	if _, exists := rebased["GeneralChunker:SixApplesFall"]; exists {
		t.Errorf("component reduced to nothing must be dropped")
	}
}

func TestRebaseStoredParamsOnPipelineEditDropsRemovedComponents(t *testing.T) {
	stored := map[string]any{
		"LegacyChunker:Gone": map[string]any{
			"chunk_token_size": 128.0,
		},
	}
	rebased := RebaseStoredParamsOnPipelineEdit(stored, rebaseDSL(512), rebaseDSL(512))
	if len(rebased) != 0 {
		t.Errorf("component absent from the new DSL must be dropped, got %v", rebased)
	}
}

func TestRebaseStoredParamsOnPipelineEditToleratesBrokenDSL(t *testing.T) {
	stored := map[string]any{
		"GeneralChunker:SixApplesFall": map[string]any{"chunk_token_size": 512.0},
	}
	for name, pair := range map[string][2][]byte{
		"old": {[]byte("not json"), rebaseDSL(64)},
		"new": {rebaseDSL(512), []byte(`{"components":null}`)},
	} {
		rebased := RebaseStoredParamsOnPipelineEdit(stored, pair[0], pair[1])
		if len(rebased) != len(stored) {
			t.Errorf("%s DSL: broken input must leave stored config untouched, got %v", name, rebased)
		}
	}
}

func TestRebaseStoredParamsOnPipelineEditComparesAcrossNumberDecodings(t *testing.T) {
	stored := map[string]any{
		"GeneralChunker:SixApplesFall": map[string]any{
			"chunk_token_size": 512,
		},
	}
	rebased := RebaseStoredParamsOnPipelineEdit(stored, rebaseDSL(512), rebaseDSL(64))
	if _, exists := rebased["GeneralChunker:SixApplesFall"]; exists {
		t.Errorf("int-typed stored value equal to the float64 DSL default must be dropped, got %v", rebased)
	}
}
