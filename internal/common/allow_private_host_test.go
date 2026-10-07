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
	"errors"
	"testing"
)

// TestAllowConfiguredPrivateHost checks the ALLOW_ANY_HOST truthy values
// accepted by the Python host guard.
func TestAllowConfiguredPrivateHost(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{value: "", want: false},
		{value: "0", want: false},
		{value: "false", want: false},
		{value: "no", want: false},
		{value: "off", want: false},
		{value: "1", want: true},
		{value: "true", want: true},
		{value: "TRUE", want: true},
		{value: " yes ", want: true},
		{value: "on", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv(EnvAllowAnyHost, tc.value)
			if got := AllowConfiguredPrivateHost(); got != tc.want {
				t.Fatalf("AllowConfiguredPrivateHost(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// TestResolveHostPinSkipsPublicCheck checks that a private resolution is
// returned as the pin, and that empty or unresolvable hosts still fail.
func TestResolveHostPinSkipsPublicCheck(t *testing.T) {
	orig := LookupHost
	LookupHost = func(host string) ([]string, error) {
		if host == "host.docker.internal" {
			return []string{"192.168.65.254", "10.0.0.2"}, nil
		}
		return nil, errors.New("no such host")
	}
	t.Cleanup(func() { LookupHost = orig })

	got, err := ResolveHostPin("host.docker.internal")
	if err != nil {
		t.Fatalf("ResolveHostPin: %v", err)
	}
	if got != "192.168.65.254" {
		t.Fatalf("ResolveHostPin = %q, want 192.168.65.254", got)
	}

	literal, err := ResolveHostPin("10.1.2.3")
	if err != nil || literal != "10.1.2.3" {
		t.Fatalf("ResolveHostPin(literal) = %q, %v", literal, err)
	}
	if _, err = ResolveHostPin("missing.internal"); err == nil {
		t.Fatal("unresolvable host must still fail")
	}
	if _, err = ResolveHostPin("  "); err == nil {
		t.Fatal("empty host must still fail")
	}
}
