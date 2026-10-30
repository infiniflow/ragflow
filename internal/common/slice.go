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

// SlicePage returns a shallow copy of s bounded to the requested page window.
// A page <= 0 or pageSize <= 0 returns s unchanged (caller did not ask to page).
// Out-of-range pages return nil; the high bound is clamped so a page that
// straddles the end never exposes elements past len(s).
//
// The page bound is checked arithmetically before computing the offset so an
// unbounded positive page/page_size can never overflow to a negative start and
// panic (review Critical).
func SlicePage[T any](s []T, page, pageSize int) []T {
	if page <= 0 || pageSize <= 0 || len(s) == 0 {
		return s
	}
	// page-1 must be <= (len(s)-1)/pageSize, i.e. start must be < len(s).
	if page-1 > (len(s)-1)/pageSize {
		return nil
	}
	start := (page - 1) * pageSize
	if pageSize >= len(s)-start {
		return s[start:]
	}
	return s[start : start+pageSize]
}
