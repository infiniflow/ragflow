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

//go:build !cgo

package office

import "errors"

// ExtractRawBlocks is not available without cgo because the underlying
// office_oxide library requires CGo.  Rebuild with CGO_ENABLED=1.
func ExtractRawBlocks(_ []byte) ([]RawBlock, error) {
	return nil, errors.New("office_oxide requires cgo; rebuild with CGO_ENABLED=1")
}
