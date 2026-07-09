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
	"encoding/base64"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// PtrString formats a pointer value as a string for debug/log output.
// Returns "<nil>" for nil pointers.
func PtrString[T any](p *T) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", *p)
}

// composite model name format: model_name@instance_name@provider_name
func IsCompositeModelName(modelName string) bool {
	parts := strings.Split(modelName, "@")
	if len(parts) != 3 {
		return false
	}
	return !slices.Contains(parts, "")
}

func IsUUID(uuid string) bool {
	// only lower case letters and numbers, length is 32
	if len(uuid) != 32 {
		return false
	}
	uuidRegex := regexp.MustCompile(`^[a-z0-9]+$`)
	if uuidRegex.MatchString(uuid) {
		return true
	}
	return false
}

// ExtractCompositeName splits a composite model name into three parts.
// Returns (modelName, instanceName, providerName, true) on success,
// or ("", "", "", false) if the name is not a valid composite name.
func ExtractCompositeName(modelName string) (string, string, string, error) {
	parts := strings.Split(modelName, "@")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid model name format")
	}
	if slices.Contains(parts, "") {
		return "", "", "", fmt.Errorf("invalid model name format")
	}
	return parts[0], parts[1], parts[2], nil
}

func EncodeToBase64(email string) string {
	return base64.StdEncoding.EncodeToString([]byte(email))
}

func DecodeFromBase64(encoded string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)
	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func FormatNumber(n int64) string {
	s := fmt.Sprintf("%d", n)
	parts := []string{}
	for i := len(s); i > 0; i -= 3 {
		start := i - 3
		if start < 0 {
			start = 0
		}
		parts = append([]string{s[start:i]}, parts...)
	}
	return strings.Join(parts, ",")
}

func ParseBytesString(s string) int64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "-" || s == "0" {
		return 0
	}

	var multiplier int64 = 1
	switch {
	case strings.HasSuffix(s, "tb"):
		multiplier = 1024 * 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "tb")
	case strings.HasSuffix(s, "gb"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "gb")
	case strings.HasSuffix(s, "mb"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "mb")
	case strings.HasSuffix(s, "kb"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "kb")
	case strings.HasSuffix(s, "b"):
		s = strings.TrimSuffix(s, "b")
	}

	s = strings.TrimSpace(s)
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(val * float64(multiplier))
}

func FormatTime(t *int64) string {
	if t == nil {
		return "N/A"
	}
	return time.UnixMilli(*t).Format("2006-01-02 15:04:05")
}

func IsValidString(v interface{}) bool {
	str, ok := v.(string)
	return ok && str != ""
}
