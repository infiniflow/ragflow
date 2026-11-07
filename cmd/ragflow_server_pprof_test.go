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
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

// freeLocalAddr returns a localhost TCP address on an OS-assigned ephemeral
// port. The kernel reserves it until the returned listener is closed, so we
// only use it to learn a free port and then close it so startPProf can bind it.
func freeLocalAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeLocalAddr: listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// probeReachable reports whether the pprof index endpoint at url answers 200.
// It dials directly (no proxy) so an ambient HTTP_PROXY cannot fake a negative.
func probeReachable(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// pprofChildBoot runs main() in the background and returns a channel closed
// when main() returns. Used by the integration tier to drive the real server
// boot path. The OSS runDeepDoc stub returns immediately, but the pprof
// goroutine outlives it; the integration tests observe that from the parent.
func pprofChildBoot() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()
	return done
}

// TestStartPProfOpensEndpointWhenEnabled pins the happy path: with a free
// address, startPProf must actually bind and serve /debug/pprof/.
func TestStartPProfOpensEndpointWhenEnabled(t *testing.T) {
	addr := freeLocalAddr(t)
	if err := startPProf(addr); err != nil {
		t.Fatalf("startPProf(%q) unexpected error: %v", addr, err)
	}
	url := "http://" + addr + "/debug/pprof/"
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if probeReachable(url) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("pprof endpoint %s not reachable after startPProf", url)
}

// TestStartPProfBindFailureIsNonFatal pins the error path (adversarial): when
// the port is taken, startPProf must return ErrPProfBindFailed and must NOT
// panic or exit the process. A wrong implementation that swallows the error or
// calls os.Exit would fail (process dies / wrong error type).
func TestStartPProfBindFailureIsNonFatal(t *testing.T) {
	addr := freeLocalAddr(t)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("pre-occupy listen: %v", err)
	}
	defer ln.Close()
	if err := startPProf(addr); !errors.Is(err, ErrPProfBindFailed) {
		t.Fatalf("startPProf on occupied %q = %v, want ErrPProfBindFailed", addr, err)
	}
}

// TestPProfAddrDefault pins that pprofAddr() defaults to localhost:6060 when
// RAGFLOW_PPROF_ADDR is unset.
func TestPProfAddrDefault(t *testing.T) {
	t.Setenv("RAGFLOW_PPROF_ADDR", "")
	if got := pprofAddr(); got != "localhost:6060" {
		t.Fatalf("pprofAddr() = %q, want localhost:6060", got)
	}
}

// TestPProfAddrFromEnv pins that pprofAddr() honors RAGFLOW_PPROF_ADDR
// (adversarial: a distinguishing override). The old hard-coded localhost:6060
// would make this fail, proving the configurability is real.
func TestPProfAddrFromEnv(t *testing.T) {
	const want = "127.0.0.1:0"
	t.Setenv("RAGFLOW_PPROF_ADDR", want)
	if got := pprofAddr(); got != want {
		t.Fatalf("pprofAddr() = %q, want %q", got, want)
	}
}
