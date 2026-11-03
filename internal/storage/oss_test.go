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
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// newOSSTestStorage builds an OSS backend over a fake transport. These tests
// were written BEFORE oss.go was slimmed onto s3Base and passed unchanged
// against the old standalone implementation — they are the behavior contract
// the refactor must preserve (characterization tests).
func newOSSTestStorage(handler s3RoundTripper, bucket, prefix string) *OSSStorage {
	client := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("access", "secret", "")),
		HTTPClient:  &http.Client{Transport: handler},
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("https://oss.test")
		options.UsePathStyle = true
		options.RetryMaxAttempts = 1
	})
	o := &OSSStorage{client: client}
	o.s3Base = s3Base{
		client:     func() *s3.Client { return o.client },
		bucket:     bucket,
		prefixPath: prefix,
		typeName:   "oss",
		resolve:    o.resolveBucketAndPath,
		makeBucket: o.createBucket,
		reconnect:  func(context.Context) {},
	}
	return o
}

func TestOSSStoragePutAndGet(t *testing.T) {
	o := newOSSTestStorage(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPut {
			return s3Response(200, ""), nil
		}
		return s3Response(200, "oss-bytes"), nil
	}, "", "")

	if err := o.Put(context.Background(), "bucket", "key", []byte("data")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := o.Get(context.Background(), "bucket", "key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "oss-bytes" {
		t.Fatalf("Get returned %q, want oss-bytes", got)
	}
}

func TestOSSStoragePutSurfacesServiceError(t *testing.T) {
	// A failing bucket creation must surface an error after the retry loop
	// gives up — Put must never silently succeed. The assertion checks
	// non-nil only: with this aws-sdk version, any failing response's error
	// chain re-resolves the auth scheme and masks the real code behind an
	// endpoint-rule error (verified experimentally). Existing s3_test.go
	// failure cases follow the same err != nil convention for this reason.
	o := newOSSTestStorage(func(req *http.Request) (*http.Response, error) {
		return s3Response(403, `<Error><Code>AccessDenied</Code></Error>`), nil
	}, "", "")

	if err := o.Put(context.Background(), "bucket", "key", []byte("data")); err == nil {
		t.Fatal("Put returned nil after the bucket creation was denied")
	}
}

func TestOSSStorageGetSurfacesContextCancellation(t *testing.T) {
	o := newOSSTestStorage(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("simulated transport error")
	}, "", "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, err := o.Get(ctx, "bucket", "key")
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Get err = %v, want context.Canceled", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Get took %v on an already-canceled ctx; want < 500ms", elapsed)
	}
}

func TestOSSStorageHealthUploadsProbeObject(t *testing.T) {
	var sawCreateBucket, sawProbePut bool
	o := newOSSTestStorage(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.Method == http.MethodHead:
			return s3Response(404, ""), nil // bucket missing
		case req.Method == http.MethodPut && strings.HasSuffix(req.URL.Path, "/txtxtxtxt1"):
			sawProbePut = true
			return s3Response(200, ""), nil
		case req.Method == http.MethodPut:
			sawCreateBucket = true
			return s3Response(200, ""), nil
		default:
			return s3Response(200, ""), nil
		}
	}, "", "")

	if !o.Health(context.Background()) {
		t.Fatal("Health = false, want true")
	}
	if !sawCreateBucket {
		t.Error("Health did not create the missing probe bucket")
	}
	if !sawProbePut {
		t.Error("Health did not upload the probe object")
	}
}

func TestOSSStorageRemoveMissingObjectIsNoOp(t *testing.T) {
	o := newOSSTestStorage(func(req *http.Request) (*http.Response, error) {
		return s3Response(404, `<Error><Code>NoSuchKey</Code></Error>`), nil
	}, "", "")

	if err := o.Remove(context.Background(), "bucket", "missing"); err != nil {
		t.Fatalf("Remove of a missing object: %v, want nil", err)
	}
}

func TestOSSStorageObjExistDistinguishesPresent(t *testing.T) {
	o := newOSSTestStorage(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/there") {
			return s3Response(200, ""), nil
		}
		return s3Response(404, `<Error><Code>NoSuchKey</Code></Error>`), nil
	}, "", "")

	if !o.ObjExist(context.Background(), "bucket", "there") {
		t.Error("ObjExist(there) = false, want true")
	}
	if o.ObjExist(context.Background(), "bucket", "absent") {
		t.Error("ObjExist(absent) = true, want false")
	}
}

func TestOSSStorageBucketExistsWithErrorSeparatesFailure(t *testing.T) {
	o := newOSSTestStorage(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "real") {
			return s3Response(200, ""), nil
		}
		return s3Response(404, `<Error><Code>NoSuchBucket</Code></Error>`), nil
	}, "", "")

	exists, err := o.BucketExistsWithError(context.Background(), "real")
	if err != nil || !exists {
		t.Errorf("BucketExistsWithError(real) = %v, %v; want true, nil", exists, err)
	}
	exists, err = o.BucketExistsWithError(context.Background(), "ghost")
	if err != nil || exists {
		t.Errorf("BucketExistsWithError(ghost) = %v, %v; want false, nil", exists, err)
	}
}

func TestOSSStorageGetPresignedURL(t *testing.T) {
	o := newOSSTestStorage(func(req *http.Request) (*http.Response, error) {
		return s3Response(200, ""), nil
	}, "", "")

	url, err := o.GetPresignedURL(context.Background(), "bucket", "key", 0)
	if err != nil {
		t.Fatalf("GetPresignedURL: %v", err)
	}
	if !strings.Contains(url, "oss.test") || !strings.Contains(url, "bucket/key") {
		t.Errorf("presigned URL = %q, want the oss.test endpoint and bucket/key path", url)
	}
}

func TestOSSStorageRemoveBucketRejectsLogicalBucketInSharedMode(t *testing.T) {
	o := newOSSTestStorage(func(req *http.Request) (*http.Response, error) {
		return s3Response(200, ""), nil
	}, "physical", "data")

	err := o.RemoveBucket(context.Background(), "logical")
	if err == nil || !strings.Contains(err.Error(), "cannot remove logical OSS bucket") {
		t.Fatalf("RemoveBucket err = %v, want the shared-mode rejection", err)
	}
}

func TestOSSStorageRemoveEmptyBucketRejectsLogicalBucketInSharedMode(t *testing.T) {
	o := newOSSTestStorage(func(req *http.Request) (*http.Response, error) {
		return s3Response(200, ""), nil
	}, "physical", "data")

	err := o.RemoveEmptyBucket(context.Background(), "logical")
	if err == nil || !strings.Contains(err.Error(), "cannot remove logical OSS bucket") {
		t.Fatalf("RemoveEmptyBucket err = %v, want the shared-mode rejection", err)
	}
}

func TestOSSStorageResolveOmitsBucketSegment(t *testing.T) {
	// OSS folds <prefix>/<fnm> — unlike S3, the logical bucket is NOT part of
	// the key in shared-bucket mode. See doc.go for the per-vendor table.
	o := newOSSTestStorage(func(req *http.Request) (*http.Response, error) {
		return s3Response(200, ""), nil
	}, "physical", "data")

	bucket, fnm := o.resolveBucketAndPath("logical", "report.txt")
	if bucket != "physical" || fnm != "data/report.txt" {
		t.Fatalf("resolve = (%q, %q), want (physical, data/report.txt)", bucket, fnm)
	}
}
