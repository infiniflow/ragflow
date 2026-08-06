//
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
//

package connector

import (
	"context"
	"fmt"
	"sync"
)

// Factory creates a connector for a task context.
type Factory func(ctx context.Context, taskContext any) (Connector, error)

// Registry maps Python connector source names to Go connector factories.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry creates an empty connector registry.
func NewRegistry() *Registry {
	return &Registry{factories: map[string]Factory{}}
}

// Register adds or replaces a connector factory.
func (r *Registry) Register(source string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[source] = factory
}

// Open creates a connector for a task context.
func (r *Registry) Open(ctx context.Context, taskContext any) (Connector, error) {
	row, ok := taskContext.(interface{ ConnectorSource() string })
	if ok {
		return r.openSource(ctx, row.ConnectorSource(), taskContext)
	}
	sourceProvider, ok := taskContext.(interface{ Source() string })
	if ok {
		return r.openSource(ctx, sourceProvider.Source(), taskContext)
	}
	rowContext, ok := taskContext.(struct{ Connector struct{ Source string } })
	if ok {
		return r.openSource(ctx, rowContext.Connector.Source, taskContext)
	}
	source := ""
	if value, ok := any(taskContext).(interface{ GetSource() string }); ok {
		source = value.GetSource()
	}
	return r.openSource(ctx, source, taskContext)
}

// openSource creates a connector for a known source.
func (r *Registry) openSource(ctx context.Context, source string, taskContext any) (Connector, error) {
	r.mu.RLock()
	factory := r.factories[source]
	r.mu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("unsupported connector source %q", source)
	}
	return factory(ctx, taskContext)
}
