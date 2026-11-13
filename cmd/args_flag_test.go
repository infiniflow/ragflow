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
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestParseArgsStandardSyntaxMatrix is the core contract of the stdlib flag
// migration: every value-taking flag must parse identically in all four
// standard forms — "--flag value", "--flag=value", "-flag value" and
// "-flag=value" (the flag package treats one and two dashes the same, and
// accepts the equals form for both).
func TestParseArgsStandardSyntaxMatrix(t *testing.T) {
	cases := []struct {
		flag  string
		value string
	}{
		{"--log-level", "debug"},
		{"--config", "cfg.yaml"},
		{"--name", "worker-1"},
		{"--port", "8080"},
		{"--admin-host", "1.2.3.4:5678"},
		{"--deepdoc-inference-concurrency", "4"},
		{"--ingestor-concurrent-workers", "8"},
		{"--ingestor-page-concurrency", "4"},
		{"--deepdoc-inference-cpu-cores", "2"},
	}
	for _, c := range cases {
		long := c.flag
		short := "-" + strings.TrimPrefix(c.flag, "-")
		forms := [][]string{
			{"ragflow_server", "--api", long, c.value},
			{"ragflow_server", "--api", long + "=" + c.value},
			{"ragflow_server", "--api", short, c.value},
			{"ragflow_server", "--api", short + "=" + c.value},
		}
		want, err := parseArgsFrom(forms[0])
		if err != nil {
			t.Fatalf("parseArgsFrom(%v) error = %v", forms[0], err)
		}
		for _, form := range forms[1:] {
			got, err := parseArgsFrom(form)
			if err != nil {
				t.Fatalf("parseArgsFrom(%v) error = %v", form, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("form %v parsed differently from %v:\n got  = %+v\n want = %+v", form, forms[0], got, want)
			}
		}
	}
}

// TestParseArgsShortAliases covers the short spellings -f/--config and
// -p/--port, which must also accept the equals form (the hand-rolled parser
// rejected "-p=80").
func TestParseArgsShortAliases(t *testing.T) {
	forms := [][]string{
		{"ragflow_server", "--api", "--config", "cfg.yaml"},
		{"ragflow_server", "--api", "--config=cfg.yaml"},
		{"ragflow_server", "--api", "-f", "cfg.yaml"},
		{"ragflow_server", "--api", "-f=cfg.yaml"},
		{"ragflow_server", "--api", "-config", "cfg.yaml"},
		{"ragflow_server", "--api", "-config=cfg.yaml"},
	}
	want, err := parseArgsFrom(forms[0])
	if err != nil {
		t.Fatalf("parseArgsFrom(%v) error = %v", forms[0], err)
	}
	for _, form := range forms[1:] {
		got, err := parseArgsFrom(form)
		if err != nil {
			t.Fatalf("parseArgsFrom(%v) error = %v", form, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("form %v parsed differently:\n got  = %+v\n want = %+v", form, got, want)
		}
	}

	portForms := [][]string{
		{"ragflow_server", "--api", "--port", "8080"},
		{"ragflow_server", "--api", "--port=8080"},
		{"ragflow_server", "--api", "-p", "8080"},
		{"ragflow_server", "--api", "-p=8080"},
		{"ragflow_server", "--api", "-port", "8080"},
		{"ragflow_server", "--api", "-port=8080"},
	}
	want, err = parseArgsFrom(portForms[0])
	if err != nil {
		t.Fatalf("parseArgsFrom(%v) error = %v", portForms[0], err)
	}
	for _, form := range portForms[1:] {
		got, err := parseArgsFrom(form)
		if err != nil {
			t.Fatalf("parseArgsFrom(%v) error = %v", form, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("form %v parsed differently:\n got  = %+v\n want = %+v", form, got, want)
		}
	}
}

// TestParseArgsBoolFlagForms covers the standard boolean spellings: bare,
// "=true" and "=false". A "=false" boolean is off, not an error.
func TestParseArgsBoolFlagForms(t *testing.T) {
	for _, form := range []string{"--profile", "--profile=true", "-profile", "-profile=true"} {
		args, err := parseArgsFrom([]string{"ragflow_server", "--api", form})
		if err != nil {
			t.Fatalf("parseArgsFrom(%q) error = %v", form, err)
		}
		if !args.EnablePProf {
			t.Errorf("%q enablePProf = false, want true", form)
		}
	}
	for _, form := range []string{"--profile=false", "-profile=false"} {
		args, err := parseArgsFrom([]string{"ragflow_server", "--api", form})
		if err != nil {
			t.Fatalf("parseArgsFrom(%q) error = %v", form, err)
		}
		if args.EnablePProf {
			t.Errorf("%q enablePProf = true, want false", form)
		}
	}
	for _, form := range []string{"--init-superuser", "--init-superuser=true", "-init-superuser"} {
		args, err := parseArgsFrom([]string{"ragflow_server", "--admin", form})
		if err != nil {
			t.Fatalf("parseArgsFrom(%q) error = %v", form, err)
		}
		if !args.InitSuperUser {
			t.Errorf("%q initSuperUser = false, want true", form)
		}
	}
	if args, err := parseArgsFrom([]string{"ragflow_server", "--admin", "--init-superuser=false"}); err != nil {
		t.Fatalf("--init-superuser=false error = %v", err)
	} else if args.InitSuperUser {
		t.Error("--init-superuser=false initSuperUser = true, want false")
	}
	// Non-boolean values are rejected, matching standard flag semantics.
	for _, form := range []string{"--profile=maybe", "--init-superuser=1.5"} {
		if _, err := parseArgsFrom([]string{"ragflow_server", "--api", form}); err == nil {
			t.Errorf("parseArgsFrom(%q) error = nil, want rejection", form)
		}
	}
}

// TestParseArgsModeFlags covers every mode flag in both dash forms and the
// last-one-wins rule across dash forms.
func TestParseArgsModeFlags(t *testing.T) {
	for _, mode := range []string{"admin", "api", "ingestor", "syncer", "deepdoc", "migrate"} {
		for _, form := range []string{"--" + mode, "-" + mode} {
			args, err := parseArgsFrom([]string{"ragflow_server", form})
			if err != nil {
				t.Fatalf("parseArgsFrom(%q) error = %v", form, err)
			}
			if args.Mode == nil || *args.Mode != mode {
				t.Errorf("parseArgsFrom(%q) mode = %v, want %s", form, args.Mode, mode)
			}
		}
	}

	lastWins := []struct {
		argv []string
		want string
	}{
		{[]string{"--api", "--admin"}, "admin"},
		{[]string{"--admin", "--api"}, "api"},
		{[]string{"-api", "-admin"}, "admin"},
		{[]string{"--api", "-migrate"}, "migrate"},
		{[]string{"--api=false", "--admin"}, "admin"},
		{[]string{"--api", "--migrate=false", "--syncer"}, "syncer"},
	}
	for _, c := range lastWins {
		args, err := parseArgsFrom(append([]string{"ragflow_server"}, c.argv...))
		if err != nil {
			t.Fatalf("parseArgsFrom(%v) error = %v", c.argv, err)
		}
		if args.Mode == nil || *args.Mode != c.want {
			t.Errorf("parseArgsFrom(%v) mode = %v, want %s", c.argv, args.Mode, c.want)
		}
	}

	// "=false" selects nothing; a lone false mode is not a mode at all.
	args, err := parseArgsFrom([]string{"ragflow_server", "--api=false"})
	if err != nil {
		t.Fatalf("--api=false error = %v", err)
	}
	if args.Mode != nil {
		t.Errorf("--api=false mode = %v, want nil", args.Mode)
	}
	// Non-boolean mode values are rejected.
	for _, form := range []string{"--api=maybe", "-migrate=1.5"} {
		if _, err := parseArgsFrom([]string{"ragflow_server", form}); err == nil {
			t.Errorf("parseArgsFrom(%q) error = nil, want rejection", form)
		}
	}
}

// TestParseArgsDoubleDashTerminator covers the standard "--" terminator: it
// ends flag parsing, and anything after it is positional (rejected here).
func TestParseArgsDoubleDashTerminator(t *testing.T) {
	args, err := parseArgsFrom([]string{"ragflow_server", "--api", "--"})
	if err != nil {
		t.Fatalf("--api -- error = %v", err)
	}
	if args.Mode == nil || *args.Mode != "api" {
		t.Errorf("--api -- mode = %v, want api", args.Mode)
	}

	args, err = parseArgsFrom([]string{"ragflow_server", "--"})
	if err != nil {
		t.Fatalf("bare -- error = %v", err)
	}
	if args.Mode != nil {
		t.Errorf("bare -- mode = %v, want nil", args.Mode)
	}

	for _, argv := range [][]string{
		{"--api", "--", "--port", "80"},
		{"--api", "--", "extra"},
		{"--", "--api"},
	} {
		if _, err := parseArgsFrom(append([]string{"ragflow_server"}, argv...)); err == nil {
			t.Errorf("parseArgsFrom(%v) error = nil, want rejection", argv)
		}
	}
}

// TestParseArgsPositionalRejected: the flag package silently stops at the
// first positional argument, which would hide a mistyped mode. This binary
// has no positional arguments, so they must be rejected.
func TestParseArgsPositionalRejected(t *testing.T) {
	for _, argv := range [][]string{
		{"apii"},
		{"--api", "leftover"},
		{"--api", "42"},
		{"--migrate", "--config", "cfg.yaml", "trailing"},
	} {
		if _, err := parseArgsFrom(append([]string{"ragflow_server"}, argv...)); err == nil {
			t.Errorf("parseArgsFrom(%v) error = nil, want rejection", argv)
		}
	}
}

// TestParseArgsUnknownFlagRejected covers unknown long/short flags in every
// spelling, including the equals form and the degenerate "-" and "--=x".
func TestParseArgsUnknownFlagRejected(t *testing.T) {
	for _, argv := range [][]string{
		{"--bogus"},
		{"--bogus=value"},
		{"-bogus"},
		{"-bogus=value"},
		{"-x"},
		{"-"},
		{"--=value"},
		{"--api", "--debug"},
		{"--api", "--enable-mcpserver"},
		{"--api", "--mcp-port=9382"},
	} {
		if _, err := parseArgsFrom(append([]string{"ragflow_server"}, argv...)); err == nil {
			t.Errorf("parseArgsFrom(%v) error = nil, want rejection", argv)
		}
	}
}

// TestParseArgsValueEdgeCases covers adversarial values around the "=" split
// and the space form.
func TestParseArgsValueEdgeCases(t *testing.T) {
	// A value containing '=' keeps everything after the first '='.
	args, err := parseArgsFrom([]string{"ragflow_server", "--name=foo=bar"})
	if err != nil {
		t.Fatalf("--name=foo=bar error = %v", err)
	}
	if args.Name == nil || *args.Name != "foo=bar" {
		t.Errorf("--name=foo=bar name = %v, want foo=bar", args.Name)
	}

	// In the space form the next argument is consumed literally even when it
	// looks like a flag, and must not be re-parsed as a mode.
	args, err = parseArgsFrom([]string{"ragflow_server", "--name", "--api"})
	if err != nil {
		t.Fatalf("--name --api error = %v", err)
	}
	if args.Name == nil || *args.Name != "--api" {
		t.Errorf("--name --api name = %v, want --api", args.Name)
	}
	if args.Mode != nil {
		t.Errorf("--name --api mode = %v, want nil", args.Mode)
	}

	// An explicitly empty string value is a valid (if odd) string value.
	args, err = parseArgsFrom([]string{"ragflow_server", "--api", "--name="})
	if err != nil {
		t.Fatalf("--name= error = %v", err)
	}
	if args.Name == nil || *args.Name != "" {
		t.Errorf("--name= name = %v, want empty string", args.Name)
	}
	args, err = parseArgsFrom([]string{"ragflow_server", "--api", "--config="})
	if err != nil {
		t.Fatalf("--config= error = %v", err)
	}
	if args.ConfigPath == nil || *args.ConfigPath != "" {
		t.Errorf("--config= configPath = %v, want empty string", args.ConfigPath)
	}

	// Empty values still fail validation for constrained flags.
	for _, argv := range [][]string{{"--api", "--log-level="}, {"--api", "--port="}, {"--api", "--deepdoc-inference-cpu-cores="}} {
		if _, err := parseArgsFrom(append([]string{"ragflow_server"}, argv...)); err == nil {
			t.Errorf("parseArgsFrom(%v) error = nil, want rejection", argv)
		}
	}
}

// TestParseArgsMissingValue: every value-taking flag at the end of argv must
// be reported as needing a value, in both dash forms.
func TestParseArgsMissingValue(t *testing.T) {
	valueFlags := []string{
		"--log-level", "--config", "--name", "--port", "--admin-host",
		"--deepdoc-inference-concurrency", "--ingestor-concurrent-workers",
		"--ingestor-page-concurrency", "--deepdoc-inference-cpu-cores",
	}
	for _, flag := range valueFlags {
		for _, spelling := range []string{flag, "-" + strings.TrimPrefix(flag, "-")} {
			argv := append([]string{"ragflow_server", "--api"}, spelling)
			if _, err := parseArgsFrom(argv); err == nil {
				t.Errorf("parseArgsFrom(%v) error = nil, want rejection", argv)
			}
		}
	}
	for _, short := range []string{"-f", "-p"} {
		argv := append([]string{"ragflow_server", "--api"}, short)
		if _, err := parseArgsFrom(argv); err == nil {
			t.Errorf("parseArgsFrom(%v) error = nil, want rejection", argv)
		}
	}
}

// TestParseArgsNumericValidation covers boundary and malformed inputs for the
// integer flags, preserving the pre-migration validation rules.
func TestParseArgsNumericValidation(t *testing.T) {
	positiveInvalid := []string{"0", "-1", "abc", "1.5"}
	for _, flag := range []string{"--deepdoc-inference-concurrency", "--ingestor-concurrent-workers", "--ingestor-page-concurrency"} {
		for _, v := range positiveInvalid {
			if _, err := parseArgsFrom([]string{"ragflow_server", "--api", flag + "=" + v}); err == nil {
				t.Errorf("%s=%s error = nil, want rejection", flag, v)
			}
		}
	}
	cpuInvalid := []string{"-1", "abc", "1.5"}
	for _, v := range cpuInvalid {
		if _, err := parseArgsFrom([]string{"ragflow_server", "--api", "--deepdoc-inference-cpu-cores=" + v}); err == nil {
			t.Errorf("--deepdoc-inference-cpu-cores=%s error = nil, want rejection", v)
		}
	}
	// cpu-cores=0 means "all cores" and must be accepted.
	args, err := parseArgsFrom([]string{"ragflow_server", "--api", "--deepdoc-inference-cpu-cores=0"})
	if err != nil {
		t.Fatalf("--deepdoc-inference-cpu-cores=0 error = %v", err)
	}
	if args.DeepdocInferenceCPUCores == nil || *args.DeepdocInferenceCPUCores != 0 {
		t.Errorf("--deepdoc-inference-cpu-cores=0 = %v, want 0 (all cores)", args.DeepdocInferenceCPUCores)
	}

	for _, port := range []string{"0", "70000", "-1", "abc"} {
		if _, err := parseArgsFrom([]string{"ragflow_server", "--api", "--port=" + port}); err == nil {
			t.Errorf("--port=%s error = nil, want rejection", port)
		}
	}
	for _, port := range []string{"1", "80", "65535"} {
		if _, err := parseArgsFrom([]string{"ragflow_server", "--api", "--port=" + port}); err != nil {
			t.Errorf("--port=%s error = %v, want nil", port, err)
		}
	}
}

// TestParseArgsAdminHost covers the ip:port split and its failure modes,
// preserving the pre-migration behavior (including IPv6 rejection, which the
// SplitN(":", 2) contract implies).
func TestParseArgsAdminHost(t *testing.T) {
	args, err := parseArgsFrom([]string{"ragflow_server", "--ingestor", "--admin-host=127.0.0.1:8000"})
	if err != nil {
		t.Fatalf("--admin-host=127.0.0.1:8000 error = %v", err)
	}
	if args.AdminHost == nil || *args.AdminHost != "127.0.0.1" {
		t.Errorf("adminHost = %v, want 127.0.0.1", args.AdminHost)
	}
	if args.AdminPort == nil || *args.AdminPort != 8000 {
		t.Errorf("adminPort = %v, want 8000", args.AdminPort)
	}

	for _, v := range []string{"127.0.0.1", ":8000", "127.0.0.1:", "127.0.0.1:abc", "::1:80", ""} {
		if _, err := parseArgsFrom([]string{"ragflow_server", "--ingestor", "--admin-host=" + v}); err == nil {
			t.Errorf("--admin-host=%q error = nil, want rejection", v)
		}
	}
}

// TestParseArgsPresenceSemantics guards the "unspecified vs explicit zero"
// contract: only flags present on the command line populate bootstrap.ServerArgs.
func TestParseArgsPresenceSemantics(t *testing.T) {
	args, err := parseArgsFrom([]string{"ragflow_server", "--api"})
	if err != nil {
		t.Fatalf("parseArgsFrom error = %v", err)
	}
	for name, got := range map[string]bool{
		"logLevel":                    args.LogLevel != nil,
		"configPath":                  args.ConfigPath != nil,
		"port":                        args.Port != nil,
		"adminHost":                   args.AdminHost != nil,
		"name":                        args.Name != nil,
		"deepdocInferenceConcurrency": args.DeepdocInferenceConcurrency != nil,
		"ingestorConcurrentWorkers":   args.IngestorConcurrentWorkers != nil,
		"ingestorPageConcurrency":     args.IngestorPageConcurrency != nil,
		"deepdocInferenceCPUCores":    args.DeepdocInferenceCPUCores != nil,
	} {
		if got {
			t.Errorf("%s set with no flag on the command line", name)
		}
	}

	args, err = parseArgsFrom([]string{"ragflow_server", "--api", "-p", "9380", "-f", "conf.yaml", "--deepdoc-inference-cpu-cores", "0"})
	if err != nil {
		t.Fatalf("parseArgsFrom error = %v", err)
	}
	if args.Port == nil || *args.Port != 9380 {
		t.Errorf("port = %v, want 9380", args.Port)
	}
	if args.ConfigPath == nil || *args.ConfigPath != "conf.yaml" {
		t.Errorf("configPath = %v, want conf.yaml", args.ConfigPath)
	}
	if args.DeepdocInferenceCPUCores == nil || *args.DeepdocInferenceCPUCores != 0 {
		t.Errorf("deepdocInferenceCPUCores = %v, want explicit 0", args.DeepdocInferenceCPUCores)
	}
}

// TestParseArgsHelpVersion covers both dash forms of the help and version
// flags, including that they combine with a mode (printHelp switches on it).
func TestParseArgsHelpVersion(t *testing.T) {
	for _, form := range []string{"-h", "--help", "-help"} {
		args, err := parseArgsFrom([]string{"ragflow_server", "--api", form})
		if err != nil {
			t.Fatalf("parseArgsFrom(%q) error = %v", form, err)
		}
		if !args.HelpFlag {
			t.Errorf("%q helpFlag = false, want true", form)
		}
		if args.Mode == nil || *args.Mode != "api" {
			t.Errorf("%q mode = %v, want api (help must not clear the mode)", form, args.Mode)
		}
	}
	for _, form := range []string{"-v", "--version", "-version"} {
		args, err := parseArgsFrom([]string{"ragflow_server", form})
		if err != nil {
			t.Fatalf("parseArgsFrom(%q) error = %v", form, err)
		}
		if !args.VersionFlag {
			t.Errorf("%q versionFlag = false, want true", form)
		}
	}
	args, err := parseArgsFrom([]string{"ragflow_server", "--api", "--help=false"})
	if err != nil {
		t.Fatalf("--help=false error = %v", err)
	}
	if args.HelpFlag {
		t.Error("--help=false helpFlag = true, want false")
	}
}

// TestParseArgsDoesNotMutateInput: parseArgsFrom must not rewrite the caller's
// argv slice (parseArgs passes os.Args straight through).
func TestParseArgsDoesNotMutateInput(t *testing.T) {
	in := []string{"ragflow_server", "--api", "--port=80", "--name=a=b"}
	want := append([]string(nil), in...)
	if _, err := parseArgsFrom(in); err != nil {
		t.Fatalf("parseArgsFrom error = %v", err)
	}
	if !reflect.DeepEqual(in, want) {
		t.Errorf("parseArgsFrom mutated its input: in = %v, want %v", in, want)
	}
}

// TestParseArgsEmptyArgv: an empty (or program-name-only) argv must parse as
// "no flags given" rather than panicking on the argv[1:] slice.
func TestParseArgsEmptyArgv(t *testing.T) {
	for _, argv := range [][]string{nil, {}, {"ragflow_server"}} {
		args, err := parseArgsFrom(argv)
		if err != nil {
			t.Fatalf("parseArgsFrom(%v) error = %v", argv, err)
		}
		if args.Mode != nil {
			t.Errorf("parseArgsFrom(%v) mode = %v, want nil", argv, args.Mode)
		}
	}
}

// TestParseArgsDashValues locks in standard flag semantics for values that
// look like flags or terminators: the space form consumes the next argument
// literally, so validation — not re-parsing — decides their fate.
func TestParseArgsDashValues(t *testing.T) {
	// Negative numbers are taken as the value, then rejected by validation.
	for _, argv := range [][]string{{"--api", "--port", "-1"}, {"--api", "--port=-1"}} {
		if _, err := parseArgsFrom(append([]string{"ragflow_server"}, argv...)); err == nil {
			t.Errorf("parseArgsFrom(%v) error = nil, want rejection", argv)
		}
	}
	// A dash-looking value for a string flag is consumed literally and then
	// fails the log-level whitelist.
	if _, err := parseArgsFrom([]string{"ragflow_server", "--api", "--log-level", "-x"}); err == nil {
		t.Error("--log-level -x error = nil, want rejection")
	}
	// "--" as a value is consumed literally, not treated as the terminator.
	args, err := parseArgsFrom([]string{"ragflow_server", "--name", "--"})
	if err != nil {
		t.Fatalf("--name -- error = %v", err)
	}
	if args.Name == nil || *args.Name != "--" {
		t.Errorf("--name -- name = %v, want --", args.Name)
	}
}

// TestParseArgsRepeatedFlagLastWins: repeating a flag keeps the last value,
// matching both the old parser and standard flag behavior.
func TestParseArgsRepeatedFlagLastWins(t *testing.T) {
	args, err := parseArgsFrom([]string{"ragflow_server", "--api", "--port", "80", "--port", "90"})
	if err != nil {
		t.Fatalf("repeated --port error = %v", err)
	}
	if args.Port == nil || *args.Port != 90 {
		t.Errorf("port = %v, want 90 (last wins)", args.Port)
	}
	args, err = parseArgsFrom([]string{"ragflow_server", "--api", "--log-level", "debug", "--log-level", "error"})
	if err != nil {
		t.Fatalf("repeated --log-level error = %v", err)
	}
	if args.LogLevel == nil || *args.LogLevel != "error" {
		t.Errorf("logLevel = %v, want error (last wins)", args.LogLevel)
	}
	args, err = parseArgsFrom([]string{"ragflow_server", "--api", "-f", "a.yaml", "--config", "b.yaml"})
	if err != nil {
		t.Fatalf("repeated -f/--config error = %v", err)
	}
	if args.ConfigPath == nil || *args.ConfigPath != "b.yaml" {
		t.Errorf("configPath = %v, want b.yaml (last wins)", args.ConfigPath)
	}
}

// TestParseArgsBoolDoesNotConsumePositional: boolean flags never consume the
// next argument, so a stray token after them is a positional (rejected).
func TestParseArgsBoolDoesNotConsumePositional(t *testing.T) {
	if _, err := parseArgsFrom([]string{"ragflow_server", "--api", "--profile", "--init-superuser", "stray"}); err == nil {
		t.Error("stray token after boolean flags error = nil, want rejection")
	}
	// Both booleans still parse when no stray token follows.
	args, err := parseArgsFrom([]string{"ragflow_server", "--api", "--profile", "--init-superuser"})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !args.EnablePProf || !args.InitSuperUser {
		t.Errorf("enablePProf = %v, initSuperUser = %v, want both true", args.EnablePProf, args.InitSuperUser)
	}
}

// TestParseArgsDoesNotMutateOsArgs is the os.Args-level version of the same
// guard, exercised through the parseArgs adapter.
func TestParseArgsDoesNotMutateOsArgs(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"ragflow_server", "--api", "--port=80"}
	snapshot := append([]string(nil), os.Args...)
	if _, err := parseArgs(); err != nil {
		t.Fatalf("parseArgs error = %v", err)
	}
	if !reflect.DeepEqual(os.Args, snapshot) {
		t.Errorf("parseArgs mutated os.Args: got %v, want %v", os.Args, snapshot)
	}
}

// TestParseArgsModeBoolSynonyms locks in the fact that mode flags route
// through strconv.ParseBool (via modeFlag.IsBoolFlag): the standard boolean
// truthy spellings 1/t/T select the mode, and the falsy spellings 0/f/F do
// not. This is stdlib flag behavior, not something the parser special-cases.
func TestParseArgsModeBoolSynonyms(t *testing.T) {
	truthy := []struct {
		argv []string
		want string
	}{
		{[]string{"--api=1"}, "api"},
		{[]string{"-api=1"}, "api"},
		{[]string{"--api=t"}, "api"},
		{[]string{"--api=T"}, "api"},
		{[]string{"--api=true"}, "api"},
		{[]string{"--migrate=1"}, "migrate"},
	}
	for _, c := range truthy {
		args, err := parseArgsFrom(append([]string{"ragflow_server"}, c.argv...))
		if err != nil {
			t.Fatalf("parseArgsFrom(%v) error = %v", c.argv, err)
		}
		if args.Mode == nil || *args.Mode != c.want {
			t.Errorf("parseArgsFrom(%v) mode = %v, want %s", c.argv, args.Mode, c.want)
		}
	}

	falsy := []string{"--api=0", "-api=0", "--api=f", "--api=F", "--api=false"}
	for _, argv := range falsy {
		args, err := parseArgsFrom([]string{"ragflow_server", argv})
		if err != nil {
			t.Fatalf("parseArgsFrom(%q) error = %v", argv, err)
		}
		if args.Mode != nil {
			t.Errorf("parseArgsFrom(%q) mode = %v, want nil (falsy boolean selects no mode)", argv, args.Mode)
		}
	}
}
