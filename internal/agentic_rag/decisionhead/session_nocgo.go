//go:build !cgo

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

package decisionhead

import "errors"

// Without cgo the ONNX Runtime cannot be linked, so the package has no
// in-process scorer and only a configured sidecar URL can serve decisions.
// This mirrors the *_nocgo fallbacks of the DeepDoc native backend and keeps
// the package compilable under CGO_ENABLED=0; the failure lands in open()'s
// degraded path, where sharedJudge logs once and Split/Judge keep everything.
func openInProcess(_ string) (scorer, error) {
	return nil, errors.New("decisionhead: the in-process scorer requires a cgo build; configure the sidecar URL instead")
}

func closeInProcess(_ scorer) {}
