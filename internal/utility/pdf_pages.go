package utility

import "sort"

// NormalizePDFPages normalizes a raw "pages" value (list[list[int]], 1-indexed
// inclusive ranges, JSON-decoded as []any of []any of float64) into a sorted,
// merged, de-duplicated [][]int. Invalid ranges are dropped. Returns nil when
// raw is empty/missing or every range is invalid (callers treat nil as
// "parse all pages").
//
// Validation per range [from, to]:
//   - both values must be integers (int, int64, or integral float64);
//   - from >= 1 (1-indexed);
//   - from <= to.
//
// Surviving ranges are sorted by `from` then merged when overlapping or
// adjacent (next.from <= cur.to + 1).
func NormalizePDFPages(raw any) [][]int {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil
	}

	ranges := make([][]int, 0, len(list))
	for _, item := range list {
		pair, ok := item.([]any)
		if !ok || len(pair) != 2 {
			continue
		}
		from, ok := toInt(pair[0])
		if !ok {
			continue
		}
		to, ok := toInt(pair[1])
		if !ok {
			continue
		}
		if from < 1 || from > to {
			continue
		}
		ranges = append(ranges, []int{from, to})
	}
	if len(ranges) == 0 {
		return nil
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
	return merged
}

// toInt coerces a JSON-decoded numeric value to int. Accepts int, int64, and
// integral float64; rejects non-numeric or non-integral values.
func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		// Reject non-integral floats (e.g. 1.5) — pages must be integers.
		if x != float64(int(x)) {
			return 0, false
		}
		return int(x), true
	default:
		return 0, false
	}
}
