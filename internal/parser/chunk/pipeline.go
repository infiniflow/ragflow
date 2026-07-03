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

// Direct chunk-pipeline API. The package exposes a typed runtime path
// for production callers and a DSL-input path for CLI-driven
// inspection/testing. Both run the same operator model:
//
//	preprocess -> split -> postprocess
//
// and report the same ChunkContext shape.
//
// Two entry points are exposed:
//
//   - Run(text, opts) — typed config. Use this for production callers
//     that know their options at compile time (TokenChunker's
//     token-size fallback, PipelineChunker's per-parser dispatch).
//
//   - RunDSL(dsl, text) — accepts a JSON DSL file at run time. This is
//     used by the CLI chunk inspection command.

package chunk

import (
	"fmt"
)

// PipelineOptions is the typed configuration for production callers.
// Each field toggles a behaviour on
// the matching operator; an unset field means "use the operator's
// zero-value default" (no-op for preprocess, default strategy for
// split, no merge / filter for postprocess).
//
// The struct is deliberately flat because current production callers
// only need a small subset of operator settings.
type PipelineOptions struct {
	// Preprocess flags. Any combination of the four is honoured; all
	// false means "no preprocess stage" (the engine skips the stage
	// rather than running an identity preprocess).
	NormalizeNewlines    bool
	StripWhitespace      bool
	RemoveEmptyLines     bool
	SoftLineBreakMerging bool

	// Split configuration. Required: callers that don't want any
	// splitting should still pick a strategy (e.g. "paragraph") and
	// override downstream with a postprocess filter — an unset
	// strategy degrades to "sentence" inside the operator.
	SplitStrategy   string
	SplitBoundaries []string
	KeepSeparators  bool

	// Postprocess configuration. Zero values mean "do not run that
	// step"; non-zero values enable it.
	//
	// MergeTargetSize > 0 enables greedy-mode merge; MergeStrategy is
	// only consulted when MergeTargetSize > 0.
	MergeTargetSize int
	MergeStrategy   string // "greedy" (only supported value today)

	// FilterMinLength > 0 drops chunks shorter than that (rune
	// count). FilterMaxLength > 0 drops chunks longer than that.
	// Both > 0 keeps only chunks within the inclusive range.
	FilterMinLength int
	FilterMaxLength int

	// AddMetadata flags reserved for future use; left typed so a
	// later plan slice can extend without bumping the signature.
	IncludeIndex bool
}

// validate ensures the typed option set is internally consistent.
// The check is cheap; an option set that fails validation will not
// produce meaningful results at run time.
func (o PipelineOptions) validate() error {
	if o.SplitStrategy == "" {
		// Default to sentence to match the operator's own default;
		// callers that want a no-op split should override with
		// "paragraph" + a FilterMinLength >= 1.
		o.SplitStrategy = "sentence"
	}
	if o.MergeTargetSize < 0 {
		return fmt.Errorf("chunk: MergeTargetSize must be >= 0 (got %d)", o.MergeTargetSize)
	}
	if o.FilterMinLength < 0 || o.FilterMaxLength < 0 {
		return fmt.Errorf("chunk: filter bounds must be >= 0 (got min=%d max=%d)",
			o.FilterMinLength, o.FilterMaxLength)
	}
	if o.FilterMinLength > 0 && o.FilterMaxLength > 0 && o.FilterMinLength > o.FilterMaxLength {
		return fmt.Errorf("chunk: FilterMinLength (%d) must be <= FilterMaxLength (%d)",
			o.FilterMinLength, o.FilterMaxLength)
	}
	return nil
}

// Run executes a chunk pipeline against `text` using the typed
// options and returns the resulting ChunkContext. The pipeline
// runs preprocess -> split -> postprocess and builds operators
// directly from typed fields, so production callers avoid a JSON
// round-trip on every invocation.
//
// On option-validation failure or operator failure, Run returns a
// partial ChunkContext with whatever output the operators had
// produced so far, plus an error. This matches the engine's
// operator model, which mutates the shared ChunkContext.
func Run(text string, opts PipelineOptions) (*ChunkContext, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	ctx := &ChunkContext{Origin: text}

	var stages []Operator
	var stageNames []string

	// Preprocess stage — only added if at least one flag is set so
	// an "all-zero" PipelineOptions produces a no-op pipeline.
	if opts.NormalizeNewlines || opts.StripWhitespace || opts.RemoveEmptyLines || opts.SoftLineBreakMerging {
		pre, err := NewPreprocessOperator(map[string]interface{}{
			"normalize_newlines":      opts.NormalizeNewlines,
			"strip_whitespace":        opts.StripWhitespace,
			"remove_empty_lines":      opts.RemoveEmptyLines,
			"soft_line_break_merging": opts.SoftLineBreakMerging,
		})
		if err != nil {
			return nil, fmt.Errorf("chunk: build preprocess: %w", err)
		}
		stages = append(stages, pre)
		stageNames = append(stageNames, "preprocess")
	}

	// Split stage — every typed-options caller uses split, so we
	// always add it. The strategy falls through to the operator's
	// own default when empty.
	splitCfg := map[string]interface{}{
		"strategy": opts.SplitStrategy,
	}
	if len(opts.SplitBoundaries) > 0 {
		splitCfg["params"] = map[string]interface{}{
			"boundaries":      toIfaceSlice(opts.SplitBoundaries),
			"keep_separators": opts.KeepSeparators,
		}
	}
	split, err := NewSplitOperator(splitCfg)
	if err != nil {
		return nil, fmt.Errorf("chunk: build split: %w", err)
	}
	stages = append(stages, split)
	stageNames = append(stageNames, "split")

	// Postprocess stage — only added if at least one toggle is set
	// so a "no postprocess" PipelineOptions skips the stage cleanly.
	postCfg := map[string]interface{}{}
	if opts.MergeTargetSize > 0 {
		strategy := opts.MergeStrategy
		if strategy == "" {
			strategy = "greedy"
		}
		postCfg["merge"] = map[string]interface{}{
			"target_size": opts.MergeTargetSize,
			"strategy":    strategy,
		}
	}
	if opts.FilterMinLength > 0 || opts.FilterMaxLength > 0 {
		filter := map[string]interface{}{}
		if opts.FilterMinLength > 0 {
			filter["min_length"] = opts.FilterMinLength
		}
		if opts.FilterMaxLength > 0 {
			filter["max_length"] = opts.FilterMaxLength
		}
		postCfg["filter"] = filter
	}
	if opts.IncludeIndex {
		postCfg["add_metadata"] = map[string]interface{}{
			"include_index": true,
		}
	}
	if len(postCfg) > 0 {
		post, err := NewPostprocessOperator(postCfg)
		if err != nil {
			return nil, fmt.Errorf("chunk: build postprocess: %w", err)
		}
		stages = append(stages, post)
		stageNames = append(stageNames, "postprocess")
	}

	for i, op := range stages {
		if err := op.Prepare(ctx); err != nil {
			return ctx, fmt.Errorf("%s: prepare: %w", stageNames[i], err)
		}
		if err := op.Execute(ctx); err != nil {
			return ctx, fmt.Errorf("%s: execute: %w", stageNames[i], err)
		}
		if err := op.Finish(ctx); err != nil {
			return ctx, fmt.Errorf("%s: finish: %w", stageNames[i], err)
		}
	}
	return ctx, nil
}

// RunDSL compiles a JSON DSL blob and runs it against `text`.
func RunDSL(dsl string, text string) (*ChunkContext, error) {
	plan, err := compileDSL(dsl)
	if err != nil {
		return nil, err
	}
	return executePlan(plan, text)
}

// ExplainDSL renders a human-readable description of a DSL blob.
func ExplainDSL(dsl string) (string, error) {
	plan, err := compileDSL(dsl)
	if err != nil {
		return "", err
	}
	return explainPlan(plan)
}

// toIfaceSlice adapts []string to the []interface{} shape the
// operator constructors type-assert through (the DSL decoders
// surface every list element as `interface{}`). Tiny helper so
// Run does not depend on encoding/json.
func toIfaceSlice(in []string) []interface{} {
	if len(in) == 0 {
		return nil
	}
	out := make([]interface{}, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}
