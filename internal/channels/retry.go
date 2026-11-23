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

const (
	initialChannelRetryDelay = 5 * time.Second
	maxChannelRetryDelay     = 24 * time.Hour
)

func channelRetryDelay(attempts int) time.Duration {
	delays := [...]time.Duration{
		initialChannelRetryDelay, 10 * time.Second, 20 * time.Second,
		time.Minute, 3 * time.Minute, 10 * time.Minute, 30 * time.Minute,
		time.Hour, 6 * time.Hour, maxChannelRetryDelay,
	}
	if attempts < 1 {
		return delays[0]
	}
	if attempts >= len(delays) {
		return delays[len(delays)-1]
	}
	return delays[attempts-1]
}

func waitForContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

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
