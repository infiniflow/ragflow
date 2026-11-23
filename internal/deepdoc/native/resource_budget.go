//go:build cgo

package native

// resource_budget.go — media-agnostic adaptive resource budget for pooled
// inference sessions.
//
// Both GPU (VRAM) and CPU (memory) inference run multiple sessions that each
// retain an arena. Without a budget, idle sessions accumulate and exhaust the
// device/machine. This file holds the shared machinery:
//   - ResourceBudget abstracts "total" and "used" for the medium in use
//   - a janitor samples "used" and prunes idle sessions / engages backpressure
//     at high water, lifting it at low water (hysteresis)
//   - the adaptive-K formula, shared by GPU and CPU
//
// A concrete ResourceBudget is supplied by the platform-specific file:
//   - resource_budget_ee.go provides the GPU (nvidia-smi) implementation
//   - this file provides the CPU (cgroup / procfs) implementation
//
// Nothing here activates by default: the caller (server startup) opts in.

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"ragflow/internal/common"
	doctype "ragflow/internal/deepdoc/parser/type"
)

// ResourceBudget reports the total and currently-used capacity (MiB) of the
// resource that backs inference sessions. Implementations must return 0 when
// detection fails, so callers can degrade gracefully.
type ResourceBudget interface {
	TotalMB() int
	UsedMB() int
	Name() string
}

const (
	envResourceBudgetEnabled = "RAGFLOW_DEEPDOC_RESOURCE_BUDGET"
	envResourceBudgetRatio   = "RAGFLOW_DEEPDOC_VRAM_BUDGET_RATIO"
	envResourceHighWater     = "RAGFLOW_DEEPDOC_VRAM_HIGH_WATER"
	envResourceLowWater      = "RAGFLOW_DEEPDOC_VRAM_LOW_WATER"
	envIdleTTL               = "RAGFLOW_DEEPDOC_IDLE_TTL_SEC"
	envIdleCapPerGB          = "RAGFLOW_DEEPDOC_IDLE_SESSIONS_PER_GB"
	envIdleMinKeep           = "RAGFLOW_DEEPDOC_IDLE_MIN_KEEP"
	envJanitorIntervalSec    = "RAGFLOW_DEEPDOC_VRAM_POLL_SEC"

	defBudgetRatio = 0.90
	defHighWater   = 0.95
	defLowWater    = 0.65
	defIdleTTL     = 20 * time.Second
	defCapPerGB    = 2
	defMinKeep     = 1
	defPollSec     = 2
)

func envFloat(name string, def float64) float64 {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil && v > 0 {
			return v
		}
	}
	return def
}
func envInt(name string, def int) int {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			return v
		}
	}
	return def
}

// InitResourceBudget installs the adaptive budget using the supplied resource
// and starts the janitor. It is the single activation point for both GPU and
// CPU builds. If b is nil or detection fails, it is a no-op.
func InitResourceBudget(ctx context.Context, b ResourceBudget, defaultOn bool) {
	if b == nil {
		return
	}
	raw, set := os.LookupEnv(envResourceBudgetEnabled)
	enabled := defaultOn
	if set {
		enabled = strings.TrimSpace(raw) != "0"
	}
	if !enabled {
		common.Info("adaptive resource budget disabled",
			zap.String("resource", b.Name()), zap.String("env", envResourceBudgetEnabled))
		return
	}

	totalMB := b.TotalMB()
	if totalMB <= 0 {
		common.Warn("adaptive resource budget: total unavailable; skipping",
			zap.String("resource", b.Name()))
		return
	}

	ratio := envFloat(envResourceBudgetRatio, defBudgetRatio)
	budgetMB := int(float64(totalMB) * ratio)
	highWaterMB := int(float64(budgetMB) * envFloat(envResourceHighWater, defHighWater))
	lowWaterMB := int(float64(budgetMB) * envFloat(envResourceLowWater, defLowWater))

	ttlSec := envInt(envIdleTTL, int(defIdleTTL/time.Second))
	capPerGB := envInt(envIdleCapPerGB, defCapPerGB)
	totalCap := (totalMB / 1024) * capPerGB
	if totalCap < 4 {
		totalCap = 4
	}

	cfg := idleBudgetConfig{
		enabled:     true,
		totalCap:    totalCap,
		highWaterMB: highWaterMB,
		lowWaterMB:  lowWaterMB,
		idleTTL:     time.Duration(ttlSec) * time.Second,
		minIdleKeep: envInt(envIdleMinKeep, defMinKeep),
	}
	ConfigureIdleBudget(cfg)

	common.Info("adaptive resource budget installed",
		zap.String("resource", b.Name()),
		zap.Int("total_mb", totalMB),
		zap.Int("budget_mb", budgetMB),
		zap.Int("high_water_mb", highWaterMB),
		zap.Int("low_water_mb", lowWaterMB),
		zap.Int("idle_total_cap", totalCap),
		zap.Int("idle_ttl_sec", ttlSec))

	pollSec := envInt(envJanitorIntervalSec, defPollSec)
	if pollSec < 1 {
		pollSec = 1
	}
	go resourceJanitor(ctx, b, time.Duration(pollSec)*time.Second, highWaterMB)
}

func resourceJanitor(ctx context.Context, b ResourceBudget, interval time.Duration, highMB int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			used := b.UsedMB()
			if used <= 0 {
				// A failed read is NOT a valid low-water sample. If an earlier
				// high sample engaged backpressure, leaving it set here would
				// block every deadline-less acquire for as long as reads keep
				// failing. Lift it on a failed sample so the gate cannot outlive
				// a valid reading.
				if doctype.Backpressure() {
					doctype.SetBackpressure(false)
					common.Warn("resource budget: memory sample unavailable, backpressure OFF to avoid stall",
						zap.String("resource", b.Name()))
				}
				continue
			}
			SetDeviceUsedMB(used)
			enabled, _, _, _ := IdleBudgetStatus()
			if !enabled {
				continue
			}
			if used > highMB {
				if !doctype.Backpressure() {
					doctype.SetBackpressure(true)
					common.Warn("resource budget: high water, backpressure ON",
						zap.String("resource", b.Name()),
						zap.Int("used_mb", used), zap.Int("high_mb", highMB))
				}
				PruneIdle(true)
			} else {
				if doctype.Backpressure() {
					doctype.SetBackpressure(false)
					common.Info("resource budget: below high water, backpressure OFF",
						zap.String("resource", b.Name()),
						zap.Int("used_mb", used), zap.Int("high_mb", highMB))
				}
				// Always apply ordinary TTL pruning when not over high water —
				// including between the watermarks. Restricting it to used<lowMB
				// left idle sessions older than idleTTL stranded in the band.
				PruneIdle(false)
			}
		}
	}
}

// computeAdaptiveK is the pure K formula, shared by GPU and CPU.
func computeAdaptiveK(totalMB, weightsMB int, budgetRatio float64, perRunMB, kMin, kMax int) int {
	budgetMB := float64(totalMB) * budgetRatio
	headroomMB := budgetMB * 0.05
	usableMB := budgetMB - headroomMB - float64(weightsMB)
	if usableMB <= 0 {
		return kMin
	}
	k := int(usableMB / float64(perRunMB))
	if k < kMin {
		k = kMin
	}
	if k > kMax {
		k = kMax
	}
	return k
}

// detectedWeightsMB sums the on-disk model files in the model directory.
// The caller passes the platform-specific file names (OSS *.ort vs EE
// *.ee.cpu.ort / *.ee.gpu.ort), keeping this shared machinery EE-agnostic.
func detectedWeightsMB(files []string) int {
	dir := strings.TrimSpace(os.Getenv("EE_MODEL_DIR"))
	if dir == "" {
		return 0
	}
	var total int64
	for _, f := range files {
		info, err := os.Stat(dir + "/" + f)
		if err == nil {
			total += info.Size()
		}
	}
	return int(total / (1024 * 1024))
}

// ── CPU resource: cgroup (v2 then v1), falling back to /proc/meminfo ──────

type CPUResource struct{}

func (CPUResource) Name() string { return "cpu-memory" }

func (CPUResource) TotalMB() int {
	if v, ok := readCgroup("memory.max"); ok && v > 0 {
		return int(v / 1024 / 1024)
	}
	if v, ok := readCgroupV1("memory/memory.limit_in_bytes"); ok && v > 0 && v < (1<<62) {
		return int(v / 1024 / 1024)
	}
	return procMemTotalMB()
}

func (CPUResource) UsedMB() int {
	if v, ok := readCgroup("memory.current"); ok && v > 0 {
		return int(v / 1024 / 1024)
	}
	if v, ok := readCgroupV1("memory/memory.usage_in_bytes"); ok && v > 0 {
		return int(v / 1024 / 1024)
	}
	return 0
}

// cgroupRoot / cgroupProcPath are env-overridable seams so the cgroup parsing
// can be tested against a synthetic filesystem. Defaults are the real paths.
func cgroupRoot() string {
	if v := strings.TrimSpace(os.Getenv("RAGFLOW_CGROUP_ROOT")); v != "" {
		return v
	}
	return "/sys/fs/cgroup"
}

func cgroupProcPath() string {
	if v := strings.TrimSpace(os.Getenv("RAGFLOW_CGROUP_PROC")); v != "" {
		return v
	}
	return "/proc/self/cgroup"
}

// readCgroup reads a cgroup v2 file from this process's cgroup path.
// "max" (unlimited) returns ok=false.
func readCgroup(file string) (int64, bool) {
	path := cgroupV2Path()
	if path == "" {
		return 0, false
	}
	data, err := os.ReadFile(cgroupRoot() + path + "/" + file)
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(data))
	if s == "max" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	return v, err == nil && v > 0
}

// readCgroupV1 reads a cgroup-v1 controller file from the PROCESS's subgroup,
// not the controller root. The process's membership comes from the matching
// line in /proc/self/cgroup (e.g. "5:memory:/ragflow"); reading the root file
// would report host-wide limits in a container and defeat the idle/adaptive
// budget entirely.
func readCgroupV1(rel string) (int64, bool) {
	subgroup := cgroupV1Subgroup("memory")
	path := filepath.Join(cgroupRoot(), rel)
	if subgroup != "" && subgroup != "/" {
		// rel is "memory/memory.limit_in_bytes"; its first segment is the
		// controller. Insert the subgroup right after it.
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) == 2 {
			path = filepath.Join(cgroupRoot(), parts[0], subgroup, parts[1])
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	return v, err == nil && v > 0
}

// cgroupV2Path returns the process's cgroup-v2 path from /proc/self/cgroup
// (unified hierarchy, controller list field is empty).
func cgroupV2Path() string {
	return cgroupPathFor("")
}

// cgroupV1Subgroup returns the process's subgroup path for the named cgroup-v1
// controller (e.g. "memory") from /proc/self/cgroup.
func cgroupV1Subgroup(controller string) string {
	return cgroupPathFor(controller)
}

// cgroupPathFor scans /proc/self/cgroup lines of the form
// "hierarchy:controllers:path" and returns path for the line whose controller
// list matches: empty name matches the v2 unified line ("0::/..."), a named
// controller matches a v1 line containing that controller.
func cgroupPathFor(controller string) string {
	f, err := os.Open(cgroupProcPath())
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), ":", 3)
		if len(parts) != 3 {
			continue
		}
		if controller == "" {
			// v2 unified: empty controller list.
			if parts[1] == "" {
				return parts[2]
			}
		} else {
			for _, c := range strings.Split(parts[1], ",") {
				if c == controller {
					return parts[2]
				}
			}
		}
	}
	return ""
}

func procMemTotalMB() int {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "MemTotal:") {
			fields := strings.Fields(sc.Text())
			if len(fields) >= 2 {
				if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					return int(kb / 1024)
				}
			}
		}
	}
	return 0
}
