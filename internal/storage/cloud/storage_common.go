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

package cloud

import (
	"context"
	"errors"
	"net/http"
	objectstorage "ragflow/internal/storage"
	"time"

	gcs "cloud.google.com/go/storage"
	"github.com/minio/minio-go/v7"

	"google.golang.org/api/googleapi"
)

// IsNotFound reports whether an object or bucket is missing from storage.
func IsNotFound(err error) bool {
	if errors.Is(err, objectstorage.ErrMemoryNotFound) || errors.Is(err, gcs.ErrObjectNotExist) || IsBucketNotFound(err) || isS3NotFound(err) {
		return true
	}
	var minioErr minio.ErrorResponse
	if errors.As(err, &minioErr) {
		return minioErr.Code == "NoSuchKey" || minioErr.Code == "NoSuchBucket" || minioErr.Code == "NotFound"
	}
	return false
}

// IsBucketNotFound reports a missing bucket without matching object-level
// 404s, so a bucket removal cannot mistake a missing object for a missing
// bucket. GCS's object iterator returns a raw googleapi 404 for this case,
// while object handles wrap theirs in a typed sentinel.
func IsBucketNotFound(err error) bool {
	if errors.Is(err, objectstorage.ErrMemoryNotFound) || errors.Is(err, gcs.ErrBucketNotExist) || isS3BucketNotFound(err) {
		return true
	}
	var minioErr minio.ErrorResponse
	if errors.As(err, &minioErr) {
		return minioErr.Code == "NoSuchBucket" || minioErr.Code == "NotFound"
	}
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound
}

// retryObjectOp runs op up to attempts times, sleeping one second between
// attempts and invoking onRetry (e.g. a client reconnect) before each sleep.
// It returns nil on the first success and the last error once attempts are
// exhausted. A canceled context short-circuits before an attempt and wins
// over the backoff sleep, so cancellation always surfaces promptly instead
// of costing a sleep window — the contract pinned by s3_cancel_test.go.
func retryObjectOp(ctx context.Context, attempts int, onRetry func(), op func() error) error {
	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := op(); err != nil {
			lastErr = err
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}
			if onRetry != nil {
				onRetry()
			}
			if serr := sleepOrAbort(ctx, time.Second); serr != nil {
				return serr
			}
			continue
		}
		return nil
	}
	return lastErr
}

func sleepOrAbort(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
