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

package cli

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestChatCompletionsStreamReportsTruncatedConnection(t *testing.T) {
	const frame = "data: {\"code\":0,\"data\":{\"answer\":\"partial\"}}\n\n"
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/chat/completions" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(frame)+1))
		_, _ = fmt.Fprint(w, frame)
	}))
	client.useAPIKey = true
	c := &CLI{
		Config:             &CommandLineConfig{CLIMode: APIMode, APIClientConfig: APIModeConfig{CurrentAPIServer: "test"}},
		APIServerClientMap: map[string]*HTTPClient{"test": client},
	}
	result, err := c.ChatCompletions(1, &Command{Params: map[string]interface{}{
		"chat_id": "chat-1", "question": "Say hello", "stream": true,
	}})
	if err == nil || !strings.Contains(err.Error(), "unexpected EOF") {
		t.Errorf("result = %#v, error = %v; want a stream read error", result, err)
	}
}
