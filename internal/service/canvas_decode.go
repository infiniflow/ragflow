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

package service

import (
	"fmt"

	"ragflow/internal/agent/canvas"
)

// decodeCanvasFromDSL converts the DSL map (in either IMPORT shape
// or NormalizeForCanvas output shape) into a *canvas.Canvas that
// canvas.Compile accepts.
//
// Two accepted input shapes:
//
//  1. IMPORT shape (top-level "obj.component_name" + "obj.params"
//     + outer "downstream" / "upstream"). The Python-era DSL
//     convention; some legacy v1 fixtures still use it directly.
//
//  2. Normalized shape (top-level "name" + "params" + "downstream" /
//     "upstream"). The output of dslpkg.NormalizeForCanvas, which
//     is what service.normalisedDSLForRun currently feeds into
//     buildRunFunc (gap analysis §11.7.4 V2 follow-up chain).
//
// The canvas.Canvas struct itself uses IMPORT shape
// (CanvasComponentObj.ComponentName with json tag
// "component_name"). normalize.go's buildGraphFromComponents
// flattens the components map to the normalized shape so the
// React-Flow editor gets a stable byte-equal layout; the runtime
// then has to walk both shapes here.
//
// All non-sentinel failures wrap ErrAgentStorageError so the
// handler's mapAgentError classifies them as CodeServerError
// (500) with a sanitized message — the raw decoder error string
// never reaches the client.
//
// Decoder strategy: direct map walking, NOT JSON round-trip. The
// Phase 4.4 V2 plan §4.3 originally specified JSON round-trip
// for the IMPORT shape, but the production path goes through
// NormalizeForCanvas first (normalized shape), and round-tripping
// the normalized shape through JSON loses the `name` →
// `obj.component_name` mapping (json.Unmarshal into Canvas does
// not coerce flat keys into nested `obj`). Direct map walking
// handles both shapes without that hazard.
func decodeCanvasFromDSL(dsl map[string]any) (*canvas.Canvas, error) {
	c, err := canvas.DecodeFromDSL(dsl)
	if err != nil {
		return nil, fmt.Errorf("decode canvas: %w: %w", err, ErrAgentStorageError)
	}
	return c, nil
}
