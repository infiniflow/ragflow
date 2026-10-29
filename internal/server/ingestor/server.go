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

// Package ingestor wires and runs the RAGFlow ingestion worker. It was
// extracted from package main so that main stays a thin dispatch layer.
package ingestor

import (
	"context"
	"fmt"
	"ragflow/internal/server/local"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	kvrocks "ragflow/internal/engine/kvrocks"
	ingestioncore "ragflow/internal/ingestion"
	"ragflow/internal/ingestion/knowledge_compile"
	ingestion "ragflow/internal/ingestion/service"
	"ragflow/internal/ingestion/task"
	"ragflow/internal/server"
	"ragflow/internal/server/bootstrap"
	"ragflow/internal/service"
	"ragflow/internal/service/nav"
	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"
)

func logTokenizerCounters() {
	var available, unavailable []string
	for _, status := range tokenizer.CounterStatuses() {
		if status.Available {
			available = append(available, status.ID)
			continue
		}
		unavailable = append(unavailable, status.ID)
	}
	common.Info("embedding tokenizer counters", zap.Strings("available", available))
	if len(unavailable) > 0 {
		common.Warn("embedding tokenizers unavailable; models that declare them cannot be ingested until the asset is restored",
			zap.Strings("unavailable", unavailable),
			zap.String("hint", "run `uv run deps/download_deps.py`, or set "+common.EnvModelAssetsDir+" to a directory holding them"))
	}
}

// Run starts the RAGFlow ingestion worker. It blocks until ctx is cancelled
// (OS shutdown signal) or the ingestor receives a shutdown command from the
// admin, then gracefully stops the ingestor.
func Run(ctx context.Context, cancel context.CancelFunc, serverName string, args *bootstrap.ServerArgs) error {
	// Initialize tokenizer (rag_analyzer)
	// tokenizer.Init handles DictPath fallback: env var → /usr/share/infinity/resource
	if err := tokenizer.Init(&tokenizer.PoolConfig{}); err != nil {
		common.Fatal("Failed to initialize tokenizer", zap.Error(err))
	}
	defer tokenizer.Close()

	// Fail fast if the cl100k_base BPE table is missing. NumTokensFromString /
	// TrimContentToTokenLimit now panic rather than degrading silently, so this
	// trades a mid-request panic for a clear startup failure.
	if err := tokenizer.InitCL100KEncoder(); err != nil {
		common.Fatal("Failed to initialize cl100k_base tokenizer", zap.Error(err))
	}
	logTokenizerCounters()

	// The dataset-level post-processing consumer cluster (§11) is owned and run by
	// the Ingestor: it is started inside ingestor.Start() and joined inside
	// ingestor.Stop(), so its lifecycle matches the ingestor. The configured
	// default LLM/embedding ids are passed so the LLM deduper is used (instead
	// of the noop fallback that still emits merged products). Best-effort: a
	// provisioning error is logged by the Ingestor and the pipeline still
	// writes available_int=0 compiled chunks; they just won't be merged until
	// the consumer is available.
	globalConfig := server.GetConfig()
	ingestorCfg := globalConfig.GetIngestorConfig()
	// Resolve the ingestor worker count (K) from CLI > env > config > default(1)
	// and fail fast on an out-of-range value.
	workers, err := globalConfig.GetIngestorConcurrentWorkers(args.IngestorConcurrentWorkers)
	if err != nil {
		return err
	}
	// Apply the configured compiler pool size (no-op when 0; the pool keeps its
	// vCPU default, overridable via KC_COMPILE_CONCURRENCY).
	knowledge_compile.SetCompilerConcurrency(ingestorCfg.CompilerPoolSize)
	ingestorDeps := &ingestioncore.Deps{
		Engine: engine.Get(),
		DB:     dao.DB,
		MQ:     engine.GetMessageQueueEngine(),
		KV:     kvrocks.Get(),
	}
	// Fail fast at the assembly point: a misconfigured ingestor (missing doc
	// engine, database, message queue, or Kvrocks) must never reach Start as a "healthy"
	// service that silently cannot consume.
	if err = ingestorDeps.Validate(); err != nil {
		common.Error("Invalid ingestor dependencies", err)
		return fmt.Errorf("ingestor dependencies invalid: %w", err)
	}
	ingestor := ingestion.NewIngestor(*args.Name, int32(workers), []string{"pdf", "docx", "txt"}, ingestorDeps)
	ingestor.SetKnowledgeCompileModelConfig(
		globalConfig.GetDefaultChatModel().Name,
		globalConfig.GetDefaultEmbeddingModel().Name,
	)
	// The dataset-level knowledge-compile consumer (tree/structure products) upserts
	// into the dataset-nav tree, so the Ingestor must install the same ES-backed
	// NavService the API server installs. Without this, nav.GetNavService() returns
	// nil and tree/structure products are dropped (the consumer logs "nav service
	// unavailable, skipping dataset-nav upsert"), leaving the dataset tree empty.
	// The embedder resolves the tenant's embedding model on demand, so both
	// Search and UpsertDoc can embed queries/summaries automatically.
	navService := nlp.NewNavService(service.NewNavEmbedder(service.NewModelFactory(), ""))
	// The cluster namer/merger is the counterpart of Python's
	// _llm_create_summary / _llm_merge: a new cluster is named after the model's
	// topic title and an existing cluster's description is fused with the joining
	// document. Passing an empty model ref resolves each tenant's default chat
	// model on demand; without one the nav service keeps its deterministic
	// behavior.
	navService.SetNavMergeLLM(service.NewNavMergeLLM(service.NewModelFactory(), ""))
	nav.SetNavService(navService)
	// Memory extraction runs on the Ingestor's shared NATS consumer + worker
	// pool (task_type="memory" dispatched by handleAndExecute -> executeMemoryTask),
	// so there is no longer a dedicated Redis memory consumer to start.
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(service.NewMemoryService(dao.DB, engine.Get(), service.WithMemoryKvrocks(kvrocks.Get()), service.WithMemoryMQEngine(engine.GetMessageQueueEngine()))))
	// Bind the relational handle and document engine into the production
	// embedder and knowledge-compiler resolvers. The task package is the
	// composition root for ingestion runs; it reads these from the explicit
	// bindings rather than the package globals.
	task.InitEmbedderResolver(dao.DB)
	task.InitKnowledgeCompilerDeps(dao.DB, engine.Get())

	// Start returns immediately (it launches the owned consume/compile
	// goroutines and joins them via Stop); a provisioning failure here must
	// fail the server (main's os.Exit(1) path) instead of reporting a
	// healthy ingestor that can never consume.
	if err = ingestor.Start(); err != nil {
		common.Error("Failed to initialize ingestor", err)
		return err
	}

	common.Info("\n    ____                      __  _\n" +
		"   /  _/___  ____ ____  _____/ /_(_)___  ____     ________  ______   _____  _____\n" +
		"   / // __ \\/ __ `/ _ \\/ ___/ __/ / __ \\/ __ \\   / ___/ _ \\/ ___/ | / / _ \\/ ___/\n" +
		" _/ // / / / /_/ /  __(__  ) /_/ / /_/ / / / /  (__  )  __/ /   | |/ /  __/ /\n" +
		"/___/_/ /_/\\__, /\\___/____/\\__/_/\\____/_/ /_/  /____/\\___/_/    |___/\\___/_/\n" +
		"          /____/\n")

	// Print RAGFlow version
	common.Info(fmt.Sprintf("RAGFlow ingestion service version: %s", common.GetRAGFlowVersion()))

	// Init admin client status in ingestor
	local.InitAdminStatus()

	// Set log level
	bootstrap.SetLogger(serverName, args)

	// Start heartbeat reporter to admin server
	if hb := bootstrap.StartHeartbeat(
		common.ServerTypeIngestion,
		fmt.Sprintf("ingestor-%s", ingestor.ID()[:8]),
		0,
		globalConfig.GetHeartbeatInterval(),
	); hb != nil {
		defer hb.Stop()
	}

	// Wait for either an OS shutdown signal or a shutdown command from the admin
	select {
	case <-ctx.Done():
		common.Info("Received shutdown signal")
		common.Info(fmt.Sprintf("Shutting down RAGFlow ingestor %s ...", *args.Name))
	case <-ingestor.ShutdownCh:
		common.Info(fmt.Sprintf("Received shutdown command from admin, stopping ingestor %s ...", *args.Name))
		cancel()
	}

	// Create context with timeout for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ingestor.Stop(shutdownCtx)

	common.Info(fmt.Sprintf("Ingestor %s shutdown complete", *args.Name))

	return nil
}
