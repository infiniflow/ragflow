package native

import "testing"

// TestModelConfigOverridable guards the contract that DeepDoc model filenames,
// the detection box threshold, and the DLA post-processing indirection are
// package-level variables (override seams assigned by a downstream build variant
// at init/startup, before any inference runs, set once and never mutated during
// serving).
//
// If any of these are still literals or constants, this test fails to compile,
// which is the intended red signal before the refactor lands.
func TestModelConfigOverridable(t *testing.T) {
	type fileVar struct {
		name string
		get  func() string
		set  func(string)
		want string
	}
	files := []fileVar{
		{"det", func() string { return modelFileDet }, func(s string) { modelFileDet = s }, "det.ort"},
		{"rec", func() string { return modelFileRec }, func(s string) { modelFileRec = s }, "rec.ort"},
		{"layout", func() string { return modelFileLayout }, func(s string) { modelFileLayout = s }, "layout.ort"},
		{"tsr", func() string { return modelFileTsr }, func(s string) { modelFileTsr = s }, "tsr.ort"},
	}
	for _, f := range files {
		if got := f.get(); got != f.want {
			t.Fatalf("%s default = %q, want %q", f.name, got, f.want)
		}
		saved := f.get()
		f.set("OVERRIDE-" + f.name)
		if got := f.get(); got != "OVERRIDE-"+f.name {
			t.Fatalf("%s override = %q, want OVERRIDE-%s", f.name, got, f.name)
		}
		f.set(saved)
		if got := f.get(); got != f.want {
			t.Fatalf("%s restore = %q, want %q", f.name, got, f.want)
		}
	}

	if detBoxThresh != 0.5 {
		t.Fatalf("detBoxThresh default = %v, want 0.5", detBoxThresh)
	}
	savedThresh := detBoxThresh
	detBoxThresh = 0.99
	if detBoxThresh != 0.99 {
		t.Fatalf("detBoxThresh override = %v, want 0.99", detBoxThresh)
	}
	detBoxThresh = savedThresh
	if detBoxThresh != 0.5 {
		t.Fatalf("detBoxThresh restore = %v, want 0.5", detBoxThresh)
	}

	if dlaPostprocessFn == nil {
		t.Fatal("dlaPostprocessFn must be set by default")
	}
}
