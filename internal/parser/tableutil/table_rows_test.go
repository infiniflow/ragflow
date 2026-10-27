//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package tableutil

import (
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

func TestResolveRowsHeaderOriginSpan(t *testing.T) {
	td := &entity.TableData{
		HeaderRows: 1,
		Grid: [][]entity.TableCell{
			{{Text: "H", RowSpan: 2}, {Text: "Name"}},
			{{Covered: true}, {Text: "Alice"}},
		},
	}
	table := ResolveRows(td, RowsTable)
	if want := [][]string{{"H", "Name"}, {"", "Alice"}}; !reflect.DeepEqual(table, want) {
		t.Fatalf("table view = %v, want %v", table, want)
	}
	data := ResolveRows(td, RowsData)
	if want := [][]string{{"H", "Name"}, {"", "Alice"}}; !reflect.DeepEqual(data, want) {
		t.Fatalf("data view = %v, want %v", data, want)
	}
	qa := ResolveRows(td, RowsQA)
	if want := [][]string{{"H", "Name"}, {"H", "Alice"}}; !reflect.DeepEqual(qa, want) {
		t.Fatalf("QA view = %v, want %v", qa, want)
	}
}

func TestResolveRowsDataOriginSpan(t *testing.T) {
	td := &entity.TableData{
		HeaderRows: 1,
		Grid: [][]entity.TableCell{
			{{Text: "Group"}, {Text: "Item"}},
			{{Text: "G1", RowSpan: 2}, {Text: "x"}},
			{{Covered: true}, {Text: "y"}},
		},
	}
	want := [][]string{{"Group", "Item"}, {"G1", "x"}, {"G1", "y"}}
	if got := ResolveRows(td, RowsTable); !reflect.DeepEqual(got, want) {
		t.Fatalf("table view = %v, want %v", got, want)
	}
	if got := ResolveRows(td, RowsQA); !reflect.DeepEqual(got, want) {
		t.Fatalf("QA view = %v, want %v", got, want)
	}
}

func TestResolveRowsQAKeepsColspanSingle(t *testing.T) {
	td := &entity.TableData{
		HeaderRows: 1,
		Grid: [][]entity.TableCell{
			{{Text: "q", ColSpan: 2}, {Covered: true}, {Text: "a"}},
		},
	}
	if got, want := ResolveRows(td, RowsTable), [][]string{{"q", "q", "a"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("table view = %v, want %v", got, want)
	}
	if got, want := ResolveRows(td, RowsQA), [][]string{{"q", "", "a"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("QA view = %v, want %v", got, want)
	}
}

func TestResolveRowsRowColSpanBlock(t *testing.T) {
	td := &entity.TableData{
		HeaderRows: 1,
		Grid: [][]entity.TableCell{
			{{Text: "Name"}, {Text: "B"}, {Text: "C"}},
			{{Text: "Wide", RowSpan: 2, ColSpan: 2}, {Covered: true}, {Text: "x"}},
			{{Covered: true}, {Covered: true}, {Text: "y"}},
		},
	}
	tableWant := [][]string{
		{"Name", "B", "C"},
		{"Wide", "Wide", "x"},
		{"Wide", "Wide", "y"},
	}
	if got := ResolveRows(td, RowsTable); !reflect.DeepEqual(got, tableWant) {
		t.Fatalf("table view = %v, want %v", got, tableWant)
	}
	qaWant := [][]string{
		{"Name", "B", "C"},
		{"Wide", "", "x"},
		{"Wide", "", "y"},
	}
	if got := ResolveRows(td, RowsQA); !reflect.DeepEqual(got, qaWant) {
		t.Fatalf("QA view = %v, want %v", got, qaWant)
	}
	if got := ResolveRows(td, RowsData); !reflect.DeepEqual(got, qaWant) {
		t.Fatalf("data view = %v, want %v", got, qaWant)
	}
}
