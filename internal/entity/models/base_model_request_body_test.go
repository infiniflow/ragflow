package models

import "testing"

func TestBuildRequestBodyOmitsNonPositiveMaxTokens(t *testing.T) {
	zero := 0
	body := buildRequestBody(
		&ChatConfig{MaxTokens: &zero},
		"test-model",
		[]Message{{Role: "user", Content: "hello"}},
		false,
	)

	if _, ok := body["max_tokens"]; ok {
		t.Fatalf("max_tokens should be omitted, got %#v", body["max_tokens"])
	}
}

func TestBuildRequestBodyOmitsPositiveMaxTokens(t *testing.T) {
	mt := 256
	body := buildRequestBody(
		&ChatConfig{MaxTokens: &mt},
		"test-model",
		[]Message{{Role: "user", Content: "hello"}},
		false,
	)

	if _, ok := body["max_tokens"]; ok {
		t.Fatalf("max_tokens should be omitted, got %#v", body["max_tokens"])
	}
}
