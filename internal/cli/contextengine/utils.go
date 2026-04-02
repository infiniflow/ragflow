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

package contextengine

import (
	"encoding/json"
	"fmt"
	"time"
)

// FormatNode formats a node for display
func FormatNode(node *Node, format string) map[string]interface{} {
	switch format {
	case "json":
		return map[string]interface{}{
			"name":       node.Name,
			"path":       node.Path,
			"type":       string(node.Type),
			"size":       node.Size,
			"created_at": node.CreatedAt.Format(time.RFC3339),
			"updated_at": node.UpdatedAt.Format(time.RFC3339),
		}
	case "table":
		return map[string]interface{}{
			"name": node.Name,
			"path": node.Path,
			"type": string(node.Type),
			"size": formatSize(node.Size),
			"created_at": formatTime(node.CreatedAt),
			"updated_at": formatTime(node.UpdatedAt),
		}
	default: // "plain"
		return map[string]interface{}{
			"name":       node.Name,
			"path":       node.Path,
			"type":       string(node.Type),
			"created_at": formatTime(node.CreatedAt),
			"updated_at": formatTime(node.UpdatedAt),
		}
	}
}

// FormatNodes formats a list of nodes for display
func FormatNodes(nodes []*Node, format string) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, FormatNode(node, format))
	}
	return result
}

// formatSize formats a size in bytes to human-readable format
func formatSize(size int64) string {
	if size == 0 {
		return "-"
	}

	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case size >= TB:
		return fmt.Sprintf("%.2f TB", float64(size)/TB)
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d B", size)
	}
}

// formatTime formats a time to a readable string
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04:05")
}

// ResultToMap converts a Result to a map for JSON serialization
func ResultToMap(result *Result) map[string]interface{} {
	if result == nil {
		return map[string]interface{}{
			"nodes": []interface{}{},
			"total": 0,
		}
	}

	nodes := make([]map[string]interface{}, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		nodes = append(nodes, nodeToMap(node))
	}

	return map[string]interface{}{
		"nodes":       nodes,
		"total":       result.Total,
		"has_more":    result.HasMore,
		"next_offset": result.NextOffset,
	}
}

// nodeToMap converts a Node to a map
func nodeToMap(node *Node) map[string]interface{} {
	m := map[string]interface{}{
		"name": node.Name,
		"path": node.Path,
		"type": string(node.Type),
	}

	if node.Size > 0 {
		m["size"] = node.Size
	}

	if !node.CreatedAt.IsZero() {
		m["created_at"] = node.CreatedAt.Format(time.RFC3339)
	}

	if !node.UpdatedAt.IsZero() {
		m["updated_at"] = node.UpdatedAt.Format(time.RFC3339)
	}

	if len(node.Metadata) > 0 {
		m["metadata"] = node.Metadata
	}

	return m
}

// MarshalJSON marshals a Result to JSON bytes
func (r *Result) MarshalJSON() ([]byte, error) {
	return json.Marshal(ResultToMap(r))
}

// PrintResult prints a result in the specified format
func PrintResult(result *Result, format string) {
	if result == nil {
		fmt.Println("No results")
		return
	}

	switch format {
	case "json":
		data, _ := json.MarshalIndent(ResultToMap(result), "", "  ")
		fmt.Println(string(data))
	case "table":
		printTable(result.Nodes)
	default: // "plain"
		for _, node := range result.Nodes {
			fmt.Println(node.Path)
		}
	}
}

// printTable prints nodes in a simple table format
func printTable(nodes []*Node) {
	if len(nodes) == 0 {
		fmt.Println("No results")
		return
	}

	// Print header
	fmt.Printf("%-40s %-12s %-12s %-20s %-20s\n", "NAME", "TYPE", "SIZE", "CREATED", "UPDATED")
	fmt.Println(string(make([]byte, 104)))

	// Print rows
	for _, node := range nodes {
		fmt.Printf("%-40s %-12s %-12s %-20s %-20s\n",
			truncateString(node.Name, 40),
			node.Type,
			formatSize(node.Size),
			formatTime(node.CreatedAt),
			formatTime(node.UpdatedAt),
		)
	}
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// IsValidPath checks if a path is valid
func IsValidPath(path string) bool {
	if path == "" {
		return false
	}

	// Check for invalid characters
	invalidChars := []string{"..", "//", "\\", "*", "?", "<", ">", "|", "\x00"}
	for _, char := range invalidChars {
		if containsString(path, char) {
			return false
		}
	}

	return true
}

// containsString checks if a string contains a substring
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// JoinPath joins path components
func JoinPath(components ...string) string {
	if len(components) == 0 {
		return ""
	}

	result := components[0]
	for i := 1; i < len(components); i++ {
		if result == "" {
			result = components[i]
		} else if components[i] == "" {
			continue
		} else {
			// Remove trailing slash from result
			for len(result) > 0 && result[len(result)-1] == '/' {
				result = result[:len(result)-1]
			}
			// Remove leading slash from component
			start := 0
			for start < len(components[i]) && components[i][start] == '/' {
				start++
			}
			result = result + "/" + components[i][start:]
		}
	}

	return result
}

// GetParentPath returns the parent path of a given path
func GetParentPath(path string) string {
	path = normalizePath(path)
	parts := SplitPath(path)

	if len(parts) <= 1 {
		return ""
	}

	return joinStrings(parts[:len(parts)-1], "/")
}

// GetBaseName returns the last component of a path
func GetBaseName(path string) string {
	path = normalizePath(path)
	parts := SplitPath(path)

	if len(parts) == 0 {
		return ""
	}

	return parts[len(parts)-1]
}

// HasPrefix checks if a path has the given prefix
func HasPrefix(path, prefix string) bool {
	path = normalizePath(path)
	prefix = normalizePath(prefix)

	if prefix == "" {
		return true
	}

	if path == prefix {
		return true
	}

	if len(path) > len(prefix) && path[:len(prefix)+1] == prefix+"/" {
		return true
	}

	return false
}
