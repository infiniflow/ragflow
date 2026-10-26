//go:build cgo

// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package decisionhead

// session.go runs the decision-head ONNX graph in-process. It owns three things the
// rest of the package must not reimplement: the process-global ONNX Runtime
// environment, the single-flight gate (see `single`), and the tensor plumbing
// between a Sequence and the model's fixed I/O contract
// (DEPLOY_CONTRACT.md §四).
//
// The graph is 5-in/2-out: input_ids, attention_mask, marker_pos, marker_mask
// and qtype in; logits and act_probs out. Only `logits` is trained and read.
// `act_probs` is requested because the graph emits it, and dropped on the floor
// because its head was never supervised (§五) — surfacing it would invite a
// caller to route on random numbers.
//
// The package imports onnxruntime_go (cgo), so this file is gated the same way
// as the DeepDoc native backend; every build and test path in build.sh passes
// the `cgo` tag, so the tag only excludes builds that could not link ORT at all.

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	ort "github.com/infiniflow/onnxruntime_go"
)

// The model's input and output names, fixed by the exported graph.
var (
	sessionInputs  = []string{"input_ids", "attention_mask", "marker_pos", "marker_mask", "qtype"}
	sessionOutputs = []string{"logits", "act_probs"}
)

// single is the process-wide single-flight gate: at most one forward pass is in
// flight at any moment, across every Session this process opens. Measured on
// the deployment host, a batch buys nothing (per-row latency is flat while RSS
// grows linearly and B=2 @2680 tokens already OOMs), so serialising the calls
// is not a throttling compromise — it is the only shape the model has that fits
// the host, and it makes the L² activation footprint a single instance rather
// than one per concurrent caller.
var single = make(chan struct{}, 1)

// ortOnce guards the one-time environment init. The DeepDoc in-process backend
// owns the same ORT environment when it is linked into the process, so an
// already-initialized environment is success here, not an error: ORT refuses a
// second InitializeEnvironment outright.
var (
	ortOnce sync.Once
	ortErr  error
)

func initORT() error {
	ortOnce.Do(func() {
		if ort.IsInitialized() {
			return
		}
		ortErr = ort.InitializeEnvironment()
	})
	return ortErr
}

// Session is one loaded decision-head checkpoint. It holds only the ORT session handle;
// every Run allocates its tensors and frees them, so an idle Session's native
// memory is its weights plus ORT's plan cache, never a pinned L/J buffer.
//
// A Session is not safe to use after a Run returns an error that came from a
// context cancellation: a force-terminated ORT session is left in an
// indeterminate state. Poisoned reports that so the owner can reopen instead of
// driving a dead handle.
type Session struct {
	sess     *ort.DynamicAdvancedSession
	poisoned bool
}

// OpenSession loads modelPath (the laya.onnx of a model directory) and prepares
// it to serve. It runs one intra-op thread per core: on this class of host the
// 16-thread default measured 2.7-3x faster than single-thread with byte-identical
// logits. That is the opposite of the DeepDoc policy (one thread), because those
// models are called with many sessions in flight while the head is single-flight.
func OpenSession(modelPath string) (*Session, error) {
	if err := initORT(); err != nil {
		return nil, fmt.Errorf("decisionhead: onnxruntime init: %w", err)
	}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("decisionhead: session options: %w", err)
	}
	defer opts.Destroy()
	if err := opts.SetIntraOpNumThreads(runtime.NumCPU()); err != nil {
		return nil, fmt.Errorf("decisionhead: intra-op threads: %w", err)
	}
	sess, err := ort.NewDynamicAdvancedSession(modelPath, sessionInputs, sessionOutputs, opts)
	if err != nil {
		return nil, fmt.Errorf("decisionhead: opening %s: %w", modelPath, err)
	}
	return &Session{sess: sess}, nil
}

// Poisoned reports whether a cancelled Run left the session unusable.
func (s *Session) Poisoned() bool { return s.poisoned }

// Close releases the ORT session handle. The process-global environment is left
// alone: it is shared and outlives any single checkpoint.
func (s *Session) Close() {
	if s.sess != nil {
		s.sess.Destroy()
		s.sess = nil
	}
}

// Logits runs one decision and returns the model's per-option scores, one per
// marker in seq.Markers. The caller owns the returned slice.
//
// ctx bounds the call: an in-flight cancellation terminates the ORT run and
// poisons the session (see Poisoned). ctx is also respected while waiting for
// the single-flight gate, so a queue of callers cannot pin goroutines behind a
// slow forward pass.
func (s *Session) Logits(ctx context.Context, seq Sequence) ([]float32, error) {
	if len(seq.IDs) == 0 {
		return nil, fmt.Errorf("decisionhead: empty sequence")
	}
	if len(seq.Markers) == 0 {
		return nil, fmt.Errorf("decisionhead: sequence carries no markers")
	}
	select {
	case single <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-single }()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.run(ctx, seq)
}

// run builds the five input tensors from seq, executes, and copies out the
// `logits` row. The `act_probs` output is a declared output of the graph, so it
// is requested and then released unread (§五).
func (s *Session) run(ctx context.Context, seq Sequence) ([]float32, error) {
	width := len(seq.IDs)
	rowShape := ort.NewShape(1, int64(width))

	ids := make([]int64, width)
	attention := make([]int64, width)
	for i, id := range seq.IDs {
		ids[i] = int64(id)
		attention[i] = 1
	}
	inIDs, err := ort.NewTensor(rowShape, ids)
	if err != nil {
		return nil, fmt.Errorf("decisionhead: input_ids tensor: %w", err)
	}
	defer inIDs.Destroy()
	inAttention, err := ort.NewTensor(rowShape, attention)
	if err != nil {
		return nil, fmt.Errorf("decisionhead: attention_mask tensor: %w", err)
	}
	defer inAttention.Destroy()

	optionShape := ort.NewShape(1, int64(len(seq.Markers)))
	positions := make([]int64, len(seq.Markers))
	valid := make([]bool, len(seq.Markers))
	for i, marker := range seq.Markers {
		positions[i] = int64(marker)
		valid[i] = true
	}
	inPos, err := ort.NewTensor(optionShape, positions)
	if err != nil {
		return nil, fmt.Errorf("decisionhead: marker_pos tensor: %w", err)
	}
	defer inPos.Destroy()
	inMarkerMask, err := ort.NewTensor(optionShape, valid)
	if err != nil {
		return nil, fmt.Errorf("decisionhead: marker_mask tensor: %w", err)
	}
	defer inMarkerMask.Destroy()
	inQType, err := ort.NewTensor(ort.NewShape(1), []int64{seq.QType})
	if err != nil {
		return nil, fmt.Errorf("decisionhead: qtype tensor: %w", err)
	}
	defer inQType.Destroy()

	runOpts, err := ort.NewRunOptions()
	if err != nil {
		return nil, fmt.Errorf("decisionhead: run options: %w", err)
	}
	defer runOpts.Destroy()
	// Terminate the ORT run when ctx is done; done closes once run returns so
	// the watcher exits on the success path too. Join the watcher before
	// runOpts.Destroy() executes (defers run LIFO) so Terminate() can never
	// touch a freed handle.
	done := make(chan struct{})
	var watcher sync.WaitGroup
	watcher.Add(1)
	go func() {
		defer watcher.Done()
		select {
		case <-ctx.Done():
			_ = runOpts.Terminate()
		case <-done:
		}
	}()
	defer func() {
		close(done)
		watcher.Wait()
	}()

	outputs := []ort.Value{nil, nil}
	inputs := []ort.Value{inIDs, inAttention, inPos, inMarkerMask, inQType}
	if err := s.sess.RunWithOptions(inputs, outputs, runOpts); err != nil {
		if ctx.Err() != nil {
			s.poisoned = true
		}
		return nil, fmt.Errorf("decisionhead: run: %w", err)
	}
	// Release both outputs; act_probs is never read.
	defer func() {
		for _, out := range outputs {
			if out != nil {
				out.Destroy()
			}
		}
	}()

	logitsVal := outputs[0]
	if logitsVal == nil {
		return nil, fmt.Errorf("decisionhead: nil logits output")
	}
	logitsT, ok := logitsVal.(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("decisionhead: unexpected logits type %T", logitsVal)
	}
	out := make([]float32, len(logitsT.GetData()))
	copy(out, logitsT.GetData())
	return out, nil
}
