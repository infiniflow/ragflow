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
	"os"
	"os/signal"
	"ragflow/internal/ingestion"
	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"
	"syscall"
	"time"

	"ragflow/internal/cache"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/server"
	"ragflow/internal/storage"

	"go.uber.org/zap"
)

func printIngestionServerHelp() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "RAGFlow Ingestion Worker - Document ingestion processing\n\n")
	fmt.Fprintf(os.Stderr, "Options:\n")
	fmt.Fprintf(os.Stderr, "  -f string\t\tPath to config file (default: auto-detect)\n")
	fmt.Fprintf(os.Stderr, "  --name string\t\tIngestion server name (default: \"default_ingestion\")\n")
	fmt.Fprintf(os.Stderr, "  --admin-host string\tAdmin server host (overrides config file)\n")
	fmt.Fprintf(os.Stderr, "  --admin-port int\tAdmin server port (overrides config file)\n")
	fmt.Fprintf(os.Stderr, "  -h, --help\t\tShow this help message and exit\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  %s                          # Start with default config\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s -f /path/to/config.yaml   # Start with custom config file\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s --admin-host 10.0.0.1 --admin-port 9383\n", os.Args[0])
}

func main() {
	// Parse command line flags
	var configPath string
	var name string
	var adminHost string
	var adminPort int

	flag.StringVar(&configPath, "f", "", "Path to config file (overrides auto-detect)")
	flag.StringVar(&name, "name", "default_ingestion", "Ingestion server name")
	flag.StringVar(&adminHost, "admin-host", "", "Admin server host (overrides config file)")
	flag.IntVar(&adminPort, "admin-port", 0, "Admin server port (overrides config file)")

	// Custom help message
	flag.Usage = printIngestionServerHelp

	flag.Parse()

	// Initialize logger with default level
	if err := common.Init("info", "ingestion_server.log"); err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}

	// Initialize configuration
	if err := server.Init(configPath); err != nil {
		common.Fatal("Failed to initialize config", zap.Error(err))
	}

	config := server.GetConfig()

	// Override admin server host with command line argument if provided
	if adminHost != "" {
		config.Admin.Host = adminHost
		common.Info("Admin host overridden by command line argument", zap.String("admin_host", adminHost))
	}

	// Override admin server port with command line argument if provided
	if adminPort > 0 {
		config.Admin.Port = adminPort
		common.Info("Admin port overridden by command line argument", zap.Int("admin_port", adminPort))
	}

	// Reinitialize logger with configured level if different
	level := config.Log.Level
	if level == "" {
		level = "info"
	}
	if err := common.Init(level, "ingestion_server.log"); err != nil {
		common.Error("Failed to reinitialize logger", err)
	}
	server.SetLogger(common.Logger)

	common.Info("Starting RAGFlow Ingestion Worker")

	// Initialize database
	if err := dao.InitDB(); err != nil {
		common.Fatal("Failed to initialize database", zap.Error(err))
	}

	// Initialize doc engine
	if err := engine.Init(&config.DocEngine); err != nil {
		common.Fatal("Failed to initialize doc engine", zap.Error(err))
	}
	defer engine.Close()

	// Initialize Redis cache
	if err := cache.Init(&config.Redis); err != nil {
		common.Fatal("Failed to initialize Redis", zap.Error(err))
	}
	defer cache.Close()

	// Initialize storage factory
	if err := storage.InitStorageFactory(); err != nil {
		common.Fatal("Failed to initialize storage factory", zap.Error(err))
	}

	// Initialize server variables (runtime variables from Redis)
	if err := server.InitVariables(cache.Get()); err != nil {
		common.Warn("Failed to initialize server variables from Redis, using defaults", zap.String("error", err.Error()))
	}

	// Initialize tokenizer (rag_analyzer)
	tokenizerCfg := &tokenizer.PoolConfig{
		DictPath: "/usr/share/infinity/resource",
	}
	if err := tokenizer.Init(tokenizerCfg); err != nil {
		common.Fatal("Failed to initialize tokenizer", zap.Error(err))
	}
	defer tokenizer.Close()

	// Initialize global QueryBuilder using tokenizer's DictPath
	if err := nlp.InitQueryBuilderFromTokenizer(tokenizerCfg.DictPath); err != nil {
		common.Fatal("Failed to initialize query builder", zap.Error(err))
	}

	ingestor := ingestion.NewIngestor(name, 2, []string{"pdf", "docx", "txt"})

	// Connect to the admin server
	serverAddress := fmt.Sprintf("%s:%d", config.Admin.Host, config.Admin.IngestionManagerPort)
	if err := ingestor.Connect(serverAddress); err != nil {
		common.Fatal(fmt.Sprintf("Error: %s", err.Error()))
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR2)

	// Print all configuration settings
	server.PrintAll()
	common.Info("\n    ____                      __  _\n" +
		"   /  _/___  ____ ____  _____/ /_(_)___  ____     ________  ______   _____  _____\n" +
		"   / // __ \\/ __ `/ _ \\/ ___/ __/ / __ \\/ __ \\   / ___/ _ \\/ ___/ | / / _ \\/ ___/\n" +
		" _/ // / / / /_/ /  __(__  ) /_/ / /_/ / / / /  (__  )  __/ /   | |/ /  __/ /\n" +
		"/___/_/ /_/\\__, /\\___/____/\\__/_/\\____/_/ /_/  /____/\\___/_/    |___/\\___/_/\n" +
		"          /____/\n")

	// Print RAGFlow version
	common.Info(fmt.Sprintf("RAGFlow admin version: %s", utility.GetRAGFlowVersion()))

	// Wait for either an OS signal or a shutdown command from the admin
	select {
	case sig := <-quit:
		common.Info("Received signal", zap.String("signal", sig.String()))
		common.Info(fmt.Sprintf("Shutting down RAGFlow ingestor %s ...", name))
	case <-ingestor.ShutdownCh:
		common.Info(fmt.Sprintf("Received shutdown command from admin, stopping ingestor %s ...", name))
	}

	// Create context with timeout for graceful shutdown
	_, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ingestor.Stop()

	common.Info(fmt.Sprintf("Ingestor %s shutdown complete", name))
}
