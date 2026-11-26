package common

import (
	"encoding/json/v2"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// AgentEvent records one execution lifecycle transition, independently of its client stream.
type AgentEvent struct {
	EventID           string    `json:"event_id"`
	EventAt           time.Time `json:"event_at"`
	Sequence          uint64    `json:"sequence"`
	EventType         string    `json:"event_type"`
	TenantID          string    `json:"tenant_id"`
	UserID            string    `json:"user_id"`
	AgentID           string    `json:"agent_id"`
	AgentVersion      string    `json:"agent_version"`
	SessionID         string    `json:"session_id"`
	MessageID         string    `json:"message_id"`
	RunID             string    `json:"run_id"`
	NodeID            string    `json:"node_id"`
	NodeName          string    `json:"node_name"`
	NodeType          string    `json:"node_type"`
	ExecutionID       string    `json:"execution_id"`
	ParentExecutionID string    `json:"parent_execution_id"`
	Attempt           uint32    `json:"attempt"`
	Status            string    `json:"status"`
	DurationMS        int64     `json:"duration_ms"`
	InputsJSON        string    `json:"inputs_json"`
	OutputsJSON       string    `json:"outputs_json"`
	ErrorCode         string    `json:"error_code"`
	ErrorMessage      string    `json:"error_message"`
	PayloadTruncated  bool      `json:"payload_truncated"`
}

// AgentEventSnapshot makes a bounded copy without retaining binary data, streams,
// credentials or the graph's internal state. It never mutates component inputs.
func AgentEventSnapshot(value any) (string, bool) {
	truncated := false
	remaining := 256
	remainingBytes := 12000
	var copyValue func(any, int) any
	copyValue = func(value any, depth int) any {
		remaining--
		if remaining < 0 || depth > 8 || remainingBytes <= 0 {
			truncated = true
			return "[truncated]"
		}
		switch value := value.(type) {
		case nil, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return value
		case string:
			if strings.HasPrefix(value, "data:") {
				truncated = true
				return "[binary omitted]"
			}
			// Parse only bounded JSON strings: HTTP nodes return response bodies as
			// strings, including token endpoint JSON. Do not retain embedded secrets.
			if len(value) <= 16384 && (strings.HasPrefix(strings.TrimSpace(value), "{") || strings.HasPrefix(strings.TrimSpace(value), "[")) {
				var decoded any
				if json.Unmarshal([]byte(value), &decoded) == nil {
					return copyValue(decoded, depth+1)
				}
			}
			limit := min(4096, remainingBytes)
			if len(value) > limit {
				truncated = true
				value = strings.ToValidUTF8(value[:limit], "")
			}
			remainingBytes -= len(value)
			return RedactAgentEventText(value)
		case map[string]map[string]any:
			out := make(map[string]any, min(len(value), 64))
			for key, child := range value {
				if len(out) >= 64 {
					truncated = true
					break
				}
				out[key] = child
			}
			return copyValue(out, depth+1)
		case map[string]any:
			out := make(map[string]any, min(len(value), 64))
			for key, child := range value {
				if len(key) > 256 {
					truncated = true
					key = strings.ToValidUTF8(key[:256], "")
				}
				remainingBytes -= len(key)
				if strings.HasPrefix(key, "__") || key == "state" {
					continue
				}
				if len(out) >= 64 || remaining < 0 {
					truncated = true
					break
				}
				if isSensitiveAgentEventKey(key) {
					out[key] = redactedLogValue
				} else {
					out[key] = copyValue(child, depth+1)
				}
			}
			return out
		case []map[string]any:
			out := make([]any, 0, min(len(value), 64))
			for i, child := range value {
				if i >= 64 {
					truncated = true
					break
				}
				out = append(out, child)
			}
			return copyValue(out, depth+1)
		case []any:
			out := make([]any, 0, min(len(value), 64))
			for i, child := range value {
				if i >= 64 || remaining < 0 {
					truncated = true
					break
				}
				out = append(out, copyValue(child, depth+1))
			}
			return out
		case []string:
			out := make([]any, 0, min(len(value), 64))
			for i, child := range value {
				if i >= 64 || remaining < 0 {
					truncated = true
					break
				}
				out = append(out, copyValue(child, depth+1))
			}
			return out
		default:
			truncated = true
			return "[non-JSON value omitted]"
		}
	}
	safe := copyValue(value, 0)
	encoded, err := json.Marshal(safe)
	if err != nil || len(encoded) > 16384 {
		return `{"truncated":true}`, true
	}
	return string(encoded), truncated
}

var agentEventCredentials = regexp.MustCompile(`(?i)(\b(?:api[_-]?key|password|secret|authorization|access[_-]?token|refresh[_-]?token|credential)\b["']?\s*[=:]\s*)(?:"[^"]*"|'[^']*'|[^\s,;&]+)`)
var agentEventAuthorization = regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[^\s,"'<>]+`)
var agentEventURLs = regexp.MustCompile(`https?://[^\s"'<>]+`)

// RedactAgentEventText masks recognizable credentials in diagnostic strings.
// Structured snapshots additionally redact sensitive field names recursively.
func RedactAgentEventText(text string) string {
	text = agentEventAuthorization.ReplaceAllString(text, "$1 [REDACTED]")
	text = agentEventCredentials.ReplaceAllString(text, "${1}[REDACTED]")
	return agentEventURLs.ReplaceAllStringFunc(text, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "[invalid URL omitted]"
		}
		// Signed download URLs are bearer credentials regardless of their
		// provider-specific parameter names. Keep only the resource location.
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		parsed.RawFragment = ""
		return parsed.String()
	})
}

func isSensitiveAgentEventKey(key string) bool {
	normalized := strings.NewReplacer("-", "", "_", "", ".", "").Replace(strings.ToLower(key))
	return isSensitiveLogKey(key) || strings.HasSuffix(normalized, "apikey") || strings.HasSuffix(normalized, "token") || normalized == "proxyauthorization" || normalized == "cookie" || normalized == "setcookie" || strings.Contains(normalized, "credential") || strings.Contains(normalized, "password") || strings.Contains(normalized, "secret")
}
