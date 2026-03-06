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
	"flag"
	"os"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/admin"
	"ragflow/internal/dao"
	"ragflow/internal/logger"
	"ragflow/internal/server"
	"ragflow/internal/utility"
)

// AdminServer admin server
type AdminServer struct {
	router  *admin.Router
	handler *admin.Handler
	service *admin.Service
	engine  *gin.Engine
	port    string
}

// NewAdminServer create admin server
func NewAdminServer(port string) *AdminServer {
	return &AdminServer{
		port: port,
	}
}

// Init initialize admin server
func (s *AdminServer) Init() error {
	gin.SetMode(gin.ReleaseMode)
	s.engine = gin.New()
	s.engine.Use(gin.Recovery())

	// Initialize layers
	s.service = admin.NewService()
	s.handler = admin.NewHandler(s.service)
	s.router = admin.NewRouter(s.handler)

	// Setup routes
	s.router.Setup(s.engine)

	return nil
}

// Run start admin server
func (s *AdminServer) Run() error {
	logger.Info("Starting admin server", zap.String("port", s.port))
	return s.engine.Run(":" + s.port)
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

	// Set logger for server package
	server.SetLogger(logger.Logger)

	cfg := server.GetConfig()
	logger.Info("Configuration loaded",
		zap.String("database_host", cfg.Database.Host),
		zap.Int("database_port", cfg.Database.Port),
	)

	// Initialize database
	if err := dao.InitDB(); err != nil {
		logger.Error("Failed to initialize database", err)
		os.Exit(1)
	}

	// Create and start admin server (port 9381)
	adminServer := NewAdminServer("9381")
	if err := adminServer.Init(); err != nil {
		logger.Error("Failed to initialize admin server", err)
		os.Exit(1)
	}

	// Print RAGFlow Admin logo
	logger.Info("" +
		"\n        ____  ___   ______________                 ___       __          _     \n" +
		"       / __ \\/   | / ____/ ____/ /___ _      __   /   | ____/ /___ ___  (_)___ \n" +
		"      / /_/ / /| |/ / __/ /_  / / __ \\ | /| / /  / /| |/ __  / __ `__ \\/ / __ \\ \n" +
		"     / _, _/ ___ / /_/ / __/ / / /_/ / |/ |/ /  / ___ / /_/ / / / / / / / / / / /\n" +
		"    /_/ |_/_/  |_\\____/_/   /_/\\____/|__/|__/  /_/  |_\\__,_/_/ /_/ /_/_/_/ /_/ \n")

	// Print RAGFlow version
	logger.Info("RAGFlow version", zap.String("version", utility.GetRAGFlowVersion()))

	// Print all configuration settings
	server.PrintAll()

	logger.Info("Starting RAGFlow Admin Server", zap.String("port", "9381"))
	if err := adminServer.Run(); err != nil {
		logger.Error("Admin server error", err)
		os.Exit(1)
	}
}
