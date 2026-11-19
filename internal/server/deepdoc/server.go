//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Package deepdoc wires and runs the RAGFlow in-process (Go) DeepDoc backend and
// the DeepDoc server dispatch. It was extracted from package main so that main
// stays a thin dispatch layer.
package deepdoc

import (
	"fmt"
	"os"
	native "ragflow/internal/deepdoc/native"
	goruntime "runtime"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
	pdf "ragflow/internal/deepdoc/parser/pdf"
	infnative "ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	"ragflow/internal/server"
	"ragflow/internal/server/bootstrap"
)

// inferenceTotalCores returns the CPU budget the DeepDoc inference config is
// validated against. It uses runtime.GOMAXPROCS(0) instead of runtime.NumCPU()
// so the budget reflects the cgroup CPU quota inside containers: Go 1.25+ derives
// GOMAXPROCS from the quota (CGroups v2 cpu.max / v1 cpu.cfs_quota_us), whereas
// runtime.NumCPU() reports the host's affinity-mask core count and ignores a
// container's CPU limit. Without this, a 2-CPU-limit pod on a 64-core host would
// "resolve" inference_cpu_cores: 0 to 64 and let inference_concurrency: 16 pass
// validation, oversubscribing the box the fail-fast guard in
// native.ValidateInferenceConfig exists to prevent.
func inferenceTotalCores() int {
	n := goruntime.GOMAXPROCS(0)
	if n < 1 {
		return 1
	}
	return n
}

// resolveDeviceSpec picks the DeepDoc device spec from CLI and env, trimming
// whitespace and defaulting to "cpu". A non-blank CLI value wins over env; a
// blank CLI value falls back to env; both blank resolve to "cpu".
func resolveDeviceSpec(cli, env string) string {
	if spec := strings.TrimSpace(cli); spec != "" {
		return spec
	}
	if spec := strings.TrimSpace(env); spec != "" {
		return spec
	}
	return "cpu"
}

// RegisterNativeDeepDoc wires the in-process (Go) DeepDoc backend as the local
// inference backend. The server is built with -tags cgo and links ONNX Runtime
// statically (libonnxruntime.a, resolved at runtime via dlopen(NULL) from the
// running binary — see github.com/infiniflow/onnxruntime_go, the org mirror of
// yalue/onnxruntime_go), so there is no external
// DeepDoc HTTP service and no dynamic .so deployment.
//
// Fail-fast contract (P0): the in-process backend must be available at startup
// (ORT + models present). There is NO silent degradation to an empty analyzer:
// if the backend is not serving, the server aborts.
func RegisterNativeDeepDoc(arguments *bootstrap.ServerArgs) error {
	modelDir := resolveDeepDocModelDir()
	dropScore := resolveDeepDocDropScore()

	// Resolve the device policy. The default build accepts only cpu; a gpu
	// request fails fast here via native.ParseDevice. CLI overrides env. No
	// silent fallback to CPU.
	cliDevice := ""
	if arguments.DeepdocDevice != nil {
		cliDevice = *arguments.DeepdocDevice
	}
	deviceSpec := resolveDeviceSpec(cliDevice, os.Getenv(common.EnvDeepDocDevice))
	dev, err := native.ParseDevice(deviceSpec)
	if err != nil {
		return fmt.Errorf("invalid DeepDoc device: %w", err)
	}
	native.SetDevice(dev)

	if err := infnative.Register(modelDir, dropScore); err != nil {
		common.Warn("in-process DeepDoc backend unavailable",
			zap.String("reason", err.Error()))
	}

	// The in-process (Go) DeepDoc backend is the ONLY production backend. Fail
	// fast rather than silently parsing without layout/table/OCR if the local
	// backend cannot serve (ORT + models must be present when built with -tags
	// cgo).
	if !infnative.Serving() {
		return fmt.Errorf("no in-process DeepDoc backend serving: provide the local ORT runtime and models in %s, and build with -tags cgo", modelDir)
	}
	common.Info("in-process DeepDoc backend registered (production backend)",
		zap.String("model_dir", modelDir))

	// Resolve the DeepDoc in-process inference configuration: the concurrency
	// budget K (max in-flight Runs) and the CPU-core budget N (max cores
	// inference may occupy). Both resolve from CLI > env > config > default. K
	// defaults to 1; N defaults to "unset", which is derived from K so each Run
	// stays single-threaded by default (the prior behaviour). An explicit N = 0
	// means "all cores". All values are fail-fast validated against the process's
	// cgroup-aware CPU budget at startup (see native.ValidateInferenceConfig and
	// inferenceTotalCores), so a misconfiguration aborts rather than silently
	// oversubscribing the box.
	cfg := server.GetConfig()
	K, _, errK := cfg.GetDeepDocInferenceConcurrency(arguments.DeepdocInferenceConcurrency)
	if errK != nil {
		return fmt.Errorf("invalid deepdoc inference concurrency: %w", errK)
	}
	rawN, explicitN, errN := cfg.GetDeepDocInferenceCPUCores(arguments.DeepdocInferenceCPUCores)
	if errN != nil {
		return fmt.Errorf("invalid deepdoc inference cpu cores: %w", errN)
	}
	// When N is unset, derive it from K so each Run opens with a single intra-op
	// thread (coresPerInference = max(1, K/K) = 1), preserving the prior
	// single-core-per-run semantics. An explicit N (including 0 = all cores) is
	// honoured as-is below.
	resolvedN := rawN
	if !explicitN {
		resolvedN = K
	}
	totalCores := inferenceTotalCores()
	// Validate K and N against the process's CPU budget (cgroup-aware) and
	// derive the per-Run intra-op thread count (max(1, N/K)). N == 0 resolves to
	// all cores here.
	coresPerInference, totalCPUCores, errV := native.ValidateInferenceConfig(totalCores, resolvedN, K)
	if errV != nil {
		return fmt.Errorf("invalid deepdoc inference configuration: %w", errV)
	}
	// Register the per-session intra-op thread count before any model session is
	// created (the native package reads it on every SessionOptions build).
	native.SetIntraOpThreads(coresPerInference)
	pdf.SetDeepDocConcurrency(K)
	native.SetInferenceLimit(K)
	common.Info("in-process DeepDoc inference limit registered",
		zap.Int("max_concurrent_inference", K),
		zap.Int("cpu_cores_per_inference", coresPerInference),
		zap.Int("total_cpu_cores_budget", totalCPUCores),
		zap.Int("gomaxprocs", goruntime.GOMAXPROCS(0)))

	// Resolve the per-document page concurrency (N) from CLI > env > config >
	// default(2) and register it with the shared page worker pool. Fail fast on
	// an out-of-range value.
	pageConcurrency, err := server.GetConfig().GetIngestorPageConcurrency()
	if err != nil {
		return err
	}
	pdf.SetPageConcurrency(pageConcurrency)
	common.Info("in-process DeepDoc page worker pool registered",
		zap.Int("page_concurrency", pageConcurrency))
	return nil
}

// resolveDeepDocModelDir delegates to common.DeepDocModelDir, the single
// source of truth shared with runtime probes (e.g. /system/doc-analyzers).
func resolveDeepDocModelDir() string {
	return common.DeepDocModelDir()
}

// resolveDeepDocDropScore returns the explicit DEEPDOC_DROP_SCORE env, else the
// in-process backend's default (infnative.DefaultDropScore, which mirrors
// the Python inference service's Recognizer.drop_score).
func resolveDeepDocDropScore() float64 {
	if v := strings.TrimSpace(common.GetEnv(common.EnvDeepDocDropScore)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		common.Warn("invalid DEEPDOC_DROP_SCORE, using default",
			zap.String("value", v), zap.Float64("default", infnative.DefaultDropScore))
	}
	return infnative.DefaultDropScore
}
