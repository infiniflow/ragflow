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

package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"ragflow/internal/agent/audio"
	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/retrievalbridge"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	agentic_rag "ragflow/internal/agentic_rag"
	"ragflow/internal/channels"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/handler"
	"ragflow/internal/router"
	"ragflow/internal/server/local"
	"ragflow/internal/service"
	"ragflow/internal/service/chunk"
	"ragflow/internal/service/dataset"
	"ragflow/internal/service/document"
	"ragflow/internal/service/file"
	"ragflow/internal/service/nav"
	"ragflow/internal/service/nlp"
	"ragflow/internal/service/wikisearch"
	"ragflow/internal/tokenizer"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/agent/component"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/ingestion/task"
	_ "ragflow/internal/ingestion/wire"
	"ragflow/internal/server"
	"ragflow/internal/server/bootstrap"
)

func Run(ctx context.Context, serverName string, args *bootstrap.ServerArgs) error {
	// Initialize admin status (default: unavailable=1)
	local.InitAdminStatus(1, "admin server not connected")

	// Initialize tokenizer (rag_analyzer)
	// tokenizer.Init fills DictPath from env var or default, so
	// tokenizerCfg.DictPath carries the resolved path for downstream use.
	tokenizerCfg := &tokenizer.PoolConfig{}
	if err := tokenizer.Init(tokenizerCfg); err != nil {
		common.Fatal("Failed to initialize tokenizer", zap.Error(err))
	}
	defer tokenizer.Close()

	// Fail fast if the cl100k_base BPE table is missing. NumTokensFromString /
	// TrimContentToTokenLimit now panic rather than degrading silently, so this
	// trades a mid-request panic for a clear startup failure.
	if err := tokenizer.InitCL100KEncoder(); err != nil {
		common.Fatal("Failed to initialize cl100k_base tokenizer", zap.Error(err))
	}

	// Initialize global QueryBuilder using tokenizer's DictPath
	// This ensures the Synonym uses the same wordnet directory as tokenizer
	if err := nlp.InitQueryBuilderFromTokenizer(tokenizerCfg.DictPath); err != nil {
		common.Fatal("Failed to initialize query builder", zap.Error(err))
	}

	if err := startServer(ctx, serverName, args); err != nil {
		return err
	}

	common.Info("Server exited")

	return nil
}

func startServer(ctx context.Context, serverName string, arguments *bootstrap.ServerArgs) error {

	globalConfig := server.GetConfig()
	serverMode := globalConfig.GetMode()
	// Set Gin mode
	if serverMode == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize service layer
	userService := service.NewUserService()
	documentService := document.NewDocumentService()
	datasetsService := dataset.NewDatasetService()
	metadataService := service.NewMetadataService()
	chunkService := chunk.NewChunkService()
	tenantService := service.NewTenantService()
	chatService := service.NewChatService()
	chatChannelService := service.NewChatChannelService()
	langfuseService := service.NewLangfuseService()
	chatSessionService := service.NewChatSessionService()
	openaiChatService := service.NewOpenAIChatService()
	systemService := service.NewSystemService()
	statsService := service.NewStatsService()
	connectorService := service.NewConnectorService()
	searchService := service.NewSearchService()
	searchService.SetTenantService(tenantService)
	fileService := file.NewFileService(documentService)
	memoryService := service.NewMemoryService(dao.DB, engine.Get(), service.WithMemoryKvrocks(kvrocks.Get()), service.WithMemoryMQEngine(engine.GetMessageQueueEngine()))
	mcpService := service.NewMCPService()
	modelProviderService := service.NewModelProviderService()
	modelFactory := service.NewModelFactory()
	modelCallService := service.NewModelCallServiceWithProviderService(modelProviderService, modelFactory)

	// Wire the real MemorySaver so the Message component can persist
	// conversation turns to memory stores declared in the canvas DSL.
	component.SetMemorySaver(service.NewMemorySaverAdapter(memoryService))
	// Bind the relational handle and document engine into the production
	// embedder and knowledge-compiler resolvers. The API process shares the
	// task-package composition root with the ingestor: without these, the
	// embedder resolver would read a nil db and fail at run time when the API
	// process runs ingestion runs (e.g. in a combined --api --ingestor deploy
	// where the ingestor bindings do not apply).
	task.InitEmbedderResolver(dao.DB)
	task.InitKnowledgeCompilerDeps(dao.DB, engine.Get())

	// Initialize doc engine for skill search
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
				access := service.ModelAccess{TenantID: tenantID}
				if strings.TrimSpace(modelRef) == "" {
					return modelFactory.NewDefaultEmbeddingModel(ctx, access)
				}
				return modelFactory.NewEmbeddingModel(ctx, access, modelRef)
			},
			Chat: func(ctx context.Context, tenantID, modelRef string) (*modelModule.ChatModel, error) {
				access := service.ModelAccess{TenantID: tenantID}
				if strings.TrimSpace(modelRef) == "" {
					return modelFactory.NewDefaultChatModel(ctx, access)
				}
				return modelFactory.NewChatModel(ctx, access, modelRef)
			},
			Rerank: func(ctx context.Context, tenantID, modelRef string) (*modelModule.RerankModel, error) {
				return modelFactory.NewRerankModel(ctx, service.ModelAccess{TenantID: tenantID}, modelRef)
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

	// Initialize handler layer
	authHandler := handler.NewAuthHandler()
	userHandler := handler.NewUserHandler(userService)
	tenantHandler := handler.NewTenantHandler(tenantService, userService, datasetsService)
	documentHandler := handler.NewDocumentHandler(documentService, datasetsService, fileService)
	datasetsHandler := handler.NewDatasetsHandler(datasetsService, metadataService)
	systemHandler := handler.NewSystemHandler(systemService)
	statsHandler := handler.NewStatsHandler(statsService)
	chunkHandler := handler.NewChunkHandler(chunkService, userService)
	chatHandler := handler.NewChatHandler(chatService, userService)
	chatChannelHandler := handler.NewChatChannelHandler(chatChannelService)
	langfuseHandler := handler.NewLangfuseHandler(langfuseService)
	chatSessionHandler := handler.NewChatSessionHandler(chatSessionService, userService)
	openaiChatHandler := handler.NewOpenAIChatHandler(openaiChatService)
	connectorHandler := handler.NewConnectorHandler(connectorService, userService)
	searchHandler := handler.NewSearchHandler(searchService, userService)
	fileHandler := handler.NewFileHandler(fileService, userService)
	memoryHandler := handler.NewMemoryHandler(memoryService)
	mcpHandler := handler.NewMCPHandler(mcpService)

	// MCP server endpoint — exposes RAGFlow capabilities as MCP tools
	// (ragflow_retrieval, ragflow_list_datasets, ragflow_list_chats) to
	// external AI clients via JSON-RPC over HTTP.
	mcpServerHandler := handler.NewMCPServerHandler(datasetsService, chatService)
	skillSearchHandler := handler.NewSkillSearchHandler(docEngine, documentService)
	providerHandler := handler.NewProviderHandler(userService, modelProviderService, modelCallService)
	// Install the agent service's Kvrocks-backed run infrastructure
	// (CheckPointStore / StateSerializer / RunTracker). When Redis
	// is unreachable (degraded boot, stand-alone mode, no-redis CI)
	// the constructors return errors, and we fall through to the
	// in-memory / no-tracking path: the agent service treats nil
	// options as the in-memory test path, so graceful degradation
	// is a 1-line if-not-nil pass-through — no separate "boot" mode
	// required.
	agentOpts := buildAgentRunOptions()
	agentService := service.NewAgentServiceWithOptions(
		agentOpts.checkpointStore,
		agentOpts.stateSerializer,
		agentOpts.runTracker,
	)
	agentHandler := handler.NewAgentHandler(ctx, agentService, fileService)

	// Public chatbot/agentbot endpoints (api/v1/chatbots/...,
	// api/v1/agentbots/...) and the agent attachment download.
	// BotService delegates the agentBot completion to agentService so
	// both paths share the same canvas runner.
	botService := service.NewBotService(agentService)
	botHandler := handler.NewBotHandler(botService)

	// Wire the TTS synthesizer to the per-tenant model-provider
	// dispatch. SynthesizeRequest is routed through
	// ModelProviderService.AudioSpeech, which fans out to the
	// tenant's configured TTS model driver. When the model
	// provider is unconfigured, the synthesizer falls back to a
	// no-op echo (the audio package contract), so this is always
	// safe to call.
	configureTTSSynthesizer(modelCallService)
	searchBotHandler := handler.NewSearchBotHandler(
		searchService,
		tenantService,
		modelFactory,
		chunkService,
	)
	askService := service.NewAskService(chunkService, nil, 0, 0)
	searchBotHandler.SetAskService(askService)
	chatHandler.SetMindMapDependencies(searchService, tenantService, modelFactory, chunkService)
	searchHandler.SetCompletionDependencies(modelFactory, askService)
	pluginHandler := handler.NewPluginHandler(service.NewPluginService())
	modelHandler := handler.NewModelHandler(service.NewModelProviderService())
	fileCommitHandler := handler.NewFileCommitHandler(file.NewFileCommitService())

	// Dify retrieval handler
	retrievalService := nlp.NewRetrievalService(docEngine, documentDAO)
	difyRetrievalHandler := handler.NewDifyRetrievalHandler(
		datasetsService,
		modelFactory,
		metadataService,
		retrievalService,
		documentDAO,
		docEngine,
	)
	componentsSvc := service.NewComponentsService()
	componentsHandler := handler.NewComponentsHandler(componentsSvc)
	pipelineHandler := handler.NewPipelineHandler()
	compilationTemplateHandler := handler.NewCompilationTemplateHandler(service.NewCompilationTemplateService())
	compilationTemplateGroupHandler := handler.NewCompilationTemplateGroupHandler(service.NewCompilationTemplateGroupService())
	datasetArtifactHandler := handler.NewDatasetArtifactHandler(service.NewDatasetArtifactService(), datasetsService, file.NewFileCommitService())

	// Install the production eino-based chat invoker as the shared chat default,
	// so agentic-search runtime LLM calls work in production. Without this,
	// chat.GetDefaultInvoker() stays nil and the runtime falls back gracefully.
	component.InstallDefaultChatInvoker()

	// Install the dataset-nav ES-backed service (internal/service/nav +
	// internal/service/nlp). The embedder resolves the tenant's embedding model
	// on demand so Search/UpsertDoc can embed queries/summaries automatically,
	// and the cluster namer/merger resolves the tenant's default chat model the
	// same way (see the ingestor's wrapping of SetNavMergeLLM).
	apiNavService := nlp.NewNavService(service.NewNavEmbedder(modelFactory, ""))
	apiNavService.SetNavMergeLLM(service.NewNavMergeLLM(modelFactory, ""))
	nav.SetNavService(apiNavService)

	// Install the compiled-wiki search service. It is backed directly by the
	// document engine: QueryPages filters the tenant-scoped index to
	// compile_kwd="wiki_page" (+ supported kinds) so ordinary source chunks are
	// never relabeled as wiki pages, and BackfillChunks fetches original chunks
	// by id. When the engine is unavailable the service degrades to empty so the
	// agent falls back to hybrid search (no failing call).
	wikisearch.SetService(wikisearch.NewEngineService(engine.Get()))

	// Initialize router
	r := router.NewRouter(authHandler,
		userHandler,
		tenantHandler,
		documentHandler,
		datasetsHandler,
		systemHandler,
		statsHandler,
		chunkHandler,
		chatHandler,
		chatChannelHandler,
		langfuseHandler,
		chatSessionHandler,
		connectorHandler,
		searchHandler,
		fileHandler,
		memoryHandler,
		mcpHandler,
		mcpServerHandler,
		skillSearchHandler,
		providerHandler,
		agentHandler,
		searchBotHandler,
		difyRetrievalHandler,
		pluginHandler,
		modelHandler,
		fileCommitHandler,
		openaiChatHandler,
		botHandler,
		componentsHandler,
		pipelineHandler,
		compilationTemplateHandler,
		compilationTemplateGroupHandler,
		datasetArtifactHandler)

	// Create Gin engine
	ginEngine := gin.New()
	// Mirror Quart's merge_slashes: collapse duplicate slashes before routing.
	ginEngine.RemoveExtraSlash = true
	// Only honor X-Forwarded-For / X-Real-IP from the configured proxies
	// (default: the loopback nginx bundled in the image), never from every
	// peer. c.ClientIP() feeds the agent webhook ip_whitelist gate and the
	// login audit records, so gin's trust-everything default would let any
	// caller pick its own address.
	if err := common.ConfigureTrustedProxies(ginEngine, globalConfig.GetAPIServerConfig().TrustedProxies); err != nil {
		common.Fatal("Failed to configure trusted proxies", zap.Error(err))
	}

	// Middleware
	// Note: common.GinLogger() is registered inside router.Setup so the
	// HTTP request log captures every endpoint the router owns (including
	// those registered by Setup itself). Registering it here would run
	// it twice for those endpoints and double every access-log line.
	ginEngine.Use(gin.Recovery())

	// Setup routes
	r.Setup(ginEngine)

	_, err := channels.Start(ctx)
	if err != nil {
		return fmt.Errorf("start chat-channel: %w", err)
	}

	apiServerConfig := globalConfig.GetAPIServerConfig()

	// Create HTTP server with timeouts to prevent slow clients from blocking shutdown
	addr := fmt.Sprintf(":%d", apiServerConfig.HTTPPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           ginEngine,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		// WriteTimeout spans "request header read → response written", so it is a
		// ceiling on the WHOLE request, not on a slow client's reads. Measured
		// (2026-09-15, FRAMES 20q): three multi-hop questions take 150–225s to reach
		// their response, and at 120s the server closed the connection with no
		// response at all — the client reports
		// `RemoteDisconnected('Remote end closed connection without response')` and
		// the benchmark re-runs the whole question (max_retries: 2), so one slow
		// question cost three full pipelines. 180s clears the measured distribution's
		// middle; questions whose composition alone runs past it still need streaming
		// or a larger budget.
		WriteTimeout: 180 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	apiListener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen API server on %s: %w", addr, err)
	}
	defer apiListener.Close()

	serveErr := make(chan error, 2)
	serve := func(name string, srv *http.Server, listener net.Listener) {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("%s server failed: %w", name, err)
		}
	}

	var mcpSrv *http.Server
	var mcpCloser interface{ Close() error }
	var mcpListener net.Listener
	mcpConfig := apiServerConfig.MCP
	if mcpConfig.Enabled {
		mcpHandler, err := mcpServerHandler.NewStandalone(ctx, authHandler, mcpConfig)
		if err != nil {
			return err
		}

		mcpCloser = mcpHandler
		defer mcpHandler.Close()
		mcpAddr := net.JoinHostPort(strings.Trim(mcpConfig.Host, "[]"), strconv.Itoa(mcpConfig.Port))
		mcpListener, err = net.Listen("tcp", mcpAddr)
		if err != nil {
			return fmt.Errorf("listen MCP server on %s: %w", mcpAddr, err)
		}
		defer mcpListener.Close()
		mcpSrv = &http.Server{
			Addr:              mcpAddr,
			Handler:           mcpHandler,
			BaseContext:       func(net.Listener) context.Context { return ctx },
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      0, // SSE streams outlive individual tool deadlines
			IdleTimeout:       120 * time.Second,
		}
		go func() {
			common.Info(fmt.Sprintf("MCP server starting on %s", mcpSrv.Addr))
			serve("MCP", mcpSrv, mcpListener)
		}()
	}

	common.Info(
		"\n        ____   ___    ______ ______ __\n" +
			"       / __ \\ /   |  / ____// ____// /____  _      __\n" +
			"      / /_/ // /| | / / __ / /_   / // __ \\| | /| / /\n" +
			"     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /\n" +
			"    /_/ |_|/_/  |_|\\____//_/    /_/ \\____/ |__/|__/\n",
	)
	common.Info(fmt.Sprintf("RAGFlow Go Version: %s", common.GetRAGFlowVersion()))
	common.Info(fmt.Sprintf("Server starting on port: %d", apiServerConfig.HTTPPort))

	// Set log level
	bootstrap.SetLogger(serverName, arguments)

	// Start server in a goroutine
	go func() {
		serve(serverName, srv, apiListener)
	}()

	// Start heartbeat reporter to admin server
	if hb := bootstrap.StartHeartbeat(
		common.ServerTypeAPI,
		fmt.Sprintf("ragflow-server-%s", common.GenerateUUID()[:8]),
		apiServerConfig.HTTPPort,
		globalConfig.GetHeartbeatInterval(),
	); hb != nil {
		defer hb.Stop()
	}

	// Wait for either shutdown signal or serving failure.
	var runErr error
	select {
	case <-ctx.Done():
		common.Info("Received shutdown signal")
	case err := <-serveErr:
		runErr = err
		common.Error("Server failed; shutting down", err)
	}
	common.Info("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if mcpCloser != nil {
		if err = mcpCloser.Close(); err != nil {
			common.Warn("Failed to close MCP handler", zap.Error(err))
		}
	}
	if err = shutdownHTTPServer(shutdownCtx, "API", srv); err != nil {
		return err
	}
	if mcpSrv != nil {
		if err = shutdownHTTPServer(shutdownCtx, "MCP", mcpSrv); err != nil {
			return err
		}
	}
	return runErr
}

func shutdownHTTPServer(ctx context.Context, name string, srv *http.Server) error {
	if err := srv.Shutdown(ctx); err != nil {
		_ = srv.Close()
		return fmt.Errorf("shutdown %s server: %w", name, err)
	}
	return nil
}

// agentRunOptions bundles the three optional injection slots the
// agent service accepts via NewAgentServiceWithOptions: the Redis-
// backed CheckPointStore, StateSerializer, and RunTracker. The
// fields stay nil when the underlying constructors fail (Redis
// unreachable, etc.); the agent service treats nil as "in-memory
// / no-tracking" so the server continues to serve traffic without
// requiring Redis to be up.
type agentRunOptions struct {
	checkpointStore canvas.CheckPointStore
	stateSerializer canvas.StateSerializer
	runTracker      *canvas.RunTracker
}

// buildAgentRunOptions installs the Kvrocks-backed run infrastructure
// when Redis is available. The Redis client is the one already
// initialized at the top of main; the TTL is a conservative 24h for
// both the checkpoint store and the run tracker. On any error
// (Redis down at boot, constructor panic, nil-Redis fallback) we
// log and return a zero-value struct — the agent service falls back
// to the in-memory path transparently.
func buildAgentRunOptions() agentRunOptions {
	var out agentRunOptions
	if !kvrocks.IsEnabled() || kvrocks.Get() == nil {
		common.Info("agent: redis client not initialised; agent run infra in in-memory mode (no checkpoints, no run tracker)")
		return out
	}
	cp := canvas.NewKvrocksCheckPointStore(24 * time.Hour)
	out.checkpointStore = cp
	// stateSerializer is intentionally left nil. eino's default
	// InternalSerializer (used when no compose.WithSerializer is
	// passed at compile time) already knows how to round-trip
	// runtime.CanvasState because the runtime package registers
	// it via compose.RegisterSerializableType[CanvasState] in
	// init(). Overriding with RAGFlow's plain-JSON
	// CanvasStateSerializer (json.Marshal/Unmarshal) produces
	// bytes the InternalSerializer cannot decode on the resume
	// pass — the UserFillUp two-node pattern surfaces this as
	// "load checkpoint from store fail: cannot unmarshal object
	// into Go struct field checkpoint.Channels of type
	// compose.channel". Rely on eino's default instead.
	rt := canvas.NewRunTracker(24 * time.Hour)
	out.runTracker = rt
	common.Info("agent: redis-backed run infra installed (24h TTL on checkpoint store + run tracker; eino default serializer)")
	return out
}

// configureTTSSynthesizer installs the audio.ModelProviderFunc
// that dispatches Synthesize requests through the project's
// ModelProviderService. The model provider's AudioSpeech method
// (internal/service/model_service.go) resolves the per-tenant TTS
// model driver, sends the request upstream, and returns
// synthesized audio bytes.
//
// The audio package's NewTTSDispatchFunc helper converts the
// audio.SynthesizeRequest shape into the model's dispatch shape
// (audioContent = req.Text, voice/lang → TTSConfig.Params,
// ModelName from req.Engine). When the model provider is
// unconfigured (nil dispatcher) the helper returns nil, which
// reverts the audio package to its default stub.
func configureTTSSynthesizer(modelCallService *service.ModelCallService) {
	if modelCallService == nil {
		common.Info("agent: model call service not initialised; TTS in no-op echo mode")
		audio.SetModelProviderSynthesizer(nil)
		return
	}
	audio.SetModelProviderSynthesizer(audio.NewTTSDispatchFunc(modelCallService))
	common.Info("agent: TTS model-call dispatch installed (audio.Synthesize → ModelCallService.AudioSpeech)")
}
