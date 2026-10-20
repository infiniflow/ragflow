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

package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestS3GetSurfacesContextCancellation pins that Get does not swallow a
// canceled context behind its reconnect/backoff loop. On a canceled context
// it must return the context error promptly instead of sleeping for the
// backoff window and returning a generic "after retries" error.
//
// This is the root fix for the FetchBinary cancellation regression flagged in
// the review of PR #21084: the storage layer must honor ctx so cancellation
// surfaces through Get itself, for every backend including AWS S3.
func TestS3GetSurfacesContextCancellation(t *testing.T) {
	// Any transport error triggers the reconnect/backoff branch of Get.
	s := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("simulated transport error")
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before the call

	start := time.Now()
	_, err := s.Get(ctx, "bucket", "key")
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Get err = %v, want context.Canceled", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Get took %v on an already-canceled ctx; want < 500ms (no reconnect sleep)", elapsed)
	}
}

// TestS3GetReturnsObjectData guards the happy path against the cancellation
// change: a successful object read still returns the bytes unchanged.
func TestS3GetReturnsObjectData(t *testing.T) {
	s := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
		return s3Response(200, "hello-bucket"), nil
	})

	got, err := s.Get(context.Background(), "bucket", "key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "hello-bucket" {
		t.Fatalf("Get returned %q, want hello-bucket", got)
	}
}

// TestS3GetSurfacesContextCancellationMidFlight covers the in-flight case: the
// context is still live when GetObject is called, but is canceled while the
// request is blocked. The post-error ctx check must surface context.Canceled
// immediately instead of running the reconnect/backoff sleep.
func TestS3GetSurfacesContextCancellationMidFlight(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := newS3TestStorage("us-east-1", func(req *http.Request) (*http.Response, error) {
		<-ctx.Done() // block until the context is canceled
		return nil, context.Canceled
	})
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := s.Get(ctx, "bucket", "key")
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Get err = %v, want context.Canceled", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Get took %v on mid-flight cancel; want < 500ms (no reconnect sleep)", elapsed)
	}
}
