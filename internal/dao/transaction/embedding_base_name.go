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

package transaction

import (
	"context"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// EmbeddingModelRef extracts the tenant_model reference a dataset's embedding
// columns point at: tenant_embd_id, or embd_id itself when it stores a raw
// tenant_model id rather than a legacy "model@instance@provider" composite.
// raw is the trimmed embd_id, kept as the fallback once id no longer resolves.
func EmbeddingModelRef(kb *entity.Knowledgebase) (id, raw string) {
	raw = strings.TrimSpace(kb.EmbdID)
	if kb.TenantEmbdID != nil {
		id = strings.TrimSpace(*kb.TenantEmbdID)
	}
	if id == "" && raw != "" && !strings.Contains(raw, "@") {
		id = raw
	}
	return id, raw
}

// EmbeddingBaseNameRead resolves one dataset's embedding reference to a bare
// model name through tenant_model. It is the read the resolver below memoizes.
type EmbeddingBaseNameRead func(ctx context.Context, db *gorm.DB, kb *entity.Knowledgebase) string

// EmbeddingBaseNameResolver answers "do these datasets share one embedding
// model?" for a batch with a single tenant_model read per distinct reference.
// Datasets commonly repeat one reference, and a request mixing N datasets would
// otherwise pay N identical reads only to compare answers that were equal all
// along.
//
// A resolver belongs to one request: build one, use it for the batch, drop it.
// Resolutions are not stable across requests — an upgraded tenant_model row
// must reach the next request.
type EmbeddingBaseNameResolver struct {
	read  EmbeddingBaseNameRead
	names map[string]string
}

// NewEmbeddingBaseNameResolver builds a resolver that answers misses through
// read.
func NewEmbeddingBaseNameResolver(read EmbeddingBaseNameRead) *EmbeddingBaseNameResolver {
	return &EmbeddingBaseNameResolver{read: read, names: make(map[string]string)}
}

// Resolve reports the bare embedding model name of kb (e.g. "BAAI/bge-m3"). A
// reference already resolved in this batch answers from the cache, and datasets
// carrying no tenant_model reference cost no read at all: their legacy
// composite form reduces to a base name in memory, so the circuit opens before
// a query is composed.
func (r *EmbeddingBaseNameResolver) Resolve(ctx context.Context, db *gorm.DB, kb *entity.Knowledgebase) string {
	id, raw := EmbeddingModelRef(kb)
	if id == "" {
		return common.BaseModelName(raw)
	}
	if cached, ok := r.names[id]; ok {
		return cached
	}
	base := r.read(ctx, db, kb)
	r.names[id] = base
	return base
}
