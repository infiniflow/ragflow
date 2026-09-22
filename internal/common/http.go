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

package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	redactedLogValue      = "[REDACTED]"
	maxLoggedVectorFloats = 3
)

var ssrfHttpClient *http.Client
var schemeSafeHttpClient *http.Client

func GetSSRFHTTPClient() *http.Client {
	if ssrfHttpClient == nil {
		var t *http.Transport
		if dt, ok := http.DefaultTransport.(*http.Transport); ok {
			t = dt.Clone()
		} else {
			t = &http.Transport{Proxy: http.ProxyFromEnvironment}
		}
		t.MaxIdleConns = 100
		t.MaxIdleConnsPerHost = 10
		t.IdleConnTimeout = 90 * time.Second
		t.DisableCompression = false
		t.ResponseHeaderTimeout = 20 * time.Minute
		t.TLSHandshakeTimeout = 30 * time.Second

		var rt http.RoundTripper = t
		rt = &strictSSRFTransport{base: rt}
		rt = newProviderLoggingTransport(rt)
		ssrfHttpClient = &http.Client{Transport: rt}
	}
	return ssrfHttpClient
}

func GetSchemeSafeHTTPClient() *http.Client {
	if schemeSafeHttpClient == nil {
		var t *http.Transport
		if dt, ok := http.DefaultTransport.(*http.Transport); ok {
			t = dt.Clone()
		} else {
			t = &http.Transport{Proxy: http.ProxyFromEnvironment}
		}
		t.MaxIdleConns = 5000
		t.MaxIdleConnsPerHost = 500
		t.IdleConnTimeout = 90 * time.Second
		t.DisableCompression = false
		t.ResponseHeaderTimeout = 60 * time.Second
		t.TLSHandshakeTimeout = 30 * time.Second

		var rt http.RoundTripper = t
		rt = &schemeSafeTransport{base: rt}
		rt = newProviderLoggingTransport(rt)
		schemeSafeHttpClient = &http.Client{Transport: rt}
	}
	return schemeSafeHttpClient
}

// providerStreamLogThreshold / providerCallLogThreshold are the durations past
// which a provider call's timings are reported without LLM_DEBUG. A streaming
// answer is reported earlier because it is the one the user waits on.
const (
	providerStreamLogThreshold = 5 * time.Second
	providerCallLogThreshold   = 30 * time.Second
)

func newProviderLoggingTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	// Always installed: LLM_DEBUG decides whether payloads are logged, not whether
	// timings are collected — first-token only exists while the call is in flight.
	return &providerLoggingTransport{base: base, now: time.Now, debug: IsLLMDebugEnabled()}
}

type providerLoggingTransport struct {
	base  http.RoundTripper
	now   func() time.Time
	debug bool
}

func (t *providerLoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	providerURL := redactProviderURL(req.URL)
	logPayload := ""
	if t.debug {
		payload, err := readAndRestoreRequestBody(req)
		if err != nil {
			return nil, err
		}
		logPayload = redactProviderBody(payload)
	}

	startedAt := t.now()
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		if t.debug {
			logProviderCall(providerURL, logPayload, 0, "", t.now().Sub(startedAt), 0, err)
		}
		return nil, err
	}
	if resp.Body == nil {
		if t.debug {
			logProviderCall(providerURL, logPayload, resp.StatusCode, "", t.now().Sub(startedAt), 0, nil)
		}
		return resp, nil
	}

	summary := providerCallTiming{
		url:       providerURL,
		status:    resp.StatusCode,
		streaming: strings.Contains(resp.Header.Get("Content-Type"), "event-stream"),
	}
	resp.Body = &providerResponseBody{
		ReadCloser: resp.Body,
		startedAt:  startedAt,
		now:        t.now,
		capture:    t.debug,
		log: func(body []byte, took, firstToken time.Duration) {
			if t.debug {
				logProviderCall(providerURL, logPayload, resp.StatusCode, redactProviderBody(body), took, firstToken, nil)
				return
			}
			summary.report(took, firstToken)
		},
	}
	return resp, nil
}

// providerCallTiming describes one provider call worth reporting.
type providerCallTiming struct {
	url       string
	status    int
	streaming bool
}

// report logs the call once it is slow enough to be worth explaining. firstToken
// is queueing, connection setup and prefill; the rest is generation — or a
// consumer stalling the stream, which shows as a large took next to a small
// firstToken.
func (p providerCallTiming) report(took, firstToken time.Duration) {
	limit := providerCallLogThreshold
	if p.streaming {
		limit = providerStreamLogThreshold
	}
	if took < limit {
		return
	}
	Info("Provider call",
		zap.String("url", p.url),
		zap.Int("status", p.status),
		zap.Bool("streaming", p.streaming),
		zap.Duration("took", took),
		zap.Duration("firstToken", firstToken),
		zap.Duration("afterFirstToken", took-firstToken))
}

// providerResponseBody captures bytes while callers consume them, preserving
// streaming delivery instead of eagerly reading the entire provider response.
type providerResponseBody struct {
	io.ReadCloser
	body       bytes.Buffer
	startedAt  time.Time
	now        func() time.Time
	firstToken time.Duration
	firstOnce  sync.Once
	logOnce    sync.Once
	capture    bool
	log        func([]byte, time.Duration, time.Duration)
}

func (b *providerResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.firstOnce.Do(func() {
			b.firstToken = b.now().Sub(b.startedAt)
		})
		if b.capture {
			_, _ = b.body.Write(p[:n])
		}
	}
	if err == io.EOF {
		b.writeLogOnce()
	}
	return n, err
}

func (b *providerResponseBody) Close() error {
	err := b.ReadCloser.Close()
	b.writeLogOnce()
	return err
}

func (b *providerResponseBody) writeLogOnce() {
	b.logOnce.Do(func() {
		b.log(b.body.Bytes(), b.now().Sub(b.startedAt), b.firstToken)
	})
}

func readAndRestoreRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read provider request body for logging: %w", err)
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	return body, nil
}

func redactProviderURL(requestURL *url.URL) string {
	if requestURL == nil {
		return ""
	}
	redacted := *requestURL
	if redacted.User != nil {
		redacted.User = url.User(redacted.User.Username())
	}
	query := redacted.Query()
	for key := range query {
		if isSensitiveLogKey(key) {
			query.Set(key, redactedLogValue)
		}
	}
	redacted.RawQuery = query.Encode()
	return redacted.String()
}

func redactProviderBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var value any
	if err := json.Unmarshal(body, &value); err == nil {
		redactProviderValue(value)
		if redacted, err := json.Marshal(value); err == nil {
			return string(redacted)
		}
	}

	lines := strings.Split(string(body), "\n")
	redactedAny := false
	for i, line := range lines {
		prefix, data, ok := strings.Cut(line, "data:")
		if !ok || strings.TrimSpace(data) == "[DONE]" {
			continue
		}
		var event any
		if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &event); err != nil {
			continue
		}
		redactProviderValue(event)
		redacted, err := json.Marshal(event)
		if err != nil {
			continue
		}
		lines[i] = prefix + "data: " + string(redacted)
		redactedAny = true
	}
	if redactedAny {
		return strings.Join(lines, "\n")
	}
	return string(body)
}

func redactProviderValue(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if isSensitiveLogKey(key) {
				value[key] = redactedLogValue
				continue
			}
			if isVectorLogKey(key) {
				child = truncateLoggedVectors(child)
				value[key] = child
			}
			redactProviderValue(child)
		}
	case []any:
		for _, child := range value {
			redactProviderValue(child)
		}
	}
}

func isVectorLogKey(key string) bool {
	switch strings.ToLower(key) {
	case "embedding", "embeddings", "vector", "vectors":
		return true
	default:
		return false
	}
}

func truncateLoggedVectors(value any) any {
	switch value := value.(type) {
	case []any:
		if isNumericVector(value) {
			if len(value) > maxLoggedVectorFloats {
				return value[:maxLoggedVectorFloats]
			}
			return value
		}
		for i, child := range value {
			value[i] = truncateLoggedVectors(child)
		}
	case map[string]any:
		for key, child := range value {
			value[key] = truncateLoggedVectors(child)
		}
	}
	return value
}

func isNumericVector(value []any) bool {
	if len(value) == 0 {
		return false
	}
	for _, item := range value {
		if _, ok := item.(float64); !ok {
			return false
		}
	}
	return true
}

func isSensitiveLogKey(key string) bool {
	normalized := strings.NewReplacer("-", "", "_", "", ".", "").Replace(strings.ToLower(key))
	switch normalized {
	case "apikey", "authorization", "accesstoken", "refreshtoken", "password", "secret", "token", "key":
		return true
	default:
		return false
	}
}

func logProviderCall(providerURL, payload string, statusCode int, responseBody string, took, firstToken time.Duration, err error) {
	request := fmt.Sprintf("url=%s payload=%s", providerURL, payload)
	response := fmt.Sprintf("response_code=%d took=%s first-token=%s response_body=%s", statusCode, took, firstToken, responseBody)
	if err != nil {
		response += " error=" + err.Error()
	}
	LogRequestResponseInfo(request, response, err == nil && statusCode >= 200 && statusCode < 300)
}

// schemeSafeTransport wraps an http.RoundTripper so every outgoing request is
// validated by the lenient SSRF guard (http/https scheme + non-empty host).
// Private and loopback hosts are permitted. Used only by local-inference
// drivers that may target a user's own network.
type schemeSafeTransport struct{ base http.RoundTripper }

func (t *schemeSafeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := AssertURLSchemeSafe(req.URL.String()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

// strictSSRFTransport wraps an http.RoundTripper so every outgoing request is
// validated by the strict SSRF guard (scheme + host + globally routable IP).
// This is the default for cloud-hosted model drivers and closes the
// go/request-forgery data flow: the user-controllable BaseURL cannot be made to
// point at private hosts, loopback, link-local, or cloud metadata endpoints.
type strictSSRFTransport struct{ base http.RoundTripper }

func (t *strictSSRFTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, _, err := AssertURLSafe(req.URL.String()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

var AllowedURLSchemes = []string{"http", "https"}

// LookupHost is the indirection used to resolve hostnames. Tests override it.
var LookupHost = net.LookupHost

// AllowAnyHostForTest is a test-only override that skips the
// public-IP routability check in AssertURLSafe and AssertHostSafe.
// Scheme, host, and DNS resolution checks are unchanged: hostnames
// still resolve, unresolvable hostnames still fail, and callers
// still pin connections to the returned resolved address. Production
// code MUST leave this at its zero value (false). Tests that need to
// talk to a local httptest server flip it on and reset it in
// t.Cleanup.
//
// The previous form (env-var ALLOW_ANY_HOST) was a live runtime
// toggle that any operator could flip to disable the SSRF guard
// globally — including the DNS pinning that the Invoke component
// relies on. PR review round 6, Major #3: this variable lives in
// process memory only, so it cannot be enabled by an env var or
// a deployment mistake. The explicit "_ForTest" suffix is the
// signal that production code must never touch it.
var AllowAnyHostForTest = false

// allowAnyHost reads the test-only override. Kept as a private
// helper so the call sites don't all have to know about the
// exported variable name.
func allowAnyHost() bool {
	return AllowAnyHostForTest
}

// AssertURLSchemeSafe is a lenient SSRF guard for drivers that may legitimately
// target private networks or loopback addresses (e.g. self-hosted Ollama, vLLM,
// Xinference). It only rejects dangerous schemes and empty hosts; it does not
// resolve DNS and does not require public routability. Use this ONLY for
// local-inference model drivers — cloud-hosted drivers must use AssertURLSafe.
var AssertURLSchemeSafe = func(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid url")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if !slices.Contains(AllowedURLSchemes, scheme) {
		sorted := append([]string(nil), AllowedURLSchemes...)
		sort.Strings(sorted)
		return fmt.Errorf("disallowed URL scheme: '%s'. Only %v are allowed", scheme, sorted)
	}

	if parsed.Hostname() == "" {
		return fmt.Errorf("URL is missing a host")
	}

	return nil
}

var AssertURLSafe = func(rawURL string) (hostname, resolvedIP string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", "", fmt.Errorf("invalid url")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if !slices.Contains(AllowedURLSchemes, scheme) {
		sorted := append([]string(nil), AllowedURLSchemes...)
		sort.Strings(sorted)
		return "", "", fmt.Errorf("disallowed URL scheme: '%s'. Only %v are allowed", scheme, sorted)
	}

	hostname = parsed.Hostname()
	if hostname == "" {
		return "", "", fmt.Errorf("URL is missing a host")
	}

	allowAny := allowAnyHost()
	addresses, err := LookupHost(hostname)
	if err != nil {
		return "", "", fmt.Errorf("could not resolve hostname '%s': %w", hostname, err)
	}
	if len(addresses) == 0 {
		return "", "", fmt.Errorf("hostname '%s' resolved to no addresses", hostname)
	}

	for _, addr := range addresses {
		ip := net.ParseIP(addr)
		if ip == nil {
			return "", "", fmt.Errorf("could not parse resolved address '%s' for hostname '%s'", addr, hostname)
		}
		if !allowAny && !isGlobalIP(effectiveIP(ip)) {
			return "", "", fmt.Errorf("URL resolves to a non-public address (%s), which is not allowed", ip.String())
		}
		if resolvedIP == "" {
			resolvedIP = ip.String()
		}
	}
	return hostname, resolvedIP, nil
}

// AssertHostSafe validates a bare host (a hostname or a literal IP, with no
// scheme or port) and returns the first resolved public IP. It is the
// host-type counterpart of AssertURLSafe: every resolved address must be
// globally routable (private, loopback, link-local, metadata, multicast and
// reserved ranges are rejected). Callers dial the returned IP directly so DNS
// cannot rebind the connection to an internal address between validation and
// the TCP connect.
//
// Used by host-based data sources (IMAP/MySQL/PostgreSQL) and by the ExeSQL /
// test_db_connection host guards, mirroring common/ssrf_guard.py:
// assert_host_is_safe.
var AssertHostSafe = func(host string) (resolvedIP string, err error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("host is missing")
	}

	allowAny := allowAnyHost()
	if ip := net.ParseIP(host); ip != nil {
		if !allowAny && !isGlobalIP(effectiveIP(ip)) {
			return "", fmt.Errorf("host is not a public address (%s), which is not allowed", ip.String())
		}
		return ip.String(), nil
	}

	addresses, err := LookupHost(host)
	if err != nil {
		return "", fmt.Errorf("could not resolve hostname '%s': %w", host, err)
	}
	if len(addresses) == 0 {
		return "", fmt.Errorf("hostname '%s' resolved to no addresses", host)
	}

	for _, addr := range addresses {
		ip := net.ParseIP(addr)
		if ip == nil {
			return "", fmt.Errorf("could not parse resolved address '%s' for hostname '%s'", addr, host)
		}
		if !allowAny && !isGlobalIP(effectiveIP(ip)) {
			return "", fmt.Errorf("hostname '%s' resolves to a non-public address (%s), which is not allowed", host, ip.String())
		}
		if resolvedIP == "" {
			resolvedIP = ip.String()
		}
	}
	return resolvedIP, nil
}

// effectiveIP unwraps IPv4-mapped IPv6 addresses (e.g. ::ffff:127.0.0.1) so
// the routability check sees the IPv4 form. Without this, an attacker could
// bypass the guard with an IPv4-mapped IPv6 representation of a private host.
func effectiveIP(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip
}

// isGlobalIP mirrors Python's ipaddress.IPv*Address.is_global: an address is
// global if it is none of {unspecified, loopback, multicast, link-local,
// private (including CGNAT and IPv6 ULA), benchmarking, documentation,
// reserved}.
func isGlobalIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsPrivate() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		// 0.0.0.0/8 — "this network"; 0.x.y.z routes to localhost on Linux.
		if v4[0] == 0 {
			return false
		}
		// CGNAT 100.64.0.0/10 — not flagged by IsPrivate in older Go versions.
		if v4[0] == 100 && v4[1]&0xC0 == 64 {
			return false
		}
		// 192.0.0.0/24 reserved for IETF protocol assignments.
		if v4[0] == 192 && v4[1] == 0 && v4[2] == 0 {
			return false
		}
		// 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24 documentation (TEST-NET-1/2/3).
		if v4[0] == 192 && v4[1] == 0 && v4[2] == 2 {
			return false
		}
		if v4[0] == 198 && v4[1] == 51 && v4[2] == 100 {
			return false
		}
		if v4[0] == 203 && v4[1] == 0 && v4[2] == 113 {
			return false
		}
		// 198.18.0.0/15 benchmarking.
		if v4[0] == 198 && (v4[1] == 18 || v4[1] == 19) {
			return false
		}
		// 240.0.0.0/4 reserved (excluding 255.255.255.255 which IsUnspecified misses).
		if v4[0] >= 240 {
			return false
		}
	} else if v6 := ip.To16(); v6 != nil {
		// 2001:db8::/32 documentation prefix.
		if v6[0] == 0x20 && v6[1] == 0x01 && v6[2] == 0x0d && v6[3] == 0xb8 {
			return false
		}
		// 100::/64 discard-only address block.
		if v6[0] == 0x01 && v6[1] == 0x00 && allZero(v6[2:8]) {
			return false
		}
		// IPv6 transition addresses (6to4, NAT64, Teredo, IPv4-compatible) embed
		// an arbitrary IPv4 address that none of the checks above look at. Unwrap
		// and re-check it so 2002:7f00:1::1 is treated as 127.0.0.1.
		for _, inner := range embeddedIPv4(v6) {
			if !isGlobalIP(inner) {
				return false
			}
		}
	}
	return true
}

// embeddedIPv4 returns the IPv4 addresses carried inside an IPv6 transition
// address, or nil when it carries none. Teredo yields two: the relay server and
// the (obfuscated) client.
func embeddedIPv4(v6 net.IP) []net.IP {
	switch {
	// 6to4 — RFC 3056, 2002::/16, IPv4 in bytes 2-6.
	case v6[0] == 0x20 && v6[1] == 0x02:
		return []net.IP{net.IPv4(v6[2], v6[3], v6[4], v6[5])}

	// NAT64 well-known prefix — RFC 6052, 64:ff9b::/96, IPv4 in the low 32 bits.
	case v6[0] == 0x00 && v6[1] == 0x64 && v6[2] == 0xff && v6[3] == 0x9b && allZero(v6[4:12]):
		return []net.IP{net.IPv4(v6[12], v6[13], v6[14], v6[15])}

	// NAT64 local-use prefix — RFC 8215, 64:ff9b:1::/48. The embedded IPv4
	// position depends on the operator's prefix length, so block the range.
	case v6[0] == 0x00 && v6[1] == 0x64 && v6[2] == 0xff && v6[3] == 0x9b && v6[4] == 0x00 && v6[5] == 0x01:
		return []net.IP{net.IPv4zero}

	// Teredo — RFC 4380, 2001::/32. Server IPv4 in bytes 4-8, client IPv4 in
	// bytes 12-16 obfuscated by XOR with 0xff.
	case v6[0] == 0x20 && v6[1] == 0x01 && v6[2] == 0x00 && v6[3] == 0x00:
		return []net.IP{
			net.IPv4(v6[4], v6[5], v6[6], v6[7]),
			net.IPv4(v6[12]^0xff, v6[13]^0xff, v6[14]^0xff, v6[15]^0xff),
		}

	// IPv4-compatible — deprecated ::a.b.c.d, not unwrapped by net.IP.To4.
	case allZero(v6[0:12]):
		return []net.IP{net.IPv4(v6[12], v6[13], v6[14], v6[15])}
	}
	return nil
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

type response struct {
	Code    ErrorCode   `json:"code"`
	Data    interface{} `json:"data"`
	Message interface{} `json:"message"`
	Total   interface{} `json:"total,omitempty"`
}

// errorResponse error response
type errorResponse struct {
	Code    ErrorCode   `json:"code"`
	Message interface{} `json:"message"`
}

// SuccessWithData returns success response with data
func SuccessWithData(c *gin.Context, data interface{}, message interface{}) {
	c.JSON(http.StatusOK, response{
		Code:    CodeSuccess,
		Data:    data,
		Message: message,
	})
}

// SuccessWithDataAndTotal returns success response with data and total number
func SuccessWithDataAndTotal(c *gin.Context, data, total, message interface{}) {
	c.JSON(http.StatusOK, response{
		Code:    CodeSuccess,
		Data:    data,
		Total:   total,
		Message: message,
	})
}

// SuccessNoMessage returns success response without message
func SuccessNoMessage(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, response{
		Code: CodeSuccess,
		Data: data,
	})
}

// SuccessNoData returns success response without data
func SuccessNoData(c *gin.Context, message interface{}) {
	c.JSON(http.StatusOK, response{
		Code:    CodeSuccess,
		Data:    nil,
		Message: message,
	})
}

// SuccessWithMessage returns success response with message only
func SuccessWithMessage(c *gin.Context, message string) {
	c.JSON(http.StatusOK, response{
		Code:    CodeSuccess,
		Message: message,
	})
}

// ErrorWithCode returns error response with code and message
func ErrorWithCode(c *gin.Context, code ErrorCode, message string) {
	c.JSON(http.StatusOK, errorResponse{
		Code:    code,
		Message: message,
	})
}

func ResponseWithCodeData(c *gin.Context, code ErrorCode, data interface{}, message string) {
	c.JSON(http.StatusOK, response{
		Code:    code,
		Data:    data,
		Message: message,
	})
}

func ResponseWithHttpCodeData(c *gin.Context, httpCode int, code ErrorCode, data interface{}, message string) {
	c.JSON(httpCode, response{
		Code:    code,
		Data:    data,
		Message: message,
	})
}

func ParseRequestIntPositive(c *gin.Context, parameter, parameterName string, defaultValue int) (int, error) {
	var parameterInt int
	var err error
	if parameter == "" {
		parameterInt = defaultValue
	} else {
		parameterInt, err = strconv.Atoi(parameter)
		if err != nil {
			return defaultValue, fmt.Errorf("%w: %s must be an integer", err, parameterName)
		}
	}

	if parameterInt < 0 {
		return defaultValue, fmt.Errorf("%w: %s must be a positive integer or zero", err, parameterName)
	}
	return parameterInt, nil
}
