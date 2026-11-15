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

import "testing"

// sharedJudge turns a failed open() into a one-line log and a disabled judge,
// so the !cgo fallback must fail CLOSED: a (nil, nil) return would install a
// scorer-less judge and the first forward pass would nil-deref.
func TestOpenInProcessFailsClosedWithoutCgo(t *testing.T) {
	scorer, err := openInProcess("unused-model-path")
	if err == nil {
		t.Fatal("openInProcess must return an error without cgo, got nil")
	}
	if scorer != nil {
		t.Fatal("openInProcess must not return a scorer without cgo")
	}
}
