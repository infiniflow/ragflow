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

package common

import "testing"

// TestStatusDialogValid_Constant pins the value of the dialog valid
// status sentinel. Changing this constant is a wire-contract change —
// it must always equal the Python StatusEnum.VALID.value at
// api/common/constants.py (the literal string "1"). All
// chatbot/agentbot authorization paths depend on this value matching
// the on-disk dialog row.
func TestStatusDialogValid_Constant(t *testing.T) {
	if StatusDialogValid != "1" {
		t.Errorf("StatusDialogValid = %q, want %q", StatusDialogValid, "1")
	}
}

// TestDialogStatus_Type pins the typed alias so future code can use it
// instead of raw string comparisons.
func TestDialogStatus_Type(t *testing.T) {
	var s DialogStatus = StatusDialogValid
	if string(s) != "1" {
		t.Errorf("DialogStatus = %q, want %q", string(s), "1")
	}
}
