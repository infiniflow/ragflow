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
	"os"
	"time"
)

// GetProjectBaseDirectory returns the current working directory.
// If an error occurs while getting the current directory, it returns ".".
//
// Returns:
//   - string: The current working directory path, or "." if an error occurs.
//
// Example:
//
//	baseDir := utility.GetProjectBaseDirectory()
//	configPath := filepath.Join(baseDir, "conf", "config.json")
func GetProjectBaseDirectory() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// StringPtr converts a string to a pointer of string.
// If the input string is empty, it returns nil.
//
// Parameters:
//   - s: The string to convert to a pointer.
//
// Returns:
//   - *string: A pointer to the input string, or nil if the input is empty.
//
// Example:
//
//	name := utility.StringPtr("example")  // returns &"example"
//	empty := utility.StringPtr("")        // returns nil
func StringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ParseInt64 parses a string to int64.
// If parsing fails, it returns 0.
//
// Parameters:
//   - s: The string to parse.
//
// Returns:
//   - int64: The parsed integer value, or 0 if parsing fails.
//
// Example:
//
//	val := utility.ParseInt64("123")   // returns 123
//	val := utility.ParseInt64("abc")   // returns 0
//	val := utility.ParseInt64("")      // returns 0
func ParseInt64(s string) int64 {
	var result int64
	fmt.Sscanf(s, "%d", &result)
	return result
}

// FormatTime formats time for display
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return "N/A (Perpetual)"
	}
	return t.Format("2006-01-02 15:04:05")
}
