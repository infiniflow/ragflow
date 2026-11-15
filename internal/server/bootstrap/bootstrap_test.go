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

package bootstrap

import (
	"ragflow/internal/common"
	"strings"
	"testing"
)

func strPtr(s string) *string { return &s }

// TestResolveServerNameDefaults pins the no-name default for every known mode.
// api/admin embed the port; ingestor/syncer/deepdoc use the pre-config
// "<mode>_server" placeholder (port is NOT part of the name); migrate is a
// fixed literal.
func TestResolveServerNameDefaults(t *testing.T) {
	tests := []struct {
		mode string
		port int
		want string
	}{
		{"api", 9380, "api_server_9380"},
		{"api", 0, "api_server_0"},
		{"admin", 9381, "admin_server_9381"},
		{"ingestor", 0, "ingestor_server"},
		{"syncer", 0, "syncer_server"},
		{"deepdoc", 0, "deepdoc_server"},
		{"migrate", 0, "migrate"},
	}
	for _, tc := range tests {
		got, err := ResolveServerName(tc.mode, nil, tc.port)
		if err != nil {
			t.Errorf("ResolveServerName(%q, nil, %d) error = %v", tc.mode, tc.port, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ResolveServerName(%q, nil, %d) = %q, want %q", tc.mode, tc.port, got, tc.want)
		}
	}
}

// TestResolveServerNamePortIgnoredForNonHTTPModes verifies the port argument
// only affects api/admin; it must not leak into ingestor/syncer/deepdoc/migrate
// names.
func TestResolveServerNamePortIgnoredForNonHTTPModes(t *testing.T) {
	for _, mode := range []string{"ingestor", "syncer", "deepdoc", "migrate"} {
		got, err := ResolveServerName(mode, nil, 65535)
		if err != nil {
			t.Fatalf("ResolveServerName(%q, nil, 65535) error = %v", mode, err)
		}
		if got == "api_server_65535" || got == "admin_server_65535" {
			t.Errorf("mode %q unexpectedly embedded the port: %q", mode, got)
		}
	}
}

// TestResolveServerNameNameOverrideWins verifies an explicit --name overrides
// the derived default for every known mode.
func TestResolveServerNameNameOverrideWins(t *testing.T) {
	for _, mode := range []string{"api", "admin", "ingestor", "syncer", "deepdoc", "migrate"} {
		got, err := ResolveServerName(mode, strPtr("custom_name"), 1234)
		if err != nil {
			t.Fatalf("ResolveServerName(%q, name, 1234) error = %v", mode, err)
		}
		if got != "custom_name" {
			t.Errorf("ResolveServerName(%q, name) = %q, want custom_name", mode, got)
		}
	}
}

// TestResolveServerNameEmptyNameOverride: an explicit empty --name (pointer to
// "") must be honored as an empty name, not fall through to the default. This
// mirrors the original precedence where a non-nil pointer always wins.
func TestResolveServerNameEmptyNameOverride(t *testing.T) {
	got, err := ResolveServerName("api", strPtr(""), 9380)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got != "" {
		t.Errorf("ResolveServerName(api, \"\") = %q, want empty", got)
	}
}

// TestResolveServerNameUnknownModeErrors verifies an unrecognized mode is a hard
// error regardless of whether a name was supplied (matching the original
// default branch, which always exits).
func TestResolveServerNameUnknownModeErrors(t *testing.T) {
	for _, name := range []*string{nil, strPtr("whatever")} {
		got, err := ResolveServerName("bogus", name, 0)
		if err == nil {
			t.Errorf("ResolveServerName(bogus, %v) error = nil, want error (got %q)", name, got)
		}
	}
	// Empty mode is also invalid.
	if _, err := ResolveServerName("", nil, 0); err == nil {
		t.Error("ResolveServerName(\"\") error = nil, want error")
	}
}

// TestResolveServerNameWorkerModesStableNoUUID guards the contract that
// ingestor/syncer/deepdoc without --name resolve to the stable "<mode>_server"
// literal. The historical code had a UUID branch keyed on `if serverName == ""`
// that was dead (the name was already pre-filled), so those servers must never
// get a per-boot UUID suffix in their log filename. This test fails (and must
// stay failing) if anyone reintroduces UUID generation for these modes.
func TestResolveServerNameWorkerModesStableNoUUID(t *testing.T) {
	for _, mode := range []string{"ingestor", "syncer", "deepdoc"} {
		got, err := ResolveServerName(mode, nil, 0)
		if err != nil {
			t.Fatalf("ResolveServerName(%q, nil, 0) error = %v", mode, err)
		}
		want := mode + "_server"
		if got != want {
			t.Errorf("ResolveServerName(%q, nil, 0) = %q, want stable literal %q", mode, got, want)
		}
		// A UUID suffix looks like a 8-4-4-4-12 hex run; assert it is absent.
		if strings.Contains(got, "-") {
			t.Errorf("ResolveServerName(%q, nil, 0) = %q contains '-' (UUID suffix leaked into worker server name)", mode, got)
		}
	}
}

// TestTempServerName verifies the pre-config placeholder used for the temporary
// stdout logger: explicit name wins, otherwise "<mode>_server".
func TestTempServerName(t *testing.T) {
	if got := TempServerName("api", strPtr("named")); got != "named" {
		t.Errorf("TempServerName(api, named) = %q, want named", got)
	}
	for _, serverType := range []common.ServerType{common.ServerTypeAPI, common.ServerTypeAdmin, common.ServerTypeIngestion, common.ServerTypeFileSyncer, common.ServerTypeDeepDocCPU, common.ServerTypeDeepDocGPU, common.ServerTypeDeepDocAscend, common.ServerTypeMigrate} {
		got := TempServerName(serverType, nil)
		if want := string(serverType) + "_server"; got != want {
			t.Errorf("TempServerName(%q, nil) = %q, want %q", serverType, got, want)
		}
	}
	// Empty-name override is honored as empty (non-nil pointer wins).
	if got := TempServerName("api", strPtr("")); got != "" {
		t.Errorf("TempServerName(api, \"\") = %q, want empty", got)
	}
}
