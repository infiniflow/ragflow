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
	"ragflow/internal/engine"
	"ragflow/internal/engine/redis"
	"ragflow/internal/utility"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/admin"
	"ragflow/internal/dao"
	"ragflow/internal/server"
)

func printHelp() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "RAGFlow Admin Server\n\n")
	fmt.Fprintf(os.Stderr, "Options:\n")
	fmt.Fprintf(os.Stderr, "  --config string\tPath to configuration file\n")
	fmt.Fprintf(os.Stderr, "  -v, --version  \tPrint version information and exit\n")
	fmt.Fprintf(os.Stderr, "  --debug        \tEnable debug-level logging\n")
	fmt.Fprintf(os.Stderr, "  --init-superuser\tInitialize superuser account\n")
	fmt.Fprintf(os.Stderr, "  -h, --help     \tShow this help message and exit\n")
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	var debugFlag bool
	flag.BoolVar(&debugFlag, "debug", false, "Enable debug-level logging")
	var versionFlag bool
	flag.BoolVar(&versionFlag, "version", false, "Print version information and exit")
	var initSuperuser bool
	flag.BoolVar(&initSuperuser, "init-superuser", false, "Initialize superuser account")

	// Custom help message
	flag.Usage = printHelp

	flag.Parse()

	// Handle --version flag: print version and exit immediately
	if versionFlag {
		fmt.Printf("RAGFlow version: %s\n", utility.GetRAGFlowVersion())
		return
	}

	// Initialize logger
	if err := common.Init("info", "admin_server.log"); err != nil {
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

	if err := common.Init(logLevel, "admin_server.log"); err != nil {
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
	// This must be done after Cache is initialized
	if err := server.InitVariables(redis.Get()); err != nil {
		common.Warn("Failed to initialize server variables from Redis, using defaults", zap.String("error", err.Error()))
	}

	adminService := admin.NewService()
	adminHandler := admin.NewHandler(adminService)

	if initSuperuser {
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
	if cfg.Server.Mode == "debug" {
		ginEngine.Use(gin.Logger())
	}
	ginEngine.Use(gin.Recovery())
	// Log request URL for every request
	ginEngine.Use(func(c *gin.Context) {
		common.Info("HTTP Request", zap.String("url", c.Request.URL.String()), zap.String("method", c.Request.Method))
		c.Next()
	})

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
