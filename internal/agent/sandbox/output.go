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

import "bytes"

// outputCapture is the bounded sink for a child process's stdout and
// stderr. Providers that stream an untrusted child's output into this
// process's memory capture it here, so the bytes retained stop growing
// at the cap instead of the whole stream arriving first and being
// measured afterwards.
//
// Write retains at most limit bytes but reports the full length it was
// handed, so a chatty child never blocks or errors on a short write;
// total still counts every byte seen, which is what lets a caller
// detect that the stream was truncated after the fact.
type outputCapture struct {
	buffer bytes.Buffer
	limit  int
	total  int
}

func (c *outputCapture) Write(p []byte) (int, error) {
	n := len(p)
	c.total += n
	if c.limit > 0 {
		p = p[:min(len(p), max(0, c.limit-c.buffer.Len()))]
	}
	_, err := c.buffer.Write(p)
	return n, err
}
