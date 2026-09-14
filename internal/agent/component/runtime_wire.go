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
// orchestrator (cmd/server_main, cmd/ragflow-cli, ...) blank-imports
// internal/agent/component to trigger this init, which is the same
// trigger that drives each component's Register(...) call.
//
// As of plan §4 Phase 0, this wires the factory via
// runtime.InstallDefaultRegistryFactory rather than calling
// runtime.SetDefaultFactory directly. The install helper installs a
// closure that performs a runtime.DefaultRegistry.Lookup on every
// invocation, so the same single source of truth serves both the
// canvas builder and any other consumer that resolves a component by
// name. Tests that want to stub the factory call
// runtime.SetDefaultFactory directly and restore it on t.Cleanup.
package component

import (
	"ragflow/internal/agent/runtime"
)

func init() {
	runtime.InstallDefaultRegistryFactory()
}
