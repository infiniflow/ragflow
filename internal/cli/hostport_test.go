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

package cli

import "testing"

// TestParseHostPortIPv6 pins bracketed IPv6 parsing, which the previous
// strings.Split("host:port") implementation rejected as too many segments.
func TestParseHostPortIPv6(t *testing.T) {
	host, port, err := parseHostPort("[::1]:8080")
	if err != nil {
		t.Fatalf("parseHostPort IPv6: %v", err)
	}
	if host != "::1" {
		t.Errorf("host = %q, want ::1", host)
	}
	if port != 8080 {
		t.Errorf("port = %d, want 8080", port)
	}
}

// TestParseHostPortIPv4AndEmpty preserves the existing accepted forms.
func TestParseHostPortIPv4AndEmpty(t *testing.T) {
	host, port, err := parseHostPort("127.0.0.1:9300")
	if err != nil || host != "127.0.0.1" || port != 9300 {
		t.Fatalf("IPv4 parse = (%q, %d, %v)", host, port, err)
	}
	host, port, err = parseHostPort("")
	if err != nil || host != "" || port != -1 {
		t.Fatalf("empty parse = (%q, %d, %v)", host, port, err)
	}
}
