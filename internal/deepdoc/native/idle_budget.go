//go:build cgo

package native

// idle_budget.go — process-wide adaptive cap on pooled (idle) ONNX sessions.
//
// The session pools keep idle sessions to avoid cold starts, but each pooled
// session owns a CUDA BFC arena that never shrinks. Under a long multi-shape
// document the number of idle sessions grows until the arenas exhaust device
// memory. This file adds a device-memory-aware budget that:
//
//   - bounds the TOTAL number of idle sessions across all four model pools,
//   - evicts idle sessions (freeing their arena) when real device memory use
//     crosses a high-water mark, and
//   - prunes idle sessions that have not been used for an idle TTL.
//
// All limits are installed at startup by the EE GPU path (vram_ledger_ee.go).
// With no configuration the budget is disabled and behaviour is unchanged,
// which keeps CPU / OSS builds identical to before.
//
// In-use sessions are never touched: only sessions sitting idle in a pool are
// eligible for eviction, so correctness (one session per concurrent Run) is
// preserved.

import (
	"sync"
	"time"
)

// idleBudgetConfig is installed once at startup. Zero fields mean "no limit".
type idleBudgetConfig struct {
	enabled     bool
	totalCap    int           // max idle sessions across all pools (0 = no cap)
	highWaterMB int           // device-used above this => aggressive eviction
	lowWaterMB  int           // evict until device-used <= this
	idleTTL     time.Duration // prune idle sessions idle longer than this
	minIdleKeep int           // per pool: never prune below this many idle sessions
}

var idleBudget = struct {
	mu           sync.Mutex
	cfg          idleBudgetConfig
	totalIdle    int
	cachedUsedMB int
	pruners      []func(maxAge time.Duration, maxCount int) int // destroy oldest idle; return count
}{}

// ConfigureIdleBudget installs the process-wide idle-session budget. Intended
// to be called once, at startup, before any session is created.
func ConfigureIdleBudget(cfg idleBudgetConfig) {
	idleBudget.mu.Lock()
	defer idleBudget.mu.Unlock()
	idleBudget.cfg = cfg
}

// registerIdlePruner registers a pool's prune callback. maxAge=0 means "any
// age"; maxCount<=0 means "no count limit". The callback must destroy idle
// sessions oldest-first, skip the newest minIdleKeep, and return how many it
// destroyed.
func registerIdlePruner(fn func(maxAge time.Duration, maxCount int) int) {
	idleBudget.mu.Lock()
	defer idleBudget.mu.Unlock()
	idleBudget.pruners = append(idleBudget.pruners, fn)
}

// SetDeviceUsedMB updates the cached device-memory reading used by admission
// decisions. Sampling nvidia-smi is expensive, so the GPU janitor calls this
// periodically rather than on every release.
func SetDeviceUsedMB(mb int) {
	idleBudget.mu.Lock()
	defer idleBudget.mu.Unlock()
	idleBudget.cachedUsedMB = mb
}

// shouldPoolIdle atomically checks whether a just-released session may be
// kept idle AND reserves a slot in the global idle budget. The check and the
// totalIdle increment happen under one lock, so two concurrent admissions
// cannot both observe a below-cap count and push past totalCap (a
// check-then-increment TOCTOU). Returning true means the slot is already
// reserved for this caller.
func shouldPoolIdle() bool {
	idleBudget.mu.Lock()
	defer idleBudget.mu.Unlock()
	cfg := idleBudget.cfg
	if !cfg.enabled {
		return true
	}
	// Over the high-water mark: stop retaining idle sessions entirely.
	if cfg.highWaterMB > 0 && idleBudget.cachedUsedMB > cfg.highWaterMB {
		return false
	}
	// Global idle cap, reserved atomically with the check.
	if cfg.totalCap > 0 && idleBudget.totalIdle >= cfg.totalCap {
		return false
	}
	idleBudget.totalIdle++
	return true
}

func noteIdleRemoved() {
	idleBudget.mu.Lock()
	if idleBudget.totalIdle > 0 {
		idleBudget.totalIdle--
	}
	idleBudget.mu.Unlock()
}

// PruneIdle evicts idle sessions across all registered pools. It is driven by
// the GPU janitor. If force is true it prunes any idle session (used > high
// water); otherwise it only prunes sessions idle longer than the TTL. It
// returns the total number of sessions destroyed.
func PruneIdle(force bool) int {
	idleBudget.mu.Lock()
	cfg := idleBudget.cfg
	pruners := append([]func(time.Duration, int) int(nil), idleBudget.pruners...)
	if !cfg.enabled || len(pruners) == 0 {
		idleBudget.mu.Unlock()
		return 0
	}
	maxAge := cfg.idleTTL
	if force {
		maxAge = 0
	}
	idleBudget.mu.Unlock()

	total := 0
	for _, prune := range pruners {
		total += prune(maxAge, 0)
	}
	return total
}

// IdleBudgetStatus is a snapshot for startup logging / diagnostics.
func IdleBudgetStatus() (enabled bool, totalIdle, cachedUsedMB, totalCap int) {
	idleBudget.mu.Lock()
	defer idleBudget.mu.Unlock()
	c := idleBudget.cfg
	return c.enabled, idleBudget.totalIdle, idleBudget.cachedUsedMB, c.totalCap
}
