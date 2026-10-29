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
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"ragflow/internal/common"
)

var dingTalkEgressProxy struct {
	sync.Once
	url string
	err error
}

type dingtalkProxyHandler struct {
	authorization string
}

func dingTalkSafeProxyURL() (string, error) {
	dingTalkEgressProxy.Do(func() {
		secret := make([]byte, 32)
		if _, dingTalkEgressProxy.err = rand.Read(secret); dingTalkEgressProxy.err != nil {
			return
		}
		password := hex.EncodeToString(secret)
		authorization := "Basic " + base64.StdEncoding.EncodeToString([]byte("ragflow:"+password))

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			dingTalkEgressProxy.err = fmt.Errorf("start DingTalk egress proxy: %w", err)
			return
		}
		server := &http.Server{
			Handler:           &dingtalkProxyHandler{authorization: authorization},
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			_ = server.Serve(listener)
		}()

		proxyURL := &url.URL{
			Scheme: "http",
			Host:   listener.Addr().String(),
			User:   url.UserPassword("ragflow", password),
		}
		dingTalkEgressProxy.url = proxyURL.String()
	})
	return dingTalkEgressProxy.url, dingTalkEgressProxy.err
}

func (h *dingtalkProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Proxy-Authorization")), []byte(h.authorization)) != 1 {
		http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
		return
	}

	_, resolvedIP, err := resolveDingTalkProxyDestination(r.Host)
	if err != nil {
		http.Error(w, "destination is not allowed", http.StatusForbidden)
		return
	}
	upstream, err := (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext(
		r.Context(),
		"tcp",
		net.JoinHostPort(resolvedIP, "443"),
	)
	if err != nil {
		http.Error(w, "destination is unavailable", http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "proxy connection is unavailable", http.StatusInternalServerError)
		return
	}
	client, buffered, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	if _, err = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if err = buffered.Flush(); err != nil {
		return
	}

	copyDone := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, buffered.Reader)
		copyDone <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		copyDone <- struct{}{}
	}()
	<-copyDone
}

func resolveDingTalkProxyDestination(address string) (string, string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return "", "", fmt.Errorf("invalid DingTalk proxy destination")
	}
	host, err = normalizeChannelHost(host)
	if err != nil {
		return "", "", fmt.Errorf("invalid DingTalk proxy host")
	}
	if port != "443" {
		return "", "", fmt.Errorf("DingTalk proxy destination must use port 443")
	}
	if host != "api.dingtalk.com" && host != "wss-open-connection.dingtalk.com" {
		return "", "", fmt.Errorf("DingTalk proxy host %q is not trusted", host)
	}
	resolvedIP, err := common.AssertHostSafe(host)
	if err != nil {
		return "", "", fmt.Errorf("DingTalk proxy host %q is not safe: %w", host, err)
	}
	return host, resolvedIP, nil
}
