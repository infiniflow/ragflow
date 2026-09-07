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

package cmdargs

import (
	"strings"
	"testing"
)

// The tests below pin the --migrate standalone-mode contract introduced in
// #19270: --migrate alone resolves to mode="migrate", --migrate combined
// with another mode is rejected, and every previously-supported mode parses
// to the expected ServerArgs shape.
func TestParseFrom(t *testing.T) {
	t.Run("--migrate alone resolves to standalone migrate mode", func(t *testing.T) {
		got, err := ParseFrom([]string{"ragflow_server", "--migrate"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Mode == nil || *got.Mode != "migrate" {
			t.Fatalf("Mode = %v, want \"migrate\"", got.Mode)
		}
		if !got.MigrateDB {
			t.Fatalf("MigrateDB = false, want true (--migrate implies migration)")
		}
	})

	t.Run("--migrate combined with --admin is rejected", func(t *testing.T) {
		_, err := ParseFrom([]string{"ragflow_server", "--admin", "--migrate"})
		if err == nil {
			t.Fatal("expected error for --migrate combined with --admin, got nil")
		}
		if !strings.Contains(err.Error(), "--migrate") || !strings.Contains(err.Error(), "--admin") {
			t.Fatalf("error message %q should mention both --migrate and --admin", err)
		}
	})

	t.Run("--migrate combined with --api is rejected", func(t *testing.T) {
		_, err := ParseFrom([]string{"ragflow_server", "--api", "--migrate"})
		if err == nil {
			t.Fatal("expected error for --migrate combined with --api, got nil")
		}
	})

	t.Run("--migrate combined with --ingestor is rejected", func(t *testing.T) {
		_, err := ParseFrom([]string{"ragflow_server", "--ingestor", "--migrate"})
		if err == nil {
			t.Fatal("expected error for --migrate combined with --ingestor, got nil")
		}
	})

	t.Run("--migrate combined with --syncer is rejected", func(t *testing.T) {
		_, err := ParseFrom([]string{"ragflow_server", "--syncer", "--migrate"})
		if err == nil {
			t.Fatal("expected error for --migrate combined with --syncer, got nil")
		}
	})

	t.Run("--admin alone still parses to mode=admin without --migrate", func(t *testing.T) {
		got, err := ParseFrom([]string{"ragflow_server", "--admin"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Mode == nil || *got.Mode != "admin" {
			t.Fatalf("Mode = %v, want \"admin\"", got.Mode)
		}
		if got.MigrateDB {
			t.Fatalf("MigrateDB = true, want false (no --migrate flag)")
		}
	})

	t.Run("--api --port 9381 parses to mode=api with port=9381", func(t *testing.T) {
		got, err := ParseFrom([]string{"ragflow_server", "--api", "--port", "9381"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Mode == nil || *got.Mode != "api" {
			t.Fatalf("Mode = %v, want \"api\"", got.Mode)
		}
		if got.Port == nil || *got.Port != 9381 {
			t.Fatalf("Port = %v, want 9381", got.Port)
		}
	})

	t.Run("--admin --admin-host 1.2.3.4:9381 parses the host:port pair", func(t *testing.T) {
		got, err := ParseFrom([]string{"ragflow_server", "--admin", "--admin-host", "1.2.3.4:9381"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.AdminHost == nil || *got.AdminHost != "1.2.3.4" {
			t.Fatalf("AdminHost = %v, want \"1.2.3.4\"", got.AdminHost)
		}
		if got.AdminPort == nil || *got.AdminPort != 9381 {
			t.Fatalf("AdminPort = %v, want 9381", got.AdminPort)
		}
	})

	t.Run("unknown parameter returns an error mentioning the flag", func(t *testing.T) {
		_, err := ParseFrom([]string{"ragflow_server", "--bogus"})
		if err == nil {
			t.Fatal("expected error for unknown parameter, got nil")
		}
		if !strings.Contains(err.Error(), "--bogus") {
			t.Fatalf("error message %q should mention the unknown flag", err)
		}
	})

	t.Run("no mode flag still leaves Mode nil (caller treats as --api per help)", func(t *testing.T) {
		got, err := ParseFrom([]string{"ragflow_server"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Mode != nil {
			t.Fatalf("Mode = %v, want nil when no mode flag is given", got.Mode)
		}
	})
}
