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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"

	"ragflow/internal/channels/core"
)

func TestNewFeishuChannelFromConfigRequiresCredentials(t *testing.T) {
	if _, err := newFeishuChannelFromConfig("account-1", map[string]any{"app_id": "app-1"}); err == nil {
		t.Fatal("newFeishuChannelFromConfig succeeded without app_secret")
	}
}

func TestNewFeishuChannelFromConfigNormalizesConfig(t *testing.T) {
	ch, err := newFeishuChannelFromConfig("account-1", map[string]any{
		"app_id":       "app-1",
		"app_secret":   "secret-1",
		"domain":       "lark",
		"timeout_secs": "7",
	})
	if err != nil {
		t.Fatalf("newFeishuChannelFromConfig returned error: %v", err)
	}
	if ch.account.AppID != "app-1" {
		t.Fatalf("app id = %q, want app-1", ch.account.AppID)
	}
	if ch.account.AppSecret != "secret-1" {
		t.Fatalf("app secret = %q, want secret-1", ch.account.AppSecret)
	}
	if ch.account.Domain != "lark" {
		t.Fatalf("domain = %q, want lark", ch.account.Domain)
	}
	if ch.account.Timeout != 7*time.Second {
		t.Fatalf("timeout = %s, want 7s", ch.account.Timeout)
	}
}

func TestFeishuMessageText(t *testing.T) {
	raw := `{"text":"hello"}`
	if got := feishuMessageText(&raw); got != "hello" {
		t.Fatalf("feishuMessageText() = %q, want hello", got)
	}

	plain := "hello"
	if got := feishuMessageText(&plain); got != "hello" {
		t.Fatalf("feishuMessageText() = %q, want raw fallback", got)
	}
}

func TestFeishuEnqueueDoesNotMarkDroppedMessageSeen(t *testing.T) {
	ch := newFeishuChannel(feishuAccount{AccountID: "account-1", AppID: "app-1", AppSecret: "secret-1"})
	worker := &feishuWorker{queue: make(chan core.IncomingMessage, 1)}
	worker.queue <- core.IncomingMessage{MessageID: "queued"}
	ch.workers["chat-1"] = worker

	ok := ch.enqueueIncoming(core.IncomingMessage{
		ChatID:    "chat-1",
		MessageID: "dropped",
	})

	if ok {
		t.Fatal("enqueueIncoming succeeded for a full queue")
	}
	if _, seen := ch.seen["dropped"]; seen {
		t.Fatal("dropped message was marked seen")
	}
}

func TestNewFeishuChannelFromConfigStreamFlag(t *testing.T) {
	for _, test := range []struct {
		name string
		cfg  map[string]any
		want bool
	}{
		{name: "enabled", cfg: map[string]any{"stream": true}, want: true},
		{name: "absent", cfg: map[string]any{}, want: false},
		{name: "non boolean value is ignored", cfg: map[string]any{"stream": "true"}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := map[string]any{"app_id": "app-1", "app_secret": "secret-1"}
			for key, value := range test.cfg {
				cfg[key] = value
			}
			ch, err := newFeishuChannelFromConfig("account-1", cfg)
			if err != nil {
				t.Fatalf("newFeishuChannelFromConfig returned error: %v", err)
			}
			if ch.StreamEnabled() != test.want {
				t.Fatalf("StreamEnabled() = %t, want %t", ch.StreamEnabled(), test.want)
			}
		})
	}
}

type feishuAPICall struct {
	method string
	path   string
	body   string
}

// newFeishuFakeAPI serves the cardkit and IM endpoints the streaming sender uses.
func newFeishuFakeAPI(t *testing.T, cardCreateCode int) (*httptest.Server, *[]feishuAPICall) {
	t.Helper()
	calls := &[]feishuAPICall{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, feishuAPICall{method: r.Method, path: r.URL.Path, body: string(body)})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "tenant_access_token"):
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok","tenant_access_token":"t-1","expire":7200}`))
		case r.URL.Path == "/open-apis/cardkit/v1/cards":
			if cardCreateCode != 0 {
				_, _ = fmt.Fprintf(w, `{"code":%d,"msg":"no permission"}`, cardCreateCode)
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"card_id":"card-1"}}`))
		case r.URL.Path == "/open-apis/im/v1/messages":
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"message_id":"om-1"}}`))
		case strings.HasSuffix(r.URL.Path, "/content"), strings.HasSuffix(r.URL.Path, "/settings"):
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"msg":"not found"}`))
		}
	}))
	t.Cleanup(server.Close)
	return server, calls
}

func newFeishuChannelAgainstFakeAPI(server *httptest.Server) *feishuChannel {
	ch := newFeishuChannel(feishuAccount{
		AccountID: "account-1",
		AppID:     "app-1",
		AppSecret: "secret-1",
		Stream:    true,
	})
	ch.rest = lark.NewClient(
		"app-1",
		"secret-1",
		lark.WithOpenBaseUrl(server.URL),
		lark.WithHttpClient(server.Client()),
	)
	return ch
}

func TestFeishuSendStreamCreatesUpdatesAndFinalizesCard(t *testing.T) {
	server, calls := newFeishuFakeAPI(t, 0)
	ch := newFeishuChannelAgainstFakeAPI(server)

	text := make(chan string, 4)
	text <- "Hel"
	text <- "Hello"
	close(text)

	if err := ch.SendStream(context.Background(), core.OutgoingMessage{ChatID: "chat-1"}, text); err != nil {
		t.Fatalf("SendStream returned error: %v", err)
	}

	var cardCreates, messageCreates, cardSettings int
	var sequences []int
	var contents []string
	lastContentAt, settingsAt := -1, -1
	var interactiveCardID string
	for callIndex, call := range *calls {
		switch {
		case call.path == "/open-apis/cardkit/v1/cards":
			cardCreates++
			var payload struct {
				Type string `json:"type"`
				Data string `json:"data"`
			}
			if err := json.Unmarshal([]byte(call.body), &payload); err != nil {
				t.Fatalf("card create body is not json: %v", err)
			}
			if payload.Type != "card_json" {
				t.Fatalf("card type = %q, want card_json", payload.Type)
			}
			var card struct {
				Schema string `json:"schema"`
				Config struct {
					StreamingMode bool `json:"streaming_mode"`
				} `json:"config"`
				Body struct {
					Elements []struct {
						Tag       string `json:"tag"`
						ElementID string `json:"element_id"`
					} `json:"elements"`
				} `json:"body"`
			}
			if err := json.Unmarshal([]byte(payload.Data), &card); err != nil {
				t.Fatalf("card data is not json: %v", err)
			}
			if card.Schema != "2.0" || !card.Config.StreamingMode {
				t.Fatalf("card data = %s, want a streaming card 2.0 json", payload.Data)
			}
			if len(card.Body.Elements) != 1 || card.Body.Elements[0].Tag != "markdown" || card.Body.Elements[0].ElementID != feishuStreamElementID {
				t.Fatalf("card elements = %+v, want one markdown element named %q", card.Body.Elements, feishuStreamElementID)
			}
		case call.path == "/open-apis/im/v1/messages":
			messageCreates++
			var payload struct {
				MsgType string `json:"msg_type"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(call.body), &payload); err != nil {
				t.Fatalf("message body is not json: %v", err)
			}
			if payload.MsgType != "interactive" {
				t.Fatalf("msg_type = %q, want interactive", payload.MsgType)
			}
			var content struct {
				Type string `json:"type"`
				Data struct {
					CardID string `json:"card_id"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(payload.Content), &content); err != nil {
				t.Fatalf("message content is not json: %v", err)
			}
			interactiveCardID = content.Data.CardID
		case strings.HasSuffix(call.path, "/content"):
			var payload struct {
				Content  string `json:"content"`
				Sequence int    `json:"sequence"`
			}
			if err := json.Unmarshal([]byte(call.body), &payload); err != nil {
				t.Fatalf("content body is not json: %v", err)
			}
			contents = append(contents, payload.Content)
			lastContentAt = callIndex
			sequences = append(sequences, payload.Sequence)
		case strings.HasSuffix(call.path, "/settings"):
			cardSettings++
			if !strings.Contains(call.body, `streaming_mode`) || !strings.Contains(call.body, "false") {
				t.Fatalf("settings body = %s, want streaming_mode false", call.body)
			}
			var payload struct {
				Sequence int `json:"sequence"`
			}
			if err := json.Unmarshal([]byte(call.body), &payload); err != nil {
				t.Fatalf("settings body is not json: %v", err)
			}
			settingsAt = callIndex
			sequences = append(sequences, payload.Sequence)
		}
	}

	if cardCreates != 1 {
		t.Fatalf("card creates = %d, want 1", cardCreates)
	}
	if messageCreates != 1 {
		t.Fatalf("message creates = %d, want 1", messageCreates)
	}
	if interactiveCardID != "card-1" {
		t.Fatalf("card message referenced %q, want card-1", interactiveCardID)
	}
	if len(sequences) == 0 {
		t.Fatal("no card update was sent")
	}
	for i := 1; i < len(sequences); i++ {
		if sequences[i] <= sequences[i-1] {
			t.Fatalf("sequences = %v, want strictly increasing", sequences)
		}
	}
	if cardSettings != 1 {
		t.Fatalf("card settings calls = %d, want 1", cardSettings)
	}
	shown := ""
	for _, content := range contents {
		if !strings.HasPrefix(content, shown) {
			t.Fatalf("content %q does not extend the previous content %q", content, shown)
		}
		shown = content
	}
	if shown != "Hello" {
		t.Fatalf("last streamed content = %q, want the complete answer", shown)
	}
	if settingsAt < lastContentAt {
		t.Fatalf("card was finalized at %d before the last content update at %d", settingsAt, lastContentAt)
	}
}

func TestNextStreamTextAdvancesByPrefix(t *testing.T) {
	for _, test := range []struct {
		name          string
		shown, target string
		step          int
		want          string
	}{
		{name: "empty shown advances by one step", shown: "", target: "abcdefgh", step: 2, want: "ab"},
		{name: "next slice advances by one step", shown: "ab", target: "abcdefgh", step: 2, want: "abcd"},
		{name: "large backlog still advances by one step", shown: "abcd", target: "abcdefghijklmnopqrst", step: 2, want: "abcdef"},
		{name: "drained target is returned whole", shown: "abcdefgh", target: "abcdefgh", step: 2, want: "abcdefgh"},
		{name: "short backlog is returned whole", shown: "abcdefg", target: "abcdefgh", step: 2, want: "abcdefgh"},
		{name: "non prefix target is returned unchanged", shown: "abcd", target: "ab", step: 2, want: "ab"},
		{name: "step below one advances a single rune", shown: "", target: "abc", step: 0, want: "a"},
		{name: "multi byte runes are never split", shown: "你好", target: "你好世界", step: 2, want: "你好世界"},
		{name: "multi byte runes advance by one step", shown: "你好", target: "你好世界再见", step: 2, want: "你好世界"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := nextStreamText(test.shown, test.target, test.step); got != test.want {
				t.Fatalf("nextStreamText(%q, %q, %d) = %q, want %q", test.shown, test.target, test.step, got, test.want)
			}
			if !strings.HasPrefix(test.target, test.shown) {
				return
			}
			if got := nextStreamText(test.shown, test.target, test.step); !strings.HasPrefix(got, test.shown) || !strings.HasPrefix(test.target, got) {
				t.Fatalf("nextStreamText(%q, %q, %d) = %q, want a prefix extension", test.shown, test.target, test.step, got)
			}
		})
	}
}

func TestFeishuSendStreamFailsWhenCardCreationFails(t *testing.T) {
	server, _ := newFeishuFakeAPI(t, 99991663)
	ch := newFeishuChannelAgainstFakeAPI(server)

	text := make(chan string, 2)
	text <- "Hello"
	close(text)

	if err := ch.SendStream(context.Background(), core.OutgoingMessage{ChatID: "chat-1"}, text); err == nil {
		t.Fatal("SendStream succeeded although the card could not be created")
	}
}

func TestFeishuSendStreamSkipsEmptyText(t *testing.T) {
	server, calls := newFeishuFakeAPI(t, 0)
	ch := newFeishuChannelAgainstFakeAPI(server)

	text := make(chan string)
	close(text)

	if err := ch.SendStream(context.Background(), core.OutgoingMessage{ChatID: "chat-1"}, text); !errors.Is(err, errFeishuNothingStreamed) {
		t.Fatalf("SendStream error = %v, want the nothing-streamed sentinel", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("SendStream called the API %d times for an empty answer", len(*calls))
	}
}
