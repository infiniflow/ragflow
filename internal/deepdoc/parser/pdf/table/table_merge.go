
package table

import (
	"sort"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// MergeTablesAcrossPages merges TableItems on consecutive pages with
// overlapping X and close Y proximity.  Matches Python's
// _extract_table_figure table merge (pdf_parser.py:1061-1080).
func MergeTablesAcrossPages(tables []pdf.TableItem, medianHeights map[int]float64) []pdf.TableItem {
	if len(tables) <= 1 {
		return tables
	}
	// Sort by position for deterministic adjacency.
	type indexed struct {
		idx int
		pg  int
		top float64
	}
	var items []indexed
	for i, tbl := range tables {
		if len(tbl.Positions) == 0 {
			continue
		}
		p := tbl.Positions[0]
		pg := 0
		if len(p.PageNumbers) > 0 {
			pg = p.PageNumbers[0]
		}
		items = append(items, indexed{i, pg, p.Top})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].pg != items[j].pg {
			return items[i].pg < items[j].pg
		}
		return items[i].top < items[j].top
	})

	merged := make([]bool, len(tables))
	var result []pdf.TableItem

	for _, it := range items {
		if merged[it.idx] {
			continue
		}
		anchor := tables[it.idx]
		merged[it.idx] = true

		// Python nomerge_lout_no: tables whose box is followed by a
		// caption/title/reference should not be merged cross-page.
		if anchor.NoMerge {
			result = append(result, anchor)
			continue
		}

		anchorPg := it.pg
		anchorBtm := anchor.Positions[0].Bottom

		// Look for consecutive-page continuations.
		for _, jt := range items {
			if merged[jt.idx] || jt.pg <= anchorPg {
				continue
			}
			// Python nomerge_lout_no: skip continuation candidates
			// tagged as no-merge.
			if tables[jt.idx].NoMerge {
				continue
			}
			if jt.pg-anchorPg > 1 {
				break // pages must be consecutive
			}
			if len(tables[jt.idx].Positions) == 0 {
				continue
			}
			bp := tables[jt.idx].Positions[0]
			bpg := 0
			if len(bp.PageNumbers) > 0 {
				bpg = bp.PageNumbers[0]
			}
			if bpg != anchorPg+1 {
				continue
			}
			// Check X overlap.
			ap := anchor.Positions[0]
			if ap.Right < bp.Left || bp.Right < ap.Left {
				continue
			}
			// Check Y proximity: page 1 table top should be close below
			// page 0 table bottom.  Python: y_dis <= mh * 23.
			mh := 10.0
			if medianHeights != nil {
				if h, ok := medianHeights[anchorPg]; ok && h > 0 {
					mh = h
				}
			}
			yDis := (bp.Top + bp.Bottom - anchorBtm - ap.Bottom) / 2
			if yDis > mh*23 {
				continue
			}
			// Merge: combine cells and positions.
			anchor.Cells = append(anchor.Cells, tables[jt.idx].Cells...)
			anchor.Positions = append(anchor.Positions, tables[jt.idx].Positions...)
			if tables[jt.idx].Caption != "" {
				if anchor.Caption != "" {
					anchor.Caption += " "
				}
				anchor.Caption += tables[jt.idx].Caption
			}
			merged[jt.idx] = true
			anchorPg = bpg
			anchorBtm = bp.Bottom
			ap = anchor.Positions[len(anchor.Positions)-1]
		}
		result = append(result, anchor)
	}
	// Append unprocessed tables (those with empty Positions) so they
	// are not silently dropped from the output.
	for i := range tables {
		if !merged[i] {
			result = append(result, tables[i])
		}
	}
	return result
}

