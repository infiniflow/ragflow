// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"
)

// platformParitySamples are provider failures as drivers report them. Each
// carries the secret; the sanitized platform error must not, and must be
// classified the same way as the tenant error.
func platformParitySamples() []error {
	const s = "sk-parity-secret"
	return []error{
		&APIStatusError{Status: 503, Body: `{"error":"model not found ` + s + `"}`},
		&APIStatusError{Status: 500, Body: "authentication failed " + s},
		&APIStatusError{Status: 429, Body: "Please retry after 20 seconds " + s},
		&APIStatusError{Status: 503, Body: "input (400 tokens) is too long " + s},
		&APIStatusError{Status: 400, Body: `{"error":{"message":"This model's maximum context length is 8192 tokens ` + s + `"}}`},
		errors.New("bad request: invalid parameter " + s),
		errors.New(`stream error: {"error":{"code":"model_not_found","message":"` + s + `"}}`),
		errors.New(`SILICONFLOW API error: 400 Bad Request, body: {"k":"` + s + `"}`),
		errors.New("Gitee API error: 401 Unauthorized, body: {" + s + "}"),
		errors.New("minimax API error: 已达到用量上限 (2056) " + s),
		errors.New("Too many requests, retry after 30 " + s),
		errors.New(`stream error: {"type":"overloaded_error","message":"` + s + `"}`),
		errors.New("anthropic messages API error: 529 Overloaded, body: " + s),
		errors.New("openai API error: 503 Service Unavailable " + s),
		errors.New("API request failed with status 503: " + s),
		errors.New("Status 503 Rate Limit exceeded " + s),
		errors.New("chat error: status 404 then status 503 " + s),
		errors.New(`SiliconFlow embeddings API error: 400, body: {"code":20015,"message":"The parameter is invalid ` + s + `"}`),
		errors.New("embedding API error: 500, body: input (900 tokens) is too large to process " + s),
		errors.New("embedding API error: 503, body: input (500 tokens) is too large to process " + s),
		errors.New(`http2: server sent goaway and closed the connection; LastStreamID=1, ErrCode=NO_ERROR, debug="` + s + `"`),
		errors.New(`failed to send request: Post "https://h/?k=` + s + `": EOF`),
		fmt.Errorf("failed to send request: %w", &url.Error{Op: "Post", URL: "https://h/?k=" + s, Err: context.Canceled}),
		fmt.Errorf("failed to send request: Post %q: %w", "https://h/?k="+s, context.DeadlineExceeded),
		fmt.Errorf("EinoChatModel.Generate(%s): %w", s, context.DeadlineExceeded),
		&url.Error{Op: "Post", URL: "https://h/?k=" + s, Err: &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}},
		fmt.Errorf("bedrock: send request: %w", &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "server misbehaving", Name: s, IsTemporary: true}}),
		&net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: s, IsNotFound: true}},
		// Lowercasing these changes byte lengths and offsets.
		errors.New("Status 503 Rate Limit exceeded " + s + " \xff"),
		errors.New(strings.Repeat("Ⱥ", 20) + " rate limit " + s + " " + strings.Repeat("K", 10)),
		// Status digits glued to a unit, one marker in several spellings, a
		// status only in the body, and letters that lowercase to ASCII.
		errors.New("chat error: no response within 400ms " + s),
		errors.New("embedding error: input rejected after 400ms, context length 8192 " + s),
		errors.New(`embedding response has no data, body: {"error":{"code":500,"message":"input is too large to process ` + s + `"}}`),
		errors.New("upstream: Rate Limit policy; rate limit reached " + s),
		errors.New("proxy said Connection Reset, then read: connection reset by peer " + s),
		errors.New("embedding error 400K: context length exceeded " + s),
		errors.New("chat failed after 404İ " + s),
		errors.New("embedding Kerror 503 " + s),
	}
}

// checkPlatformParity runs one classifier on each sample as a tenant model
// and as a platform model sees it.
func checkPlatformParity[T comparable](t *testing.T, name string, classify func(error) T) {
	t.Helper()
	for _, raw := range platformParitySamples() {
		safe := PlatformDriverError(raw)
		if strings.Contains(safe.Error(), "sk-parity-secret") {
			t.Fatalf("%s: secret kept in %q", name, safe.Error())
		}
		if tenant, platform := classify(raw), classify(safe); tenant != platform {
			t.Errorf("%s(%q) = %v, platform %q = %v", name, raw.Error(), tenant, safe.Error(), platform)
		}
	}
}

func TestPlatformErrorClassificationParity(t *testing.T) {
	checkPlatformParity(t, "IsTransientError", common.IsTransientError)
	checkPlatformParity(t, "IsOverLimitError", tokenizer.IsOverLimitError)
	checkPlatformParity(t, "cacheableSweepFailure", cacheableSweepFailure)
	checkPlatformParity(t, "canceled", func(err error) bool { return errors.Is(err, context.Canceled) })
	checkPlatformParity(t, "deadline", func(err error) bool { return errors.Is(err, context.DeadlineExceeded) })
	checkPlatformParity(t, "status", func(err error) int {
		var status *APIStatusError
		if errors.As(err, &status) {
			return status.Status
		}
		return 0
	})
}
