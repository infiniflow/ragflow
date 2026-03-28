package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"ragflow/internal/common"
	"ragflow/internal/server"
	"ragflow/internal/server/local"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/cache"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/handler"
	"ragflow/internal/logger"
	"ragflow/internal/router"
	"ragflow/internal/service"
	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"
)

func printHelp() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "RAGFlow Server - Open-source RAG engine based on deep document understanding\n\n")
	fmt.Fprintf(os.Stderr, "Options:\n")
	fmt.Fprintf(os.Stderr, "  -p, --port int\tServer port (overrides config file)\n")
	fmt.Fprintf(os.Stderr, "  -h, --help   \tShow this help message and exit\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  %s           # Start server with config file port\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s -p 8080   # Start server on port 8080\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s --port 8080 # Start server on port 8080\n", os.Args[0])
}

func main() {
	// Parse command line flags
	var portFlag int
	flag.IntVar(&portFlag, "port", 0, "Server port (overrides config file)")
	flag.IntVar(&portFlag, "p", 0, "Server port (shorthand, overrides config file)")

	// Custom help message
	flag.Usage = printHelp

	flag.Parse()

	// Initialize logger with default level
	// logger.Init("info"); // set debug log level
	if err := logger.Init("info"); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	// Initialize configuration
	if err := server.Init(""); err != nil {
		logger.Fatal("Failed to initialize config", zap.Error(err))
	}

	// Override port with command line argument if provided
	if portFlag > 0 {
		config := server.GetConfig()
		config.Server.Port = portFlag
		logger.Info("Port overridden by command line argument", zap.Int("port", portFlag))
	}

	// Load model providers configuration
	if err := server.LoadModelProviders(""); err != nil {
		logger.Fatal("Failed to load model providers", zap.Error(err))
	}
	logger.Info("Model providers loaded", zap.Int("count", len(server.GetModelProviders())))

	config := server.GetConfig()
	if config.Server.Port == 0 {
		logger.Fatal("Server port is not configured. Please specify via --port flag or config file.")
	}

	// Reinitialize logger with configured level if different
	if config.Log.Level != "" && config.Log.Level != "info" {
		if err := logger.Init(config.Log.Level); err != nil {
			logger.Error("Failed to reinitialize logger with configured level", err)
		}
	}
	server.SetLogger(logger.Logger)

	logger.Info("Server mode", zap.String("mode", config.Server.Mode))

	// Print all configuration settings
	server.PrintAll()

	// Initialize database
	if err := dao.InitDB(); err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}

	// Initialize LLM factory data models from configuration file
	if err := dao.InitLLMFactory(); err != nil {
		logger.Error("Failed to initialize LLM factory", err)
	} else {
		logger.Info("LLM factory initialized successfully")
	}

	// Initialize doc engine
	if err := engine.Init(&config.DocEngine); err != nil {
		logger.Fatal("Failed to initialize doc engine", zap.Error(err))
	}
	defer engine.Close()

	// Initialize Redis cache
	if err := cache.Init(&config.Redis); err != nil {
		logger.Fatal("Failed to initialize Redis", zap.Error(err))
	}
	defer cache.Close()

	if err := storage.InitStorageFactory(); err != nil {
		logger.Fatal("Failed to initialize storage factory", zap.Error(err))
	}

	// Initialize server variables (runtime variables that can change during operation)
	// This must be done after Cache is initialized
	if err := server.InitVariables(cache.Get()); err != nil {
		logger.Warn("Failed to initialize server variables from Redis, using defaults", zap.String("error", err.Error()))
	}

	// Initialize admin status (default: unavailable=1)
	local.InitAdminStatus(1, "admin server not connected")

	// Initialize tokenizer (rag_analyzer)
	tokenizerCfg := &tokenizer.PoolConfig{
		DictPath: "/usr/share/infinity/resource",
	}
	if err := tokenizer.Init(tokenizerCfg); err != nil {
		logger.Fatal("Failed to initialize tokenizer", zap.Error(err))
	}
	defer tokenizer.Close()

	// Initialize global QueryBuilder using tokenizer's DictPath
	// This ensures the Synonym uses the same wordnet directory as tokenizer
	if err := nlp.InitQueryBuilderFromTokenizer(tokenizerCfg.DictPath); err != nil {
		logger.Fatal("Failed to initialize query builder", zap.Error(err))
	}

	startServer(config)

	logger.Info("Server exited")
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
	datasetsService := service.NewDatasetsService()
	kbService := service.NewKnowledgebaseService()
	chunkService := service.NewChunkService()
	llmService := service.NewLLMService()
	tenantService := service.NewTenantService()
	chatService := service.NewChatService()
	chatSessionService := service.NewChatSessionService()
	systemService := service.NewSystemService()
	connectorService := service.NewConnectorService()
	searchService := service.NewSearchService()
	fileService := service.NewFileService()
	memoryService := service.NewMemoryService()

	// Initialize handler layer
	authHandler := handler.NewAuthHandler()
	userHandler := handler.NewUserHandler(userService)
	tenantHandler := handler.NewTenantHandler(tenantService, userService)
	documentHandler := handler.NewDocumentHandler(documentService)
	datasetsHandler := handler.NewDatasetsHandler(datasetsService)
	systemHandler := handler.NewSystemHandler(systemService)
	kbHandler := handler.NewKnowledgebaseHandler(kbService, userService, documentService)
	chunkHandler := handler.NewChunkHandler(chunkService, userService)
	llmHandler := handler.NewLLMHandler(llmService, userService)
	chatHandler := handler.NewChatHandler(chatService, userService)
	chatSessionHandler := handler.NewChatSessionHandler(chatSessionService, userService)
	connectorHandler := handler.NewConnectorHandler(connectorService, userService)
	searchHandler := handler.NewSearchHandler(searchService, userService)
	fileHandler := handler.NewFileHandler(fileService, userService)
	memoryHandler := handler.NewMemoryHandler(memoryService)

	// Initialize router
	r := router.NewRouter(authHandler, userHandler, tenantHandler, documentHandler, datasetsHandler, systemHandler, kbHandler, chunkHandler, llmHandler, chatHandler, chatSessionHandler, connectorHandler, searchHandler, fileHandler, memoryHandler)

	// Create Gin engine
	ginEngine := gin.New()

	// Middleware
	if config.Server.Mode == "debug" {
		ginEngine.Use(gin.Logger())
	}
	ginEngine.Use(gin.Recovery())

	// Setup routes
	r.Setup(ginEngine)

	// Create HTTP server
	addr := fmt.Sprintf(":%d", config.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: ginEngine,
	}

	// Start server in a goroutine
	go func() {
		logger.Info(
			"\n        ____   ___    ______ ______ __\n" +
				"       / __ \\ /   |  / ____// ____// /____  _      __\n" +
				"      / /_/ // /| | / / __ / /_   / // __ \\| | /| / /\n" +
				"     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /\n" +
				"    /_/ |_|/_/  |_|\\____//_/    /_/ \\____/ |__/|__/\n",
		)
		logger.Info(fmt.Sprintf("RAGFlow Go Version: %s", utility.GetRAGFlowVersion()))
		logger.Info(fmt.Sprintf("Server starting on port: %d", config.Server.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Get local IP address for heartbeat reporting
	localIP := utility.GetLocalIP()
	if localIP == "" {
		localIP = "127.0.0.1"
	}

	// Initialize and start heartbeat reporter to admin server
	heartbeatService := service.NewHeartbeatSender(
		logger.Logger,
		common.ServerTypeAPI,
		fmt.Sprintf("ragflow-server-%d", config.Server.Port),
		localIP,
		config.Server.Port,
	)
	if err := heartbeatService.InitHTTPClient(); err != nil {
		logger.Warn("Failed to initialize heartbeat service", zap.Error(err))
	} else {
		// Start heartbeat reporter with 30 seconds interval
		heartbeatReporter := utility.NewScheduledTask("Heartbeat reporter", 3*time.Second, func() {
			if err = heartbeatService.SendHeartbeat(); err == nil {
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

	logger.Info(fmt.Sprintf("Receives %s signal to shutdown server", strings.ToUpper(sig.String())))
	logger.Info("Shutting down server...")

	// Create context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}
}
