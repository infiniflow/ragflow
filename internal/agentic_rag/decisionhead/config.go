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

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
)

// Defaults for the environment knobs. The segment budget is the load-bearing
// one: measured on the deployment host, one forward over 512 tokens costs
// ~0.41 s and ~2.1 GB peak RSS, while the same chunk as a single 3000-token
// forward costs 4.13 s and 3.6 GB — the global-attention layers materialise
// [heads, L, L] matrices, so both time and memory grow super-linearly in L and
// the curve only gets worse (8000 tokens OOMs the host). Splitting long chunks
// into segments is therefore not an optimisation, it is what keeps the model
// inside the host's envelope.
const (
	DefaultSegmentTokens  = 512
	DefaultMaxSegments    = 12
	DefaultMinProbability = 0.6
	DefaultBudgetMS       = 20000
)

// Config is the runtime configuration, all environment-driven. The zero value
// is "disabled": an empty Dir means the model is not deployed and every caller
// must degrade to unfiltered behaviour.
type Config struct {
	// Dir is the model directory holding laya_config.json and
	// tokenizer/tokenizer.json (the exported-head layout), plus laya.ort when
	// the graph runs in process.
	//
	// The in-process checkpoint is the ORT format, not the .onnx the training
	// side exports: the ONNX Runtime linked into this binary is an ORT-format
	// minimal build, so it rejects every .onnx file outright and loads only
	// .ort. Convert with
	// `python -m onnxruntime.tools.convert_onnx_models_to_ort`.
	Dir string
	// URL is the decision-head inference sidecar, when the graph runs out of process.
	// Set RAGFLOW_DECISION_URL and the .ort is not needed: the assembled ids are
	// posted to the sidecar and only the logits come back. That is the only
	// configuration that works with the ONNX Runtime linked into this binary
	// today — the minimal build is missing operators the head graph needs
	// (Shape, Range, TopK, ...), so the in-process path cannot start.
	URL string
	// SegmentTokens is the per-segment state budget. A chunk longer than this
	// is split before judging (see SplitSegments).
	SegmentTokens int
	// MaxSegments bounds how many segments of one chunk are judged; text past
	// the bound is kept unjudged rather than paid for.
	MaxSegments int
	// MinProbability is the calibrated-probability threshold at or above which a
	// segment counts as useful.
	MinProbability float64
	// Budget caps the total judging time of one chunk; when it runs out the
	// remaining segments are kept unjudged (never dropped).
	Budget time.Duration
	// QuestionStyle selects what the head is asked. StyleSufficiency is the
	// contract's trained phrasing and the default; StyleRelevance asks whether
	// the evidence is relevant instead (see relevanceTemplates), which is out of
	// the checkpoint's training distribution and exists for measurement.
	QuestionStyle string
}

// The two phrasings the filter can ask the head. Only StyleSufficiency is in
// the checkpoint's training distribution.
const (
	StyleSufficiency = "sufficiency"
	StyleRelevance   = "relevance"
)

// AsksRelevance reports whether the filter asks the relevance phrasing.
func (c Config) AsksRelevance() bool { return c.QuestionStyle == StyleRelevance }

// ConfigFromEnv reads the configuration from the environment. A missing
// RAGFLOW_DECISION_DIR yields a disabled Config.
func ConfigFromEnv() Config {
	return Config{
		Dir:            strings.TrimSpace(common.GetEnv(common.EnvDecisionDir)),
		URL:            strings.TrimRight(strings.TrimSpace(common.GetEnv(common.EnvDecisionHeadURL)), "/"),
		SegmentTokens:  envInt(common.EnvDecisionSegmentTokens, DefaultSegmentTokens),
		MaxSegments:    envInt(common.EnvDecisionMaxSegments, DefaultMaxSegments),
		MinProbability: envFloat(common.EnvDecisionMinProbability, DefaultMinProbability),
		Budget:         time.Duration(envInt(common.EnvDecisionBudgetMS, DefaultBudgetMS)) * time.Millisecond,
		QuestionStyle:  envStyle(common.EnvDecisionQuestionStyle),
	}
}

// Enabled reports whether the model directory is configured.
func (c Config) Enabled() bool { return c.Dir != "" }

// Available reports whether the model directory holds the artifacts this
// deployment needs. The contract and the tokenizer are always needed — they are
// what the assembly and the threshold are built from — while the checkpoint
// itself is only needed when the graph runs in process.
//
// It is the caller's cheap pre-flight: a deployment that has the env var but not
// the files logs the gap once and runs unfiltered.
func (c Config) Available() bool {
	if !c.Enabled() {
		return false
	}
	required := []string{
		filepath.Join("onnx", "laya_config.json"),
		filepath.Join("onnx", "tokenizer", "tokenizer.json"),
	}
	if c.URL == "" {
		required = append(required, filepath.Join("onnx", "laya.ort"))
	}
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(c.Dir, rel)); err != nil {
			return false
		}
	}
	return true
}

// Paths of the three artifacts under Dir.
func (c Config) Paths() (modelPath, contractPath, tokenizerPath string) {
	base := c.Dir
	return filepath.Join(base, "onnx", "laya.ort"),
		filepath.Join(base, "onnx", "laya_config.json"),
		filepath.Join(base, "onnx", "tokenizer", "tokenizer.json")
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(common.GetEnv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

// envStyle reads the question style, falling back to the trained phrasing for
// anything unrecognised: an unknown value must not silently switch the filter
// onto an out-of-distribution question.
func envStyle(key string) string {
	switch strings.ToLower(strings.TrimSpace(common.GetEnv(key))) {
	case StyleRelevance:
		return StyleRelevance
	default:
		return StyleSufficiency
	}
}

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(common.GetEnv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 || v >= 1 {
		return fallback
	}
	return v
}

// String renders the config for the startup log line.
func (c Config) String() string {
	if !c.Enabled() {
		return "disabled"
	}
	return fmt.Sprintf("dir=%s url=%q segment_tokens=%d max_segments=%d min_probability=%.2f budget=%s question_style=%s",
		c.Dir, c.URL, c.SegmentTokens, c.MaxSegments, c.MinProbability, c.Budget, c.QuestionStyle)
}
