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
	"reflect"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// BuildParamsFromStruct reflects a Go struct and generates an eino schema.ParamsOneOf.
// The struct field tags are used as the single source of truth:
//   - `json:"name"` determines the parameter name (skipped if `-`)
//   - `doc:"description"` or `desc:"description"` provides the documentation
//   - `required:"true"` marks the parameter as required
//   - `enum:"val1,val2"` specifies permissible string values
//   - `schema:"-"` hides the field from LLM tool schema (e.g. for canvas-only parameters)
func BuildParamsFromStruct(sample any) *schema.ParamsOneOf {
	params := make(map[string]*schema.ParameterInfo)
	if sample == nil {
		return schema.NewParamsOneOfByParams(params)
	}

	typ, ok := sample.(reflect.Type)
	if !ok {
		typ = reflect.TypeOf(sample)
	}
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return schema.NewParamsOneOfByParams(params)
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" || field.Tag.Get("schema") == "-" || field.Tag.Get("doc") == "-" {
			continue
		}
		paramName := strings.Split(jsonTag, ",")[0]
		if paramName == "" {
			paramName = strings.ToLower(field.Name)
		}

		desc := field.Tag.Get("doc")
		if desc == "" {
			desc = field.Tag.Get("desc")
		}
		if desc == "" {
			desc = field.Tag.Get("description")
		}

		required := false
		if reqTag := field.Tag.Get("required"); reqTag == "true" {
			required = true
		} else if strings.Contains(field.Tag.Get("binding"), "required") {
			required = true
		}

		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}

		dataType := mapKindToDataType(fieldType.Kind())
		if typeTag := field.Tag.Get("type"); typeTag != "" {
			switch strings.ToLower(typeTag) {
			case "integer", "int":
				dataType = schema.Integer
			case "string":
				dataType = schema.String
			case "boolean", "bool":
				dataType = schema.Boolean
			case "number", "float":
				dataType = schema.Number
			case "array":
				dataType = schema.Array
			case "object":
				dataType = schema.Object
			}
		}

		var enumVals []string
		if enumTag := field.Tag.Get("enum"); enumTag != "" {
			for _, item := range strings.Split(enumTag, ",") {
				item = strings.TrimSpace(item)
				if item != "" {
					enumVals = append(enumVals, item)
				}
			}
		}

		paramInfo := &schema.ParameterInfo{
			Type:     dataType,
			Desc:     desc,
			Required: required,
			Enum:     enumVals,
		}

		if dataType == schema.Array {
			elemDataType := schema.String
			if fieldType.Kind() == reflect.Slice || fieldType.Kind() == reflect.Array {
				elemType := fieldType.Elem()
				for elemType.Kind() == reflect.Pointer {
					elemType = elemType.Elem()
				}
				elemDataType = mapKindToDataType(elemType.Kind())
			}
			paramInfo.ElemInfo = &schema.ParameterInfo{
				Type: elemDataType,
			}
		}

		params[paramName] = paramInfo
	}

	return schema.NewParamsOneOfByParams(params)
}

// BuildToolInfo builds a complete schema.ToolInfo using the struct as the schema source of truth.
func BuildToolInfo(name, desc string, sample any) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        name,
		Desc:        desc,
		ParamsOneOf: BuildParamsFromStruct(sample),
	}, nil
}

// BuildComponentSpec builds a ComponentSpec from a struct's documented fields.
// Unlike BuildParamsFromStruct, BuildComponentSpec preserves `schema:"-"` fields
// so Canvas nodes still have access to node-level configuration like top_n.
func BuildComponentSpec(sample any) ComponentSpec {
	inputs := make(map[string]string)
	if sample == nil {
		return ComponentSpec{Inputs: inputs}
	}

	typ, ok := sample.(reflect.Type)
	if !ok {
		typ = reflect.TypeOf(sample)
	}
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return ComponentSpec{Inputs: inputs}
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" || field.Tag.Get("component") == "-" || field.Tag.Get("doc") == "-" {
			continue
		}
		paramName := strings.Split(jsonTag, ",")[0]
		if paramName == "" {
			paramName = strings.ToLower(field.Name)
		}

		desc := field.Tag.Get("doc")
		if desc == "" {
			desc = field.Tag.Get("desc")
		}
		if desc == "" {
			desc = field.Tag.Get("description")
		}

		if desc == "" && field.Tag.Get("schema") == "-" {
			continue
		}

		inputs[paramName] = desc
	}

	return ComponentSpec{Inputs: inputs}
}

func mapKindToDataType(kind reflect.Kind) schema.DataType {
	switch kind {
	case reflect.String:
		return schema.String
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return schema.Integer
	case reflect.Float32, reflect.Float64:
		return schema.Number
	case reflect.Bool:
		return schema.Boolean
	case reflect.Slice, reflect.Array:
		return schema.Array
	case reflect.Struct, reflect.Map:
		return schema.Object
	default:
		return schema.String
	}
}
