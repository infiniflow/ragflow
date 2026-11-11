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

package dao

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/entity"
)

// WithStableDatasetEmbedding serializes a vector write with dataset model
// changes and rejects vectors computed against an outdated binding. The write
// must become searchable before it returns so the next model change can see it.
func WithStableDatasetEmbedding(ctx context.Context, db *gorm.DB, kbID, tenantID, ref string, write func() error) error {
	if db == nil || strings.TrimSpace(ref) == "" || write == nil {
		return errors.New("dataset embedding binding is unavailable")
	}
	return ScopedDB(ctx, db).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var kb entity.Knowledgebase
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Take(&kb, "id = ? AND tenant_id = ? AND status = ?", kbID, tenantID, string(entity.StatusValid)).Error; err != nil {
			return err
		}
		if kb.EmbeddingModelReference() != ref {
			return errors.New("dataset embedding model changed while preparing vectors; retry the operation")
		}
		return write()
	})
}

// WithStableDatasetEmbeddingBindings holds the dataset locks while read uses
// a model resolved from the supplied snapshots. Lock order is deterministic.
func WithStableDatasetEmbeddingBindings(ctx context.Context, db *gorm.DB, datasets []*entity.Knowledgebase, read func(context.Context) error) error {
	if db == nil || read == nil {
		return errors.New("dataset database is not initialized")
	}
	ordered := append([]*entity.Knowledgebase(nil), datasets...)
	for _, kb := range ordered {
		if kb == nil {
			return errors.New("dataset embedding binding is unavailable")
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, expected := range ordered {
			if expected == nil || expected.EmbeddingModelReference() == "" {
				return errors.New("dataset embedding binding is unavailable")
			}
			var current entity.Knowledgebase
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Take(&current, "id = ? AND tenant_id = ? AND status = ?", expected.ID, expected.TenantID, string(entity.StatusValid)).Error; err != nil {
				return err
			}
			if current.EmbeddingModelReference() != expected.EmbeddingModelReference() {
				return errors.New("dataset embedding model changed during retrieval; retry the operation")
			}
		}
		return read(WithScopedDB(ctx, tx))
	})
}

// scopedDBKey carries a transaction for nested database operations.
type scopedDBKey struct{}

type scopedLocksKey struct{}
type scopedLocks struct {
	mu   sync.Mutex
	held map[string]*sync.Mutex
}

// WithScopedLocks holds per-resource locks until the surrounding transaction ends.
func WithScopedLocks(ctx context.Context) (context.Context, func()) {
	locks := &scopedLocks{held: make(map[string]*sync.Mutex)}
	return context.WithValue(ctx, scopedLocksKey{}, locks), func() {
		for _, mu := range locks.held {
			mu.Unlock()
		}
	}
}

// HoldScopedLock returns an unlock function for ordinary calls. In a scoped
// transaction it retains the lock until the transaction commits or rolls back.
func HoldScopedLock(ctx context.Context, key string, mu *sync.Mutex) func() {
	locks, ok := ctx.Value(scopedLocksKey{}).(*scopedLocks)
	if !ok {
		mu.Lock()
		return mu.Unlock
	}
	locks.mu.Lock()
	defer locks.mu.Unlock()
	if _, ok := locks.held[key]; !ok {
		mu.Lock()
		locks.held[key] = mu
	}
	return func() {}
}

// ScopedDB returns the transaction carried by ctx or the fallback database.
func ScopedDB(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if db, ok := ctx.Value(scopedDBKey{}).(*gorm.DB); ok && db != nil {
		return db
	}
	return fallback
}

// WithScopedDB shares the current transaction with nested scoped operations.
func WithScopedDB(ctx context.Context, db *gorm.DB) context.Context {
	return context.WithValue(ctx, scopedDBKey{}, db)
}
