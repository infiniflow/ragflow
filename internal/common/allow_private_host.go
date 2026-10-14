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
	"net"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// AllowConfiguredPrivateHost reports whether ALLOW_ANY_HOST is set to a
// truthy value (1, true, yes, on), matching the Python host guard.
//
// The flag is the development opt-in documented in docker/.env. It applies
// only to host-based database and connector dials (MySQL, PostgreSQL, IMAP,
// test_db_connection, ExeSQL), including host.docker.internal. URL and Invoke
// guards do not call this, so the flag cannot turn off DNS pinning for
// user-controlled URLs. Default is off.
func AllowConfiguredPrivateHost() bool {
	switch strings.ToLower(strings.TrimSpace(GetEnv(EnvAllowAnyHost))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

var noteAllowConfiguredPrivateHost sync.Once

// noteConfiguredPrivateHost logs once that private hosts are permitted.
func noteConfiguredPrivateHost(host string) {
	noteAllowConfiguredPrivateHost.Do(func() {
		zap.L().Warn("ALLOW_ANY_HOST is enabled; private database and connector hosts are allowed",
			zap.String("host", host),
		)
	})
}

// PinConfiguredPrivateHost is the shared host-dial opt-in. When ALLOW_ANY_HOST
// is unset, enabled is false and the caller keeps the strict public-address
// check. When it is set, the host is resolved and the dial must use the
// returned address. enabled stays true when resolution fails.
func PinConfiguredPrivateHost(host string) (pinned string, enabled bool, err error) {
	if !AllowConfiguredPrivateHost() {
		return "", false, nil
	}
	noteConfiguredPrivateHost(host)
	pinned, err = ResolveHostPin(host)
	return pinned, true, err
}

// ResolveHostPin resolves host and returns the first address without checking
// whether it is globally routable. Callers dial the returned IP so a later
// DNS change cannot rebind the connection.
func ResolveHostPin(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("host is missing")
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}

	addresses, err := LookupHost(host)
	if err != nil {
		return "", fmt.Errorf("could not resolve hostname '%s': %w", host, err)
	}
	if len(addresses) == 0 {
		return "", fmt.Errorf("hostname '%s' resolved to no addresses", host)
	}

	var resolved string
	for _, addr := range addresses {
		ip := net.ParseIP(addr)
		if ip == nil {
			return "", fmt.Errorf("could not parse resolved address '%s' for hostname '%s'", addr, host)
		}
		if resolved == "" {
			resolved = ip.String()
		}
	}
	return resolved, nil
}
