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
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The DeepDoc early boot returns before the regular pprof block (which sits
// after the infrastructure init that the early return skips), so the early
// path must start the profiling endpoint itself. These tests pin that
// contract end-to-end through main(), re-executing the test binary as a
// subprocess (the pattern established by TestMainMigrateReachesConfiguration).

const (
	pprofProbeURL   = "http://localhost:6060/debug/pprof/"
	pprofSkipExit   = 42               // child exit code: port 6060 held by a stray process
	pprofBootWait   = 15 * time.Second // max time for main() to finish its early boot
	pprofProbeWait  = 15 * time.Second // max time for the pprof endpoint to come up
	pprofSettleWait = 5 * time.Second  // silence window for the negative case
)

// pprofProbeClient dials localhost directly: an ambient HTTP_PROXY must not
// intercept the probe and fake a negative.
var pprofProbeClient = &http.Client{
	Timeout:   2 * time.Second,
	Transport: &http.Transport{Proxy: nil},
}

// pprofReachable reports whether the pprof index endpoint answers with 200.
func pprofReachable() bool {
	resp, err := pprofProbeClient.Get(pprofProbeURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// pprofChildBoot starts main() in the background inside a re-executed test
// binary and returns a channel closed once main() returns. The OSS runDeepDoc
// stub returns immediately, but a pprof goroutine started by main() outlives
// it for the rest of this process, which is what the child assertions observe.
func pprofChildBoot() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()
	return done
}

// runPProfChild re-executes this test binary as a subprocess with envVar set
// so the named test takes its child branch, and maps the exit code:
// 0 → pass, pprofSkipExit → skip (port 6060 held by a stray process),
// anything else → fail with the combined child output. It returns the output
// so callers can add secondary assertions.
func runPProfChild(t *testing.T, testName, envVar string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+testName+"$", "-test.v")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), envVar+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == pprofSkipExit {
			t.Skip("localhost:6060 is held by another process; cannot test pprof deterministically")
		}
		t.Fatalf("%s child failed: %v\n%s", testName, err, out)
	}
	return string(out)
}

// TestMainDeepDocProfileStartsPProf pins the contract that a standalone
// DeepDoc server booted with --profile serves pprof on localhost:6060. The
// DeepDoc early boot returns before the regular pprof block, so without a
// start in the early path, --deepdoc --profile silently ignores the flag the
// --deepdoc help text advertises. The secondary assertion on the startup log
// line proves the endpoint was opened by this boot path, not by a stray
// listener that slipped past the child's pre-check.
func TestMainDeepDocProfileStartsPProf(t *testing.T) {
	const envVar = "RAGFLOW_TEST_DEEPDOC_PROFILE"
	if os.Getenv(envVar) == "1" {
		if pprofReachable() {
			os.Exit(pprofSkipExit)
		}
		os.Args = []string{os.Args[0], "--deepdoc", "--profile"}
		pprofChildBoot()
		deadline := time.Now().Add(pprofProbeWait)
		for time.Now().Before(deadline) {
			if pprofReachable() {
				os.Exit(0)
			}
			time.Sleep(50 * time.Millisecond)
		}
		fmt.Fprintln(os.Stderr, "pprof endpoint never became reachable")
		os.Exit(1)
	}
	out := runPProfChild(t, "TestMainDeepDocProfileStartsPProf", envVar)
	if !strings.Contains(out, "Starting pprof server") {
		t.Fatalf("pprof startup line missing from child output:\n%s", out)
	}
}

// TestMainDeepDocWithoutProfileDoesNotStartPProf is the adversarial guard for
// the flag gate: --deepdoc WITHOUT --profile must not open the profiling
// port. An implementation that starts pprof unconditionally (ignoring
// arguments.EnablePProf) passes the positive test above but fails here. The
// child waits for main() to finish its early boot first, so a pprof server
// started by that path would already be listening when the silence window
// opens.
func TestMainDeepDocWithoutProfileDoesNotStartPProf(t *testing.T) {
	const envVar = "RAGFLOW_TEST_DEEPDOC_PROFILE_NEGATIVE"
	if os.Getenv(envVar) == "1" {
		if pprofReachable() {
			os.Exit(pprofSkipExit)
		}
		os.Args = []string{os.Args[0], "--deepdoc"}
		select {
		case <-pprofChildBoot():
		case <-time.After(pprofBootWait):
			fmt.Fprintln(os.Stderr, "main() did not return in time")
			os.Exit(1)
		}
		deadline := time.Now().Add(pprofSettleWait)
		for time.Now().Before(deadline) {
			if pprofReachable() {
				fmt.Fprintln(os.Stderr, "pprof endpoint reachable without --profile")
				os.Exit(1)
			}
			time.Sleep(50 * time.Millisecond)
		}
		os.Exit(0)
	}
	runPProfChild(t, "TestMainDeepDocWithoutProfileDoesNotStartPProf", envVar)
}

// TestMainDeepDocProfilePProfBindFailureIsNonFatal pins the error path: when
// the profiling port is already taken, the DeepDoc boot must log the failure
// and keep running — the inference server matters more than profiling. The
// child occupies localhost:6060 itself before booting main(), so main()'s
// listener attempt is guaranteed to fail; the test passes only if main()
// still returns without exiting the process.
func TestMainDeepDocProfilePProfBindFailureIsNonFatal(t *testing.T) {
	const envVar = "RAGFLOW_TEST_DEEPDOC_PROFILE_BINDFAIL"
	if os.Getenv(envVar) == "1" {
		ln, err := net.Listen("tcp", "localhost:6060")
		if err != nil {
			os.Exit(pprofSkipExit)
		}
		defer ln.Close()
		os.Args = []string{os.Args[0], "--deepdoc", "--profile"}
		select {
		case <-pprofChildBoot():
			os.Exit(0)
		case <-time.After(pprofBootWait):
			fmt.Fprintln(os.Stderr, "main() did not return in time")
			os.Exit(1)
		}
	}
	runPProfChild(t, "TestMainDeepDocProfilePProfBindFailureIsNonFatal", envVar)
}
