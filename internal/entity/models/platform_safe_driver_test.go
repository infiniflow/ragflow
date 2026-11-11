package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"
)

func TestPlatformOpenAICompatibleRedirectsStayOnOrigin(t *testing.T) {
	for _, makeDriver := range []func() ModelDriver{
		func() ModelDriver { return NewOpenAIModel(nil, URLSuffix{}) },
		func() ModelDriver { return NewOpenAIAPICompatibleModel(nil, URLSuffix{}) },
	} {
		driver := makeDriver()
		wrapped := NewPlatformSafeDriver(driver)
		protected, _ := PlatformDriverBase(wrapped)
		if protected == driver {
			t.Fatalf("%T was not isolated", driver)
		}
		var client *http.Client
		switch d := protected.(type) {
		case *OpenAIModel:
			client = d.baseModel.httpClient
		case *OpenAIAPICompatibleModel:
			client = d.baseModel.httpClient
		}
		original, _ := http.NewRequest(http.MethodGet, "https://provider.example:443/path", nil)
		same, _ := http.NewRequest(http.MethodGet, "https://provider.example/next", nil)
		other, _ := http.NewRequest(http.MethodGet, "https://provider.example:8443/next", nil)
		same.Header.Set("Authorization", "Bearer secret")
		other.Header.Set("Authorization", "Bearer secret")
		if client.CheckRedirect == nil || client.CheckRedirect(same, []*http.Request{original}) != nil || client.CheckRedirect(other, []*http.Request{original}) == nil {
			t.Fatalf("%T did not enforce same-origin redirects", driver)
		}
	}
}

func TestPlatformWrapperPreservesSpecializedDriver(t *testing.T) {
	driver := NewSystemOneModel(nil, URLSuffix{})
	base, ok := PlatformDriverBase(NewPlatformSafeDriver(driver))
	if !ok || base != driver {
		t.Fatalf("specialized driver changed to %T", base)
	}
}

func TestJoinedPlatformErrorPreservesPermanentRetryClassification(t *testing.T) {
	err := errors.Join(errors.New("status 503"), errors.New("status 498"))
	if common.IsTransientError(err) || common.IsTransientError(PlatformDriverError(err)) {
		t.Fatal("joined permanent error became retryable")
	}
	single := errors.New("status 503 status 498")
	if !common.IsTransientError(single) || !common.IsTransientError(PlatformDriverError(single)) {
		t.Fatal("single-error retry policy changed")
	}
}

var errTestStreamDone = errors.New("stream done")

type platformStreamProbe struct{ *DummyModel }

func (d platformStreamProbe) ChatStreamlyWithSender(_ context.Context, _ string, _ []Message, _ *APIConfig, _ *ChatConfig, _ *common.ModelUsage, sender func(*string, *string) error) error {
	text := "hi"
	if err := sender(&text, nil); err != nil {
		return err
	}
	done := "[DONE]"
	return sender(&done, nil)
}

func TestPlatformDriverKeepsSenderErrors(t *testing.T) {
	driver := NewPlatformSafeDriver(platformStreamProbe{NewDummyModel(nil, URLSuffix{})})
	err := driver.ChatStreamlyWithSender(t.Context(), "model", nil, nil, nil, nil, func(text, _ *string) error {
		if text != nil && *text == "[DONE]" {
			return errTestStreamDone
		}
		return nil
	})
	if !errors.Is(err, errTestStreamDone) {
		t.Fatalf("sender sentinel was replaced: %v", err)
	}
}

func TestPlatformDriverErrorKeepsFailureKind(t *testing.T) {
	// A structured status stays structured; a status only in the text stays
	// text, so failover caches and text classifiers both see what they did.
	structured := PlatformDriverError(&APIStatusError{Status: 503, Body: "sk-secret"})
	var status *APIStatusError
	if !errors.As(structured, &status) || status.Status != 503 || strings.Contains(structured.Error(), "sk-secret") {
		t.Fatalf("structured status: %v", structured)
	}
	for _, tc := range []struct {
		err  error
		want string
	}{
		{errors.New("OpenAI embeddings API error: 429 Too Many Requests: key sk-secret"), "error 429"},
		{errors.New("anthropic messages API error: 529 Overloaded, body: sk-secret"), "error 529"},
		{errors.New("API request failed with status 401: invalid api key sk-secret"), "status 401"},
	} {
		safe := PlatformDriverError(tc.err)
		if errors.As(safe, &status) || !strings.Contains(safe.Error(), tc.want) || strings.Contains(safe.Error(), "sk-secret") {
			t.Fatalf("text status %q: %v", tc.want, safe)
		}
	}

	dns := &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "server misbehaving", Name: "secret.internal", IsTemporary: true}}
	if safe := PlatformDriverError(fmt.Errorf("bedrock: send request: %w", dns)); !strings.Contains(safe.Error(), "dial tcp") || strings.Contains(safe.Error(), "secret.internal") {
		t.Fatalf("dns: %v", safe)
	}

	reset := &net.OpError{Op: "read", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.7"), Port: 8443}, Err: os.NewSyscallError("read", syscall.ECONNRESET)}
	safe := PlatformDriverError(reset)
	var opErr *net.OpError
	if !errors.As(safe, &opErr) || !errors.Is(opErr.Err, syscall.ECONNRESET) {
		t.Fatalf("connection reset lost: %#v", safe)
	}
	if strings.Contains(safe.Error(), "10.0.0.7") || !strings.Contains(safe.Error(), "connection reset") {
		t.Fatalf("unsafe or unclassifiable network error: %v", safe)
	}

	withRetry := PlatformDriverError(errors.New("embedding error: status 429 retry-after: 7 token=abc"))
	// The hint reaches the retry policy through the original text only.
	if strings.Contains(withRetry.Error(), "retry-after") || strings.Contains(withRetry.Error(), "token=abc") ||
		!strings.Contains(common.ErrorText(withRetry), "retry-after: 7") {
		t.Fatalf("retry hint: %v", withRetry)
	}
	if got := PlatformDriverError(errors.New("rate limit reached for key sk-secret")).Error(); !strings.Contains(got, "rate limit") || strings.Contains(got, "sk-secret") {
		t.Fatalf("rate limit hint: %q", got)
	}
}

type platformOCRProbe struct{ *DummyModel }

func (d platformOCRProbe) OCRFile(context.Context, *string, []byte, *string, *APIConfig, *OCRConfig, *common.ModelUsage) (*OCRFileResponse, error) {
	text := "page"
	return &OCRFileResponse{Text: &text, Warnings: []string{"paddleocr result: entry 1: errorCode 5: upstream said sk-secret"}}, nil
}

func TestPlatformDriverHidesOCRWarnings(t *testing.T) {
	probe := platformOCRProbe{NewDummyModel(nil, URLSuffix{})}
	resp, err := NewPlatformSafeDriver(probe).OCRFile(t.Context(), nil, nil, nil, nil, nil, nil)
	if err != nil || len(resp.Warnings) != 1 || strings.Contains(resp.Warnings[0], "sk-secret") {
		t.Fatalf("warnings = %v, %v", resp.Warnings, err)
	}
	original, _ := probe.OCRFile(t.Context(), nil, nil, nil, nil, nil, nil)
	if !strings.Contains(original.Warnings[0], "sk-secret") {
		t.Fatal("tenant warnings changed")
	}
}

func TestPlatformDriverErrorKeepsRetrySignals(t *testing.T) {
	overLimit := []error{
		&APIStatusError{Status: 400, Body: `{"error":{"message":"This model's maximum context length is 8192 tokens; key sk-secret"}}`},
		errors.New(`SiliconFlow embeddings API error: 400, body: {"code":20015,"message":"The parameter is invalid sk-secret"}`),
		errors.New("embedding API error: 500, body: input (900 tokens) is too large to process sk-secret"),
	}
	for _, err := range overLimit {
		safe := PlatformDriverError(err)
		if !tokenizer.IsOverLimitError(safe) || strings.Contains(safe.Error(), "sk-secret") {
			t.Fatalf("over-limit signal: %v", safe)
		}
	}
	retry := PlatformDriverError(&APIStatusError{Status: 429, Body: "Please retry after 20 seconds sk-secret"})
	if !strings.Contains(common.ErrorText(retry), "retry after 20") || strings.Contains(retry.Error(), "sk-secret") {
		t.Fatalf("retry hint: %v", retry)
	}
	deadline := PlatformDriverError(fmt.Errorf("failed to send request: Post \"https://h/?k=sk-secret\": %w", context.DeadlineExceeded))
	if !errors.Is(deadline, context.DeadlineExceeded) || !strings.Contains(deadline.Error(), "failed to send request") || strings.Contains(deadline.Error(), "sk-secret") {
		t.Fatalf("send deadline: %v", deadline)
	}
	eof := PlatformDriverError(errors.New(`failed to send request: Post "https://h/?k=sk-secret": EOF`))
	if !strings.Contains(eof.Error(), "failed to send request") || strings.Contains(eof.Error(), "sk-secret") {
		t.Fatalf("send EOF: %v", eof)
	}
}

func TestPlatformDriverErrorKeepsOnlySafeClassification(t *testing.T) {
	raw := errors.New("embedding error 400K: context length exceeded sk-hidden-secret")
	wrapped := fmt.Errorf("embed batch: %w", PlatformDriverError(raw))
	for _, shown := range []string{wrapped.Error(), fmt.Sprintf("%v", wrapped), fmt.Sprintf("%+v", wrapped), fmt.Sprintf("%#v", wrapped), fmt.Sprintf("%#v", PlatformDriverError(raw))} {
		if strings.Contains(shown, "sk-hidden-secret") {
			t.Fatalf("original text shown: %q", shown)
		}
	}
	if encoded, err := json.Marshal(wrapped); err != nil || strings.Contains(string(encoded), "secret") {
		t.Fatalf("original text encoded: %s, %v", encoded, err)
	}
	if got := common.ErrorText(wrapped); strings.Contains(got, "sk-hidden-secret") || !strings.Contains(got, "context length") {
		t.Fatalf("unsafe or incomplete classification text = %q", got)
	}
	if got := common.ErrorText(PlatformDriverError(wrapped)); strings.Contains(got, "sk-hidden-secret") || !strings.Contains(got, "context length") {
		t.Fatalf("re-sanitized classification text = %q", got)
	}
}

func TestPlatformDriverErrorCopiesNoProviderDigits(t *testing.T) {
	for _, tc := range []struct{ raw, secret, want string }{
		{"MinerU V1 POST http://192.168.10.20:8000/file_parse: HTTP 500: upstream", "192", "error 500"},
		{"decode https://mineru.example/api/v4/extract/task?code=2718281828: invalid character '<'", "2718281828", ""},
		{"xunfei error: appid 1234567890 apisecret mismatch", "1234567890", ""},
		{"status 20240618123456 rejected", "20240618123456", ""},
	} {
		safe := PlatformDriverError(errors.New(tc.raw)).Error()
		if strings.Contains(safe, tc.secret) || !strings.Contains(safe, tc.want) {
			t.Errorf("raw %q -> safe %q", tc.raw, safe)
		}
	}
}

func TestPlatformDriverErrorKeepsNoPortOrRetryDigits(t *testing.T) {
	for _, raw := range []string{
		"dial mineru.internal:502 error",
		"retry-after: 10.24 via node 10.24.3.7",
		"failed to send request: dial tcp 127.0.0.1:529: connect: connection refused",
		"failed to send request: could not resolve hostname 'llm-429.invalid'",
		"runner on gpu-node-503 error",
		"key sk-live-401 error",
		"proxy at ops@500 error",
	} {
		safe := PlatformDriverError(errors.New(raw)).Error()
		if strings.Contains(safe, "502") || strings.Contains(safe, "10.24") ||
			strings.Contains(safe, "529") || strings.Contains(safe, "429") ||
			strings.Contains(safe, "503") || strings.Contains(safe, "401") || strings.Contains(safe, "500") {
			t.Errorf("raw %q -> safe %q", raw, safe)
		}
	}
}

func TestPlatformDriverErrorKeepsStreamSentinels(t *testing.T) {
	raw := &url.Error{Op: "Post", URL: "https://h/?k=sk-secret", Err: io.EOF}
	safe := PlatformDriverError(raw)
	if !errors.Is(safe, io.EOF) || strings.Contains(safe.Error(), "sk-secret") {
		t.Fatalf("EOF lost or text shown: %v", safe)
	}
	if errors.Is(PlatformDriverError(errors.New("boom")), io.EOF) {
		t.Fatal("unrelated error matches EOF")
	}
}

func TestPlatformDriverErrorKeepsAPIKeyHintWords(t *testing.T) {
	safe := PlatformDriverError(&APIStatusError{Status: 401, Body: `{"error":{"message":"invalid api key provided sk-secret"}}`}).Error()
	if !strings.Contains(safe, "invalid api") || strings.Contains(safe, "sk-secret") {
		t.Fatalf("safe = %q", safe)
	}
}
