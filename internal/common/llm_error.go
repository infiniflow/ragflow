// Package common — typed error that attributes a chat-model failure to the
// user's configured LLM (provider rejection or unusable model config) so
// user-visible task detail can say "your model service failed" instead of
// surfacing a raw internal error chain that reads like a RAGFlow defect.
package common

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
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
	return &LLMError{Kind: LLMErrorProvider, Model: model, Provider: provider, StatusCode: ExtractHTTPStatus(err), Err: err}
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

// UserMessage renders the short, self-attributing line shown in the
// task detail on the front end. The full error chain stays in the
// server-side logs.
func (e *LLMError) UserMessage() string {
	model := e.Model
	if model == "" {
		model = "unknown"
	}
	provider := strings.TrimSpace(e.Provider)
	reason := "no response"
	if e.Err != nil {
		reason = TruncateForUser(e.Err.Error())
	}
	label := model
	if provider != "" {
		label = model + "@" + provider
	}
	switch e.Kind {
	case LLMErrorConfig:
		return fmt.Sprintf("Chat model %q is not usable: %s. Please check the model configuration in Model Providers settings — this is a model setup issue, not a RAGFlow error.", label, reason)
	default:
		return fmt.Sprintf("LLM call to %q failed: %s. This error was returned by your model service, not RAGFlow — please check the model's API key, quota and service status, then retry.", label, reason)
	}
}

// userStatusRES matches the HTTP status formats produced by the model
// drivers ("API request failed with status 429: ...", "status code: 429")
// and by provider SDK messages ("API error: 429 Too Many Requests").
var userStatusRES = []*regexp.Regexp{
	regexp.MustCompile(`(?i)status code:\s*(\d{3})\b`),
	regexp.MustCompile(`(?i)\bstatus:?\s*(\d{3})\b`),
	regexp.MustCompile(`(?i)\bAPI error:?\s*(\d{3})\b`),
}

// ExtractHTTPStatus parses an embedded HTTP status code out of a provider
// error message. Returns 0 when no status is recognizable.
func ExtractHTTPStatus(err error) int {
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

// TruncateForUser collapses whitespace and caps length so a provider error
// body (often JSON/HTML) fits in a single readable detail line.
func TruncateForUser(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxUserReasonLen {
		s = s[:maxUserReasonLen] + "..."
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
