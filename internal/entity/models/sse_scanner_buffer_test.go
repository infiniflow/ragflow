package models

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// chatStreamer is the streaming entrypoint shared by every OpenAI-compatible
// provider. The buffer regression below exercises it through a table so a new
// provider only needs one row.
type chatStreamer interface {
	ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error
}

// largeSSEStreamServer streams a single SSE "data:" line whose content delta is
// larger than the default 64KB bufio.Scanner token size, followed by a
// finish_reason chunk and the [DONE] sentinel. Without scanner.Buffer(...) the
// oversized line makes scanner.Scan() return false with bufio.ErrTooLong and the
// stream is truncated; with the raised buffer the full content is delivered.
func largeSSEStreamServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"content":"`+content+`"}}]}`+"\n"+
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
}

func TestChatStreamLargeChunkNotTruncated(t *testing.T) {
	// 128KB content delta: comfortably past the 64KB default so the bare
	// scanner would fail, well under the 1MB raised cap so the fix succeeds.
	const big = 128 * 1024
	content := strings.Repeat("a", big)

	suffix := URLSuffix{Chat: "chat/completions", Models: "models"}
	build := func(c func(map[string]string, URLSuffix) chatStreamer) func(string) chatStreamer {
		return func(baseURL string) chatStreamer {
			return c(map[string]string{"default": baseURL}, suffix)
		}
	}

	cases := []struct {
		name  string
		build func(string) chatStreamer
	}{
		{"deepinfra", build(func(b map[string]string, s URLSuffix) chatStreamer { return NewDeepInfraModel(b, s) })},
		{"vllm", build(func(b map[string]string, s URLSuffix) chatStreamer { return NewVllmModel(b, s) })},
		{"openrouter", build(func(b map[string]string, s URLSuffix) chatStreamer { return NewOpenRouterModel(b, s) })},
		{"siliconflow", build(func(b map[string]string, s URLSuffix) chatStreamer { return NewSiliconflowModel(b, s) })},
		{"moonshot", build(func(b map[string]string, s URLSuffix) chatStreamer { return NewMoonshotModel(b, s) })},
		{"deepseek", build(func(b map[string]string, s URLSuffix) chatStreamer { return NewDeepSeekModel(b, s) })},
		{"nvidia", build(func(b map[string]string, s URLSuffix) chatStreamer { return NewNvidiaModel(b, s) })},
		{"lmstudio", build(func(b map[string]string, s URLSuffix) chatStreamer { return NewLmStudioModel(b, s) })},
		{"gitee", build(func(b map[string]string, s URLSuffix) chatStreamer { return NewGiteeModel(b, s) })},
		{"tokenhub", build(func(b map[string]string, s URLSuffix) chatStreamer { return NewTokenHubModel(b, s) })},
		{"jiekouai", build(func(b map[string]string, s URLSuffix) chatStreamer { return NewJieKouAIModel(b, s) })},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := largeSSEStreamServer(t, content)
			defer srv.Close()

			apiKey := "test-key"
			var got strings.Builder
			err := tc.build(srv.URL).ChatStreamlyWithSender(
				"test-model",
				[]Message{{Role: "user", Content: "hi"}},
				&APIConfig{ApiKey: &apiKey},
				// Empty (non-nil) config: providers default stream=true and
				// only override when Stream != nil, so this streams for all of
				// them while avoiding a nil-config deref in providers that read
				// modelConfig unconditionally.
				&ChatConfig{},
				func(c *string, _ *string) error {
					if c != nil && *c != "[DONE]" {
						got.WriteString(*c)
					}
					return nil
				},
			)
			if err != nil {
				t.Fatalf("ChatStreamlyWithSender returned error (large chunk truncated?): %v", err)
			}
			if got.Len() != big {
				t.Fatalf("delivered %d bytes, want %d (content was truncated)", got.Len(), big)
			}
			if got.String() != content {
				t.Fatalf("delivered content does not match streamed content")
			}
		})
	}
}
