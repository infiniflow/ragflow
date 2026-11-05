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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"ragflow/internal/common"
)

func allowChannelURLForTest(t *testing.T, rawURL string) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test endpoint: %v", err)
	}
	allowed := strings.TrimSpace(os.Getenv(channelAllowedHostsEnv))
	if allowed != "" {
		allowed += ","
	}
	t.Setenv(channelAllowedHostsEnv, allowed+parsed.Host)
}

func TestSafeChannelHTTPClientRejectsUntrustedHost(t *testing.T) {
	client := newSafeChannelHTTPClient(time.Second, channelURLPolicy{})
	_, err := client.Get("https://attacker.example/path")
	if err == nil || !strings.Contains(err.Error(), "is not trusted") {
		t.Fatalf("Get() error = %v, want an untrusted-host rejection", err)
	}
}

func TestSafeChannelHTTPClientRejectsPrivateOfficialHost(t *testing.T) {
	t.Setenv(channelAllowedHostsEnv, "")
	client := newSafeChannelHTTPClient(time.Second, channelURLPolicy{exactHosts: []string{"127.0.0.1"}})
	_, err := client.Get("https://127.0.0.1/")
	if err == nil || !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("Get() error = %v, want a private-address rejection", err)
	}
}

func TestSafeChannelHTTPClientPinsOperatorAllowedHost(t *testing.T) {
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	targetURL := fmt.Sprintf("http://channel-gateway.test:%s/ping", serverURL.Port())
	allowChannelURLForTest(t, targetURL)

	lookupHost := common.LookupHost
	common.LookupHost = func(host string) ([]string, error) {
		if host == "channel-gateway.test" {
			return []string{"127.0.0.1"}, nil
		}
		return lookupHost(host)
	}
	t.Cleanup(func() { common.LookupHost = lookupHost })

	client := newSafeChannelHTTPClient(time.Second, channelURLPolicy{})
	resp, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	_ = resp.Body.Close()
	select {
	case host := <-requests:
		if host != "channel-gateway.test:"+serverURL.Port() {
			t.Fatalf("request Host = %q", host)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not receive the pinned request")
	}
}

func TestSafeChannelHTTPClientDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Store(true)
	}))
	defer destination.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer source.Close()
	allowChannelURLForTest(t, source.URL)

	client := newSafeChannelHTTPClient(time.Second, channelURLPolicy{})
	resp, err := client.Get(source.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("response status = %d, want %d", resp.StatusCode, http.StatusFound)
	}
	if redirected.Load() {
		t.Fatal("client followed a redirect")
	}
}

func TestSafeChannelWebSocketRejectsUntrustedLocalHost(t *testing.T) {
	t.Setenv(channelAllowedHostsEnv, "")
	_, _, err := dialSafeChannelWebSocket(
		context.Background(),
		"ws://127.0.0.1/whatsapp/test/events/ws",
		nil,
		time.Second,
		channelURLPolicy{},
	)
	if err == nil || !strings.Contains(err.Error(), "is not trusted") {
		t.Fatalf("dialSafeChannelWebSocket() error = %v, want an untrusted-host rejection", err)
	}
}

func TestChannelSDKWebSocketDialerOnlyAllowsTLSPlatformEndpoints(t *testing.T) {
	dialer := newChannelSDKWebSocketDialer(&websocket.Dialer{})
	for _, test := range []struct {
		url      string
		wantFail bool
	}{
		{url: "wss://msg-frontier.feishu.cn/ws/v2"},
		{url: "wss://msg-frontier.larksuite.com/ws/v2"},
		{url: "wss://wss-open-connection.dingtalk.com/connect"},
		{url: "ws://msg-frontier.feishu.cn:443/ws/v2", wantFail: true},
		{url: "wss://attacker.example/connect", wantFail: true},
		{url: "wss://wss-open-connection.dingtalk.com:8443/connect", wantFail: true},
	} {
		t.Run(test.url, func(t *testing.T) {
			parsed, err := url.Parse(test.url)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Scheme == "wss" {
				parsed.Scheme = "https"
			} else if parsed.Scheme == "ws" {
				parsed.Scheme = "http"
			}
			_, err = dialer.Proxy(&http.Request{URL: parsed})
			if (err != nil) != test.wantFail {
				t.Fatalf("Proxy() error = %v, wantFail %t", err, test.wantFail)
			}
		})
	}
}

func TestChannelSDKWebSocketDialerRejectsNonPublicResolution(t *testing.T) {
	originalLookup := common.LookupHost
	t.Cleanup(func() { common.LookupHost = originalLookup })
	dialer := newChannelSDKWebSocketDialer(&websocket.Dialer{})

	for _, address := range []string{"169.254.169.254", "::ffff:169.254.169.254", "fe80::1"} {
		t.Run(address, func(t *testing.T) {
			common.LookupHost = func(host string) ([]string, error) {
				if host != "msg-frontier.feishu.cn" {
					t.Fatalf("unexpected DNS lookup host %q", host)
				}
				return []string{address}, nil
			}
			_, err := dialer.NetDialContext(context.Background(), "tcp", "msg-frontier.feishu.cn:443")
			if err == nil || !strings.Contains(err.Error(), "not safe") {
				t.Fatalf("NetDialContext() error = %v, want unsafe-address rejection", err)
			}
		})
	}
}

func TestLookupChannelHostRejectsLinkLocalAddresses(t *testing.T) {
	originalLookup := common.LookupHost
	t.Cleanup(func() { common.LookupHost = originalLookup })

	for _, address := range []string{"169.254.169.254", "::ffff:169.254.169.254", "fe80::1"} {
		t.Run(address, func(t *testing.T) {
			common.LookupHost = func(string) ([]string, error) { return []string{address}, nil }
			if _, _, err := lookupChannelHost("channel-gateway.test"); err == nil || !strings.Contains(err.Error(), "link-local") {
				t.Fatalf("lookupChannelHost() error = %v, want link-local rejection", err)
			}
		})
	}
}

func TestOperatorAllowedChannelRejectsLinkLocalForAllSchemes(t *testing.T) {
	t.Setenv(channelAllowedHostsEnv, "channel-gateway.test")
	originalLookup := common.LookupHost
	t.Cleanup(func() { common.LookupHost = originalLookup })
	common.LookupHost = func(string) ([]string, error) { return []string{"169.254.169.254"}, nil }

	for _, scheme := range []string{"http", "https", "ws", "wss"} {
		t.Run(scheme, func(t *testing.T) {
			parsed, err := url.Parse(scheme + "://channel-gateway.test/path")
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, err = resolveChannelDestination(parsed, channelURLPolicy{}, scheme == "ws" || scheme == "wss")
			if err == nil || !strings.Contains(err.Error(), "link-local") {
				t.Fatalf("resolveChannelDestination() error = %v, want link-local rejection", err)
			}
		})
	}
}

func TestWhatsAppDefaultGatewayExceptionIsPathScoped(t *testing.T) {
	t.Setenv(channelAllowedHostsEnv, "")
	policy := channelURLPolicy{
		localBaseURL:    defaultGatewayBaseURL,
		localPathPrefix: "/whatsapp/",
	}
	allowed, err := url.Parse("ws://127.0.0.1:3005/whatsapp/test/events/ws")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := resolveChannelDestination(allowed, policy, true); err != nil {
		t.Fatalf("default WhatsApp gateway rejected: %v", err)
	}

	for _, path := range []string{"/admin", "/whatsapp/../admin", "/whatsapp/%2e%2e/admin", "/whatsapp/%2fadmin"} {
		disallowed, err := url.Parse("ws://127.0.0.1:3005" + path)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := resolveChannelDestination(disallowed, policy, true); err == nil {
			t.Errorf("default WhatsApp exception accepted unsafe path %q", path)
		}
	}
}

func TestWhatsAppExplicitDefaultGatewayIsAllowed(t *testing.T) {
	channel := newWhatsAppChannel(whatsappAccount{GatewayBaseURL: defaultGatewayBaseURL})
	if !channel.allowDefaultGateway {
		t.Fatal("explicit default gateway was not recognized as the built-in local gateway")
	}
}

func TestSafeChannelWebSocketPinsOperatorAllowedHost(t *testing.T) {
	upgrader := websocket.Upgrader{}
	connected := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		connected <- struct{}{}
		_ = conn.Close()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	allowChannelURLForTest(t, wsURL)
	conn, _, err := dialSafeChannelWebSocket(context.Background(), wsURL, nil, time.Second, channelURLPolicy{})
	if err != nil {
		t.Fatalf("dialSafeChannelWebSocket() error = %v", err)
	}
	_ = conn.Close()
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("WebSocket server did not receive the pinned connection")
	}
}
