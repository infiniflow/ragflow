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

// runtime_wire.go — installs the production ComponentFactory into the
// shared runtime package at package init time.
//
// The canvas builder (internal/agent/canvas) consumes the factory via
// runtime.DefaultFactory() so it can resolve real component bodies at
// BuildWorkflow time without importing the component package. The
// orchestrator (cmd/server_main, cmd/ragflow_cli, ...) blank-imports
// internal/agent/component to trigger this init, which is the same
// trigger that drives each component's Register(...) call.
package component

import (
	"ragflow/internal/agent/runtime"
)

func init() {
	// Adapter: component.New returns (component.Component, error),
	// and component.Component satisfies runtime.Component
	// structurally (Invoke is the only method runtime.Component
	// declares). A typed return is required so the closure's
	// signature matches runtime.ComponentFactory.
	runtime.SetDefaultFactory(func(name string, params map[string]any) (runtime.Component, error) {
		c, err := New(name, params)
		if err != nil {
			return nil, err
		}
		return c, nil
	})
}
