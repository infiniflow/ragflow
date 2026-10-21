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

	einotool "github.com/cloudwego/eino/components/tool"

	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
)

// referenceRecordingTool decorates an Agent tool so each invocation records
// the retrieval references it produced into the canvas state. Canvas tool
// nodes already do this in ToolBackedComponent.Invoke; tools called from the
// Agent's ReAct loop go straight to InvokableRun, so without this wrapper
// state.Retrieval stays empty and citation grounding has no sources.
type referenceRecordingTool struct {
	einotool.InvokableTool
	builder agenttool.ReferenceBuilder
}

func (t *referenceRecordingTool) InvokableRun(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error) {
	out, err := t.InvokableTool.InvokableRun(ctx, argsJSON, opts...)
	if err != nil {
		return out, err
	}
	envelope := parseToolEnvelope(out)
	if _, invalid := envelope["_raw"]; invalid {
		return out, nil
	}
	chunks, docAggs := t.builder.BuildReferences(ctx, envelope)
	if state, stateErr := runtime.GetStateFromContext(ctx); stateErr == nil && state != nil {
		state.SetRetrievalReferences(chunks, docAggs)
	}
	return out, nil
}

// wrapReferenceRecordingTools decorates the tools handed to the ReAct loop.
// buildAgentTools keeps returning the raw tools so metadata paths such as
// AgentComponent.GetInputForm and Reset still see the original methods.
func wrapReferenceRecordingTools(tools []einotool.BaseTool) []einotool.BaseTool {
	wrapped := make([]einotool.BaseTool, len(tools))
	copy(wrapped, tools)
	for i, t := range tools {
		builder, buildsReferences := t.(agenttool.ReferenceBuilder)
		invokable, isInvokable := t.(einotool.InvokableTool)
		if buildsReferences && isInvokable {
			wrapped[i] = &referenceRecordingTool{InvokableTool: invokable, builder: builder}
		}
	}
	return wrapped
}
