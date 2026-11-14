//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package channels

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// A malformed sessionWebhook still yields a *url.Error whose URL carries the
// whole query, so the credential must be redacted on the parse-failure path.
func TestRedactURLErrorMalformedURL(t *testing.T) {
	const secret = "SESSION_SECRET_VALUE"
	cases := []string{
		"https://oapi.dingtalk.com/robot/send%zz?session=" + secret,
		"http://oapi.dingtalk.com:bad/robot/send?session=" + secret,
		"https://oapi.dingtalk.com/robot/send\x7f?session=" + secret,
	}
	for _, raw := range cases {
		_, err := http.NewRequestWithContext(context.Background(), http.MethodPost, raw, nil)
		if err == nil {
			t.Fatalf("%q: expected a request error", raw)
		}
		redacted := redactURLError(err)
		if strings.Contains(redacted.Error(), secret) {
			t.Fatalf("credential leaked in %q: %v", raw, redacted)
		}
		if !strings.Contains(redacted.Error(), "parse") {
			t.Fatalf("%q: unexpected redacted error: %v", raw, redacted)
		}
	}
}
