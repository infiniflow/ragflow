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
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"ragflow/internal/cache"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/admin"
	"ragflow/internal/dao"
	"ragflow/internal/handler"
	"ragflow/internal/logger"
	"ragflow/internal/server"
	"ragflow/internal/service"
	"ragflow/internal/utility"
)

// AdminServer admin server
type AdminServer struct {
	router      *admin.Router
	handler     *admin.Handler
	service     *admin.Service
	userHandler *handler.UserHandler
	engine      *gin.Engine
	port        string
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	flag.Parse()

	// Initialize logger
	if err := logger.Init("debug"); err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	// Initialize configuration
	if err := server.Init(configPath); err != nil {
		logger.Error("Failed to initialize configuration", err)
		os.Exit(1)
	}

	cfg := server.GetConfig()

	// Reinitialize logger with configured level if different
	if cfg.Log.Level != "" && cfg.Log.Level != "info" {
		if err := logger.Init(cfg.Log.Level); err != nil {
			logger.Error("Failed to reinitialize logger with configured level", err)
		}
	}

	// Set logger for server package
	server.SetLogger(logger.Logger)

	logger.Info("Server mode", zap.String("mode", cfg.Server.Mode))

	// Print all configuration settings
	server.PrintAll()

	// Set Gin mode
	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// Initialize database
	if err := dao.InitDB(); err != nil {
		logger.Error("Failed to initialize database", err)
		os.Exit(1)
	}

	// Initialize Redis cache
	if err := cache.Init(&cfg.Redis); err != nil {
		logger.Fatal("Failed to initialize Redis", zap.Error(err))
	}
	defer cache.Close()

	// Initialize server variables (runtime variables that can change during operation)
	// This must be done after Cache is initialized
	if err := server.InitVariables(cache.Get()); err != nil {
		logger.Warn("Failed to initialize server variables from Redis, using defaults", zap.String("error", err.Error()))
	}

	adminService := admin.NewService()
	userService := service.NewUserService()
	adminHandler := admin.NewHandler(adminService, userService)

	// Initialize router
	r := admin.NewRouter(adminHandler)

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
	addr := fmt.Sprintf(":9381")
	srv := &http.Server{
		Addr:    addr,
		Handler: ginEngine,
	}

	// Print RAGFlow version
	logger.Info("RAGFlow version", zap.String("version", utility.GetRAGFlowVersion()))

	logger.Info("" +
		"\n        ____  ___   ______________                 ___       __          _     \n" +
		"       / __ \\/   | / ____/ ____/ /___ _      __   /   | ____/ /___ ___  (_)___ \n" +
		"      / /_/ / /| |/ / __/ /_  / / __ \\ | /| / /  / /| |/ __  / __ `__ \\/ / __ \\ \n" +
		"     / _, _/ ___ / /_/ / __/ / / /_/ / |/ |/ /  / ___ / /_/ / / / / / / / / / / /\n" +
		"    /_/ |_/_/  |_\\____/_/   /_/\\____/|__/|__/  /_/  |_\\__,_/_/ /_/ /_/_/_/ /_/ \n")

	// Start server in a goroutine
	go func() {
		logger.Info(fmt.Sprintf("Server starting on port: %d", cfg.Server.Port))
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
