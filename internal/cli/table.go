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

package cli

import (
	"fmt"
	"strings"
	"unicode"
)

// PrintTableSimple prints data in a simple table format
// Similar to Python's _print_table_simple
func PrintTableSimple(data []map[string]interface{}) {
	if len(data) == 0 {
		fmt.Println("No data to print")
		return
	}

	// Collect all column names
	columnSet := make(map[string]bool)
	for _, item := range data {
		for key := range item {
			columnSet[key] = true
		}
	}

	// Sort columns
	columns := make([]string, 0, len(columnSet))
	for col := range columnSet {
		columns = append(columns, col)
	}
	// Simple sort - in production you might want specific column ordering
	for i := 0; i < len(columns); i++ {
		for j := i + 1; j < len(columns); j++ {
			if columns[i] > columns[j] {
				columns[i], columns[j] = columns[j], columns[i]
			}
		}
	}

	// Calculate column widths
	colWidths := make(map[string]int)
	for _, col := range columns {
		maxWidth := getStringWidth(col)
		for _, item := range data {
			value := fmt.Sprintf("%v", item[col])
			valueWidth := getStringWidth(value)
			if valueWidth > maxWidth {
				maxWidth = valueWidth
			}
		}
		if maxWidth < 2 {
			maxWidth = 2
		}
		colWidths[col] = maxWidth
	}

	// Generate separator
	separatorParts := make([]string, 0, len(columns))
	for _, col := range columns {
		separatorParts = append(separatorParts, strings.Repeat("-", colWidths[col]+2))
	}
	separator := "+" + strings.Join(separatorParts, "+") + "+"

	// Print header
	fmt.Println(separator)
	headerParts := make([]string, 0, len(columns))
	for _, col := range columns {
		headerParts = append(headerParts, fmt.Sprintf(" %-*s ", colWidths[col], col))
	}
	fmt.Println("|" + strings.Join(headerParts, "|") + "|")
	fmt.Println(separator)

	// Print data rows
	for _, item := range data {
		rowParts := make([]string, 0, len(columns))
		for _, col := range columns {
			value := fmt.Sprintf("%v", item[col])
			valueWidth := getStringWidth(value)
			// Truncate if too long
			if valueWidth > colWidths[col] {
				runes := []rune(value)
				truncated := truncateString(runes, colWidths[col])
				value = truncated
				valueWidth = getStringWidth(value)
			}
			// Pad to column width
			padding := colWidths[col] - valueWidth + len(value)
			rowParts = append(rowParts, fmt.Sprintf(" %-*s ", padding, value))
		}
		fmt.Println("|" + strings.Join(rowParts, "|") + "|")
	}

	fmt.Println(separator)
}

// getStringWidth calculates the display width of a string
// Treats CJK characters as width 2
func getStringWidth(text string) int {
	width := 0
	for _, r := range text {
		if isHalfWidth(r) {
			width++
		} else {
			width += 2
		}
	}
	return width
}

// isHalfWidth checks if a rune is half-width
func isHalfWidth(r rune) bool {
	// ASCII printable characters and common whitespace
	if r >= 0x20 && r <= 0x7E {
		return true
	}
	if r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	return false
}

// truncateString truncates a string to fit within maxWidth display width
func truncateString(runes []rune, maxWidth int) string {
	width := 0
	for i, r := range runes {
		if isHalfWidth(r) {
			width++
		} else {
			width += 2
		}
		if width > maxWidth-3 {
			return string(runes[:i]) + "..."
		}
	}
	return string(runes)
}

// getMax returns the maximum of two integers
func getMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// isWideChar checks if a character is wide (CJK, etc.)
func isWideChar(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}
