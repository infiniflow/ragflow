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

// Package admin wires and runs the RAGFlow admin HTTP server. It was extracted
// from package main so that main stays a thin dispatch layer.
package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	adminapi "ragflow/internal/admin"
	"ragflow/internal/common"
	"ragflow/internal/server"
	"ragflow/internal/server/bootstrap"
)

// Run starts the RAGFlow admin HTTP server. It blocks until ctx is cancelled,
// then gracefully shuts the HTTP server down.
func Run(ctx context.Context, serverName string, args *bootstrap.ServerArgs) error {
	globalConfig := server.GetConfig()
	serverMode := globalConfig.GetMode()

	// Set Gin mode
	if serverMode == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	adminService := adminapi.NewService()
	adminHandler := adminapi.NewHandler(adminService)

	if err := adminapi.InitLicense(ctx); err != nil {
		common.Warn("Failed to initialize license", zap.Error(err))
	}

	if args.InitSuperUser {
		// Initialize default admin user
		if err := adminService.InitDefaultAdmin(); err != nil {
			common.Error("Failed to initialize default admin user", err)
		}
	}

	// Initialize router
	r := adminapi.NewRouter(adminHandler)

	// Create Gin engine
	ginEngine := gin.New()
	// Mirror Quart's merge_slashes: collapse duplicate slashes before routing.
	ginEngine.RemoveExtraSlash = true
	// Only honor X-Forwarded-For / X-Real-IP from the configured proxies
	// (default: the loopback nginx bundled in the image), never from every peer.
	if err := common.ConfigureTrustedProxies(ginEngine, globalConfig.GetAPIServerConfig().TrustedProxies); err != nil {
		common.Fatal("Failed to configure trusted proxies", zap.Error(err))
	}

	// Middleware
	ginEngine.Use(common.GinLogger())
	ginEngine.Use(gin.Recovery())

	// Setup routes
	r.Setup(ginEngine)

	adminConfig := globalConfig.GetAdminServerConfig()
	addr := fmt.Sprintf(":%d", adminConfig.HTTPPort)
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
	common.Info(fmt.Sprintf("RAGFlow admin version: %s", common.GetRAGFlowVersion()))

	// Set log level
	bootstrap.SetLogger(serverName, args)

	// Start HTTP server in a goroutine
	go func() {
		common.Info(fmt.Sprintf("Starting RAGFlow admin HTTP server on port: %d", adminConfig.HTTPPort))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			common.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for shutdown signal from main's signal.NotifyContext
	<-ctx.Done()

	common.Info("Received shutdown signal")
	common.Info("Shutting down RAGFlow HTTP server...")

	// Create context with timeout for graceful shutdown
	quitCtx, quitCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer quitCancel()

	// Shutdown HTTP server
	if err := srv.Shutdown(quitCtx); err != nil {
		common.Fatal("Server forced to shutdown", zap.Error(err))
	}

	common.Info("Admin HTTP server exited")
	return nil
}
