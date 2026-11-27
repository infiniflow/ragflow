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

//go:build cgo

package native

import "fmt"

// ModelInput pairs inference input data with its tensor shape. The shape is
// carried explicitly (not reconstructed from len(Data)) because distinct
// shapes can have identical element counts (e.g. 960x640 vs 640x960).
type ModelInput struct {
	Data  []float32
	Shape []int64
}

// validate checks that Shape is well-formed and consistent with Data length.
func (in ModelInput) validate() error {
	if len(in.Shape) == 0 {
		return fmt.Errorf("ModelInput: empty shape")
	}
	var count int64 = 1
	for _, d := range in.Shape {
		if d <= 0 {
			return fmt.Errorf("ModelInput: non-positive dimension %d in shape %v", d, in.Shape)
		}
		count *= d
	}
	if int64(len(in.Data)) != count {
		return fmt.Errorf("ModelInput: data len %d != shape product %d (%v)",
			len(in.Data), count, in.Shape)
	}
	return nil
}
