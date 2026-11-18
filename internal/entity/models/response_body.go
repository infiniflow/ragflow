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

package models

import (
	"fmt"
	"io"
)

const (
	// maxModelResponseBodyBytes caps JSON/text provider responses that should stay modest.
	maxModelResponseBodyBytes int64 = 16 << 20
	// maxModelErrorBodyBytes caps provider error pages before they are included in errors.
	maxModelErrorBodyBytes int64 = 1 << 20
)

func readModelResponseBodyLimited(body io.Reader, maxBytes int64) ([]byte, error) {
	if body == nil {
		return nil, fmt.Errorf("response body is nil")
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("response body limit must be positive")
	}

	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxBytes)
	}
	return data, nil
}

func readModelResponseBody(body io.Reader) ([]byte, error) {
	return readModelResponseBodyLimited(body, maxModelResponseBodyBytes)
}

func readModelErrorBody(body io.Reader) ([]byte, error) {
	return readModelResponseBodyLimited(body, maxModelErrorBodyBytes)
}
