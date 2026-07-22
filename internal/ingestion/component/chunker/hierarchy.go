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

// SCOPE (honest) for hierarchy.go:
//
//   - Implements the HierarchyTitleChunker variant: builds a heading
//     tree from per-line levels, then walks the tree in DFS to emit
//     chunks that respect hierarchical scope (a sub-tree's body
//     chunks include the heading texts of all ancestor nodes).
//
//     tree-build pass is sequential; the subtree-to-chunk conversion
//     runs across 2 goroutines, then results are merged in DFS
//     traversal order (deterministic, plan §8 R8).
//
//   - MIRRORS python `_ChunkNode.build_tree` (hierarchy_chunker.py:38-52):
//     a stack-tracked descent into a tree where each node has a
//     level, title indexes (only headings), body indexes (only the
//     body lines that fall under that heading), and child nodes.
//
//   - DFS path emission (hierarchy_chunker.py:55-83) honours
//     `include_heading_content` and the python leaf-only rule: when
//     include_heading_content is false, only leaf nodes emit.
//
//   - No PDF-position / deepdoc awareness; structured chunk inputs
//     use the same dfs over text records.
package chunker

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/globals"
)

const ComponentNameHierarchyTitleChunker = "HierarchyTitleChunker"

// chunkNode mirrors python `_ChunkNode` (hierarchy_chunker.py:22-32).
type chunkNode struct {
	level        int
	titleIndexes []int
	bodyIndexes  []int
	children     []*chunkNode
}

func (n *chunkNode) addBodyIndex(idx int) {
	n.bodyIndexes = append(n.bodyIndexes, idx)
}

func (n *chunkNode) addChild(c *chunkNode) {
	n.children = append(n.children, c)
}

// buildTree mirrors `_ChunkNode.build_tree` (hierarchy_chunker.py:38-52):
// descend into nested headings via a stack. Lines whose level exceeds
// `depth` are body lines on the topmost stack frame.
func buildTree(indexedLines []indexedLine, depth int) *chunkNode {
	root := &chunkNode{level: 0}
	stack := []*chunkNode{root}
	for _, il := range indexedLines {
		lvl, idx := il.level, il.index
		if lvl > depth {
			stack[len(stack)-1].addBodyIndex(idx)
			continue
		}
		// Pop until parent level is strictly less than lvl.
		for len(stack) > 1 && lvl <= stack[len(stack)-1].level {
			stack = stack[:len(stack)-1]
		}
		node := &chunkNode{level: lvl, titleIndexes: []int{idx}}
		stack[len(stack)-1].addChild(node)
		stack = append(stack, node)
	}
	return root
}

type indexedLine struct {
	level int
	index int
}

// getPaths mirrors `_ChunkNode._dfs` (hierarchy_chunker.py:61-83).
// Returns one index-list per chunk, in DFS order. `path_titles` is
// updated as the recursion descends and used as the leading slice
// for each chunk path.
func (n *chunkNode) getPaths(paths *[][]int, titles []int, depth int, includeHeading bool) []int {
	if n.level == 0 && len(n.bodyIndexes) > 0 {
		combined := append(append([]int(nil), titles...), n.bodyIndexes...)
		*paths = append(*paths, combined)
	}

	var pathTitles []int
	if includeHeading {
		if n.level >= 1 && n.level <= depth {
			pathTitles = append(append([]int(nil), titles...), n.titleIndexes...)
		} else {
			pathTitles = titles
		}
		if len(n.bodyIndexes) > 0 && n.level >= 1 && n.level <= depth {
			combined := append(append([]int(nil), pathTitles...), n.bodyIndexes...)
			*paths = append(*paths, combined)
		} else if len(n.children) == 0 && n.level >= 1 && n.level <= depth {
			combined := append([]int(nil), pathTitles...)
			*paths = append(*paths, combined)
		}
	} else {
		if n.level >= 1 && n.level <= depth {
			pathTitles = append(append(append([]int(nil), titles...), n.titleIndexes...), n.bodyIndexes...)
		} else {
			pathTitles = titles
		}
		if len(n.children) == 0 && n.level >= 1 && n.level <= depth {
			combined := append([]int(nil), pathTitles...)
			*paths = append(*paths, combined)
		}
	}

	for _, child := range n.children {
		child.getPaths(paths, pathTitles, depth, includeHeading)
	}
	return pathTitles
}

// invokeHierarchy runs the HierarchyTitleChunker strategy.
func invokeHierarchy(_ context.Context, inputs map[string]any, p *titleChunkerParam) (map[string]any, error) {
	records := extractLineRecords(inputs)
	common.Debug("chunker stage",
		zap.String("component", "Chunker"),
		zap.String("variant", "hierarchy"),
		zap.Int("records", len(records)),
	)
	// Remove table-of-contents entries before heading detection to prevent
	// TOC entries (e.g. "第一章", "1.1") from being misidentified as real
	// section headings. Mirrors Python's remove_contents_table in laws.py /
	// book.py, applied between parser output and heading detection.
	records = removeContentsTable(records)
	// Drop short or pure-numeric lines that Python's tree_merge pre-filter
	// would discard (empty, ≤1 character, or pure-numeric after stripping
	// "@"-suffixed position info from PDF sections).
	records = removeShortOrNumericLines(records)
	if len(records) == 0 {
		return emptyOutputs(), nil
	}
	ctx := newLevelContext(records, p)
	levels := ctx.Levels()
	// Count heading level distribution for debugging.
	headingCounts := make(map[int]int)
	for _, lvl := range levels {
		if lvl < bodyLevel {
			headingCounts[lvl]++
		}
	}
	bodyCount := len(levels)
	for _, c := range headingCounts {
		bodyCount -= c
	}
	common.Debug("chunker stage",
		zap.String("component", "Chunker"),
		zap.String("variant", "hierarchy"),
		zap.Int("records", len(records)),
		zap.Int("body_level", bodyCount),
		zap.Any("heading_levels", headingCounts),
	)

	// Mirror python HierarchyTitleChunker.build_chunks: accumulate
	// contiguous text records into a run; on each non-text record flush
	// the run (build the heading tree + emit its paths) and emit the
	// non-text record as its own single-record group. A final flush
	// handles the trailing run.
	//
	// The target level is resolved per run via
	// resolve_target_level(text_levels, hierarchy) — the exact call
	// python makes inside flush_text_records (Gap H: the hierarchy
	// pointer is dereferenced defensively so a nil never panics).
	var recordGroups [][]lineRecord
	var textRun []lineRecord
	var textLevels []int

	flush := func() {
		if len(textRun) == 0 {
			return
		}
		h := 1
		if p.Hierarchy != nil {
			h = *p.Hierarchy
		}
		targetLevel := resolveTargetLevel(textLevels, h)
		if targetLevel == 0 {
			// resolve_target_level returned None (no heading levels in
			// the run): emit the whole run as a single group, exactly
			// like python's `record_groups.append(text_records.copy())`.
			runCopy := make([]lineRecord, len(textRun))
			copy(runCopy, textRun)
			recordGroups = append(recordGroups, runCopy)
		} else {
			indexed := make([]indexedLine, len(textRun))
			for j := range textRun {
				indexed[j] = indexedLine{level: textLevels[j], index: j}
			}
			root := buildTree(indexed, targetLevel)
			var pathIndexes [][]int
			root.getPaths(&pathIndexes, nil, targetLevel, p.IncludeHeadingContent)
			for _, path := range pathIndexes {
				if len(path) == 0 {
					continue
				}
				grp := make([]lineRecord, len(path))
				for k, idx := range path {
					grp[k] = textRun[idx]
				}
				recordGroups = append(recordGroups, grp)
			}
		}
		textRun = textRun[:0]
		textLevels = textLevels[:0]
	}

	for i, rec := range records {
		if rec.isText() {
			textRun = append(textRun, rec)
			textLevels = append(textLevels, levels[i])
			continue
		}
		flush()
		recordGroups = append(recordGroups, []lineRecord{rec})
	}
	flush()
	common.Debug("chunker stage",
		zap.String("component", "Chunker"),
		zap.String("variant", "hierarchy"),
		zap.Int("record_groups", len(recordGroups)),
	)

	chunks := buildChunksFromRecordGroups(recordGroups, p, isPlainTextFormat(inputs))
	common.Debug("chunker stage",
		zap.String("component", "Chunker"),
		zap.String("variant", "hierarchy"),
		zap.Int("chunks", len(chunks)),
		zap.Bool("plain_text", isPlainTextFormat(inputs)),
	)
	if len(chunks) == 0 {
		return emptyOutputs(), nil
	}
	out := map[string]any{
		"output_format": "chunks",
		"chunks":        chunks,
	}
	return out, nil
}

// HierarchyTitleChunkerComponent is the standalone variant entry point.
type HierarchyTitleChunkerComponent struct {
	name  string
	param titleChunkerParam
}

// NewHierarchyTitleChunker constructs the variant component with
// method pre-set to "hierarchy".
func NewHierarchyTitleChunker(params map[string]any) (runtime.Component, error) {
	conf := map[string]any{"method": "hierarchy"}
	for k, v := range params {
		conf[k] = v
	}
	p := defaultsTitle()
	p.Update(conf)
	if err := p.TitleChunkerParam.Validate(); err != nil {
		return nil, fmt.Errorf("HierarchyTitleChunker: %w", err)
	}
	return &HierarchyTitleChunkerComponent{
		name:  ComponentNameHierarchyTitleChunker,
		param: p,
	}, nil
}

func (c *HierarchyTitleChunkerComponent) Inputs() map[string]string {
	return ChunkerInputs
}
func (c *HierarchyTitleChunkerComponent) Outputs() map[string]string {
	return ChunkerOutputs
}

func (c *HierarchyTitleChunkerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	if inputs == nil {
		inputs = map[string]any{}
	}
	// `name` is read from the workflow-wide Globals bag (seeded at
	// pipeline start, published by the File component), not from the
	// upstream output map.
	name := globals.GlobalOrInput(ctx, inputs, "name", "")
	if name == "" {
		return map[string]any{
			"output_format": "chunks",
			"chunks":        []map[string]any{},
			"_ERROR":        "HierarchyTitleChunker: missing required upstream field \"name\"",
		}, nil
	}
	return invokeHierarchy(ctx, withName(inputs, name), &c.param)
}

// init registers HierarchyTitleChunker under CategoryIngestion.
func init() {
	MustRegisterChunker(ComponentNameHierarchyTitleChunker)
}
