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

package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

var codeExecSystemOutputKeys = map[string]struct{}{
	"content":             {},
	"actual_type":         {},
	"attachments":         {},
	"_ERROR":              {},
	"_ARTIFACTS":          {},
	"_ATTACHMENT_CONTENT": {},
	"raw_result":          {},
	"_created_time":       {},
	"_elapsed_time":       {},
}

type CodeExecContract struct {
	BusinessOutput string
	Value          any
	ActualType     string
	Content        string
}

func BuildCodeExecContract(outputs map[string]any, rawResult any) (*CodeExecContract, error) {
	businessName, businessMeta, err := selectCodeExecBusinessOutput(outputs)
	if err != nil {
		return nil, err
	}

	normalizedValue := NormalizeCodeExecOutputValue(rawResult)
	if err := validateCodeExecTopLevelValueDomain(normalizedValue); err != nil {
		return nil, err
	}

	expectedType := ""
	if meta, ok := businessMeta.(map[string]any); ok {
		expectedType = strings.TrimSpace(fmt.Sprint(meta["type"]))
	}
	if err := validateCodeExecExpectedType(expectedType, normalizedValue, ""); err != nil {
		return nil, err
	}

	return &CodeExecContract{
		BusinessOutput: businessName,
		Value:          normalizedValue,
		ActualType:     InferCodeExecActualType(normalizedValue),
		Content:        RenderCodeExecCanonicalContent(normalizedValue),
	}, nil
}

func NormalizeCodeExecOutputValue(value any) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, NormalizeCodeExecOutputValue(item))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = NormalizeCodeExecOutputValue(item)
		}
		return out
	default:
		return v
	}
}

func InferCodeExecActualType(value any) string {
	value = NormalizeCodeExecOutputValue(value)
	switch v := value.(type) {
	case nil:
		return "Null"
	case bool:
		return "Boolean"
	case string:
		return "String"
	case map[string]any:
		return "Object"
	case []any:
		if len(v) == 0 {
			return "Array<Any>"
		}
		first := InferCodeExecActualType(v[0])
		for _, item := range v[1:] {
			if InferCodeExecActualType(item) != first {
				return "Array<Any>"
			}
		}
		return "Array<" + first + ">"
	case json.Number:
		return "Number"
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "Any"
		}
		return "Number"
	case float32:
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return "Any"
		}
		return "Number"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "Number"
	default:
		return "Any"
	}
}

func RenderCodeExecCanonicalContent(value any) string {
	value = NormalizeCodeExecOutputValue(value)
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case map[string]any, []any:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	default:
		return fmt.Sprint(v)
	}
}

func selectCodeExecBusinessOutput(outputs map[string]any) (string, any, error) {
	if len(outputs) == 1 {
		for name, meta := range outputs {
			if err := validateCodeExecBusinessOutputName(name); err != nil {
				return "", nil, err
			}
			return name, meta, nil
		}
	}

	var (
		businessName string
		businessMeta any
		count        int
	)
	for name, meta := range outputs {
		if _, reserved := codeExecSystemOutputKeys[name]; reserved {
			continue
		}
		count++
		businessName = name
		businessMeta = meta
	}
	if count != 1 {
		return "", nil, fmt.Errorf("CodeExec contract must contain exactly one business output, got %d", count)
	}
	if err := validateCodeExecBusinessOutputName(businessName); err != nil {
		return "", nil, err
	}
	return businessName, businessMeta, nil
}

func validateCodeExecBusinessOutputName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("CodeExec business output name must not be empty")
	}
	if _, reserved := codeExecSystemOutputKeys[name]; reserved {
		return fmt.Errorf("CodeExec reserved output name is not allowed: %s", name)
	}
	if strings.Contains(name, ".") {
		return fmt.Errorf("CodeExec business output name must not contain '.': %s", name)
	}
	return nil
}

func validateCodeExecTopLevelValueDomain(value any) error {
	switch value.(type) {
	case nil, bool, string, map[string]any, []any, json.Number,
		float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return nil
	default:
		return fmt.Errorf(
			"CodeExec unsupported top-level result type: %T. Allowed top-level values are String, Number, Boolean, Object, Array, or Null.",
			value,
		)
	}
}

func validateCodeExecExpectedType(expectedType string, value any, path string) error {
	etype, err := normalizeCodeExecExpectedType(expectedType)
	if err != nil {
		return err
	}
	if etype == "" || strings.EqualFold(etype, "Any") {
		return nil
	}

	value = NormalizeCodeExecOutputValue(value)
	if strings.HasPrefix(etype, "Array<") && strings.HasSuffix(etype, ">") {
		list, ok := value.([]any)
		if !ok {
			return fmt.Errorf(
				"CodeExec contract mismatch at %s: expected type %s, got %s",
				codeExecPathOrValue(path), etype, InferCodeExecActualType(value),
			)
		}
		innerType := strings.TrimSpace(etype[len("Array<") : len(etype)-1])
		for i, item := range list {
			childPath := fmt.Sprintf("[%d]", i)
			if path != "" {
				childPath = path + childPath
			}
			if err := validateCodeExecExpectedType(innerType, item, childPath); err != nil {
				return err
			}
		}
		return nil
	}

	valid := false
	switch etype {
	case "String":
		_, valid = value.(string)
	case "Number":
		switch value.(type) {
		case json.Number, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			valid = true
		}
	case "Boolean":
		_, valid = value.(bool)
	case "Object":
		_, valid = value.(map[string]any)
	case "Null":
		valid = value == nil
	default:
		return fmt.Errorf("Unsupported expected type: %s", expectedType)
	}
	if valid {
		return nil
	}
	return fmt.Errorf(
		"CodeExec contract mismatch at %s: expected type %s, got %s",
		codeExecPathOrValue(path), etype, InferCodeExecActualType(value),
	)
}

func normalizeCodeExecExpectedType(expectedType string) (string, error) {
	etype := strings.TrimSpace(expectedType)
	if etype == "" {
		return "", nil
	}
	switch strings.ToLower(etype) {
	case "string":
		return "String", nil
	case "number":
		return "Number", nil
	case "boolean":
		return "Boolean", nil
	case "object":
		return "Object", nil
	case "null":
		return "Null", nil
	case "any":
		return "Any", nil
	}
	low := strings.ToLower(etype)
	if strings.HasPrefix(low, "array<") && strings.HasSuffix(etype, ">") {
		inner := strings.TrimSpace(etype[len("Array<") : len(etype)-1])
		if inner == "" {
			return "", fmt.Errorf("Unsupported expected type: %s", expectedType)
		}
		normalizedInner, err := normalizeCodeExecExpectedType(inner)
		if err != nil {
			return "", err
		}
		return "Array<" + normalizedInner + ">", nil
	}
	return etype, nil
}

func codeExecPathOrValue(path string) string {
	if strings.TrimSpace(path) == "" {
		return "value"
	}
	return path
}
