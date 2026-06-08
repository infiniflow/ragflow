package models

import "testing"

func TestNormalizeModelFamily(t *testing.T) {
	tests := []struct {
		name  string
		input *string
		want  string
	}{
		{name: "nil", input: nil, want: ""},
		{name: "empty", input: modelFamilyTestString(""), want: ""},
		{name: "qwen3", input: modelFamilyTestString("qwen3"), want: "qwen3"},
		{name: "qwen3 variant", input: modelFamilyTestString("qwen3-8b"), want: "qwen3"},
		{name: "provider-prefixed qwen3", input: modelFamilyTestString("qwen/qwen3-8b"), want: "qwen3"},
		{name: "case-varied qwen3", input: modelFamilyTestString("Qwen/Qwen3.5-4B"), want: "qwen3"},
		{name: "provider-prefixed non-qwen", input: modelFamilyTestString("deepseek/deepseek-r1"), want: "deepseek"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeModelFamily(tt.input); got != tt.want {
				t.Fatalf("NormalizeModelFamily()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetThinkingAndAnswerExtractsQwenThinking(t *testing.T) {
	tests := []string{
		"qwen3",
		"qwen3-8b",
		"qwen/qwen3",
		"qwen/qwen3-8b",
		"Qwen/Qwen3.5-4B",
	}

	for _, modelFamily := range tests {
		t.Run(modelFamily, func(t *testing.T) {
			content := "<think>\nreasoning</think>\nanswer"
			reasoning, answer := GetThinkingAndAnswer(&modelFamily, &content)

			if reasoning == nil || *reasoning != "reasoning" {
				t.Fatalf("reasoning=%v, want reasoning", reasoning)
			}
			if answer == nil || *answer != "answer" {
				t.Fatalf("answer=%v, want answer", answer)
			}
		})
	}
}

func TestGetThinkingAndAnswerSkipsUnknownFamily(t *testing.T) {
	modelFamily := "deepseek/deepseek-r1"
	content := "<think>reasoning</think>answer"

	reasoning, answer := GetThinkingAndAnswer(&modelFamily, &content)
	if reasoning != nil {
		t.Fatalf("reasoning=%q, want nil", *reasoning)
	}
	if answer == nil || *answer != content {
		t.Fatalf("answer=%v, want original content", answer)
	}
}

func TestGetThinkingAndAnswerHandlesNilInputs(t *testing.T) {
	reasoning, answer := GetThinkingAndAnswer(nil, nil)
	if reasoning != nil || answer != nil {
		t.Fatalf("GetThinkingAndAnswer(nil, nil)=(%v, %v), want nils", reasoning, answer)
	}

	content := "answer"
	reasoning, answer = GetThinkingAndAnswer(nil, &content)
	if reasoning != nil {
		t.Fatalf("reasoning=%q, want nil", *reasoning)
	}
	if answer == nil || *answer != content {
		t.Fatalf("answer=%v, want original content", answer)
	}
}

func modelFamilyTestString(value string) *string {
	return &value
}
