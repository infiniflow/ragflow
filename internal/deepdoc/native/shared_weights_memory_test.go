//go:build cgo && manual

package native

// Manual (local-only) verification that the patched ONNX Runtime static
// library actually shares one copy of the model weights across many sessions,
// instead of each session deserializing its own private copy.
//
// Empirical method (run twice, one mode per process to avoid glibc arena
// cross-contamination):
//   MODE=nonshared  -> open N sessions that each deserialize their own weights
//   MODE=shared     -> open N sessions that all inject the same cached weight set
// In each mode we create the sessions one at a time (never freeing between) and
// record the per-session RSS delta. The non-shared slope includes the
// per-session weight copy (~model weights); the shared slope must not — the
// weights live in the single cached set allocated once at extraction. If
// sharing works, the shared per-session delta is ~model-weights smaller than
// the non-shared one.
//
// This is the runtime counterpart to the ORT source proof:
//   - SessionOptions::AddInitializer stores the OrtValue pointer (no data copy)
//     (session_options.cc)
//   - the session consumes it via `ort_value = *(initializers_to_share_map[name])`
//     — a shallow OrtValue copy that keeps the same user-owned data buffer
//     (session_state_utils.cc).
//
// rec.ort is used because its input tensor is tiny (~184KB), so per-session
// input-tensor allocation does not mask the weight-sharing signal.
//
// Run with:
//   MODE=shared   MODEL_DIR=/path/to/models \
//     go test -tags cgo,manual -run TestWeightSharingMemory -v .
//   MODE=nonshared ... (same, different MODE)

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The native package's TestMain skips the whole binary unless the external
// golden testdata was fetched. This manual verification needs only MODEL_DIR
// (the .ort weights), not the golden fixtures, so opt in here.
func init() {
	testdataFetchAttempted = true
}

func rssMB() int64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return (pages * int64(os.Getpagesize())) >> 20
}

func fileSizeMB(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size() >> 20
}

// TestWeightSharingMemory opens N rec sessions in shared or non-shared mode and
// reports the total RSS delta. Compare the two modes to confirm sharing removes
// the per-session weight copy from the slope.
func TestWeightSharingMemory(t *testing.T) {
	if os.Getenv("MODEL_DIR") == "" {
		t.Skip("set MODEL_DIR to run the weight-sharing memory verification")
	}
	mode := os.Getenv("MODE")
	if mode != "shared" && mode != "nonshared" {
		t.Skip("set MODE=shared or MODE=nonshared")
	}
	if err := InitORT(); err != nil {
		t.Fatalf("InitORT: %v", err)
	}

	modelPath := filepath.Join(os.Getenv("MODEL_DIR"), "rec.ort")
	inName := "x"
	inShape := []int64{1, 3, 48, 320}
	outName := "softmax_11.tmp_0"

	const N = 50

	var ws *weightSet
	if mode == "shared" {
		var e error
		ws, e = sharedWeights(modelPath, inName, inShape, outName)
		if e != nil {
			t.Fatalf("sharedWeights: %v", e)
		}
		t.Logf("extracted %d initializers (one shared copy, allocated once)", len(ws.vals))
	}

	sessions := make([]*recSession, 0, N)
	rss0 := rssMB()
	prev := rss0
	var totalDelta int64
	for i := 0; i < N; i++ {
		var h *recSession
		var e error
		if mode == "shared" {
			h, e = newRecSession(modelPath, inName, inShape, outName, ws)
		} else {
			h, e = newRecSession(modelPath, inName, inShape, outName, nil)
		}
		if e != nil {
			t.Fatalf("newRecSession #%d (%s): %v", i+1, mode, e)
		}
		sessions = append(sessions, h)
		runtime.GC()
		cur := rssMB()
		totalDelta += cur - prev
		prev = cur
	}
	// Exercise every session so ORT actually uses the weights.
	ctx := context.Background()
	input := make([]float32, prod(inShape))
	for _, h := range sessions {
		if _, err := h.Run(ctx, input); err != nil {
			t.Fatalf("Run: %v", err)
		}
	}
	t.Logf("MODE=%s: N=%d sessions, total RSS delta=%d MB (from %d MB)", mode, N, totalDelta, rss0)
	for _, h := range sessions {
		h.Destroy()
	}
}
