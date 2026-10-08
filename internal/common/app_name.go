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
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// maxNameRetries bounds how many numbered suffixes UniqueName may try before
// giving up on a free name.
const maxNameRetries = 1000

// splitNameCounter splits a name stem (already stripped of its file extension)
// into its base and a trailing "(N)" counter. The boolean reports whether a
// trailing counter was present.
//
// Example:
//
//	splitNameCounter("test(5)") returns ("test", 5, true)
//	splitNameCounter("test")    returns ("test", 0, false)
func splitNameCounter(stem string) (string, int, bool) {
	re := regexp.MustCompile(`^(.*)\((\d+)\)$`)
	matches := re.FindStringSubmatch(stem)
	if matches == nil {
		return stem, 0, false
	}
	counter, err := strconv.Atoi(matches[2])
	if err != nil {
		return stem, 0, false
	}
	return strings.TrimRight(matches[1], " "), counter, true
}

// UniqueName returns a non-colliding variant of name. When the name already
// exists it appends "(1)", "(2)", ... It continues from an existing trailing
// counter and preserves the file extension:
//
//	UniqueName("topic", ...)           -> "topic(1)"
//	UniqueName("topic(1)", ...)        -> "topic(2)"
//	UniqueName("report.pdf", ...)      -> "report(1).pdf"
//	UniqueName("report(1).pdf", ...)   -> "report(2).pdf"
//
// exists reports whether a candidate already collides. A lookup error is
// returned to the caller instead of being treated as "free".
func UniqueName(name string, exists func(candidate string) (bool, error)) (string, error) {
	current := name
	for i := 0; i < maxNameRetries; i++ {
		taken, err := exists(current)
		if err != nil {
			return "", err
		}
		if !taken {
			return current, nil
		}

		ext := path.Ext(current)
		stem := strings.TrimSuffix(current, ext)
		base, counter, ok := splitNameCounter(stem)
		if !ok {
			base = stem
		}
		current = fmt.Sprintf("%s(%d)%s", base, counter+1, ext)
		if err := ValidateName(current); err != nil {
			return "", err
		}
	}

	return "", fmt.Errorf("failed to generate a unique name after %d attempts, conflict name: %s", maxNameRetries, name)
}

// NameAvailable reports whether newName is free to use for a rename.
//
// A case-only change (strings.EqualFold) is always allowed and skips the
// existence check, matching the dataset rename rule. Otherwise the caller's
// case-insensitive existence check decides.
//
// It returns (true, nil) when the name can be used, (false, nil) when it is
// already taken, and (false, err) when the existence lookup fails.
func NameAvailable(currentName, newName string, exists func(candidate string) (bool, error)) (bool, error) {
	if strings.EqualFold(currentName, newName) {
		return true, nil
	}
	taken, err := exists(newName)
	if err != nil {
		return false, err
	}
	return !taken, nil
}

// AppNameLimit is the maximum allowed length of a name, in bytes.
const AppNameLimit = 256

// ValidateName rejects empty names and names longer than AppNameLimit bytes.
func ValidateName(name string) error {
	// Validate name is not empty after trimming
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return fmt.Errorf("name cannot be empty or whitespace")
	}

	// Validate name length in bytes (not characters) - same as Python len(search_name.encode("utf-8"))
	if len([]byte(name)) > AppNameLimit {
		return fmt.Errorf("name length is %d which is large than %d", len([]byte(name)), AppNameLimit)
	}

	return nil
}
