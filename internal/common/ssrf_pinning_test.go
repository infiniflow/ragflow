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

package common

import (
	"context"
	"testing"
)

func TestPinTableLookupMissReturnsFalse(t *testing.T) {
	pt := NewPinTable()
	if ip, ok := pt.Lookup("never.listened"); ok {
		t.Fatalf("expected miss, got %q", ip)
	}
}

func TestPinTableLookupEmptyHostname(t *testing.T) {
	pt := NewPinTable()
	pt.Pin("real.example", "93.184.216.34")
	if ip, ok := pt.Lookup(""); ok {
		t.Fatalf("empty hostname must not match, got %q", ip)
	}
}

func TestPinTablePinEmptyIgnored(t *testing.T) {
	pt := NewPinTable()
	pt.Pin("", "93.184.216.34")
	pt.Pin("real.example", "")
	if ip, ok := pt.Lookup("real.example"); ok {
		t.Fatalf("empty pin must not store, got %q", ip)
	}
	if ip, ok := pt.Lookup(""); ok {
		t.Fatalf("empty hostname must not match, got %q", ip)
	}
}

func TestPinTableLookupIsCaseInsensitive(t *testing.T) {
	pt := NewPinTable()
	pt.Pin("Example.COM", "93.184.216.34")
	cases := []string{"example.com", "EXAMPLE.com", "Example.COM"}
	for _, h := range cases {
		ip, ok := pt.Lookup(h)
		if !ok {
			t.Fatalf("expected hit for %q", h)
		}
		if ip != "93.184.216.34" {
			t.Fatalf("got %q, want 93.184.216.34", ip)
		}
	}
}

func TestPinTableWrapDialContextPinsValidatedIP(t *testing.T) {
	pt := NewPinTable()
	pt.Pin("rebinding.example", "93.184.216.34")

	rec := &recordingAddrDialer{}
	wrapped := pt.WrapDialContext(rec.DialContext)
	if _, err := wrapped(context.Background(), "tcp", "rebinding.example:80"); err == nil {
		t.Fatal("expected recording dialer to return its sentinel error")
	}
	if len(rec.addrs) != 1 || rec.addrs[0] != "93.184.216.34:80" {
		t.Fatalf("dialer saw %v; want [93.184.216.34:80] (the pinned IP, not the rebinding hostname)", rec.addrs)
	}
}

func TestPinTableWrapDialContextFallsThroughWhenUnpinned(t *testing.T) {
	pt := NewPinTable()
	pt.Pin("pinned.example", "93.184.216.34")

	rec := &recordingAddrDialer{}
	wrapped := pt.WrapDialContext(rec.DialContext)
	if _, err := wrapped(context.Background(), "tcp", "other.example:80"); err == nil {
		t.Fatal("expected recording dialer to return its sentinel error")
	}
	if len(rec.addrs) != 1 || rec.addrs[0] != "other.example:80" {
		t.Fatalf("dialer saw %v; want [other.example:80] (unchanged, no pin)", rec.addrs)
	}
}

func TestPinTableWrapDialContextSkipsLiteralIP(t *testing.T) {
	pt := NewPinTable()
	pt.Pin("127.0.0.1", "93.184.216.34") // would never happen in practice, but the wrapper must not consult the table for IP literals

	rec := &recordingAddrDialer{}
	wrapped := pt.WrapDialContext(rec.DialContext)
	if _, err := wrapped(context.Background(), "tcp", "127.0.0.1:80"); err == nil {
		t.Fatal("expected recording dialer to return its sentinel error")
	}
	if len(rec.addrs) != 1 || rec.addrs[0] != "127.0.0.1:80" {
		t.Fatalf("dialer saw %v; want [127.0.0.1:80] (IP literals must not be pinned)", rec.addrs)
	}
}

func TestPinTableWrapDialContextRejectsInvalidAddr(t *testing.T) {
	pt := NewPinTable()
	rec := &recordingAddrDialer{}
	wrapped := pt.WrapDialContext(rec.DialContext)
	if _, err := wrapped(context.Background(), "tcp", "no-port"); err == nil {
		t.Fatal("expected split-host-port to fail")
	}
	if len(rec.addrs) != 0 {
		t.Fatalf("dialer must not run for malformed addr, got %v", rec.addrs)
	}
}

func TestPinTableConcurrentPinLookup(t *testing.T) {
	pt := NewPinTable()
	const writers, reader = 50, 50
	done := make(chan struct{}, writers+reader)

	for i := 0; i < writers; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			host := "h.example"
			ip := "10.0.0.1"
			if i%2 == 0 {
				ip = "10.0.0.2"
			}
			pt.Pin(host, ip)
		}(i)
	}
	for i := 0; i < reader; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			if ip, ok := pt.Lookup("h.example"); !ok || ip == "" {
				t.Errorf("missed pinned value: ip=%q ok=%v", ip, ok)
			}
		}()
	}
	for i := 0; i < writers+reader; i++ {
		<-done
	}
}
