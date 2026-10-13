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

import (
	"bytes"
	"encoding/json"
)

// FlexibleJSONString unmarshals a JSON string, or a JSON object/array by
// keeping the compact JSON text. Provider UIs (MinerU, OpenDataLoader)
// historically POST api_key as an object while instance APIs persist it as
// TEXT; Sonic otherwise fails with "Mismatch type string with value object".
type FlexibleJSONString string

func (s FlexibleJSONString) String() string { return string(s) }

func (s *FlexibleJSONString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*s = ""
		return nil
	}
	if data[0] == '"' {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		*s = FlexibleJSONString(str)
		return nil
	}
	*s = FlexibleJSONString(data)
	return nil
}

func (s FlexibleJSONString) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

// StringSlice is a []string that unmarshals from either a JSON string or
// a JSON array of strings.  This matches Python endpoints that accept
// both "kb1" and ["kb1", "kb2"] for list-valued parameters.
type StringSlice []string

// UnmarshalJSON implements json.Unmarshaler.
func (s *StringSlice) UnmarshalJSON(data []byte) error {
	// Try array first.
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*s = arr
		return nil
	}

	// Fall back to a single string → wrap as one-element slice.
	var single string
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*s = StringSlice{single}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (s StringSlice) MarshalJSON() ([]byte, error) {
	return json.Marshal([]string(s))
}
