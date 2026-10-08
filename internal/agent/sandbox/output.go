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

package sandbox

import (
	"bytes"
	"sync"
)

// outputBudget is the single allowance shared by one child process's
// stdout and stderr captures. The configured max_output_bytes is a
// combined ceiling, so both streams draw from one pool: a process that
// floods stdout cannot spend the budget twice by also filling stderr.
//
// The two streams are written by independent goroutines (one copy
// goroutine per stream in both os/exec and x/crypto/ssh), so every
// field is mutex-guarded.
type outputBudget struct {
	mu    sync.Mutex
	limit int
	left  int
	total int
}

// newOutputBudget returns a budget of limit bytes, or an unlimited one
// when limit is not positive.
func newOutputBudget(limit int) *outputBudget {
	return &outputBudget{limit: limit, left: limit}
}

// take records n bytes received and reports how many of them the caller
// may retain. A limit of zero or less retains everything.
func (b *outputBudget) take(n int) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total += n
	if b.limit <= 0 {
		return n
	}
	retained := min(n, max(0, b.left))
	b.left -= retained
	return retained
}

// usage reports the combined bytes received across both streams and
// whether that exceeded the budget.
func (b *outputBudget) usage() (total int, exceeded bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.total, b.limit > 0 && b.total > b.limit
}

// outputCapture is the bounded sink for one of a child process's output
// streams. Providers that stream an untrusted child's output into this
// process's memory capture it here, so the bytes retained stop growing
// at the budget instead of the whole stream arriving first and being
// measured afterwards.
//
// Write retains only what the shared budget still allows but reports
// the full length it was handed, so a chatty child never blocks or
// errors on a short write.
type outputCapture struct {
	buffer bytes.Buffer
	budget *outputBudget
}

func (c *outputCapture) Write(p []byte) (int, error) {
	retained := c.budget.take(len(p))
	_, err := c.buffer.Write(p[:retained])
	return len(p), err
}
