package api

import (
	"bufio"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"
)

type sseEvent struct{ Event, Data string }

// parseSSE handles comments, CRLF, multiline data, and explicit event boundaries.
func parseSSE(r io.Reader) ([]sseEvent, error) {
	limited := &io.LimitedReader{R: r, N: 4<<20 + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var events []sseEvent
	var event sseEvent
	var data []string
	total := 0
	for scanner.Scan() {
		line := scanner.Text()
		total += len(line) + 1
		if total > 4<<20 {
			return nil, fmt.Errorf("SSE exceeds 4 MiB")
		}
		if line == "" {
			if len(data) > 0 {
				event.Data = strings.Join(data, "\n")
				events = append(events, event)
			}
			event = sseEvent{}
			data = nil
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		key, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch key {
		case "event":
			event.Event = value
		case "data":
			data = append(data, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if limited.N == 0 {
		return nil, fmt.Errorf("SSE exceeds 4 MiB")
	}
	if len(data) > 0 {
		return nil, fmt.Errorf("unterminated SSE event")
	}
	return events, nil
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "-", "_")
	for _, part := range []string{"token", "authorization", "password", "secret", "api_key", "apikey", "credential"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func redactValue(v any) any {
	switch value := v.(type) {
	case map[string]any:
		for key, item := range value {
			if sensitiveKey(key) {
				value[key] = "[REDACTED]"
			} else {
				value[key] = redactValue(item)
			}
		}
	case []any:
		for i, item := range value {
			value[i] = redactValue(item)
		}
	}
	return v
}

func diagnosticBody(body []byte, secrets ...string) string {
	var value any
	var text string
	if json.Unmarshal(body, &value) == nil {
		b, err := json.Marshal(redactValue(value))
		if err == nil {
			text = string(b)
		}
	} else {
		text = "[non-JSON body omitted]"
	}
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	if len(text) > 4096 {
		text = text[:4096] + "...[truncated]"
	}
	return text
}

func diagnosticURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[invalid URL]"
	}
	u.User = nil
	q := u.Query()
	for key := range q {
		if sensitiveKey(key) {
			q.Set(key, "[REDACTED]")
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func object(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T", value)
	}
	return result
}
func array(t *testing.T, value any) []any {
	t.Helper()
	result, ok := value.([]any)
	if !ok {
		t.Fatalf("expected array, got %T", value)
	}
	return result
}

func TestRESTHelperSSE(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		count       int
		bad         bool
	}{
		{"multiline", ": heartbeat\r\nevent: message\r\ndata: first\r\ndata:second\r\n\r\ndata: [DONE]\n\n", 2, false},
		{"missing boundary", "data: [DONE]\n", 0, true},
		{"oversized event", "data: " + strings.Repeat("x", 1<<20) + "\n\n", 0, true},
		{"empty", "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events, err := parseSSE(strings.NewReader(tc.input))
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v", err)
			}
			if len(events) != tc.count {
				t.Fatalf("events=%d", len(events))
			}
			if tc.name == "multiline" && (events[0].Event != "message" || events[0].Data != "first\nsecond" || events[1].Data != "[DONE]") {
				t.Fatalf("events=%v", events)
			}
		})
	}
}

func TestRESTHelperRedaction(t *testing.T) {
	body := diagnosticBody([]byte(`{"password":"p","nested":{"api_key":"k","X-API-Key":"hyphen-secret"},"content":"actual-token","items":[{"authorization":"a"}]}`), "actual-token")
	for _, secret := range []string{`"p"`, `"k"`, `"a"`, "hyphen-secret", "actual-token"} {
		if strings.Contains(body, secret) {
			t.Fatalf("secret leaked: %s", body)
		}
	}
	if strings.Contains(diagnosticURL("https://user:pass@host/chats?token=secret&page=2"), "secret") {
		t.Fatal("URL secret leaked")
	}
	if strings.Contains(diagnosticBody([]byte("token=secret")), "secret") {
		t.Fatal("raw body leaked")
	}
}
