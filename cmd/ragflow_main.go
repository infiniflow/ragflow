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
	"ragflow/internal/admin"
	"ragflow/internal/ingestion"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/agent/audio"
	"ragflow/internal/agent/canvas"
	_ "ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/redis"
	"ragflow/internal/handler"
	"ragflow/internal/router"
	"ragflow/internal/server"
	"ragflow/internal/server/local"
	"ragflow/internal/service"
	"ragflow/internal/service/chunk"
	"ragflow/internal/service/nlp"
	"ragflow/internal/utility"
)

type serverArgs struct {
	mode          *string // admin | api | ingestor
	helpFlag      bool
	versionFlag   bool
	debugLog      bool
	configPath    *string // Used by admin, api; user defined config path
	initSuperUser bool    // Used by admin;
	port          *int    // Used by admin, api
	adminHost     *string // Used by api and ingestor for heartbeat
	adminPort     *int    // Used by api and ingestor for heartbeat, "ip:port"
}

func parseArgs() (*serverArgs, error) {
	args := &serverArgs{}

	var serverMode string
	var configPath string
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch arg {
		case "--admin":
			serverMode = "admin"
			args.mode = &serverMode
		case "--ingestor":
			serverMode = "ingestor"
			args.mode = &serverMode
		case "--api":
			serverMode = "api"
			args.mode = &serverMode
		case "-h", "--help":
			args.helpFlag = true
		case "-v", "--version":
			args.versionFlag = true
		case "--debug":
			args.debugLog = true
		case "--config":
			configPath = arg
			args.configPath = &configPath
		case "--init-superuser":
			args.initSuperUser = true
		case "--port":
			port, convErr := strconv.Atoi(arg)
			if convErr != nil {
				return nil, fmt.Errorf("invalid port: %w", convErr)
			}
			args.port = &port
		case "--admin-host":
			adminHost := arg
			// split ip:port into ip and port
			ip, portStr := strings.SplitN(adminHost, ":", 2)[0], strings.SplitN(adminHost, ":", 2)[1]
			if len(portStr) == 0 {
				return nil, errors.New("--admin-host must be in the form 'ip:port'")
			}
			port, convErr := strconv.Atoi(portStr)
			if convErr != nil {
				return nil, fmt.Errorf("invalid admin port: %w", convErr)
			}
			args.adminHost = &ip
			args.adminPort = &port
		}
	}
	return args, nil
}

func printHelp(args *serverArgs) {
	switch {
	case args.mode == nil:
		fmt.Fprintf(os.Stderr, "Usage: %s --api|--admin|--ingestor [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Server - Open-source RAG engine based on deep document understanding\n\n")
		fmt.Fprintf(os.Stderr, "Mode selection (default: --api):\n")
		fmt.Fprintf(os.Stderr, "  --api          \tRun as API server\n")
		fmt.Fprintf(os.Stderr, "  --admin        \tRun as admin server\n")
		fmt.Fprintf(os.Stderr, "  --ingestor     \tRun as ingestion worker\n\n")
		fmt.Fprintf(os.Stderr, "Common options:\n")
		fmt.Fprintf(os.Stderr, "  --config string\tPath to configuration file\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug        \tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \tShow this help message and exit\n\n")
		fmt.Fprintf(os.Stderr, "Run '%s --api --help' for API server options.\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Run '%s --admin --help' for admin server options.\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Run '%s --ingestor --help' for ingester options.\n", os.Args[0])
	case *args.mode == "api":
		fmt.Fprintf(os.Stderr, "Usage: %s --api [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow API Server\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  --port int     \tServer port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  --config string\tPath to configuration file\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug        \tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \tShow this help message and exit\n")
	case *args.mode == "admin":
		fmt.Fprintf(os.Stderr, "Usage: %s --admin [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Admin Server\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  --config string\t\tPath to configuration file\n")
		fmt.Fprintf(os.Stderr, "  --port int    \t\tServer port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  --init-superuser\tInitialize superuser account\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug        \tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \tShow this help message and exit\n")
	case *args.mode == "ingestor":
		fmt.Fprintf(os.Stderr, "Usage: %s --ingestor [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Ingestion Worker - Document ingestion processing\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  --config string\t\tPath to config file\n")
		fmt.Fprintf(os.Stderr, "  --name string\t\t\tIngestion server name (default: \"default_ingestion\")\n")
		fmt.Fprintf(os.Stderr, "  --admin-host string\t\tAdmin server host:port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \t\tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug        \t\tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \t\tShow this help message and exit\n")
	}
}

func main() {
	arguments, err := parseArgs()
	if err != nil {
		fmt.Printf("Failed to parse arguments: %v\n", err)
		return
	}

	if arguments.versionFlag {
		fmt.Printf("RAGFlow version: %s\n", utility.GetRAGFlowVersion())
		return
	}

	if arguments.helpFlag {
		printHelp(arguments)
		return
	}
}

// ---------------------------------------------------------------------------
// Admin server
// ---------------------------------------------------------------------------

func runAdmin(configPath string, debugFlag,
	bool, initSuperuserFlag bool) {
	// Initialize logger
	if err := common.Init("info", common.FileOutput{Path: "admin_server.log"}); err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	// Initialize configuration
	if err := server.Init(configPath); err != nil {
		common.Error("Failed to initialize configuration", err)
		os.Exit(1)
	}

	cfg := server.GetConfig()

	// Reinitialize logger with configured level if different
	logLevel := cfg.Log.Level
	if logLevel == "" {
		logLevel = "info"
	}
	if debugFlag {
		logLevel = "debug"
	}

	fileOut := common.FileOutput{
		Path:       "admin_server.log",
		MaxSize:    cfg.Log.MaxSize,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAge:     cfg.Log.MaxAge,
		Compress:   common.ResolveCompress(cfg.Log.Compress),
	}
	if cfg.Log.Path != "" {
		fileOut.Path = cfg.Log.Path
	}
	if err := common.Init(logLevel, fileOut); err != nil {
		common.Error("Failed to reinitialize logger with configured level", err)
	}

	// Set logger for server package
	server.SetLogger(common.Logger)

	common.Info("Server mode", zap.String("mode", cfg.Server.Mode))

	// Set Gin mode
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// Initialize database
	if err := dao.InitDB(); err != nil {
		common.Error("Failed to initialize database", err)
		os.Exit(1)
	}

	// Initialize doc engine
	if err := engine.Init(&cfg.DocEngine); err != nil {
		common.Fatal("Failed to initialize doc engine", zap.Error(err))
	}
	defer engine.Close()

	// Initialize Redis cache
	if err := redis.Init(&cfg.Redis); err != nil {
		common.Fatal("Failed to initialize Redis", zap.Error(err))
	}
	defer redis.Close()

	if err := engine.InitMessageQueueEngine(cfg.TaskExecutor.MessageQueueType); err != nil {
		common.Error("Failed to initialize message queue engine", err)
	}

	// Initialize server variables (runtime variables that can change during operation)
	if err := server.InitVariables(redis.Get()); err != nil {
		common.Warn("Failed to initialize server variables from Redis, using defaults", zap.String("error", err.Error()))
	}

	adminService := admin.NewService()
	adminHandler := admin.NewHandler(adminService)

	if initSuperuserFlag {
		if err := adminService.InitDefaultAdmin(); err != nil {
			common.Error("Failed to initialize default admin user", err)
		}
	}

	// Initialize router
	r := admin.NewRouter(adminHandler)

	// Create Gin engine
	ginEngine := gin.New()

	// Middleware
	ginEngine.Use(common.GinLogger())
	ginEngine.Use(gin.Recovery())

	// Setup routes
	r.Setup(ginEngine)

	// Create HTTP server
	addr := fmt.Sprintf(":%d", cfg.Admin.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: ginEngine,
	}

	// Print all configuration settings
	server.PrintAll()

	// Print RAGFlow Admin logo
	common.Info("" +
		"\n        ____  ___   ______________                 ___       __          _     \n" +
		"       / __ \\/   | / ____/ ____/ /___ _      __   /   | ____/ /___ ___  (_)___ \n" +
		"      / /_/ / /| |/ / __/ /_  / / __ \\ | /| / /  / /| |/ __  / __ `__ \\/ / __ \\ \n" +
		"     / _, _/ ___ / /_/ / __/ / / /_/ / |/ |/ /  / ___ / /_/ / / / / / / / / / /\n" +
		"    /_/ |_/_/  |_\\____/_/   /_/\\____/|__/|__/  /_/  |_\\__,_/_/ /_/ /_/_/_/ /_/ \n")

	// Print RAGFlow version
	common.Info(fmt.Sprintf("RAGFlow admin version: %s", utility.GetRAGFlowVersion()))

	// Start HTTP server in a goroutine
	go func() {
		common.Info(fmt.Sprintf("Starting RAGFlow admin HTTP server on port: %d", cfg.Admin.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			common.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR2)
	sig := <-quit

	common.Info("Received signal", zap.String("signal", sig.String()))
	common.Info("Shutting down RAGFlow HTTP server...")

	// Create context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if err := srv.Shutdown(ctx); err != nil {
		common.Fatal("Server forced to shutdown", zap.Error(err))
	}

	common.Info("Admin HTTP server exited")
}

// ---------------------------------------------------------------------------
// Ingestion worker
// ---------------------------------------------------------------------------

func runIngestor(configPath string, debugFlag bool, name string, adminHostArg string, adminPortArg int) {
	// Initialize logger with default level
	if err := common.Init("info", common.FileOutput{Path: "ingestion_server.log"}); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	// Initialize configuration
	if err := server.Init(configPath); err != nil {
		common.Fatal("Failed to initialize config", zap.Error(err))
	}

	config := server.GetConfig()

	// Override admin server host with command line argument if provided
	if adminHostArg != "" {
		config.Admin.Host = adminHostArg
		common.Info("Admin host overridden by command line argument", zap.String("admin_host", adminHostArg))
	}

	// Override admin server port with command line argument if provided
	if adminPortArg > 0 {
		config.Admin.Port = adminPortArg
		common.Info("Admin port overridden by command line argument", zap.Int("admin_port", adminPortArg))
	}

	// Reinitialize logger with configured level if different
	level := config.Log.Level
	if level == "" {
		level = "info"
	}
	if debugFlag {
		level = "debug"
	}

	fileOut := common.FileOutput{
		Path:       "ingestion_server.log",
		MaxSize:    config.Log.MaxSize,
		MaxBackups: config.Log.MaxBackups,
		MaxAge:     config.Log.MaxAge,
		Compress:   common.ResolveCompress(config.Log.Compress),
	}
	if config.Log.Path != "" {
		fileOut.Path = config.Log.Path
	}
	if err := common.Init(level, fileOut); err != nil {
		common.Error("Failed to reinitialize logger", err)
	}
	server.SetLogger(common.Logger)

	common.Info("Starting RAGFlow Ingestion Worker")

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

	// Initialize storage factory
	if err := storage.InitStorageFactory(); err != nil {
		common.Fatal("Failed to initialize storage factory", zap.Error(err))
	}

	if err := engine.InitMessageQueueEngine(config.TaskExecutor.MessageQueueType); err != nil {
		common.Fatal(fmt.Sprintf("Failed to initialize message queue engine: %w", err))
	}

	// Initialize server variables (runtime variables from Redis)
	if err := server.InitVariables(redis.Get()); err != nil {
		common.Warn("Failed to initialize server variables from Redis, using defaults", zap.String("error", err.Error()))
	}

	// Initialize tokenizer (rag_analyzer)
	tokenizerCfg := &tokenizer.PoolConfig{
		DictPath: "/usr/share/infinity/resource",
	}
	if err := tokenizer.Init(tokenizerCfg); err != nil {
		common.Fatal("Failed to initialize tokenizer", zap.Error(err))
	}
	defer tokenizer.Close()

	// Initialize global QueryBuilder using tokenizer's DictPath
	if err := nlp.InitQueryBuilderFromTokenizer(tokenizerCfg.DictPath); err != nil {
		common.Fatal("Failed to initialize query builder", zap.Error(err))
	}

	ingestor := ingestion.NewIngestor(name, 2, []string{"pdf", "docx", "txt"})

	go func() {
		err := ingestor.Start()
		if err != nil {
			common.Error("Failed to initialize ingestor", err)
			return
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR2)

	// Print all configuration settings
	server.PrintAll()
	common.Info("\n    ____                      __  _\n" +
		"   /  _/___  ____ ____  _____/ /_(_)___  ____     ________  ______   _____  _____\n" +
		"   / // __ \\/ __ `/ _ \\/ ___/ __/ / __ \\/ __ \\   / ___/ _ \\/ ___/ | / / _ \\/ ___/\n" +
		" _/ // / / / /_/ /  __(__  ) /_/ / /_/ / / / /  (__  )  __/ /   | |/ /  __/ /\n" +
		"/___/_/ /_/\\__, /\\___/____/\\__/_/\\____/_/ /_/  /____/\\___/_/    |___/\\___/_/\n" +
		"          /____/\n")

	// Print RAGFlow version
	common.Info(fmt.Sprintf("RAGFlow ingestion service version: %s", utility.GetRAGFlowVersion()))

	// Get local IP address for heartbeat reporting
	localIP, err := utility.GetLocalIP()
	if err != nil {
		common.Fatal("fail to get local ip address")
	}

	// Initialize and start heartbeat reporter to admin server
	service.AdminServiceClient = service.NewAdminClient(
		common.Logger,
		common.ServerTypeIngestion,
		fmt.Sprintf("ingestor-%s", ingestor.ID()),
		localIP,
		-1,
	)
	if err = service.AdminServiceClient.InitHTTPClient(); err != nil {
		common.Warn("Failed to initialize heartbeat service", zap.Error(err))
	} else {
		// Start heartbeat reporter with 3 seconds interval
		heartbeatReporter := utility.NewScheduledTask("Heartbeat reporter", 3*time.Second, func() {
			if err = service.AdminServiceClient.SendHeartbeat(); err == nil {
				local.SetAdminStatus(0, "")
			} else {
				local.SetAdminStatus(1, err.Error())
			}
		})
		heartbeatReporter.Start()
		defer heartbeatReporter.Stop()
	}

	// Wait for either an OS signal or a shutdown command from the admin
	select {
	case sig := <-quit:
		common.Info("Received signal", zap.String("signal", sig.String()))
		common.Info(fmt.Sprintf("Shutting down RAGFlow ingestor %s ...", name))
	case <-ingestor.ShutdownCh:
		common.Info(fmt.Sprintf("Received shutdown command from admin, stopping ingestor %s ...", name))
	}

	// Create context with timeout for graceful shutdown
	_, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ingestor.Stop()

	common.Info(fmt.Sprintf("Ingestor %s shutdown complete", name))
}

// ---------------------------------------------------------------------------
// API server (default)
// ---------------------------------------------------------------------------

func runAPI(configPath string, debugFlag bool, portFlag int) {
	// Temporarily default to debug while investigating the Go chat/SSE path.
	if err := common.Init("debug", common.FileOutput{Path: "server_main.log"}); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	// Initialize configuration
	if err := server.Init(configPath); err != nil {
		common.Fatal("Failed to initialize config", zap.Error(err))
	}

	config := server.GetConfig()

	// Override port with command line argument if provided
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
		level = "debug"
	}
	if debugFlag {
		level = "debug"
	}

	fileOut := common.FileOutput{
		Path:       "server_main.log",
		MaxSize:    config.Log.MaxSize,
		MaxBackups: config.Log.MaxBackups,
		MaxAge:     config.Log.MaxAge,
		Compress:   common.ResolveCompress(config.Log.Compress),
	}
	if config.Log.Path != "" {
		fileOut.Path = config.Log.Path
	}
	if err := common.Init(level, fileOut); err != nil {
		common.Error("Failed to reinitialize logger", err)
	}
	server.SetLogger(common.Logger)
	if config.Log.Level == "" {
		config.Log.Level = common.GetLevel()
	}

	common.Info("Server mode", zap.String("mode", config.Server.Mode))

	// Print all configuration settings
	server.PrintAll()

	// Set Gin mode
	if config.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

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
	if err := nlp.InitQueryBuilderFromTokenizer(tokenizerCfg.DictPath); err != nil {
		common.Fatal("Failed to initialize query builder", zap.Error(err))
	}

	startAPIServer(config)
}

func startAPIServer(config *server.Config) {
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
	chatChannelService := service.NewChatChannelService()
	langfuseService := service.NewLangfuseService()
	chatSessionService := service.NewChatSessionService()
	openaiChatService := service.NewOpenAIChatService()
	systemService := service.NewSystemService()
	connectorService := service.NewConnectorService()
	searchService := service.NewSearchService()
	searchService.SetTenantService(tenantService)
	fileService := service.NewFileService()
	memoryService := service.NewMemoryService()
	mcpService := service.NewMCPService()
	modelProviderService := service.NewModelProviderService()

	// Initialize doc engine for skill search
	docEngine := engine.Get()
	documentDAO := dao.NewDocumentDAO()
	agenttool.SetRetrievalService(agenttool.NewNLPRetrievalAdapterFromDeps(docEngine, documentDAO))
	common.Info("agent: retrieval service adapter installed")

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
	chatChannelHandler := handler.NewChatChannelHandler(chatChannelService)
	langfuseHandler := handler.NewLangfuseHandler(langfuseService)
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
	//agentOpts := buildAgentRunOptions()
	//agentService := service.NewAgentServiceWithOptions(
	//	agentOpts.checkpointStore,
	//	agentOpts.stateSerializer,
	//	agentOpts.runTracker,
	//)
	//agentHandler := handler.NewAgentHandler(agentService, fileService)
	//
	//botService := service.NewBotService(agentService, llmService)
	//botHandler := handler.NewBotHandler(botService)

	configureTTSSynthesizer(modelProviderService)
	searchBotLLM := &handler.SearchBotRealLLM{Svc: modelProviderService}
	searchBotHandler := handler.NewSearchBotHandler(
		searchService,
		tenantService,
		searchBotLLM,
		chunkService,
	)
	searchBotHandler.SetStreamLLM(searchBotLLM)
	askService := service.NewAskService(chunkService, nil, 0, 0)
	searchBotHandler.SetAskService(askService)
	searchHandler.SetCompletionDependencies(searchBotLLM, askService)
	pluginHandler := handler.NewPluginHandler(service.NewPluginService())
	modelHandler := handler.NewModelHandler(service.NewModelProviderService())
	fileCommitHandler := handler.NewFileCommitHandler(service.NewFileCommitService())

	// Dify retrieval handler
	retrievalService := nlp.NewRetrievalService(docEngine, documentDAO)
	difyRetrievalHandler := handler.NewDifyRetrievalHandler(
		knowledgebaseService,
		modelProviderService,
		metadataService,
		retrievalService,
		documentDAO,
		docEngine,
	)
	// Per-tenant canvas-runtime override selector
	var adminRuntimeSelector *runtime.Selector
	if rdb := redis.Get().GetClient(); rdb != nil {
		adminRuntimeSelector = runtime.NewSelector(rdb, common.Logger)
	}
	adminRuntimeHandler := handler.NewAdminRuntimeHandler(adminRuntimeSelector)

	// Initialize router
	r := router.NewRouter(authHandler, userHandler, tenantHandler, documentHandler, datasetsHandler, systemHandler, knowledgebaseHandler, chunkHandler, llmHandler, chatHandler, chatChannelHandler, langfuseHandler, chatSessionHandler, connectorHandler, searchHandler, fileHandler, memoryHandler, mcpHandler, skillSearchHandler, providerHandler, nil, searchBotHandler, difyRetrievalHandler, pluginHandler, modelHandler, fileCommitHandler, adminRuntimeHandler, openaiChatHandler, nil)

	// Create Gin engine
	ginEngine := gin.New()

	// Middleware
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
		// Start heartbeat reporter with 3 seconds interval
		heartbeatReporter := utility.NewScheduledTask("Heartbeat reporter", 3*time.Second, func() {
			if err = service.AdminServiceClient.SendHeartbeat(); err == nil {
				local.SetAdminStatus(0, "")
			} else {
				local.SetAdminStatus(1, err.Error())
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

	common.Info("Server exited")
}

// ---------------------------------------------------------------------------
// Agent run options helpers (used by API server)
// ---------------------------------------------------------------------------

// agentRunOptions bundles the three optional injection slots the
// agent service accepts via NewAgentServiceWithOptions.
type agentRunOptions struct {
	checkpointStore canvas.CheckPointStore
	stateSerializer canvas.StateSerializer
	runTracker      *canvas.RunTracker
}

// buildAgentRunOptions installs the Redis-backed run infrastructure
// when Redis is available.
func buildAgentRunOptions() agentRunOptions {
	var out agentRunOptions
	if !redis.IsEnabled() || redis.Get() == nil {
		common.Info("agent: redis client not initialised; agent run infra in in-memory mode (no checkpoints, no run tracker)")
		return out
	}
	cp := canvas.NewRedisCheckPointStore(24 * time.Hour)
	out.checkpointStore = cp
	rt := canvas.NewRunTracker(24 * time.Hour)
	out.runTracker = rt
	common.Info("agent: redis-backed run infra installed (24h TTL on checkpoint store + run tracker; eino default serializer)")
	return out
}

// configureTTSSynthesizer installs the audio.ModelProviderFunc
// that dispatches Synthesize requests through the project's
// ModelProviderService.
func configureTTSSynthesizer(modelProviderService *service.ModelProviderService) {
	if modelProviderService == nil {
		common.Info("agent: model provider service not initialised; TTS in no-op echo mode")
		audio.SetModelProviderSynthesizer(nil)
		return
	}
	audio.SetModelProviderSynthesizer(audio.NewTTSDispatchFunc(modelProviderService))
	common.Info("agent: TTS model-provider dispatch installed (audio.Synthesize → ModelProviderService.AudioSpeech)")
}
