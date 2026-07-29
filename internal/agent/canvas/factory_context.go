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

package canvas

import (
	"context"

	"ragflow/internal/agent/runtime"
)

type componentFactoryContextKey struct{}

// WithComponentFactory attaches a per-pipeline component factory override
// to ctx. BuildWorkflow uses it when constructing node bodies so one
// pipeline instance can resolve components independently of the process-wide
// runtime.DefaultFactory.
func WithComponentFactory(ctx context.Context, factory runtime.ComponentFactory) context.Context {
	if factory == nil {
		return ctx
	}
	return context.WithValue(ctx, componentFactoryContextKey{}, factory)
}

func componentFactoryFromContext(ctx context.Context) runtime.ComponentFactory {
	factory, _ := ctx.Value(componentFactoryContextKey{}).(runtime.ComponentFactory)
	return factory
}
