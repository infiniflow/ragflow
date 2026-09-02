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
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"ragflow/internal/admin"
	"ragflow/internal/agent/audio"
	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/retrievalbridge"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/channels"
	"ragflow/internal/handler"
	"ragflow/internal/ingestion/knowledge_compile"
	ingestion "ragflow/internal/ingestion/service"
	"ragflow/internal/mcp"
	"ragflow/internal/router"
	"ragflow/internal/server/local"
	"ragflow/internal/service"
	"ragflow/internal/service/chunk"
	dataset "ragflow/internal/service/dataset"
	"ragflow/internal/service/document"
	"ragflow/internal/service/file"
	"ragflow/internal/service/nav"
	"ragflow/internal/service/nlp"
	"ragflow/internal/service/wikisearch"
	"ragflow/internal/storage"
	"ragflow/internal/syncer"
	"ragflow/internal/tokenizer"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/agent/component"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	"ragflow/internal/engine"
	"ragflow/internal/engine/redis"
	_ "ragflow/internal/ingestion/wire"
	"ragflow/internal/server"
	"ragflow/internal/servermode"
	"ragflow/internal/utility"
)

type serverArgs struct {
	mode          *string // admin | api | ingestor | syncer
	helpFlag      bool
	versionFlag   bool
	debugLog      bool
	migrateDB     bool
	configPath    *string // Used by admin, api; user defined config path
	initSuperUser bool    // Used by admin;
	port          *int    // Used by admin, api
	adminHost     *string // Used by api, ingestor, syncer for heartbeat
	adminPort     *int    // Used by api, ingestor, syncer for heartbeat, "ip:port"
	name          *string // server name
}

func parseArgs() (*serverArgs, error) {
	args := &serverArgs{}

	var serverMode string
	var configPath string
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch arg {
		case "--admin":
			serverMode = "admin"
			args.mode = &serverMode
		case "--migrate":
			args.migrateDB = true
		case "--ingestor":
			serverMode = "ingestor"
			args.mode = &serverMode
		case "--api":
			serverMode = "api"
			args.mode = &serverMode
		case "--syncer":
			serverMode = "syncer"
			args.mode = &serverMode
		case "-h", "--help":
			args.helpFlag = true
		case "-v", "--version":
			args.versionFlag = true
		case "--debug":
			args.debugLog = true
		case "-f", "--config":
			if i+1 >= len(os.Args) {
				return nil, fmt.Errorf("%s requires a value", arg)
			}
			i++
			configPath = os.Args[i]
			args.configPath = &configPath
		case "--init-superuser":
			args.initSuperUser = true
		case "-p", "--port":
			if i+1 >= len(os.Args) {
				return nil, errors.New("--port requires a value")
			}
			i++
			port, convErr := strconv.Atoi(os.Args[i])
			if convErr != nil {
				return nil, fmt.Errorf("invalid port: %w", convErr)
			}
			args.port = &port
			if port <= 0 || port > 65535 {
				return nil, fmt.Errorf("invalid port: %d", port)
			}
		case "--admin-host":
			if i+1 >= len(os.Args) {
				return nil, errors.New("--admin-host requires a value")
			}
			i++
			parts := strings.SplitN(os.Args[i], ":", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return nil, errors.New("--admin-host must be in the form 'ip:port'")
			}
			ip, portStr := parts[0], parts[1]
			port, convErr := strconv.Atoi(portStr)
			if convErr != nil {
				return nil, fmt.Errorf("failed to parse admin port: %w", convErr)
			}
			args.adminHost = &ip
			args.adminPort = &port
		case "--name":
			if i+1 >= len(os.Args) {
				return nil, errors.New("--name requires a value")
			}
			i++
			args.name = &os.Args[i]
		default:
			return nil, fmt.Errorf("unknown parameter: %s", arg)
		}
	}
	return args, nil
}

// registerNativeDeepDoc wires the in-process (Go) DeepDoc backend as the local
// fallback used when no external DeepDoc HTTP service is configured. It is
// compiled into the server built with -tags cgo, which statically links the
// ONNX Runtime backend (libonnxruntime.a); the unit-test tier builds without
// cgo and stays free of the onnxruntime dependency.
func printHelp(args *serverArgs) {
	switch {
	case args.mode == nil:
		fmt.Fprintf(os.Stderr, "Usage: %s --api|--admin|--ingestor|--syncer [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Server - Open-source RAG engine based on deep document understanding\n\n")
		fmt.Fprintf(os.Stderr, "Mode selection (default: --api):\n")
		fmt.Fprintf(os.Stderr, "  --api          \tRun as API server\n")
		fmt.Fprintf(os.Stderr, "  --admin        \tRun as admin server\n")
		fmt.Fprintf(os.Stderr, "  --ingestor     \tRun as ingestion worker\n")
		fmt.Fprintf(os.Stderr, "  --syncer       \tRun as file sync service\n\n")
		fmt.Fprintf(os.Stderr, "Common options:\n")
		fmt.Fprintf(os.Stderr, "  --config string\tPath to configuration file\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug        \tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \tShow this help message and exit\n\n")
		fmt.Fprintf(os.Stderr, "Run '%s --api --help' for API server options.\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Run '%s --admin --help' for admin server options.\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Run '%s --ingestor --help' for ingester options.\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Run '%s --syncer --help' for syncer options.\n", os.Args[0])
	case *args.mode == "api":
		fmt.Fprintf(os.Stderr, "Usage: %s --api [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow API Server\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  --port int     	\tServer port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\tPath to configuration file\n")
		fmt.Fprintf(os.Stderr, "  -v, --version 	 \tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug       	 \tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  -h, --help       	  \tShow this help message and exit\n")
	case *args.mode == "admin":
		fmt.Fprintf(os.Stderr, "Usage: %s --admin [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Admin Server\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\t\tPath to configuration file\n")
		fmt.Fprintf(os.Stderr, "  --port int    \t\t\tServer port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  --init-superuser\t\t\tInitialize superuser account\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \t\t\tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug        \t\t\tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \t\t\tShow this help message and exit\n")
	case *args.mode == "ingestor":
		fmt.Fprintf(os.Stderr, "Usage: %s --ingestor [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Ingestion Worker - Document ingestion processing\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\tPath to config file\n")
		fmt.Fprintf(os.Stderr, "  --name string\t\t\tIngestion server name (default: \"default_ingestion\")\n")
		fmt.Fprintf(os.Stderr, "  --admin-host string\tAdmin server host:port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \t\tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug        \t\tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \t\tShow this help message and exit\n")
	case *args.mode == "syncer":
		fmt.Fprintf(os.Stderr, "Usage: %s --syncer [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Sync Service - Sync files from source to RAGFlow\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\tPath to config file\n")
		fmt.Fprintf(os.Stderr, "  --name string\t\t\tSync service server name (default: \"default_syncer\")\n")
		fmt.Fprintf(os.Stderr, "  --admin-host string\tAdmin server host:port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \t\tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug        \t\tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \t\tShow this help message and exit\n")
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer cancel()

	arguments, err := parseArgs()
	if err != nil {
		fmt.Printf("Failed to parse arguments: %v\n", err)
		os.Exit(1)
	}

	if arguments.helpFlag || arguments.mode == nil {
		printHelp(arguments)
		os.Exit(1)
	}

	if arguments.versionFlag {
		fmt.Printf("RAGFlow version: %s\n", common.GetRAGFlowVersion())
		os.Exit(1)
	}

	// Initialize local variables (runtime variables from Redis)
	err = server.InitLocalVariables()
	if err != nil {
		fmt.Printf("Failed to start %s server: %v\n", *arguments.mode, err)
		os.Exit(1)
	}

	// Temporary logger initialization
	var logFileName string
	var serverName string
	if arguments.name != nil {
		serverName = *arguments.name
	} else {
		serverName = fmt.Sprintf("%s_server", *arguments.mode)
	}
	logFileName = fmt.Sprintf("%s.log", serverName)

	logLevel := "info"
	if arguments.debugLog {
		logLevel = "debug"
	}

	if err = common.InitLogger(logLevel, common.FileOutput{Filename: logFileName, Path: "logs"}, serverName); err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	// Initialize configuration
	var configPath string
	if arguments.configPath != nil {
		configPath = *arguments.configPath
	}

	if err = server.Init(configPath); err != nil {
		common.Error("Failed to initialize configuration", err)
		os.Exit(1)
	}

	// Register the in-process (Go) DeepDoc backend for the modes that run the
	// document parsing pipeline. This server is built with -tags cgo and links
	// ONNX Runtime statically (libonnxruntime.a, resolved via dlopen(NULL)),
	// so the in-process backend is the production backend with no external
	// DeepDoc HTTP service. Fail fast if it cannot serve — but only for modes
	// that actually need it: "api" parses in-process (dataflow debug and other
	// routes) and "ingestor" runs the ingestion pipeline. "admin"/"syncer"
	// never instantiate the analyzer, so they must not fail-fast when ORT and
	// models are absent on their node.
	if servermode.NeedsDeepDoc(*arguments.mode) {
		registerNativeDeepDoc()
	}

	globalConfig := server.GetConfig()

	// override default port if provided
	switch *arguments.mode {
	case "api":
		apiServerConfig := globalConfig.GetAPIServerConfig()
		port := apiServerConfig.HTTPPort
		if arguments.port != nil {
			port = *arguments.port
			apiServerConfig.HTTPPort = port
		}
		if arguments.name == nil {
			serverName = fmt.Sprintf("api_server_%d", port)
		}
	case "admin":
		adminServerConfig := globalConfig.GetAdminServerConfig()
		port := adminServerConfig.HTTPPort
		if arguments.port != nil {
			port = *arguments.port
			adminServerConfig.HTTPPort = port
		}
		if arguments.name == nil {
			serverName = fmt.Sprintf("admin_server_%d", port)
		}
	case "ingestor":
		if serverName == "" {
			uuid := utility.GenerateUUID()
			serverName = fmt.Sprintf("ingestor_server_%s", uuid)
		}
	case "syncer":
		if serverName == "" {
			uuid := utility.GenerateUUID()
			serverName = fmt.Sprintf("syncer_server_%s", uuid)
		}
	default:
		err = errors.New(*arguments.mode)
		common.Error("invalid server mode", err)
		os.Exit(1)
	}

	// set server name and log file path
	server.SetServerName(serverName)

	// rename log filename
	logFileName = fmt.Sprintf("%s.log", serverName)

	logConfig := globalConfig.GetLogConfig()

	// Reinitialize logger with configured level if different
	logLevel = logConfig.Level
	if logLevel == "" {
		logLevel = "info"
	}

	if arguments.debugLog {
		logLevel = "debug"
	}

	globalConfig.SetLogLevel(logLevel)

	fileOut := common.FileOutput{
		Filename:   logFileName,
		Path:       logConfig.Path,
		MaxSize:    logConfig.MaxSize,
		MaxBackups: logConfig.MaxBackups,
		MaxAge:     logConfig.MaxAge,
		Compress:   logConfig.Compress,
	}

	common.SyncLog()
	if err = common.InitLogger(logLevel, fileOut, serverName); err != nil {
		common.Error("Failed to reinitialize logger with configured level", err)
	}

	// Print all configuration settings
	common.Info(fmt.Sprintf("Starting %s server: %s, mode: %s", *arguments.mode, serverName, globalConfig.GetMode()))
	server.PrintAll()

	// Initialize database
	if err = dao.InitDB(ctx, arguments.migrateDB); err != nil {
		common.Fatal("Failed to initialize database", zap.Error(err))
	}

	// Initialize doc engine
	if err = engine.InitDocEngine(ctx); err != nil {
		common.Fatal("Failed to initialize doc engine", zap.Error(err))
	}
	defer engine.Close()

	// Initialize Redis cache
	if err = redis.Init(ctx); err != nil {
		common.Fatal("Failed to initialize Redis", zap.Error(err))
	}
	defer redis.Close()

	if err = storage.Init(ctx); err != nil {
		common.Error("Failed to initialize storage factory", err)
	}
	defer storage.CloseStorage()

	if err = engine.InitMessageQueue(); err != nil {
		common.Fatal("Failed to initialize message queue engine", zap.Error(err))
	}

	// Initialize server variables (runtime variables that can change during operation)
	// This must be done after Cache is initialized
	if err = server.InitVariables(redis.Get()); err != nil {
		common.Warn("Failed to initialize server variables from Redis, using defaults", zap.String("error", err.Error()))
	}

	if err = server.StartServer(ctx, cancel, serverName); err != nil {
		common.Error("Failed to start EE server", err)
		os.Exit(1)
	}
	defer server.ShutdownServer(ctx)

	if arguments.name == nil {
		arguments.name = &serverName
	}

	switch *arguments.mode {
	case "api":
		if err = runAPI(ctx, arguments); err != nil {
			fmt.Printf("Failed to start API server: %v\n", err)
			os.Exit(1)
		}
	case "admin":
		if err = runAdmin(ctx, arguments); err != nil {
			fmt.Printf("Failed to start ADMIN server: %v\n", err)
			os.Exit(1)
		}
	case "ingestor":
		if err = runIngestor(ctx, cancel, arguments); err != nil {
			fmt.Printf("Failed to start INGESTION worker: %v\n", err)
			os.Exit(1)
		}
	case "syncer":
		if err = runSyncer(ctx, cancel, arguments); err != nil {
			fmt.Printf("Failed to start SYNCER: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Invalid server mode: %s\n", *arguments.mode)
		os.Exit(1)
	}
}

func runAdmin(ctx context.Context, args *serverArgs) error {

	globalConfig := server.GetConfig()
	serverMode := globalConfig.GetMode()

	// Set Gin mode
	if serverMode == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	adminService := admin.NewService()
	adminHandler := admin.NewHandler(adminService)

	if err := admin.InitLicense(); err != nil {
		common.Warn("Failed to initialize license", zap.Error(err))
	}

	if args.initSuperUser {
		// Initialize default admin user
		if err := adminService.InitDefaultAdmin(); err != nil {
			common.Error("Failed to initialize default admin user", err)
		}
	}

	// Initialize router
	r := admin.NewRouter(adminHandler)

	// Create Gin engine
	ginEngine := gin.New()
	// Mirror Quart's merge_slashes: collapse duplicate slashes before routing.
	ginEngine.RemoveExtraSlash = true

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

// startHeartbeat initializes and starts the heartbeat reporter to the admin server.
// It is shared by API, ingestion, and syncer server modes.
// The caller must defer the returned *utility.ScheduledTask's Stop() method.
func startHeartbeat(serverType common.ServerType, serverID string, port int, heartBeatInterval time.Duration) *utility.ScheduledTask {
	localIP, err := utility.GetLocalIP()
	if err != nil {
		common.Fatal("fail to get local ip address")
	}

	service.AdminServiceClient = service.NewAdminClient(
		serverType,
		serverID,
		localIP,
		port,
	)
	if err = service.AdminServiceClient.InitHTTPClient(); err != nil {
		common.Warn("Failed to initialize heartbeat service", zap.Error(err))
		return nil
	}

	heartbeatReporter := utility.NewScheduledTask("Heartbeat reporter", heartBeatInterval, func() {
		if err = service.AdminServiceClient.SendHeartbeat(); err == nil {
			local.SetAdminStatus(0, "")
		} else {
			local.SetAdminStatus(1, err.Error())
		}
	})
	heartbeatReporter.Start()
	return heartbeatReporter
}

func runIngestor(ctx context.Context, cancel context.CancelFunc, args *serverArgs) error {
	// Initialize tokenizer (rag_analyzer)
	// tokenizer.Init handles DictPath fallback: env var → /usr/share/infinity/resource
	if err := tokenizer.Init(&tokenizer.PoolConfig{}); err != nil {
		common.Fatal("Failed to initialize tokenizer", zap.Error(err))
	}
	defer tokenizer.Close()

	// Fail fast if the cl100k_base BPE table is missing: without it
	// NumTokensFromString silently returns 0, corrupting every token budget.
	if err := tokenizer.InitCL100KEncoder(); err != nil {
		common.Fatal("Failed to initialize cl100k_base tokenizer", zap.Error(err))
	}

	// The dataset-level post-processing consumer cluster (§11) is owned and run by
	// the Ingestor: it is started inside ingestor.Start() and joined inside
	// ingestor.Stop(), so its lifecycle matches the ingestor. The configured
	// default LLM/embedding ids are passed so the LLM deduper is used (instead
	// of the noop fallback that still emits merged products). Best-effort: a
	// provisioning error is logged by the Ingestor and the pipeline still
	// writes available_int=0 compiled chunks; they just won't be merged until
	// the consumer is available.
	globalConfig := server.GetConfig()
	ingestorCfg := globalConfig.GetIngestorConfig()
	const maxIngestorConcurrency = int32(1<<30 - 1)
	if ingestorCfg.MaxConcurrentWorkers > int(maxIngestorConcurrency) {
		return fmt.Errorf("ingestor max_concurrent_workers %d exceeds maximum %d", ingestorCfg.MaxConcurrentWorkers, maxIngestorConcurrency)
	}
	// Apply the configured compiler pool size (no-op when 0; the pool keeps its
	// vCPU default, overridable via KC_COMPILE_CONCURRENCY).
	knowledge_compile.SetCompilerConcurrency(ingestorCfg.CompilerPoolSize)
	ingestor := ingestion.NewIngestor(*args.name, int32(ingestorCfg.MaxConcurrentWorkers), []string{"pdf", "docx", "txt"})
	ingestor.SetKnowledgeCompileModelConfig(
		globalConfig.GetDefaultChatModel().Name,
		globalConfig.GetDefaultEmbeddingModel().Name,
	)
	// The dataset-level knowledge-compile consumer (tree/structure products) upserts
	// into the dataset-nav tree, so the Ingestor must install the same ES-backed
	// NavService the API server installs. Without this, nav.GetNavService() returns
	// nil and tree/structure products are dropped (the consumer logs "nav service
	// unavailable, skipping dataset-nav upsert"), leaving the dataset tree empty.
	// The embedder resolves the tenant's embedding model on demand, so both
	// Search and UpsertDoc can embed queries/summaries automatically.
	nav.SetNavService(nlp.NewNavService(service.NewNavEmbedder(service.NewModelProviderService(), "")))
	// Memory extraction runs on the Ingestor's shared NATS consumer + worker
	// pool (task_type="memory" dispatched by processMessage -> executeMemoryTask),
	// so there is no longer a dedicated Redis memory consumer to start.
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(service.NewMemoryService()))

	// Start returns immediately (it launches the owned consume/compile
	// goroutines and joins them via Stop); a provisioning failure here must
	// fail the server (main's os.Exit(1) path) instead of reporting a
	// healthy ingestor that can never consume.
	if err := ingestor.Start(); err != nil {
		common.Error("Failed to initialize ingestor", err)
		return err
	}

	common.Info("\n    ____                      __  _\n" +
		"   /  _/___  ____ ____  _____/ /_(_)___  ____     ________  ______   _____  _____\n" +
		"   / // __ \\/ __ `/ _ \\/ ___/ __/ / __ \\/ __ \\   / ___/ _ \\/ ___/ | / / _ \\/ ___/\n" +
		" _/ // / / / /_/ /  __(__  ) /_/ / /_/ / / / /  (__  )  __/ /   | |/ /  __/ /\n" +
		"/___/_/ /_/\\__, /\\___/____/\\__/_/\\____/_/ /_/  /____/\\___/_/    |___/\\___/_/\n" +
		"          /____/\n")

	// Print RAGFlow version
	common.Info(fmt.Sprintf("RAGFlow ingestion service version: %s", common.GetRAGFlowVersion()))

	// Start heartbeat reporter to admin server
	if hb := startHeartbeat(
		common.ServerTypeIngestion,
		fmt.Sprintf("ingestor-%s", ingestor.ID()),
		-1,
		globalConfig.GetHeartbeatInterval(),
	); hb != nil {
		defer hb.Stop()
	}

	// Wait for either an OS shutdown signal or a shutdown command from the admin
	select {
	case <-ctx.Done():
		common.Info("Received shutdown signal")
		common.Info(fmt.Sprintf("Shutting down RAGFlow ingestor %s ...", *args.name))
	case <-ingestor.ShutdownCh:
		common.Info(fmt.Sprintf("Received shutdown command from admin, stopping ingestor %s ...", *args.name))
		cancel()
	}

	// Create context with timeout for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ingestor.Stop(shutdownCtx)

	common.Info(fmt.Sprintf("Ingestor %s shutdown complete", *args.name))

	return nil
}

func runSyncer(ctx context.Context, cancel context.CancelFunc, args *serverArgs) error {
	globalConfig := server.GetConfig()
	syncerConfig := globalConfig.GetSyncerConfig()
	fileSyncer := syncer.NewSyncer(syncerConfig.MaxConcurrentSyncs)

	if err := fileSyncer.StartContext(ctx); err != nil {
		common.Error("Failed to initialize file syncer", err)
		return err
	}

	common.Info("\n     _______ __        _____\n" +
		"    / ____(_) /__     / ___/__  ______  ________  _____\n" +
		"   / /_  / / / _ \\    \\__ \\/ / / / __ \\/ ___/ _ \\/ ___/\n" +
		"  / __/ / / /  __/   ___/ / /_/ / / / / /__/  __/ /\n" +
		" /_/   /_/_/\\___/   /____/\\__, /_/ /_/\\___/\\___/_/\n" +
		"                           /____/    \n")

	// Print RAGFlow version
	common.Info(fmt.Sprintf("RAGFlow file syncer service version: %s", common.GetRAGFlowVersion()))

	// Start heartbeat reporter to admin server
	if hb := startHeartbeat(
		common.ServerTypeFileSyncer,
		fmt.Sprintf("syncer-%s", fileSyncer.ID()),
		-1,
		globalConfig.GetHeartbeatInterval(),
	); hb != nil {
		defer hb.Stop()
	}

	// Wait for either an OS shutdown signal or a shutdown command from the admin
	select {
	case <-ctx.Done():
		common.Info("Received shutdown signal")
		common.Info(fmt.Sprintf("Shutting down RAGFlow file syncer %s ...", *args.name))
	case <-fileSyncer.ShutdownCh:
		common.Info(fmt.Sprintf("Received shutdown command from admin, stopping file syncer %s ...", *args.name))
		cancel()
	}

	fileSyncer.Stop()
	common.Info(fmt.Sprintf("File syncer %s shutdown complete", *args.name))

	return nil
}

func runAPI(ctx context.Context, args *serverArgs) error {
	// Initialize admin status (default: unavailable=1)
	local.InitAdminStatus(1, "admin server not connected")

	// Initialize tokenizer (rag_analyzer)
	// tokenizer.Init fills DictPath from env var or default, so
	// tokenizerCfg.DictPath carries the resolved path for downstream use.
	tokenizerCfg := &tokenizer.PoolConfig{}
	if err := tokenizer.Init(tokenizerCfg); err != nil {
		common.Fatal("Failed to initialize tokenizer", zap.Error(err))
	}
	defer tokenizer.Close()

	// Fail fast if the cl100k_base BPE table is missing: without it
	// NumTokensFromString silently returns 0, corrupting every token budget.
	if err := tokenizer.InitCL100KEncoder(); err != nil {
		common.Fatal("Failed to initialize cl100k_base tokenizer", zap.Error(err))
	}

	// Initialize global QueryBuilder using tokenizer's DictPath
	// This ensures the Synonym uses the same wordnet directory as tokenizer
	if err := nlp.InitQueryBuilderFromTokenizer(tokenizerCfg.DictPath); err != nil {
		common.Fatal("Failed to initialize query builder", zap.Error(err))
	}

	startServer(ctx)

	common.Info("Server exited")

	return nil
}

func startServer(ctx context.Context) {

	globalConfig := server.GetConfig()
	serverMode := globalConfig.GetMode()
	// Set Gin mode
	if serverMode == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize service layer
	userService := service.NewUserService()
	documentService := document.NewDocumentService()
	datasetsService := dataset.NewDatasetService()
	metadataService := service.NewMetadataService()
	chunkService := chunk.NewChunkService()
	llmService := service.NewLLMService()
	tenantService := service.NewTenantService()
	chatService := service.NewChatService()
	chatChannelService := service.NewChatChannelService()
	langfuseService := service.NewLangfuseService()
	chatSessionService := service.NewChatSessionService()
	openaiChatService := service.NewOpenAIChatService()
	systemService := service.NewSystemService()
	statsService := service.NewStatsService()
	connectorService := service.NewConnectorService()
	searchService := service.NewSearchService()
	searchService.SetTenantService(tenantService)
	fileService := file.NewFileService(service.CheckFileTeamPermission, documentService)
	memoryService := service.NewMemoryService()
	mcpService := service.NewMCPService()
	modelProviderService := service.NewModelProviderService()

	// Wire the real MemorySaver so the Message component can persist
	// conversation turns to memory stores declared in the canvas DSL.
	component.SetMemorySaver(service.NewMemorySaverAdapter(memoryService))

	// Initialize doc engine for skill search
	docEngine := engine.Get()
	documentDAO := dao.NewDocumentDAO()
	retrievalEnhancer := retrievalbridge.NewEnhancer(docEngine, metadataService)
	agenttool.SetRetrievalService(agenttool.NewNLPRetrievalAdapterFromDeps(
		docEngine,
		documentDAO,
		modelProviderService,
		retrievalEnhancer,
	))
	agenttool.SetMemoryRetrievalService(retrievalbridge.NewMemoryAdapter(memoryService))
	common.Info("agent: retrieval service adapter installed")

	// Initialize handler layer
	authHandler := handler.NewAuthHandler()
	userHandler := handler.NewUserHandler(userService)
	tenantHandler := handler.NewTenantHandler(tenantService, userService, datasetsService)
	documentHandler := handler.NewDocumentHandler(documentService, datasetsService, fileService)
	datasetsHandler := handler.NewDatasetsHandler(datasetsService, metadataService)
	systemHandler := handler.NewSystemHandler(systemService)
	statsHandler := handler.NewStatsHandler(statsService)
	chunkHandler := handler.NewChunkHandler(chunkService, userService)
	llmHandler := handler.NewLLMHandler(llmService, userService)
	chatHandler := handler.NewChatHandler(chatService, userService)
	chatChannelHandler := handler.NewChatChannelHandler(chatChannelService)
	langfuseHandler := handler.NewLangfuseHandler(langfuseService)
	chatSessionHandler := handler.NewChatSessionHandler(chatSessionService, userService)
	openaiChatHandler := handler.NewOpenAIChatHandler(openaiChatService)
	connectorHandler := handler.NewConnectorHandler(connectorService, userService)
	searchHandler := handler.NewSearchHandler(searchService, userService)
	fileHandler := handler.NewFileHandler(fileService, userService)
	memoryHandler := handler.NewMemoryHandler(memoryService)
	mcpHandler := handler.NewMCPHandler(mcpService)

	// MCP server endpoint — exposes RAGFlow capabilities as MCP tools
	// (ragflow_retrieval, ragflow_list_datasets, ragflow_list_chats) to
	// external AI clients via JSON-RPC over HTTP.
	mcpServerHandler := handler.NewMCPServerHandler(
		func(ctx context.Context, userID string, page, pageSize int, orderBy string, desc bool) ([]map[string]interface{}, int64, error) {
			return handler.MCPListDatasets(ctx, datasetsService, userID, page, pageSize, orderBy, desc)
		},
		func(ctx context.Context, userID string, page, pageSize int, orderBy string, desc bool) ([]map[string]interface{}, int64, error) {
			return handler.MCPListChats(ctx, chatService, userID, page, pageSize, orderBy, desc)
		},
		func(ctx context.Context, userID string, req mcp.RetrievalRequest) (string, error) {
			return handler.MCPRetrieval(ctx, datasetsService, userID, req)
		},
	)
	skillSearchHandler := handler.NewSkillSearchHandler(docEngine, documentService)
	providerHandler := handler.NewProviderHandler(userService, modelProviderService)
	// Install the agent service's Redis-backed run infrastructure
	// (CheckPointStore / StateSerializer / RunTracker). When Redis
	// is unreachable (degraded boot, stand-alone mode, no-redis CI)
	// the constructors return errors, and we fall through to the
	// in-memory / no-tracking path: the agent service treats nil
	// options as the in-memory test path, so graceful degradation
	// is a 1-line if-not-nil pass-through — no separate "boot" mode
	// required.
	agentOpts := buildAgentRunOptions()
	agentService := service.NewAgentServiceWithOptions(
		agentOpts.checkpointStore,
		agentOpts.stateSerializer,
		agentOpts.runTracker,
	)
	agentHandler := handler.NewAgentHandler(ctx, agentService, fileService)

	// Public chatbot/agentbot endpoints (api/v1/chatbots/...,
	// api/v1/agentbots/...) and the agent attachment download.
	// BotService delegates the agentBot completion to agentService so
	// both paths share the same canvas runner. Reuse the llmService
	// already constructed above (line 222) — do NOT redeclare with
	// `:=` since the variable is in scope.
	botService := service.NewBotService(agentService, llmService)
	botHandler := handler.NewBotHandler(botService)

	// Wire the TTS synthesizer to the per-tenant model-provider
	// dispatch. SynthesizeRequest is routed through
	// ModelProviderService.AudioSpeech, which fans out to the
	// tenant's configured TTS model driver. When the model
	// provider is unconfigured, the synthesizer falls back to a
	// no-op echo (the audio package contract), so this is always
	// safe to call.
	configureTTSSynthesizer(modelProviderService)
	searchBotHandler := handler.NewSearchBotHandler(
		searchService,
		tenantService,
		modelProviderService,
		chunkService,
	)
	searchBotHandler.SetStreamLLM(modelProviderService)
	askService := service.NewAskService(chunkService, nil, 0, 0)
	searchBotHandler.SetAskService(askService)
	chatHandler.SetMindMapDependencies(searchService, tenantService, modelProviderService, chunkService)
	searchHandler.SetCompletionDependencies(modelProviderService, askService)
	pluginHandler := handler.NewPluginHandler(service.NewPluginService())
	modelHandler := handler.NewModelHandler(service.NewModelProviderService())
	fileCommitHandler := handler.NewFileCommitHandler(file.NewFileCommitService())

	// Dify retrieval handler
	retrievalService := nlp.NewRetrievalService(docEngine, documentDAO)
	difyRetrievalHandler := handler.NewDifyRetrievalHandler(
		datasetsService,
		modelProviderService,
		metadataService,
		retrievalService,
		documentDAO,
		docEngine,
	)
	componentsSvc := service.NewComponentsService()
	componentsHandler := handler.NewComponentsHandler(componentsSvc)
	pipelineHandler := handler.NewPipelineHandler()
	compilationTemplateHandler := handler.NewCompilationTemplateHandler(service.NewCompilationTemplateService())
	compilationTemplateGroupHandler := handler.NewCompilationTemplateGroupHandler(service.NewCompilationTemplateGroupService())
	datasetArtifactHandler := handler.NewDatasetArtifactHandler(service.NewDatasetArtifactService(), datasetsService, file.NewFileCommitService())

	// Install the production eino-based chat invoker as the shared chat default,
	// so agentic-search harness LLM calls work in production. Without this,
	// chat.GetDefaultInvoker() stays nil and the harness falls back gracefully.
	component.InstallDefaultChatInvoker()

	// Install the dataset-nav ES-backed service (internal/service/nav +
	// internal/service/nlp). The embedder resolves the tenant's embedding model
	// on demand so Search/UpsertDoc can embed queries/summaries automatically.
	nav.SetNavService(nlp.NewNavService(service.NewNavEmbedder(modelProviderService, "")))

	// Install the compiled-wiki search service. It is backed directly by the
	// document engine: QueryPages filters the tenant-scoped index to
	// compile_kwd="wiki_page" (+ supported kinds) so ordinary source chunks are
	// never relabeled as wiki pages, and BackfillChunks fetches original chunks
	// by id. When the engine is unavailable the service degrades to empty so the
	// agent falls back to hybrid search (no failing call).
	wikisearch.SetService(wikisearch.NewEngineService(engine.Get()))

	// Initialize router
	r := router.NewRouter(authHandler,
		userHandler,
		tenantHandler,
		documentHandler,
		datasetsHandler,
		systemHandler,
		statsHandler,
		chunkHandler,
		llmHandler,
		chatHandler,
		chatChannelHandler,
		langfuseHandler,
		chatSessionHandler,
		connectorHandler,
		searchHandler,
		fileHandler,
		memoryHandler,
		mcpHandler,
		mcpServerHandler,
		skillSearchHandler,
		providerHandler,
		agentHandler,
		searchBotHandler,
		difyRetrievalHandler,
		pluginHandler,
		modelHandler,
		fileCommitHandler,
		openaiChatHandler,
		botHandler,
		componentsHandler,
		pipelineHandler,
		compilationTemplateHandler,
		compilationTemplateGroupHandler,
		datasetArtifactHandler)

	// Create Gin engine
	ginEngine := gin.New()
	// Mirror Quart's merge_slashes: collapse duplicate slashes before routing.
	ginEngine.RemoveExtraSlash = true

	// Middleware
	// Note: common.GinLogger() is registered inside router.Setup so the
	// HTTP request log captures every endpoint the router owns (including
	// those registered by Setup itself). Registering it here would run
	// it twice for those endpoints and double every access-log line.
	ginEngine.Use(gin.Recovery())

	// Setup routes
	r.Setup(ginEngine)

	_, err := channels.Start(ctx)
	if err != nil {
		common.Fatal("Fail to start chat-channel", zap.Error(err))
	}

	apiServerConfig := globalConfig.GetAPIServerConfig()

	// Create HTTP server with timeouts to prevent slow clients from blocking shutdown
	addr := fmt.Sprintf(":%d", apiServerConfig.HTTPPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           ginEngine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		common.Info(
			"\n        ____   ___    ______ ______ __\n" +
				"       / __ \\ /   |  / ____// ____// /____  _      __\n" +
				"      / /_/ // /| | / / __ / /_   / // __ \\| | /| / /\n" +
				"     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /\n" +
				"    /_/ |_|/_/  |_|\\____//_/    /_/ \\____/ |__/|__/\n",
		)
		common.Info(fmt.Sprintf("RAGFlow Go Version: %s", common.GetRAGFlowVersion()))
		common.Info(fmt.Sprintf("Server starting on port: %d", apiServerConfig.HTTPPort))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			common.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Start heartbeat reporter to admin server
	if hb := startHeartbeat(
		common.ServerTypeAPI,
		fmt.Sprintf("ragflow-server-%d", apiServerConfig.HTTPPort),
		apiServerConfig.HTTPPort,
		globalConfig.GetHeartbeatInterval(),
	); hb != nil {
		defer hb.Stop()
	}

	// Wait for shutdown signal from main's signal.NotifyContext
	<-ctx.Done()

	common.Info(fmt.Sprintf("Received shutdown signal"))
	common.Info("Shutting down server...")

	// Create context with timeout for graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		common.Fatal("Server forced to shutdown", zap.Error(err))
	}
}

// agentRunOptions bundles the three optional injection slots the
// agent service accepts via NewAgentServiceWithOptions: the Redis-
// backed CheckPointStore, StateSerializer, and RunTracker. The
// fields stay nil when the underlying constructors fail (Redis
// unreachable, etc.); the agent service treats nil as "in-memory
// / no-tracking" so the server continues to serve traffic without
// requiring Redis to be up.
type agentRunOptions struct {
	checkpointStore canvas.CheckPointStore
	stateSerializer canvas.StateSerializer
	runTracker      *canvas.RunTracker
}

// buildAgentRunOptions installs the Redis-backed run infrastructure
// when Redis is available. The Redis client is the one already
// initialized at the top of main; the TTL is a conservative 24h for
// both the checkpoint store and the run tracker. On any error
// (Redis down at boot, constructor panic, nil-Redis fallback) we
// log and return a zero-value struct — the agent service falls back
// to the in-memory path transparently.
func buildAgentRunOptions() agentRunOptions {
	var out agentRunOptions
	if !redis.IsEnabled() || redis.Get() == nil {
		common.Info("agent: redis client not initialised; agent run infra in in-memory mode (no checkpoints, no run tracker)")
		return out
	}
	cp := canvas.NewRedisCheckPointStore(24 * time.Hour)
	out.checkpointStore = cp
	// stateSerializer is intentionally left nil. eino's default
	// InternalSerializer (used when no compose.WithSerializer is
	// passed at compile time) already knows how to round-trip
	// runtime.CanvasState because the runtime package registers
	// it via compose.RegisterSerializableType[CanvasState] in
	// init(). Overriding with RAGFlow's plain-JSON
	// CanvasStateSerializer (json.Marshal/Unmarshal) produces
	// bytes the InternalSerializer cannot decode on the resume
	// pass — the UserFillUp two-node pattern surfaces this as
	// "load checkpoint from store fail: cannot unmarshal object
	// into Go struct field checkpoint.Channels of type
	// compose.channel". Rely on eino's default instead.
	rt := canvas.NewRunTracker(24 * time.Hour)
	out.runTracker = rt
	common.Info("agent: redis-backed run infra installed (24h TTL on checkpoint store + run tracker; eino default serializer)")
	return out
}

// configureTTSSynthesizer installs the audio.ModelProviderFunc
// that dispatches Synthesize requests through the project's
// ModelProviderService. The model provider's AudioSpeech method
// (internal/service/model_service.go) resolves the per-tenant TTS
// model driver, sends the request upstream, and returns
// synthesized audio bytes.
//
// The audio package's NewTTSDispatchFunc helper converts the
// audio.SynthesizeRequest shape into the model's dispatch shape
// (audioContent = req.Text, voice/lang → TTSConfig.Params,
// ModelName from req.Engine). When the model provider is
// unconfigured (nil dispatcher) the helper returns nil, which
// reverts the audio package to its default stub.
func configureTTSSynthesizer(modelProviderService *service.ModelProviderService) {
	if modelProviderService == nil {
		common.Info("agent: model provider service not initialised; TTS in no-op echo mode")
		audio.SetModelProviderSynthesizer(nil)
		return
	}
	audio.SetModelProviderSynthesizer(audio.NewTTSDispatchFunc(modelProviderService))
	common.Info("agent: TTS model-provider dispatch installed (audio.Synthesize → ModelProviderService.AudioSpeech)")
}

// registerNativeDeepDoc wires the in-process (Go) DeepDoc backend as the local
// inference backend. The server is built with -tags cgo and links ONNX Runtime
// statically (libonnxruntime.a, resolved at runtime via dlopen(NULL) from the
// running binary — see the onnxruntime_go fork), so there is no external
// DeepDoc HTTP service and no dynamic .so deployment.
//
// Fail-fast contract (P0): the in-process backend must be available at startup
// (ORT + models present). There is NO silent degradation to an empty analyzer:
// if the backend is not serving, the server aborts.
func registerNativeDeepDoc() {
	modelDir := resolveDeepDocModelDir()
	dropScore := resolveDeepDocDropScore()

	if err := infnative.Register(modelDir, dropScore); err != nil {
		common.Warn("in-process DeepDoc backend unavailable",
			zap.String("reason", err.Error()))
	}

	// The in-process (Go) DeepDoc backend is the ONLY production backend. Fail
	// fast rather than silently parsing without layout/table/OCR if the local
	// backend cannot serve (ORT + models must be present when built with -tags
	// cgo).
	if !infnative.Serving() {
		common.Fatal("no in-process DeepDoc backend serving: provide the local ORT "+
			"runtime + models and build with -tags cgo",
			zap.String("model_dir", modelDir),
			zap.String("ort_lib", "static (libonnxruntime.a via dlopen(NULL))"))
	}
	common.Info("in-process DeepDoc backend registered (production backend)",
		zap.String("model_dir", modelDir))
}

// resolveDeepDocModelDir picks the model directory: the explicit DEEPDOC_MODEL_DIR
// env, else the RAGFlow default (rag/res/deepdoc, mirroring deepdoc_server.py),
// else the snapshot fetched by ragflow_deps/download_deps.py. The first
// candidate that actually contains the required weights wins.
func resolveDeepDocModelDir() string {
	if v := strings.TrimSpace(common.GetEnv(common.EnvDeepDocModelDir)); v != "" {
		return v
	}
	wd, _ := os.Getwd()
	candidates := []string{
		filepath.Join(wd, "rag", "res", "deepdoc"),
		filepath.Join(wd, "huggingface.co", "InfiniFlow", "deepdoc"),
	}
	for _, c := range candidates {
		if dirHasModels(c) {
			return c
		}
	}
	// None verified; return the canonical default so any error message points
	// at the conventional location.
	return filepath.Join(wd, "rag", "res", "deepdoc")
}

// resolveDeepDocDropScore returns the explicit DEEPDOC_DROP_SCORE env, else the
// in-process backend's default (infnative.DefaultDropScore, which mirrors
// the Python inference service's Recognizer.drop_score).
func resolveDeepDocDropScore() float64 {
	if v := strings.TrimSpace(common.GetEnv(common.EnvDeepDocDropScore)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		common.Warn("invalid DEEPDOC_DROP_SCORE, using default",
			zap.String("value", v), zap.Float64("default", infnative.DefaultDropScore))
	}
	return infnative.DefaultDropScore
}

// dirHasModels reports whether dir contains every required model file.
func dirHasModels(dir string) bool {
	return common.HasModelFiles(dir)
}
