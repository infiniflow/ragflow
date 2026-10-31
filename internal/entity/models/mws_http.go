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

package models

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ragflow/internal/common"
)

const mwsAPIHost = "gpt.mwsapis.ru"

type mwsDialTargetKey struct{}

type mwsDialTarget struct {
	hostname   string
	resolvedIP string
}

type mwsDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type mwsSSRFTransport struct {
	base http.RoundTripper
}

func (t *mwsSSRFTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := validateMWSRequestURL(request.URL); err != nil {
		return nil, err
	}
	hostname, resolvedIP, err := common.AssertURLSafe(request.URL.String())
	if err != nil {
		return nil, fmt.Errorf("MWS request destination is not allowed: %w", err)
	}
	if !strings.EqualFold(hostname, mwsAPIHost) || resolvedIP == "" {
		return nil, fmt.Errorf("MWS request destination is not allowed")
	}

	ctx := context.WithValue(request.Context(), mwsDialTargetKey{}, mwsDialTarget{
		hostname:   hostname,
		resolvedIP: resolvedIP,
	})
	return t.base.RoundTrip(request.Clone(ctx))
}

func validateMWSRequestURL(requestURL *url.URL) error {
	if requestURL == nil || !strings.EqualFold(requestURL.Scheme, "https") || !strings.EqualFold(requestURL.Hostname(), mwsAPIHost) {
		return fmt.Errorf("MWS requests must use https://%s", mwsAPIHost)
	}
	if port := requestURL.Port(); port != "" && port != "443" {
		return fmt.Errorf("MWS requests must use the default HTTPS port")
	}
	if requestURL.User != nil || requestURL.Opaque != "" || requestURL.RawQuery != "" || requestURL.ForceQuery || requestURL.Fragment != "" {
		return fmt.Errorf("MWS requests must not contain credentials, a query string, or a fragment")
	}
	return nil
}

func newMWSHTTPClient() *http.Client {
	var transport *http.Transport
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = defaultTransport.Clone()
	} else {
		transport = &http.Transport{}
	}
	transport.Proxy = nil
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.ResponseHeaderTimeout = 20 * time.Minute
	transport.TLSHandshakeTimeout = 30 * time.Second

	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = newMWSDialContext(dialer)

	return &http.Client{
		Transport:     common.NewProviderLoggingTransport(&mwsSSRFTransport{base: transport}),
		CheckRedirect: rejectMWSRedirect,
	}
}

func newMWSDialContext(dialer mwsDialer) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		target, ok := ctx.Value(mwsDialTargetKey{}).(mwsDialTarget)
		hostname, port, err := net.SplitHostPort(address)
		if !ok || err != nil || target.resolvedIP == "" || !strings.EqualFold(hostname, target.hostname) || !strings.EqualFold(hostname, mwsAPIHost) {
			return nil, fmt.Errorf("unvalidated MWS HTTP destination %q", address)
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(target.resolvedIP, port))
	}
}

func rejectMWSRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}
