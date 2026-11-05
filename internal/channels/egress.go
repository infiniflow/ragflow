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

package channels

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/net/idna"

	"ragflow/internal/common"
	"ragflow/internal/utility"
)

// RAGFLOW_CHANNEL_ALLOWED_HOSTS is a comma-separated list of operator-approved
// exact hosts, optionally with ports. It does not accept wildcards.
const channelAllowedHostsEnv = "RAGFLOW_CHANNEL_ALLOWED_HOSTS"

type channelURLPolicy struct {
	exactHosts      []string
	hostSuffixes    []string
	localBaseURL    string
	localPathPrefix string
}

type channelHTTPTransport struct {
	policy     channelURLPolicy
	timeout    time.Duration
	mu         sync.Mutex
	transports map[string]http.RoundTripper
}

func newSafeChannelHTTPClient(timeout time.Duration, policy channelURLPolicy) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: &channelHTTPTransport{policy: policy, timeout: timeout, transports: map[string]http.RoundTripper{}},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func newChannelSDKWebSocketDialer(base *websocket.Dialer) *websocket.Dialer {
	if base == nil {
		base = &websocket.Dialer{}
	}
	dialer := *base
	dialer.NetDial = nil
	dialer.NetDialTLSContext = nil
	dialer.Proxy = func(req *http.Request) (*url.URL, error) {
		if req == nil || req.URL == nil || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Fragment != "" {
			return nil, fmt.Errorf("channel SDK WebSocket must use a valid TLS URL")
		}
		if err := validateChannelSDKWebSocketDestination(req.URL.Hostname(), channelURLPort(req.URL)); err != nil {
			return nil, err
		}
		return nil, nil
	}
	dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" && network != "tcp4" && network != "tcp6" {
			return nil, fmt.Errorf("unsupported channel SDK WebSocket network %q", network)
		}
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid channel SDK WebSocket destination")
		}
		host, err = normalizeChannelHost(host)
		if err != nil {
			return nil, fmt.Errorf("invalid channel SDK WebSocket host")
		}
		if err := validateChannelSDKWebSocketDestination(host, port); err != nil {
			return nil, err
		}
		resolvedIP, err := common.AssertHostSafe(host)
		if err != nil {
			return nil, fmt.Errorf("channel SDK WebSocket host %q is not safe: %w", host, err)
		}
		return (&net.Dialer{Timeout: dialer.HandshakeTimeout, KeepAlive: 30 * time.Second}).DialContext(
			ctx,
			network,
			net.JoinHostPort(resolvedIP, port),
		)
	}
	return &dialer
}

func validateChannelSDKWebSocketDestination(host, port string) error {
	host, err := normalizeChannelHost(host)
	if err != nil {
		return fmt.Errorf("invalid channel SDK WebSocket host")
	}
	if port != "443" {
		return fmt.Errorf("channel SDK WebSocket destination must use port 443")
	}
	switch host {
	case "msg-frontier.feishu.cn", "msg-frontier.larksuite.com", "wss-open-connection.dingtalk.com":
		return nil
	default:
		return fmt.Errorf("channel SDK WebSocket host %q is not trusted", host)
	}
}

func (t *channelHTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("channel request has no URL")
	}
	host, resolvedIP, _, err := resolveChannelDestination(req.URL, t.policy, false)
	if err != nil {
		return nil, err
	}
	transport := t.transport(host, resolvedIP)
	return transport.RoundTrip(req)
}

func (t *channelHTTPTransport) transport(host, resolvedIP string) http.RoundTripper {
	key := host + "\x00" + resolvedIP
	t.mu.Lock()
	defer t.mu.Unlock()
	if transport := t.transports[key]; transport != nil {
		return transport
	}
	client := utility.PinnedHTTPClient(host, resolvedIP, t.timeout)
	t.transports[key] = client.Transport
	return client.Transport
}

func dialSafeChannelWebSocket(ctx context.Context, rawURL string, header http.Header, timeout time.Duration, policy channelURLPolicy) (*websocket.Conn, *http.Response, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, nil, fmt.Errorf("invalid channel WebSocket URL")
	}
	host, resolvedIP, port, err := resolveChannelDestination(parsed, policy, true)
	if err != nil {
		return nil, nil, err
	}

	dialer := *websocket.DefaultDialer
	dialer.Proxy = nil
	dialer.NetDial = nil
	dialer.NetDialTLSContext = nil
	dialer.HandshakeTimeout = timeout
	dialer.NetDialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		addressHost, addressPort, splitErr := net.SplitHostPort(address)
		normalizedAddressHost, normalizeErr := normalizeChannelHost(addressHost)
		if splitErr != nil || normalizeErr != nil || normalizedAddressHost != host || addressPort != port {
			return nil, fmt.Errorf("unvalidated channel WebSocket destination")
		}
		return (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext(
			ctx,
			network,
			net.JoinHostPort(resolvedIP, addressPort),
		)
	}
	return dialer.DialContext(ctx, rawURL, header)
}

func resolveChannelDestination(parsed *url.URL, policy channelURLPolicy, websocketURL bool) (string, string, string, error) {
	if parsed == nil || !parsed.IsAbs() || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", "", "", fmt.Errorf("invalid channel destination URL")
	}

	scheme := strings.ToLower(parsed.Scheme)
	switch {
	case websocketURL && scheme != "ws" && scheme != "wss":
		return "", "", "", fmt.Errorf("channel WebSocket URL must use ws or wss")
	case !websocketURL && scheme != "http" && scheme != "https":
		return "", "", "", fmt.Errorf("channel HTTP URL must use http or https")
	}

	host, err := normalizeChannelHost(parsed.Hostname())
	if err != nil {
		return "", "", "", fmt.Errorf("invalid channel destination host")
	}
	port := channelURLPort(parsed)

	if localHost, localIP, localPort, ok := resolveAllowedLocalChannelDestination(parsed, policy, websocketURL); ok {
		return localHost, localIP, localPort, nil
	}

	operatorAllowed, operatorPortAllowed := operatorAllowsChannelDestination(parsed)
	if !operatorPortAllowed && !isStandardChannelPort(parsed) {
		return "", "", "", fmt.Errorf("nonstandard port is not allowed for channel destination %q", host)
	}

	if operatorAllowed {
		resolvedIP, privateOnly, err := lookupChannelHost(parsed.Hostname())
		if err != nil {
			return "", "", "", err
		}
		if !isSecureChannelScheme(scheme) && !privateOnly {
			return "", "", "", fmt.Errorf("unencrypted channel destination %q must resolve only to private addresses", host)
		}
		return host, resolvedIP, port, nil
	}

	if !isTrustedChannelHost(host, policy) {
		return "", "", "", fmt.Errorf("channel destination host %q is not trusted; configure %s for an approved custom endpoint", host, channelAllowedHostsEnv)
	}
	if !isSecureChannelScheme(scheme) {
		return "", "", "", fmt.Errorf("channel destination %q must use TLS", host)
	}

	checkURL := *parsed
	if websocketURL {
		if scheme == "ws" {
			checkURL.Scheme = "http"
		} else {
			checkURL.Scheme = "https"
		}
	}
	_, resolvedIP, err := common.AssertURLSafe(checkURL.String())
	if err != nil {
		return "", "", "", fmt.Errorf("channel destination %q is not safe: %w", host, err)
	}
	return host, resolvedIP, port, nil
}

func isTrustedChannelHost(host string, policy channelURLPolicy) bool {
	for _, allowedHost := range policy.exactHosts {
		normalized, err := normalizeChannelHost(allowedHost)
		if err == nil && host == normalized {
			return true
		}
	}
	for _, suffix := range policy.hostSuffixes {
		normalized, err := normalizeChannelHost(suffix)
		if err == nil && (host == normalized || strings.HasSuffix(host, "."+normalized)) {
			return true
		}
	}
	return false
}

func operatorAllowsChannelDestination(parsed *url.URL) (bool, bool) {
	rawHosts := strings.Split(os.Getenv(channelAllowedHostsEnv), ",")
	requestHost, err := normalizeChannelHost(parsed.Hostname())
	if err != nil {
		return false, false
	}
	requestPort := channelURLPort(parsed)
	for _, rawHost := range rawHosts {
		rawHost = strings.TrimSpace(rawHost)
		if rawHost == "" || strings.Contains(rawHost, "://") {
			continue
		}
		allowed, err := url.Parse("//" + rawHost)
		if err != nil || allowed.Hostname() == "" || allowed.User != nil || allowed.Path != "" || allowed.RawQuery != "" || allowed.Fragment != "" {
			continue
		}
		allowedHost, err := normalizeChannelHost(allowed.Hostname())
		if err != nil || requestHost != allowedHost {
			continue
		}
		if allowed.Port() == "" {
			if isStandardChannelPort(parsed) {
				return true, true
			}
			continue
		}
		if allowed.Port() == requestPort {
			return true, true
		}
	}
	return false, false
}

func lookupChannelHost(hostname string) (string, bool, error) {
	addresses, err := common.LookupHost(hostname)
	if err != nil {
		return "", false, fmt.Errorf("could not resolve channel destination host %q: %w", hostname, err)
	}
	if len(addresses) == 0 {
		return "", false, fmt.Errorf("channel destination host %q resolved to no addresses", hostname)
	}
	resolvedIP := ""
	privateOnly := true
	for _, address := range addresses {
		ip := net.ParseIP(address)
		if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
			return "", false, fmt.Errorf("channel destination host %q resolved to an invalid address", hostname)
		}
		if ipv4 := ip.To4(); ipv4 != nil {
			ip = ipv4
		}
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return "", false, fmt.Errorf("channel destination host %q resolved to a link-local address", hostname)
		}
		if resolvedIP == "" {
			resolvedIP = ip.String()
		}
		if !ip.IsPrivate() && !ip.IsLoopback() {
			privateOnly = false
		}
	}
	return resolvedIP, privateOnly, nil
}

func resolveAllowedLocalChannelDestination(parsed *url.URL, policy channelURLPolicy, websocketURL bool) (string, string, string, bool) {
	if policy.localBaseURL == "" || policy.localPathPrefix == "" || !isSafeLocalChannelPath(parsed.EscapedPath(), policy.localPathPrefix) {
		return "", "", "", false
	}
	base, err := url.Parse(policy.localBaseURL)
	if err != nil || base.User != nil || base.Path != "" || base.RawQuery != "" || base.Fragment != "" {
		return "", "", "", false
	}
	baseHost, err := normalizeChannelHost(base.Hostname())
	if err != nil || !isLocalChannelHost(baseHost) {
		return "", "", "", false
	}
	resolvedIP, err := lookupLoopbackChannelHost(base.Hostname())
	if err != nil {
		return "", "", "", false
	}
	expectedScheme := strings.ToLower(base.Scheme)
	if websocketURL {
		switch expectedScheme {
		case "http":
			expectedScheme = "ws"
		case "https":
			expectedScheme = "wss"
		}
	}
	actualHost, err := normalizeChannelHost(parsed.Hostname())
	if err != nil || !strings.EqualFold(parsed.Scheme, expectedScheme) || actualHost != baseHost || channelURLPort(parsed) != channelURLPort(base) {
		return "", "", "", false
	}
	return actualHost, resolvedIP, channelURLPort(parsed), true
}

func isSafeLocalChannelPath(path, prefix string) bool {
	if strings.HasSuffix(prefix, "/") {
		if !strings.HasPrefix(path, prefix) {
			return false
		}
	} else if path != prefix {
		return false
	}
	if strings.ContainsAny(path, "\\") || strings.Contains(strings.ToLower(path), "%2f") || strings.Contains(strings.ToLower(path), "%5c") {
		return false
	}
	decoded, err := url.PathUnescape(path)
	if err != nil || strings.ContainsAny(decoded, "\\") {
		return false
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func isLocalChannelHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(host, "localhost")
}

func lookupLoopbackChannelHost(hostname string) (string, error) {
	addresses, err := common.LookupHost(hostname)
	if err != nil {
		return "", fmt.Errorf("could not resolve local channel destination %q: %w", hostname, err)
	}
	if len(addresses) == 0 {
		return "", fmt.Errorf("local channel destination %q resolved to no addresses", hostname)
	}
	resolvedIP := ""
	for _, address := range addresses {
		ip := net.ParseIP(address)
		if ip == nil || !ip.IsLoopback() {
			return "", fmt.Errorf("local channel destination %q did not resolve only to loopback addresses", hostname)
		}
		if resolvedIP == "" {
			resolvedIP = ip.String()
		}
	}
	return resolvedIP, nil
}

func channelURLPort(parsed *url.URL) string {
	if port := parsed.Port(); port != "" {
		return port
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "ws":
		return "80"
	case "https", "wss":
		return "443"
	default:
		return ""
	}
}

func isStandardChannelPort(parsed *url.URL) bool {
	port := channelURLPort(parsed)
	switch strings.ToLower(parsed.Scheme) {
	case "http", "ws":
		return port == "80"
	case "https", "wss":
		return port == "443"
	default:
		return false
	}
}

func isSecureChannelScheme(scheme string) bool {
	return strings.EqualFold(scheme, "https") || strings.EqualFold(scheme, "wss")
}

func normalizeChannelHost(host string) (string, error) {
	host = strings.TrimSuffix(strings.TrimSpace(strings.ToLower(host)), ".")
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}
	return idna.Lookup.ToASCII(host)
}
