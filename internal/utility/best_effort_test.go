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

package utility

import (
	"errors"
	"testing"
)

func TestBestEffort_NoPanicOnSuccess(t *testing.T) {
	// Must not panic when f returns nil.
	BestEffort("test-success", func() error { return nil })
}

func TestBestEffort_NoPanicOnError(t *testing.T) {
	// Must not panic when f returns an error — it should be logged, not
	// propagated.
	BestEffort("test-fail", func() error { return errors.New("boom") })
}

func TestBestEffort_NoPanicOnNilFunc(t *testing.T) {
	// Must not panic when f is nil.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("BestEffort with nil func panicked: %v", r)
		}
	}()
	BestEffort("test-nil", nil)
}
