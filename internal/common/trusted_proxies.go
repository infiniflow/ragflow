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
	"fmt"

	"github.com/gin-gonic/gin"
)

// DefaultTrustedProxies is the proxy set whose X-Forwarded-For / X-Real-IP
// headers gin honours when no `ragflow.trusted_proxies` list is configured.
// It covers the nginx that docker/entrypoint.sh starts inside the same
// container as this server: docker/nginx/ragflow.conf.golang proxies /v1 and
// /api to 127.0.0.1:9384 and proxy.conf appends the caller to
// X-Forwarded-For. Nothing else is trusted, so a request that reaches the
// server from any other peer is attributed to that peer regardless of the
// headers it carries.
var DefaultTrustedProxies = []string{"127.0.0.0/8", "::1/128"}

// ConfigureTrustedProxies replaces gin's default trust-everything proxy list
// (0.0.0.0/0, ::/0) with proxies, falling back to DefaultTrustedProxies when
// proxies is nil. An explicitly empty list disables header-based client IP
// resolution entirely, making c.ClientIP() the socket peer for every request.
// Every c.ClientIP() consumer (the agent webhook ip_whitelist gate, login
// audit records, access logs) depends on this being applied to the engine.
func ConfigureTrustedProxies(engine *gin.Engine, proxies []string) error {
	if proxies == nil {
		proxies = DefaultTrustedProxies
	}
	if err := engine.SetTrustedProxies(proxies); err != nil {
		return fmt.Errorf("invalid trusted_proxies %v: %w", proxies, err)
	}
	return nil
}
