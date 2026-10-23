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
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/kvrocks"
	_ "ragflow/internal/ingestion/wire"
	"ragflow/internal/server"
	"ragflow/internal/server/admin"
	apipkg "ragflow/internal/server/api"
	"ragflow/internal/server/bootstrap"
	deepdocpkg "ragflow/internal/server/deepdoc"
	ingestorpkg "ragflow/internal/server/ingestor"
	syncerpkg "ragflow/internal/server/syncer"
	"ragflow/internal/storage"
)

// isStandaloneDeepDocServer reports whether st is a standalone DeepDoc
// inference server (CPU/GPU/Ascend). These boot the ONNX runtime only and never
// the full RAGFlow infrastructure, so main() returns early for them before
// initializing Redis/DB/engine/cache/storage/MQ. Every other server type — and
// the unknown sentinel — returns false.
func isStandaloneDeepDocServer(st common.ServerType) bool {
	return st == common.ServerTypeDeepDocCPU ||
		st == common.ServerTypeDeepDocGPU ||
		st == common.ServerTypeDeepDocAscend
}

// startPProfIfRequested launches the pprof HTTP server on localhost:6060.
// The blank "net/http/pprof" import above registers the handlers on
// http.DefaultServeMux, so only the listener needs starting. Both boot
// paths (standalone DeepDoc and the regular one) call it right after the
// logger is up, so its startup line lands in the run's log file. A bind
// failure is logged and non-fatal, matching the previous inline behavior.
func startPProfIfRequested() {
	go func() {
		common.Info("Starting pprof server", zap.String("addr", "localhost:6060"))
		if pprofErr := http.ListenAndServe("localhost:6060", nil); pprofErr != nil {
			common.Error("pprof server failed", pprofErr)
		}
	}()
}

// parseArgsFrom parses argv (argv[0] is the program name, mirroring os.Args)
// into a bootstrap.ServerArgs using the standard library flag package. The accepted
// syntax is exactly the standard one: every flag works as "-flag value",
// "-flag=value", "--flag value" and "--flag=value"; boolean flags also accept
// "=true"/"=false". Flags absent from argv leave their bootstrap.ServerArgs field nil,
// which callers use to distinguish "unspecified" from an explicit zero value.
func parseArgsFrom(argv []string) (*bootstrap.ServerArgs, error) {
	fs := flag.NewFlagSet("ragflow_server", flag.ContinueOnError)
	// Suppress the standard usage/error dump: main prints its own one-line
	// error and printHelp owns the usage text.
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	args := &bootstrap.ServerArgs{}

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
			args.LogLevel = &logLevel
		case "config", "f":
			args.ConfigPath = &configPath
		case "name":
			args.Name = &name
		case "port", "p":
			args.Port = &port
		case "deepdoc-inference-concurrency":
			args.DeepdocInferenceConcurrency = &deepdocConcurrency
		case "ingestor-max-concurrent-workers":
			args.IngestorMaxConcurrentWorkers = &ingestorWorkers
		case "ingestor-page-concurrency":
			args.IngestorPageConcurrency = &pageConcurrency
		case "deepdoc-inference-cpu-cores":
			args.DeepdocInferenceCPUCores = &cpuCores
		case "init-superuser":
			args.InitSuperUser = initSuperUser
		case "profile":
			args.EnablePProf = enablePProf
		case "help", "h":
			args.HelpFlag = helpFlag
		case "version", "v":
			args.VersionFlag = versionFlag
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
		args.AdminHost = &parts[0]
		args.AdminPort = &adminPort
	}

	// Mode flags share one slot; the last one on the command line wins.
	if len(modes) > 0 {
		mode := modes[len(modes)-1]
		args.Mode = &mode
	}

	if args.LogLevel != nil {
		if err := validateLogLevel(*args.LogLevel); err != nil {
			return nil, err
		}
	}
	if args.Port != nil && (*args.Port <= 0 || *args.Port > 65535) {
		return nil, fmt.Errorf("invalid port: %d", *args.Port)
	}
	if args.DeepdocInferenceConcurrency != nil && *args.DeepdocInferenceConcurrency <= 0 {
		return nil, fmt.Errorf("invalid --deepdoc-inference-concurrency %d: must be positive", *args.DeepdocInferenceConcurrency)
	}
	if args.IngestorMaxConcurrentWorkers != nil && *args.IngestorMaxConcurrentWorkers <= 0 {
		return nil, fmt.Errorf("invalid --ingestor-max-concurrent-workers %d: must be positive", *args.IngestorMaxConcurrentWorkers)
	}
	if args.IngestorPageConcurrency != nil && *args.IngestorPageConcurrency <= 0 {
		return nil, fmt.Errorf("invalid --ingestor-page-concurrency %d: must be positive", *args.IngestorPageConcurrency)
	}
	if args.DeepdocInferenceCPUCores != nil && *args.DeepdocInferenceCPUCores < 0 {
		return nil, fmt.Errorf("invalid --deepdoc-inference-cpu-cores %d: must be >= 0 (0 means all cores)", *args.DeepdocInferenceCPUCores)
	}

	return args, nil
}

func parseArgs() (*bootstrap.ServerArgs, error) {
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

func printHelp(args *bootstrap.ServerArgs) {
	switch {
	case args.Mode == nil || *args.Mode == "migrate":
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
	case *args.Mode == "api":
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
	case *args.Mode == "admin":
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
	case *args.Mode == "ingestor":
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
	case *args.Mode == "syncer":
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
	case *args.Mode == "deepdoc":
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

	if arguments.HelpFlag || arguments.Mode == nil {
		printHelp(arguments)
		os.Exit(1)
	}

	if arguments.VersionFlag {
		fmt.Printf("RAGFlow version: %s\n", common.GetRAGFlowVersion())
		os.Exit(1)
	}

	// The DeepDoc inference server runs standalone: it only needs the ONNX
	// runtime and MODEL_DIR, never the full RAGFlow infrastructure
	// (DB/engine/cache/storage/MQ). Boot it and return before the heavy
	// infrastructure init, so a bare inference container needs no MySQL/Redis.
	if st := bootstrap.GetServerType(arguments); isStandaloneDeepDocServer(st) {
		// SetServerName writes into the package-level local variables that
		// InitLocalVariables allocates; this early boot returns before the
		// shared InitLocalVariables call below, so it must allocate them
		// itself. The initializer constructs the in-memory struct only and
		// touches no external service, so the bare inference container stays
		// free of MySQL/Redis.
		if err = server.InitLocalVariables(); err != nil {
			fmt.Printf("Failed to start %s server: %v\n", *arguments.Mode, err)
			os.Exit(1)
		}
		logLevel := bootstrap.SelectedLogLevel(arguments, "")
		if higherThanInfo := common.LogLevelHigherThanInfo(logLevel); higherThanInfo {
			logLevel = "info"
		}
		if err = common.InitLogger(logLevel, common.FileOutput{}, bootstrap.TempServerName(st, arguments.Name)); err != nil {
			panic("failed to initialize logger: " + err.Error())
		}
		finalName, nameErr := bootstrap.ResolveServerName(*arguments.Mode, arguments.Name, 0)
		if nameErr != nil {
			common.Error("invalid server mode", nameErr)
			os.Exit(1)
		}
		server.SetServerName(finalName)
		// The regular pprof block sits after the infrastructure init this
		// path skips, so --profile must be honored here or the flag the
		// --deepdoc help text advertises would be silently ignored.
		if arguments.EnablePProf {
			startPProfIfRequested()
		}
		if err = runDeepDoc(ctx, finalName, arguments); err != nil {
			fmt.Printf("Failed to start DEEPDOC: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Initialize local variables (runtime variables from Redis)
	err = server.InitLocalVariables()
	if err != nil {
		fmt.Printf("Failed to start %s server: %v\n", *arguments.Mode, err)
		os.Exit(1)
	}

	serverType := bootstrap.GetServerType(arguments)

	// Temporary logger name (pre-config): explicit --name wins, else
	// "<mode>_server". The final name (with the resolved port for api/admin)
	// is derived below after the configuration is loaded.
	serverName := bootstrap.TempServerName(serverType, arguments.Name)

	logLevel := bootstrap.SelectedLogLevel(arguments, "")
	if higherThanInfo := common.LogLevelHigherThanInfo(logLevel); higherThanInfo {
		logLevel = "info"
	}

	if err = common.InitLogger(logLevel, common.FileOutput{}, serverName); err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	// Initialize configuration
	var configPath string
	if arguments.ConfigPath != nil {
		configPath = *arguments.ConfigPath
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
	resolvedLogLevel, err := globalConfig.ResolveLogLevel(arguments.LogLevel)
	if err != nil {
		fmt.Printf("Failed to resolve log level: %v\n", err)
		os.Exit(1)
	}
	globalConfig.SetLogLevel(resolvedLogLevel)

	// Resolve the effective server port (CLI > config/env > default) and set it
	// on the config so the server binds the right port. This block must stay
	// side-effect-free on the LOG (no registerNativeDeepDoc here): it runs
	// while only the temporary stdout-only logger exists, so anything it logs is
	// lost from the file. Side effects that log (DeepDoc registration) move
	// below, after the real file-backed logger is up. The dead UUID branch for
	// ingestor/syncer/deepdoc (serverName was already pre-filled above) is
	// dropped: ResolveServerName resolves them to "<mode>_server", matching the
	// original no-name behavior.
	var resolvedPort int
	switch serverType {
	case common.ServerTypeAPI:
		port, portErr := globalConfig.ResolvePort(globalConfig.GetAPIServerConfig().HTTPPort, arguments.Port)
		if portErr != nil {
			fmt.Printf("Failed to resolve API server port: %v\n", portErr)
			os.Exit(1)
		}
		globalConfig.SetAPIServerHTTPPort(port)
		resolvedPort = port
	case common.ServerTypeAdmin:
		port, portErr := globalConfig.ResolvePort(globalConfig.GetAdminServerConfig().HTTPPort, arguments.Port)
		if portErr != nil {
			fmt.Printf("Failed to resolve admin server port: %v\n", portErr)
			os.Exit(1)
		}
		globalConfig.SetAdminHTTPPort(port)
		resolvedPort = port
	case common.ServerTypeIngestion, common.ServerTypeFileSyncer, common.ServerTypeDeepDocGPU, common.ServerTypeDeepDocAscend, common.ServerTypeDeepDocCPU, common.ServerTypeMigrate:
		// No port embedding; the final name is derived below.
	default:
		common.Error(fmt.Sprintf("invalid server mode: %v", serverType), nil)
		os.Exit(1)
	}

	// Final server name: explicit --name wins; api/admin embed the resolved
	// port; ingestor/syncer/deepdoc/migrate use their fixed forms. An unknown
	// mode is rejected by the switch above.
	finalName, nameErr := bootstrap.ResolveServerName(*arguments.Mode, arguments.Name, resolvedPort)
	if nameErr != nil {
		common.Error("invalid server mode", nameErr)
		os.Exit(1)
	}
	serverName = finalName

	// set server name and log file path
	server.SetServerName(serverName)

	// Wire the in-process DeepDoc backend only after the REAL file-backed
	// logger exists: its registration lines (and the Fatal abort on a missing
	// backend) must land in the run's log file, not in the pre-config
	// stdout-only window.
	switch serverType {
	case common.ServerTypeAPI, common.ServerTypeIngestion:
		if err = deepdocpkg.RegisterNativeDeepDoc(arguments); err != nil {
			common.Error("Failed to register in-process DeepDoc backend", err)
			os.Exit(1)
		}
	default:
	}

	// Print all configuration settings
	common.Info(fmt.Sprintf("Starting %v server: %s, mode: %s", serverType, serverName, globalConfig.GetMode()))
	server.PrintAll()

	// Start pprof server if requested
	if arguments.EnablePProf {
		startPProfIfRequested()
	}

	// Initialize database. The migrate mode stops here: it needs neither the
	// downgrade check nor any of the engines started below, so it can run
	// before any server mode boots (see docker/entrypoint.sh and
	// docker/launch_backend_service.sh).
	migrate := serverType == common.ServerTypeMigrate
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

	if err = server.StartServer(ctx, cancel, serverName, serverType); err != nil {
		common.Error("Failed to start EE server", err)
		os.Exit(1)
	}
	defer server.ShutdownServer(ctx)

	if arguments.Name == nil {
		arguments.Name = &serverName
	}

	switch serverType {
	case common.ServerTypeAPI:
		if err = apipkg.Run(ctx, serverName, arguments); err != nil {
			fmt.Printf("Failed to start API server: %v\n", err)
			os.Exit(1)
		}
	case common.ServerTypeAdmin:
		if err = admin.Run(ctx, serverName, arguments); err != nil {
			fmt.Printf("Failed to start ADMIN server: %v\n", err)
			os.Exit(1)
		}
	case common.ServerTypeIngestion:
		if err = ingestorpkg.Run(ctx, cancel, serverName, arguments); err != nil {
			fmt.Printf("Failed to start INGESTION worker: %v\n", err)
			os.Exit(1)
		}
	case common.ServerTypeFileSyncer:
		if err = syncerpkg.Run(ctx, cancel, serverName, arguments); err != nil {
			fmt.Printf("Failed to start SYNCER: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Invalid server mode: %v\n", serverType)
		os.Exit(1)
	}
}

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
