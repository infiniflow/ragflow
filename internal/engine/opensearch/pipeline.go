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

package opensearch

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// pipelineManager holds validated OpenSearch 2.19.x version metadata.
type pipelineManager struct {
	version string
	major   int
	minor   int
	patch   int
}

// Version returns the raw version string.
func (p *pipelineManager) Version() string {
	if p == nil {
		return ""
	}
	return p.version
}

// Major returns the major version.
func (p *pipelineManager) Major() int {
	if p == nil {
		return 0
	}
	return p.major
}

// Minor returns the minor version.
func (p *pipelineManager) Minor() int {
	if p == nil {
		return 0
	}
	return p.minor
}

// Patch returns the patch version.
func (p *pipelineManager) Patch() int {
	if p == nil {
		return 0
	}
	return p.patch
}

// parseSemVer parses a semantic version string into major, minor, patch.
// Any build metadata, prerelease tags (e.g. -SNAPSHOT, +build), or leading "v"/"V" prefixes are stripped.
func parseSemVer(v string) (int, int, int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, 0, 0, fmt.Errorf("empty version string")
	}
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return 0, 0, 0, fmt.Errorf("invalid semver: %s", v)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid semver major %q: %w", parts[0], err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid semver minor %q: %w", parts[1], err)
	}
	patch := 0
	if len(parts) >= 3 {
		if p, err := strconv.Atoi(parts[2]); err == nil {
			patch = p
		}
	}
	return major, minor, patch, nil
}

// NormalizeWeights ensures a 2-element slice summing to 1.0.
// It applies four-decimal rounding and residual absorption to guarantee
// that the normalized weights sum exactly to 1.0, preventing OpenSearch
// normalization-processor validation errors (e.g. "weights must sum to 1.0").
// If weights is invalid, NaN, Inf, non-positive, or incorrect length, defaults to [0.5, 0.5].
func NormalizeWeights(weights []float64) []float64 {
	if len(weights) != 2 {
		return []float64{0.5, 0.5}
	}
	w0, w1 := weights[0], weights[1]
	if math.IsNaN(w0) || math.IsInf(w0, 0) || math.IsNaN(w1) || math.IsInf(w1, 0) {
		return []float64{0.5, 0.5}
	}
	if w0 < 0 {
		w0 = 0
	}
	if w1 < 0 {
		w1 = 0
	}
	sum := w0 + w1
	if math.IsNaN(sum) || math.IsInf(sum, 0) || sum <= 0 {
		return []float64{0.5, 0.5}
	}
	roundedW0 := math.Round((w0/sum)*10000) / 10000
	if roundedW0 <= 0 {
		roundedW0 = 0.0001
	} else if roundedW0 >= 1.0 {
		roundedW0 = 0.9999
	}
	roundedW1 := math.Round((1.0-roundedW0)*10000) / 10000
	return []float64{roundedW0, roundedW1}
}

// BuildInlinePipeline constructs an inline search_pipeline block for OpenSearch 2.19.x.
func BuildInlinePipeline(weights []float64) map[string]interface{} {
	normWeights := NormalizeWeights(weights)
	return map[string]interface{}{
		"phase_results_processors": []interface{}{
			map[string]interface{}{
				"normalization-processor": map[string]interface{}{
					"normalization": map[string]interface{}{
						"technique": "min_max",
					},
					"combination": map[string]interface{}{
						"technique": "arithmetic_mean",
						"parameters": map[string]interface{}{
							"weights": normWeights,
						},
					},
				},
			},
		},
	}
}

// BuildInlinePipeline constructs an inline search_pipeline block on the pipelineManager.
func (p *pipelineManager) BuildInlinePipeline(weights []float64) map[string]interface{} {
	return BuildInlinePipeline(weights)
}

// newPipelineManager creates and validates a pipelineManager for OpenSearch 2.19.x.
// It strictly accepts OpenSearch 2.19.x. If the cluster reports 7.10.2 (compatibility mode),
// unparseable, or non-2.19.x versions, it returns a descriptive error immediately.
func newPipelineManager(version string) (*pipelineManager, error) {
	if strings.HasPrefix(version, "7.") {
		return nil, fmt.Errorf("unsupported OpenSearch version %q: 7.x compatibility mode is not supported; OpenSearch must report its actual 2.19.x version (please disable compatibility.override_main_response_version)", version)
	}
	major, minor, patch, err := parseSemVer(version)
	if err != nil {
		return nil, fmt.Errorf("unsupported OpenSearch version %q: %w", version, err)
	}
	if major != 2 || minor != 19 {
		return nil, fmt.Errorf("unsupported OpenSearch version %q: only OpenSearch 2.19.x is supported", version)
	}
	return &pipelineManager{
		version: version,
		major:   major,
		minor:   minor,
		patch:   patch,
	}, nil
}
