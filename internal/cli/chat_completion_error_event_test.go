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
	"testing"
)

func TestChatCompletionsStreamReportsServerError(t *testing.T) {
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"code\":500,\"message\":\"chat unavailable\",\"data\":{\"answer\":\"**ERROR**: chat unavailable\",\"reference\":[]}}\n\n")
	}))
	client.useAPIKey = true
	c := &CLI{
		Config:             &CommandLineConfig{CLIMode: APIMode, APIClientConfig: APIModeConfig{CurrentAPIServer: "test"}},
		APIServerClientMap: map[string]*HTTPClient{"test": client},
	}
	result, err := c.ChatCompletions(1, &Command{Params: map[string]interface{}{"chat_id": "chat-id", "question": "hello", "stream": true}})
	if err != nil {
		t.Fatal(err)
	}
	reply := result.(*ChatCompletionsResponse)
	if reply.Code != 500 || reply.Message != "chat unavailable" {
		t.Fatalf("code = %d, message = %q, want 500 and chat unavailable", reply.Code, reply.Message)
	}
}
