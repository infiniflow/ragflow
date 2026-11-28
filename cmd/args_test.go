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
	"fmt"
	"reflect"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/server/bootstrap"
	"ragflow/internal/server/config"
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

func TestPortArguments(t *testing.T) {
	args, err := parseArgsFrom([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	if args.Port != nil {
		t.Fatalf("omitted --port = %v, want nil", args.Port)
	}
	for _, mode := range []string{"", "--api", "--admin"} {
		for _, port := range []int{1, 9400, 65535} {
			for _, flags := range [][]string{{"--port", fmt.Sprint(port)}, {fmt.Sprintf("--port=%d", port)}} {
				argv := []string{"server"}
				if mode != "" {
					argv = append(argv, mode)
				}
				args, err := parseArgsFrom(append(argv, flags...))
				if err != nil {
					t.Fatalf("%v %v: %v", argv, flags, err)
				}
				if args.Port == nil || *args.Port != port {
					t.Fatalf("%v %v: port = %v, want %d", argv, flags, args.Port, port)
				}
			}
		}
	}
	for _, flag := range []string{"--port", "--port=", "--port=abc", "--port=-1", "--port=0", "--port=65536"} {
		if _, err := parseArgsFrom([]string{"server", flag}); err == nil {
			t.Errorf("accepted invalid argument %s", flag)
		}
	}
}

func TestLogLevelArguments(t *testing.T) {
	for _, configured := range []string{"", "debug", "info", "warn", "error"} {
		for _, level := range []string{"", "debug", "info", "warn", "error"} {
			argv := []string{"server"}
			if level != "" {
				argv = append(argv, "--log-level", level)
			}
			args, err := parseArgsFrom(argv)
			if err != nil {
				t.Fatal(err)
			}
			if level == "" && args.LogLevel != nil {
				t.Fatalf("omitted --log-level = %v, want nil", args.LogLevel)
			}
			c := &config.Config{}
			c.SetLogLevel(configured)
			got, err := c.ResolveLogLevel(args.LogLevel)
			if err != nil {
				t.Fatal(err)
			}
			want := level
			if want == "" {
				want = configured
			}
			if want == "" {
				want = "warn"
			}
			if got != want {
				t.Errorf("configured %q, CLI %q: resolved level = %q, want %q", configured, level, got, want)
			}
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
