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

package tool

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/utility"
)

// recordingDialer is an exesqlDialer that records the host it was
// asked to dial so SSRF-pinning tests can assert that the resolved
// public IP was substituted before connect.
type recordingDialer struct {
	mu       sync.Mutex
	dialed   []dialRecord
	failFast bool
}

type dialRecord struct {
	driver string
	dsn    string
}

func (r *recordingDialer) dial(driver, dsn string) (*sql.DB, error) {
	r.mu.Lock()
	r.dialed = append(r.dialed, dialRecord{driver: driver, dsn: dsn})
	r.mu.Unlock()
	if r.failFast {
		return nil, errors.New("dialer: test stop before real DB I/O")
	}
	// Open a connection to a non-existent address — the test only
	// cares about whether the SSRF guard accepted the host, not
	// whether the connect succeeded. Returning a real *sql.DB
	// prevents the InvokableRun defer from panicking on Close.
	db, _ := sql.Open(driver, dsn)
	return db, nil
}

// TestExeSQL_SSRF_RejectsLoopbackLinkLocalAndRFC1918 mirrors the
// Python test_exesql_ssrf.py rejection cases. Each non-public host
// must be rejected before any driver dispatch — InvokableRun must
// not call the dialer.
func TestExeSQL_SSRF_RejectsLoopbackLinkLocalAndRFC1918(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		host string
	}{
		{"loopback_ipv4", "127.0.0.1"},
		{"loopback_alias", "localhost"},
		{"cloud_metadata", "169.254.169.254"},
		{"rfc1918_10", "10.0.0.1"},
		{"rfc1918_192", "192.168.1.1"},
		{"rfc1918_172", "172.16.0.1"},
		{"unspecified", "0.0.0.0"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			rec := &recordingDialer{}
			e := NewExeSQLTool(exesqlConnParams{
				DBType: "mysql", Host: c.host, Port: 3306,
				Database: "d", Username: "u", Password: "p",
				MaxRecords: 10,
			}).WithExeSQLDialer(rec.dial)

			_, err := e.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`)
			if err == nil {
				t.Fatalf("expected SSRF rejection for %q, got nil", c.host)
			}
			if !errors.Is(err, ErrSSRFBlocked) {
				t.Fatalf("err = %v, want ErrSSRFBlocked for host %q", err, c.host)
			}
			rec.mu.Lock()
			defer rec.mu.Unlock()
			if len(rec.dialed) != 0 {
				t.Fatalf("dialer should not be called when host is rejected; got %d calls", len(rec.dialed))
			}
		})
	}
}

// TestExeSQL_SSRF_RejectsEmptyHost covers the empty-string case which
// the loopback / link-local / RFC1918 cases do not exercise.
func TestExeSQL_SSRF_RejectsEmptyHost(t *testing.T) {
	t.Parallel()

	rec := &recordingDialer{}
	e := NewExeSQLTool(exesqlConnParams{
		DBType: "mysql", Host: "   ", Port: 3306,
		Database: "d", Username: "u", Password: "p",
		MaxRecords: 10,
	}).WithExeSQLDialer(rec.dial)

	_, err := e.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`)
	if err == nil {
		t.Fatal("expected SSRF rejection for empty host")
	}
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("err = %v, want ErrSSRFBlocked", err)
	}
	if len(rec.dialed) != 0 {
		t.Fatalf("dialer should not be called for empty host; got %d calls", len(rec.dialed))
	}
}

// TestExeSQL_SSRF_PinsToValidatedIP ensures that when a DNS name
// resolves to a public IP, InvokableRun dials the validated IP, not
// the original hostname — closing the TOCTOU window for DNS
// rebinding. The resolver is stubbed via utility.LookupHost so the
// test does not depend on real DNS.
func TestExeSQL_SSRF_PinsToValidatedIP(t *testing.T) {
	// Stub the resolver so example.test -> 1.2.3.4 (public, stable).
	origLookup := utility.LookupHost
	utility.LookupHost = func(host string) ([]string, error) {
		if host == "example.test" {
			return []string{"1.2.3.4"}, nil
		}
		return origLookup(host)
	}
	t.Cleanup(func() { utility.LookupHost = origLookup })

	// failFast:true makes the test dialer return an error instead of
	// opening a real *sql.DB, so the InvokableRun path stops after
	// recording the DSN — no real TCP connect to 1.2.3.4:3306 is
	// attempted (which would either succeed or hang on a TCP
	// timeout, making the test slow and flaky on networks that
	// reach Cloudflare). PR review round 6, Major #1.
	rec := &recordingDialer{failFast: true}
	e := NewExeSQLTool(exesqlConnParams{
		DBType: "mysql", Host: "example.test", Port: 3306,
		Database: "d", Username: "u", Password: "p",
		MaxRecords: 10,
	}).WithExeSQLDialer(rec.dial)

	_, _ = e.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.dialed) == 0 {
		t.Fatal("dialer was not called; SSRF guard may have blocked a public host")
	}
	got := rec.dialed[0].dsn
	if !strings.Contains(got, "1.2.3.4:3306") {
		t.Fatalf("DSN should dial pinned IP 1.2.3.4, got %q", got)
	}
	if strings.Contains(got, "example.test") {
		t.Fatalf("DSN should not contain original hostname, got %q", got)
	}
}

// TestValidateDBHost_LiteralPublicIPAccepted: literal public IP is
// accepted without DNS lookup. Round-trips through net.ParseIP.
func TestValidateDBHost_LiteralPublicIPAccepted(t *testing.T) {
	t.Parallel()
	got, err := ValidateDBHost("8.8.8.8")
	if err != nil {
		t.Fatalf("ValidateDBHost(8.8.8.8): %v", err)
	}
	if got != "8.8.8.8" {
		t.Fatalf("got %q, want 8.8.8.8", got)
	}
	parsed := net.ParseIP(got)
	if parsed == nil {
		t.Fatalf("returned host %q is not a valid IP", got)
	}
}
