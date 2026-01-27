package main

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/config"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/handler"
	"ragflow/internal/logger"
	"ragflow/internal/router"
	"ragflow/internal/service"
)

func main() {
	// Initialize logger with default level
	if err := logger.Init("debug"); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	// Initialize configuration
	if err := config.Init(""); err != nil {
		logger.Fatal("Failed to initialize config", zap.Error(err))
	}

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

	// Initialize service layer
	userService := service.NewUserService()
	documentService := service.NewDocumentService()
	kbService := service.NewKnowledgebaseService()
	chunkService := service.NewChunkService()
	llmService := service.NewLLMService()
	tenantService := service.NewTenantService()

	// Initialize handler layer
	userHandler := handler.NewUserHandler(userService)
	tenantHandler := handler.NewTenantHandler(tenantService, userService)
	documentHandler := handler.NewDocumentHandler(documentService)
	kbHandler := handler.NewKnowledgebaseHandler(kbService, userService)
	chunkHandler := handler.NewChunkHandler(chunkService, userService)
	llmHandler := handler.NewLLMHandler(llmService, userService)

	// Initialize router
	r := router.NewRouter(userHandler, tenantHandler, documentHandler, kbHandler, chunkHandler, llmHandler)

	// Create Gin engine
	engine := gin.New()

	// Middleware
	if cfg.Server.Mode == "debug" {
		engine.Use(gin.Logger())
	}
	engine.Use(gin.Recovery())

	// Setup routes
	r.Setup(engine)

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("Server starting", zap.String("addr", addr))
	if err := engine.Run(addr); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}
