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
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/deepdoc/native"
	pdf "ragflow/internal/deepdoc/parser/pdf"
	infnative "ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	"ragflow/internal/server"
	"ragflow/internal/server/bootstrap"
	"ragflow/internal/server/ingestor"
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

	if err := infnative.Register(modelDir, dropScore); err != nil {
		common.Warn("in-process DeepDoc backend unavailable",
			zap.String("reason", err.Error()))
	}

	// The in-process (Go) DeepDoc backend is the ONLY production backend. Fail
	// fast rather than silently parsing without layout/table/OCR if the local
	// backend cannot serve (ORT + models must be present when built with -tags
	// cgo).
	if !infnative.Serving() {
		common.Fatal("no in-process DeepDoc backend serving: provide the local ORT "+
			"runtime + models and build with -tags cgo",
			zap.String("model_dir", modelDir),
			zap.String("ort_lib", "static (libonnxruntime.a via dlopen(NULL))"))
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
		common.Fatal("invalid deepdoc inference concurrency", zap.Error(errK))
	}
	rawN, explicitN, errN := cfg.GetDeepDocInferenceCPUCores(arguments.DeepdocInferenceCPUCores)
	if errN != nil {
		common.Fatal("invalid deepdoc inference cpu cores", zap.Error(errN))
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
		common.Fatal("invalid deepdoc inference configuration", zap.Error(errV))
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
	pageConcurrency, err := ingestor.ResolveIngestorPageConcurrency(arguments, server.GetConfig().GetIngestorConfig().PageConcurrency)
	if err != nil {
		return err
	}
	pdf.SetPageConcurrency(pageConcurrency)
	common.Info("in-process DeepDoc page worker pool registered",
		zap.Int("page_concurrency", pageConcurrency))
	return nil
}

// resolveDeepDocModelDir picks the model directory: the explicit DEEPDOC_MODEL_DIR
// env, else the RAGFlow default model dir (internal/rag/res/deepdoc),
// else the assets fetched by deps/download_deps.py. The first
// candidate that actually contains the required weights wins.
func resolveDeepDocModelDir() string {
	if v := strings.TrimSpace(common.GetEnv(common.EnvDeepDocModelDir)); v != "" {
		return v
	}
	wd, _ := os.Getwd()
	// MODEL_ASSETS_DIR first: the shared model-asset root keeps this layout too, so one
	// variable can point at the DeepDoc weights as well as the embedding tokenizers.
	candidates := append([]string(nil), common.ModelAssetCandidates("huggingface.co/InfiniFlow/deepdoc")...)
	candidates = append(candidates,
		filepath.Join(wd, "internal", "rag", "res", "deepdoc"),
		filepath.Join(wd, "huggingface.co", "InfiniFlow", "deepdoc"),
	)
	for _, c := range candidates {
		if dirHasModels(c) {
			return c
		}
	}
	// None verified; return the canonical default so any error message points
	// at the conventional location.
	return filepath.Join(wd, "internal", "rag", "res", "deepdoc")
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

// dirHasModels reports whether dir contains every required model files.
func dirHasModels(dir string) bool {
	return common.HasModelFiles(dir)
}
