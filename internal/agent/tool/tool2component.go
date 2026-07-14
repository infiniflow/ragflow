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
	"context"

	einotool "github.com/cloudwego/eino/components/tool"
)

// ToolInvoker is the invocation seam shared by Eino tools and Canvas
// components backed by those tools.
type ToolInvoker interface {
	InvokableRun(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error)
}

// ComponentSpec describes the Canvas-facing surface of a tool. It is kept
// separate from Info(), whose schema contains only model-emitted arguments.
type ComponentSpec struct {
	Inputs    map[string]string
	Outputs   map[string]string
	InputForm map[string]any
}

// ToolComponent is the required Canvas adaptation contract implemented by a
// tool that can back a Canvas component.
type ToolComponent interface {
	ToolInvoker
	ComponentSpec() ComponentSpec
}

// ReferenceBuilder is an optional capability for tools that add retrieval
// references to Canvas state.
type ReferenceBuilder interface {
	BuildReferences(ctx context.Context, results []any) (chunks []map[string]any, docAggs []map[string]any)
}

// ComponentOutputBuilder is an optional capability for tools that construct
// their final Canvas outputs after references have been built. chunks are the
// exact references recorded in Canvas state.
type ComponentOutputBuilder interface {
	BuildComponentOutputs(results []any, chunks []map[string]any) map[string]any
}
