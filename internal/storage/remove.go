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
	"time"
)

// RemoveBlob deletes the object a write already detached from its row, retrying
// on a context that survives the originating request.
//
// It deliberately does NOT use the request context: a cancelled request must not
// leak the blob it already wrote, nor orphan a blob whose row was deleted. The
// retries are bounded by a timeout so a wedged storage SDK cannot block the
// caller forever, and a cancellation of the cleanup context itself is terminal
// rather than retried.
func RemoveBlob(ctx context.Context, impl Storage, bucket, object string) error {
	newCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := impl.Remove(newCtx, bucket, object); err != nil {
			lastErr = err
			// Treat cancellation of the *new* cleanup ctx as terminal.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				break
			}
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
			continue
		}
		return nil
	}
	return lastErr
}
