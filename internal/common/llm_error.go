// Package common — typed error that attributes a chat-model failure to the
// user's configured LLM (provider rejection or unusable model config) so
// user-visible task detail can say "your model service failed" instead of
// surfacing a raw internal error chain that reads like a RAGFlow defect.
package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// LLMErrorKind distinguishes the two user-attributable failure sources.
type LLMErrorKind string

const (
	// LLMErrorProvider: the request reached (or tried to reach) the model
	// service and the service failed or rejected it — quota, auth, 4xx/5xx,
	// content filter, timeout.
	LLMErrorProvider LLMErrorKind = "provider"
	// LLMErrorConfig: the configured chat model could not be resolved to a
	// usable target — unknown/removed model id, missing provider, bad
	// driver configuration. The user must fix the model settings.
	LLMErrorConfig LLMErrorKind = "config"
)

// maxUserReasonLen bounds the upstream error text echoed into the
// user-facing message: provider bodies can be large HTML/JSON payloads.
const maxUserReasonLen = 300

// LLMError wraps an error whose root cause lies in the tenant's chat model
// (provider service or model configuration), not in RAGFlow internals.
// Wrap at the LLM call boundary; classify at the top with errors.As.
type LLMError struct {
	Kind       LLMErrorKind
	Model      string // model name as configured (bare or composite)
	Provider   string // provider/driver key, may be empty when unresolved
	StatusCode int    // embedded HTTP status, 0 when not recognized
	Err        error  // underlying error, kept verbatim for server-side logs
}

// NewLLMProviderError attributes err to the model service call.
func NewLLMProviderError(provider, model string, err error) *LLMError {
	return &LLMError{Kind: LLMErrorProvider, Model: model, Provider: provider, StatusCode: extractHTTPStatus(err), Err: err}
}

// NewLLMConfigError attributes err to unusable model configuration.
func NewLLMConfigError(provider, model string, err error) *LLMError {
	return &LLMError{Kind: LLMErrorConfig, Model: model, Provider: provider, Err: err}
}

func (e *LLMError) Error() string {
	var b strings.Builder
	b.WriteString("llm ")
	b.WriteString(string(e.Kind))
	b.WriteString(" error")
	if e.Provider != "" {
		fmt.Fprintf(&b, " [provider=%s", e.Provider)
		if e.Model != "" {
			fmt.Fprintf(&b, " model=%s", e.Model)
		}
		b.WriteString("]")
	} else if e.Model != "" {
		fmt.Fprintf(&b, " [model=%s]", e.Model)
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

func (e *LLMError) Unwrap() error { return e.Err }

// UserSummary renders the factual part of the attribution — which model
// failed and why — without the trailing guidance sentence. The per-stage
// step log uses it so the guidance appears once, in the terminal task
// detail (UserMessage), instead of repeating on every line.
func (e *LLMError) UserSummary() string {
	model := e.Model
	if model == "" {
		model = "unknown"
	}
	provider := strings.TrimSpace(e.Provider)
	reason := "no response"
	if e.Err != nil {
		reason = truncateForUser(refineReason(e.Err.Error()))
	}
	label := model
	if provider != "" {
		label = model + "@" + provider
	}
	switch e.Kind {
	case LLMErrorConfig:
		return fmt.Sprintf("Chat model %q is not usable: %s", label, reason)
	default:
		return fmt.Sprintf("LLM call to %q failed: %s", label, reason)
	}
}

// UserMessage renders the short, self-attributing line shown in the
// task detail on the front end. The full error chain stays in the
// server-side logs.
func (e *LLMError) UserMessage() string {
	advice := "This error was returned by your model service, not RAGFlow — please check the model's API key, quota and service status, then retry."
	if e.Kind == LLMErrorConfig {
		advice = "Please check the model configuration in Model Providers settings — this is a model setup issue, not a RAGFlow error."
	}
	summary := e.UserSummary()
	if strings.HasSuffix(summary, ".") {
		return summary + " " + advice
	}
	return summary + ". " + advice
}

// userStatusRES matches the HTTP status formats produced by the model
// drivers ("API request failed with status 429: ...", "status code: 429")
// and by provider SDK messages ("API error: 429 Too Many Requests").
var userStatusRES = []*regexp.Regexp{
	regexp.MustCompile(`(?i)status code:\s*(\d{3})\b`),
	regexp.MustCompile(`(?i)\bstatus:?\s*(\d{3})\b`),
	regexp.MustCompile(`(?i)\bAPI error:?\s*(\d{3})\b`),
}

// extractHTTPStatus parses an embedded HTTP status code out of a provider
// error message. Returns 0 when no status is recognizable.
func extractHTTPStatus(err error) int {
	if err == nil {
		return 0
	}
	msg := err.Error()
	for _, re := range userStatusRES {
		if m := re.FindStringSubmatch(msg); m != nil {
			code := 0
			for _, r := range m[1] {
				code = code*10 + int(r-'0')
			}
			if code >= 100 && code <= 599 {
				return code
			}
		}
	}
	return 0
}

// truncateForUser collapses whitespace, redacts credential-shaped values and
// caps length (never splitting a multi-byte rune) so a provider error body —
// often JSON/HTML, occasionally echoing request URLs or keys — fits in a
// single safe, readable detail line.
func truncateForUser(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = RedactCredentials(s)
	if len(s) > maxUserReasonLen {
		n := maxUserReasonLen
		for n > 0 && !utf8.RuneStart(s[n]) {
			n--
		}
		s = s[:n] + "..."
	}
	return s
}

// errorAuthorizationRE matches scheme-prefixed authorization values
// ("authorization: Bearer <token>") whose token part the generic assignment
// pattern would miss: that pattern stops at the first whitespace and would
// redact only the scheme word.
var errorAuthorizationRE = regexp.MustCompile(`(?i)(authorization)\s*["']?\s*[:=]\s*["']?(?:bearer|basic)\s+[^,\s}"']+`)

// errorCredentialRE matches credential-valued assignments ("api-key: sk-...",
// "password=...", ...) keeping the field name so readers see what was hidden.
var errorCredentialRE = regexp.MustCompile(`(?i)(api[-_ ]?key|access[-_ ]?token|authorization|password|secret)\s*["']?\s*[:=]\s*["']?[^,\s}"']+`)

// errorAPIKeyRE matches bare API-key-shaped tokens.
var errorAPIKeyRE = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]+`)

// RedactCredentials removes credential-shaped values from text before it is
// surfaced to users (provider errors routinely echo request URLs and keys).
// The scheme-prefixed authorization pattern runs first so Bearer/Basic tokens
// (which the generic pattern would cut at the scheme's trailing space) are
// removed whole.
func RedactCredentials(s string) string {
	s = errorAuthorizationRE.ReplaceAllString(s, "$1=[REDACTED]")
	s = errorCredentialRE.ReplaceAllString(s, "$1=[REDACTED]")
	return errorAPIKeyRE.ReplaceAllString(s, "[REDACTED]")
}

// refineReason replaces an embedded provider JSON error body with the
// human-readable message it carries: users read
// "503 Service Unavailable, body: {\"error\":{\"message\":\"No available
// channel...\"}}" as gibberish, while "503 Service Unavailable: No available
// channel..." states the actual problem. The raw body stays in server logs
// via Error()/Unwrap; only the user message is refined. Candidate JSON
// objects are located by their opening brace and decoded independently of
// key order, whitespace, or trailing text; text without a parseable body
// (or without a message inside it) is returned as-is.
func refineReason(s string) string {
	for i := 0; i < len(s); {
		start := strings.IndexByte(s[i:], '{')
		if start < 0 {
			break
		}
		start += i
		var payload struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(strings.NewReader(s[start:])).Decode(&payload); err == nil {
			message := payload.Error.Message
			if message == "" {
				message = payload.Message
			}
			if message != "" {
				prefix := strings.TrimRight(strings.TrimSuffix(strings.TrimSpace(s[:start]), "body:"), " :,:")
				if prefix == "" {
					return message
				}
				return prefix + ": " + message
			}
		}
		i = start + 1
	}
	return s
}

// AsLLMError finds the first LLMError in err's chain.
func AsLLMError(err error) (*LLMError, bool) {
	var le *LLMError
	if errors.As(err, &le) {
		return le, true
	}
	return nil, false
}
