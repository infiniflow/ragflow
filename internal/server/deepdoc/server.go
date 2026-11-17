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
	goruntime "runtime"
	"strings"

	doctype "ragflow/internal/deepdoc/parser/type"
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

// deriveLocalBudget derives the in-process Run budget from the single
// user-facing K and the machine's core budget. It is min(K, totalCores) so
// local inference never oversubscribes, further capped by an explicit
// inference_cpu_cores when set. K itself (the remote/outer gate) is never
// reduced by this — only the local gate uses the result.
func deriveLocalBudget(k, totalCores, rawN int, explicitN bool) int {
	cap := doctype.LocalInferenceCapacity(k, totalCores)
	if explicitN && rawN > 0 && rawN < cap {
		cap = rawN
	}
	return cap
}
