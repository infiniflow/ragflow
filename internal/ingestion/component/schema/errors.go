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

package schema

import "fmt"

// errRequiredField is the typed error returned by schema Validate()
// methods when a required field is missing or empty. It carries the
// field name so callers can produce structured error responses.
type errRequiredField struct {
	Field string
}

func (e errRequiredField) Error() string {
	return fmt.Sprintf("schema: required field %q is missing or empty", e.Field)
}

// errInvalidValue is the typed error returned by schema Validate()
// methods when a field's value is not in the allowed set. It carries
// the field name and the offending value.
type errInvalidValue struct {
	Field string
	Value string
}

func (e errInvalidValue) Error() string {
	return fmt.Sprintf("schema: field %q has invalid value %q", e.Field, e.Value)
}
