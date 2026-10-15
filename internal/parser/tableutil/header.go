package tableutil

import (
	"regexp"
	"strings"
	"time"

	"ragflow/internal/entity"
)

// Header identifies a contiguous column-header block in the source grid.
// Rows == 0 means no column header was established; Start is then zero.
type Header struct {
	Start int
	Rows  int
}

// HeaderEvidence contains source-only evidence, never persisted on TableData.
// Explicit includes a format's confirmed absence of a header (Rows == 0).
type HeaderEvidence struct {
	Explicit *Header
	Styled   [][]bool
}

// DetectHeader is shared by source parsers. Ambiguous text and ordinary data
// remain data. It samples at most twenty body rows and never changes the grid.
// Call it before segmentation; rendering/re-reading TableData must not infer
// a new header from content.
func DetectHeader(grid [][]entity.TableCell, evidence HeaderEvidence) Header {
	if h := evidence.Explicit; h != nil {
		if h.Start >= 0 && h.Start <= len(grid) && h.Rows >= 0 && h.Rows <= len(grid)-h.Start {
			if h.Rows == 0 {
				return Header{}
			}
			return *h
		}
		return Header{}
	}
	for start := 0; start < len(grid) && start < 4; start++ {
		for count := 1; count <= 3 && start+count < len(grid); count++ {
			end := start + count
			labels := grid[end-1]
			if isSummary(labels) || countHeaderValues(labels) == 0 {
				continue
			}
			// A single wide cell is a title, not a set of column names.
			if countHeaderValues(labels) == 1 && len(grid[end]) > 1 {
				continue
			}
			changed, typed, stable := 0, 0, 0
			var bodyRows []int
			for r := end; r < len(grid) && len(bodyRows) < 20; r++ {
				if countHeaderValues(grid[r]) > 0 {
					bodyRows = append(bodyRows, r)
				}
			}
			for col, label := range labels {
				if label.Covered || strings.TrimSpace(label.Text) == "" {
					continue
				}
				kind, matches, samples := "", 0, 0
				kinds := make(map[string]int, 4)
				for _, r := range bodyRows {
					if col >= len(grid[r]) || grid[r][col].Covered || strings.TrimSpace(grid[r][col].Text) == "" {
						continue
					}
					k := headerValueKind(grid[r][col].Text)
					if k == "missing" {
						continue
					}
					kinds[k]++
					if kinds[k] > matches {
						kind = k
						matches = kinds[k]
					}
					samples++
				}
				if samples == 0 || matches*5 < samples*4 {
					continue
				}
				stable++
				if kind != "text" {
					typed++
					if headerValueKind(label.Text) == "text" {
						changed++
					}
				}
			}
			if stable == 0 {
				continue
			}
			contrast := changed > 0 && changed == typed
			styled := hasStyleBoundary(evidence.Styled, grid, start, end)
			known, values := countColumnLabels(labels)
			if !(contrast || (known > 0 && (styled || start == 0 && known == values))) {
				continue
			}
			if start > 0 && !styled {
				continue
			}
			if count > 1 {
				// An extra header row needs structural/style evidence; do not
				// consume an ordinary first record just to find a contrast later.
				valid := true
				for r := start; r < end-1; r++ {
					if !hasMergedGroup(grid[r]) && !hasStyleBoundary(evidence.Styled, grid, r, end) {
						valid = false
					}
				}
				if !valid {
					continue
				}
			}
			return Header{Start: start, Rows: count}
		}
		// Only look below a preamble. Never move a header past data rows.
		if countHeaderValues(grid[start]) > 1 {
			break
		}
	}
	return Header{}
}

var headerNumber = regexp.MustCompile(`^[\p{Sc}]?[+\-]?([0-9][0-9,]*(\.[0-9]*)?|\.[0-9]+)([eE][+\-]?[0-9]+)?%?$`)

func headerValueKind(value string) string {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "-", "--", "?", "n/a", "na", "null", "none", "unknown", "无", "空":
		return "missing"
	}
	if headerNumber.MatchString(value) {
		return "number"
	}
	for _, layout := range []string{"2006-01-02", "2006/01/02", "02/01/2006", time.RFC3339} {
		if _, err := time.Parse(layout, value); err == nil {
			return "date"
		}
	}
	if strings.EqualFold(value, "true") || strings.EqualFold(value, "false") {
		return "bool"
	}
	return "text"
}

func countHeaderValues(row []entity.TableCell) int {
	n := 0
	for _, c := range row {
		if !c.Covered && strings.TrimSpace(c.Text) != "" {
			n++
		}
	}
	return n
}

func hasMergedGroup(row []entity.TableCell) bool {
	for _, c := range row {
		if !c.Covered && c.ColSpan > 1 {
			return true
		}
	}
	return false
}

func hasStyleBoundary(styles [][]bool, grid [][]entity.TableCell, start, end int) bool {
	if start >= len(styles) || end >= len(styles) {
		return false
	}
	for r := start; r < end; r++ {
		if r >= len(styles) {
			return false
		}
		styled, bodyStyled, values := 0, 0, 0
		for c, cell := range grid[r] {
			// Unobserved columns are not evidence of an unstyled cell.
			if c >= len(styles[r]) || c >= len(styles[end]) || cell.Covered || strings.TrimSpace(cell.Text) == "" {
				continue
			}
			values++
			if styles[r][c] {
				styled++
			}
			if styles[end][c] {
				bodyStyled++
			}
		}
		if values == 0 || styled*2 < values || bodyStyled*2 >= values {
			return false
		}
	}
	return true
}

func isSummary(row []entity.TableCell) bool {
	for _, cell := range row {
		switch strings.TrimRight(strings.ToLower(strings.TrimSpace(cell.Text)), ":：") {
		case "total", "totals", "subtotal", "grand total", "summary", "sum", "合计", "总计", "小计", "汇总", "总额":
			return true
		}
	}
	return false
}

// Labels supplement type/style evidence for text-only columns. They are never
// applied to a single record, and an unknown label does not imply data.
func countColumnLabels(row []entity.TableCell) (int, int) {
	known, values := 0, 0
	for _, cell := range row {
		if cell.Covered || strings.TrimSpace(cell.Text) == "" {
			continue
		}
		values++
		if headerValueKind(cell.Text) != "text" {
			return 0, values
		}
		switch strings.ToLower(strings.TrimSpace(cell.Text)) {
		case "name", "age", "city", "date", "time", "amount", "price", "quantity", "count", "product", "item", "value", "id", "status", "description", "question", "answer", "姓名", "年龄", "地区", "日期", "金额", "数量", "名称", "编号", "问题", "回答":
			known++
		}
	}
	return known, values
}
