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
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsCreds "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/middleware"
	"github.com/minio/minio-go/v7"
	minioCreds "github.com/minio/minio-go/v7/pkg/credentials"
)

// errSentinelPresign is a controlled error injected into the S3 presign path to
// verify that GetPresignedURL preserves (wraps) the underlying error instead of
// replacing it with a generic "after N retries" string.
var errSentinelPresign = errors.New("injected presign failure")

// failBuild is a build-stage middleware that forces every operation to fail with
// errSentinelPresign. Presigning runs the build/serialize middleware stack, so
// this deterministically fails the shared s3Base.GetPresignedURL path.
type failBuild struct{ err error }

func (f failBuild) ID() string { return "failBuild" }

func (f failBuild) HandleBuild(ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler) (middleware.BuildOutput, middleware.Metadata, error) {
	return middleware.BuildOutput{}, middleware.Metadata{}, f.err
}

// newFailingPresignS3Base builds an s3Base whose s3 client fails every presign
// attempt through a build-stage middleware, so the retry helper returns the
// sentinel error. This exercises the shared s3Base.GetPresignedURL path used by
// both S3 and OSS.
func newFailingPresignS3Base() *s3Base {
	client := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(awsCreds.NewStaticCredentialsProvider("access", "secret", "")),
		HTTPClient:  &http.Client{Transport: s3RoundTripper(func(req *http.Request) (*http.Response, error) { return s3Response(200, ""), nil })},
	}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("https://s3.test")
		o.UsePathStyle = true
		o.RetryMaxAttempts = 1
		o.APIOptions = append(o.APIOptions, func(stack *middleware.Stack) error {
			return stack.Build.Add(failBuild{err: errSentinelPresign}, middleware.Before)
		})
	})
	return &s3Base{
		client:     func() *s3.Client { return client },
		typeName:   "s3",
		resolve:    func(bucket, fnm string) (string, string) { return bucket, fnm },
		makeBucket: func(ctx context.Context, bucket string) error { return nil },
		reconnect:  func(context.Context) {},
	}
}

// TestS3BaseGetPresignedURLWrapsUnderlyingError pins that the shared
// GetPresignedURL surfaces the underlying presign error via errors.Is, so
// callers can still detect cancellations/deadlines and specific failures.
func TestS3BaseGetPresignedURLWrapsUnderlyingError(t *testing.T) {
	base := newFailingPresignS3Base()

	_, err := base.GetPresignedURL(context.Background(), "bucket", "key", time.Minute)
	if err == nil {
		t.Fatalf("GetPresignedURL err = nil, want the underlying presign error")
	}
	if !errors.Is(err, errSentinelPresign) {
		t.Fatalf("GetPresignedURL err = %v, want it to wrap %v (errors.Is must hold)", err, errSentinelPresign)
	}
}

// TestMinioGetPresignedURLWrapsUnderlyingError pins the same contract for the
// MinIO backend: a failing presign attempt must preserve the underlying error
// identity rather than replacing it with a generic string.
func TestMinioGetPresignedURLWrapsUnderlyingError(t *testing.T) {
	// A negative expiry makes minio-go's PresignedGetObject fail locally and
	// deterministically on every retry attempt, with no server required.
	storage := &MinioStorage{client: mustMinioClient(t)}
	_, err := storage.GetPresignedURL(context.Background(), "bucket", "key", -1*time.Second)
	if err == nil {
		t.Fatalf("GetPresignedURL err = nil, want a non-nil underlying error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "expires") {
		t.Fatalf("GetPresignedURL err = %v, want it to preserve the underlying expiry error text", err)
	}
}

// mustMinioClient builds a MinIO client pointed at a dummy endpoint. Presigning
// is computed locally and never contacts the server, so no service is needed.
func mustMinioClient(t *testing.T) *minio.Client {
	t.Helper()
	c, err := minio.New("localhost:9000", &minio.Options{
		Creds:  minioCreds.NewStaticV4("access", "secret", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("minio.New: %v", err)
	}
	return c
}
