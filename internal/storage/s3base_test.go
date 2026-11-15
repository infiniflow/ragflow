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

package storage

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Compile-time contract: every backend must satisfy the full Storage
// interface. These pin the embedding-based decomposition — if a future
// refactor drops a method from s3Base or a thin shell, compilation breaks.
var (
	_ Storage = (*S3Storage)(nil)
	_ Storage = (*OSSStorage)(nil)
	_ Storage = (*MinioStorage)(nil)
	_ Storage = (*MemoryStorage)(nil)
	_ Storage = (*GCSStorage)(nil)
)

// newBareS3Base builds an s3Base directly — no S3Storage wrapper — proving
// the base class is self-sufficient: a backend only needs to inject its
// client getter, path resolver, bucket factory, reconnect and type name.
func newBareS3Base(handler s3RoundTripper, typeName string) *s3Base {
	client := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("access", "secret", "")),
		HTTPClient:  &http.Client{Transport: handler},
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("https://s3.test")
		options.UsePathStyle = true
		options.RetryMaxAttempts = 1
	})
	return &s3Base{
		client:     func() *s3.Client { return client },
		typeName:   typeName,
		resolve:    func(bucket, fnm string) (string, string) { return bucket, fnm },
		makeBucket: func(ctx context.Context, bucket string) error { return nil },
		reconnect:  func(context.Context) {},
	}
}

func TestS3BaseStandalonePutAndGet(t *testing.T) {
	base := newBareS3Base(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPut {
			return s3Response(200, ""), nil
		}
		return s3Response(200, "hello-bucket"), nil
	}, "s3")

	if err := base.Put(context.Background(), "bucket", "key", []byte("data")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := base.Get(context.Background(), "bucket", "key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "hello-bucket" {
		t.Fatalf("Get returned %q, want hello-bucket", got)
	}
}

func TestS3BaseTypeReturnsInjectedName(t *testing.T) {
	base := newBareS3Base(func(req *http.Request) (*http.Response, error) {
		return s3Response(200, ""), nil
	}, "oss")
	if got := base.Type(); got != "oss" {
		t.Errorf("Type() = %q, want oss (injected name, not a hardcoded one)", got)
	}
}

func TestS3BasePutSurfacesUnderlyingErrorAfterRetries(t *testing.T) {
	// Put must surface the last underlying error (not a generic string) once
	// attempts are exhausted — the behavior unified in the helper extraction.
	base := newBareS3Base(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("simulated transport error")
	}, "s3")

	err := base.Put(context.Background(), "bucket", "key", []byte("data"))
	if err == nil || !strings.Contains(err.Error(), "simulated transport error") {
		t.Fatalf("Put err = %v, want the underlying transport error", err)
	}
}

func TestS3BaseClientGetterSeesReconnect(t *testing.T) {
	// The client getter must return the CURRENT client: after reconnect swaps
	// it, the next attempt must use the replacement, not a stale handle.
	oldClient := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("access", "secret", "")),
		HTTPClient: &http.Client{Transport: s3RoundTripper(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("old client: offline")
		})},
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("https://s3.test")
		options.UsePathStyle = true
		options.RetryMaxAttempts = 1
	})
	newClient := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("access", "secret", "")),
		HTTPClient: &http.Client{Transport: s3RoundTripper(func(req *http.Request) (*http.Response, error) {
			return s3Response(200, ""), nil
		})},
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("https://s3.test")
		options.UsePathStyle = true
		options.RetryMaxAttempts = 1
	})

	var current = oldClient
	base := &s3Base{
		client:     func() *s3.Client { return current },
		typeName:   "s3",
		resolve:    func(bucket, fnm string) (string, string) { return bucket, fnm },
		makeBucket: func(ctx context.Context, bucket string) error { return nil },
		reconnect:  func(context.Context) { current = newClient },
	}

	if err := base.Put(context.Background(), "bucket", "key", []byte("data")); err != nil {
		t.Fatalf("Put after reconnect: %v, want nil (second attempt must use the new client)", err)
	}
}
