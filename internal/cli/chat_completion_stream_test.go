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
//

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// completionStreamFixture encodes the native endpoint's public SSE envelope.
func completionStreamFixture(t testing.TB, frames ...any) string {
	t.Helper()
	var wire strings.Builder
	for _, data := range frames {
		payload, err := json.Marshal(map[string]any{"code": 0, "message": "", "data": data})
		if err != nil {
			t.Fatal(err)
		}
		wire.WriteString("data: ")
		wire.Write(payload)
		wire.WriteString("\n\n")
	}
	return wire.String()
}

// TestChatCompletionStreamAnswers checks visible text and complete response state.
func TestChatCompletionStreamAnswers(t *testing.T) {
	ref := map[string]any{"chunks": []map[string]any{{"chunk_id": "fixture_chunk", "content_with_weight": "source", "document_name": "fixture.md"}}}
	cases := []struct {
		name          string
		legacy        bool
		frames        []any
		answer        string
		output        string
		wantReference bool
	}{
		{"deltas_and_same_final", false, []any{map[string]any{"answer": "Hello "}, map[string]any{"answer": "world"}, map[string]any{"answer": "Hello world", "final": true}, true}, "Hello world", "Answer: Hello world\n", false},
		{"genuine_repeated_deltas", false, []any{map[string]any{"answer": "ha"}, map[string]any{"answer": "ha"}, map[string]any{"answer": "haha", "final": true}, true}, "haha", "Answer: haha\n", false},
		{"final_only", false, []any{map[string]any{"answer": "最终答案🙂", "final": true}, true}, "最终答案🙂", "Answer: 最终答案🙂\n", false},
		{"final_appends_suffix", false, []any{map[string]any{"answer": "Hello"}, map[string]any{"answer": "Hello!", "final": true}, true}, "Hello!", "Answer: Hello!\n", false},
		{"repeated_final", false, []any{map[string]any{"answer": "Hello", "final": true}, map[string]any{"answer": "Hello", "final": true}, true}, "Hello", "Answer: Hello\n", false},
		{"final_repairs_citations", false, []any{map[string]any{"answer": "First fact. Second fact."}, map[string]any{"answer": "First fact.[ID:0] Second fact.[ID:1]", "final": true, "reference": ref}, true}, "First fact.[ID:0] Second fact.[ID:1]", "Answer: First fact. Second fact.\nFinal answer: First fact.[ID:0] Second fact.[ID:1]\n", true},
		{"empty_final_preserves_answer", false, []any{map[string]any{"answer": "Hello"}, map[string]any{"answer": "", "final": true, "reference": ref}, true}, "Hello", "Answer: Hello\n", true},
		{"metadata_only", false, []any{map[string]any{"final": true, "reference": ref}, true}, "", "", true},
		{"unknown_fields", false, []any{map[string]any{"answer": "Hello", "future_field": map[string]any{"value": 1}}, true}, "Hello", "Answer: Hello\n", false},
		{"markdown_and_unicode", false, []any{map[string]any{"answer": "\n# 标题\n"}, map[string]any{"answer": "🙂\n```go\nx := 1\n```"}, true}, "\n# 标题\n🙂\n```go\nx := 1\n```", "Answer: \n# 标题\n🙂\n```go\nx := 1\n```\n", false},
		{"thinking_is_not_injected_twice", false, []any{map[string]any{"answer": "<think>checking", "start_to_think": true}, map[string]any{"answer": "</think>Hello", "end_to_think": true}, map[string]any{"answer": "Hello", "final": true}, true}, "Hello", "Answer: <think>checking</think>Hello\nFinal answer: Hello\n", false},
		{"legacy_cumulative", true, []any{map[string]any{"answer": "Hello "}, map[string]any{"answer": "Hello world"}, map[string]any{"answer": "Hello world", "reference": ref}, true}, "Hello world", "Answer: Hello world\n", true},
		{"legacy_repeated_snapshot", true, []any{map[string]any{"answer": "Hello"}, map[string]any{"answer": "Hello"}, true}, "Hello", "Answer: Hello\n", false},
		{"legacy_revision_at_completion", true, []any{map[string]any{"answer": "Wrong."}, map[string]any{"answer": "Correct"}, map[string]any{"answer": "Correct."}, true}, "Correct.", "Answer: Wrong.\nFinal answer: Correct.\n", false},
		{"legacy_recovers_prefix_growth", true, []any{map[string]any{"answer": "abc"}, map[string]any{"answer": "xyz"}, map[string]any{"answer": "abcdef"}, true}, "abcdef", "Answer: abcdef\n", false},
		{"legacy_explicit_final", true, []any{map[string]any{"answer": "Wrong."}, map[string]any{"answer": "Correct.", "final": true}, true}, "Correct.", "Answer: Wrong.\nFinal answer: Correct.\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			data, printed, err := consumeChatCompletionStream(strings.NewReader(completionStreamFixture(t, tc.frames...)), &output, tc.legacy)
			if err != nil {
				t.Fatal(err)
			}
			if data.Answer != tc.answer {
				t.Errorf("answer=%q, want %q", data.Answer, tc.answer)
			}
			if output.String() != tc.output {
				t.Errorf("stdout=%q, want %q", output.String(), tc.output)
			}
			if printed != (tc.answer != "") {
				t.Errorf("printed=%v", printed)
			}
			if tc.wantReference {
				expected, _ := json.Marshal(ref)
				if !bytes.Equal(data.Reference, expected) {
					t.Errorf("reference=%s, want %s", data.Reference, expected)
				}
			}
		})
	}
}

// TestChatCompletionStreamMetadata preserves identifiers across sparse events.
func TestChatCompletionStreamMetadata(t *testing.T) {
	frames := []any{
		map[string]any{"answer": "Hello", "id": "initial", "session_id": "session", "chat_id": "chat", "reference": map[string]any{"chunks": []any{}}},
		map[string]any{"final": true, "id": "final", "reference": map[string]any{"chunks": []map[string]any{{"chunk_id": "source"}}}},
		true,
	}
	data, _, err := consumeChatCompletionStream(strings.NewReader(completionStreamFixture(t, frames...)), io.Discard, false)
	if err != nil {
		t.Fatal(err)
	}
	if data.ID != "final" || data.SessionID != "session" || data.ChatID != "chat" {
		t.Errorf("identifiers=%+v", data)
	}
	if !strings.Contains(string(data.Reference), "source") {
		t.Errorf("final reference=%s", data.Reference)
	}
}

// TestChatCompletionStreamFraming handles SSE comments, multiline data and markers.
func TestChatCompletionStreamFraming(t *testing.T) {
	cases := []struct{ name, wire, output string }{
		{"multiline_and_crlf", ": heartbeat\r\nevent: message\r\nid: fixture\r\ndata: {\"code\":0,\r\ndata: \"data\":{\"answer\":\"Hello\"}}\r\n\r\ndata: [DONE]\r\n\r\n", "Answer: Hello\n"},
		{"empty_data", "data:\n\ndata: [DONE]\n\n", ""},
		{"native_done_no_blank_line", completionStreamFixture(t, map[string]any{"answer": "Hello"}) + "data: {\"code\":0,\"data\":true}", "Answer: Hello\n"},
		{"done_text_no_blank_line", completionStreamFixture(t, map[string]any{"answer": "Hello"}) + "data: [DONE]", "Answer: Hello\n"},
		{"ignores_data_after_done", completionStreamFixture(t, map[string]any{"answer": "Hello"}, true) + "data: broken\n\n", "Answer: Hello\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			_, _, err := consumeChatCompletionStream(strings.NewReader(tc.wire), &output, false)
			if err != nil {
				t.Fatal(err)
			}
			if output.String() != tc.output {
				t.Errorf("stdout=%q, want %q", output.String(), tc.output)
			}
		})
	}
}

// completionObservedWriter signals after the first answer text reaches the output.
type completionObservedWriter struct {
	bytes.Buffer
	observed chan struct{}
	once     sync.Once
}

func (w *completionObservedWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if strings.Contains(w.Buffer.String(), "Hello") {
		w.once.Do(func() { close(w.observed) })
	}
	return n, err
}

// WriteString observes writes that would otherwise use the embedded buffer directly.
func (w *completionObservedWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// TestChatCompletionStreamPrintsBeforeCompletion proves output without closing input.
func TestChatCompletionStreamPrintsBeforeCompletion(t *testing.T) {
	reader, sender := io.Pipe()
	t.Cleanup(func() { _ = reader.Close(); _ = sender.Close() })
	output := &completionObservedWriter{observed: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, _, err := consumeChatCompletionStream(reader, output, false)
		_ = reader.Close()
		done <- err
	}()
	first := completionStreamFixture(t, map[string]any{"answer": "Hello"})
	go func() { _, _ = io.WriteString(sender, first) }()
	select {
	case <-output.observed:
		if output.String() != "Answer: Hello" {
			t.Fatalf("first output=%q", output.String())
		}
	case err := <-done:
		t.Fatalf("stream finished before its first answer: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("answer was not displayed before completion")
	}
	if _, err := io.WriteString(sender, completionStreamFixture(t, true)); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("completion marker did not stop the still-open input")
	}
	if output.String() != "Answer: Hello\n" {
		t.Errorf("stdout=%q", output.String())
	}
}

// TestChatCompletionStreamErrors preserves partial output while reporting failures.
func TestChatCompletionStreamErrors(t *testing.T) {
	prefix := completionStreamFixture(t, map[string]any{"answer": "partial"})
	cases := []struct {
		name, wire, want, output string
		unexpectedEOF            bool
	}{
		{"invalid_json", "data: {broken}\n\n", "invalid SSE event", "", false},
		{"invalid_answer", "data: {\"code\":0,\"data\":{\"answer\":17}}\n\n", "invalid SSE answer", "", false},
		{"business_error", "data: {\"code\":7,\"message\":\"denied\",\"data\":{\"answer\":\"do not print\"}}\n\n", "code 7: denied", "", false},
		{"data_null", "data: {\"code\":0,\"data\":null}\n\n", "expected an answer object or true", "", false},
		{"data_array", "data: {\"code\":0,\"data\":[]}\n\n", "expected an answer object or true", "", false},
		{"data_false", "data: {\"code\":0,\"data\":false}\n\n", "expected an answer object or true", "", false},
		{"missing_data", "data: {\"code\":0}\n\n", "expected an answer object or true", "", false},
		{"unexpected_eof", prefix, "completion marker", "Answer: partial\n", true},
		{"json_error_after_answer", prefix + "data: broken\n\n", "invalid SSE event", "Answer: partial\n", false},
		{"business_error_after_answer", prefix + "data: {\"code\":500,\"message\":\"failed\",\"data\":{\"answer\":\"not content\"}}\n\n", "code 500: failed", "Answer: partial\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			_, _, err := consumeChatCompletionStream(strings.NewReader(tc.wire), &output, false)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
			if tc.unexpectedEOF && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Errorf("error identity=%v", err)
			}
			if output.String() != tc.output {
				t.Errorf("stdout=%q, want %q", output.String(), tc.output)
			}
		})
	}
}

type completionErrorReader struct {
	io.Reader
	err error
}

func (r completionErrorReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		return n, r.err
	}
	return n, err
}

type completionErrorWriter struct {
	err   error
	short bool
}

func (w completionErrorWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, w.err
}

// TestChatCompletionStreamIOErrors keeps underlying read and write error identities.
func TestChatCompletionStreamIOErrors(t *testing.T) {
	t.Run("reader", func(t *testing.T) {
		failure := errors.New("reader interrupted")
		input := completionErrorReader{strings.NewReader(completionStreamFixture(t, map[string]any{"answer": "partial"})), failure}
		var output bytes.Buffer
		_, _, err := consumeChatCompletionStream(input, &output, false)
		if !errors.Is(err, failure) {
			t.Errorf("read error=%v", err)
		}
		if output.String() != "Answer: partial\n" {
			t.Errorf("partial output=%q", output.String())
		}
	})
	for _, short := range []bool{false, true} {
		name := "writer"
		failure := error(io.ErrClosedPipe)
		if short {
			name, failure = "short_writer", io.ErrShortWrite
		}
		t.Run(name, func(t *testing.T) {
			input := strings.NewReader(completionStreamFixture(t, map[string]any{"answer": "Hello"}, true))
			_, _, err := consumeChatCompletionStream(input, completionErrorWriter{err: io.ErrClosedPipe, short: short}, false)
			if !errors.Is(err, failure) {
				t.Errorf("write error=%v", err)
			}
		})
	}
}

// TestChatCompletionStreamLargeEvents covers both long lines and total event limits.
func TestChatCompletionStreamLargeEvents(t *testing.T) {
	t.Run("above_default_scanner_limit", func(t *testing.T) {
		answer := strings.Repeat("界", 30000) + "🙂"
		var output bytes.Buffer
		data, _, err := consumeChatCompletionStream(strings.NewReader(completionStreamFixture(t, map[string]any{"answer": answer, "final": true}, true)), &output, false)
		if err != nil {
			t.Fatal(err)
		}
		if data.Answer != answer || output.String() != "Answer: "+answer+"\n" {
			t.Fatal("long answer was truncated")
		}
	})
	t.Run("overlong_line", func(t *testing.T) {
		input := completionStreamFixture(t, map[string]any{"answer": strings.Repeat("x", 2*1024*1024)}, true)
		_, _, err := consumeChatCompletionStream(strings.NewReader(input), io.Discard, false)
		if err == nil || !strings.Contains(err.Error(), "token too long") {
			t.Errorf("overlong line error=%v", err)
		}
	})
	t.Run("oversized_multiline_event", func(t *testing.T) {
		input := strings.Repeat("data: "+strings.Repeat("x", 8192)+"\n", 129) + "\n"
		_, _, err := consumeChatCompletionStream(strings.NewReader(input), io.Discard, false)
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Errorf("event size error=%v", err)
		}
	})
}

// captureCompletionOutput captures the CLI's existing stdout boundary, without a pipe limit.
func captureCompletionOutput(t *testing.T, run func()) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = file
	t.Cleanup(func() { os.Stdout = previous; _ = file.Close() })
	run()
	os.Stdout = previous
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}

// TestChatCompletionsCommandOutput exercises parsing, HTTP, streaming and final printing.
func TestChatCompletionsCommandOutput(t *testing.T) {
	for _, apiKey := range []bool{false, true} {
		for _, mode := range []string{"default", "nonstream", "stream", "legacy", "error"} {
			name := mode + "_login"
			if apiKey {
				name = mode + "_api_key"
			}
			t.Run(name, func(t *testing.T) {
				stream := mode == "stream" || mode == "legacy" || mode == "error"
				client := newTestHTTPClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost || r.URL.Path != "/api/v1/chat/completions" {
						t.Errorf("request=%s %s", r.Method, r.URL.Path)
					}
					auth := "fixture-only"
					if apiKey {
						auth = "Bearer " + auth
					}
					if r.Header.Get("Authorization") != auth {
						t.Errorf("authorization=%q", r.Header.Get("Authorization"))
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["stream"] != stream || body["question"] != "fixture question" || body["chat_id"] != "fixture_chat" {
						t.Errorf("body=%v", body)
					}
					if mode == "legacy" && body["legacy"] != true {
						t.Error("legacy flag was not sent")
					}
					if !stream {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, "{\"code\":0,\"data\":{\"answer\":\"Hello world\"}}")
						return
					}
					if r.Header.Get("Accept") != "text/event-stream" {
						t.Error("missing SSE Accept header")
					}
					w.Header().Set("Content-Type", "text/event-stream")
					second := "world"
					if mode == "legacy" {
						second = "Hello world"
					}
					wire := completionStreamFixture(t, map[string]any{"answer": "Hello "}, map[string]any{"answer": second})
					if mode == "error" {
						wire += "data: {\"code\":500,\"message\":\"failed\",\"data\":null}\n\n"
					} else {
						wire += completionStreamFixture(t, map[string]any{"answer": "Hello world", "final": true, "reference": map[string]any{"chunks": []map[string]any{{"chunk_id": "fixture_chunk", "content": "source"}}}}, true)
					}
					_, _ = io.WriteString(w, wire)
				}))
				token := "fixture-only"
				if apiKey {
					client.APIKey, client.useAPIKey = &token, true
				} else {
					client.LoginToken = &token
				}
				c := &CLI{Config: &CommandLineConfig{CLIMode: APIMode, APIClientConfig: APIModeConfig{CurrentAPIServer: DefaultAPIServer}}, APIServerClientMap: map[string]*HTTPClient{DefaultAPIServer: client}}
				command := `CHAT COMPLETIONS "fixture question" chat_id "fixture_chat"`
				if mode == "nonstream" {
					command += " stream false"
				}
				if stream {
					command += " stream true"
				}
				if mode == "legacy" {
					command += " legacy true"
				}
				var executeErr error
				output := captureCompletionOutput(t, func() { executeErr = c.execute(command + ";") })
				if mode == "error" {
					if executeErr == nil || !strings.Contains(executeErr.Error(), "code 500") {
						t.Errorf("command error=%v", executeErr)
					}
					if output != "Answer: Hello world\n" {
						t.Errorf("partial stdout=%q", output)
					}
					return
				}
				if executeErr != nil {
					t.Fatal(executeErr)
				}
				if !strings.HasPrefix(output, "Answer: Hello world\n") || strings.Count(output, "Hello world") != 1 || strings.Count(output, "Time:") != 1 {
					t.Errorf("stdout=%q", output)
				}
				if stream && (strings.Count(output, "Reference:") != 1 || !strings.Contains(output, "fixture_chunk")) {
					t.Errorf("final reference missing: %q", output)
				}
			})
		}
	}
}

type completionRoundTripper func(*http.Request) (*http.Response, error)

func (f completionRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type completionClosingBody struct {
	io.Reader
	closed bool
}

func (b *completionClosingBody) Close() error { b.closed = true; return nil }

// TestChatCompletionsClosesStreamBody verifies resource release on success and errors.
func TestChatCompletionsClosesStreamBody(t *testing.T) {
	for _, failure := range []bool{false, true} {
		name := "completion"
		if failure {
			name = "invalid_json"
		}
		t.Run(name, func(t *testing.T) {
			wire := completionStreamFixture(t, map[string]any{"answer": "Hello"}, true)
			if failure {
				wire = "data: broken\n\n"
			}
			body := &completionClosingBody{Reader: strings.NewReader(wire)}
			client := NewHTTPClient()
			client.client = &http.Client{Transport: completionRoundTripper(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: r}, nil
			})}
			token := "fixture-only"
			client.LoginToken = &token
			c := &CLI{Config: &CommandLineConfig{CLIMode: APIMode, APIClientConfig: APIModeConfig{CurrentAPIServer: DefaultAPIServer}}, APIServerClientMap: map[string]*HTTPClient{DefaultAPIServer: client}}
			captureCompletionOutput(t, func() {
				_, err := c.ChatCompletions(1, &Command{Params: map[string]any{"question": "fixture question", "stream": true}})
				if (err != nil) != failure {
					t.Errorf("command error=%v", err)
				}
			})
			if !body.closed {
				t.Error("response body was not closed")
			}
		})
	}
}

// FuzzChatCompletionStream checks malformed input and the successful-output invariant.
func FuzzChatCompletionStream(f *testing.F) {
	f.Add("data: {\"code\":0,\"data\":{\"answer\":\"Hello\"}}\n\ndata: {\"code\":0,\"data\":true}\n\n", false)
	f.Add("data: {\"code\":0,\"data\":{\"answer\":\"Hello\"}}\n\ndata: {\"code\":0,\"data\":{\"answer\":\"Hello world\"}}\n\ndata: [DONE]\n\n", true)
	f.Add("data: broken\n\n", false)
	f.Fuzz(func(t *testing.T, wire string, legacy bool) {
		var output bytes.Buffer
		data, printed, err := consumeChatCompletionStream(strings.NewReader(wire), &output, legacy)
		if err == nil && data.Answer != "" && !printed {
			t.Fatal("completed answer was not displayed")
		}
		if printed && !strings.HasPrefix(output.String(), "Answer: ") {
			t.Fatalf("printed answer has no label: %q", output.String())
		}
	})
}

// BenchmarkConsumeChatCompletionStream measures decoding and rendering without HTTP overhead.
func BenchmarkConsumeChatCompletionStream(b *testing.B) {
	for _, frames := range []int{128, 1024, 4096} {
		b.Run(fmt.Sprintf("chunks_%d", frames), func(b *testing.B) {
			var wire strings.Builder
			chunk := completionStreamFixture(b, map[string]any{"answer": strings.Repeat("x", 32)})
			for range frames {
				wire.WriteString(chunk)
			}
			wire.WriteString(completionStreamFixture(b, true))
			payload := wire.String()
			b.ReportAllocs()
			b.SetBytes(int64(frames * 32))
			b.ResetTimer()
			for range b.N {
				data, _, err := consumeChatCompletionStream(strings.NewReader(payload), io.Discard, false)
				if err != nil || len(data.Answer) != frames*32 {
					b.Fatalf("answer bytes=%d, error=%v", len(data.Answer), err)
				}
			}
		})
	}
}
