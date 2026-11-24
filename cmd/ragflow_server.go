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
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/kvrocks"
	_ "ragflow/internal/ingestion/wire"
	"ragflow/internal/server"
	"ragflow/internal/server/admin"
	apipkg "ragflow/internal/server/api"
	"ragflow/internal/server/bootstrap"
	chatterpkg "ragflow/internal/server/chatter"
	"ragflow/internal/server/config"
	deepdocpkg "ragflow/internal/server/deepdoc"
	ingestorpkg "ragflow/internal/server/ingestor"
	"ragflow/internal/server/local"
	syncerpkg "ragflow/internal/server/syncer"
	"ragflow/internal/service/billing"
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

// ErrPProfBindFailed is returned by startPProf when the profiling port is
// unavailable. Callers treat it as non-fatal: the inference server matters more
// than profiling, so a bind failure is logged and startup continues.
var ErrPProfBindFailed = errors.New("pprof bind failed")

// startPProf opens a TCP listener on addr and serves the pprof handlers
// (registered by the blank "net/http/pprof" import) for the process lifetime.
// It returns ErrPProfBindFailed if addr is already taken instead of crashing
// the server. The address is configurable via pprofAddr so tests (and ops) can
// avoid the hard-coded localhost:6060 clash.
func startPProf(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPProfBindFailed, err)
	}
	go func() {
		common.Info("Starting pprof server", zap.String("addr", addr))
		if pprofErr := http.Serve(ln, nil); pprofErr != nil {
			common.Error("pprof server failed", pprofErr)
		}
	}()
	return nil
}

// pprofAddr returns the listen address for the pprof server. It honors
// RAGFLOW_PPROF_ADDR so callers/tests can override the default localhost:6060,
// eliminating hard-coded-port clashes between concurrent tests and processes.
func pprofAddr() string {
	if a := os.Getenv("RAGFLOW_PPROF_ADDR"); a != "" {
		return a
	}
	return "localhost:6060"
}

// parseArgsFrom parses argv (argv[0] is the program name, mirroring os.Args)
// into a bootstrap.ServerArgs using kingpin. An omitted port or log level
// leaves its field nil so configuration and environment values can take effect.
func parseArgsFrom(argv []string) (*bootstrap.ServerArgs, error) {

	args := &bootstrap.ServerArgs{}
	app := kingpin.New("ragflow_server", "RAGFlow server")

	args.ConfigPath = app.Flag("config", "Config path").Short('f').String()
	args.DeepdocDevice = app.Flag("deepdoc-device", "DeepDoc inference device").String()
	args.EnablePProf = app.Flag("profile", "Enable pprof profiling").Bool()
	var logLevelSet, portSet bool
	args.LogLevel = app.Flag("log-level", "Log level").IsSetByUser(&logLevelSet).String()
	args.Port = app.Flag("port", "HTTP port (API by default, admin with --admin)").IsSetByUser(&portSet).Int()
	validateFlags := registerEEFlags(app, args)

	modes := []string{"admin", "api", "ingestor", "syncer", "chatter", "migrate"}
	modeFlags := make([]*bool, len(modes))
	for i, mode := range modes {
		modeFlags[i] = app.Flag(mode, fmt.Sprintf("Enable %s mode", mode)).Bool()
	}

	// kingpin owns --help itself, so registering another "help" flag above would
	// fail with "duplicate long flag --help". Handle -h/--help and -v/--version
	// ourselves: keep them out of app.Parse, and return early so no server-only
	// validation runs. Also return parse errors instead of letting MustParse
	// os.Exit (which kills the test binary and hides the failing case).
	help, version := false, false
	rest := make([]string, 0, len(argv))
	if len(argv) > 0 {
		rest = append(rest, argv[0])
	}
	for _, arg := range argv[1:] {
		switch arg {
		case "-h", "--help":
			help = true
		case "-v", "--version":
			version = true
		default:
			rest = append(rest, arg)
		}
	}
	args.HelpFlag = &help
	args.VersionFlag = &version

	if _, err := app.Parse(rest[1:]); err != nil {
		return nil, err
	}
	if help || version {
		return args, nil
	}

	for i, enabled := range modeFlags {
		if !*enabled {
			continue
		}
		if args.Mode != nil {
			return nil, fmt.Errorf("only one mode flag may be specified: --%s and --%s", *args.Mode, modes[i])
		}
		args.Mode = &modes[i]
	}

	args.InitSuperUser = args.InitSuperUser || (args.Mode != nil && *args.Mode == "admin")
	if err := validateFlags(); err != nil {
		return nil, err
	}

	if logLevelSet {
		if err := validateLogLevel(*args.LogLevel); err != nil {
			return nil, err
		}
	} else {
		args.LogLevel = nil
	}
	if portSet {
		if *args.Port < config.MinPort || *args.Port > config.MaxPort {
			return nil, fmt.Errorf("invalid --port %d: must be in [%d, %d]", *args.Port, config.MinPort, config.MaxPort)
		}
	} else {
		args.Port = nil
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

func printHelp(_ *bootstrap.ServerArgs) {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "RAGFlow Server - runs admin, API, ingestor, syncer and chatter in one process by default.")
	fmt.Fprintln(os.Stderr, "Default startup runs database migrations and initializes the superuser automatically.")
	fmt.Fprintln(os.Stderr, "Default ports: admin 9381, API 9380.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Select at most one mode:")
	fmt.Fprintln(os.Stderr, "  --admin            \tRun only the admin server and initialize the superuser")
	fmt.Fprintln(os.Stderr, "  --api              \tRun only the API server")
	fmt.Fprintln(os.Stderr, "  --ingestor         \tRun only the ingestion worker")
	fmt.Fprintln(os.Stderr, "  --syncer           \tRun only the file syncer")
	fmt.Fprintln(os.Stderr, "  --chatter          \tRun only the chat-channel service")
	fmt.Fprintln(os.Stderr, "  --migrate          \tRun database migrations and exit")
	fmt.Fprintln(os.Stderr, "Run --migrate before starting individual services.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "  -f, --config string\tPath to configuration file")
	fmt.Fprintln(os.Stderr, "  -v, --version      \tPrint version information and exit")
	fmt.Fprintln(os.Stderr, "  --log-level string \tLog level: debug, info, warn, error (default: warn)")
	fmt.Fprintln(os.Stderr, "  --port int         \tHTTP port: API by default, admin with --admin (1-65535)")
	fmt.Fprintln(os.Stderr, "  --profile          \tEnable pprof server")
	fmt.Fprintln(os.Stderr, "  -h, --help         \tShow this help message and exit")
	printEEHelp()
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer cancel()

	arguments, err := parseArgsFrom(os.Args)
	if err != nil {
		fmt.Printf("Failed to parse arguments: %v\n", err)
		os.Exit(1)
	}

	if *arguments.HelpFlag {
		printHelp(arguments)
		return
	}

	if *arguments.VersionFlag {
		fmt.Printf("RAGFlow version: %s\n", common.GetRAGFlowVersion())
		return
	}

	if err = runServer(ctx, cancel, arguments); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start RAGFlow: %v\n", err)
		os.Exit(1)
	}
}

func runServer(ctx context.Context, _ context.CancelFunc, arguments *bootstrap.ServerArgs) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	serverType := common.ServerTypeStandalone
	serverName := "ragflow_standalone"
	if arguments.Mode != nil {
		serverType = bootstrap.GetServerType(arguments)
		serverName = bootstrap.TempServerName(serverType, arguments.Name)
	}

	// Init local variables
	server.InitLocalVariables()

	// Init logger from arguments
	logLevel := bootstrap.GetLogLevel(arguments)
	if common.LogLevelHigherThanInfo(logLevel) {
		logLevel = "info"
	}
	common.InitLogger(logLevel, common.FileOutput{}, serverName)
	defer common.SyncLog()

	var configPath string
	if arguments.ConfigPath != nil {
		configPath = *arguments.ConfigPath
	}

	// Read config file
	if err := server.Init(configPath); err != nil {
		return fmt.Errorf("initialize configuration: %w", err)
	}

	// Set log level according to config file
	globalConfig := server.GetConfig()
	resolvedLogLevel, err := globalConfig.ResolveLogLevel(arguments.LogLevel)
	if err != nil {
		return err
	}
	globalConfig.SetLogLevel(resolvedLogLevel)

	var port int
	if serverType == common.ServerTypeStandalone || serverType == common.ServerTypeAPI {
		port, err = globalConfig.ResolvePort(globalConfig.GetAPIServerConfig().HTTPPort, arguments.Port)
		if err != nil {
			return fmt.Errorf("resolve API port: %w", err)
		}
		globalConfig.SetAPIServerHTTPPort(port)
	}

	if serverType == common.ServerTypeStandalone || serverType == common.ServerTypeAdmin {
		var adminPort *int
		if serverType == common.ServerTypeAdmin {
			adminPort = arguments.Port
		}
		port, err = globalConfig.ResolvePort(globalConfig.GetAdminServerConfig().HTTPPort, adminPort)
		if err != nil {
			return fmt.Errorf("resolve admin port: %w", err)
		}
		globalConfig.SetAdminHTTPPort(port)
	}

	if serverType != common.ServerTypeStandalone {
		arguments.Name = globalConfig.GetServerName(*arguments.Mode)
		serverName, err = bootstrap.ResolveServerName(*arguments.Mode, arguments.Name, port)
		if err != nil {
			return err
		}
		arguments.Name = &serverName
	}
	server.SetServerName(serverName)
	bootstrap.SetLogger(serverName, resolvedLogLevel)
	onReady := arguments.Ready
	arguments.Ready = func() error {
		if err := common.SetLogLevel(resolvedLogLevel); err != nil {
			return fmt.Errorf("set runtime log level: %w", err)
		}
		if onReady != nil {
			return onReady()
		}
		return nil
	}
	if globalConfig.GetMode() == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	local.InitAdminStatus()
	auditConfig := globalConfig.GetAuditConfig()
	if err = common.InitAudit(auditConfig.Type, auditConfig.GetFileOutput(), serverName); err != nil {
		return fmt.Errorf("initialize audit logger: %w", err)
	}
	defer common.SyncAudit()
	if *arguments.EnablePProf {
		if err = startPProf(pprofAddr()); err != nil {
			common.Error("pprof server failed", err)
		}
	}
	if isStandaloneDeepDocServer(serverType) {
		return runDeepDoc(ctx, serverName, arguments)
	}
	runtimeNeeded := serverType == common.ServerTypeStandalone || serverType == common.ServerTypeAPI || serverType == common.ServerTypeIngestion
	common.Info("Starting RAGFlow", zap.String("server", serverName))
	// Keep the full configuration dump tied to the operator's chosen verbosity.
	if !common.LogLevelHigherThanInfo(resolvedLogLevel) {
		server.PrintAll()
	}
	defer func() {
		if dao.DB == nil {
			return
		}
		db, err := dao.DB.DB()
		if err != nil {
			common.Error("Failed to get database for shutdown", err)
			return
		}
		if err := db.Close(); err != nil {
			common.Error("Failed to close database", err)
		}
	}()

	if err = dao.InitDB(ctx, serverType == common.ServerTypeStandalone || serverType == common.ServerTypeMigrate); err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	// Migration mode exits before initializing dependencies used only by services.
	if serverType == common.ServerTypeMigrate {
		common.Info("Database migrations completed")
		return nil
	}
	if err = checkDatabaseVersion(ctx); err != nil {
		return err
	}
	if err = engine.InitDocEngine(ctx); err != nil {
		return fmt.Errorf("initialize doc engine: %w", err)
	}
	defer func() {
		if err := engine.Close(); err != nil {
			common.Error("Failed to close doc engine", err)
		}
	}()
	if err = kvrocks.Init(ctx); err != nil {
		return fmt.Errorf("initialize Kvrocks: %w", err)
	}
	defer kvrocks.Close()
	if err = storage.Init(ctx); err != nil {
		return fmt.Errorf("initialize storage: %w", err)
	}
	defer storage.CloseStorage()
	if err = engine.InitMessageQueue(); err != nil {
		return fmt.Errorf("initialize message queue: %w", err)
	}
	if closer, ok := engine.GetMessageQueueEngine().(io.Closer); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				common.Error("Failed to close message queue", err)
			}
		}()
	}
	if err = server.InitVariables(kvrocks.Get()); err != nil {
		common.Warn("Failed to initialize server variables from Redis, using defaults", zap.Error(err))
	}
	if serverType == common.ServerTypeStandalone || serverType == common.ServerTypeAPI || serverType == common.ServerTypeIngestion {
		if err = billing.Init(ctx, globalConfig.GetBillingConfig()); err != nil {
			return fmt.Errorf("initialize billing: %w", err)
		}
	}
	if serverType == common.ServerTypeStandalone || serverType == common.ServerTypeAPI {
		if err = billing.Start(ctx); err != nil {
			return fmt.Errorf("start billing: %w", err)
		}
	}
	if err = server.StartServer(ctx, cancel, serverName, serverType); err != nil {
		return err
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer shutdownCancel()
		if err := server.ShutdownServer(shutdownCtx); err != nil {
			common.Error("Failed to stop server services", err)
		}
	}()
	defer cancel()
	var services *bootstrap.AgentServices
	if runtimeNeeded {
		if err = deepdocpkg.RegisterNativeDeepDoc(arguments); err != nil {
			return fmt.Errorf("register in-process DeepDoc backend: %w", err)
		}
		closeTokenizer, err := bootstrap.InitializeAgentTokenizer()
		if err != nil {
			return err
		}
		defer closeTokenizer()
		bootstrap.SetIngestLog(serverName)
		defer common.SyncIngestLog()
		services = bootstrap.NewAgentServices()
	}
	// Stop and join service work before closing any process-wide resource.
	defer cancel()
	switch serverType {
	case common.ServerTypeStandalone:
		return runServices(ctx, cancel, arguments, services)
	case common.ServerTypeAdmin:
		return admin.Run(ctx, arguments)
	case common.ServerTypeAPI:
		return apipkg.Run(ctx, *arguments.Name, arguments, services)
	case common.ServerTypeIngestion:
		return ingestorpkg.Run(ctx, cancel, arguments, services)
	case common.ServerTypeFileSyncer:
		return syncerpkg.Run(ctx, cancel, arguments)
	case common.ServerTypeChatter:
		return chatterpkg.Run(ctx, arguments)
	default:
		return fmt.Errorf("unsupported server mode %q", *arguments.Mode)
	}
}

func runServices(ctx context.Context, cancel context.CancelFunc, args *bootstrap.ServerArgs, services *bootstrap.AgentServices) error {
	group, serviceCtx := errgroup.WithContext(ctx)
	var workerReady []<-chan struct{}
	waitReady := func(mode string, ready <-chan struct{}) error {
		timer := time.NewTimer(5 * time.Minute)
		defer timer.Stop()
		select {
		case <-ready:
			return nil
		case <-serviceCtx.Done():
			return serviceCtx.Err()
		case <-timer.C:
			return fmt.Errorf("%s startup timed out", mode)
		}
	}
	var startupErr error
	startService := func(mode string, run func(context.Context, *bootstrap.ServerArgs) error) {
		if startupErr != nil || serviceCtx.Err() != nil {
			return
		}
		serviceArgs := *args
		serviceArgs.Mode = &mode
		serviceArgs.InitSuperUser = mode == "admin"
		var port int
		config := server.GetConfig()
		if mode == "admin" {
			port = config.GetAdminServerConfig().HTTPPort
		} else if mode == "api" {
			port = config.GetAPIServerConfig().HTTPPort
		}
		name, err := bootstrap.ResolveServerName(mode, config.GetServerName(mode), port)
		if err != nil {
			startupErr = err
			return
		}
		serviceArgs.Name = &name
		ready := make(chan struct{})
		serviceArgs.Ready = func() error { close(ready); return nil }
		group.Go(func() error {
			err := run(serviceCtx, &serviceArgs)
			if err != nil {
				if errors.Is(err, context.Canceled) && serviceCtx.Err() != nil {
					return nil
				}
				return fmt.Errorf("%s: %w", mode, err)
			}
			if serviceCtx.Err() == nil {
				return fmt.Errorf("%s exited unexpectedly", mode)
			}
			return nil
		})
		if mode == "admin" || mode == "api" {
			startupErr = waitReady(mode, ready)
		} else {
			workerReady = append(workerReady, ready)
		}
	}
	startService("admin", admin.Run)
	startService("api", func(ctx context.Context, args *bootstrap.ServerArgs) error {
		return apipkg.Run(ctx, *args.Name, args, services)
	})
	startService("ingestor", func(ctx context.Context, args *bootstrap.ServerArgs) error {
		return ingestorpkg.Run(ctx, cancel, args, services)
	})
	startService("syncer", func(ctx context.Context, args *bootstrap.ServerArgs) error {
		return syncerpkg.Run(ctx, cancel, args)
	})
	startService("chatter", func(ctx context.Context, args *bootstrap.ServerArgs) error {
		return chatterpkg.Run(ctx, args)
	})
	if startupErr == nil {
		for _, ready := range workerReady {
			if startupErr = waitReady("worker", ready); startupErr != nil {
				break
			}
		}
	}
	if startupErr == nil && serviceCtx.Err() == nil && args.Ready != nil {
		startupErr = args.Ready()
	}
	if startupErr != nil {
		cancel()
	}
	if err := group.Wait(); err != nil {
		return err
	}
	if errors.Is(startupErr, context.Canceled) {
		return nil
	}
	return startupErr
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
	older, comparableVersion := common.IsOlderReleaseThan(codeVersion, databaseVersion)
	if !comparableVersion {
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
