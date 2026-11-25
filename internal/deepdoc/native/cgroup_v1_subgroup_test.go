//go:build cgo

package native

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCPUMemoryLimitUsesCgroupSubgroup reproduces the cgroup-v1 path bug:
// readCgroupV1 reads the controller-ROOT file, ignoring the process's own
// subgroup from /proc/self/cgroup. In a container limited to a 2 GiB subgroup
// on a 64 GiB host, TotalMB then reports host RAM and the idle cap / pressure
// thresholds are computed against the wrong limit.
//
// Adversarial fixture: root says 64 GiB, subgroup says 2 GiB. A correct
// implementation must report the subgroup value, not the root value. The test
// injects mock paths via env seams; the current code ignores them and reads
// the real (root) files -> RED.
func TestCPUMemoryLimitUsesCgroupSubgroup(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "cgroup")
	// cgroup-v1 layout: controller root + a subgroup for this process.
	sub := filepath.Join(root, "memory", "ragflow")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	// Root limit = 64 GiB; subgroup limit = 2 GiB.
	writeLimit(t, filepath.Join(root, "memory", "memory.limit_in_bytes"), 64<<30)
	writeLimit(t, filepath.Join(sub, "memory.limit_in_bytes"), 2<<30)
	writeLimit(t, filepath.Join(root, "memory", "memory.usage_in_bytes"), 60<<30)
	writeLimit(t, filepath.Join(sub, "memory.usage_in_bytes"), 1<<30)

	// /proc/self/cgroup line for cgroup v1 memory controller.
	proc := filepath.Join(tmp, "proc_self_cgroup")
	if err := os.WriteFile(proc, []byte("5:memory:/ragflow\n"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("RAGFLOW_CGROUP_ROOT", root)
	t.Setenv("RAGFLOW_CGROUP_PROC", proc)

	// The v2 read must not fire (no v2 file in the mock), so this exercises v1.
	if got := (CPUResource{}).TotalMB(); got != 2<<10 {
		t.Fatalf("TotalMB=%d MiB, want 2048 (subgroup limit, not root)", got)
	}
	if got := (CPUResource{}).UsedMB(); got != 1<<10 {
		t.Fatalf("UsedMB=%d MiB, want 1024 (subgroup usage, not root)", got)
	}
}

func writeLimit(t *testing.T, path string, bytes int64) {
	t.Helper()
	if err := os.WriteFile(path, []byte(itoa(bytes)), 0644); err != nil {
		t.Fatal(err)
	}
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [24]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
