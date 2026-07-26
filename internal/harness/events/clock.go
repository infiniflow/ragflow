package events

import (
	"sync/atomic"
)

// LogicalClock is a monotonic logical clock that provides a total order
// for events within a process. It is safe for concurrent use.
type LogicalClock struct {
	value atomic.Uint64
}

// NewLogicalClock creates a new LogicalClock starting at 0.
func NewLogicalClock() *LogicalClock {
	return &LogicalClock{}
}

// Tick atomically increments the clock and returns the new value.
func (c *LogicalClock) Tick() uint64 {
	return c.value.Add(1)
}

// Now returns the current clock value without incrementing.
func (c *LogicalClock) Now() uint64 {
	return c.value.Load()
}

// Reset resets the clock to zero. Use with caution — this should only
// be done when starting a completely independent execution context.
func (c *LogicalClock) Reset() {
	c.value.Store(0)
}
