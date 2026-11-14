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

package bootstrap

import (
	"context"
	"strings"

	"ragflow/internal/agent/retrievalbridge"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/agentic_rag"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/permission"
	"ragflow/internal/service"
	"ragflow/internal/service/nlp"
)

// InitializeRetrievalServices installs corpus tools for API and ingestor processes.
func InitializeRetrievalServices(metadataService *service.MetadataService, memoryService *service.MemoryService, modelFactory *service.ModelFactory) {
	docEngine := engine.Get()
	documentDAO := dao.NewDocumentDAO()
	retrievalEnhancer := retrievalbridge.NewEnhancer(docEngine, metadataService)
	// Keep the concrete adapter: it is both the canvas/agent-tool backend and the
	// target of the agentic-RAG bridge wired below. The bridge must hold it
	// directly (not look it up in the registry) because the registry is shared —
	// agenttool.SetRetrievalService and runtime.SetRetrievalService write the same
	// singleton, so a bridge that resolved its target per call would find itself.
	retrievalAdapter := agenttool.NewNLPRetrievalAdapterFromDeps(
		docEngine,
		documentDAO,
		agenttool.RetrievalModelConstructors{
			Embedding: func(ctx context.Context, tenantID, modelRef string) (*modelModule.EmbeddingModel, error) {
				if strings.TrimSpace(modelRef) == "" {
					return modelFactory.NewDefaultEmbeddingModel(permission.WithTenantID(ctx, tenantID))
				}
				return modelFactory.NewEmbeddingModel(permission.WithTenantID(ctx, tenantID), modelRef)
			},
			Chat: func(ctx context.Context, tenantID, modelRef string) (*modelModule.ChatModel, error) {
				if strings.TrimSpace(modelRef) == "" {
					return modelFactory.NewDefaultChatModel(permission.WithTenantID(ctx, tenantID))
				}
				return modelFactory.NewChatModel(permission.WithTenantID(ctx, tenantID), modelRef)
			},
			Rerank: func(ctx context.Context, tenantID, modelRef string) (*modelModule.RerankModel, error) {
				return modelFactory.NewRerankModel(permission.WithTenantID(ctx, tenantID), modelRef)
			},
		},
		retrievalEnhancer,
	)
	agenttool.SetRetrievalService(retrievalAdapter)
	agenttool.SetMemoryRetrievalService(retrievalbridge.NewMemoryAdapter(memoryService, retrievalEnhancer))
	common.Info("agent: retrieval service adapter installed")

	// The smart-reasoning agent's corpus tools (internal/agentic_rag): regex
	// pushdown and lexical BM25 over the same doc engine the retrieval adapter
	// wraps. The locate tools resolve these services per call, so they must be
	// registered before the server starts serving.
	runtime.SetGrepService(agentic_rag.NewGrepAdapter(docEngine))
	bm25Adapter := agentic_rag.NewBm25Adapter(docEngine)
	bm25Adapter.SetQueryBuilder(nlp.GetQueryBuilder())
	runtime.SetBm25Service(bm25Adapter)
	common.Info("agent: smart-reasoning corpus services installed (grep + bm25)")

	// The compiled-knowledge graph walk (graph_explore) resolves its service per
	// call the same way. Without this registration GetKgService() returns the
	// failing stub, so the tool could only ever report an infrastructure error.
	// The seed encoder is left unset: the walk then seeds by keyword match on the
	// entity rows, the documented fallback on a deployment without an embedder.
	agentic_rag.SetKgService(agentic_rag.NewKgAdapter(docEngine))
	common.Info("agent: smart-reasoning compiled-knowledge graph service installed")
	// The smart-reasoning agent's corpus tools resolve the shared retrieval
	// service per call, so the production adapter must be registered before the
	// server starts serving.
	runtime.SetRetrievalService(retrievalbridge.NewRuntimeAdapter(retrievalAdapter))
	common.Info("agent: runtime retrieval service wired")
}
