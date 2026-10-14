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
	"io"
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
	smartagentic "ragflow/internal/agentic_rag"
	"ragflow/internal/channels"
	"ragflow/internal/deepdoc/native"
	"ragflow/internal/deepdoc/parser/pdf"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/handler"
	"ragflow/internal/ingestion/knowledge_compile"
	ingestion "ragflow/internal/ingestion/service"
	"ragflow/internal/router"
	"ragflow/internal/server/local"
	"ragflow/internal/service"
	"ragflow/internal/service/chunk"
	"ragflow/internal/service/dataset"
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
	infnative "ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	"ragflow/internal/engine"
	"ragflow/internal/engine/kvrocks"
	_ "ragflow/internal/ingestion/wire"
	"ragflow/internal/server"
	"ragflow/internal/utility"
)

type serverArgs struct {
	mode          *string // admin | api | ingestor | syncer | deepdoc | migrate
	helpFlag      bool
	versionFlag   bool
	logLevel      *string
	configPath    *string // Used by admin, api; user defined config path
	initSuperUser bool    // Used by admin;
	port          *int    // Used by admin, api
	adminHost     *string // Used by api, ingestor, syncer, deepdoc for heartbeat
	adminPort     *int    // Used by api, ingestor, syncer, deepdoc for heartbeat, "ip:port"
	name          *string // server name
	enablePProf   bool    // enable pprof

	// deepdocInferenceConcurrency, when set, overrides the DeepDoc inference
	// concurrency from env/config. nil means "unspecified".
	deepdocInferenceConcurrency *int
	// ingestorMaxConcurrentWorkers, when set, overrides the ingestor worker
	// count (NATS consumer count K) from env/config. nil means "unspecified".
	ingestorMaxConcurrentWorkers *int
	// ingestorPageConcurrency, when set, overrides the per-document page
	// concurrency (N) from env/config. nil means "unspecified".
	ingestorPageConcurrency *int
	// deepdocInferenceCPUCores, when set, overrides the DeepDoc inference CPU-core
	// budget from env/config. nil means "unspecified" (0 means "all cores").
	deepdocInferenceCPUCores *int
}

// parseArgsFrom parses argv (argv[0] is the program name, mirroring os.Args)
// into a serverArgs using the standard library flag package. The accepted
// syntax is exactly the standard one: every flag works as "-flag value",
// "-flag=value", "--flag value" and "--flag=value"; boolean flags also accept
// "=true"/"=false". Flags absent from argv leave their serverArgs field nil,
// which callers use to distinguish "unspecified" from an explicit zero value.
func parseArgsFrom(argv []string) (*serverArgs, error) {
	fs := flag.NewFlagSet("ragflow_server", flag.ContinueOnError)
	// Suppress the standard usage/error dump: main prints its own one-line
	// error and printHelp owns the usage text.
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	args := &serverArgs{}

	var (
		logLevel           string
		configPath         string
		adminHost          string
		name               string
		port               int
		deepdocConcurrency int
		ingestorWorkers    int
		pageConcurrency    int
		cpuCores           int
		initSuperUser      bool
		enablePProf        bool
		helpFlag           bool
		versionFlag        bool
		modes              []string
	)

	fs.StringVar(&logLevel, "log-level", "", "")
	fs.StringVar(&configPath, "config", "", "")
	fs.StringVar(&configPath, "f", "", "")
	fs.StringVar(&adminHost, "admin-host", "", "")
	fs.StringVar(&name, "name", "", "")
	fs.IntVar(&port, "port", 0, "")
	fs.IntVar(&port, "p", 0, "")
	fs.IntVar(&deepdocConcurrency, "deepdoc-inference-concurrency", 0, "")
	fs.IntVar(&ingestorWorkers, "ingestor-max-concurrent-workers", 0, "")
	fs.IntVar(&pageConcurrency, "ingestor-page-concurrency", 0, "")
	fs.IntVar(&cpuCores, "deepdoc-inference-cpu-cores", 0, "")
	fs.BoolVar(&initSuperUser, "init-superuser", false, "")
	fs.BoolVar(&enablePProf, "profile", false, "")
	fs.BoolVar(&helpFlag, "help", false, "")
	fs.BoolVar(&helpFlag, "h", false, "")
	fs.BoolVar(&versionFlag, "version", false, "")
	fs.BoolVar(&versionFlag, "v", false, "")
	for _, mode := range []string{"admin", "api", "ingestor", "syncer", "deepdoc", "migrate"} {
		fs.Var(modeFlag{name: mode, seen: &modes}, mode, "")
	}

	if len(argv) > 0 {
		if err := fs.Parse(argv[1:]); err != nil {
			return nil, err
		}
	}
	// The flag package silently stops at the first positional argument, which
	// would hide a mistyped mode (e.g. "ragflow_server apii" starting with no
	// mode). This binary has no positional arguments, so reject them.
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}

	// Visit visits exactly the flags present on the command line, which
	// preserves the "unspecified" (nil) vs explicit-zero distinction.
	seen := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		seen[f.Name] = true
		switch f.Name {
		case "log-level":
			args.logLevel = &logLevel
		case "config", "f":
			args.configPath = &configPath
		case "name":
			args.name = &name
		case "port", "p":
			args.port = &port
		case "deepdoc-inference-concurrency":
			args.deepdocInferenceConcurrency = &deepdocConcurrency
		case "ingestor-max-concurrent-workers":
			args.ingestorMaxConcurrentWorkers = &ingestorWorkers
		case "ingestor-page-concurrency":
			args.ingestorPageConcurrency = &pageConcurrency
		case "deepdoc-inference-cpu-cores":
			args.deepdocInferenceCPUCores = &cpuCores
		case "init-superuser":
			args.initSuperUser = initSuperUser
		case "profile":
			args.enablePProf = enablePProf
		case "help", "h":
			args.helpFlag = helpFlag
		case "version", "v":
			args.versionFlag = versionFlag
		}
	})

	if seen["admin-host"] {
		parts := strings.SplitN(adminHost, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, errors.New("--admin-host must be in the form 'ip:port'")
		}
		adminPort, convErr := strconv.Atoi(parts[1])
		if convErr != nil {
			return nil, fmt.Errorf("failed to parse admin port: %w", convErr)
		}
		args.adminHost = &parts[0]
		args.adminPort = &adminPort
	}

	// Mode flags share one slot; the last one on the command line wins.
	if len(modes) > 0 {
		mode := modes[len(modes)-1]
		args.mode = &mode
	}

	if args.logLevel != nil {
		if err := validateLogLevel(*args.logLevel); err != nil {
			return nil, err
		}
	}
	if args.port != nil && (*args.port <= 0 || *args.port > 65535) {
		return nil, fmt.Errorf("invalid port: %d", *args.port)
	}
	if args.deepdocInferenceConcurrency != nil && *args.deepdocInferenceConcurrency <= 0 {
		return nil, fmt.Errorf("invalid --deepdoc-inference-concurrency %d: must be positive", *args.deepdocInferenceConcurrency)
	}
	if args.ingestorMaxConcurrentWorkers != nil && *args.ingestorMaxConcurrentWorkers <= 0 {
		return nil, fmt.Errorf("invalid --ingestor-max-concurrent-workers %d: must be positive", *args.ingestorMaxConcurrentWorkers)
	}
	if args.ingestorPageConcurrency != nil && *args.ingestorPageConcurrency <= 0 {
		return nil, fmt.Errorf("invalid --ingestor-page-concurrency %d: must be positive", *args.ingestorPageConcurrency)
	}
	if args.deepdocInferenceCPUCores != nil && *args.deepdocInferenceCPUCores < 0 {
		return nil, fmt.Errorf("invalid --deepdoc-inference-cpu-cores %d: must be >= 0 (0 means all cores)", *args.deepdocInferenceCPUCores)
	}

	return args, nil
}

func parseArgs() (*serverArgs, error) {
	return parseArgsFrom(os.Args)
}

// modeFlag is a boolean flag that records which mode flag was set and in what
// order, so the last mode on the command line wins (matching the previous
// hand-rolled parser). "=false" selects nothing.
type modeFlag struct {
	name string
	seen *[]string
}

func (m modeFlag) String() string   { return "" }
func (m modeFlag) IsBoolFlag() bool { return true }

func (m modeFlag) Set(value string) error {
	on, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	if on {
		*m.seen = append(*m.seen, m.name)
	}
	return nil
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
	if args.logLevel != nil {
		return *args.logLevel
	}

	if configured == "" {
		return "info"
	}

	return configured
}

func printHelp(args *serverArgs) {
	switch {
	case args.mode == nil || *args.mode == "migrate":
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
		fmt.Fprintf(os.Stderr, "  --ingestor-max-concurrent-workers int\tIngestor NATS worker count K, range [1, 256] (default: 1)\n")
		fmt.Fprintf(os.Stderr, "  --ingestor-page-concurrency int\tProcess-wide page concurrency N, range [1, 16] (default: 2)\n")
		fmt.Fprintf(os.Stderr, "  --init-superuser\tInitialize superuser account (admin)\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \tPrint version information and exit\n")
		fmt.Fprintf(os.Stderr, "  --log-level string\tLog level: debug, info, warn, error (default: warn)\n")
		fmt.Fprintf(os.Stderr, "  --profile      \tEnable pprof server\n")
		fmt.Fprintf(os.Stderr, "  -h, --help     \tShow this help message and exit\n\n")
		fmt.Fprintln(os.Stderr, "MCP settings: service_conf.yaml mcp section or RAGFLOW_MCP_* environment variables.")
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
		fmt.Fprintln(os.Stderr, "\nMCP settings: service_conf.yaml mcp section or RAGFLOW_MCP_* environment variables.")
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
		fmt.Fprintf(os.Stderr, "  --ingestor-max-concurrent-workers int\tIngestor NATS worker count K, range [1, 256] (default: 1)\n")
		fmt.Fprintf(os.Stderr, "  --ingestor-page-concurrency int\tProcess-wide page concurrency N, range [1, 16] (default: 2)\n")
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
		fmt.Fprintf(os.Stderr, "  --name string\t\t\tDeepDoc server name (default: \"deepdoc_server\")\n")
		fmt.Fprintf(os.Stderr, "  --admin-host string\tAdmin server host:port (overrides config file)\n")
		fmt.Fprintf(os.Stderr, "  -v, --version  \t\tPrint version information and exit\n")
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
	var serverName string
	if arguments.name != nil {
		serverName = *arguments.name
	} else {
		serverName = fmt.Sprintf("%s_server", *arguments.mode)
	}

	logLevel := selectedLogLevel(arguments, "")
	if higherThanInfo := common.LogLevelHigherThanInfo(logLevel); higherThanInfo {
		logLevel = "info"
	}

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

	// Resolve the effective log level once (CLI > config/env > default) and
	// validate the final value: an invalid level from the config file or the
	// environment must abort startup instead of being silently downgraded by
	// the logger. The early logger above stays CLI-only because the config is
	// not loaded yet at that point.
	resolvedLogLevel, err := globalConfig.ResolveLogLevel(arguments.logLevel)
	if err != nil {
		fmt.Printf("Failed to resolve log level: %v\n", err)
		os.Exit(1)
	}
	globalConfig.SetLogLevel(resolvedLogLevel)

	// Resolve the effective server port (CLI > config/env > default) and
	// validate it.
	// NOTE: this switch must stay side-effect-free on the LOG (no
	// registerNativeDeepDoc here): it runs while only the temporary
	// stdout-only logger exists, so anything it logs is lost from the file.
	// Side effects that log (DeepDoc registration) move below, after the
	// real file-backed logger is up.
	switch *arguments.mode {
	case "api":
		port, portErr := globalConfig.ResolvePort(globalConfig.GetAPIServerConfig().HTTPPort, arguments.port)
		if portErr != nil {
			fmt.Printf("Failed to resolve API server port: %v\n", portErr)
			os.Exit(1)
		}
		globalConfig.SetAPIServerHTTPPort(port)
		// Disambiguate the log file per bound port when no explicit --name
		// is given (e.g. two API servers on the same host on different
		// ports). Restored after the precedence refactor dropped it.
		if arguments.name == nil {
			serverName = fmt.Sprintf("api_server_%d", port)
		}
	case "admin":
		port, portErr := globalConfig.ResolvePort(globalConfig.GetAdminServerConfig().HTTPPort, arguments.port)
		if portErr != nil {
			fmt.Printf("Failed to resolve admin server port: %v\n", portErr)
			os.Exit(1)
		}
		globalConfig.SetAdminHTTPPort(port)
		if arguments.name == nil {
			serverName = fmt.Sprintf("admin_server_%d", port)
		}
	case "ingestor":
		if serverName == "" {
			uuid := common.GenerateUUID()
			serverName = fmt.Sprintf("ingestor_server_%s", uuid)
		}
	case "syncer":
		if serverName == "" {
			uuid := common.GenerateUUID()
			serverName = fmt.Sprintf("syncer_server_%s", uuid)
		}
	case "deepdoc":
		if serverName == "" {
			uuid := common.GenerateUUID()
			serverName = fmt.Sprintf("deepdoc_server_%s", uuid)
		}
	case "migrate":
		if arguments.name == nil {
			serverName = "migrate"
		}
	default:
		err = errors.New(*arguments.mode)
		common.Error("invalid server mode", err)
		os.Exit(1)
	}

	// set server name and log file path
	server.SetServerName(serverName)

	// Wire the in-process DeepDoc backend only after the REAL file-backed
	// logger exists: its registration lines (and the Fatal abort on a missing
	// backend) must land in the run's log file, not in the pre-config
	// stdout-only window.
	switch *arguments.mode {
	case "api", "ingestor":
		if err = registerNativeDeepDoc(arguments); err != nil {
			common.Error("Failed to register in-process DeepDoc backend", err)
			os.Exit(1)
		}
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

	// Initialize database. The migrate mode stops here: it needs neither the
	// downgrade check nor any of the engines started below, so it can run
	// before any server mode boots (see docker/entrypoint.sh and
	// docker/launch_backend_service.sh).
	migrate := *arguments.mode == "migrate"
	if migrate {
		common.Info("Running database migrations")
	}
	if err = dao.InitDB(ctx, migrate); err != nil {
		common.Fatal("Failed to initialize database", zap.Error(err))
	}
	if migrate {
		common.Info("Database migrations completed")
		return
	}

	if err = checkDatabaseVersion(ctx); err != nil {
		common.Fatal("Database has a newer version", zap.Error(err))
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
		if err = runAPI(ctx, serverName, arguments); err != nil {
			fmt.Printf("Failed to start API server: %v\n", err)
			os.Exit(1)
		}
	case "admin":
		if err = runAdmin(ctx, serverName, arguments); err != nil {
			fmt.Printf("Failed to start ADMIN server: %v\n", err)
			os.Exit(1)
		}
	case "ingestor":
		if err = runIngestor(ctx, cancel, serverName, arguments); err != nil {
			fmt.Printf("Failed to start INGESTION worker: %v\n", err)
			os.Exit(1)
		}
	case "syncer":
		if err = runSyncer(ctx, cancel, serverName, arguments); err != nil {
			fmt.Printf("Failed to start SYNCER: %v\n", err)
			os.Exit(1)
		}
	case "deepdoc":
		if err = runDeepDoc(ctx, serverName, arguments); err != nil {
			fmt.Printf("Failed to start DEEPDOC: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Invalid server mode: %s\n", *arguments.mode)
		os.Exit(1)
	}
}

func setLogger(serverName string) {
	// log filename
	logFileName := fmt.Sprintf("%s.log", serverName)

	globalConfig := server.GetConfig()
	logConfig := globalConfig.GetLogConfig()

	// The effective level was already resolved (CLI > config/env > default)
	// and validated in main; read it back instead of re-deciding here.
	logLevel := logConfig.Level

	fileOut := common.FileOutput{
		Filename:   logFileName,
		Path:       logConfig.Path,
		MaxSize:    logConfig.MaxSize,
		MaxBackups: logConfig.MaxBackups,
		MaxAge:     logConfig.MaxAge,
		Compress:   logConfig.Compress,
	}

	common.SyncLog()
	if err := common.InitLogger(logLevel, fileOut, serverName); err != nil {
		common.Error("Failed to reinitialize logger with configured level", err)
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

func runAdmin(ctx context.Context, serverName string, args *serverArgs) error {

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
	setLogger(serverName)

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

func runIngestor(ctx context.Context, cancel context.CancelFunc, serverName string, args *serverArgs) error {
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
	// Resolve the ingestor worker count (K) from CLI > env > config > default(1)
	// and fail fast on an out-of-range value.
	workers, err := globalConfig.GetIngestorMaxConcurrentWorkers(args.ingestorMaxConcurrentWorkers)
	if err != nil {
		return err
	}
	// Apply the configured compiler pool size (no-op when 0; the pool keeps its
	// vCPU default, overridable via KC_COMPILE_CONCURRENCY).
	knowledge_compile.SetCompilerConcurrency(ingestorCfg.CompilerPoolSize)
	ingestor := ingestion.NewIngestor(*args.name, int32(workers), []string{"pdf", "docx", "txt"})
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
	navService := nlp.NewNavService(service.NewNavEmbedder(service.NewModelFactory(), ""))
	// The cluster namer/merger is the counterpart of Python's
	// _llm_create_summary / _llm_merge: a new cluster is named after the model's
	// topic title and an existing cluster's description is fused with the joining
	// document. Passing an empty model ref resolves each tenant's default chat
	// model on demand; without one the nav service keeps its deterministic
	// behavior.
	navService.SetNavMergeLLM(service.NewNavMergeLLM(service.NewModelFactory(), ""))
	nav.SetNavService(navService)
	// Memory extraction runs on the Ingestor's shared NATS consumer + worker
	// pool (task_type="memory" dispatched by handleAndExecute -> executeMemoryTask),
	// so there is no longer a dedicated Redis memory consumer to start.
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(service.NewMemoryService()))

	// Start returns immediately (it launches the owned consume/compile
	// goroutines and joins them via Stop); a provisioning failure here must
	// fail the server (main's os.Exit(1) path) instead of reporting a
	// healthy ingestor that can never consume.
	if err = ingestor.Start(); err != nil {
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

	// Set log level
	setLogger(serverName)

	// Start heartbeat reporter to admin server
	if hb := startHeartbeat(
		common.ServerTypeIngestion,
		fmt.Sprintf("ingestor-%s", ingestor.ID()[:8]),
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

func runSyncer(ctx context.Context, cancel context.CancelFunc, serverName string, args *serverArgs) error {
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

	// Set log level
	setLogger(serverName)

	// Start heartbeat reporter to admin server
	if hb := startHeartbeat(
		common.ServerTypeFileSyncer,
		fmt.Sprintf("syncer-%s", fileSyncer.ID()[:8]),
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

func runAPI(ctx context.Context, serverName string, args *serverArgs) error {
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

	if err := startServer(ctx, serverName, args); err != nil {
		return err
	}

	common.Info("Server exited")

	return nil
}

func startServer(ctx context.Context, serverName string, arguments *serverArgs) error {

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
	fileService := file.NewFileService(documentService)
	memoryService := service.NewMemoryService()
	mcpService := service.NewMCPService()
	modelProviderService := service.NewModelProviderService()
	modelFactory := service.NewModelFactory()
	modelCallService := service.NewModelCallServiceWithProviderService(modelProviderService, modelFactory)

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
		agenttool.RetrievalModelConstructors{
			Embedding: func(ctx context.Context, tenantID, modelRef string) (*modelModule.EmbeddingModel, error) {
				access := service.ModelAccess{TenantID: tenantID}
				if strings.TrimSpace(modelRef) == "" {
					return modelFactory.NewDefaultEmbeddingModel(ctx, access)
				}
				return modelFactory.NewEmbeddingModel(ctx, access, modelRef)
			},
			Chat: func(ctx context.Context, tenantID, modelRef string) (*modelModule.ChatModel, error) {
				access := service.ModelAccess{TenantID: tenantID}
				if strings.TrimSpace(modelRef) == "" {
					return modelFactory.NewDefaultChatModel(ctx, access)
				}
				return modelFactory.NewChatModel(ctx, access, modelRef)
			},
			Rerank: func(ctx context.Context, tenantID, modelRef string) (*modelModule.RerankModel, error) {
				return modelFactory.NewRerankModel(ctx, service.ModelAccess{TenantID: tenantID}, modelRef)
			},
		},
		retrievalEnhancer,
	)
	agenttool.SetRetrievalService(retrievalAdapter)
	agenttool.SetMemoryRetrievalService(retrievalbridge.NewMemoryAdapter(memoryService, retrievalEnhancer))
	common.Info("agent: retrieval service adapter installed")

	// The smart-reasoning agent's corpus tools (internal/agentic_rag): regex
	// pushdown and lexical BM25 over the same doc engine the retrieval adapter
	// wraps. The locate tools resolve these services per call, so they must be
	// registered before the server starts serving.
	runtime.SetGrepService(smartagentic.NewGrepAdapter(docEngine))
	bm25Adapter := smartagentic.NewBm25Adapter(docEngine)
	bm25Adapter.SetQueryBuilder(nlp.GetQueryBuilder())
	runtime.SetBm25Service(bm25Adapter)
	common.Info("agent: smart-reasoning corpus services installed (grep + bm25)")

	// The compiled-knowledge graph walk (graph_explore) resolves its service per
	// call the same way. Without this registration GetKgService() returns the
	// failing stub, so the tool could only ever report an infrastructure error.
	// The seed encoder is left unset: the walk then seeds by keyword match on the
	// entity rows, the documented fallback on a deployment without an embedder.
	smartagentic.SetKgService(smartagentic.NewKgAdapter(docEngine))
	common.Info("agent: smart-reasoning compiled-knowledge graph service installed")
	// The smart-reasoning agent's corpus tools resolve the shared retrieval
	// service per call, so the production adapter must be registered before the
	// server starts serving.
	runtime.SetRetrievalService(retrievalbridge.NewRuntimeAdapter(retrievalAdapter))
	common.Info("agent: runtime retrieval service wired")

	// Initialize handler layer
	authHandler := handler.NewAuthHandler()
	userHandler := handler.NewUserHandler(userService)
	tenantHandler := handler.NewTenantHandler(tenantService, userService, datasetsService)
	documentHandler := handler.NewDocumentHandler(documentService, datasetsService, fileService)
	datasetsHandler := handler.NewDatasetsHandler(datasetsService, metadataService)
	systemHandler := handler.NewSystemHandler(systemService)
	statsHandler := handler.NewStatsHandler(statsService)
	chunkHandler := handler.NewChunkHandler(chunkService, userService)
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
	mcpServerHandler := handler.NewMCPServerHandler(datasetsService, chatService)
	skillSearchHandler := handler.NewSkillSearchHandler(docEngine, documentService)
	providerHandler := handler.NewProviderHandler(userService, modelProviderService, modelCallService)
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
	// both paths share the same canvas runner.
	botService := service.NewBotService(agentService)
	botHandler := handler.NewBotHandler(botService)

	// Wire the TTS synthesizer to the per-tenant model-provider
	// dispatch. SynthesizeRequest is routed through
	// ModelProviderService.AudioSpeech, which fans out to the
	// tenant's configured TTS model driver. When the model
	// provider is unconfigured, the synthesizer falls back to a
	// no-op echo (the audio package contract), so this is always
	// safe to call.
	configureTTSSynthesizer(modelCallService)
	searchBotHandler := handler.NewSearchBotHandler(
		searchService,
		tenantService,
		modelFactory,
		chunkService,
	)
	askService := service.NewAskService(chunkService, nil, 0, 0)
	searchBotHandler.SetAskService(askService)
	chatHandler.SetMindMapDependencies(searchService, tenantService, modelFactory, chunkService)
	searchHandler.SetCompletionDependencies(modelFactory, askService)
	pluginHandler := handler.NewPluginHandler(service.NewPluginService())
	modelHandler := handler.NewModelHandler(service.NewModelProviderService())
	fileCommitHandler := handler.NewFileCommitHandler(file.NewFileCommitService())

	// Dify retrieval handler
	retrievalService := nlp.NewRetrievalService(docEngine, documentDAO)
	difyRetrievalHandler := handler.NewDifyRetrievalHandler(
		datasetsService,
		modelFactory,
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
	// on demand so Search/UpsertDoc can embed queries/summaries automatically,
	// and the cluster namer/merger resolves the tenant's default chat model the
	// same way (see the ingestor's wrapping of SetNavMergeLLM).
	apiNavService := nlp.NewNavService(service.NewNavEmbedder(modelFactory, ""))
	apiNavService.SetNavMergeLLM(service.NewNavMergeLLM(modelFactory, ""))
	nav.SetNavService(apiNavService)

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
	// Only honor X-Forwarded-For / X-Real-IP from the configured proxies
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
	mcpConfig := apiServerConfig.MCP
	if mcpConfig.Enabled {
		mcpHandler, err := mcpServerHandler.NewStandalone(ctx, authHandler, mcpConfig)
		if err != nil {
			return err
		}

		mcpCloser = mcpHandler
		defer mcpHandler.Close()
		mcpAddr := net.JoinHostPort(strings.Trim(mcpConfig.Host, "[]"), strconv.Itoa(mcpConfig.Port))
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

	common.Info(
		"\n        ____   ___    ______ ______ __\n" +
			"       / __ \\ /   |  / ____// ____// /____  _      __\n" +
			"      / /_/ // /| | / / __ / /_   / // __ \\| | /| / /\n" +
			"     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /\n" +
			"    /_/ |_|/_/  |_|\\____//_/    /_/ \\____/ |__/|__/\n",
	)
	common.Info(fmt.Sprintf("RAGFlow Go Version: %s", common.GetRAGFlowVersion()))
	common.Info(fmt.Sprintf("Server starting on port: %d", apiServerConfig.HTTPPort))

	// Set log level
	setLogger(serverName)

	// Start server in a goroutine
	go func() {
		serve(serverName, srv, apiListener)
	}()

	// Start heartbeat reporter to admin server
	if hb := startHeartbeat(
		common.ServerTypeAPI,
		fmt.Sprintf("ragflow-server-%s", common.GenerateUUID()[:8]),
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
		if err = mcpCloser.Close(); err != nil {
			common.Warn("Failed to close MCP handler", zap.Error(err))
		}
	}
	if err = shutdownHTTPServer(shutdownCtx, "API", srv); err != nil {
		return err
	}
	if mcpSrv != nil {
		if err = shutdownHTTPServer(shutdownCtx, "MCP", mcpSrv); err != nil {
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
func configureTTSSynthesizer(modelCallService *service.ModelCallService) {
	if modelCallService == nil {
		common.Info("agent: model call service not initialised; TTS in no-op echo mode")
		audio.SetModelProviderSynthesizer(nil)
		return
	}
	audio.SetModelProviderSynthesizer(audio.NewTTSDispatchFunc(modelCallService))
	common.Info("agent: TTS model-call dispatch installed (audio.Synthesize → ModelCallService.AudioSpeech)")
}

// inferenceTotalCores returns the CPU budget the DeepDoc inference config is
// validated against. It uses runtime.GOMAXPROCS(0) instead of runtime.NumCPU()
// so the budget reflects the cgroup CPU quota inside containers: Go 1.25+ derives
// GOMAXPROCS from the quota (CGroups v2 cpu.max / v1 cpu.cfs_quota_us), whereas
// runtime.NumCPU() reports the host's affinity-mask core count and ignores a
// container's CPU limit. Without this, a 2-CPU-limit pod on a 64-core host would
// "resolve" inference_cpu_cores: 0 to 64 and let inference_concurrency: 16 pass
// validation, oversubscribing the box the fail-fast guard in
// native.ValidateInferenceConfig exists to prevent.
func inferenceTotalCores() int {
	n := goruntime.GOMAXPROCS(0)
	if n < 1 {
		return 1
	}
	return n
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
func registerNativeDeepDoc(arguments *serverArgs) error {
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

	// Resolve the DeepDoc in-process inference configuration: the concurrency
	// budget K (max in-flight Runs) and the CPU-core budget N (max cores
	// inference may occupy). Both resolve from CLI > env > config > default. K
	// defaults to 1; N defaults to "unset", which is derived from K so each Run
	// stays single-threaded by default (the prior behaviour). An explicit N = 0
	// means "all cores". All values are fail-fast validated against the process's
	// cgroup-aware CPU budget at startup (see native.ValidateInferenceConfig and
	// inferenceTotalCores), so a misconfiguration aborts rather than silently
	// oversubscribing the box.
	cfg := server.GetConfig()
	K, _, errK := cfg.GetDeepDocInferenceConcurrency(arguments.deepdocInferenceConcurrency)
	if errK != nil {
		common.Fatal("invalid deepdoc inference concurrency", zap.Error(errK))
	}
	rawN, explicitN, errN := cfg.GetDeepDocInferenceCPUCores(arguments.deepdocInferenceCPUCores)
	if errN != nil {
		common.Fatal("invalid deepdoc inference cpu cores", zap.Error(errN))
	}
	// When N is unset, derive it from K so each Run opens with a single intra-op
	// thread (coresPerInference = max(1, K/K) = 1), preserving the prior
	// single-core-per-run semantics. An explicit N (including 0 = all cores) is
	// honoured as-is below.
	resolvedN := rawN
	if !explicitN {
		resolvedN = K
	}
	totalCores := inferenceTotalCores()
	// Validate K and N against the process's CPU budget (cgroup-aware) and
	// derive the per-Run intra-op thread count (max(1, N/K)). N == 0 resolves to
	// all cores here.
	coresPerInference, totalCPUCores, errV := native.ValidateInferenceConfig(totalCores, resolvedN, K)
	if errV != nil {
		common.Fatal("invalid deepdoc inference configuration", zap.Error(errV))
	}
	// Register the per-session intra-op thread count before any model session is
	// created (the native package reads it on every SessionOptions build).
	native.SetIntraOpThreads(coresPerInference)
	pdf.SetDeepDocConcurrency(K)
	native.SetInferenceLimit(K)
	common.Info("in-process DeepDoc inference limit registered",
		zap.Int("max_concurrent_inference", K),
		zap.Int("cpu_cores_per_inference", coresPerInference),
		zap.Int("total_cpu_cores_budget", totalCPUCores),
		zap.Int("gomaxprocs", goruntime.GOMAXPROCS(0)))

	// Resolve the per-document page concurrency (N) from CLI > env > config >
	// default(2) and register it with the shared page worker pool. Fail fast on
	// an out-of-range value.
	pageConcurrency, err := server.GetConfig().GetIngestorPageConcurrency(arguments.ingestorPageConcurrency)
	if err != nil {
		return err
	}
	pdf.SetPageConcurrency(pageConcurrency)
	common.Info("in-process DeepDoc page worker pool registered",
		zap.Int("page_concurrency", pageConcurrency))
	return nil
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
			zap.String("hint", "run `uv run ragflow_deps/download_deps.py`, or set "+common.EnvModelAssetsDir+" to a directory holding them"))
	}
}

// resolveDeepDocModelDir picks the model directory: the explicit DEEPDOC_MODEL_DIR
// env, else the RAGFlow default model dir (internal/rag/res/deepdoc),
// else the assets fetched by ragflow_deps/download_deps.py. The first
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
		filepath.Join(wd, "internal", "rag", "res", "deepdoc"),
		filepath.Join(wd, "huggingface.co", "InfiniFlow", "deepdoc"),
	)
	for _, c := range candidates {
		if dirHasModels(c) {
			return c
		}
	}
	// None verified; return the canonical default so any error message points
	// at the conventional location.
	return filepath.Join(wd, "internal", "rag", "res", "deepdoc")
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
