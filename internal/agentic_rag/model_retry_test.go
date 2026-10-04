package agentic_rag

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// The exact provider error that aborted a q268 benchmark run (529 overload).
const overloadErrStr = `[NodeRunError] models: EinoChatModel.Generate(MiniMax-M3): API request failed with status 529: {"type":"error","error":{"type":"overloaded_error","message":"当前服务集群负载较高，请稍后重试，感谢您的耐心等待。 (2064)"}}`

func TestTransientLLMError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{overloadErrStr, true},
		{"API request failed with status 429: rate limited", true},
		{"API request failed with status 503: upstream unavailable", true},
		{"connection reset by peer", true},
		{"minimax API error: 已达到 Token Plan 速率限制：请升级 Token Plan 套餐或切换为按量付费 API 使用。", true},
		// The gateway uses the SAME wording for a short TPM/RPM burst as for
		// true plan exhaustion (and the message carries no HTTP status), so
		// 用量上限 is retryable: a burst recovers within the 2s/5s/10s
		// backoff, a real wall merely costs llmRetryMax attempts before it
		// fails fast. Treating it as terminal aborted whole concurrent
		// benchmark runs on momentary bursts.
		{"minimax API error: 已达到 Token Plan 用量上限：请升级 Token Plan 套餐或购买积分补充用量。", true},
		{"API request failed with status 400: invalid request body", false},
		{"API request failed with status 401: bad api key", false},
	}
	for _, tc := range cases {
		if got := transientLLMError(errors.New(tc.msg)); got != tc.want {
			t.Errorf("transientLLMError(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}

// The eino-native retry policy must retry transient provider failures only
// while nothing has streamed out, and never double a spent deadline.
func TestAgentModelRetryShouldRetry(t *testing.T) {
	rc := agentModelRetryConfig()
	decision := func(err error, output *schema.Message) bool {
		d := rc.ShouldRetry(context.Background(), &adk.RetryContext{
			Err:           err,
			OutputMessage: output,
		})
		return d != nil && d.Retry
	}

	cases := []struct {
		name    string
		err     error
		output  *schema.Message
		wantRun bool
	}{
		{name: "529 overload before any delta retries", err: errors.New(overloadErrStr), wantRun: true},
		{name: "429 retries", err: errors.New("status 429: rate limit"), wantRun: true},
		{name: "connection reset retries", err: errors.New("connection reset by peer"), wantRun: true},
		{name: "client error fails fast", err: errors.New("status 401: bad api key"), wantRun: false},
		{name: "spent deadline fails fast", err: context.DeadlineExceeded, wantRun: false},
		{name: "spent Generate budget fails fast", err: errors.New("EinoChatModel.Generate(MiniMax-M3): context deadline exceeded (300s)"), wantRun: false},
		{name: "send-phase deadline retries", err: errors.New(`EinoChatModel.Generate(MiniMax-M3): failed to send request: Post "https://api.minimaxi.com/v1/text/chatcompletion_v2": context deadline exceeded`), wantRun: true},
		{name: "canceled context fails fast", err: context.Canceled, wantRun: false},
		{name: "mid-stream failure after partial output fails", err: errors.New(overloadErrStr), output: schema.AssistantMessage("partial", nil), wantRun: false},
		{name: "clean success is never retried", err: nil, wantRun: false},
	}
	for _, tc := range cases {
		if got := decision(tc.err, tc.output); got != tc.wantRun {
			t.Errorf("%s: retry = %v, want %v", tc.name, got, tc.wantRun)
		}
	}
}

// Backoff rides out an overload spike: 2s → 5s → 10s, then flat.
func TestAgentModelRetryBackoff(t *testing.T) {
	bf := agentModelRetryConfig().BackoffFunc
	want := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 10 * time.Second}
	for attempt := 1; attempt <= 4; attempt++ {
		if got := bf(context.Background(), attempt); got != want[attempt-1] {
			t.Errorf("backoff(attempt %d) = %v, want %v", attempt, got, want[attempt-1])
		}
	}
}
