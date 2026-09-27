//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
//
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

package doctype

// DefaultDLALabels returns the 10-class DLA taxonomy matching Python's
// deepdoc/vision/dla_cli.py:10-21. Duplicates at indices 4, 7, 9 are
// kept verbatim for backward compatibility with existing inference servers.
//
// This list is the wire contract: the in-process detector (native)
// serialises its DLA output through these same indices, and its internal
// yoloDlaLabels must stay element-for-element identical to this (same order,
// same duplicate indices 4/7/9). The two live in separate modules, so they
// cannot share one Go constant; keep them in sync by hand.
func DefaultDLALabels() []string {
	return []string{
		LayoutTypeTitle, LayoutTypeText, LayoutTypeReference,
		LayoutTypeFigure, DLALabelFigureCaption,
		LayoutTypeTable, DLALabelTableCaption, DLALabelTableCaption,
		LayoutTypeEquation, DLALabelFigureCaption,
	}
}

// DefaultTSRLabels returns the 6-class TSR taxonomy matching Python's
// deepdoc/server/adapters/tsr_adapter.py:21-26.
func DefaultTSRLabels() []string {
	return []string{
		"table", "table column", "table row",
		"table column header", "table projected row header",
		"table spanning cell",
	}
}
