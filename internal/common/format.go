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
	"strings"
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
	for _, p := range parts {
		if p == "" {
			return false
		}
	}
	return true
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
	for _, p := range parts {
		if p == "" {
			return "", "", "", fmt.Errorf("invalid model name format")
		}
	}
	return parts[0], parts[1], parts[2], nil
}

func EncodeEmail(email string) string {
	return base64.StdEncoding.EncodeToString([]byte(email))
}

func DecodeEmail(encoded string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
