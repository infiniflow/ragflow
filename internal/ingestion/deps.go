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

// Package ingestion holds the dependency bundle shared by the ingestion
// orchestration layers (service, task, pipeline, knowledge_compile, component).
//
// The process-wide singletons (doc engine, database handle, message-queue
// engine, kvrocks client) are resolved exactly once, at the server assembly
// points (internal/server/api, internal/server/ingestor), and passed down
// explicitly through Deps. Orchestration code must never call engine.Get(),
// dao.DB, or kvrocks.Get() itself; arch_test.go guards that rule.
package ingestion

import (
	"fmt"

	"ragflow/internal/engine"
	kvrocks "ragflow/internal/engine/kvrocks"

	"gorm.io/gorm"
)

// Deps bundles the process-wide singletons an ingestion run needs. It is
// constructed once per process at the assembly point and threaded through the
// Ingestor, TaskContext, PipelineExecutor, and the knowledge-compile Consumer.
//
// Engine, DB, MQ and KV are required for a production ingestor. HTML
// ingestion uses KV to hand decoded images to downstream components, so
// Validate reports a missing client at startup.
type Deps struct {
	// Engine persists and searches chunks (document index, memory messages,
	// compiled knowledge products).
	Engine engine.DocEngine
	// DB is the relational handle. DAO methods take it as a parameter so
	// callers can substitute a transaction; the bundle value is the default.
	DB *gorm.DB
	// MQ is the task broker (NATS JetStream) the ingestor consumes.
	MQ engine.MessageQueue
	// KV is the Kvrocks client for HTML media, chunkcache, run trackers and memory ids.
	KV *kvrocks.Client
}

// Validate reports the required production ingestor dependencies that are missing.
func (d *Deps) Validate() error {
	if d == nil {
		return fmt.Errorf("ingestion deps: nil dependency bundle")
	}
	var missing []string
	if d.Engine == nil {
		missing = append(missing, "doc engine")
	}
	if d.DB == nil {
		missing = append(missing, "database")
	}
	if d.MQ == nil {
		missing = append(missing, "message queue")
	}
	if d.KV == nil || d.KV.GetClient() == nil {
		missing = append(missing, "kvrocks")
	}
	if len(missing) > 0 {
		return fmt.Errorf("ingestion deps: missing required dependencies: %v", missing)
	}
	return nil
}
