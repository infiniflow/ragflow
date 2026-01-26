package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"

	"ragflow/internal/config"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/handler"
	"ragflow/internal/router"
	"ragflow/internal/service"
)

func main() {
	// Initialize configuration
	if err := config.Init(""); err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	cfg := config.Get()
	log.Printf("Server mode: %s", cfg.Server.Mode)

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
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize doc engine
	if err := engine.Init(&cfg.DocEngine); err != nil {
		log.Fatalf("Failed to initialize doc engine: %v", err)
	}
	defer engine.Close()

	// Initialize service layer
	userService := service.NewUserService()
	documentService := service.NewDocumentService()
	kbService := service.NewKnowledgebaseService()
	chunkService := service.NewChunkService()
	llmService := service.NewLLMService()

	// Initialize handler layer
	userHandler := handler.NewUserHandler(userService)
	documentHandler := handler.NewDocumentHandler(documentService)
	kbHandler := handler.NewKnowledgebaseHandler(kbService, userService)
	chunkHandler := handler.NewChunkHandler(chunkService, userService)
	llmHandler := handler.NewLLMHandler(llmService, userService)

	// Initialize router
	r := router.NewRouter(userHandler, documentHandler, kbHandler, chunkHandler, llmHandler)

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
	log.Printf("Server starting on %s", addr)
	if err := engine.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
