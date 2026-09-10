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

package knowledge_compile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"ragflow/internal/engine"
	"ragflow/internal/entity"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"

	"gorm.io/gorm"
)

// Option configures a Consumer.
type Option func(*Consumer)

// WithTTL sets the per-KB claim lease TTL.
func WithTTL(d time.Duration) Option {
	return func(c *Consumer) {
		if d > 0 {
			c.ttl = d
		}
	}
}

// WithHeartbeat sets the claim heartbeat period (must be < TTL).
func WithHeartbeat(d time.Duration) Option {
	return func(c *Consumer) {
		if d > 0 {
			c.heartbeat = d
		}
	}
}

// WithPollInterval sets how often the worker poll loop looks for a claimable KB
// when no NATS notify arrives.
func WithPollInterval(d time.Duration) Option {
	return func(c *Consumer) {
		if d > 0 {
			c.pollInterval = d
		}
	}
}

// WithSweepInterval sets how often the worker tick calls TryClaim, which
// reclaims inflight left by crashed workers before claiming any ready batch.
func WithSweepInterval(d time.Duration) Option {
	return func(c *Consumer) {
		if d > 0 {
			c.sweepInterval = d
		}
	}
}

// WithReader overrides the chunk Reader (used in tests).
func WithReader(r Reader) Option { return func(c *Consumer) { c.reader = r } }

// WithWriter overrides the chunk Writer (used in tests).
func WithWriter(w Writer) Option { return func(c *Consumer) { c.writer = w } }

// WithDeduperFactory overrides the per-tenant Deduper factory.
func WithDeduperFactory(f DeduperFactory) Option { return func(c *Consumer) { c.factory = f } }

// withWikiContributionStore overrides the durable document-contribution store
// in package tests.
func withWikiContributionStore(store wikiContributionStore) Option {
	return func(c *Consumer) { c.contributions = store }
}

// defaultLLMID / defaultEmbedding are the model ids used when resolving LLM
// deps for the dataset-level deduper. Set via SetModelConfig (typically from the
// server bootstrap). Empty strings make the factory fall back to the noop
// deduper until production wiring supplies real values.
var (
	defaultLLMID     string
	defaultEmbedding string
)

// SetModelConfig records the model ids used to build the LLM deduper.
func SetModelConfig(llmID, embedding string) {
	defaultLLMID = llmID
	defaultEmbedding = embedding
}

// InitializePublisher installs the process-wide publisher used by document
// lifecycle events. API processes publish events but do not own the dataset
// consumer, so they need this producer-only initialization without starting a
// worker.
func InitializePublisher(db *gorm.DB, mq engine.MessageQueue) {
	if defaultPublisher == nil {
		SetScheduler(newScheduler(db, mq, generateHolder(), 2*time.Minute))
	}
}

// defaultDeduperFactory resolves the per-tenant LLM deps and builds the
// KB-scoped deduper. On any failure it returns an error so the caller falls
// back to the noop deduper (merged products are still written, just without
// cross-document LLM merging).
func defaultDeduperFactory(tenant string) (Deduper, error) {
	deps, err := kccommon.ResolveDeps(tenant, defaultLLMID, defaultEmbedding)
	if err != nil {
		return nil, err
	}
	return NewLLMDeduper(deps.Chat, deps.Embed, defaultLLMID, 0.99, deps.ModelContextLen, deps.ModelMaxOutput), nil
}

func generateHolder() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("knowledgecompile-%d", time.Now().UnixNano())
	}
	return "knowledgecompile-" + hex.EncodeToString(b)
}

// Provision initializes the dataset-level compile scheduling store and installs
// the package-level Scheduler used by the publishing path. It is called once by
// the owning Ingestor (which then drives the consumer from its own worker pool),
// so the dataset-level consumer shares the ingestor's lifecycle instead of
// being a standalone service. Best-effort: provisioning errors are returned so
// the caller can log and continue (the pipeline still writes available_int=0
// compiled chunks; they just won't be merged until a scheduler is available).
func Provision(ctx context.Context, mq engine.MessageQueue, db *gorm.DB) error {
	if db == nil {
		return nil
	}
	kcDB = db
	s := newScheduler(db, mq, generateHolder(), 2*time.Minute)
	// Bound the startup AutoMigrate so a slow/unreachable DB cannot block
	// startup indefinitely. The caller's ctx is also honoured (cancelled on
	// shutdown).
	runCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.Provision(ctx); err != nil {
		return err
	}
	if err := db.WithContext(ctx).AutoMigrate(&entity.WikiDocumentDirty{}); err != nil {
		return err
	}
	SetScheduler(s)
	go runWikiDirtyWorker(runCtx, db, s.holder)
	return nil
}
