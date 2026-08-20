//	Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//	Licensed under the Apache License, Version 2.0 (the "License");
//	you may not use this file except in compliance with the License.
//	You may obtain a copy of the License at
//
//	    http://www.apache.org/licenses/LICENSE-2.0
//
//	Unless required by applicable law or agreed to in writing, software
//	distributed under the License is distributed on an "AS IS" BASIS,
//	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//	See the License for the specific language governing permissions and
//	limitations under the License.
//
// ManualChunker is the Go port of Python's `manual` doc-type chunk method
// (rag/app/manual.py). Like GroupTitleChunker it merges adjacent text records
// into heading-bounded groups, but it first re-sorts the records into physical
// reading order before grouping.
//
// Why the resort matters: for multi-column / manually-laid-out PDFs the parser
// emits records in READING order (column-by-column), so a naive grouping
// interleaves the left column's later content with the right column's earlier
// content. ManualChunker re-orders records by (page, top, left) — mirroring
// Python's
//
//	sorted(sections, key=lambda x: (x[-1][0][0], x[-1][0][3], x[-1][0][1]))
//
// so grouping then follows the true top-down, left-to-right layout.
//
// Crucially, docx (and any coordinate-free payload) emits records in
// document-logical order with NO positions — exactly as Python's manual docx
// branch, which performs no resort. When no record carries coordinates,
// ManualChunker skips the sort entirely and reuses the identical grouping
// pipeline as GroupTitleChunker, so its output is bit-for-bit equal (locked by
// TestManualChunker_NoPositionsEqualsGroupChunker). No parser change is needed.
package chunker

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/globals"

	"gorm.io/gorm"
)

const ComponentNameManualChunker = "ManualChunker"

// ManualChunkerComponent is the standalone manual-layout chunker. It shares
// the GroupTitleChunker grouping body (chunkFromRecords) and only differs by
// the optional physical-position pre-sort.
type ManualChunkerComponent struct {
	name  string
	param titleChunkerParam
}

// NewManualChunker constructs the component. It accepts the same heading-level
// params as TitleChunker (method is pinned to "group"); the position resort is
// automatic based on whether the upstream payload carries coordinates.
func NewManualChunker(params map[string]any) (runtime.Component, error) {
	// method is pinned to "group": ManualChunker's entire value-add is the
	// physical-position resort, which only fires inside the group path.
	// A caller must not downgrade it to "naive"/"title" (that would skip the
	// resort and silently diverge from Python's manual.py), so method is
	// deliberately ignored here even if passed in params.
	conf := map[string]any{"method": "group"}
	for k, v := range params {
		if k == "method" {
			continue
		}
		conf[k] = v
	}
	p := defaultsTitle()
	p.Update(conf)
	if err := p.TitleChunkerParam.Validate(); err != nil {
		return nil, fmt.Errorf("ManualChunker: %w", err)
	}
	return &ManualChunkerComponent{
		name:  ComponentNameManualChunker,
		param: p,
	}, nil
}

func (c *ManualChunkerComponent) Inputs() map[string]string  { return ChunkerInputs }
func (c *ManualChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

func (c *ManualChunkerComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
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
			"_ERROR":        "ManualChunker: missing required upstream field \"name\"",
		}, nil
	}
	return c.invoke(ctx, db, withName(inputs, name))
}

func (c *ManualChunkerComponent) invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	records := extractLineRecords(inputs)
	if len(records) == 0 {
		return emptyOutputs(), nil
	}
	// Coordinate-free payloads (docx, plain text) need no resort — and
	// skipping it keeps the output identical to GroupTitleChunker.
	if hasPdfPositions(records) {
		sortRecordsByPosition(records)
	}
	// ManualChunker is exempt from the title-family token cap (#18455): it
	// does not inherit BaseTitleChunker, so it always passes tokenCap=0.
	return chunkFromRecords(ctx, db, inputs, &c.param, records, 0)
}

// hasPdfPositions reports whether any record carries a PDF coordinate matrix
// (either the structured `_pdf_positions` or the legacy `positions` form).
func hasPdfPositions(records []lineRecord) bool {
	for _, r := range records {
		if len(r.pdfPositions) > 0 || len(r.positions) > 0 {
			return true
		}
	}
	return false
}

// sortRecordsByPosition re-orders records into physical reading order
// (page, then top, then left) using a stable sort, so records that share a
// key keep their original relative order. Records without coordinates act as
// immovable barriers: they keep their original relative order and are never
// compared against positioned records.
//
// The slice is split into maximal contiguous runs of positioned records and
// each run is sorted independently. This is required because a single
// sort.SliceStable over a MIXED slice (some records with coordinates, some
// without) is not a strict weak ordering: a coordinate-free record B sits
// incomparable between two positioned records A and C, yet A and C remain
// comparable — breaking equivalence-class transitivity and making the sort
// undefined behaviour. Segmenting on coordinate-free records keeps each sort
// a valid strict weak ordering while preserving the documented behaviour that
// coordinate-free records keep their input order (see hasPdfPositions gate).
func sortRecordsByPosition(records []lineRecord) {
	runStart := -1
	flush := func(end int) {
		if runStart < 0 {
			return
		}
		sort.SliceStable(records[runStart:end], func(i, j int) bool {
			ar, _ := firstPositionRow(records[runStart+i])
			br, _ := firstPositionRow(records[runStart+j])
			return pdfPosRowLess(ar, br)
		})
		runStart = -1
	}
	for i := 0; i <= len(records); i++ {
		positioned := i < len(records)
		if positioned {
			_, positioned = firstPositionRow(records[i])
		}
		if positioned {
			if runStart < 0 {
				runStart = i
			}
		} else {
			flush(i)
		}
	}
}

// firstPositionRow returns the first PDF coordinate 5-tuple
// [page,left,right,top,bottom] of a record's coordinate matrix, or
// (nil, false) when the record carries no usable coordinates. It reads the
// structured `_pdf_positions` key first and falls back to the legacy
// `positions` key. Callers (pdfPosRowLess) index offsets 0/1/3, so we enforce
// len(row) >= 5 here.
func firstPositionRow(r lineRecord) ([]float64, bool) {
	var raw json.RawMessage
	switch {
	case len(r.pdfPositions) > 0:
		raw = r.pdfPositions
	case len(r.positions) > 0:
		raw = r.positions
	default:
		return nil, false
	}
	var mat [][]float64
	if err := json.Unmarshal(raw, &mat); err != nil || len(mat) == 0 {
		return nil, false
	}
	row := mat[0]
	if len(row) < 5 {
		return nil, false
	}
	return row, true
}

// init registers ManualChunker under CategoryIngestion.
func init() {
	MustRegisterChunker(ComponentNameManualChunker)
}
