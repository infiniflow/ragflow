//go:build !cgo

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

package deepdoc

import (
	"errors"

	"ragflow/internal/server/bootstrap"
)

// ErrDeepDocCGORequired is returned by RegisterNativeDeepDoc when the binary was built without cgo.
var ErrDeepDocCGORequired = errors.New("deepdoc: in-process DeepDoc backend requires cgo (rebuild with CGO_ENABLED=1 and -tags cgo)")

// RegisterNativeDeepDoc fails fast without cgo because the in-process ONNX Runtime backend cannot be linked.
func RegisterNativeDeepDoc(_ *bootstrap.ServerArgs) error {
	return ErrDeepDocCGORequired
}
