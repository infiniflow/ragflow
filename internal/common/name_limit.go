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
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Name limits are Unicode code points, matching VARCHAR(n) on MySQL and
// PostgreSQL. Callers pass the width of the column they are about to write.
const (
	NameLimit128 = 128
	NameLimit255 = 255

	NicknameLimit   = 100
	TenantNameLimit = 128

	PipelineTitleLimit = 255
	VersionTitleLimit  = 255

	// VersionTitleTimeLayout is the timestamp suffix buildVersionTitle appends
	// to a canvas title. VersionBaseTitleLimit leaves room for that suffix.
	VersionTitleTimeLayout = "2006-01-02 15:04:05"
	VersionBaseTitleLimit  = VersionTitleLimit - (1 + len(VersionTitleTimeLayout))
)

// ValidateNameRunes rejects empty names and names longer than max Unicode
// code points. It mirrors the character semantics of a VARCHAR(max) column.
func ValidateNameRunes(name string, max int) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("name cannot be empty or whitespace")
	}
	if length := utf8.RuneCountInString(name); length > max {
		return fmt.Errorf("name length is %d which is larger than %d", length, max)
	}
	return nil
}

// ValidateAliasRunes validates an optional alias. An empty alias means "no
// alias" and is valid; a non-empty alias must fit max Unicode code points.
func ValidateAliasRunes(alias string, max int) error {
	if strings.TrimSpace(alias) == "" {
		return nil
	}
	return ValidateNameRunes(alias, max)
}
