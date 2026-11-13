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

package component

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/storage"
)

// fetchSpyStorage overrides only Get; every other Storage method delegates to
// the embedded implementation (an in-memory one) so the spy stays small.
type fetchSpyStorage struct {
	storage.Storage
	get func(ctx context.Context, bucket, fnm string, tenantID ...string) ([]byte, error)
}

func (s *fetchSpyStorage) Get(ctx context.Context, bucket, fnm string, tenantID ...string) ([]byte, error) {
	return s.get(ctx, bucket, fnm, tenantID...)
}

// withFetchStorage installs s as the storage backend for one test.
func withFetchStorage(t *testing.T, s storage.Storage) {
	t.Helper()
	prev := SetStorageFactoryOverride
	SetStorageFactoryOverride = func() storage.Storage { return s }
	t.Cleanup(func() { SetStorageFactoryOverride = prev })
}

func TestFetchBinaryReturnsObjectAndForwardsContext(t *testing.T) {
	payload := []byte("pdf-bytes-\x00\x01\xff")
	type seen struct {
		bucket, fnm string
		ctx         context.Context
	}
	var got seen
	withFetchStorage(t, &fetchSpyStorage{
		Storage: storage.NewMemoryStorage(),
		get: func(ctx context.Context, bucket, fnm string, _ ...string) ([]byte, error) {
			got = seen{bucket: bucket, fnm: fnm, ctx: ctx}
			return payload, nil
		},
	})

	ctx := context.Background()
	data, err := FetchBinary(ctx, "kb-1", "docs/a.pdf")
	if err != nil {
		t.Fatalf("FetchBinary: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("FetchBinary returned %q, want %q", data, payload)
	}
	if got.bucket != "kb-1" || got.fnm != "docs/a.pdf" {
		t.Fatalf("Get called with (%q, %q), want (kb-1, docs/a.pdf)", got.bucket, got.fnm)
	}
	// FetchBinary must hand the caller's ctx to the storage layer so a
	// cancellation reaches the in-flight download.
	if got.ctx != ctx {
		t.Error("FetchBinary did not forward the caller's ctx to storage.Get")
	}
}

func TestFetchBinaryWrapsStorageError(t *testing.T) {
	sentinel := errors.New("object missing")
	withFetchStorage(t, &fetchSpyStorage{
		Storage: storage.NewMemoryStorage(),
		get: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return nil, sentinel
		},
	})

	_, err := FetchBinary(context.Background(), "kb-1", "docs/missing.pdf")
	if err == nil {
		t.Fatal("FetchBinary succeeded despite a storage error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error %v does not wrap the storage error", err)
	}
	// The wrap must identify the object so operators can find it in logs.
	for _, want := range []string{`"kb-1"`, `"docs/missing.pdf"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

func TestFetchBinaryAlreadyCanceledContext(t *testing.T) {
	called := false
	withFetchStorage(t, &fetchSpyStorage{
		Storage: storage.NewMemoryStorage(),
		get: func(ctx context.Context, _, _ string, _ ...string) ([]byte, error) {
			called = true
			return nil, ctx.Err()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := FetchBinary(ctx, "kb-1", "docs/a.pdf")
	if err == nil {
		t.Fatal("FetchBinary succeeded with a canceled ctx")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v does not wrap context.Canceled", err)
	}
	if !called {
		t.Error("storage.Get was never reached; the cancellation must surface through it")
	}
}

// TestFetchBinaryCancelDuringDownload pins that a cancellation while the
// download is in flight surfaces promptly instead of waiting for the whole
// object. The spy models a storage layer that honors ctx, as the real backends
// do — which is why FetchBinary needs no goroutine of its own to abort early.
func TestFetchBinaryCancelDuringDownload(t *testing.T) {
	entered := make(chan struct{})
	withFetchStorage(t, &fetchSpyStorage{
		Storage: storage.NewMemoryStorage(),
		get: func(ctx context.Context, _, _ string, _ ...string) ([]byte, error) {
			close(entered)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-entered
		cancel()
	}()

	start := time.Now()
	_, err := FetchBinary(ctx, "kb-1", "docs/slow.pdf")
	elapsed := time.Since(start)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("FetchBinary returned %v, want a context.Canceled wrap", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("FetchBinary took %v to observe the cancellation; it must not wait for the download", elapsed)
	}
}

func TestFetchBinaryLargePayload(t *testing.T) {
	payload := make([]byte, 4<<20) // 4 MiB
	for i := range payload {
		payload[i] = byte(i)
	}
	withFetchStorage(t, &fetchSpyStorage{
		Storage: storage.NewMemoryStorage(),
		get: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			out := make([]byte, len(payload))
			copy(out, payload)
			return out, nil
		},
	})

	data, err := FetchBinary(context.Background(), "kb-1", "docs/big.pdf")
	if err != nil {
		t.Fatalf("FetchBinary: %v", err)
	}
	if len(data) != len(payload) {
		t.Fatalf("FetchBinary returned %d bytes, want %d", len(data), len(payload))
	}
	for i := range data {
		if data[i] != payload[i] {
			t.Fatalf("payload differs at byte %d: got %d, want %d", i, data[i], payload[i])
		}
	}
}

func TestFetchBinaryConcurrentCalls(t *testing.T) {
	payload := []byte("shared-object")
	withFetchStorage(t, &fetchSpyStorage{
		Storage: storage.NewMemoryStorage(),
		get: func(_ context.Context, _, _ string, _ ...string) ([]byte, error) {
			return payload, nil
		},
	})

	const callers = 16
	var wg sync.WaitGroup
	errs := make([]error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data, err := FetchBinary(context.Background(), "kb-1", "docs/shared.pdf")
			if err == nil && string(data) != string(payload) {
				err = fmt.Errorf("got %q, want %q", data, payload)
			}
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("caller %d: %v", i, err)
		}
	}
}

func TestFetchBinaryWithoutStorageBackend(t *testing.T) {
	if storage.GetStorageFactory().GetStorage() != nil {
		t.Skip("a global storage backend is registered in this test binary; the nil-backend path cannot be observed")
	}
	withFetchStorage(t, nil)

	_, err := FetchBinary(context.Background(), "kb-1", "docs/a.pdf")
	if err == nil {
		t.Fatal("FetchBinary succeeded without any storage backend")
	}
	if !strings.Contains(err.Error(), "no storage backend registered") {
		t.Fatalf("error %q does not mention the missing storage backend", err.Error())
	}
}
