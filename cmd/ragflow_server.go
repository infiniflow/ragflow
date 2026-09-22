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
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"ragflow/internal/admin"
	"ragflow/internal/agent/audio"
	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/retrievalbridge"
	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/channels"
	native "ragflow/internal/deepdoc/native"
	pdf "ragflow/internal/deepdoc/parser/pdf"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/handler"
	"ragflow/internal/ingestion/knowledge_compile"
	ingestion "ragflow/internal/ingestion/service"
	"ragflow/internal/mcp"
	"ragflow/internal/rag/agentic-rag"
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
	goruntime "runtime"
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
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/entity"
	_ "ragflow/internal/ingestion/wire"
	"ragflow/internal/server"
	"ragflow/internal/utility"
)

type serverArgs struct {
	mode          *string // admin | api | ingestor | syncer | deepdoc
	helpFlag      bool
	versionFlag   bool
	logLevel      *string
	migrateDB     bool
	configPath    *string // Used by admin, api; user defined config path
	initSuperUser bool    // Used by admin;
	port          *int    // Used by admin, api
	adminHost     *string // Used by api, ingestor, syncer, deepdoc for heartbeat
	adminPort     *int    // Used by api, ingestor, syncer, deepdoc for heartbeat, "ip:port"
	name          *string // server name
	enablePProf   bool    // enable pprof
	mcpEnabled    bool
	mcpHost       string
	mcpPort       int
	mcpMode       string
	mcpAPIKey     string
	mcpSSE        bool
	mcpStreamable bool
	mcpJSON       bool
}

func parseArgs() (*serverArgs, error) {
	args := &serverArgs{
		mcpHost:       "127.0.0.1",
		mcpPort:       9382,
		mcpMode:       "self-host",
		mcpSSE:        true,
		mcpStreamable: true,
		mcpJSON:       true,
	}

	var serverMode string
	var configPath string
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if key, value, ok := strings.Cut(arg, "="); ok {
			switch key {
			case "--log-level":
				if err := validateLogLevel(value); err != nil {
					return nil, err
				}
				args.logLevel = &value
				continue
			case "--mcp-host":
				args.mcpHost = value
				continue
			case "--mcp-port":
				port, err := parsePort(value, "MCP")
				if err != nil {
					return nil, err
				}
				args.mcpPort = port
				continue
			case "--mcp-mode":
				args.mcpMode = value
				continue
			case "--mcp-host-api-key":
				args.mcpAPIKey = value
				continue
			}
		}
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
		case "--enable-mcpserver":
			args.mcpEnabled = true
		case "--transport-sse-enabled":
			args.mcpSSE = true
		case "--no-transport-sse-enabled":
			args.mcpSSE = false
		case "--transport-streamable-http-enabled":
			args.mcpStreamable = true
		case "--no-transport-streamable-http-enabled":
			args.mcpStreamable = false
		case "--json-response":
			args.mcpJSON = true
		case "--no-json-response":
			args.mcpJSON = false
		case "--syncer":
			serverMode = "syncer"
			args.mode = &serverMode
		case "--deepdoc":
			serverMode = "deepdoc"
			args.mode = &serverMode
		case "-h", "--help":
			args.helpFlag = true
		case "-v", "--version":
			args.versionFlag = true
		case "--log-level":
			if i+1 >= len(os.Args) {
				return nil, errors.New("--log-level requires a value")
			}
			i++
			level := os.Args[i]
			if err := validateLogLevel(level); err != nil {
				return nil, err
			}
			args.logLevel = &level
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
		case "--profile":
			args.enablePProf = true
		default:
			return nil, fmt.Errorf("unknown parameter: %s", arg)
		}
	}

	if err := applyMCPEnv(args); err != nil {
		return nil, err
	}
	if err := validateMCPArgs(args); err != nil {
		return nil, err
	}
	if args.migrateDB && args.mode != nil {
		return nil, errors.New("--migrate cannot be combined with a server mode")
	}
	return args, nil
}

func validateLogLevel(level string) error {
	switch level {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("invalid --log-level %q: must be debug, info, warn, or error", level)
	}
}

func selectedLogLevel(args *serverArgs, configured string) string {
	level := configured
	if level == "" {
		level = "warn"
	}
	if args.logLevel != nil {
		level = *args.logLevel
	}
	return level
}

func applyMCPEnv(args *serverArgs) error {
	if value, ok := os.LookupEnv("RAGFLOW_MCP_HOST"); ok {
		args.mcpHost = value
	}
	if value, ok := os.LookupEnv("RAGFLOW_MCP_PORT"); ok {
		port, err := parsePort(value, "MCP")
		if err != nil {
			return err
		}
		args.mcpPort = port
	}
	if value, ok := os.LookupEnv("RAGFLOW_MCP_LAUNCH_MODE"); ok {
		args.mcpMode = value
	}
	if value, ok := os.LookupEnv("RAGFLOW_MCP_HOST_API_KEY"); ok {
		args.mcpAPIKey = value
	}
	if value, ok := os.LookupEnv("RAGFLOW_MCP_ENABLED"); ok {
		args.mcpEnabled = parseMCPBool(value)
	}
	if value, ok := os.LookupEnv("RAGFLOW_MCP_TRANSPORT_SSE_ENABLED"); ok {
		args.mcpSSE = parseMCPBool(value)
	}
	if value, ok := os.LookupEnv("RAGFLOW_MCP_TRANSPORT_STREAMABLE_ENABLED"); ok {
		args.mcpStreamable = parseMCPBool(value)
	}
	if value, ok := os.LookupEnv("RAGFLOW_MCP_JSON_RESPONSE"); ok {
		args.mcpJSON = parseMCPBool(value)
	}
	return nil
}

func validateMCPArgs(args *serverArgs) error {
	if args.mcpMode != "self-host" && args.mcpMode != "host" {
		return fmt.Errorf("invalid MCP mode: %s", args.mcpMode)
	}
	if !args.mcpStreamable && args.mcpJSON {
		args.mcpJSON = false
	}
	if !args.mcpSSE && !args.mcpStreamable {
		args.mcpStreamable = true
	}
	if args.mcpEnabled && args.mcpMode == "self-host" && args.mcpAPIKey == "" {
		return errors.New("--mcp-host-api-key is required when --mcp-mode=self-host")
	}
	return nil
}

func parseMCPBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parsePort(value, name string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid %s port: %s", name, value)
	}
	return port, nil
}

// registerNativeDeepDoc wires the in-process (Go) DeepDoc backend as the local
// fallback used when no external DeepDoc HTTP service is configured. It is
// compiled into the server built with -tags cgo, which statically links the
// ONNX Runtime backend (libonnxruntime.a); the unit-test tier builds without
// cgo and stays free of the onnxruntime dependency.
func printHelp(args *serverArgs) {
	switch {
	case args.mode == nil:
		fmt.Fprintf(os.Stderr, "Usage: %s --api|--admin|--ingestor|--syncer|--deepdoc [OPTIONS]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s --migrate [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Server - Open-source RAG engine based on deep document understanding\n\n")
		fmt.Fprintf(os.Stderr, "Mode selection (default: --api):\n")
		fmt.Fprintf(os.Stderr, "  --api          \tRun as API server\n")
		fmt.Fprintf(os.Stderr, "  --admin        \tRun as admin server\n")
		fmt.Fprintf(os.Stderr, "  --ingestor     \tRun as ingestion worker\n")
		fmt.Fprintf(os.Stderr, "  --syncer       \tRun as file sync service\n")
		fmt.Fprintf(os.Stderr, "  --deepdoc      \tRun as DeepDoc server\n\n")
		fmt.Fprintf(os.Stderr, "Standalone action (mutually exclusive with a mode):\n")
		fmt.Fprintf(os.Stderr, "  --migrate      \tRun database migrations and exit\n\n")
		fmt.Fprintf(os.Stderr, "Common options:\n")
		fmt.Fprintf(os.Stderr, "  -f, --config string\tPath to configuration file\n")
		fmt.Fprintf(os.Stderr, "  -p, --port int \tServer port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  --admin-host string\tAdmin server host:port (ingestor, syncer, deepdoc)\n")
		fmt.Fprintf(os.Stderr, "  --name string  \tServer name (ingestor, syncer, deepdoc)\n")
		fmt.Fprintf(os.Stderr, "  --init-superuser\tInitialize superuser account (admin)\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --log-level string\tLog level: debug, info, warn, error (default: warn)\n")
		fmt.Fprintf(os.Stderr, "  --profile      \tEnable pprof server\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \tShow this help message and exit\n\n")
		fmt.Fprintf(os.Stderr, "API MCP options:\n")
		fmt.Fprintf(os.Stderr, "  --enable-mcpserver\tEnable the MCP server\n")
		fmt.Fprintf(os.Stderr, "  --mcp-host=string\tMCP bind address (default: 127.0.0.1)\n")
		fmt.Fprintf(os.Stderr, "  --mcp-port=int \tMCP port (default: 9382)\n")
		fmt.Fprintf(os.Stderr, "  --mcp-mode=self-host|host\tMCP mode (default: self-host)\n")
		fmt.Fprintf(os.Stderr, "  --mcp-host-api-key=string\tAPI key required in self-host mode\n")
		fmt.Fprintf(os.Stderr, "  --transport-sse-enabled, --no-transport-sse-enabled\tEnable or disable MCP SSE transport (default: enabled)\n")
		fmt.Fprintf(os.Stderr, "  --transport-streamable-http-enabled, --no-transport-streamable-http-enabled\tEnable or disable MCP streamable HTTP transport (default: enabled)\n")
		fmt.Fprintf(os.Stderr, "  --json-response, --no-json-response\tEnable or disable MCP JSON responses (default: enabled)\n\n")
		fmt.Fprintf(os.Stderr, "Run '%s --api --help' for API server options.\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Run '%s --admin --help' for admin server options.\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Run '%s --ingestor --help' for ingester options.\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Run '%s --syncer --help' for syncer options.\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Run '%s --deepdoc --help' for DeepDoc server options.\n", os.Args[0])
	case *args.mode == "api":
		fmt.Fprintf(os.Stderr, "Usage: %s --api [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow API Server\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  --port int     	\tServer port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\tPath to configuration file\n")
		fmt.Fprintf(os.Stderr, "  -v, --version 	 \tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --log-level string\tLog level: debug, info, warn, error (default: warn)\n")
		fmt.Fprintf(os.Stderr, "  --profile          \t\tEnable pprof server\n")
		fmt.Fprintf(os.Stderr, "  -h, --help       	  \tShow this help message and exit\n")
		fmt.Fprintf(os.Stderr, "\nMCP options:\n")
		fmt.Fprintf(os.Stderr, "  --enable-mcpserver\tEnable the MCP server\n")
		fmt.Fprintf(os.Stderr, "  --mcp-host=string\tMCP bind address (default: 127.0.0.1)\n")
		fmt.Fprintf(os.Stderr, "  --mcp-port=int \tMCP port (default: 9382)\n")
		fmt.Fprintf(os.Stderr, "  --mcp-mode=self-host|host\tMCP mode (default: self-host)\n")
		fmt.Fprintf(os.Stderr, "  --mcp-host-api-key=string\tAPI key required in self-host mode\n")
		fmt.Fprintf(os.Stderr, "  --transport-sse-enabled, --no-transport-sse-enabled\tEnable or disable MCP SSE transport (default: enabled)\n")
		fmt.Fprintf(os.Stderr, "  --transport-streamable-http-enabled, --no-transport-streamable-http-enabled\tEnable or disable MCP streamable HTTP transport (default: enabled)\n")
		fmt.Fprintf(os.Stderr, "  --json-response, --no-json-response\tEnable or disable MCP JSON responses (default: enabled)\n")
	case *args.mode == "admin":
		fmt.Fprintf(os.Stderr, "Usage: %s --admin [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Admin Server\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\t\tPath to configuration file\n")
		fmt.Fprintf(os.Stderr, "  --port int    \t\t\tServer port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  --init-superuser\t\t\tInitialize superuser account\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \t\t\tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --log-level string\t\tLog level: debug, info, warn, error (default: warn)\n")
		fmt.Fprintf(os.Stderr, "  --profile      \t\t\tEnable pprof server\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \t\t\tShow this help message and exit\n")
	case *args.mode == "ingestor":
		fmt.Fprintf(os.Stderr, "Usage: %s --ingestor [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Ingestion Worker - Document ingestion processing\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\tPath to config file\n")
		fmt.Fprintf(os.Stderr, "  --name string\t\t\tIngestion server name (default: \"default_ingestion\")\n")
		fmt.Fprintf(os.Stderr, "  --admin-host string\tAdmin server host:port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \t\tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --log-level string\tLog level: debug, info, warn, error (default: warn)\n")
		fmt.Fprintf(os.Stderr, "  --profile      \t\tEnable pprof server\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \t\tShow this help message and exit\n")
	case *args.mode == "syncer":
		fmt.Fprintf(os.Stderr, "Usage: %s --syncer [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow Sync Service - Sync files from source to RAGFlow\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\tPath to config file\n")
		fmt.Fprintf(os.Stderr, "  --name string\t\t\tSync service server name (default: \"default_syncer\")\n")
		fmt.Fprintf(os.Stderr, "  --admin-host string\tAdmin server host:port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \t\tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --log-level string\tLog level: debug, info, warn, error (default: warn)\n")
		fmt.Fprintf(os.Stderr, "  --profile      \t\tEnable pprof server\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \t\tShow this help message and exit\n")
	case *args.mode == "deepdoc":
		fmt.Fprintf(os.Stderr, "Usage: %s --deepdoc [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "RAGFlow DeepDoc Inference Service - DeepDoc model inference service\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -f --config string\tPath to config file\n")
		fmt.Fprintf(os.Stderr, "  --name string\t\t\tSync service server name (default: \"default_syncer\")\n")
		fmt.Fprintf(os.Stderr, "  --admin-host string\tAdmin server host:port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \t\tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --debug        \t\tEnable debug-level logging\n")
		fmt.Fprintf(os.Stderr, "  --profile      \t\tEnable pprof server\n")
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

	if arguments.helpFlag || (arguments.mode == nil && !arguments.migrateDB) {
		printHelp(arguments)
		os.Exit(1)
	}

	if arguments.versionFlag {
		fmt.Printf("RAGFlow version: %s\n", common.GetRAGFlowVersion())
		os.Exit(1)
	}

	if arguments.migrateDB {
		if err = runMigrate(ctx, arguments); err != nil {
			common.Fatal("Failed to run database migration", zap.Error(err))
		}
		return
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

	logLevel := selectedLogLevel(arguments, "")

	// Temporary pre-config logger: STDOUT ONLY (empty FileOutput). The port
	// is not known yet, so a file here would be an orphaned log (e.g.
	// logs/api_server.log next to the real logs/api_server_9384.log); the
	// real file sink is attached by the post-config re-initialization below.
	if err = common.InitLogger(logLevel, common.FileOutput{}, serverName); err != nil {
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

	globalConfig := server.GetConfig()

	// override default port if provided
	// NOTE: this switch must stay side-effect-free on the LOG (no
	// registerNativeDeepDoc here): it runs while only the temporary
	// stdout-only logger exists, so anything it logs is lost from the file.
	// Side effects that log (DeepDoc registration) move below, after the
	// real file-backed logger is up.
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
	case "deepdoc":
		if serverName == "" {
			uuid := utility.GenerateUUID()
			serverName = fmt.Sprintf("deepdoc_server_%s", uuid)
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

	// Reinitialize logger with the configured level and CLI overrides.
	logLevel = selectedLogLevel(arguments, logConfig.Level)

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

	// Wire the in-process DeepDoc backend only after the REAL file-backed
	// logger exists: its registration lines (and the Fatal abort on a missing
	// backend) must land in the run's log file, not in the pre-config
	// stdout-only window.
	switch *arguments.mode {
	case "api", "ingestor":
		registerNativeDeepDoc()
	default:
	}

	// Print all configuration settings
	common.Info(fmt.Sprintf("Starting %s server: %s, mode: %s", *arguments.mode, serverName, globalConfig.GetMode()))
	server.PrintAll()

	// Start pprof server if requested
	if arguments.enablePProf {
		go func() {
			common.Info("Starting pprof server", zap.String("addr", "localhost:6060"))
			if pprofErr := http.ListenAndServe("localhost:6060", nil); pprofErr != nil {
				common.Error("pprof server failed", pprofErr)
			}
		}()
	}

	// Initialize database
	if err = dao.InitDB(ctx, false); err != nil {
		common.Fatal("Failed to initialize database", zap.Error(err))
	}

	if err = checkDatabaseVersion(ctx); err != nil {
		common.Fatal("Refusing to start: database was migrated by a newer version", zap.Error(err))
	}

	// Initialize doc engine
	if err = engine.InitDocEngine(ctx); err != nil {
		common.Fatal("Failed to initialize doc engine", zap.Error(err))
	}
	defer engine.Close()

	// Initialize Kvrocks cache
	if err = kvrocks.Init(ctx); err != nil {
		common.Fatal("Failed to initialize Kvrocks", zap.Error(err))
	}
	defer kvrocks.Close()

	if err = storage.Init(ctx); err != nil {
		common.Error("Failed to initialize storage factory", err)
	}
	defer storage.CloseStorage()

	if err = engine.InitMessageQueue(); err != nil {
		common.Fatal("Failed to initialize message queue engine", zap.Error(err))
	}

	// Initialize server variables (runtime variables that can change during operation)
	// This must be done after Cache is initialized
	if err = server.InitVariables(kvrocks.Get()); err != nil {
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
	case "deepdoc":
		if err = runDeepDoc(ctx, arguments); err != nil {
			fmt.Printf("Failed to start DEEPDOC: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Invalid server mode: %s\n", *arguments.mode)
		os.Exit(1)
	}
}

// checkDatabaseVersion refuses to run a server when the running code is older
// than the version recorded in the system_settings migration marker. Migrating
// the database forward is a one-way operation, so an older binary would read and
// write a schema it does not understand.
//
// A missing marker, or a version on either side that cannot be parsed, never
// blocks startup: without a usable comparison there is no evidence that the
// database is ahead of the code.
//
// RAGFLOW_DEV_MODE turns the check off entirely. A development build can carry
// a marker for a release that is not tagged yet, in which case the comparison
// would reject the build that wrote the marker.
func checkDatabaseVersion(ctx context.Context) error {
	if common.DevModeEnabled() {
		common.Warn("Development mode is enabled, skipping the database downgrade check",
			zap.String("env", common.EnvRAGFlowDevMode))
		return nil
	}

	databaseVersion, err := dao.GetDatabaseMigrationVersion(ctx, dao.DB)
	if err != nil {
		return fmt.Errorf("read database version marker: %w", err)
	}
	if databaseVersion == "" {
		return nil
	}

	codeVersion := common.GetRAGFlowVersion()
	older, comparable := common.IsOlderReleaseThan(codeVersion, databaseVersion)
	if !comparable {
		common.Warn("Cannot compare code version with database version, skipping the downgrade check",
			zap.String("code_version", codeVersion),
			zap.String("database_version", databaseVersion))
		return nil
	}
	if older {
		return fmt.Errorf("code version %s is older than database version %s: upgrade this deployment to %s or newer before starting",
			codeVersion, databaseVersion, databaseVersion)
	}

	common.Info("Database version check passed",
		zap.String("code_version", codeVersion),
		zap.String("database_version", databaseVersion))
	return nil
}

// runMigrate runs the database schema and data migrations and returns. It is
// the whole of the standalone --migrate action: load the configuration, run
// dao.InitDB with migrations enabled, then exit. It deliberately does not call
// registerNativeDeepDoc or initialize the doc engine, Redis, storage or the
// message queue, so it can run on its own, before any server mode boots (see
// docker/entrypoint-go.sh and docker/launch_backend_service.sh).
func runMigrate(ctx context.Context, args *serverArgs) error {
	const serverName = "migrate"

	if err := server.InitLocalVariables(); err != nil {
		return fmt.Errorf("initialize local variables: %w", err)
	}

	logLevel := selectedLogLevel(args, "")
	if err := common.InitLogger(logLevel, common.FileOutput{Filename: serverName + ".log", Path: "logs"}, serverName); err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}

	var configPath string
	if args.configPath != nil {
		configPath = *args.configPath
	}
	if err := server.Init(configPath); err != nil {
		return fmt.Errorf("initialize configuration: %w", err)
	}

	globalConfig := server.GetConfig()
	server.SetServerName(serverName)
	logConfig := globalConfig.GetLogConfig()
	logLevel = selectedLogLevel(args, logConfig.Level)
	globalConfig.SetLogLevel(logLevel)

	common.SyncLog()
	if err := common.InitLogger(logLevel, common.FileOutput{
		Filename:   serverName + ".log",
		Path:       logConfig.Path,
		MaxSize:    logConfig.MaxSize,
		MaxBackups: logConfig.MaxBackups,
		MaxAge:     logConfig.MaxAge,
		Compress:   logConfig.Compress,
	}, serverName); err != nil {
		common.Error("Failed to reinitialize logger with configured level", err)
	}

	common.Info("Running database migrations")
	if err := dao.InitDB(ctx, true); err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	common.Info("Database migrations completed")
	return nil
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
	// Only honour X-Forwarded-For / X-Real-IP from the configured proxies
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

	// Fail fast if the cl100k_base BPE table is missing. NumTokensFromString /
	// TrimContentToTokenLimit now panic rather than degrading silently, so this
	// trades a mid-request panic for a clear startup failure.
	if err := tokenizer.InitCL100KEncoder(); err != nil {
		common.Fatal("Failed to initialize cl100k_base tokenizer", zap.Error(err))
	}
	logTokenizerCounters()

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
	// pool (task_type="memory" dispatched by handleAndExecute -> executeMemoryTask),
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
		0,
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
		0,
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

	// Fail fast if the cl100k_base BPE table is missing. NumTokensFromString /
	// TrimContentToTokenLimit now panic rather than degrading silently, so this
	// trades a mid-request panic for a clear startup failure.
	if err := tokenizer.InitCL100KEncoder(); err != nil {
		common.Fatal("Failed to initialize cl100k_base tokenizer", zap.Error(err))
	}

	// Initialize global QueryBuilder using tokenizer's DictPath
	// This ensures the Synonym uses the same wordnet directory as tokenizer
	if err := nlp.InitQueryBuilderFromTokenizer(tokenizerCfg.DictPath); err != nil {
		common.Fatal("Failed to initialize query builder", zap.Error(err))
	}

	if err := startServer(ctx, args); err != nil {
		return err
	}

	common.Info("Server exited")

	return nil
}

func startServer(ctx context.Context, args *serverArgs) error {

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
	modelSolver := service.NewModelSolver()

	// Wire the real MemorySaver so the Message component can persist
	// conversation turns to memory stores declared in the canvas DSL.
	component.SetMemorySaver(service.NewMemorySaverAdapter(memoryService))

	// Initialize doc engine for skill search
	docEngine := engine.Get()
	documentDAO := dao.NewDocumentDAO()
	retrievalEnhancer := retrievalbridge.NewEnhancer(docEngine, metadataService)
	// Keep the concrete adapter: it is both the canvas/agent-tool backend and the
	// target of the agentic-RAG bridge wired below. The bridge must hold it
	// directly (not look it up in the registry) because the registry is shared —
	// agenttool.SetRetrievalService and runtime.SetRetrievalService write the same
	// singleton, so a bridge that resolved its target per call would find itself.
	retrievalAdapter := agenttool.NewNLPRetrievalAdapterFromDeps(
		docEngine,
		documentDAO,
		nil,
		retrievalEnhancer,
	)
	retrievalAdapter.SetModelConfigResolver(func(ctx context.Context, tenantID string, modelType entity.ModelType, modelRef string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error) {
		var target *service.ModelTarget
		var err error
		if strings.TrimSpace(modelRef) == "" {
			target, err = modelSolver.ResolveDefaultModelConfig(ctx, tenantID, modelType)
		} else {
			target, err = modelSolver.ResolveModelConfig(ctx, tenantID, modelType, modelRef)
		}
		if err != nil {
			return nil, "", nil, 0, err
		}
		return target.Driver, target.ModelName, target.APIConfig, target.MaxTokens, nil
	})
	agenttool.SetRetrievalService(retrievalAdapter)
	agenttool.SetMemoryRetrievalService(retrievalbridge.NewMemoryAdapter(memoryService))
	common.Info("agent: retrieval service adapter installed")

	// Wire the agentic-RAG runtime as the Go chat pipeline's evidence engine
	// (internal/service/chat_pipeline.retrieveViaHarness): it runs each request on a
	// model resolved from the caller's ModelID and searches through the adapter
	// above. Activating the agentic loop here enables the full planner/SCA path for
	// reasoning chats.
	runtime.SetRetrievalService(retrievalbridge.NewRuntimeAdapter(retrievalAdapter))
	agentic_rag.SetAgenticLoop(agentic_rag.NewAgenticLoop())
	service.SetHarnessRetriever(retrievalbridge.NewHarnessRetriever(modelProviderService, metadataService, docEngine))
	common.Info("agent: runtime chat retriever wired (runtime retrieval + agentic loop)")

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
	// Install the agent service's Kvrocks-backed run infrastructure
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
	// so agentic-search runtime LLM calls work in production. Without this,
	// chat.GetDefaultInvoker() stays nil and the runtime falls back gracefully.
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
	// Only honour X-Forwarded-For / X-Real-IP from the configured proxies
	// (default: the loopback nginx bundled in the image), never from every
	// peer. c.ClientIP() feeds the agent webhook ip_whitelist gate and the
	// login audit records, so gin's trust-everything default would let any
	// caller pick its own address.
	if err := common.ConfigureTrustedProxies(ginEngine, globalConfig.GetAPIServerConfig().TrustedProxies); err != nil {
		common.Fatal("Failed to configure trusted proxies", zap.Error(err))
	}

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
		return fmt.Errorf("start chat-channel: %w", err)
	}

	apiServerConfig := globalConfig.GetAPIServerConfig()

	// Create HTTP server with timeouts to prevent slow clients from blocking shutdown
	addr := fmt.Sprintf(":%d", apiServerConfig.HTTPPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           ginEngine,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		// WriteTimeout spans "request header read → response written", so it is a
		// ceiling on the WHOLE request, not on a slow client's reads. Measured
		// (2026-09-15, FRAMES 20q): three multi-hop questions take 150–225s to reach
		// their response, and at 120s the server closed the connection with no
		// response at all — the client reports
		// `RemoteDisconnected('Remote end closed connection without response')` and
		// the benchmark re-runs the whole question (max_retries: 2), so one slow
		// question cost three full pipelines. 180s clears the measured distribution's
		// middle; questions whose composition alone runs past it still need streaming
		// or a larger budget.
		WriteTimeout: 180 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	apiListener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen API server on %s: %w", addr, err)
	}
	defer apiListener.Close()

	serveErr := make(chan error, 2)
	serve := func(name string, srv *http.Server, listener net.Listener) {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("%s server failed: %w", name, err)
		}
	}

	var mcpSrv *http.Server
	var mcpCloser interface{ Close() error }
	var mcpListener net.Listener
	if args != nil && args.mcpEnabled {
		resolveUser := func(ctx context.Context, authorization string) (string, error) {
			if args.mcpMode == "self-host" {
				authorization = args.mcpAPIKey
			}
			user, err := authHandler.ResolveMCPUser(ctx, authorization)
			if err != nil {
				return "", err
			}
			return user.ID, nil
		}
		if args.mcpMode == "self-host" {
			authCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			_, err := resolveUser(authCtx, "")
			cancel()
			if err != nil {
				return errors.New("invalid configured MCP API key")
			}
		}
		mcpHandler := handler.NewStandaloneMCPHandler(
			resolveUser,
			func(ctx context.Context, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error) {
				return handler.MCPListDatasets(ctx, datasetsService, userID, page, pageSize, orderby, desc)
			},
			func(ctx context.Context, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error) {
				return handler.MCPListChats(ctx, chatService, userID, page, pageSize, orderby, desc)
			},
			func(ctx context.Context, userID string, req mcp.RetrievalRequest) (string, error) {
				return handler.MCPRetrieval(ctx, datasetsService, userID, req)
			},
			mcp.Options{SSE: args.mcpSSE, StreamableHTTP: args.mcpStreamable, JSONResponse: args.mcpJSON},
		)
		mcpCloser = mcpHandler
		defer mcpHandler.Close()
		mcpAddr := fmt.Sprintf("%s:%d", args.mcpHost, args.mcpPort)
		mcpListener, err = net.Listen("tcp", mcpAddr)
		if err != nil {
			return fmt.Errorf("listen MCP server on %s: %w", mcpAddr, err)
		}
		defer mcpListener.Close()
		mcpSrv = &http.Server{
			Addr:              mcpAddr,
			Handler:           mcpHandler,
			BaseContext:       func(net.Listener) context.Context { return ctx },
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      0, // SSE streams outlive individual tool deadlines
			IdleTimeout:       120 * time.Second,
		}
		go func() {
			common.Info(fmt.Sprintf("MCP server starting on %s", mcpSrv.Addr))
			serve("MCP", mcpSrv, mcpListener)
		}()
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
		serve("API", srv, apiListener)
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

	// Wait for either shutdown signal or serving failure.
	var runErr error
	select {
	case <-ctx.Done():
		common.Info("Received shutdown signal")
	case err := <-serveErr:
		runErr = err
		common.Error("Server failed; shutting down", err)
	}
	common.Info("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if mcpCloser != nil {
		if err := mcpCloser.Close(); err != nil {
			common.Warn("Failed to close MCP handler", zap.Error(err))
		}
	}
	if err := shutdownHTTPServer(shutdownCtx, "API", srv); err != nil {
		return err
	}
	if mcpSrv != nil {
		if err := shutdownHTTPServer(shutdownCtx, "MCP", mcpSrv); err != nil {
			return err
		}
	}
	return runErr
}

func shutdownHTTPServer(ctx context.Context, name string, srv *http.Server) error {
	if err := srv.Shutdown(ctx); err != nil {
		_ = srv.Close()
		return fmt.Errorf("shutdown %s server: %w", name, err)
	}
	return nil
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

// buildAgentRunOptions installs the Kvrocks-backed run infrastructure
// when Redis is available. The Redis client is the one already
// initialized at the top of main; the TTL is a conservative 24h for
// both the checkpoint store and the run tracker. On any error
// (Redis down at boot, constructor panic, nil-Redis fallback) we
// log and return a zero-value struct — the agent service falls back
// to the in-memory path transparently.
func buildAgentRunOptions() agentRunOptions {
	var out agentRunOptions
	if !kvrocks.IsEnabled() || kvrocks.Get() == nil {
		common.Info("agent: redis client not initialised; agent run infra in in-memory mode (no checkpoints, no run tracker)")
		return out
	}
	cp := canvas.NewKvrocksCheckPointStore(24 * time.Hour)
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
// running binary — see github.com/infiniflow/onnxruntime_go, the org mirror of
// yalue/onnxruntime_go), so there is no external
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

	// DeepDoc sessions run single-threaded, so the process inference budget is a
	// plain concurrency cap. Register it with the native gate every inference
	// call passes through (internal/deepdoc/native/inference_limit.go); without
	// this the process would let every page worker call inference at once.
	limit := pdf.DeepDocConcurrency()
	native.SetInferenceLimit(limit)
	common.Info("in-process DeepDoc inference limit registered",
		zap.Int("max_concurrent_inference", limit),
		zap.Int("gomaxprocs", goruntime.GOMAXPROCS(0)))
}

// logTokenizerCounters reports, once at startup, which embedding tokenizers this process
// can count with. An unavailable counter is not fatal here: the process still starts, and
// untagged models keep counting with the calibrated estimate by design. But the models that
// declare it cannot be ingested - the embedder refuses to substitute the calibrated count -
// so this report is what tells an operator which asset to restore before documents fail.
func logTokenizerCounters() {
	var available, unavailable []string
	for _, status := range tokenizer.CounterStatuses() {
		if status.Available {
			available = append(available, status.ID)
			continue
		}
		unavailable = append(unavailable, status.ID)
	}
	common.Info("embedding tokenizer counters", zap.Strings("available", available))
	if len(unavailable) > 0 {
		common.Warn("embedding tokenizers unavailable; models that declare them cannot be ingested until the asset is restored",
			zap.Strings("unavailable", unavailable),
			zap.String("hint", "run `uv run ragflow_deps/download_go_deps.py`, or set "+common.EnvModelAssetsDir+" to a directory holding them"))
	}
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
	// MODEL_ASSETS_DIR first: the shared model-asset root keeps this layout too, so one
	// variable can point at the DeepDoc weights as well as the embedding tokenizers.
	candidates := append([]string(nil), common.ModelAssetCandidates("huggingface.co/InfiniFlow/deepdoc")...)
	candidates = append(candidates,
		filepath.Join(wd, "rag", "res", "deepdoc"),
		filepath.Join(wd, "huggingface.co", "InfiniFlow", "deepdoc"),
	)
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
