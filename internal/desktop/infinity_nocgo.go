//go:build desktop && !cgo

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

package desktop

import "errors"

// embeddedInfinity keeps the package compiling under CGO_ENABLED=0, where the
// native document engine cannot be linked. A desktop binary is built with cgo
// (build.sh --desktop), so this shape exists for tooling such as `go vet` with
// cgo disabled, not as a supported runtime.
type embeddedInfinity struct {
	addr string
}

func startEmbeddedInfinity(string) (*embeddedInfinity, error) {
	return nil, errors.New("desktop: the document engine requires cgo")
}

func (e *embeddedInfinity) Addr() string { return e.addr }

func (e *embeddedInfinity) Close() error { return nil }
