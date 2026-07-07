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
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"ragflow/internal/admin"
	"ragflow/internal/agent/audio"
	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/handler"
	"ragflow/internal/ingestion"
	"ragflow/internal/mcp"
	"ragflow/internal/router"
	"ragflow/internal/server/local"
	"ragflow/internal/service"
	"ragflow/internal/service/chunk"
	"ragflow/internal/service/nlp"
	"ragflow/internal/storage"
	"ragflow/internal/syncer"
	"ragflow/internal/tokenizer"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	_ "ragflow/internal/agent/component"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/redis"
	_ "ragflow/internal/ingestion/wire" // single owner for ingestion-component registration (File / Parser / Tokenizer / Extractor + 4 Chunker variants)
	"ragflow/internal/server"
	"ragflow/internal/utility"
)

type serverArgs struct {
	mode          *string // admin | api | ingestor | syncer
	helpFlag      bool
	versionFlag   bool
	debugLog      bool
	migrateDB     bool
	configPath    *string // Used by admin, api; user defined config path
	initSuperUser bool    // Used by admin;
	port          *int    // Used by admin, api
	adminHost     *string // Used by api, ingestor, syncer for heartbeat
	adminPort     *int    // Used by api, ingestor, syncer for heartbeat, "ip:port"
	name          *string // server name
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
		case "--migrate":
			args.migrateDB = true
		case "--ingestor":
			serverMode = "ingestor"
			args.mode = &serverMode
		case "--api":
			serverMode = "api"
			args.mode = &serverMode
		case "--syncer":
			serverMode = "syncer"
			args.mode = &serverMode
		case "-h", "--help":
			args.helpFlag = true
		case "-v", "--version":
			args.versionFlag = true
		case "--debug":
			args.debugLog = true
		case "-f", "--config":
			if i+1 >= len(os.Args) {
				return nil, fmt.Errorf("%s requires a value", arg)
			}
			i++
			configPath = os.Args[i]
			args.configPath = &configPath
		case "--init-superuser":
			args.initSuperUser = true
		case "-p", "--port":
			if i+1 >= len(os.Args) {
				return nil, errors.New("--port requires a value")
			}
			i++
			port, convErr := strconv.Atoi(os.Args[i])
			if convErr != nil {
				return nil, fmt.Errorf("invalid port: %w", convErr)
			}
			args.port = &port
			if port <= 0 || port > 65535 {
				return nil, fmt.Errorf("invalid port: %d", port)
			}
		case "--admin-host":
			if i+1 >= len(os.Args) {
				return nil, errors.New("--admin-host requires a value")
			}
			i++
			parts := strings.SplitN(os.Args[i], ":", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return nil, errors.New("--admin-host must be in the form 'ip:port'")
			}
			ip, portStr := parts[0], parts[1]
			port, convErr := strconv.Atoi(portStr)
			if convErr != nil {
				return nil, fmt.Errorf("failed to parse admin port: %w", convErr)
			}
			args.adminHost = &ip
			args.adminPort = &port
		case "--name":
			if i+1 >= len(os.Args) {
				return nil, errors.New("--name requires a value")
			}
			i++
			args.name = &os.Args[i]
		default:
			return nil, fmt.Errorf("unknown parameter: %s", arg)
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
		fmt.Fprintf(os.Stderr, "  --port int     	\tServer port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\tPath to configuration file\n")
		fmt.Fprintf(os.Stderr, "  -v, --version 	 \tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug       	 \tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  -h, --help       	  \tShow this help message and exit\n")
	case *args.mode == "admin":
		fmt.Fprintf(os.Stderr, "Usage: %s --admin [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Admin Server\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\t\tPath to configuration file\n")
		fmt.Fprintf(os.Stderr, "  --port int    \t\t\tServer port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  --init-superuser\t\t\tInitialize superuser account\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \t\t\tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug        \t\t\tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \t\t\tShow this help message and exit\n")
	case *args.mode == "ingestor":
		fmt.Fprintf(os.Stderr, "Usage: %s --ingestor [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Ingestion Worker - Document ingestion processing\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\tPath to config file\n")
		fmt.Fprintf(os.Stderr, "  --name string\t\t\tIngestion server name (default: \"default_ingestion\")\n")
		fmt.Fprintf(os.Stderr, "  --admin-host string\tAdmin server host:port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \t\tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug        \t\tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \t\tShow this help message and exit\n")
	case *args.mode == "syncer":
		fmt.Fprintf(os.Stderr, "Usage: %s --syncer [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Sync Service - Sync files from source to RAGFlow\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\tPath to config file\n")
		fmt.Fprintf(os.Stderr, "  --name string\t\t\tSync service server name (default: \"default_syncer\")\n")
		fmt.Fprintf(os.Stderr, "  --admin-host string\tAdmin server host:port (overrides config file)\n")
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

	if arguments.helpFlag || arguments.mode == nil {
		printHelp(arguments)
		return
	}

	if arguments.versionFlag {
		fmt.Printf("RAGFlow version: %s\n", utility.GetRAGFlowVersion())
		return
	}

	// Initialize local variables (runtime variables from Redis)
	err = server.InitLocalVariables()
	if err != nil {

		fmt.Printf("Failed to start %s server: %v\n", *arguments.mode, err)
		os.Exit(1)
	}

	// Temporary logger initialization
	var logFile string
	var serverName string
	if arguments.name != nil {
		serverName = *arguments.name
	} else {
		serverName = fmt.Sprintf("%s_server", *arguments.mode)
	}
	logFile = fmt.Sprintf("%s.log", serverName)

	logLevel := "info"
	if arguments.debugLog {
		logLevel = "debug"
	}

	if err = common.Init(logLevel, common.FileOutput{Path: logFile}); err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	// Initialize configuration
	var configPath string
	if arguments.configPath != nil {
		configPath = *arguments.configPath
	}

	if err = server.Init(configPath); err != nil {
		common.Error("Failed to initialize configuration", err)
		os.Exit(1)
	}

	config := server.GetConfig()

	// override default port if provided
	switch *arguments.mode {
	case "api":
		port := config.Server.Port
		if arguments.port != nil {
			port = *arguments.port
			config.Server.Port = port
		}
		if arguments.name == nil {
			serverName = fmt.Sprintf("api_server_%d", port)
		}
	case "admin":
		port := config.Admin.Port
		if arguments.port != nil {
			port = *arguments.port
			config.Admin.Port = port
		}
		if arguments.name == nil {
			serverName = fmt.Sprintf("admin_server_%d", port)
		}
	case "ingestor":
		if serverName == "" {
			uuid := utility.GenerateUUID()
			serverName = fmt.Sprintf("ingestor_server_%s", uuid)
		}
	case "syncer":
		if serverName == "" {
			uuid := utility.GenerateUUID()
			serverName = fmt.Sprintf("syncer_server_%s", uuid)
		}
	default:
		common.Error("invalid server mode", errors.New(*arguments.mode))
		os.Exit(1)
	}

	// set server name and log file path
	server.SetServerName(serverName)
	logFile = fmt.Sprintf("%s.log", serverName)

	// Reinitialize logger with configured level if different
	logLevel = config.Log.Level
	if logLevel == "" {
		logLevel = "info"
	}

	if arguments.debugLog {
		logLevel = "debug"
	}

	config.Log.Level = logLevel

	fileOut := common.FileOutput{
		Path:       logFile,
		MaxSize:    config.Log.MaxSize,
		MaxBackups: config.Log.MaxBackups,
		MaxAge:     config.Log.MaxAge,
		Compress:   common.ResolveCompress(config.Log.Compress),
	}
	if config.Log.Path != "" {
		fileOut.Path = config.Log.Path
	}
	if err = common.Init(logLevel, fileOut); err != nil {
		common.Error("Failed to reinitialize logger with configured level", err)
	}

	server.SetLogger(common.Logger)

	// Print all configuration settings
	common.Info(fmt.Sprintf("Starting %s server: %s, mode: %s", *arguments.mode, serverName, config.Server.Mode))
	server.PrintAll()

	// Initialize database
	if err = dao.InitDB(arguments.migrateDB); err != nil {
		common.Fatal("Failed to initialize database", zap.Error(err))
	}

	// Initialize doc engine
	if err = engine.Init(&config.DocEngine); err != nil {
		common.Fatal("Failed to initialize doc engine", zap.Error(err))
	}
	defer engine.Close()

	// Initialize Redis cache
	if err = redis.Init(&config.Redis); err != nil {
		common.Fatal("Failed to initialize Redis", zap.Error(err))
	}
	defer redis.Close()

	if err = storage.InitStorageFactory(); err != nil {
		common.Fatal("Failed to initialize storage factory", zap.Error(err))
	}

	if err = engine.InitMessageQueueEngine(config.TaskExecutor.MessageQueueType); err != nil {
		common.Fatal("Failed to initialize message queue engine", zap.Error(err))
	}

	// Initialize server variables (runtime variables that can change during operation)
	// This must be done after Cache is initialized
	if err = server.InitVariables(redis.Get()); err != nil {
		common.Warn("Failed to initialize server variables from Redis, using defaults", zap.String("error", err.Error()))
	}

	if arguments.name == nil {
		arguments.name = &serverName
	}

	switch *arguments.mode {
	case "api":
		if err = runAPI(arguments); err != nil {
			fmt.Printf("Failed to start API server: %v\n", err)
			os.Exit(1)
		}
	case "admin":
		if err = runAdmin(arguments); err != nil {
			fmt.Printf("Failed to start admin server: %v\n", err)
			os.Exit(1)
		}
	case "ingestor":
		if err = runIngestor(arguments); err != nil {
			fmt.Printf("Failed to start ingestion worker: %v\n", err)
			os.Exit(1)
		}
	case "syncer":
		if err = runSyncer(arguments); err != nil {
			fmt.Printf("Failed to start syncer: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Invalid server mode: %s\n", *arguments.mode)
		os.Exit(1)
	}
}

func runAdmin(args *serverArgs) error {
	adminService := admin.NewService()
	adminHandler := admin.NewHandler(adminService)

	if args.initSuperUser {
		// Initialize default admin user
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
	config := server.GetConfig()
	addr := fmt.Sprintf(":%d", config.Admin.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: ginEngine,
	}

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
		common.Info(fmt.Sprintf("Starting RAGFlow admin HTTP server on port: %d", config.Admin.Port))
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
	return nil
}

func runIngestor(args *serverArgs) error {

	ingestor := ingestion.NewIngestor(*args.name, 2, []string{"pdf", "docx", "txt"})

	go func() {
		err := ingestor.Start()
		if err != nil {
			common.Error("Failed to initialize ingestor", err)
			return
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR2)

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

	// Wait for either an OS signal or a shutdown command from the admin
	select {
	case sig := <-quit:
		common.Info("Received signal", zap.String("signal", sig.String()))
		common.Info(fmt.Sprintf("Shutting down RAGFlow ingestor %s ...", *args.name))
	case <-ingestor.ShutdownCh:
		common.Info(fmt.Sprintf("Received shutdown command from admin, stopping ingestor %s ...", *args.name))
	}

	// Create context with timeout for graceful shutdown
	_, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ingestor.Stop()

	common.Info(fmt.Sprintf("Ingestor %s shutdown complete", *args.name))

	return nil
}

func runSyncer(args *serverArgs) error {
	config := server.GetConfig()
	fileSyncer := syncer.NewSyncer(config.FileSyncer.MaxConcurrentSyncs, time.Duration(config.FileSyncer.SyncInterval)*time.Second)

	go func() {
		err := fileSyncer.Start()
		if err != nil {
			common.Error("Failed to initialize file syncer", err)
			return
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR2)

	common.Info("\n     _______ __        _____\n" +
		"    / ____(_) /__     / ___/__  ______  ________  _____\n" +
		"   / /_  / / / _ \\    \\__ \\/ / / / __ \\/ ___/ _ \\/ ___/\n" +
		"  / __/ / / /  __/   ___/ / /_/ / / / / /__/  __/ /\n" +
		" /_/   /_/_/\\___/   /____/\\__, /_/ /_/\\___/\\___/_/\n" +
		"                           /____/    \n")

	// Print RAGFlow version
	common.Info(fmt.Sprintf("RAGFlow file syncer service version: %s", utility.GetRAGFlowVersion()))

	// Get local IP address for heartbeat reporting
	localIP, err := utility.GetLocalIP()
	if err != nil {
		common.Fatal("fail to get local ip address")
	}

	// Initialize and start heartbeat reporter to admin server
	service.AdminServiceClient = service.NewAdminClient(
		common.Logger,
		common.ServerTypeFileSyncer,
		fmt.Sprintf("syncer-%s", fileSyncer.ID()),
		localIP,
		-1,
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

	// Wait for either an OS signal or a shutdown command from the admin
	select {
	case sig := <-quit:
		common.Info("Received signal", zap.String("signal", sig.String()))
		common.Info(fmt.Sprintf("Shutting down RAGFlow file syncer %s ...", *args.name))
	case <-fileSyncer.ShutdownCh:
		common.Info(fmt.Sprintf("Received shutdown command from admin, stopping file syncer %s ...", *args.name))
	}

	// Create context with timeout for graceful shutdown
	_, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fileSyncer.Stop()

	common.Info(fmt.Sprintf("File syncer %s shutdown complete", *args.name))

	return nil
}

func runAPI(args *serverArgs) error {
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

	config := server.GetConfig()
	startServer(config)

	common.Info("Server exited")

	return nil
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
	tenantHandler := handler.NewTenantHandler(tenantService, userService, datasetsService)
	documentHandler := handler.NewDocumentHandler(documentService, datasetsService)
	datasetsHandler := handler.NewDatasetsHandler(datasetsService, metadataService)
	systemHandler := handler.NewSystemHandler(systemService)
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

	// MCP server endpoint — exposes RAGFlow capabilities as MCP tools
	// (ragflow_retrieval, ragflow_list_datasets, ragflow_list_chats) to
	// external AI clients via JSON-RPC over HTTP.
	mcpServerHandler := handler.NewMCPServerHandler(
		func(userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error) {
			return handler.MCPListDatasets(datasetsService, userID, page, pageSize, orderby, desc)
		},
		func(userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error) {
			return handler.MCPListChats(chatService, userID, page, pageSize, orderby, desc)
		},
		func(userID string, req mcp.RetrievalRequest) (string, error) {
			return handler.MCPRetrieval(datasetsService, userID, req)
		},
	)
	skillSearchHandler := handler.NewSkillSearchHandler(docEngine)
	providerHandler := handler.NewProviderHandler(userService, modelProviderService)
	// Install the agent service's Redis-backed run infrastructure
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
	agentHandler := handler.NewAgentHandler(agentService, fileService)

	// Public chatbot/agentbot endpoints (api/v1/chatbots/...,
	// api/v1/agentbots/...) and the agent attachment download.
	// BotService delegates the agentbot completion to agentService so
	// both paths share the same canvas runner. Reuse the llmService
	// already constructed above (line 222) — do NOT redeclare with
	// `:=` since the variable is in scope.
	botService := service.NewBotService(agentService, llmService)
	botHandler := handler.NewBotHandler(botService)

	// Wire the TTS synthesizer to the per-tenant model-provider
	// dispatch. SynthesizeRequest is routed through
	// ModelProviderService.AudioSpeech, which fans out to the
	// tenant's configured TTS model driver. When the model
	// provider is unconfigured, the synthesizer falls back to a
	// no-op echo (the audio package contract), so this is always
	// safe to call.
	configureTTSSynthesizer(modelProviderService)
	searchBotHandler := handler.NewSearchBotHandler(
		searchService,
		tenantService,
		modelProviderService,
		chunkService,
	)
	searchBotHandler.SetStreamLLM(modelProviderService)
	askService := service.NewAskService(chunkService, nil, 0, 0)
	searchBotHandler.SetAskService(askService)
	chatHandler.SetMindMapDependencies(searchService, tenantService, modelProviderService, chunkService)
	searchHandler.SetCompletionDependencies(modelProviderService, askService)
	pluginHandler := handler.NewPluginHandler(service.NewPluginService())
	modelHandler := handler.NewModelHandler(service.NewModelProviderService())
	fileCommitHandler := handler.NewFileCommitHandler(service.NewFileCommitService())

	// Dify retrieval handler
	docDAO := documentDAO
	retrievalService := nlp.NewRetrievalService(docEngine, docDAO)
	difyRetrievalHandler := handler.NewDifyRetrievalHandler(
		datasetsService,
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
	if redisClient := redis.Get(); redisClient != nil {
		if rdb := redisClient.GetClient(); rdb != nil {
			adminRuntimeSelector = runtime.NewSelector(rdb, common.Logger)
		}
	}
	adminRuntimeHandler := handler.NewAdminRuntimeHandler(adminRuntimeSelector)
	componentsHandler := handler.NewComponentsHandler(service.NewComponentsService())

	// Initialize router
	r := router.NewRouter(authHandler,
		userHandler,
		tenantHandler,
		documentHandler,
		datasetsHandler,
		systemHandler,
		chunkHandler,
		llmHandler,
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
		adminRuntimeHandler,
		openaiChatHandler,
		botHandler,
		componentsHandler)

	// Create Gin enginegit diff

	ginEngine := gin.New()

	// Middleware
	// Note: common.GinLogger() is registered inside router.Setup so the
	// HTTP request log captures every endpoint the router owns (including
	// those registered by Setup itself). Registering it here would run
	// it twice for those endpoints and double every access-log line.
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
// initialized at the top of main; the TTL is a conservative 24h for
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
