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

package admin

import (
	"os"
	"strings"
	"testing"

	"ragflow/internal/common"
)

func TestConfiguredSuperuserPassword(t *testing.T) {
	tests := []struct {
		name       string
		canonical  string
		adminAlias string
		want       string
		ok         bool
	}{
		{name: "both unset"},
		{name: "docker alias", adminAlias: "from-dotenv", want: "from-dotenv", ok: true},
		{name: "canonical name", canonical: "from-config", want: "from-config", ok: true},
		{name: "canonical wins when both are set", canonical: "from-config", adminAlias: "from-dotenv", want: "from-config", ok: true},
		{name: "blank alias is unset", adminAlias: "   "},
		{name: "blank canonical uses alias", canonical: " \t", adminAlias: "from-dotenv", want: "from-dotenv", ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(common.EnvDefaultSuperuserPassword, tt.canonical)
			t.Setenv(common.EnvAdminDefaultPassword, tt.adminAlias)
			got, ok := configuredSuperuserPassword()
			if ok != tt.ok || got != tt.want {
				t.Fatalf("configuredSuperuserPassword() = %q, %v, want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestUnsetSuperuserPasswordWritesBootstrapFile(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(common.EnvDefaultSuperuserPassword, "")
	t.Setenv(common.EnvAdminDefaultPassword, "   ")

	got, err := superuserPasswordForNewAdmin()
	if err != nil {
		t.Fatalf("superuserPasswordForNewAdmin() error = %v", err)
	}
	if got == "" || got == "admin" {
		t.Fatalf("superuserPasswordForNewAdmin() = %q, want a generated password", got)
	}

	data, err := os.ReadFile(adminBootstrapPasswordFile)
	if err != nil {
		t.Fatalf("read bootstrap password: %v", err)
	}
	if strings.TrimSpace(string(data)) != got {
		t.Fatalf("bootstrap file = %q, want %q", data, got)
	}
	info, err := os.Stat(adminBootstrapPasswordFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("bootstrap file mode = %o, want 600", info.Mode().Perm())
	}

	again, err := superuserPasswordForNewAdmin()
	if err != nil {
		t.Fatalf("second superuserPasswordForNewAdmin() error = %v", err)
	}
	if again != got {
		t.Fatalf("second password = %q, want the password already written", again)
	}
}

func TestExistingBootstrapPasswordIsReused(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(common.EnvDefaultSuperuserPassword, "")
	t.Setenv(common.EnvAdminDefaultPassword, "")
	if err := os.MkdirAll("logs", 0o755); err != nil {
		t.Fatal(err)
	}
	const stored = "already-generated-secret"
	if err := os.WriteFile(adminBootstrapPasswordFile, []byte(stored+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := superuserPasswordForNewAdmin()
	if err != nil {
		t.Fatal(err)
	}
	if got != stored {
		t.Fatalf("superuserPasswordForNewAdmin() = %q, want %q", got, stored)
	}
}

func TestConfiguredPasswordSkipsBootstrapFile(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(common.EnvDefaultSuperuserPassword, "")
	t.Setenv(common.EnvAdminDefaultPassword, "from-dotenv")

	got, err := superuserPasswordForNewAdmin()
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-dotenv" {
		t.Fatalf("superuserPasswordForNewAdmin() = %q, want from-dotenv", got)
	}
	if _, err := os.Stat(adminBootstrapPasswordFile); !os.IsNotExist(err) {
		t.Fatalf("bootstrap file stat err = %v, want not exist", err)
	}
}

func TestBootstrapPasswordWriteFailureRefusesAdmin(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(common.EnvDefaultSuperuserPassword, "")
	t.Setenv(common.EnvAdminDefaultPassword, "")
	if err := os.WriteFile("logs", []byte("not-a-directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := superuserPasswordForNewAdmin()
	if err == nil {
		t.Fatal("expected an error when the bootstrap password file cannot be written")
	}
	if got != "" {
		t.Fatalf("superuserPasswordForNewAdmin() = %q, want no password", got)
	}
}
