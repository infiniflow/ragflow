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
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestChatCompletionsStreamPreservesFinalReference(t *testing.T) {
	client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, `data: {"code":0,"message":"success","data":{"answer":"See "}}

data: {"code":0,"message":"success","data":{"answer":"the manual.","reference":{"chunks":[{"document_name":"manual.pdf"}]}}}

data: {"code":0,"message":"success","data":true}

`)
	}))
	client.useAPIKey = true
	c := &CLI{
		Config: &CommandLineConfig{
			CLIMode:         APIMode,
			APIClientConfig: APIModeConfig{CurrentAPIServer: "test"},
		},
		APIServerClientMap: map[string]*HTTPClient{"test": client},
	}
	result, err := c.ChatCompletions(1, &Command{Params: map[string]interface{}{
		"chat_id": "chat-id", "question": "where?", "stream": true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	response := result.(*ChatCompletionsResponse)
	if response.Data.Answer != "See the manual." {
		t.Errorf("answer = %q", response.Data.Answer)
	}
	var reference struct {
		Chunks []struct {
			DocumentName string `json:"document_name"`
		} `json:"chunks"`
	}
	if err := json.Unmarshal(response.Data.Reference, &reference); err != nil {
		t.Fatalf("reference = %s: %v", response.Data.Reference, err)
	}
	if len(reference.Chunks) != 1 || reference.Chunks[0].DocumentName != "manual.pdf" {
		t.Errorf("reference = %s, want manual.pdf chunk", response.Data.Reference)
	}
}
