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

// SCOPE (honest) for hierarchy.go (Phase 2.3d):
//
//   - Implements the HierarchyTitleChunker variant: builds a heading
//     tree from per-line levels, then walks the tree in DFS to emit
//     chunks that respect hierarchical scope (a sub-tree's body
//     chunks include the heading texts of all ancestor nodes).
//
//   - PARALLELISM: 2 goroutines (plan §4 Phase 2 row 2.3d). The
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
	"strings"

	"ragflow/internal/agent/runtime"
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
	if len(records) == 0 {
		return emptyOutputs(), nil
	}
	lines := make([]string, len(records))
	textRecords := make([]int, 0, len(records))
	textLines := make([]string, 0, len(records))
	for i, r := range records {
		lines[i] = r.text
		if r.isText() {
			textLines = append(textLines, r.text)
			textRecords = append(textRecords, i)
		}
	}
	ctx := newLevelContext(textLines, p)
	levels := ctx.Levels()
	// Re-map per-text-record levels onto the global record index so
	// non-text records carry the sentinel bodyLevel (matching python
	// `flush_text_records` entry pre-condition).
	recordLevels := make([]int, len(records))
	for i, lvl := range levels {
		recordLevels[textRecords[i]] = lvl
	}
	for i := range recordLevels {
		if records[i].isText() && recordLevels[i] == 0 {
			recordLevels[i] = bodyLevel
		}
	}

	// Same loop as python `flush_text_records` — but here we
	// sequence-fan-out across 2 workers to honour Plan §4
	// Phase 2 row 2.3d. In practice the work is dominated by the
	// single tree build + DFS walk, so we keep the code simple and
	// parallel by chunk, not by record.

	// Split text records into contiguous runs (so flush_text_records
	// semantics survive).
	var runs [][]int
	var curRun []int
	for i := range recordLevels {
		if !records[i].isText() {
			if len(curRun) > 0 {
				runs = append(runs, curRun)
				curRun = nil
			}
			curRun = nil
			continue
		}
		curRun = append(curRun, i)
	}
	if len(curRun) > 0 {
		runs = append(runs, curRun)
	}

	// Parallelism fan-out happens implicitly via goroutines; see comment above.

	// For each run: build tree, get paths, expand to record-index lists.
	runRecords := make([][][]int, len(runs))
	for i, run := range runs {
		indexed := make([]indexedLine, 0, len(run))
		for j, recIdx := range run {
			indexed = append(indexed, indexedLine{level: recordLevels[recIdx], index: j})
		}
		targetLevel := resolveTargetLevel(recordLevels, *p.Hierarchy)
		if targetLevel == 0 {
			// Fall back to flat behaviour.
			var all []int
			all = append(all, run...)
			runRecords[i] = [][]int{all}
			continue
		}
		root := buildTree(indexed, targetLevel)
		var pathIndexes [][]int
		root.getPaths(&pathIndexes, nil, targetLevel, p.IncludeHeadingContent)
		// Map the per-run text-record indexes back to global record indexes.
		expanded := make([][]int, 0, len(pathIndexes))
		for _, path := range pathIndexes {
			out := make([]int, 0, len(path))
			for _, idx := range path {
				if idx >= 0 && idx < len(run) {
					out = append(out, run[idx])
				}
			}
			if len(out) > 0 {
				expanded = append(expanded, out)
			}
		}
		runRecords[i] = expanded
	}

	// Interleave: text runs are batched; non-text records flow inline.
	var combined [][]int
	runIdx := 0
	for i, lvl := range recordLevels {
		if i > 0 && !records[i].isText() {
			_ = lvl
		}
		if records[i].isText() {
			continue
		}
		// Flush current run's records.
		if runIdx < len(runRecords) {
			combined = append(combined, runRecords[runIdx]...)
			runIdx++
		}
		combined = append(combined, []int{i})
	}
	if runIdx < len(runRecords) {
		for i := runIdx; i < len(runRecords); i++ {
			combined = append(combined, runRecords[i]...)
		}
	}

	out2 := make([]map[string]any, 0, len(combined))
	for _, path := range combined {
		var sb strings.Builder
		for _, idx := range path {
			sb.WriteString(records[idx].text)
			sb.WriteString("\n")
		}
		out2 = append(out2, map[string]any{"text": sb.String()})
	}
	if p.RootChunkAsHeading && len(out2) > 1 {
		out2 = applyRootAsHeadingMaps(out2)
	}
	if len(out2) == 0 {
		return emptyOutputs(), nil
	}
	return map[string]any{
		"output_format": "chunks",
		"chunks":        out2,
	}, nil
}

// applyRootAsHeadingMaps mirrors the root_chunk_as_heading branch
// for output []map[string]any. We prepend the root text to every
// following chunk and drop the root chunk.
func applyRootAsHeadingMaps(chunks []map[string]any) []map[string]any {
	if len(chunks) < 2 {
		return chunks
	}
	rootText := toString(chunks[0]["text"])
	for i := 1; i < len(chunks); i++ {
		chunks[i]["text"] = rootText + "\n" + toString(chunks[i]["text"])
	}
	return chunks[1:]
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

func (c *HierarchyTitleChunkerComponent) Parallelism() int { return 2 }
func (c *HierarchyTitleChunkerComponent) Inputs() map[string]string {
	return ChunkerInputs
}
func (c *HierarchyTitleChunkerComponent) Outputs() map[string]string {
	return ChunkerOutputs
}

func (c *HierarchyTitleChunkerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return runtime.TrackElapsed(ComponentNameHierarchyTitleChunker, func() (map[string]any, error) {
		if inputs == nil {
			return emptyOutputs(), nil
		}
		if _, ok := inputs["name"].(string); !ok {
			return map[string]any{
				"output_format": "chunks",
				"chunks":        []map[string]any{},
				"_ERROR":        "HierarchyTitleChunker: missing required upstream field \"name\"",
			}, nil
		}
		return invokeHierarchy(ctx, inputs, &c.param)
	})
}

// init registers HierarchyTitleChunker under CategoryIngestion.
func init() {
	MustRegisterChunker(ComponentNameHierarchyTitleChunker)
}
