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

import "strings"

// GetFileExtension extracts the lowercase file extension from a filename
func GetFileExtension(filename string) string {
	idx := -1
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			idx = i
			break
		}
		if filename[i] == '/' || filename[i] == '\\' {
			break
		}
	}
	if idx == -1 || idx == len(filename)-1 {
		return ""
	}
	return strings.ToLower(filename[idx:])
}
