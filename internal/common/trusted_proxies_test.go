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
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func clientIPWith(t *testing.T, proxies []string, remoteAddr, forwardedFor string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, engine := gin.CreateTestContext(httptest.NewRecorder())
	if err := ConfigureTrustedProxies(engine, proxies); err != nil {
		t.Fatalf("ConfigureTrustedProxies(%v): %v", proxies, err)
	}
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		c.Request.Header.Set("X-Forwarded-For", forwardedFor)
	}
	return c.ClientIP()
}

func TestConfigureTrustedProxies(t *testing.T) {
	for _, test := range []struct {
		name         string
		proxies      []string
		remoteAddr   string
		forwardedFor string
		want         string
	}{
		{name: "default trusts loopback nginx", proxies: nil, remoteAddr: "127.0.0.1:0", forwardedFor: "203.0.113.7", want: "203.0.113.7"},
		{name: "default ignores headers from other peers", proxies: nil, remoteAddr: "192.168.1.5:0", forwardedFor: "203.0.113.7", want: "192.168.1.5"},
		{name: "default stops at first undeclared hop", proxies: nil, remoteAddr: "127.0.0.1:0", forwardedFor: "203.0.113.7, 198.51.100.9", want: "198.51.100.9"},
		{name: "configured proxy honoured", proxies: []string{"10.0.0.0/8"}, remoteAddr: "10.1.2.3:0", forwardedFor: "203.0.113.7", want: "203.0.113.7"},
		{name: "configured list replaces loopback", proxies: []string{"10.0.0.0/8"}, remoteAddr: "127.0.0.1:0", forwardedFor: "203.0.113.7", want: "127.0.0.1"},
		{name: "empty list trusts nobody", proxies: []string{}, remoteAddr: "127.0.0.1:0", forwardedFor: "203.0.113.7", want: "127.0.0.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := clientIPWith(t, test.proxies, test.remoteAddr, test.forwardedFor); got != test.want {
				t.Fatalf("ClientIP = %q, want %q", got, test.want)
			}
		})
	}
}

func TestConfigureTrustedProxiesRejectsInvalidEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if err := ConfigureTrustedProxies(gin.New(), []string{"not-an-ip"}); err == nil {
		t.Fatal("expected an error for an unparsable trusted proxy entry")
	}
}
