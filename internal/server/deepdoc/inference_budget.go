package deepdoc

import (
	native "ragflow/internal/deepdoc/native"
	pdf "ragflow/internal/deepdoc/parser/pdf"
	doctype "ragflow/internal/deepdoc/parser/type"
)

// inferenceBudget is the resolved DeepDoc inference budget shared by both gates:
// an outer gate bounding every inference call (K) and an inner gate bounding
// only in-process ONNX Runs (localCap). It is a value object so the resolution
// can be unit-tested without any global registration side effect.
type inferenceBudget struct {
	K                 int // outer gate capacity: max in-flight inference calls
	localCap          int // inner/local gate capacity: max in-process ONNX Runs
	resolvedN         int // CPU-core budget N actually passed to the validator
	coresPerInference int // intra-op threads per Run
	totalCPUCores     int // resolved total CPU cores (for logging)
}

// resolveInferenceBudget derives and validates both gate capacities from the
// user-facing concurrency K, the optional CPU-core budget N, and the machine's
// cgroup-aware core budget. It is a pure function: no global state is written,
// so every backend wiring path can reuse the exact same resolution semantics.
//
// The critical contract: an EXPLICIT N is honoured verbatim — including N=0,
// which means "all cores" to ValidateInferenceConfig. Only an UNSET N is
// derived from K. Replacing an explicit zero with localCap would collapse
// coresPerInference from totalCores/K to 1, gutting local throughput.
func resolveInferenceBudget(k, rawN int, explicitN bool, totalCores int) (inferenceBudget, error) {
	localCap := deriveLocalBudget(k, totalCores, rawN, explicitN)
	resolvedN := rawN
	if !explicitN {
		resolvedN = localCap
	}
	coresPerInference, totalCPUCores, err := native.ValidateInferenceConfig(totalCores, resolvedN, localCap)
	if err != nil {
		return inferenceBudget{}, err
	}
	return inferenceBudget{
		K:                 k,
		localCap:          localCap,
		resolvedN:         resolvedN,
		coresPerInference: coresPerInference,
		totalCPUCores:     totalCPUCores,
	}, nil
}

// RegisterInferenceBudget resolves the two-gate inference budget and registers
// it with both the shared doctype gates and the native thread policy. It is the
// reusable wiring seam: any DeepDoc backend — in-process sessions or a remote
// inference client — calls this once at startup to bound its concurrency. It
// deliberately does NOT depend on the in-process/native backend being available,
// so a deployment that only uses remote inference can still register the budget.
func RegisterInferenceBudget(k, rawN int, explicitN bool, totalCores int) (inferenceBudget, error) {
	budget, err := resolveInferenceBudget(k, rawN, explicitN, totalCores)
	if err != nil {
		return inferenceBudget{}, err
	}
	native.SetIntraOpThreads(budget.coresPerInference)
	pdf.SetDeepDocConcurrency(k)
	doctype.SetInferenceLimit(k)
	doctype.SetLocalInferenceLimit(budget.localCap)
	return budget, nil
}
