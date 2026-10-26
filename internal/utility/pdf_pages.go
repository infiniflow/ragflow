package utility

import (
	"fmt"
	"ragflow/internal/common"
	"sort"
)

// NormalizePDFPages normalizes a raw "pages" value (list[list[int]], 1-indexed
// inclusive ranges, JSON-decoded as []any of []any of float64) into a sorted,
// merged, de-duplicated [][]int.
//
// Semantics (fail-fast):
//   - nil or empty list → (nil, nil): "no value", callers treat as "parse all
//     pages".
//   - Any invalid range → (nil, error): the whole input is rejected; callers
//     should surface the error and abort the request. No partial dropping.
//   - All ranges valid → (normalized, nil).
//
// Validation per range [from, to]:
//   - both values must be integers (int, int64, or integral float64);
//   - from >= 1 (1-indexed);
//   - from <= to.
//
// Surviving ranges are sorted by `from` then merged when overlapping or
// adjacent (next.from <= cur.to + 1).
func NormalizePDFPages(raw any) ([][]int, error) {
	list, ok := raw.([]any)
	if !ok {
		// nil raw (no key / JSON null) is "no value"; a non-list raw is a type
		// error. raw==nil falls through the !ok branch because nil does not
		// satisfy []any.
		if raw == nil {
			return nil, nil
		}
		return nil, fmt.Errorf("pages must be a list of [from,to] ranges, got %T", raw)
	}
	if len(list) == 0 {
		return nil, nil
	}

	ranges := make([][]int, 0, len(list))
	for _, item := range list {
		var pair []any
		pair, ok = item.([]any)
		if !ok || len(pair) != 2 {
			return nil, fmt.Errorf("invalid page range %v: must be a [from,to] pair", item)
		}

		var from int
		from, ok = common.ToInt(pair[0])
		if !ok {
			return nil, fmt.Errorf("invalid page range [%v,%v]: from must be an integer", pair[0], pair[1])
		}

		var to int
		to, ok = common.ToInt(pair[1])
		if !ok {
			return nil, fmt.Errorf("invalid page range [%v,%v]: to must be an integer", pair[0], pair[1])
		}
		if from < 1 {
			return nil, fmt.Errorf("invalid page range [%d,%d]: from must be >= 1", from, to)
		}
		if from > to {
			return nil, fmt.Errorf("invalid page range [%d,%d]: from must be <= to", from, to)
		}
		ranges = append(ranges, []int{from, to})
	}

	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i][0] != ranges[j][0] {
			return ranges[i][0] < ranges[j][0]
		}
		return ranges[i][1] < ranges[j][1]
	})

	merged := ranges[:1]
	for _, r := range ranges[1:] {
		last := merged[len(merged)-1]
		if r[0] <= last[1]+1 { // overlap or adjacent
			if r[1] > last[1] {
				last[1] = r[1]
			}
		} else {
			merged = append(merged, r)
		}
	}
	return merged, nil
}

// PDFPageInRanges reports whether the 1-indexed page lies inside ranges, as
// returned by NormalizePDFPages. Empty ranges select every page.
func PDFPageInRanges(ranges [][]int, page int) bool {
	if len(ranges) == 0 {
		return true
	}
	for _, r := range ranges {
		if page >= r[0] && page <= r[1] {
			return true
		}
	}
	return false
}
