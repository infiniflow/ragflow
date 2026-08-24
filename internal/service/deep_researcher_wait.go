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

package service

import (
	"context"
	"sync"
)

func waitForDeepResearchWorkers(ctx context.Context, workers *sync.WaitGroup) error {
	// Sub-research goroutines invoke the progress callback, which
	// writes to the caller's channel; do not orphan them. Draining
	// here guarantees no callback fires after Research returns
	// (mirrors Python's asyncio.gather cancellation semantics).
	workers.Wait()
	return ctx.Err()
}
