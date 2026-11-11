package models

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"
)

// platformNetworkError reports a transport failure of a platform model without
// the request URL or addresses, which may carry shared credentials. It keeps
// the timeout flag and the underlying errno so retry policies behave as they do
// for tenant models.
type platformNetworkError struct {
	timeout bool
	cause   error
}

func (e platformNetworkError) Error() string {
	msg := "platform model network request failed"
	if e.cause != nil {
		msg += ": " + e.cause.Error()
	}
	return msg
}
func (e platformNetworkError) Timeout() bool   { return e.timeout }
func (e platformNetworkError) Temporary() bool { return false }
func (e platformNetworkError) Unwrap() error   { return e.cause }

// platformSafeDriver prevents provider errors from exposing shared credentials to tenants.
type platformSafeDriver struct{ ModelDriver }

// NewPlatformSafeDriver hides untrusted provider errors for shared models.
func NewPlatformSafeDriver(driver ModelDriver) ModelDriver {
	switch restricted := driver.(type) {
	case *OpenAIModel:
		driver = restricted.platformRestrictedClone()
	case *VllmModel:
		driver = restricted.platformRestrictedClone()
	case *OpenAIAPICompatibleModel:
		driver = restricted.platformRestrictedClone()
	}
	return platformSafeDriver{driver}
}

// PlatformDriverBase exposes the underlying driver for capability inspection only.
func PlatformDriverBase(driver ModelDriver) (ModelDriver, bool) {
	if safe, ok := driver.(platformSafeDriver); ok {
		return safe.ModelDriver, true
	}
	return driver, false
}

// The sanitized text keeps a status only when the provider wrote it as one:
// a known HTTP status directly after or before status wording ("error: 429",
// "HTTP 500", "status 503", "503 error"), standing alone and outside URLs.
// Classifiers read the original text (common.ErrorText), so this only keeps
// the sanitized message informative; it must not copy other provider digits.
var (
	platformStatusBefore = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_])(?:http|status|code|error)[\s:=]{0,3}(\d{3})(?:[^a-z0-9_.@-]|$)`)
	platformStatusAfter  = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_.@:/-])(\d{3})[\s:]{1,3}(?:http|status|error)(?:[^a-z0-9_]|$)`)
	platformURL          = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://\S*`)
	platformRetryAfter   = regexp.MustCompile(`(?i)retry[-_ ]?after["'\s:=]+[0-9]{1,4}(?:\.[0-9]{1,3})?`)
	platformClassTokens  = regexp.MustCompile(`(?i)status\s+[1-5][0-9]{2}|body:|\b(?:400|413|422|429|498|500|503|529|20015)\b`)
)

// platformHTTPStatuses are the statuses the sanitized text may name.
var platformHTTPStatuses = map[string]bool{}

func init() {
	for code := 100; code < 600; code++ {
		if http.StatusText(code) != "" {
			platformHTTPStatuses[strconv.Itoa(code)] = true
		}
	}
	platformHTTPStatuses["529"] = true
	platformHTTPStatuses["498"] = true
}

// platformErrorMarkers are the phrases that retry, failover and truncation
// policies read from error text: the extractor's permanent-error words, the
// agentic RAG transient markers, the embedder's rate-limit words and the
// over-limit wording. They are fixed vocabulary, never provider data, so a
// sanitized error can carry the ones the original error contained and be
// classified the same way. Keep this list in sync with those classifiers.
var platformErrorMarkers = append([]string{
	"unauthorized", "authentication", "api key", "bad request", "not found",
	"model not found", "content filter", "no driver resolved", "model_name is required",
	"resolve driver", "529", "overloaded_error", "429", "rate limit", "rate limiting",
	"status 5", "速率限制", "用量上限", "failed to send request", "dial tcp",
	"i/o timeout", "tls handshake", "connection reset", "connection refused",
	"too many requests", "tpm limit", "http2: server sent goaway and closed the connection",
	"errcode=no_error", "is too large to process",
	// Chat answers append an API key hint when they see these.
	"invalid api", "invalid key",
}, tokenizer.OverLimitMarkers...)

// platformMarkerPatterns match each word marker case-insensitively in the
// text. Numeric markers (429, 529) are not copied: in provider text they can
// be a port or part of a host name, and a real status is kept as
// "status <code>" anyway. Classifiers read the original text.
var platformMarkerPatterns = func() []*regexp.Regexp {
	var patterns []*regexp.Regexp
	for _, marker := range platformErrorMarkers {
		if _, err := strconv.Atoi(marker); err == nil {
			continue
		}
		patterns = append(patterns, regexp.MustCompile("(?i)"+regexp.QuoteMeta(marker)))
	}
	return patterns
}()

// platformStatusFromMessage finds the status the provider wrote in its text.
func platformStatusFromMessage(msg string) (string, bool) {
	msg = platformURL.ReplaceAllString(msg, " ")
	for _, re := range []*regexp.Regexp{platformStatusBefore, platformStatusAfter} {
		for _, m := range re.FindAllStringSubmatch(msg, -1) {
			if platformHTTPStatuses[m[1]] {
				return m[1], true
			}
		}
	}
	return "", false
}

// platformMarkers copies, in their original spelling, the fixed phrases of
// msg that error classifiers read; nothing else of the provider text is kept.
func platformMarkers(msg string) []string {
	msg = platformURL.ReplaceAllString(msg, " ")
	var markers []string
	// A status the provider wrote stays readable in the sanitized text.
	// Classifiers do not depend on it; they read the original text.
	if code, ok := platformStatusFromMessage(msg); ok {
		markers = append(markers, "status "+code)
	}
	for _, pattern := range platformMarkerPatterns {
		if m := pattern.FindString(msg); m != "" {
			markers = append(markers, m)
		}
	}
	return markers
}

// platformClassification keeps only recognized classifier signals, in their
// original order and case, without retaining arbitrary provider text.
func platformClassification(msg string) string {
	type signal struct {
		start, end int
		value      string
	}
	var signals []signal
	patterns := append(append([]*regexp.Regexp{}, platformMarkerPatterns...), platformClassTokens, platformRetryAfter)
	for _, pattern := range patterns {
		for _, loc := range pattern.FindAllStringIndex(msg, -1) {
			signals = append(signals, signal{loc[0], loc[1], msg[loc[0]:loc[1]]})
		}
	}
	sort.SliceStable(signals, func(i, j int) bool {
		if signals[i].start == signals[j].start {
			return signals[i].end > signals[j].end
		}
		return signals[i].start < signals[j].start
	})
	var parts []string
	end := -1
	for _, s := range signals {
		if s.start < end {
			continue
		}
		parts = append(parts, s.value)
		end = s.end
	}
	return strings.Join(parts, " ")
}

// withPlatformMarkers appends to base the markers it does not already show.
func withPlatformMarkers(base error, markers []string) error {
	text := base.Error()
	var extra []string
	seen := map[string]bool{}
	for _, marker := range markers {
		if seen[marker] || strings.Contains(text, marker) {
			continue
		}
		seen[marker] = true
		extra = append(extra, marker)
	}
	if len(extra) == 0 {
		return base
	}
	return fmt.Errorf("%w; %s", base, strings.Join(extra, "; "))
}

func platformNetworkCause(err error) error {
	var dns error
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		dns = &net.DNSError{Err: "lookup failed", IsTimeout: dnsErr.IsTimeout, IsTemporary: dnsErr.IsTemporary, IsNotFound: dnsErr.IsNotFound}
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		// Keep the operation ("dial tcp") that retry policies read.
		cause := &net.OpError{Op: opErr.Op, Net: opErr.Net, Err: errors.New("network error")}
		var errno syscall.Errno
		if dns != nil {
			cause.Err = dns
		} else if errors.As(opErr.Err, &errno) {
			cause.Err = errno
		}
		return cause
	}
	if dns != nil {
		return dns
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno
	}
	return nil
}

// PlatformDriverError replaces a platform provider error with one that keeps
// its kind (cancellation, HTTP status, transport failure) and the fixed
// phrases that retry and truncation policies read, but none of the provider's
// text, which can echo credentials or private endpoints. A tenant model and a
// platform model failing the same way are therefore handled the same way.
//
// The original text stays available to retry and limit policies through
// common.ErrorText, so they decide exactly as for a tenant model; the markers
// cover callers that only see the sanitized text.
func PlatformDriverError(err error) error {
	if err == nil {
		return nil
	}
	original := common.ErrorText(err)
	var sentinels []error
	for _, sentinel := range platformSentinels {
		if errors.Is(err, sentinel) {
			sentinels = append(sentinels, sentinel)
		}
	}
	var joined interface{ Unwrap() []error }
	isJoined := errors.As(err, &joined)
	var prior interface{ RetryTransient() (bool, bool) }
	transient := false
	if isJoined {
		transient = common.IsTransientError(err)
	} else if errors.As(err, &prior) {
		transient, isJoined = prior.RetryTransient()
	}
	return &platformError{safe: sanitizePlatformError(err, original), original: platformClassification(original), sentinels: sentinels, joined: isJoined, transient: transient}
}

// platformSentinels are standard errors that callers compare with errors.Is
// to decide how a call ended (a closed stream, a dropped connection). They
// carry no provider data, so the sanitized error still matches them.
var platformSentinels = []error{io.EOF, io.ErrUnexpectedEOF, io.ErrClosedPipe, net.ErrClosed, os.ErrDeadlineExceeded}

// platformError shows only the sanitized error; the original text is kept for
// classification and must not reach responses or logs.
type platformError struct {
	safe      error
	original  string
	sentinels []error
	joined    bool
	transient bool
}

func (e *platformError) Error() string                { return e.safe.Error() }
func (e *platformError) Unwrap() error                { return e.safe }
func (e *platformError) ClassificationText() string   { return e.original }
func (e *platformError) RetryTransient() (bool, bool) { return e.transient, e.joined }

func (e *platformError) Is(target error) bool {
	for _, sentinel := range e.sentinels {
		if target == sentinel {
			return true
		}
	}
	return false
}

func sanitizePlatformError(err error, msg string) error {
	markers := platformMarkers(msg)
	var apiStatus *APIStatusError
	var network net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return withPlatformMarkers(context.Canceled, markers)
	case errors.Is(err, context.DeadlineExceeded):
		return withPlatformMarkers(context.DeadlineExceeded, markers)
	case errors.As(err, &network):
		// Transport errors come before message parsing: their text holds URLs
		// and ports that the status patterns could misread.
		return withPlatformMarkers(platformNetworkError{timeout: network.Timeout(), cause: platformNetworkCause(err)}, markers)
	case platformNetworkCause(err) != nil:
		return withPlatformMarkers(platformNetworkError{cause: platformNetworkCause(err)}, markers)
	case errors.As(err, &apiStatus):
		// A structured status stays structured, so failover and retry
		// policies that inspect *APIStatusError see it as before.
		base := &APIStatusError{Status: apiStatus.Status, hideBody: true}
		if extra := withPlatformMarkers(base, markers); extra != error(base) {
			body := strings.TrimPrefix(extra.Error(), base.Error()+"; ")
			return &APIStatusError{Status: apiStatus.Status, Body: body}
		}
		return base
	}
	base := "platform model request failed"
	// A status only written in the text stays text, in a wording that the
	// status readers recognize; the original status wording is a marker.
	if code, ok := platformStatusFromMessage(msg); ok {
		base += ": error " + code
	}
	return withPlatformMarkers(errors.New(base), markers)
}

// platformCallbackError keeps errors returned by caller-supplied callbacks,
// such as stream senders: they come from our own code and callers compare
// them with sentinels.
type platformCallbackError struct{ err error }

func (p *platformCallbackError) wrap(sender func(*string, *string) error) func(*string, *string) error {
	return func(text, reasoning *string) error {
		err := sender(text, reasoning)
		if err != nil {
			p.err = err
		}
		return err
	}
}

func (p *platformCallbackError) result(err error) error {
	if err == nil {
		return nil
	}
	if p.err != nil && errors.Is(err, p.err) {
		return p.err
	}
	return PlatformDriverError(err)
}

func (d platformSafeDriver) NewInstance(urls map[string]string) ModelDriver {
	return NewPlatformSafeDriver(d.ModelDriver.NewInstance(urls))
}
func (d platformSafeDriver) ChatWithMessages(ctx context.Context, name string, msg []Message, api *APIConfig, cfg *ChatConfig, usage *common.ModelUsage) (*ChatResponse, error) {
	r, e := d.ModelDriver.ChatWithMessages(ctx, name, msg, api, cfg, usage)
	return r, PlatformDriverError(e)
}
func (d platformSafeDriver) ChatStreamlyWithSender(ctx context.Context, name string, msg []Message, api *APIConfig, cfg *ChatConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	safeSender := sender
	if strings.EqualFold(d.ModelDriver.Name(), "jina") {
		safeSender = func(text, reasoning *string) error {
			// Jina reports a failure in the stream text; sanitize it like a
			// returned error so answer checks see the same fixed phrases.
			if text != nil && strings.HasPrefix(*text, "**ERROR**: ") {
				sanitized := "**ERROR**: " + PlatformDriverError(errors.New(strings.TrimPrefix(*text, "**ERROR**: "))).Error()
				return sender(&sanitized, reasoning)
			}
			return sender(text, reasoning)
		}
	}
	var callback platformCallbackError
	return callback.result(d.ModelDriver.ChatStreamlyWithSender(ctx, name, msg, api, cfg, usage, callback.wrap(safeSender)))
}
func (d platformSafeDriver) Embed(ctx context.Context, name *string, req EmbedRequest, api *APIConfig, cfg *EmbeddingConfig, usage *common.ModelUsage) ([]EmbeddingData, error) {
	r, e := d.ModelDriver.Embed(ctx, name, req, api, cfg, usage)
	return r, PlatformDriverError(e)
}
func (d platformSafeDriver) Rerank(ctx context.Context, name *string, req RerankRequest, api *APIConfig, cfg *RerankConfig, usage *common.ModelUsage) (*RerankResponse, error) {
	r, e := d.ModelDriver.Rerank(ctx, name, req, api, cfg, usage)
	return r, PlatformDriverError(e)
}
func (d platformSafeDriver) TranscribeAudio(ctx context.Context, name, file *string, api *APIConfig, cfg *ASRConfig, usage *common.ModelUsage) (*ASRResponse, error) {
	r, e := d.ModelDriver.TranscribeAudio(ctx, name, file, api, cfg, usage)
	return r, PlatformDriverError(e)
}
func (d platformSafeDriver) TranscribeAudioWithSender(ctx context.Context, name, file *string, api *APIConfig, cfg *ASRConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	var callback platformCallbackError
	return callback.result(d.ModelDriver.TranscribeAudioWithSender(ctx, name, file, api, cfg, usage, callback.wrap(sender)))
}
func (d platformSafeDriver) AudioSpeech(ctx context.Context, name, text *string, api *APIConfig, cfg *TTSConfig, usage *common.ModelUsage) (*TTSResponse, error) {
	r, e := d.ModelDriver.AudioSpeech(ctx, name, text, api, cfg, usage)
	return r, PlatformDriverError(e)
}
func (d platformSafeDriver) AudioSpeechWithSender(ctx context.Context, name, text *string, api *APIConfig, cfg *TTSConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	var callback platformCallbackError
	return callback.result(d.ModelDriver.AudioSpeechWithSender(ctx, name, text, api, cfg, usage, callback.wrap(sender)))
}
func (d platformSafeDriver) OCRFile(ctx context.Context, name *string, content []byte, url *string, api *APIConfig, cfg *OCRConfig, usage *common.ModelUsage) (*OCRFileResponse, error) {
	r, e := d.ModelDriver.OCRFile(ctx, name, content, url, api, cfg, usage)
	if r != nil && len(r.Warnings) > 0 {
		// Partial-failure warnings quote the provider's error messages.
		safe := *r
		safe.Warnings = make([]string, len(r.Warnings))
		for i := range safe.Warnings {
			safe.Warnings[i] = "platform OCR model returned a partial failure"
		}
		r = &safe
	}
	return r, PlatformDriverError(e)
}
func (d platformSafeDriver) ParseFile(ctx context.Context, name *string, content []byte, url *string, api *APIConfig, cfg *ParseFileConfig, usage *common.ModelUsage) (*ParseFileResponse, error) {
	r, e := d.ModelDriver.ParseFile(ctx, name, content, url, api, cfg, usage)
	return r, PlatformDriverError(e)
}
func (d platformSafeDriver) ListModels(ctx context.Context, api *APIConfig) ([]ListModelResponse, error) {
	r, e := d.ModelDriver.ListModels(ctx, api)
	return r, PlatformDriverError(e)
}
func (d platformSafeDriver) Balance(ctx context.Context, api *APIConfig) (map[string]interface{}, error) {
	r, e := d.ModelDriver.Balance(ctx, api)
	return r, PlatformDriverError(e)
}
func (d platformSafeDriver) CheckConnection(ctx context.Context, api *APIConfig) error {
	return PlatformDriverError(d.ModelDriver.CheckConnection(ctx, api))
}
func (d platformSafeDriver) ListTasks(ctx context.Context, api *APIConfig) ([]ListTaskStatus, error) {
	r, e := d.ModelDriver.ListTasks(ctx, api)
	return r, PlatformDriverError(e)
}
func (d platformSafeDriver) ShowTask(ctx context.Context, id string, api *APIConfig) (*TaskResponse, error) {
	r, e := d.ModelDriver.ShowTask(ctx, id, api)
	return r, PlatformDriverError(e)
}
func (d platformSafeDriver) Decide(ctx context.Context, name *string, req *DecideRequest, api *APIConfig, usage *common.ModelUsage) (*DecideResponse, error) {
	r, e := d.ModelDriver.Decide(ctx, name, req, api, usage)
	return r, PlatformDriverError(e)
}
