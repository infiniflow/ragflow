//go:build cgo

package native

// session.go — thin wrapper around onnxruntime_go.
//
// Hides all onnxruntime-go specifics from the recognizers so each task module
// only deals with float32 tensors. One model input, one model output (every
// DeepDoc ONNX we port fits this shape). CPU-only by design.
//
// The ONNX Runtime environment is process-global: InitORT sets the shared
// library and initializes it exactly once. Sessions only own their tensors and
// the advanced-session handle, so running several tasks in one process (or one
// task per CLI invocation) never double-initializes or prematurely tears down
// the shared environment.

import (
	"context"
	"fmt"
	"sync"

	ort "github.com/infiniflow/onnxruntime_go"
)

var (
	ortOnce    sync.Once
	ortInitErr error
	// ortReady is true once InitializeEnvironment has succeeded. It lets
	// callers decide whether the in-process backend can serve without
	// triggering a panic from a session Run against an uninitialized
	// environment.
	ortReady bool
)

// InitORT initializes the process-global ONNX Runtime environment. Safe to
// call multiple times; only the first takes effect. Call it once at process
// start (the CLI does this from main).
//
// The in-process DeepDoc backend links ONNX Runtime statically (libonnxruntime.a
// is linked into the binary with --whole-archive and exported via
// -Wl,--export-dynamic; see build.sh: ONNX_RUNTIME_STATIC_DIR). The org
// onnxruntime_go binding (github.com/infiniflow/onnxruntime_go) resolves
// OrtGetApiBase from the running binary itself via dlopen(NULL) (the
// process-global symbol table), so no external libonnxruntime.so is needed and
// there is no dynamic .so deployment. A main executable CANNOT be dlopen'd by
// its own file path (glibc refuses), which is exactly why the binding uses the
// NULL handle instead of a path. InitORT therefore takes no library path;
// ragflow never calls SetSharedLibraryPath, so the binding resolves ORT from the
// running binary via dlopen(NULL).
func InitORT() error {
	ortOnce.Do(func() {
		ortInitErr = ort.InitializeEnvironment()
		if ortInitErr == nil {
			ortReady = true
		}
	})
	return ortInitErr
}

// Initialized reports whether ONNX Runtime's process-global environment has
// been successfully initialized. The in-process DeepDoc backend uses this to
// decide whether it can serve, degrading to an empty analyzer otherwise.
func Initialized() bool { return ortReady }

// session loads one ONNX model and runs single-input/single-output inference.
type session struct {
	inName  string
	outName string
	outSize int64
	sess    *ort.AdvancedSession
	in      *ort.Tensor[float32]
	out     *ort.Tensor[float32]
	// poisoned is set when a Run is cancelled/terminated via context. ONNX
	// Runtime does not guarantee a session is reusable after a forced
	// termination, so the pool must Destroy rather than re-Put it.
	poisoned bool
}

// NewSession opens modelPath. inShape/outShape describe the fixed tensor
// dimensions; outSize is the total element count of the output tensor.
// intraOpThreads controls ONNX Runtime's intra-op parallelism. DLA/TSR/OCR-rec
// pass 0 to use all cores — matching deepdoc's Python onnxruntime
// (intra_op_num_threads defaults to 0 = all cores); their Run path does no
// contour extraction, so the parallel reduction order matches Python for
// bit-stable parity.
//
// The DB text detector is currently pinned to 1 (single-threaded). NOTE: the
// historical rationale for this — that a multi-threaded Run would leave ONNX
// Runtime worker threads settling while the postprocess's OpenCV findContours
// ran its parallel_for_ and under-ran — does NOT apply to this pure-Go port.
// Here the postprocess (hand-rolled findContours / boxScoreFast / fillPoly) is
// fully synchronous and only runs after RunWithOptions returns, so there is no
// thread competition with the detector. The pin is preserved as-is because the
// det pred map was verified at intraOpThreads=1 (mean|Δ|≈4e-5 vs the Python
// reference); flipping it to 0 must be re-confirmed on the det integration
// fixtures before landing, since the reduction order differs.
// InitORT must have been called first.
func NewSession(modelPath, inName string, inShape []int64, outName string, outShape []int64, intraOpThreads int) (*session, error) {
	in := make([]float32, prod(inShape))
	out := make([]float32, prod(outShape))
	inT, err := ort.NewTensor(ort.NewShape(inShape...), in)
	if err != nil {
		return nil, err
	}
	outT, err := ort.NewTensor(ort.NewShape(outShape...), out)
	if err != nil {
		inT.Destroy()
		return nil, err
	}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		inT.Destroy()
		outT.Destroy()
		return nil, err
	}
	// intraOpThreads == 0 → all cores (mirrors Python's onnxruntime default);
	// the DB detector passes 1, preserved as-is for verified det parity (see
	// NewSession doc above — the old findContours/parallel_for_ rationale does
	// not apply to this pure-Go port).
	if err := opts.SetIntraOpNumThreads(intraOpThreads); err != nil {
		opts.Destroy()
		inT.Destroy()
		outT.Destroy()
		return nil, err
	}
	sess, err := ort.NewAdvancedSession(modelPath,
		[]string{inName}, []string{outName},
		[]ort.Value{inT}, []ort.Value{outT}, opts)
	if err != nil {
		opts.Destroy()
		inT.Destroy()
		outT.Destroy()
		return nil, err
	}
	return &session{
		inName: inName, outName: outName,
		outSize: prod(outShape),
		sess:    sess, in: inT, out: outT,
	}, nil
}

// Run copies input into the input tensor, executes, and returns the output
// tensor contents. ctx bounds the inference: if it is cancelled while Run is
// in flight, the underlying ONNX Runtime call is terminated via RunOptions.
// A terminated session is left in an indeterminate state, so it is marked
// poisoned and the pool destroys it instead of reusing it.
func (s *session) Run(ctx context.Context, input []float32) ([]float32, error) {
	if len(input) != len(s.in.GetData()) {
		return nil, fmt.Errorf("session %s: input len %d != tensor len %d",
			s.outName, len(input), len(s.in.GetData()))
	}
	opts, err := ort.NewRunOptions()
	if err != nil {
		return nil, err
	}
	defer opts.Destroy()
	// Cancel an in-flight Run when the context is done. done closes once Run
	// returns so the watcher exits even on the success path.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = opts.Terminate()
		case <-done:
		}
	}()

	copy(s.in.GetData(), input)
	if err := s.sess.RunWithOptions(opts); err != nil {
		if ctx.Err() != nil {
			s.poisoned = true
		}
		return nil, err
	}
	out := make([]float32, s.outSize)
	copy(out, s.out.GetData())
	return out, nil
}

// Destroy releases the tensors and advanced-session handle. It does NOT touch
// the process-global environment.
func (s *session) Destroy() {
	if s.sess != nil {
		s.sess.Destroy()
	}
	if s.in != nil {
		s.in.Destroy()
	}
	if s.out != nil {
		s.out.Destroy()
	}
}

func (s *session) isPoisoned() bool { return s.poisoned }

// markPoisoned records that a Run was force-terminated; a poisoned session is
// not safe to reuse, so the pool Destroys it on release.
func (s *session) markPoisoned() { s.poisoned = true }

func prod(shape []int64) int64 {
	p := int64(1)
	for _, d := range shape {
		p *= d
	}
	return p
}
