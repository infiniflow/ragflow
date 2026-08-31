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
	"fmt"

	"ragflow/internal/common"
)

// BestEffort executes f and logs any error via Error. It never panics (nil f
// is handled) and never propagates the error — best-effort means the caller
// does not want the main flow interrupted by this failure. Use it for
// resource cleanup, soft-state deletion, and progress mirroring: operations
// whose failure must be visible (logged) but must not abort the task.
func BestEffort(label string, f func() error) {
	if f == nil {
		common.Error(fmt.Sprintf("best-effort %s: nil function", label), fmt.Errorf("nil function"))
		return
	}
	if err := f(); err != nil {
		common.Error(fmt.Sprintf("best-effort %s failed: %v", label, err), err)
	}
}
