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

package chunker

import (
	"testing"

	"ragflow/internal/entity"
)

// charTokens counts runes, a cheap deterministic stand-in for a real tokenizer.
func charTokens(s string) int { return len([]rune(s)) }

func makeTable(dataRows int) *entity.TableData {
	rows := [][]string{{"Name", "Age"}} // header
	for i := 0; i < dataRows; i++ {
		rows = append(rows, []string{"person", "30"})
	}
	return &entity.TableData{Rows: rows, HeaderRows: 1}
}

func TestSplitLargeTableFitsWhole(t *testing.T) {
	td := makeTable(3)
	parts, ranges, headerRows := splitLargeTable(td, 1_000_000, charTokens)
	if ranges != nil || len(parts) != 0 {
		t.Fatalf("small table should not split: parts=%v ranges=%v", parts, ranges)
	}
	if headerRows != 0 {
		t.Fatalf("headerRows = %d, want 0 when unsplit", headerRows)
	}
}

func TestSplitLargeTableRowGranular(t *testing.T) {
	td := makeTable(20)
	// Budget bounded so a part holds ~3 data rows.
	parts, ranges, headerRows := splitLargeTable(td, 22, charTokens)
	if headerRows != 1 {
		t.Fatalf("headerRows = %d, want 1", headerRows)
	}
	if len(parts) <= 1 {
		t.Fatalf("expected multiple parts, got %d", len(parts))
	}
	if len(parts) != len(ranges) {
		t.Fatalf("parts(%d) != ranges(%d)", len(parts), len(ranges))
	}
	// Every part replicates the header and holds a whole number of rows; the
	// data ranges are contiguous and cover all 20 data rows exactly once.
	seen := 0
	for i, part := range parts {
		if part.HeaderRows != 1 {
			t.Fatalf("part %d headerRows = %d", i, part.HeaderRows)
		}
		if len(part.Rows) < 2 {
			t.Fatalf("part %d has <1 data row: %v", i, part.Rows)
		}
		if part.Rows[0][0] != "Name" {
			t.Fatalf("part %d missing header row", i)
		}
		r := ranges[i]
		if r[0] != seen {
			t.Fatalf("part %d range start %d != running %d", i, r[0], seen)
		}
		seen = r[1]
	}
	if seen != 20 {
		t.Fatalf("data rows covered = %d, want 20", seen)
	}
	// The concatenation of every part's data rows must equal the original rows.
	var rebuilt [][]string
	for _, part := range parts {
		rebuilt = append(rebuilt, part.Rows[1:]...)
	}
	for i, row := range td.Rows[1:] {
		if len(rebuilt[i]) != len(row) || rebuilt[i][0] != row[0] {
			t.Fatalf("row %d mismatch: got %v want %v", i, rebuilt[i], row)
		}
	}
}

func TestSplitLargeTableSingleRowOverBudgetKeepsRow(t *testing.T) {
	// One data row alone exceeds the budget; it must still form its own chunk
	// (row integrity wins over the budget). A single part means "not split,
	// emit whole" — the over-budget row becomes one chunk rather than being cut.
	td := &entity.TableData{
		Rows:       [][]string{{"H"}, {"x" + string(make([]byte, 200))}},
		HeaderRows: 1,
	}
	parts, ranges, _ := splitLargeTable(td, 5, charTokens)
	if ranges != nil || len(parts) != 0 {
		t.Fatalf("over-budget single row must emit whole (ranges nil): parts=%v ranges=%v", parts, ranges)
	}
}

func TestSplitLargeTableHeaderOnly(t *testing.T) {
	td := &entity.TableData{Rows: [][]string{{"Name", "Age"}}, HeaderRows: 1}
	parts, ranges, _ := splitLargeTable(td, 10, charTokens)
	if ranges != nil || len(parts) != 0 {
		t.Fatalf("header-only table should not split")
	}
}
