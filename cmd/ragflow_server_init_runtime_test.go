// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"testing"
	"time"

	"ragflow/internal/server/bootstrap"
)

// initRuntimeBasics is the shared logger/name/pprof bootstrap extracted from
// main() during the dispatch refactor. It is now consumed by BOTH the
// standalone DeepDoc path and the full-infrastructure path, so its pprof gating
// must be pinned independently of main()'s deepdoc branch. These tests exercise
// the helper directly (not through main()) so a regression in the extracted
// function - rather than in the dispatch wiring - fails loudly. They reuse the
// freeLocalAddr/probeReachable helpers already present in this package.

// TestInitRuntimeBasicsStartsPProfWhenEnabled pins that initRuntimeBasics opens
// the pprof endpoint (at the configurable pprofAddr) when EnablePProf is true.
func TestInitRuntimeBasicsStartsPProfWhenEnabled(t *testing.T) {
	addr := freeLocalAddr(t)
	t.Setenv("RAGFLOW_PPROF_ADDR", addr)
	args := &bootstrap.ServerArgs{EnablePProf: true}
	if err := initRuntimeBasics(args, "initrt_test"); err != nil {
		t.Fatalf("initRuntimeBasics error: %v", err)
	}
	url := "http://" + addr + "/debug/pprof/"
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if probeReachable(url) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("pprof endpoint %s not reachable after initRuntimeBasics", url)
}

// TestInitRuntimeBasicsWithoutProfileDoesNotStartPProf is the adversarial guard
// for the flag gate: EnablePProf=false must not open the profiling port. An
// implementation that calls startPProf unconditionally passes the positive test
// above but fails here.
func TestInitRuntimeBasicsWithoutProfileDoesNotStartPProf(t *testing.T) {
	addr := freeLocalAddr(t)
	t.Setenv("RAGFLOW_PPROF_ADDR", addr)
	args := &bootstrap.ServerArgs{EnablePProf: false}
	if err := initRuntimeBasics(args, "initrt_test"); err != nil {
		t.Fatalf("initRuntimeBasics error: %v", err)
	}
	url := "http://" + addr + "/debug/pprof/"
	// Give any erroneously started server a moment, then assert it is down.
	time.Sleep(500 * time.Millisecond)
	if probeReachable(url) {
		t.Fatalf("pprof endpoint %s reachable without EnablePProf", url)
	}
}

// TestInitRuntimeBasicsCleanReturnsNil pins the happy-path contract: a clean
// call (EnablePProf=false, valid args) must return a nil error so main() can
// proceed. It guards against a future change that turns initRuntimeBasics into a
// silent no-op or swallows an underlying InitLocalVariables failure.
func TestInitRuntimeBasicsCleanReturnsNil(t *testing.T) {
	addr := freeLocalAddr(t)
	t.Setenv("RAGFLOW_PPROF_ADDR", addr)
	args := &bootstrap.ServerArgs{EnablePProf: false}
	if err := initRuntimeBasics(args, "initrt_test"); err != nil {
		t.Fatalf("initRuntimeBasics clean call error: %v", err)
	}
}
