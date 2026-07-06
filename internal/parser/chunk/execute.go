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

// Internal chunk execution entrypoint used by production callers.
package chunk

import "fmt"

// Run executes the internal chunk steps against `text` using typed
// options. The sequence is preprocess -> split -> postprocess.
func Run(text string, opts ChunkOptions) (*ChunkContext, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}

	ctx := &ChunkContext{
		Origin:              text,
		TextAfterPreprocess: text,
	}

	var stages []Operator
	var stageNames []string

	if opts.NormalizeNewlines || opts.StripWhitespace || opts.RemoveEmptyLines {
		pre, err := NewPreprocessOperator(map[string]interface{}{
			"normalize_newlines": opts.NormalizeNewlines,
			"strip_whitespace":   opts.StripWhitespace,
			"remove_empty_lines": opts.RemoveEmptyLines,
		})
		if err != nil {
			return nil, fmt.Errorf("chunk: build preprocess: %w", err)
		}
		stages = append(stages, pre)
		stageNames = append(stageNames, "preprocess")
	}

	split, err := NewSplitOperator(map[string]interface{}{
		"strategy": opts.SplitStrategy,
	})
	if err != nil {
		return nil, fmt.Errorf("chunk: build split: %w", err)
	}
	stages = append(stages, split)
	stageNames = append(stageNames, "split")

	postCfg := map[string]interface{}{}
	if opts.MergeTargetSize > 0 {
		postCfg["merge"] = map[string]interface{}{
			"target_size": float64(opts.MergeTargetSize),
			"strategy":    "greedy",
		}
	}
	if opts.FilterMinLength > 0 {
		postCfg["filter"] = map[string]interface{}{
			"min_length": float64(opts.FilterMinLength),
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
	if len(ctx.ResultChunks) == 0 && len(ctx.SplitChunks) > 0 {
		ctx.ResultChunks = ctx.SplitChunks
	}
	return ctx, nil
}
