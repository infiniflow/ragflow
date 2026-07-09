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

package task

// AddPositions adds position fields to a chunk map.
// Input positions is a flat []float64 grouped as [pn, left, right, top, bottom]
// every 5 elements. pn is 0-indexed; output is 1-indexed.
//
// Mirrors Python: rag.nlp.add_positions()
//   for pn, left, right, top, bottom in poss:
//       page_num_int.append(int(pn + 1))
//       top_int.append(int(top))
//       position_int.append((int(pn + 1), int(left), int(right), int(top), int(bottom)))
func AddPositions(chunk map[string]any, positions []float64) {
	if len(positions) == 0 || len(positions)%5 != 0 {
		return
	}
	n := len(positions) / 5
	pageNumInt := make([]int, 0, n)
	topInt := make([]int, 0, n)
	positionInt := make([][]int, 0, n)

	for i := 0; i < len(positions); i += 5 {
		pn := int(positions[i]) + 1 // 0-indexed → 1-indexed
		left := int(positions[i+1])
		right := int(positions[i+2])
		top := int(positions[i+3])
		bottom := int(positions[i+4])

		pageNumInt = append(pageNumInt, pn)
		topInt = append(topInt, top)
		positionInt = append(positionInt, []int{pn, left, right, top, bottom})
	}

	chunk["page_num_int"] = pageNumInt
	chunk["top_int"] = topInt
	chunk["position_int"] = positionInt
}
