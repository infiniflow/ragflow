package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/common"
)

// TestTaskFailureDetailLLMAttribution walks the same wrap chain a real
// extractor failure produces (retry wrap -> component wrap -> canvas wrap)
// and asserts the user-visible detail names the model service instead of
// leaking the internal pipeline.
func TestTaskFailureDetailLLMAttribution(t *testing.T) {
	provider := common.NewLLMProviderError("deepseek", "deepseek-chat",
		errors.New("API request failed with status 402: insufficient balance"))
	chain := fmt.Errorf("pipeline: run canvas workflow: %w",
		fmt.Errorf("canvas: component %q invoke: %w", "Extractor:0",
			fmt.Errorf("extractor: chunk 3 keywords: %w",
				fmt.Errorf("failed after 3 retries: %w", provider))))

	got := taskFailureDetail(chain)
	if !strings.HasPrefix(got, "[ERROR] ") {
		t.Fatalf("failure detail must carry the [ERROR] prefix the front end uses for red styling, got %q", got)
	}
	if !strings.Contains(got, "deepseek") || !strings.Contains(got, "insufficient balance") {
		t.Fatalf("detail must name model + provider reason, got %q", got)
	}
	if strings.Contains(got, "canvas:") || strings.Contains(got, "run canvas workflow") {
		t.Fatalf("internal wrap must not reach the user, got %q", got)
	}

	configErr := common.NewLLMConfigError("", "gpt-9@nope",
		errors.New("extractor: tenant model \"gpt-9@nope\" not found or not usable: record not found"))
	got = taskFailureDetail(fmt.Errorf("canvas: component %q invoke: %w", "Extractor:0", configErr))
	if !strings.Contains(got, "Model Providers") || !strings.Contains(got, "gpt-9@nope") {
		t.Fatalf("config failure must point at model settings, got %q", got)
	}
}

func TestTaskFailureDetailInternalUnchanged(t *testing.T) {
	internal := errors.New("extractor: chunk text is empty")
	got := taskFailureDetail(fmt.Errorf("pipeline: run canvas workflow: %w", internal))
	if got != `[ERROR] Task failed: pipeline: run canvas workflow: extractor: chunk text is empty` {
		t.Fatalf("non-LLM errors must keep the full chain verbatim behind the [ERROR] prefix, got %q", got)
	}
}
