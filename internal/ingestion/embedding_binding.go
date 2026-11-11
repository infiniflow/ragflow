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

package ingestion

import (
	"context"
	"fmt"
	"sync"
)

type embeddingBindingKey struct{}

type embeddingBinding struct {
	mu  sync.Mutex
	ref string
}

func WithEmbeddingBinding(ctx context.Context) context.Context {
	return context.WithValue(ctx, embeddingBindingKey{}, &embeddingBinding{})
}

func RecordEmbeddingBinding(ctx context.Context, ref string) error {
	binding, _ := ctx.Value(embeddingBindingKey{}).(*embeddingBinding)
	if binding == nil {
		return nil
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	if binding.ref != "" && binding.ref != ref {
		return fmt.Errorf("ingestion: different embedding models used in one dataset run")
	}
	binding.ref = ref
	return nil
}

func EmbeddingBindingFromContext(ctx context.Context) string {
	binding, _ := ctx.Value(embeddingBindingKey{}).(*embeddingBinding)
	if binding == nil {
		return ""
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	return binding.ref
}
