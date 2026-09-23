//go:build cgo

package native

// session.go — thin wrapper around onnxruntime_go.
//
// Hides all onnxruntime-go specifics from the recognizers so each task module
// only deals with float32 tensors. One model input, one model output (every
// DeepDoc ONNX we port fits this shape). CPU-only by design.
//
// The ONNX Runtime environment is process-global: InitORT sets the shared
// library and initializes it exactly once. A session owns only its
// DynamicAdvancedSession handle (input/output tensors are allocated per Run and
// freed afterwards), so running several tasks in one process (or one task per
// CLI invocation) never double-initializes or prematurely tears down the
// shared environment.

import (
	"context"
	"fmt"
	"log"
	"sync"

	ort "github.com/infiniflow/onnxruntime_go"
)

// intraOpThreads is the intra-op thread count every session is opened with.
//
// ONNX Runtime gives each session its own intra-op thread pool (the C API
// never switches a session onto a shared/global pool), so the threads DeepDoc
// inference occupies in this process are intraOpThreads × the number of
// concurrently running sessions. Pinning it to 1 keeps every Run to a single
// thread, which is what makes the process ceiling a plain concurrency budget:
// the capacity registered in inference_limit.go bounds how many Runs may be in
// flight, and each of them costs exactly one thread.
const intraOpThreads = 1

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
// The in-process DeepDoc backend links ONNX Runtime statically:
// libonnxruntime.a is linked in with no --whole-archive, so GNU ld drops the
// kernels and execution providers that nothing references, and only
// OrtGetApiBase is exported, via --dynamic-list (see build.sh:
// ONNXRUNTIME_STATIC_PREFIX). The org
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
// It is a DynamicAdvancedSession: it owns no input/output tensors. Every Run
// allocates fresh input/output tensors and frees them afterwards (see Run), so
// a pooled session's steady-state native memory is just its weights plus ORT's
// plan cache — never the pinned in/out buffers that previously dominated the
// ~14 GB of native memory across the rec/det/DLA/TSR pools.
type session struct {
	inName  string
	outName string
	inShape []int64
	sess    *ort.DynamicAdvancedSession
	// poisoned is set when a Run is cancelled/terminated via context. ONNX
	// Runtime does not guarantee a session is reusable after a forced
	// termination, so the pool must Destroy rather than re-Put it.
	poisoned bool
}

// weightSet is a process-wide cached copy of one model's constant initializers
// (its weights). Every pooled session of the same modelPath injects these
// shared buffers into its SessionOptions so the weight buffers live in memory
// exactly once, instead of each of the ~220 live rec/det/DLA/TSR sessions
// deserializing its own independent copy. The SharedInitializers (and the
// malloc'd buffers they wrap) are owned here and must outlive every session
// that references them; they are never Destroy'd until process exit, which is
// safe because the DeepDoc models are fixed for the life of the process.
type weightSet struct {
	names []string
	vals  []*ort.SharedInitializer
}

var (
	// weightMu guards weightCache. Extraction is cheap but must not race.
	weightMu    sync.Mutex
	weightCache = map[string]*weightSet{}
)

// sharedWeights extracts the constant initializers from modelPath once and
// caches them keyed by model path. Each subsequent call for the same model
// returns the cached buffers, so every pooled session of that model shares a
// single copy of its weights. Extraction uses a throwaway AdvancedSession (the
// only onnxruntime_go type exposing the GetInitializer* API); the session is
// Destroy'd right after extraction, leaving the SharedInitializers (user-owned
// malloc'd copies) alive in the cache. Returns (nil, nil) when the model has no
// initializers worth sharing.
func sharedWeights(modelPath, inName string, inShape []int64, outName string) (*weightSet, error) {
	weightMu.Lock()
	defer weightMu.Unlock()
	if ws, ok := weightCache[modelPath]; ok {
		return ws, nil
	}
	// Throwaway extraction session: the weights live in the graph, not in any
	// input/output tensor, so we only need valid in/out tensors to load the
	// model. We never Run it, so the output tensor's shape is irrelevant.
	inT, err := ort.NewTensor(ort.NewShape(inShape...), make([]float32, prod(inShape)))
	if err != nil {
		return nil, fmt.Errorf("allocate extraction input for %s: %w", modelPath, err)
	}
	defer inT.Destroy()
	outT, err := ort.NewTensor(ort.NewShape(1), []float32{0})
	if err != nil {
		return nil, fmt.Errorf("allocate extraction output for %s: %w", modelPath, err)
	}
	defer outT.Destroy()
	ext, err := ort.NewAdvancedSession(modelPath,
		[]string{inName}, []string{outName},
		[]ort.Value{inT}, []ort.Value{outT}, nil)
	if err != nil {
		return nil, fmt.Errorf("open extraction session for %s: %w", modelPath, err)
	}
	defer ext.Destroy()

	count, err := ext.GetInitializerCount()
	if err != nil {
		return nil, fmt.Errorf("initializer count for %s: %w", modelPath, err)
	}
	if count == 0 {
		return nil, nil
	}
	ws := &weightSet{
		names: make([]string, 0, count),
		vals:  make([]*ort.SharedInitializer, 0, count),
	}
	for i := 0; i < count; i++ {
		name, err := ext.GetInitializerName(i)
		if err != nil {
			return nil, fmt.Errorf("initializer name %d for %s: %w", i, modelPath, err)
		}
		val, err := ext.GetInitializer(name)
		if err != nil {
			return nil, fmt.Errorf("get initializer %q for %s: %w", name, modelPath, err)
		}
		ws.names = append(ws.names, name)
		ws.vals = append(ws.vals, val)
	}
	weightCache[modelPath] = ws
	return ws, nil
}

// NewSession opens modelPath. inShape describes the fixed input tensor
// dimensions; output tensors are allocated per Run (their shape is
// model-determined, so no outShape argument is needed). The session runs
// intraOpThreads intra-op threads (see the constant): one thread per Run, with
// the process-wide ceiling owned by the inference budget the process owner
// registers (see inference_limit.go). Input/output tensors are allocated and
// freed on every Run (see session.Run), so a pooled session holds only its
// weights in steady state. Weight sharing is applied transparently: the model's
// constant initializers are extracted once per modelPath and injected into the
// session options, so every session of the same model shares a single copy of
// the weight buffers. InitORT must have been called first.
func NewSession(modelPath, inName string, inShape []int64, outName string) (*session, error) {
	weights, werr := sharedWeights(modelPath, inName, inShape, outName)
	if werr != nil {
		// Degrade gracefully: a model still loads and runs correctly without
		// sharing; it just deserializes its own weight copy.
		log.Printf("deepdoc/native: weight sharing unavailable for %s: %v",
			modelPath, werr)
		weights = nil
	}
	return newRawSession(modelPath, inName, inShape, outName, weights)
}

// newRawSession opens modelPath with no weight sharing unless weights != nil,
// in which case each shared initializer is injected into the session options
// before the model is loaded. The session is a DynamicAdvancedSession: it owns
// no input/output tensors. Every Run allocates fresh input/output tensors and
// frees them afterwards (see session.Run), so a pooled session's steady-state
// native memory is just its weights plus ORT's plan cache — never the pinned
// in/out buffers. It is the single point where the ORT running session is
// created; sharedWeights funnels its one-shot extraction source through a
// separate AdvancedSession (the only type exposing GetInitializer*).
func newRawSession(modelPath, inName string, inShape []int64, outName string, weights *weightSet) (*session, error) {
	opts, err := newSessionOptions(weights)
	if err != nil {
		return nil, err
	}
	// The C session copies these options (including the shared-initializer
	// references) at creation time, so the options handle can be released once
	// the session is built. The shared weight buffers themselves are owned by
	// the process-wide weightCache and outlive every session, so releasing opts
	// here does not free them.
	defer opts.Destroy()
	sess, err := ort.NewDynamicAdvancedSession(modelPath,
		[]string{inName}, []string{outName}, opts)
	if err != nil {
		return nil, err
	}
	return &session{
		inName: inName, outName: outName,
		inShape: inShape,
		sess:    sess,
	}, nil
}

// newSessionOptions builds the SessionOptions shared by every running session:
// one intra-op thread (so each Run costs exactly one CPU thread — see the
// intraOpThreads constant and inference_limit.go) and the BFC arena disabled
// (idle sessions then keep only their weights; activation tensors are allocated
// per Run and freed after). When weights != nil, each shared initializer is
// injected so the model's constant buffers live in memory a single time across
// every pooled session of the same model.
func newSessionOptions(weights *weightSet) (*ort.SessionOptions, error) {
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	// One intra-op thread per session: the session's Runs then cost one thread
	// each, so the process-wide inference ceiling is exactly the number of
	// concurrent Runs the caller admits (see the intraOpThreads constant).
	if err := opts.SetIntraOpNumThreads(intraOpThreads); err != nil {
		opts.Destroy()
		return nil, err
	}
	// Disable the BFC memory arena for this session. The arena pre-reserves a
	// native block per session and never shrinks it, so every pooled (often
	// idle) session hoards one. Across the rec/det/DLA/TSR pools (~220 live
	// sessions while parsing a large PDF) this dominated the ~14 GB of native
	// memory seen at the ~20 GB OOM peak. With the arena off, idle sessions keep
	// only their weights; activation tensors are allocated directly and freed
	// after each Run.
	if err := opts.SetCpuMemArena(false); err != nil {
		opts.Destroy()
		return nil, err
	}
	if weights != nil {
		for i, v := range weights.vals {
			if err := opts.AddInitializer(weights.names[i], v); err != nil {
				opts.Destroy()
				return nil, fmt.Errorf("inject shared initializer %q: %w",
					weights.names[i], err)
			}
		}
	}
	return opts, nil
}

// checkOutputLength fails fast when a model emits an unexpectedly sized output
// tensor. The previous pinned-output design got this check for free: ORT errored
// when the bound output tensor's shape mismatched the model. The dynamic design
// allocates the output per Run (nil output, ORT allocates), so ORT no longer
// validates the shape; without this guard, postprocessing (dlaPostprocess
// indexes 300*6, tsrPostprocess indexes 11*8400, RunDet fills rh*rw) panics or
// silently misreads a truncated output instead of returning a clean error.
func checkOutputLength(model string, got, want int) error {
	if got != want {
		return fmt.Errorf("%s model output length %d, expected %d", model, got, want)
	}
	return nil
}

// Run allocates a fresh input tensor, executes with an auto-allocated (dynamic)
// output, and returns the output data. Both tensors are destroyed before
// returning; out is a fresh copy the caller owns. ctx bounds the inference: if
// it is cancelled while Run is in flight, the underlying ONNX Runtime call is
// terminated via RunOptions. A terminated session is left in an indeterminate
// state, so it is marked poisoned and the pool destroys it instead of reusing
// it.
func (s *session) Run(ctx context.Context, input []float32) ([]float32, error) {
	if len(input) != int(prod(s.inShape)) {
		return nil, fmt.Errorf("session %s: input len %d != expected %d",
			s.outName, len(input), int(prod(s.inShape)))
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

	// Allocate a fresh input tensor for this Run and free it right after the
	// call. The output is passed as nil so ORT allocates it; we copy the data
	// out and free the returned Value. This keeps each pooled session's
	// steady-state native memory to just its weights (plus ORT's plan cache)
	// instead of pinning fixed in/out buffers that previously dominated the
	// ~14 GB of native memory seen across the rec/det/DLA/TSR pools.
	inT, err := ort.NewTensor(ort.NewShape(s.inShape...), input)
	if err != nil {
		return nil, err
	}
	defer inT.Destroy()
	outputs := []ort.Value{nil}
	if err := s.sess.RunWithOptions([]ort.Value{inT}, outputs, opts); err != nil {
		if ctx.Err() != nil {
			s.poisoned = true
		}
		return nil, err
	}
	outVal := outputs[0]
	if outVal == nil {
		return nil, fmt.Errorf("session %s: nil output tensor", s.outName)
	}
	defer outVal.Destroy()
	outT, ok := outVal.(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("session %s: unexpected output value type %T",
			s.outName, outVal)
	}
	out := make([]float32, len(outT.GetData()))
	copy(out, outT.GetData())
	return out, nil
}

// Destroy releases the dynamic advanced-session handle. Input/output tensors
// are no longer owned by the session (they are allocated per Run and freed
// there), so there is nothing else to release here. It does NOT touch the
// process-global environment.
func (s *session) Destroy() {
	if s.sess != nil {
		s.sess.Destroy()
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
