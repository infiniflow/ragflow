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

// state_serializer.go implements eino's compose.Serializer interface for
// CanvasState. See plan §2.6 — the eino Serializer signature is
// Marshal(v any) / Unmarshal(data []byte, v any) with NO context.Context.
package canvas

import (
	"encoding/json"
)

// CanvasStateSerializer marshals a *CanvasState (or any value) to/from
// JSON. eino calls this when persisting or restoring a checkpoint;
// the value type is *CanvasState in the canvas engine.
type CanvasStateSerializer struct{}

// Marshal implements compose.Serializer.
func (CanvasStateSerializer) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal implements compose.Serializer. The caller passes a pointer
// (eino provides a fresh *checkpoint-like value).
func (CanvasStateSerializer) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
