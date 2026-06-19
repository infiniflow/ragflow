//go:build ignore

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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"ragflow/internal/common"
	"ragflow/internal/engine/redis"
	"ragflow/internal/server"
	"ragflow/internal/server/local"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/agent/audio"
	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/handler"
	"ragflow/internal/router"
	"ragflow/internal/service"
	"ragflow/internal/service/chunk"
	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"
)

func printHelp() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "RAGFlow Server - Open-source RAG engine based on deep document understanding\n\n")
	fmt.Fprintf(os.Stderr, "Options:\n")
	fmt.Fprintf(os.Stderr, "  -p, --port int\t\tServer port (overrides config file)\n")
	fmt.Fprintf(os.Stderr, "  -v, --version  \tPrint version information and exit\n")
	fmt.Fprintf(os.Stderr, "  --debug        \tEnable debug-level logging\n")
	fmt.Fprintf(os.Stderr, "  -h, --help     \tShow this help message and exit\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  %s           \t\t# Start server with config file port\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s -p 8080   \t\t# Start server on port 8080\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s --port 8080 \t# Start server on port 8080\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s --version  \t# Show version and exit\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s --debug    \t# Start server with debug logging\n", os.Args[0])
}

func main() {
	// Parse command line flags
	var portFlag int
	flag.IntVar(&portFlag, "port", 0, "Server port (overrides config file)")
	flag.IntVar(&portFlag, "p", 0, "Server port (shorthand, overrides config file)")
	var debugFlag bool
	flag.BoolVar(&debugFlag, "debug", false, "Enable debug-level logging")
	var versionFlag bool
	flag.BoolVar(&versionFlag, "version", false, "Print version information and exit")

	// Custom help message
	flag.Usage = printHelp

	flag.Parse()

	// Handle --version flag: print version and exit immediately
	if versionFlag {
		fmt.Printf("RAGFlow version: %s\n", utility.GetRAGFlowVersion())
		return
	}

	// Initialize logger with default level
	// logger.Init("info"); // set debug log level
	if err := common.Init("info", "server_main.log"); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	// Initialize configuration
	if err := server.Init(""); err != nil {
		common.Fatal("Failed to initialize config", zap.Error(err))
	}

	// Override port with command line argument if provided
	config := server.GetConfig()
	if portFlag > 0 {
		config.Server.Port = portFlag
		common.Info("Port overridden by command line argument", zap.Int("port", portFlag))
	}

	if config.Server.Port == 0 {
		common.Fatal("Server port is not configured. Please specify via --port flag or config file.")
	}

	// Reinitialize logger with configured level if different
	level := config.Log.Level
	if level == "" {
		level = "info"
	}

	if debugFlag {
		level = "debug"
	}

	if err := common.Init(level, "server_main.log"); err != nil {
		common.Error("Failed to reinitialize logger", err)
	}
	server.SetLogger(common.Logger)
	if config.Log.Level == "" {
		config.Log.Level = common.GetLevel()
	}

	common.Info("Server mode", zap.String("mode", config.Server.Mode))

	// Print all configuration settings
	server.PrintAll()

	// Initialize database
	if err := dao.InitDB(); err != nil {
		common.Fatal("Failed to initialize database", zap.Error(err))
	}

	// Initialize doc engine
	if err := engine.Init(&config.DocEngine); err != nil {
		common.Fatal("Failed to initialize doc engine", zap.Error(err))
	}
	defer engine.Close()

	// Initialize Redis cache
	if err := redis.Init(&config.Redis); err != nil {
		common.Fatal("Failed to initialize Redis", zap.Error(err))
	}
	defer redis.Close()

	if err := storage.InitStorageFactory(); err != nil {
		common.Fatal("Failed to initialize storage factory", zap.Error(err))
	}

	if err := engine.InitMessageQueueEngine(config.TaskExecutor.MessageQueueType); err != nil {
		common.Error("Failed to initialize message queue engine", err)
	}

	// Initialize server variables (runtime variables that can change during operation)
	// This must be done after Cache is initialized
	if err := server.InitVariables(redis.Get()); err != nil {
		common.Warn("Failed to initialize server variables from Redis, using defaults", zap.String("error", err.Error()))
	}

	// Initialize admin status (default: unavailable=1)
	local.InitAdminStatus(1, "admin server not connected")

	// Initialize tokenizer (rag_analyzer)
	dictPath := os.Getenv("RAGFLOW_DICT_PATH")
	if dictPath == "" {
		dictPath = "/usr/share/infinity/resource"
	}
	tokenizerCfg := &tokenizer.PoolConfig{
		DictPath: dictPath,
	}
	if err := tokenizer.Init(tokenizerCfg); err != nil {
		common.Fatal("Failed to initialize tokenizer", zap.Error(err))
	}
	defer tokenizer.Close()

	// Initialize global QueryBuilder using tokenizer's DictPath
	// This ensures the Synonym uses the same wordnet directory as tokenizer
	if err := nlp.InitQueryBuilderFromTokenizer(tokenizerCfg.DictPath); err != nil {
		common.Fatal("Failed to initialize query builder", zap.Error(err))
	}

	startServer(config)

	common.Info("Server exited")
}

func startServer(config *server.Config) {

	// Set Gin mode
	if config.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// Initialize service layer
	userService := service.NewUserService()
	documentService := service.NewDocumentService()
	datasetsService := service.NewDatasetService()
	knowledgebaseService := service.NewKnowledgebaseService()
	metadataService := service.NewMetadataService()
	chunkService := chunk.NewChunkService()
	llmService := service.NewLLMService()
	tenantService := service.NewTenantService()
	chatService := service.NewChatService()
	chatSessionService := service.NewChatSessionService()
	openaiChatService := service.NewOpenAIChatService()
	systemService := service.NewSystemService()
	connectorService := service.NewConnectorService()
	searchService := service.NewSearchService()
	fileService := service.NewFileService()
	memoryService := service.NewMemoryService()
	mcpService := service.NewMCPService()
	modelProviderService := service.NewModelProviderService()

	// Initialize doc engine for skill search
	docEngine := engine.Get()

	// Initialize handler layer
	authHandler := handler.NewAuthHandler()
	userHandler := handler.NewUserHandler(userService)
	tenantHandler := handler.NewTenantHandler(tenantService, userService, knowledgebaseService)
	documentHandler := handler.NewDocumentHandler(documentService, datasetsService)
	datasetsHandler := handler.NewDatasetsHandler(datasetsService, metadataService)
	systemHandler := handler.NewSystemHandler(systemService)
	knowledgebaseHandler := handler.NewKnowledgebaseHandler(knowledgebaseService, userService, documentService)
	chunkHandler := handler.NewChunkHandler(chunkService, userService)
	llmHandler := handler.NewLLMHandler(llmService, userService)
	chatHandler := handler.NewChatHandler(chatService, userService)
	chatSessionHandler := handler.NewChatSessionHandler(chatSessionService, userService)
	openaiChatHandler := handler.NewOpenAIChatHandler(openaiChatService)
	connectorHandler := handler.NewConnectorHandler(connectorService, userService)
	searchHandler := handler.NewSearchHandler(searchService, userService)
	fileHandler := handler.NewFileHandler(fileService, userService)
	memoryHandler := handler.NewMemoryHandler(memoryService)
	mcpHandler := handler.NewMCPHandler(mcpService)
	skillSearchHandler := handler.NewSkillSearchHandler(docEngine)
	providerHandler := handler.NewProviderHandler(userService, modelProviderService)
	// Install the agent service's Redis-backed run infrastructure
	// (CheckPointStore / StateSerializer / RunTracker). When Redis
	// is unreachable (degraded boot, stand-alone mode, no-redis CI)
	// the constructors return errors and we fall through to the
	// in-memory / no-tracking path: the agent service treats nil
	// options as the in-memory test path, so graceful degradation
	// is a 1-line if-not-nil pass-through — no separate "boot" mode
	// required.
	agentOpts := buildAgentRunOptions()
	agentHandler := handler.NewAgentHandler(service.NewAgentServiceWithOptions(
		agentOpts.checkpointStore,
		agentOpts.stateSerializer,
		agentOpts.runTracker,
	), fileService)

	// Wire the TTS synthesizer to the per-tenant model-provider
	// dispatch. SynthesizeRequest is routed through
	// ModelProviderService.AudioSpeech, which fans out to the
	// tenant's configured TTS model driver. When the model
	// provider is unconfigured, the synthesizer falls back to a
	// no-op echo (the audio package contract), so this is always
	// safe to call.
	configureTTSSynthesizer(modelProviderService)
	searchBotLLM := &handler.SearchBotRealLLM{Svc: modelProviderService}
	searchBotHandler := handler.NewSearchBotHandler(
		searchService,
		tenantService,
		searchBotLLM,
		chunkService,
	)
	searchBotHandler.SetStreamLLM(searchBotLLM)
	searchBotHandler.SetAskService(service.NewAskService(chunkService, nil, 0, 0))
	pluginHandler := handler.NewPluginHandler(service.NewPluginService())
	modelHandler := handler.NewModelHandler(service.NewModelProviderService())
	fileCommitHandler := handler.NewFileCommitHandler(service.NewFileCommitService())

	// Dify retrieval handler
	docDAO := dao.NewDocumentDAO()
	retrievalService := nlp.NewRetrievalService(docEngine, docDAO)
	difyRetrievalHandler := handler.NewDifyRetrievalHandler(
		knowledgebaseService,
		modelProviderService,
		metadataService,
		retrievalService,
		docDAO,
		docEngine,
	)

	// Per-tenant canvas-runtime override selector, backed by the
	// existing Redis client and the global logger. The handler is
	// ALWAYS constructed, even when Redis is briefly unavailable at
	// startup, so the POST /api/v1/admin/canvas-runtime/:tenant_id
	// endpoint stays registered and returns the explicit
	// ErrSelectorNotConfigured (HTTP 500) path until Redis recovers.
	// Skipping handler construction when rdb == nil silently removed
	// the route until the next process restart, so a transient
	// Redis blip at boot stranded canary operators with a 404 they
	// could not diagnose from the client side. Keep the route hot.
	var adminRuntimeSelector *runtime.Selector
	if rdb := redis.Get().GetClient(); rdb != nil {
		adminRuntimeSelector = runtime.NewSelector(rdb, common.Logger)
	}
	adminRuntimeHandler := handler.NewAdminRuntimeHandler(adminRuntimeSelector)

	// Initialize router
	r := router.NewRouter(authHandler, userHandler, tenantHandler, documentHandler, datasetsHandler, systemHandler, knowledgebaseHandler, chunkHandler, llmHandler, chatHandler, chatSessionHandler, connectorHandler, searchHandler, fileHandler, memoryHandler, mcpHandler, skillSearchHandler, providerHandler, agentHandler, searchBotHandler, difyRetrievalHandler, pluginHandler, modelHandler, fileCommitHandler, adminRuntimeHandler, openaiChatHandler)

	// Create Gin engine
	ginEngine := gin.New()

	// Middleware
	if config.Server.Mode == "debug" {
		ginEngine.Use(gin.Logger())
	}
	ginEngine.Use(gin.Recovery())

	// Setup routes
	r.Setup(ginEngine)

	// Create HTTP server with timeouts to prevent slow clients from blocking shutdown
	addr := fmt.Sprintf(":%d", config.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           ginEngine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		common.Info(
			"\n        ____   ___    ______ ______ __\n" +
				"       / __ \\ /   |  / ____// ____// /____  _      __\n" +
				"      / /_/ // /| | / / __ / /_   / // __ \\| | /| / /\n" +
				"     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /\n" +
				"    /_/ |_|/_/  |_|\\____//_/    /_/ \\____/ |__/|__/\n",
		)
		common.Info(fmt.Sprintf("RAGFlow Go Version: %s", utility.GetRAGFlowVersion()))
		common.Info(fmt.Sprintf("Server starting on port: %d", config.Server.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			common.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Get local IP address for heartbeat reporting
	localIP, err := utility.GetLocalIP()
	if err != nil {
		common.Fatal("fail to get local ip address")
	}

	// Initialize and start heartbeat reporter to admin server
	service.AdminServiceClient = service.NewAdminClient(
		common.Logger,
		common.ServerTypeAPI,
		fmt.Sprintf("ragflow-server-%d", config.Server.Port),
		localIP,
		config.Server.Port,
	)
	if err = service.AdminServiceClient.InitHTTPClient(); err != nil {
		common.Warn("Failed to initialize heartbeat service", zap.Error(err))
	} else {
		// Start heartbeat reporter with 30 seconds interval
		heartbeatReporter := utility.NewScheduledTask("Heartbeat reporter", 3*time.Second, func() {
			if err = service.AdminServiceClient.SendHeartbeat(); err == nil {
				local.SetAdminStatus(0, "")
			} else {
				local.SetAdminStatus(1, err.Error())
				//logger.Warn(fmt.Sprintf(err.Error()))
			}
		})
		heartbeatReporter.Start()
		defer heartbeatReporter.Stop()
	}

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR2)
	sig := <-quit

	common.Info(fmt.Sprintf("Receives %s signal to shutdown server", strings.ToUpper(sig.String())))
	common.Info("Shutting down server...")

	// Create context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server
	if err = srv.Shutdown(ctx); err != nil {
		common.Fatal("Server forced to shutdown", zap.Error(err))
	}
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

// buildAgentRunOptions installs the Redis-backed run infrastructure
// when Redis is available. The Redis client is the one already
// initialised at the top of main; the TTL is a conservative 24h for
// both the checkpoint store and the run tracker. On any error
// (Redis down at boot, constructor panic, nil-Redis fallback) we
// log and return a zero-value struct — the agent service falls back
// to the in-memory path transparently.
func buildAgentRunOptions() agentRunOptions {
	var out agentRunOptions
	if !redis.IsEnabled() || redis.Get() == nil {
		common.Info("agent: redis client not initialised; agent run infra in in-memory mode (no checkpoints, no run tracker)")
		return out
	}
	cp := canvas.NewRedisCheckPointStore(24 * time.Hour)
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
func configureTTSSynthesizer(modelProviderService *service.ModelProviderService) {
	if modelProviderService == nil {
		common.Info("agent: model provider service not initialised; TTS in no-op echo mode")
		audio.SetModelProviderSynthesizer(nil)
		return
	}
	audio.SetModelProviderSynthesizer(audio.NewTTSDispatchFunc(modelProviderService))
	common.Info("agent: TTS model-provider dispatch installed (audio.Synthesize → ModelProviderService.AudioSpeech)")
}
