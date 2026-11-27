//
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

package component

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"ragflow/internal/entity/models"
)

// TestOCRFailoverCanRetry verifies that the gp- flip-and-retry decision only
// fires for failures that implicate the model member: polling timeouts,
// document-property failures, and context cancellation must not flip.
func TestOCRFailoverCanRetry(t *testing.T) {
	wrap := func(err error) error { return fmt.Errorf("parser: Unlimited-OCR-2 result: %w", err) }

	cases := []struct {
		name      string
		err       error
		wantRetry bool
	}{
		{name: "nil", err: nil, wantRetry: false},
		{name: "member failure", err: errors.New("connection refused"), wantRetry: true},
		{name: "poll timeout plain", err: errOCRTaskPollTimeout, wantRetry: false},
		{name: "poll timeout wrapped", err: wrap(fmt.Errorf("timed out: %w (%w)", errors.New("boom"), errOCRTaskPollTimeout)), wantRetry: false},
		{name: "no textual layouts", err: wrap(errMonkeyOCRNoTextualLayouts), wantRetry: false},
		{name: "image OCR empty text", err: wrap(fmt.Errorf("selected image OCR returned no text (%w)", errOCRResultNoText)), wantRetry: false},
		{name: "MinerU response terminal", err: wrap(fmt.Errorf("parser: MinerU extract: %w (%w)", errors.New("corrupt archive"), errOCRResponseTerminal)), wantRetry: false},
		// Per-member timeouts come from inner contexts; the outer ctx stays
		// alive, so a hung member must still fail over. Caller cancellation
		// is handled by InvokeWithGroupFailover's ctx.Err() guard instead.
		{name: "inner per-member deadline", err: wrap(context.DeadlineExceeded), wantRetry: true},
		{name: "deadline wrapped in provider error", err: wrap(fmt.Errorf("post: %w", context.DeadlineExceeded)), wantRetry: true},
		{name: "inner context canceled", err: wrap(context.Canceled), wantRetry: true},
		// Request-level refusals fail identically on any member and must not
		// flip one.
		{name: "payload too large", err: &models.APIStatusError{Status: http.StatusRequestEntityTooLarge, Body: "file too large"}, wantRetry: false},
		{name: "bad request", err: fmt.Errorf("post: %w", &models.APIStatusError{Status: http.StatusBadRequest, Body: "invalid pages"}), wantRetry: false},
		{name: "server error still flips", err: &models.APIStatusError{Status: http.StatusBadGateway, Body: "upstream"}, wantRetry: true},
		{name: "rate limit still flips", err: &models.APIStatusError{Status: http.StatusTooManyRequests, Body: "slow down"}, wantRetry: true},
		{name: "request timeout still flips", err: &models.APIStatusError{Status: http.StatusRequestTimeout, Body: "timed out"}, wantRetry: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ocrFailoverCanRetry(tc.err); got != tc.wantRetry {
				t.Fatalf("ocrFailoverCanRetry(%v) = %v, want %v", tc.err, got, tc.wantRetry)
			}
		})
	}
}
