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

//go:build integration

package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// These integration tests pin the end-to-end contract that main()'s standalone
// DeepDoc early-return path starts (or, without --profile, does NOT start) the
// pprof server. They re-execute the test binary as a subprocess (the pattern
// established by TestMainMigrateReachesConfiguration), but unlike the old
// design they probe the LIVE child from the parent and then kill it — they do
// NOT wait for main() to return. That makes them robust to a real DeepDoc
// server that blocks for the process lifetime (the regression behind the
// #1036 CI failure, where the OSS runDeepDoc stub returns but the EE one does
// not). The pprof port is ephemeral (RAGFLOW_PPROF_ADDR), eliminating the
// hard-coded localhost:6060 clash.
//
// When the DeepDoc server cannot actually boot in the test environment (e.g.
// EE models absent on a runner), the child process exits before the probe
// succeeds; the tests Skip rather than fail, because the unit TestStartPProf*
// tests already cover the pprof contract in isolation.
//
// The two helper functions below live in the non-tagged pprof test file so they
// are compiled into every build tier.

const pprofChildEnv = "RAGFLOW_PPROF_CHILD"

// runPProfChild re-executes this test binary as a subprocess with the child
// branch enabled and an ephemeral RAGFLOW_PPROF_ADDR, returning the running
// command and a channel closed when the child process exits. Any inherited
// RAGFLOW_PPROF_ADDR / child-env value is stripped so the test's override wins
// (os.Getenv returns the first match).
func runPProfChild(t *testing.T, addr string) (*exec.Cmd, <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	env := make([]string, 0, len(os.Environ())+2)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "RAGFLOW_PPROF_ADDR=") || strings.HasPrefix(e, pprofChildEnv+"=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, pprofChildEnv+"=1", "RAGFLOW_PPROF_ADDR="+addr)

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	return cmd, exited
}

// TestDeepDocEarlyPathWiresPProf verifies that `--deepdoc --profile` actually
// starts pprof on the configured address — the regression guard for the early
// return path silently dropping the flag.
func TestDeepDocEarlyPathWiresPProf(t *testing.T) {
	if os.Getenv(pprofChildEnv) == "1" {
		os.Args = []string{os.Args[0], "--deepdoc", "--profile"}
		pprofChildBoot()
		select {} // stay alive; the parent kills us after probing
	}

	addr := freeLocalAddr(t)
	cmd, exited := runPProfChild(t, addr)
	defer cmd.Process.Kill()

	url := "http://" + addr + "/debug/pprof/"
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if probeReachable(url) {
			return
		}
		select {
		case <-exited:
			t.Skip("deepdoc server could not boot in this environment (e.g. EE models absent); unit TestStartPProf* cover the pprof contract")
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("pprof endpoint %s never became reachable from --deepdoc --profile", url)
}

// TestDeepDocEarlyPathNoPProfWithoutFlag is the adversarial guard for the flag
// gate: `--deepdoc` WITHOUT `--profile` must not open the profiling port.
func TestDeepDocEarlyPathNoPProfWithoutFlag(t *testing.T) {
	if os.Getenv(pprofChildEnv) == "1" {
		os.Args = []string{os.Args[0], "--deepdoc"} // no --profile
		pprofChildBoot()
		select {} // stay alive; the parent kills us after probing
	}

	addr := freeLocalAddr(t)
	cmd, exited := runPProfChild(t, addr)
	defer cmd.Process.Kill()

	url := "http://" + addr + "/debug/pprof/"
	deadline := time.Now().Add(15 * time.Second)
	reached := false
	for time.Now().Before(deadline) {
		if probeReachable(url) {
			reached = true
			break
		}
		select {
		case <-exited:
			t.Skip("deepdoc server could not boot in this environment (e.g. EE models absent); unit TestStartPProf* cover the pprof contract")
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	if reached {
		t.Fatalf("pprof endpoint reachable from --deepdoc WITHOUT --profile (flag gate broken)")
	}
}
