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

	"ragflow/internal/handler"
	"ragflow/internal/router"
	"ragflow/internal/service"
	"ragflow/internal/service/chunk"
	"ragflow/internal/service/dataset"
	"ragflow/internal/service/document"
	"ragflow/internal/service/file"
	"ragflow/internal/service/nlp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/server"
	"ragflow/internal/server/bootstrap"
)

func Run(ctx context.Context, serverName string, arguments *bootstrap.ServerArgs, agentServices *bootstrap.AgentServices) error {
	if !kvrocks.IsEnabled() {
		return fmt.Errorf("API requires Kvrocks for shared channel runtime status")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	globalConfig := server.GetConfig()

	// Install the mode-8 Codex configuration before any chat path reads it. A
	// missing endpoint keeps mode 8 disabled (the engine degrades to the
	// regular pipeline).
	apiServerCfg := globalConfig.GetAPIServerConfig()
	service.SetCodexConfig(&apiServerCfg.Codex)
	// Bind the mode 8 MCP handle/binding store to the shared Kvrocks, so
	// authorization survives across the processes that mint (API) and resolve
	// (/mcp/codex).
	service.InstallCodexKVStore()
	// Install the bridge's web-search seam: the /mcp/codex handler rebuilds a
	// conversation's provider callback on demand (only for turns that enabled
	// web search), so the credentials never enter the shared KV.
	service.InstallCodexWebSearchResolver()

	// Initialize service layer
	userService := service.NewUserService()
	userTenantService := service.NewUserTenantService()
	documentService := document.NewDocumentService()
	datasetsService := dataset.NewDatasetService()
	metadataService := agentServices.MetadataService
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
	fileService := file.NewFileService(documentService)
	memoryService := agentServices.MemoryService
	mcpService := service.NewMCPService()
	modelProviderService := service.NewModelProviderService()
	modelFactory := agentServices.ModelFactory
	modelCallService := agentServices.ModelCallService
	docEngine := engine.Get()
	documentDAO := dao.NewDocumentDAO()

	// Initialize handler layer
	authHandler := handler.NewAuthHandler()
	userHandler := handler.NewUserHandler(userService, userTenantService)
	tenantHandler := handler.NewTenantHandler(tenantService, userTenantService, datasetsService)
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
	agentService := agentServices.AgentService
	agentHandler := handler.NewAgentHandler(ctx, agentService, fileService)

	// Public chatbot/agentbot endpoints (api/v1/chatbots/...,
	// api/v1/agentbots/...) and the agent attachment download.
	// BotService delegates the agentBot completion to agentService so
	// both paths share the same canvas runner.
	botService := service.NewBotService(agentService)
	botHandler := handler.NewBotHandler(botService)

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
		return fmt.Errorf("configure trusted proxies: %w", err)
	}

	// Middleware
	// Note: common.GinLogger() is registered inside router.Setup so the
	// HTTP request log captures every endpoint the router owns (including
	// those registered by Setup itself). Registering it here would run
	// it twice for those endpoints and double every access-log line.
	ginEngine.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		if !c.Writer.Written() {
			common.Respond(c, http.StatusInternalServerError, common.CodeServerError, nil, fmt.Sprint(recovered))
		}
		c.Abort()
	}))

	// Setup routes
	r.Setup(ginEngine)

	cache := service.NewChatChannelRuntimeCache(kvrocks.Get().GetClient())
	service.SetChatChannelRuntimeProvider(cache.Read)

	apiServerConfig := globalConfig.GetAPIServerConfig()

	requestCtx, cancelRequests := newRequestContext(ctx)
	defer cancelRequests()

	// Create HTTP server with timeouts to prevent slow clients from blocking shutdown
	addr := fmt.Sprintf(":%d", apiServerConfig.HTTPPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           ginEngine,
		BaseContext:       func(net.Listener) context.Context { return requestCtx },
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
		common.Info(fmt.Sprintf("MCP server starting on %s", mcpSrv.Addr))
		go serve("MCP", mcpSrv, mcpListener)
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
		defer hb.Wait()
		defer hb.Stop()
	}
	if arguments.Ready != nil {
		if err = arguments.Ready(); err != nil {
			srv.Close()
			if mcpSrv != nil {
				mcpSrv.Close()
			}
			return fmt.Errorf("report API readiness: %w", err)
		}
	}

	// Wait for either shutdown signal or serving failure.
	var runErr error
	select {
	case <-ctx.Done():
		common.Info("Received shutdown signal")
	case err = <-serveErr:
		runErr = err
		common.Error("Server failed; shutting down", err)
	}
	common.Info("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
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

	if runErr == nil {
		common.Info("Server exited successfully")
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
