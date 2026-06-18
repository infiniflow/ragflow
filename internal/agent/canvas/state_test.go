// Package canvas — state unit tests.
package canvas

import (
	"reflect"
	"sync"
	"testing"
)

// TestCanvasState_GetVarSetVar covers all 4 ref kinds (cpn@param, sys.x,
// env.x, item/index) plus missing keys, dot-path traversal, and concurrent
// read/write under the simple RWMutex.
func TestCanvasState_GetVarSetVar(t *testing.T) {
	type step struct {
		name    string
		ref     string
		want    any
		wantErr bool
	}
	cases := []struct {
		title  string
		setup  func(s *CanvasState)
		checks []step
	}{
		{
			title: "cpn_id@param direct",
			setup: func(s *CanvasState) {
				s.SetVar("retrieval_0", "chunks", []string{"a", "b"})
			},
			checks: []step{
				{"hit", "retrieval_0@chunks", []string{"a", "b"}, false},
				{"miss unknown cpn", "missing_0@chunks", nil, false},
				{"miss unknown param on known cpn", "retrieval_0@other", nil, false},
			},
		},
		{
			title: "cpn_id@param dot-path",
			setup: func(s *CanvasState) {
				s.SetVar("llm_0", "result", map[string]any{
					"text": "hi",
					"meta": map[string]any{"tokens": 42},
				})
			},
			checks: []step{
				{"two-level", "llm_0@result.meta.tokens", 42, false},
				{"one-level", "llm_0@result.text", "hi", false},
				{"deep miss", "llm_0@result.meta.absent", nil, false},
			},
		},
		{
			title: "sys namespace",
			setup: func(s *CanvasState) {
				s.Sys["query"] = "what is ragflow"
				s.Sys["user_id"] = "tenant-1"
			},
			checks: []step{
				{"sys.query", "sys.query", "what is ragflow", false},
				{"sys.user_id", "sys.user_id", "tenant-1", false},
				{"sys absent", "sys.missing", nil, false},
			},
		},
		{
			title: "env namespace",
			setup: func(s *CanvasState) {
				s.Env["max_tokens"] = 1024
			},
			checks: []step{
				{"env.max_tokens", "env.max_tokens", 1024, false},
				{"env absent", "env.min_tokens", nil, false},
			},
		},
		{
			title: "iteration aliases",
			setup: func(s *CanvasState) {
				// Tests run single-threaded; writing the Globals map
				// directly is safe and exercises the same read path
				// (GetVar locks internally) as production code.
				s.Globals["__item__"] = "item-value"
				s.Globals["__index__"] = 7
			},
			checks: []step{
				{"item", "item", "item-value", false},
				{"index", "index", 7, false},
			},
		},
		{
			title: "invalid ref",
			setup: func(s *CanvasState) {},
			checks: []step{
				{"no namespace and no @", "garbage", nil, true},
				{"empty", "", nil, true},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			s := NewCanvasState("run-test", "task-test")
			c.setup(s)
			for _, ch := range c.checks {
				got, err := s.GetVar(ch.ref)
				if ch.wantErr {
					if err == nil {
						t.Errorf("%s: expected error for ref %q, got nil (val=%v)", ch.name, ch.ref, got)
					}
					continue
				}
				if err != nil {
					t.Errorf("%s: unexpected error for ref %q: %v", ch.name, ch.ref, err)
					continue
				}
				if !equalValue(got, ch.want) {
					t.Errorf("%s: ref %q: got %v (%T), want %v (%T)", ch.name, ch.ref, got, got, ch.want, ch.want)
				}
			}
		})
	}
}

// TestCanvasState_SetVar_AutocreateNested confirms SetVar creates
// intermediate dicts for a dot-path, mirroring Python's
// set_variable_param_value (canvas.py:261-271).
func TestCanvasState_SetVar_AutocreateNested(t *testing.T) {
	s := NewCanvasState("r", "t")
	s.SetVar("cpn_0", "a.b.c", "deep")

	// GetVar locks internally; no need to wrap with an outer RLock
	// (a recursive Read lock would also work but is unnecessary).
	got, err := s.GetVar("cpn_0@a.b.c")
	if err != nil {
		t.Fatalf("GetVar: %v", err)
	}
	if got != "deep" {
		t.Fatalf("got %v, want \"deep\"", got)
	}
}

// TestCanvasState_ConcurrentReadWrite sanity-checks the RWMutex under mixed
// workload. The hard-gate benchmark (state_bench_test.go) measures the
// real numbers; this is a smoke test for race-detector cleanliness.
func TestCanvasState_ConcurrentReadWrite(t *testing.T) {
	s := NewCanvasState("r", "t")
	for i := 0; i < 50; i++ {
		s.SetVar(cpnID(i), "v", i)
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_, _ = s.GetVar(cpnID(i%50) + "@v")
				s.SetVar(cpnID(i%50), "v", i)
			}
		}()
	}
	wg.Wait()
}

// TestReadVars covers batch resolution for parameter binding.
func TestReadVars(t *testing.T) {
	s := NewCanvasState("r", "t")
	s.SetVar("a", "x", "alpha")
	s.SetVar("b", "y", "beta")
	s.Sys["query"] = "q1"

	refs := []string{"a@x", "b@y", "sys.query", "missing@z"}
	got, err := s.ReadVars(refs)
	if err != nil {
		t.Fatalf("ReadVars: %v", err)
	}
	if got["a@x"] != "alpha" {
		t.Errorf("a@x: got %v", got["a@x"])
	}
	if got["b@y"] != "beta" {
		t.Errorf("b@y: got %v", got["b@y"])
	}
	if got["sys.query"] != "q1" {
		t.Errorf("sys.query: got %v", got["sys.query"])
	}
	if got["missing@z"] != nil {
		t.Errorf("missing@z: expected nil, got %v", got["missing@z"])
	}
}

// equalValue is a small structural comparator — `int(42)` and `float64(42)`
// both count as "42" because the table tests were written for clarity, plus
// slice/map/struct equality via reflect.DeepEqual. Avoids the runtime panic
// that `==` produces on uncomparable types like []string.
func equalValue(got, want any) bool {
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}
	switch w := want.(type) {
	case int:
		switch g := got.(type) {
		case int:
			return w == g
		case int64:
			return int64(w) == g
		case float64:
			return float64(w) == g
		}
	}
	return reflect.DeepEqual(got, want)
}
