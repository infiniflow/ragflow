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

package dao

import (
	"errors"
	"testing"
)

func TestMCPServerOrderColumnInvalidField(t *testing.T) {
	_, err := mcpServerOrderColumn("bad_field")
	if err == nil {
		t.Fatal("expected invalid orderby error")
	}

	var orderbyErr *InvalidMCPServerOrderByError
	if !errors.As(err, &orderbyErr) {
		t.Fatalf("expected InvalidMCPServerOrderByError, got %T", err)
	}

	want := `AttributeError("type object 'MCPServer' has no attribute 'bad_field'")`
	if err.Error() != want {
		t.Fatalf("expected %q, got %q", want, err.Error())
	}
}
