package models

import (
	"testing"
)

func TestExtractOpenAIUsage(t *testing.T) {
	cases := []struct {
		name   string
		body   map[string]any
		want   *TokenUsage
		wantOK bool
	}{
		{
			name: "standard openai",
			body: map[string]any{
				"usage": map[string]any{
					"prompt_tokens":     float64(10),
					"completion_tokens": float64(20),
					"total_tokens":      float64(30),
				},
			},
			want:   &TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
			wantOK: true,
		},
		{
			name: "total missing falls back to sum",
			body: map[string]any{
				"usage": map[string]any{
					"prompt_tokens":     float64(10),
					"completion_tokens": float64(20),
				},
			},
			want:   &TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30},
			wantOK: true,
		},
		{
			name: "no usage key",
			body: map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{"content": "hi"}}},
			},
			want:   nil,
			wantOK: false,
		},
		{
			name: "cohere style input_tokens/output_tokens",
			body: map[string]any{
				"usage": map[string]any{
					"input_tokens":  float64(100),
					"output_tokens": float64(50),
				},
			},
			want:   &TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			wantOK: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractOpenAIUsage(tc.body)
			if ok != tc.wantOK {
				t.Fatalf("extractOpenAIUsage() ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if got.PromptTokens != tc.want.PromptTokens ||
				got.CompletionTokens != tc.want.CompletionTokens ||
				got.TotalTokens != tc.want.TotalTokens {
				t.Fatalf("extractOpenAIUsage() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestExtractOpenAIStreamUsage(t *testing.T) {
	cases := []struct {
		name   string
		event  map[string]any
		want   *TokenUsage
		wantOK bool
	}{
		{
			name: "standard streaming event",
			event: map[string]any{
				"usage": map[string]any{
					"prompt_tokens":     float64(100),
					"completion_tokens": float64(50),
					"total_tokens":      float64(150),
				},
			},
			want:   &TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
			wantOK: true,
		},
		{
			name: "null usage",
			event: map[string]any{
				"usage": nil,
			},
			want:   nil,
			wantOK: false,
		},
		{
			name: "no usage key",
			event: map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{"content": "hi"}}},
			},
			want:   nil,
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractOpenAIStreamUsage(tc.event)
			if ok != tc.wantOK {
				t.Fatalf("extractOpenAIStreamUsage() ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if got.PromptTokens != tc.want.PromptTokens ||
				got.CompletionTokens != tc.want.CompletionTokens ||
				got.TotalTokens != tc.want.TotalTokens {
				t.Fatalf("extractOpenAIStreamUsage() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestExtractToken(t *testing.T) {
	m := map[string]any{
		"a": float64(1),
		"b": 2,
		"c": int64(3),
	}

	if got := extractToken(m, "a"); got != 1 {
		t.Fatalf("extractToken(a) = %d, want 1", got)
	}
	if got := extractToken(m, "b"); got != 2 {
		t.Fatalf("extractToken(b) = %d, want 2", got)
	}
	if got := extractToken(m, "c"); got != 3 {
		t.Fatalf("extractToken(c) = %d, want 3", got)
	}
	if got := extractToken(m, "missing"); got != 0 {
		t.Fatalf("extractToken(missing) = %d, want 0", got)
	}
	if got := extractToken(m, "missing", "a"); got != 1 {
		t.Fatalf("extractToken(missing, a) = %d, want 1", got)
	}
}
