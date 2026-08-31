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

package component

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"

	"gorm.io/gorm"
)

// ToolBackedComponent is the single Canvas adapter for tools that implement
// agenttool.ToolComponent.
type ToolBackedComponent struct {
	name string
	tool agenttool.ToolComponent
	spec agenttool.ComponentSpec
}

func newToolComponentFactory(componentName, toolName string) Factory {
	return func(params map[string]any) (Component, error) {
		base, err := agenttool.BuildByName(toolName, params)
		if err != nil {
			return nil, err
		}
		componentTool, ok := base.(agenttool.ToolComponent)
		if !ok {
			return nil, fmt.Errorf("%s: tool %q does not implement ToolComponent", componentName, toolName)
		}
		return &ToolBackedComponent{
			name: componentName,
			tool: componentTool,
			spec: componentTool.ComponentSpec(),
		}, nil
	}
}

func (c *ToolBackedComponent) Name() string { return c.name }

func (c *ToolBackedComponent) Inputs() map[string]string { return c.spec.Inputs }

func (c *ToolBackedComponent) Outputs() map[string]string { return c.spec.Outputs }

func (c *ToolBackedComponent) GetInputForm() map[string]any { return c.spec.InputForm }

func (c *ToolBackedComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	argsJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("canvas: %s: encode inputs: %w", c.name, err)
	}

	raw, invokeErr := c.tool.InvokableRun(ctx, string(argsJSON))
	var decoded map[string]any
	if c.spec.PreserveJSONNumbers {
		decoded = parseToolEnvelopeLossless(raw)
	} else {
		decoded = parseToolEnvelope(raw)
	}
	if rawValue, invalid := decoded["_raw"]; invalid {
		if invokeErr != nil {
			return nil, fmt.Errorf("canvas: %s: %w", c.name, invokeErr)
		}
		return nil, fmt.Errorf("canvas: %s: invalid tool result: %v", c.name, rawValue)
	}
	if existing, _ := decoded["_ERROR"].(string); strings.TrimSpace(existing) != "" {
		outputs := c.tool.BuildComponentOutputs(decoded)
		if outputs == nil {
			outputs = make(map[string]any, 1)
		}
		outputs["_ERROR"] = existing
		return outputs, nil
	}
	if invokeErr != nil {
		return nil, fmt.Errorf("canvas: %s: %w", c.name, invokeErr)
	}

	if builder, ok := c.tool.(agenttool.ReferenceBuilder); ok {
		chunks, docAggs := builder.BuildReferences(ctx, decoded)
		if state, _, stateErr := runtime.GetStateFromContext[*runtime.CanvasState](ctx); stateErr == nil && state != nil {
			state.SetRetrievalReferences(chunks, docAggs)
		}
	}
	return c.tool.BuildComponentOutputs(decoded), nil
}

func parseToolEnvelopeLossless(jsonStr string) map[string]any {
	var out map[string]any
	decoder := json.NewDecoder(strings.NewReader(jsonStr))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return map[string]any{"_raw": jsonStr}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return map[string]any{"_raw": jsonStr}
	}
	return out
}

func (c *ToolBackedComponent) Stream(_ context.Context, _ *gorm.DB, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

var toolComponentRegistrations = []struct {
	componentName string
	toolName      string
}{
	{componentName: "GitHub", toolName: "github"},
	{componentName: "BGPT", toolName: "bgpt"},
	{componentName: "ArXiv", toolName: "arxiv"},
	{componentName: "DuckDuckGo", toolName: "duckduckgo"},
	{componentName: "Email", toolName: "email"},
	{componentName: "ExeSQL", toolName: "execute_sql"},
	{componentName: "Google", toolName: "google"},
	{componentName: "GoogleScholar", toolName: "google_scholar"},
	{componentName: "KeenableSearch", toolName: "keenable"},
	{componentName: "PubMed", toolName: "pubmed"},
	{componentName: "QueritContents", toolName: "querit_contents"},
	{componentName: "QueritSearch", toolName: "querit_search"},
	{componentName: "SearXNG", toolName: "searxng"},
	{componentName: "TavilySearch", toolName: "tavily"},
	{componentName: "TavilyExtract", toolName: "tavily_extract"},
	{componentName: "WenCai", toolName: "wencai"},
	{componentName: "Wikipedia", toolName: "wikipedia"},
	{componentName: "YahooFinance", toolName: "yahoo_finance"},
}

func init() {
	for _, registration := range toolComponentRegistrations {
		Register(
			registration.componentName,
			newToolComponentFactory(registration.componentName, registration.toolName),
		)
	}
}
