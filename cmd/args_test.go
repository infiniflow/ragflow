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

package main

import (
	"reflect"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/server/bootstrap"
)

func TestCommonArguments(t *testing.T) {
	want, err := parseArgsFrom([]string{"server", "--config", "local.yaml", "--log-level", "debug", "--profile"})
	if err != nil {
		t.Fatal(err)
	}
	for _, argv := range [][]string{
		{"server", "-f", "local.yaml", "--log-level=debug", "--profile"},
	} {
		got, err := parseArgsFrom(argv)
		if err != nil {
			t.Fatalf("%v: %v", argv, err)
		}
		// kingpin allocates a fresh pointer for every flag registration, so
		// compare the pointed-to values instead of struct identity.
		gotVals := []any{*got.LogLevel, *got.ConfigPath, *got.DeepdocDevice, *got.EnablePProf, got.InitSuperUser}
		wantVals := []any{*want.LogLevel, *want.ConfigPath, *want.DeepdocDevice, *want.EnablePProf, want.InitSuperUser}
		if !reflect.DeepEqual(gotVals, wantVals) {
			t.Fatalf("%v: got values %+v, want %+v (structs: %+v vs %+v)", argv, gotVals, wantVals, got, want)
		}
	}
	for _, flag := range []string{"--help", "-h", "--version", "-v"} {
		args, err := parseArgsFrom([]string{"server", flag})
		if err != nil || !(*args.HelpFlag || *args.VersionFlag) {
			t.Fatalf("%s: args=%+v, err=%v", flag, args, err)
		}
	}
	for _, argv := range [][]string{{"--config"}, {"--log-level=trace"}, {"--profile=true"}, {"unexpected"}} {
		if _, err := parseArgsFrom(append([]string{"server"}, argv...)); err == nil {
			t.Fatalf("accepted invalid arguments %v", argv)
		}
	}
}

func TestServerModeArguments(t *testing.T) {
	for _, tt := range []struct {
		mode       string
		serverType common.ServerType
	}{
		{"admin", common.ServerTypeAdmin},
		{"api", common.ServerTypeAPI},
		{"ingestor", common.ServerTypeIngestion},
		{"syncer", common.ServerTypeFileSyncer},
		{"chatter", common.ServerTypeChatter},
		{"migrate", common.ServerTypeMigrate},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			for _, flag := range []string{"--" + tt.mode} {
				args, err := parseArgsFrom([]string{"server", flag, "--config", "local.yaml"})
				if err != nil {
					t.Fatalf("%s: %v", flag, err)
				}
				if args.Mode == nil || *args.Mode != tt.mode {
					t.Fatalf("%s: mode = %v, want %s", flag, args.Mode, tt.mode)
				}
				if got := bootstrap.GetServerType(args); got != tt.serverType {
					t.Errorf("%s: server type = %q, want %q", flag, got, tt.serverType)
				}
				if args.ConfigPath == nil || *args.ConfigPath != "local.yaml" {
					t.Errorf("%s: configuration path was not preserved", flag)
				}
				if args.InitSuperUser != (tt.mode == "admin") {
					t.Errorf("%s: InitSuperUser = %v, want %v", flag, args.InitSuperUser, tt.mode == "admin")
				}
			}
		})
	}
}

func TestDisabledServerModeArguments(t *testing.T) {
	for _, flags := range [][]string{
		nil,
		{"--no-migrate"},
		{"--no-admin", "--no-api", "--no-ingestor", "--no-syncer", "--no-chatter", "--no-migrate"},
	} {
		args, err := parseArgsFrom(append([]string{"server"}, flags...))
		if err != nil {
			t.Fatal(err)
		}
		if args.Mode != nil || args.InitSuperUser {
			t.Errorf("%v: expected default startup arguments, got %+v", flags, args)
		}
	}
	args, err := parseArgsFrom([]string{"server", "--no-api", "--migrate"})
	if err != nil || args.Mode == nil || *args.Mode != "migrate" {
		t.Fatalf("disabled API flag affected migration selection: args=%+v, err=%v", args, err)
	}
}

func TestConflictingServerModeArguments(t *testing.T) {
	modes := []string{"admin", "api", "ingestor", "syncer", "chatter", "migrate"}
	for i, first := range modes {
		for _, second := range modes[i+1:] {
			if _, err := parseArgsFrom([]string{"server", "--" + first, "--" + second}); err == nil {
				t.Errorf("accepted conflicting modes --%s and --%s", first, second)
			}
		}
	}
}
