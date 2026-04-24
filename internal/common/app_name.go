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
	"strings"

	"github.com/google/uuid"
)

// splitNameCounter splits a filename into base name and counter
// Handles names in format "filename(123)" pattern
//
// Parameters:
//   - filename: The filename to split
//
// Returns:
//   - string: The base name without counter
//   - *int: The counter value, or nil if no counter exists
//
// Example:
//
//	splitNameCounter("test(5)") returns ("test", 5)
//	splitNameCounter("test") returns ("test", nil)
func splitNameCounter(filename string) (string, *int) {
	re := regexp.MustCompile(`^(.+)\((\d+)\)$`)
	matches := re.FindStringSubmatch(filename)
	if len(matches) >= 3 {
		counter := -1
		fmt.Sscanf(matches[2], "%d", &counter)
		stem := strings.TrimRight(matches[1], " ")
		return stem, &counter
	}
	return filename, nil
}

// DuplicateName generates a unique name by appending a counter if the name already exists
// It tries up to 1000 times to generate a unique name
//
// Parameters:
//   - queryFunc: Function to check if a name already exists (returns true if exists)
//   - name: The original name
//   - tenantID: The tenant ID for name uniqueness check
//
// Returns:
//   - string: A unique name (either original or with counter appended)
//
// Example:
//
//	DuplicateName(func(name string, tid string) bool { return false }, "test", "tenant1") returns "test"
//	DuplicateName(func(name string, tid string) bool { return true }, "test", "tenant1") returns "test(1)"
func DuplicateName(queryFunc func(name string, tenantID string) bool, name string, tenantID string) (string, error) {
	const maxRetries = 1000

	originalName := name
	currentName := name
	retries := 0

	for retries < maxRetries {
		if !queryFunc(currentName, tenantID) {
			return currentName, nil
		}

		stem, counter := splitNameCounter(currentName)
		ext := path.Ext(stem)
		stemBase := strings.TrimSuffix(stem, ext)

		newCounter := 1
		if counter != nil {
			newCounter = *counter + 1
		}

		currentName = fmt.Sprintf("%s(%d)%s", stemBase, newCounter, ext)
		retries++

		if err := ValidateName(currentName); err != nil {
			return "", err
		}
	}

	return "", fmt.Errorf("failed to generate unique name after %d attempts, conflict name: %s", maxRetries, originalName)
}

const AppNameLimit = 256

func ValidateName(name string) error {
	// Validate name is not empty after trimming
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return fmt.Errorf("name can't be empty")
	}

	// Validate name length in bytes (not characters) - same as Python len(search_name.encode("utf-8"))
	if len([]byte(name)) > AppNameLimit {
		return fmt.Errorf("name length is %d which is large than %d", len([]byte(name)), AppNameLimit)
	}

	return nil
}

// GenerateUUID generates a UUID without dashes
func GenerateUUID() string {
	newID := strings.ReplaceAll(uuid.New().String(), "-", "")
	if len(newID) > 32 {
		newID = newID[:32]
	}
	return newID
}
