// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func newMinioHealthTestStorage(t *testing.T, region string, handler http.HandlerFunc) *MinioStorage {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := minio.New(strings.TrimPrefix(server.URL, "http://"), &minio.Options{
		Region:     region,
		Creds:      credentials.NewStaticV4("test-access", "test-secret", ""),
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &MinioStorage{client: client}
}

func TestMinioStorageHealthProbesConfiguredRegion(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var requests atomic.Int32
			storage := newMinioHealthTestStorage(t, "eu-west-2", func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodHead || r.URL.Path == "/" {
					t.Errorf("expected a bucket HEAD probe, got %s %s", r.Method, r.URL)
				}
				if !strings.Contains(r.Header.Get("Authorization"), "/eu-west-2/s3/aws4_request") {
					t.Error("probe did not retain the configured signing region")
				}
				w.WriteHeader(status)
			})
			if got, want := storage.Health(t.Context()), status != http.StatusInternalServerError; got != want {
				t.Errorf("Health() = %t for HTTP %d, want %t", got, status, want)
			}
			if got := requests.Load(); got != 1 {
				t.Errorf("probe requests = %d, want 1 even with an explicit region", got)
			}
		})
	}
}

func TestMinioStorageHealthConcurrentChecks(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	releaseProbes := sync.OnceFunc(func() { close(release) })
	defer releaseProbes()
	storage := newMinioHealthTestStorage(t, "us-east-1", func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
			w.WriteHeader(http.StatusNotFound)
		case <-r.Context().Done():
		}
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	results := make(chan bool, 2)
	for range 2 {
		go func() { results <- storage.Health(ctx) }()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("concurrent probes did not both reach the endpoint")
		}
	}
	// Both probes are in flight before either response is sent.
	releaseProbes()
	for range 2 {
		if !<-results {
			t.Fatal("a concurrent probe reported an available endpoint as unhealthy")
		}
	}
}

func TestMinioStorageHealthRepeatedChecks(t *testing.T) {
	var requests atomic.Int32
	storage := newMinioHealthTestStorage(t, "us-east-1", func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})
	for i := range 20 {
		if !storage.Health(t.Context()) {
			t.Fatalf("Health() = false on consecutive probe %d", i+1)
		}
	}
	if got := requests.Load(); got != 20 {
		t.Errorf("probe requests = %d, want 20", got)
	}
}

func TestMinioStorageHealthCallerCancellation(t *testing.T) {
	t.Run("already canceled", func(t *testing.T) {
		var requests atomic.Int32
		storage := newMinioHealthTestStorage(t, "us-east-1", func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			w.WriteHeader(http.StatusOK)
		})
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if storage.Health(ctx) {
			t.Error("Health() = true for an already canceled caller")
		}
		if got := requests.Load(); got != 0 {
			t.Errorf("probe requests = %d for canceled caller, want 0", got)
		}
	})
	t.Run("in flight", func(t *testing.T) {
		entered := make(chan struct{}, 1)
		storage := newMinioHealthTestStorage(t, "us-east-1", func(_ http.ResponseWriter, r *http.Request) {
			entered <- struct{}{}
			<-r.Context().Done()
		})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		result := make(chan bool, 1)
		go func() { result <- storage.Health(ctx) }()
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("probe did not reach the endpoint")
		}
		cancel()
		select {
		case healthy := <-result:
			if healthy {
				t.Error("Health() = true after caller cancellation")
			}
		case <-time.After(time.Second):
			t.Fatal("probe did not stop after caller cancellation")
		}
	})
}

func TestMinioStorageHealthHasFiveSecondLimit(t *testing.T) {
	storage := newMinioHealthTestStorage(t, "us-east-1", func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	started := time.Now()
	if storage.Health(t.Context()) {
		t.Error("Health() = true when the endpoint never responds")
	}
	if elapsed := time.Since(started); elapsed < 4*time.Second || elapsed > 7*time.Second {
		t.Errorf("Health() duration = %v, want the five-second probe limit", elapsed)
	}
}

func TestMinioStorageHealthNetworkFailureRecovers(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	storage := newMinioHealthTestStorage(t, "us-east-1", func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	if storage.Health(t.Context()) {
		t.Error("Health() = true for a broken connection")
	}
	fail.Store(false)
	if !storage.Health(t.Context()) {
		t.Error("Health() = false after the connection recovers")
	}
}

func TestMinioStorageHealthProbesAfterRegionDiscovery(t *testing.T) {
	var locations, heads atomic.Int32
	storage := newMinioHealthTestStorage(t, "", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Query().Has("location") {
			locations.Add(1)
			fmt.Fprint(w, `<LocationConstraint>us-east-1</LocationConstraint>`)
			return
		}
		if r.Method != http.MethodHead {
			t.Errorf("unexpected health request: %s %s", r.Method, r.URL)
		}
		heads.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})
	for range 2 {
		if !storage.Health(t.Context()) {
			t.Fatal("Health() = false after region discovery")
		}
	}
	if locations.Load() != 1 || heads.Load() != 2 {
		t.Errorf("location requests = %d, HEAD requests = %d, want 1 and 2", locations.Load(), heads.Load())
	}
}
