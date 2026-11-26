//go:build cgo

package native

import (
	"testing"
	"time"
)

// fakePooledSession is a minimal pooledSession for budget tests.
type fakePooledSession struct{ destroyed *bool }

func (f fakePooledSession) Destroy() {
	if f.destroyed != nil {
		*f.destroyed = true
	}
}
func (f fakePooledSession) isPoisoned() bool { return false }
func (f fakePooledSession) markPoisoned()    {}

func TestIdleBudgetAdmissionAndPrune(t *testing.T) {
	// Reset state.
	idleBudget.mu.Lock()
	idleBudget.cfg = idleBudgetConfig{}
	idleBudget.totalIdle = 0
	idleBudget.cachedUsedMB = 0
	idleBudget.pruners = nil
	idleBudget.mu.Unlock()

	// With budget disabled, everything is always pooled.
	if !shouldPoolIdle() {
		t.Fatal("disabled budget must always pool")
	}

	// Enable with a tiny cap and a high-water mark. shouldPoolIdle() now
	// check-and-reserves atomically: each true reserves one idle slot.
	ConfigureIdleBudget(idleBudgetConfig{
		enabled: true, totalCap: 2, highWaterMB: 100, lowWaterMB: 50,
		idleTTL: 1 * time.Second, minIdleKeep: 0,
	})
	if !shouldPoolIdle() { // reserves slot 1
		t.Fatal("below high water should pool")
	}

	SetDeviceUsedMB(200) // above high water
	if shouldPoolIdle() {
		t.Fatal("above high water must not pool")
	}

	SetDeviceUsedMB(20)    // below high water
	if !shouldPoolIdle() { // reserves slot 2 -> totalIdle == cap
		t.Fatal("below high water should pool")
	}
	if shouldPoolIdle() {
		t.Fatal("at cap must not pool")
	}
	noteIdleRemoved() // free one slot
	if !shouldPoolIdle() {
		t.Fatal("below cap should pool again")
	}
}

func TestIdleBudgetStatusDefaultDisabled(t *testing.T) {
	idleBudget.mu.Lock()
	idleBudget.cfg = idleBudgetConfig{}
	idleBudget.mu.Unlock()
	en, _, _, _ := IdleBudgetStatus()
	if en {
		t.Fatal("default config must be disabled")
	}
}
