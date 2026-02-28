package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/config"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/handler"
	"ragflow/internal/logger"
	"ragflow/internal/router"
	"ragflow/internal/service"
	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"
)

func main() {
	// Initialize logger with default level
	// logger.Init("info"); // set debug log level
	if err := logger.Init("info"); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	// Initialize configuration
	if err := config.Init(""); err != nil {
		logger.Fatal("Failed to initialize config", zap.Error(err))
	}

	// Load model providers configuration
	if err := config.LoadModelProviders(""); err != nil {
		logger.Fatal("Failed to load model providers", zap.Error(err))
	}
	logger.Info("Model providers loaded", zap.Int("count", len(config.GetModelProviders())))

	cfg := config.Get()

	// Reinitialize logger with configured level if different
	if cfg.Log.Level != "" && cfg.Log.Level != "info" {
		if err := logger.Init(cfg.Log.Level); err != nil {
			logger.Error("Failed to reinitialize logger with configured level", err)
		}
	}
	config.SetLogger(logger.Logger)

	logger.Info("Server mode", zap.String("mode", cfg.Server.Mode))

	// Print all configuration settings
	config.PrintAll()

	// Set Gin mode
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// Initialize database
	if err := dao.InitDB(); err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}

	// Initialize doc engine
	if err := engine.Init(&cfg.DocEngine); err != nil {
		logger.Fatal("Failed to initialize doc engine", zap.Error(err))
	}
	defer engine.Close()

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

	// Initialize service layer
	userService := service.NewUserService()
	documentService := service.NewDocumentService()
	kbService := service.NewKnowledgebaseService()
	chunkService := service.NewChunkService()
	llmService := service.NewLLMService()
	tenantService := service.NewTenantService()
	chatService := service.NewChatService()
	chatSessionService := service.NewChatSessionService()

	// Initialize handler layer
	userHandler := handler.NewUserHandler(userService)
	tenantHandler := handler.NewTenantHandler(tenantService, userService)
	documentHandler := handler.NewDocumentHandler(documentService)
	kbHandler := handler.NewKnowledgebaseHandler(kbService, userService)
	chunkHandler := handler.NewChunkHandler(chunkService, userService)
	llmHandler := handler.NewLLMHandler(llmService, userService)
	chatHandler := handler.NewChatHandler(chatService, userService)
	chatSessionHandler := handler.NewChatSessionHandler(chatSessionService, userService)

	// Initialize router
	r := router.NewRouter(userHandler, tenantHandler, documentHandler, kbHandler, chunkHandler, llmHandler, chatHandler, chatSessionHandler)

	// Create Gin engine
	ginEngine := gin.New()

	// Middleware
	if cfg.Server.Mode == "debug" {
		ginEngine.Use(gin.Logger())
	}
	ginEngine.Use(gin.Recovery())

	// Setup routes
	r.Setup(ginEngine)

	// Create HTTP server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: ginEngine,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("Server starting", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR2)
	sig := <-quit

	logger.Info("Received signal", zap.String("signal", sig.String()))
	logger.Info("Shutting down server...")

	// Create context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown server
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}
