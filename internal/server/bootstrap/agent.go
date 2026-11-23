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
	"fmt"
	"time"

	"ragflow/internal/agent/audio"
	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/component"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/ingestion/task"
	_ "ragflow/internal/ingestion/wire"
	"ragflow/internal/service"
	"ragflow/internal/service/nav"
	"ragflow/internal/service/nlp"
	"ragflow/internal/service/wikisearch"
	"ragflow/internal/tokenizer"
)

// AgentServices holds the production dependencies shared by API and chatter.
type AgentServices struct {
	AgentService     *service.AgentService
	MetadataService  *service.MetadataService
	MemoryService    *service.MemoryService
	ModelFactory     *service.ModelFactory
	ModelCallService *service.ModelCallService
}

// InitializeAgentTokenizer initializes token counting and retrieval query parsing.
// The caller closes the tokenizer after its agent runtime stops.
func InitializeAgentTokenizer() (func(), error) {
	cfg := &tokenizer.PoolConfig{}
	if err := tokenizer.Init(cfg); err != nil {
		return nil, fmt.Errorf("initialize tokenizer: %w", err)
	}
	if err := tokenizer.InitCL100KEncoder(); err != nil {
		tokenizer.Close()
		return nil, fmt.Errorf("initialize cl100k_base tokenizer: %w", err)
	}
	if err := nlp.InitQueryBuilderFromTokenizer(cfg.DictPath); err != nil {
		tokenizer.Close()
		return nil, fmt.Errorf("initialize query builder: %w", err)
	}
	return tokenizer.Close, nil
}

// NewAgentServices installs the production canvas, tool and model dependencies.
func NewAgentServices() *AgentServices {
	services := &AgentServices{
		MetadataService:  service.NewMetadataService(),
		MemoryService:    service.NewMemoryService(dao.DB, engine.Get(), service.WithMemoryKvrocks(kvrocks.Get()), service.WithMemoryMQEngine(engine.GetMessageQueueEngine())),
		ModelFactory:     service.NewModelFactory(),
		ModelCallService: service.NewModelCallService(),
	}
	component.SetMemorySaver(service.NewMemorySaverAdapter(services.MemoryService))
	task.InitEmbedderResolver(dao.DB)
	task.InitKnowledgeCompilerDeps(dao.DB, engine.Get())
	InitializeRetrievalServices(services.MetadataService, services.MemoryService, services.ModelFactory)
	opts := buildAgentRunOptions()
	// Eino's default serializer preserves typed checkpoint channels during resume.
	services.AgentService = service.NewAgentServiceWithOptions(opts.checkpointStore, nil, opts.runTracker)
	configureTTSSynthesizer(services.ModelCallService)
	component.InstallDefaultChatInvoker()
	navService := nlp.NewNavService(service.NewNavEmbedder(services.ModelFactory, ""))
	navService.SetNavMergeLLM(service.NewNavMergeLLM(services.ModelFactory, ""))
	nav.SetNavService(navService)
	wikisearch.SetService(wikisearch.NewEngineService(engine.Get()))
	return services
}

// agentRunOptions uses Kvrocks when available, otherwise the agent runs in memory.
type agentRunOptions struct {
	checkpointStore canvas.CheckPointStore
	runTracker      *canvas.RunTracker
}

func buildAgentRunOptions() agentRunOptions {
	var out agentRunOptions
	if !kvrocks.IsEnabled() || kvrocks.Get() == nil {
		common.Info("agent: redis client not initialised; agent run infra in in-memory mode (no checkpoints, no run tracker)")
		return out
	}
	cp := canvas.NewKvrocksCheckPointStore(24 * time.Hour)
	out.checkpointStore = cp
	rt := canvas.NewRunTracker(24 * time.Hour)
	out.runTracker = rt
	common.Info("agent: redis-backed run infra installed (24h TTL on checkpoint store + run tracker; eino default serializer)")
	return out
}

func configureTTSSynthesizer(modelCallService *service.ModelCallService) {
	if modelCallService == nil {
		common.Info("agent: model call service not initialised; TTS in no-op echo mode")
		audio.SetModelProviderSynthesizer(nil)
		return
	}
	audio.SetModelProviderSynthesizer(audio.NewTTSDispatchFunc(modelCallService))
	common.Info("agent: TTS model-call dispatch installed (audio.Synthesize → ModelCallService.AudioSpeech)")
}
