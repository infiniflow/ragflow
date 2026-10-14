// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package component

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gorm.io/gorm"

	"ragflow/internal/agent/chat"
)

type structuredOutput struct {
	schemaJSON string
	validator  *jsonschema.Schema
}

func newStructuredOutput(raw any) (*structuredOutput, error) {
	if raw == nil {
		return nil, nil
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("structured output schema must be an object")
	}
	definition := maps.Clone(object)
	delete(definition, "value")
	if len(definition) == 0 {
		return nil, nil
	}
	if typ, exists := definition["type"]; exists {
		if typ, ok := typ.(string); !ok || typ != "object" {
			return nil, fmt.Errorf("structured output schema root type must be object")
		}
	} else {
		definition["type"] = "object"
	}
	if draft, exists := definition["$schema"]; exists {
		version, ok := draft.(string)
		if !ok {
			return nil, fmt.Errorf("unsupported structured output schema version: %v", draft)
		}
		version = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(version, "https://"), "http://"), "#")
		switch version {
		case "json-schema.org/draft-04/schema", "json-schema.org/draft-06/schema", "json-schema.org/draft-07/schema", "json-schema.org/draft/2019-09/schema", "json-schema.org/draft/2020-12/schema":
		default:
			return nil, fmt.Errorf("unsupported structured output schema version: %v", draft)
		}
	}
	encoded, err := json.Marshal(definition)
	if err != nil {
		return nil, fmt.Errorf("encode structured output schema: %w", err)
	}
	parsed, err := jsonschema.UnmarshalJSON(strings.NewReader(string(encoded)))
	if err != nil {
		return nil, fmt.Errorf("invalid structured output schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	// An empty scheme map blocks external network and file references; standard meta-schemas are built in.
	compiler.UseLoader(jsonschema.SchemeURLLoader{})
	const resource = "urn:ragflow:structured-output"
	if err := compiler.AddResource(resource, parsed); err != nil {
		return nil, fmt.Errorf("invalid structured output schema: %w", err)
	}
	validator, err := compiler.Compile(resource)
	if err != nil {
		return nil, fmt.Errorf("invalid structured output schema: %w", err)
	}
	return &structuredOutput{schemaJSON: string(encoded), validator: validator}, nil
}

func structuredOutputFrom(params map[string]any) (any, bool) {
	if outputs, ok := mapFrom(params, "outputs"); ok {
		if value, ok := outputs["structured"]; ok {
			return value, true
		}
	}
	value, ok := params["output_structure"]
	return value, ok
}

func (s *structuredOutput) prompt() string {
	return "\nReturn only a JSON object that satisfies this JSON Schema:\n" + s.schemaJSON
}

func (s *structuredOutput) parse(content string) (map[string]any, error) {
	value, err := jsonschema.UnmarshalJSON(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("invalid structured output JSON: %w", err)
	}
	parsed, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("structured output must be a JSON object")
	}
	if err := s.validator.Validate(parsed); err != nil {
		return nil, fmt.Errorf("structured output schema validation: %w", err)
	}
	if _, err := structuredOutputRuntimeValue(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

// Downstream components use float64; validate exactly, then reject numbers that cannot round-trip losslessly.
func structuredOutputRuntimeValue(value any) (any, error) {
	switch value := value.(type) {
	case json.Number:
		number, err := value.Float64()
		if err != nil {
			return nil, fmt.Errorf("structured output number %s exceeds runtime range: %w", value, err)
		}
		original, ok := new(big.Rat).SetString(string(value))
		roundTrip, roundTripOK := new(big.Rat).SetString(strconv.FormatFloat(number, 'g', -1, 64))
		if !ok || !roundTripOK || original.Cmp(roundTrip) != 0 {
			return nil, fmt.Errorf("structured output number %s cannot be represented without precision loss", value)
		}
		return number, nil
	case map[string]any:
		for key, item := range value {
			converted, err := structuredOutputRuntimeValue(item)
			if err != nil {
				return nil, err
			}
			value[key] = converted
		}
	case []any:
		for i, item := range value {
			converted, err := structuredOutputRuntimeValue(item)
			if err != nil {
				return nil, err
			}
			value[i] = converted
		}
	}
	return value, nil
}

func (s *structuredOutput) validateAndRepair(ctx context.Context, db *gorm.DB, inv ChatInvoker, req ChatInvokeRequest, resp *ChatInvokeResponse, retries int) (*ChatInvokeResponse, map[string]any, error) {
	if retries < 0 {
		return nil, nil, fmt.Errorf("structured output correction retries must be nonnegative")
	}
	// Correction generates text only; copy messages to preserve caller history and avoid further tool calls.
	req.Messages = slices.Clone(req.Messages)
	req.Tools = nil
	req.ToolChoice = chat.ToolChoiceNone
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if resp == nil {
			return nil, nil, fmt.Errorf("structured output received a nil model response")
		}
		if len(resp.ToolCalls) > 0 {
			return nil, nil, fmt.Errorf("structured output received tool calls instead of a final answer")
		}
		content := strings.TrimSpace(resp.Content)
		// Strip outer wrappers only, preserving thinking and fence markers inside JSON strings.
		if !json.Valid([]byte(content)) && strings.HasPrefix(content, "<think>") {
			if end := strings.Index(content, "</think>"); end >= 0 {
				content = strings.TrimSpace(content[end+len("</think>"):])
			}
		}
		if !json.Valid([]byte(content)) && strings.HasPrefix(content, "```json") && strings.HasSuffix(content, "```") {
			content = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(content, "```json"), "```"))
		}
		parsed, err := s.parse(content)
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if err == nil {
			result := *resp
			result.Content = content
			return &result, parsed, nil
		}
		if attempt == retries {
			return nil, nil, fmt.Errorf("structured output failed after %d correction attempts: %w", attempt, err)
		}
		if inv == nil {
			return nil, nil, fmt.Errorf("structured output correction requires a chat invoker")
		}
		req.Messages = append(req.Messages,
			schema.Message{Role: schema.Assistant, Content: resp.Content},
			schema.Message{Role: schema.User, Content: "Correct the previous answer. Validation error: " + err.Error() + s.prompt()},
		)
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		resp, err = unwrapChatInvoker(inv).Invoke(ctx, db, req)
		if err != nil {
			return nil, nil, fmt.Errorf("structured output correction: %w", err)
		}
	}
}
