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
	"net"
	"net/http"
	"ragflow/internal/utility"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	adminapi "ragflow/internal/admin"
	"ragflow/internal/common"
	"ragflow/internal/server"
	"ragflow/internal/server/bootstrap"
)

// Run starts the RAGFlow admin HTTP server. It blocks until ctx is canceled,
// then gracefully shuts the HTTP server down.
func Run(ctx context.Context, serverType common.ServerType, arguments *bootstrap.ServerArgs) error {
	globalConfig := server.GetConfig()

	adminService := adminapi.NewService()
	adminHandler := adminapi.NewHandler(adminService)

	if err := adminapi.InitLicense(ctx); err != nil {
		common.Warn("Failed to initialize license", zap.Error(err))
	}

	if arguments.InitSuperUser {
		// Initialize default admin user
		if err := adminService.InitDefaultAdmin(); err != nil {
			return fmt.Errorf("initialize default admin user: %w", err)
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
		return fmt.Errorf("configure trusted proxies: %w", err)
	}

	// Middleware
	ginEngine.Use(common.GinLogger())
	ginEngine.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		if !c.Writer.Written() {
			common.Respond(c, http.StatusInternalServerError, common.CodeServerError, nil, fmt.Sprint(recovered))
		}
		c.Abort()
	}))

	// Setup routes
	r.Setup(ginEngine)

	adminConfig := globalConfig.GetAdminServerConfig()
	addr := fmt.Sprintf(":%d", adminConfig.HTTPPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: ginEngine,
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen admin server on %s: %w", addr, err)
	}
	defer listener.Close()
	if serverType == common.ServerTypeAdmin {
		// Print RAGFlow Admin logo
		common.Info("" +
			"\n        ____  ___   ______________                 ___       __          _     \n" +
			"       / __ \\/   | / ____/ ____/ /___ _      __   /   | ____/ /___ ___  (_)___ \n" +
			"      / /_/ / /| |/ / __/ /_  / / __ \\ | /| / /  / /| |/ __  / __ `__ \\/ / __ \\ \n" +
			"     / _, _/ ___ / /_/ / __/ / / /_/ / |/ |/ /  / ___ / /_/ / / / / / / / / / /\n" +
			"    /_/ |_/_/  |_\\____/_/   /_/\\____/|__/|__/  /_/  |_\\__,_/_/ /_/ /_/_/_/ /_/ \n")
	}

	// Print RAGFlow version
	common.Info(fmt.Sprintf("RAGFlow admin version: %s", common.GetRAGFlowVersion()))

	validator := utility.NewScheduledTask("Admin validator", globalConfig.GetHeartbeatInterval(), adminapi.CheckServicePeriodically)
	validator.Start()
	defer validator.Wait()
	defer validator.Stop()
	stopMetrics := startMetrics(ctx)
	defer stopMetrics()

	// Start HTTP server in a goroutine
	serveErr := make(chan error, 1)
	common.Info(fmt.Sprintf("Starting RAGFlow admin HTTP server on port: %d", adminConfig.HTTPPort))
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	if arguments.Ready != nil {
		if err = arguments.Ready(); err != nil {
			srv.Close()
			return fmt.Errorf("report admin readiness: %w", err)
		}
	}

	// Wait for shutdown signal from main's signal.NotifyContext
	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-serveErr:
	}

	common.Info("Received shutdown signal")
	common.Info("Shutting down RAGFlow HTTP server...")

	// Create context with timeout for graceful shutdown
	quitCtx, quitCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer quitCancel()

	// Shutdown HTTP server
	if err = srv.Shutdown(quitCtx); err != nil {
		srv.Close()
		return fmt.Errorf("shutdown admin server: %w", err)
	}

	common.Info("Admin HTTP server exited")
	return runErr
}
