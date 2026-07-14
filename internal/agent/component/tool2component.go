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
	"fmt"

	einotool "github.com/cloudwego/eino/components/tool"

	agenttool "ragflow/internal/agent/tool"
)

type toolInvoker interface {
	InvokableRun(ctx context.Context, argsJSON string, opts ...einotool.Option) (string, error)
}

type toolComponentSpec struct {
	componentName string
	toolName      string
	wrap          func(toolInvoker) Component
}

func newToolComponentFactory(spec toolComponentSpec) Factory {
	return func(params map[string]any) (Component, error) {
		inner, err := agenttool.BuildByName(spec.toolName, params)
		if err != nil {
			return nil, err
		}
		invoker, ok := inner.(toolInvoker)
		if !ok {
			return nil, fmt.Errorf("%s: tool does not implement InvokableRun", spec.componentName)
		}
		return spec.wrap(invoker), nil
	}
}

func registerToolComponent(spec toolComponentSpec) {
	Register(spec.componentName, newToolComponentFactory(spec))
}
