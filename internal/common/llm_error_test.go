package common

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestLLMErrorChainPreserved(t *testing.T) {
	raw := errors.New("API request failed with status 429: too many requests")
	le := NewLLMProviderError("openai", "gpt-4o@openai", raw)
	wrapped := fmt.Errorf("extractor: chunk 3 keywords: %w", le)
	outer := fmt.Errorf("canvas: component \"Extractor:0\" invoke: %w", wrapped)

	got, ok := AsLLMError(outer)
	if !ok {
		t.Fatalf("AsLLMError through multi-layer %%w wrapping: not found in %v", outer)
	}
	if got != le {
		t.Fatalf("AsLLMError returned %v, want %v", got, le)
	}
	if !errors.Is(outer, raw) {
		t.Fatal("underlying provider error no longer reachable via errors.Is")
	}
}

func TestExtractHTTPStatus(t *testing.T) {
	cases := []struct {
		msg  string
		want int
	}{
		{"API request failed with status 502: bad gateway", 502},
		{"API request failed with status 429: {}", 429},
		{"error, status code: 401, message: unauthorized", 401},
		{"SILICONFLOW API error: 429 Too Many Requests", 429},
		{"dial tcp: connection refused", 0},
		{"context deadline exceeded after 400ms", 0},
		{"", 0},
	}
	for _, c := range cases {
		var err error
		if c.msg != "" {
			err = errors.New(c.msg)
		}
		if got := ExtractHTTPStatus(err); got != c.want {
			t.Errorf("ExtractHTTPStatus(%q) = %d, want %d", c.msg, got, c.want)
		}
	}
}

func TestLLMErrorUserMessage(t *testing.T) {
	le := NewLLMProviderError("deepseek", "deepseek-chat@Tongyi-Qianwen",
		errors.New("API request failed with status 402: insufficient balance"))
	msg := le.UserMessage()
	if !strings.Contains(msg, "deepseek-chat@Tongyi-Qianwen") {
		t.Errorf("UserMessage should name the model: %q", msg)
	}
	if !strings.Contains(msg, "insufficient balance") {
		t.Errorf("UserMessage should echo the provider reason: %q", msg)
	}
	if !strings.Contains(msg, "not RAGFlow") {
		t.Errorf("UserMessage must attribute the failure to the model service: %q", msg)
	}

	ce := NewLLMConfigError("", "gpt-9@nope", errors.New("tenant model not found"))
	cmsg := ce.UserMessage()
	if !strings.Contains(cmsg, "gpt-9@nope") || !strings.Contains(cmsg, "Model Providers") {
		t.Errorf("config UserMessage should point at model settings: %q", cmsg)
	}

	long := NewLLMProviderError("p", "m", errors.New(strings.Repeat("x", 5000)))
	if len(long.UserMessage()) > 1000 {
		t.Error("UserMessage should truncate long provider bodies")
	}
}

func TestTruncateForUser(t *testing.T) {
	if got := TruncateForUser("line1\n  line2\ttab"); got != "line1 line2 tab" {
		t.Errorf("whitespace not collapsed: %q", got)
	}
	big := TruncateForUser(strings.Repeat("y", maxUserReasonLen+100))
	if !strings.HasSuffix(big, "...") || len(big) != maxUserReasonLen+3 {
		t.Errorf("length cap failed: len=%d", len(big))
	}
}

func TestLLMErrorTextKeepsCause(t *testing.T) {
	le := NewLLMProviderError("openai", "gpt", errors.New("bad request: content filter"))
	text := le.Error()
	if !strings.Contains(text, "content filter") {
		t.Errorf("Error() must retain the underlying message for logs: %q", text)
	}
	if !strings.Contains(text, "provider=openai") || !strings.Contains(text, "model=gpt") {
		t.Errorf("Error() must carry attribution fields: %q", text)
	}
}
