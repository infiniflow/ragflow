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

package channels

import (
	"context"
	"time"
)

// retryChannelCall runs call up to attempts times, bounding every attempt with
// perCallLimit so a stalled request cannot consume the whole reply, and backing off
// between attempts. A cancelled outer context stops the retries immediately: at that
// point the turn is over and repeating the call cannot help.
func retryChannelCall(ctx context.Context, attempts int, perCallLimit, baseBackoff time.Duration, call func(context.Context) error) error {
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, perCallLimit)
		err = call(attemptCtx)
		cancel()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt == attempts {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * baseBackoff):
		}
	}
	return err
}
