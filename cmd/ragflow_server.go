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
	"syscall"
	"time"

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
	deepdocpkg "ragflow/internal/server/deepdoc"
	ingestorpkg "ragflow/internal/server/ingestor"
	"ragflow/internal/server/local"
	syncerpkg "ragflow/internal/server/syncer"
	billing "ragflow/internal/service/billing"
	"ragflow/internal/storage"
)

var (
	registerEditionFlags = func(_ *flag.FlagSet, _ *bootstrap.ServerArgs) func() error {
		return func() error { return nil }
	}
	printHelp  = printStandaloneHelp
	runEdition = runService
)

func printStandaloneHelp(_ *bootstrap.ServerArgs) {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "RAGFlow Server - runs admin, API, ingestor, syncer and chatter in one process.")
	fmt.Fprintln(os.Stderr, "Database migration and superuser initialization run automatically.")
	fmt.Fprintln(os.Stderr, "Default ports: admin 9381, API 9380.")
	fmt.Fprintln(os.Stderr)
	printCommonHelp()
}

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

	var logLevel, configPath string
	fs.StringVar(&logLevel, "log-level", "", "")
	fs.StringVar(&configPath, "config", "", "")
	fs.StringVar(&configPath, "f", "", "")
	fs.BoolVar(&args.EnablePProf, "profile", false, "")
	fs.BoolVar(&args.HelpFlag, "help", false, "")
	fs.BoolVar(&args.HelpFlag, "h", false, "")
	fs.BoolVar(&args.VersionFlag, "version", false, "")
	fs.BoolVar(&args.VersionFlag, "v", false, "")
	finish := registerEditionFlags(fs, args)

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
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "log-level":
			args.LogLevel = &logLevel
		case "config", "f":
			args.ConfigPath = &configPath
		}
	})
	if err := finish(); err != nil {
		return nil, err
	}

	if args.LogLevel != nil {
		if err := validateLogLevel(*args.LogLevel); err != nil {
			return nil, err
		}
	}

	return args, nil
}

func parseArgs() (*bootstrap.ServerArgs, error) {
	return parseArgsFrom(os.Args)
}

func validateLogLevel(level string) error {
	switch level {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("invalid --log-level %q: must be debug, info, warn, or error", level)
	}
}

func printCommonHelp() {
	fmt.Fprintln(os.Stderr, "  -f, --config string\tPath to configuration file")
	fmt.Fprintln(os.Stderr, "  -v, --version      \tPrint version information and exit")
	fmt.Fprintln(os.Stderr, "  --log-level string \tLog level: debug, info, warn, error (default: warn)")
	fmt.Fprintln(os.Stderr, "  --profile          \tEnable pprof server")
	fmt.Fprintln(os.Stderr, "  -h, --help         \tShow this help message and exit")
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer cancel()

	arguments, err := parseArgs()
	if err != nil {
		fmt.Printf("Failed to parse arguments: %v\n", err)
		os.Exit(1)
	}

	if arguments.HelpFlag {
		printHelp(arguments)
		return
	}

	if arguments.VersionFlag {
		fmt.Printf("RAGFlow version: %s\n", common.GetRAGFlowVersion())
		return
	}

	if err := runEdition(ctx, cancel, arguments); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start RAGFlow: %v\n", err)
		os.Exit(1)
	}
}

func runService(ctx context.Context, _ context.CancelFunc, arguments *bootstrap.ServerArgs) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	combined := arguments.Mode == nil
	serverType := common.ServerTypeAPI
	serverName := "ragflow_server"
	if !combined {
		serverType = bootstrap.GetServerType(arguments)
		serverName = bootstrap.TempServerName(serverType, arguments.Name)
	}
	if err := server.InitLocalVariables(); err != nil {
		return fmt.Errorf("initialize local variables: %w", err)
	}
	logLevel := bootstrap.SelectedLogLevel(arguments, "")
	if common.LogLevelHigherThanInfo(logLevel) {
		logLevel = "info"
	}
	if err := common.InitLogger(logLevel, common.FileOutput{}, serverName); err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	defer common.SyncLog()
	var configPath string
	if arguments.ConfigPath != nil {
		configPath = *arguments.ConfigPath
	}
	if err := server.Init(configPath); err != nil {
		return fmt.Errorf("initialize configuration: %w", err)
	}
	config := server.GetConfig()
	resolvedLogLevel, err := config.ResolveLogLevel(arguments.LogLevel)
	if err != nil {
		return err
	}
	config.SetLogLevel(resolvedLogLevel)
	var port int
	if combined || serverType == common.ServerTypeAPI {
		port, err = config.ResolvePort(config.GetAPIServerConfig().HTTPPort, arguments.Port)
		if err != nil {
			return fmt.Errorf("resolve API port: %w", err)
		}
		config.SetAPIServerHTTPPort(port)
	}
	if combined || serverType == common.ServerTypeAdmin {
		port, err = config.ResolvePort(config.GetAdminServerConfig().HTTPPort, arguments.Port)
		if err != nil {
			return fmt.Errorf("resolve admin port: %w", err)
		}
		config.SetAdminHTTPPort(port)
	}
	if !combined {
		arguments.Name = config.GetServerName(*arguments.Mode)
		serverName, err = bootstrap.ResolveServerName(*arguments.Mode, arguments.Name, port)
		if err != nil {
			return err
		}
		arguments.Name = &serverName
	}
	server.SetServerName(serverName)
	bootstrap.SetLogger(serverName, arguments)
	if config.GetMode() == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	local.InitAdminStatus()
	auditConfig := config.GetAuditConfig()
	if err := common.InitAudit(auditConfig.Type, auditConfig.GetFileOutput(), serverName); err != nil {
		return fmt.Errorf("initialize audit logger: %w", err)
	}
	defer common.SyncAudit()
	if arguments.EnablePProf {
		if err := startPProf(pprofAddr()); err != nil {
			common.Error("pprof server failed", err)
		}
	}
	if isStandaloneDeepDocServer(serverType) {
		return runDeepDoc(ctx, serverName, arguments)
	}
	runtimeNeeded := combined || serverType == common.ServerTypeAPI || serverType == common.ServerTypeIngestion || serverType == common.ServerTypeChatter
	common.Info("Starting RAGFlow", zap.String("server", serverName))
	server.PrintAll()
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
	if err := dao.InitDB(ctx, combined || serverType == common.ServerTypeMigrate); err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	if serverType == common.ServerTypeMigrate {
		common.Info("Database migrations completed")
		return nil
	}
	if err := checkDatabaseVersion(ctx); err != nil {
		return err
	}
	if err := engine.InitDocEngine(ctx); err != nil {
		return fmt.Errorf("initialize doc engine: %w", err)
	}
	defer func() {
		if err := engine.Close(); err != nil {
			common.Error("Failed to close doc engine", err)
		}
	}()
	if err := kvrocks.Init(ctx); err != nil {
		return fmt.Errorf("initialize Kvrocks: %w", err)
	}
	defer kvrocks.Close()
	if err := storage.Init(ctx); err != nil {
		return fmt.Errorf("initialize storage: %w", err)
	}
	defer storage.CloseStorage()
	if err := engine.InitMessageQueue(); err != nil {
		return fmt.Errorf("initialize message queue: %w", err)
	}
	if closer, ok := engine.GetMessageQueueEngine().(io.Closer); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				common.Error("Failed to close message queue", err)
			}
		}()
	}
	if err := server.InitVariables(kvrocks.Get()); err != nil {
		common.Warn("Failed to initialize server variables from Redis, using defaults", zap.Error(err))
	}
	if combined || serverType == common.ServerTypeAPI || serverType == common.ServerTypeChatter {
		if err := billing.Init(ctx, config.GetBillingConfig()); err != nil {
			return fmt.Errorf("initialize billing: %w", err)
		}
		if err := billing.Start(ctx); err != nil {
			return fmt.Errorf("start billing: %w", err)
		}
	}
	if err := server.StartServer(ctx, cancel, serverName, serverType); err != nil {
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
		if err := deepdocpkg.RegisterNativeDeepDoc(arguments); err != nil {
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
	if combined {
		return runServices(ctx, cancel, arguments, services)
	}
	return runMode(ctx, cancel, arguments, services)
}

func runMode(ctx context.Context, cancel context.CancelFunc, args *bootstrap.ServerArgs, services *bootstrap.AgentServices) error {
	switch *args.Mode {
	case "admin":
		return admin.Run(ctx, args)
	case "api":
		return apipkg.Run(ctx, *args.Name, args, services)
	case "ingestor":
		return ingestorpkg.Run(ctx, cancel, args, services)
	case "syncer":
		return syncerpkg.Run(ctx, cancel, args)
	case "chatter":
		return chatterpkg.Run(ctx, args, services)
	default:
		return fmt.Errorf("invalid server mode: %s", *args.Mode)
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
	for _, mode := range []string{"admin", "api", "ingestor", "syncer", "chatter"} {
		if serviceCtx.Err() != nil {
			break
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
			break
		}
		serviceArgs.Name = &name
		ready := make(chan struct{})
		serviceArgs.Ready = func() error { close(ready); return nil }
		group.Go(func() error {
			err := runMode(serviceCtx, cancel, &serviceArgs, services)
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
			if startupErr = waitReady(mode, ready); startupErr != nil {
				break
			}
		} else {
			workerReady = append(workerReady, ready)
		}
	}
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
