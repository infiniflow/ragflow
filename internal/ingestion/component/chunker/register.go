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

// Package chunker holds the ingestion chunker components: TokenChunker,
// TitleChunker, GroupTitleChunker, HierarchyTitleChunker. The four
// components share the same upstream payload (schema.ChunkerFromUpstream)
// and the same output shape (schema.ChunkerOutputs).
//
// The package is intentionally separate from internal/agent/component/
// (the agent canvas) and from internal/ingestion/component/schema/
// (the wire types). Wiring it as a separate package keeps the
// registry tidy.
package chunker

import (
	"ragflow/internal/agent/runtime"
)

// MustRegisterChunker registers a single chunker component under
// CategoryIngestion. The four chunker files each carry exactly one
// init() that calls this with the registered component's name; the
// factory body resolves the typed constructor via newChunkerByName
// (in common.go).
// One helper call per file keeps the registration surface flat.
func MustRegisterChunker(name string) {
	factory := func(_ string, params map[string]any) (runtime.Component, error) {
		comp, err := newChunkerByName(name, params)
		if err != nil {
			return nil, err
		}
		// newChunkerByName returns runtime.Component directly (each
		// NewXxxChunker constructor satisfies the interface, so no
		// intermediate type assertion is needed).
		return comp, nil
	}
	runtime.MustRegister(name, runtime.CategoryIngestion, factory, runtime.Metadata{
		Version: "1.0.0",
		Inputs:  ChunkerInputs,
		Outputs: ChunkerOutputs,
	})
}

// ChunkerInputs is the static, registered input descriptor shared
// by all four chunker variants.
var ChunkerInputs = map[string]string{
	"text":          "Plain-text input. The chunker slices this into downstream chunks.",
	"content":       "Alias for \"text\".",
	"chunks":        "Optional upstream chunk list (structured JSON form).",
	"name":          "Source document name. Required by the upstream payload convention.",
	"_created_time": "Optional upstream timestamp (RFC3339Nano, s).",
	"_elapsed_time": "Optional upstream elapsed time (s).",
}

// ChunkerOutputs is the static, registered output descriptor shared
// by all four chunker variants.
var ChunkerOutputs = map[string]string{
	"output_format": "Always \"chunks\" on success.",
	"chunks":        "list[object]: per-chunk map (text + optional meta keys).",
	"_ERROR":        "Set only on validation failure.",
}
