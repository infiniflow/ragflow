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

package progress

import (
	"context"
	"sync"

	"ragflow/internal/agent/runtime"
)

// Counter reports a component's discrete progress positions under one unit.
// It emits 0/total at construction, keeps current monotonic, and Finish()
// reports the terminal position. A zero total disables reporting, so callers
// with no measurable work stay observer-independent.
//
// The fraction channel is driven by the embedded Window, the same abstraction
// Window.Counter uses, so both progress paths share one fraction implementation.
type Counter struct {
	mu       sync.Mutex
	ctx      context.Context
	unit     string
	total    uint32
	current  uint32
	finished bool
	window   Window
}

// NewCounter creates a counter for unit and immediately reports 0/total when
// total is positive. The Window shapes the fraction channel; a zero Span keeps
// the counter detail-only.
func NewCounter(ctx context.Context, unit string, total uint32, window Window) *Counter {
	c := &Counter{ctx: ctx, unit: unit, total: total, window: window}
	if total > 0 {
		// Establish the detail position without emitting a fraction: document
		// progress is monotonic and must not be reset to zero at phase start.
		runtime.ReportProgressDetail(ctx, unit, 0, total)
	}
	return c
}

// Report advances to current/total. The first report establishes the total
// when the constructor did not know it. Stale or duplicate positions are
// ignored so out-of-order callbacks cannot regress the bar; values above the
// total are clamped.
func (c *Counter) Report(current, total uint32) {
	if total == 0 {
		return
	}
	c.mu.Lock()
	if c.finished {
		c.mu.Unlock()
		return
	}
	if c.total == 0 {
		c.total = total
	}
	if current > c.total {
		current = c.total
	}
	if current <= c.current {
		c.mu.Unlock()
		return
	}
	c.current = current
	// Emit while holding the lock: concurrent callers must observe positions
	// in increasing order (the sink logs every report, not just the latest).
	c.emit(current, c.total)
	c.mu.Unlock()
}

// Inc advances the counter by one position.
func (c *Counter) Inc() {
	c.mu.Lock()
	if c.finished || c.total == 0 || c.current >= c.total {
		c.mu.Unlock()
		return
	}
	c.current++
	c.emit(c.current, c.total)
	c.mu.Unlock()
}

// Finish reports the terminal position once. Later reports are ignored.
func (c *Counter) Finish() {
	c.mu.Lock()
	if c.finished || c.total == 0 {
		c.mu.Unlock()
		return
	}
	c.finished = true
	already := c.current == c.total
	c.current = c.total
	if !already {
		c.emit(c.total, c.total)
	}
	c.mu.Unlock()
}

func (c *Counter) emit(current, total uint32) {
	if c.unit != "" && c.ctx.Err() == nil {
		runtime.ReportProgressDetail(c.ctx, c.unit, current, total)
	}
	c.window.Report(c.ctx, float64(current)/float64(total))
}
