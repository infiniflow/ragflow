package common

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
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
		if got := extractHTTPStatus(err); got != c.want {
			t.Errorf("extractHTTPStatus(%q) = %d, want %d", c.msg, got, c.want)
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
	if got := truncateForUser("line1\n  line2\ttab"); got != "line1 line2 tab" {
		t.Errorf("whitespace not collapsed: %q", got)
	}
	big := truncateForUser(strings.Repeat("y", maxUserReasonLen+100))
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

func TestRefineReason(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		dont string
	}{
		{
			name: "nested error.message",
			in:   `API request failed with status 402: {"error":{"message":"Insufficient Balance","type":"billing_error"}}`,
			want: "API request failed with status 402: Insufficient Balance",
			dont: "billing_error",
		},
		{
			name: "body suffix",
			in:   `anthropic messages API error: 503 Service Unavailable, body: {"error":{"code":"model_not_found","message":"No available channel for model claude-haiku under group auto (distributor)","type":"new_api_error"}}`,
			want: "503 Service Unavailable: No available channel for model claude-haiku under group auto (distributor)",
			dont: "new_api_error",
		},
		{
			name: "top-level message",
			in:   `bad request: {"message":"model id not found"}`,
			want: "bad request: model id not found",
			dont: "{",
		},
		{
			name: "unparseable json unchanged",
			in:   `failed: {"error":{"message":"truncated`,
			want: `failed: {"error":{"message":"truncated`,
		},
		{
			name: "json without message unchanged",
			in:   `failed: {"error":{"code":"rate_limited"}}`,
			want: `failed: {"error":{"code":"rate_limited"}}`,
		},
		{
			name: "reordered keys",
			in:   `upstream error: {"type":"error","error":{"message":"overloaded_error"}}`,
			want: "upstream error: overloaded_error",
			dont: "type",
		},
		{
			name: "pretty printed body",
			in:   "failed:\n{\n  \"error\": {\n    \"message\": \"quota exceeded\"\n  }\n}",
			want: "failed: quota exceeded",
			dont: "{",
		},
		{
			name: "trailing text after body ignored",
			in:   `API error: 400: {"error":{"message":"bad input"}} (request id: abc)`,
			want: "API error: 400: bad input",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := refineReason(tc.in)
			if !strings.Contains(got, tc.want) {
				t.Errorf("refineReason(%q) = %q, want to contain %q", tc.in, got, tc.want)
			}
			if tc.dont != "" && strings.Contains(got, tc.dont) {
				t.Errorf("refineReason(%q) = %q, want NOT to contain %q", tc.in, got, tc.dont)
			}
		})
	}
}

func TestUserMessageRefinesProviderJSONBody(t *testing.T) {
	raw := errors.New(`API request failed with status 503: {"error":{"message":"No available channel for model x","type":"upstream_error"}}`)
	msg := NewLLMProviderError("anthropic", "claude-x", raw).UserMessage()
	if !strings.Contains(msg, "No available channel for model x") {
		t.Errorf("UserMessage must surface the nested provider message: %s", msg)
	}
	if strings.Contains(msg, "upstream_error") || strings.Contains(msg, `{"error"`) {
		t.Errorf("UserMessage must not leak the raw JSON body: %s", msg)
	}
}

func TestUserSummaryVersusUserMessage(t *testing.T) {
	le := NewLLMProviderError("anthropic", "claude-x", errors.New(`API error: 503: {"error":{"message":"No available channel for model claude-x"}}`))

	summary := le.UserSummary()
	if !strings.Contains(summary, `LLM call to "claude-x@anthropic" failed: API error: 503: No available channel`) {
		t.Errorf("summary = %q", summary)
	}
	if strings.Contains(summary, "not RAGFlow") {
		t.Errorf("summary must stay factual, no guidance: %q", summary)
	}

	msg := le.UserMessage()
	if !strings.HasPrefix(msg, summary) || !strings.Contains(msg, "This error was returned by your model service, not RAGFlow") {
		t.Errorf("UserMessage must equal summary + guidance once: %q", msg)
	}
	if n := strings.Count(msg, "not RAGFlow"); n != 1 {
		t.Errorf("guidance repeated %d times: %q", n, msg)
	}
}

func TestUserMessagePunctuationJoinsCleanly(t *testing.T) {
	// Provider messages commonly end with a period; no ".." may appear.
	le := NewLLMProviderError("openai", "gpt", errors.New("Incorrect API key provided."))
	msg := le.UserMessage()
	if strings.Contains(msg, "..") {
		t.Errorf("double period in user message: %q", msg)
	}
	if !strings.Contains(msg, "Incorrect API key provided. This error") {
		t.Errorf("period-ending reason must join with a single space: %q", msg)
	}
}

func TestTruncateForUserRedactsCredentials(t *testing.T) {
	in := `API request failed with status 401: invalid api-key: sk-abc123XYZ provided (authorization: Bearer tok-9f8e7d6c secret=whatever)`
	got := truncateForUser(in)
	for _, leaked := range []string{"sk-abc123XYZ", "tok-9f8e7d6c", "whatever"} {
		if strings.Contains(got, leaked) {
			t.Errorf("credential value %q must be redacted: %q", leaked, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("expected redaction marker: %q", got)
	}
	if strings.Contains(got, "Bearer ") {
		t.Errorf("authorization scheme-token pair must be redacted whole, not only the scheme: %q", got)
	}
}

func TestRedactCredentialsBasicScheme(t *testing.T) {
	got := RedactCredentials("auth failed: Authorization: Basic dXNlcjpwYXNz")
	if strings.Contains(got, "dXNlcjpwYXNz") {
		t.Errorf("basic auth token must be redacted: %q", got)
	}
}

func TestTruncateForUserIsRuneSafe(t *testing.T) {
	in := strings.Repeat("模型服务不可用", 100) // multi-byte runes spanning the cap
	got := truncateForUser(in)
	if !utf8.ValidString(got) {
		t.Errorf("truncated string splits a rune: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncation marker missing: %q", got)
	}
}
