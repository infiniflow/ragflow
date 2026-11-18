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
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

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
	chatterpkg "ragflow/internal/server/chatter"
	deepdocpkg "ragflow/internal/server/deepdoc"
	ingestorpkg "ragflow/internal/server/ingestor"
	syncerpkg "ragflow/internal/server/syncer"
	billing "ragflow/internal/service/billing"
	"ragflow/internal/storage"
)

const serviceChildEnv = "RAGFLOW_INTERNAL_SERVICE"

var (
	registerEditionFlags = func(_ *flag.FlagSet, _ *bootstrap.ServerArgs) func() error {
		return func() error { return nil }
	}
	printHelp  = printStandaloneHelp
	runEdition = runStandalone
)

func printStandaloneHelp(_ *bootstrap.ServerArgs) {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "RAGFlow Server - starts admin, API, ingestor, syncer and chatter.")
	fmt.Fprintln(os.Stderr, "Database migration and superuser initialization run automatically.")
	fmt.Fprintln(os.Stderr, "Default ports: admin 9381, API 9380.")
	fmt.Fprintln(os.Stderr)
	printCommonHelp()
}

func runStandalone(ctx context.Context, _ context.CancelFunc, args *bootstrap.ServerArgs) error {
	if mode := os.Getenv(serviceChildEnv); mode != "" {
		switch mode {
		case "migrate", "admin", "api", "ingestor", "syncer", "chatter":
		default:
			return fmt.Errorf("invalid internal service %q", mode)
		}
		// The inherited pipe belongs only to a supervised child. It reports
		// readiness without probing ports that another process might own.
		ready := os.NewFile(3, "service-ready")
		if ready == nil {
			return errors.New("missing service readiness pipe")
		}
		defer ready.Close()
		info, err := ready.Stat()
		if err != nil {
			return fmt.Errorf("inspect service readiness pipe: %w", err)
		}
		if info.Mode()&os.ModeNamedPipe == 0 {
			return errors.New("service readiness descriptor is not a pipe")
		}
		syscall.CloseOnExec(int(ready.Fd()))
		if err := os.Unsetenv(serviceChildEnv); err != nil {
			return err
		}
		args.Mode = &mode
		args.InitSuperUser = mode == "admin"
		args.EnablePProf = args.EnablePProf && mode == "api"
		args.Ready = func() error {
			_, err := ready.Write([]byte{1})
			return err
		}
		childCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		return runService(childCtx, cancel, args)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return superviseServices(ctx, executable, os.Args[1:])
}

// superviseServices owns every child until it exits. Failure of any service
// stops the entire application; cancellation gives children time to clean up
// before exec's WaitDelay forces termination.
func superviseServices(ctx context.Context, executable string, argv []string) error {
	ctx, cancel := context.WithCancel(ctx)
	var children sync.WaitGroup
	defer children.Wait()
	defer cancel()

	exits := make(chan error, 6)
	start := func(mode string) (<-chan error, <-chan error, error) {
		readPipe, writePipe, err := os.Pipe()
		if err != nil {
			return nil, nil, err
		}
		cmd := exec.CommandContext(ctx, executable, argv...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		for _, env := range os.Environ() {
			if !strings.HasPrefix(env, serviceChildEnv+"=") {
				cmd.Env = append(cmd.Env, env)
			}
		}
		cmd.Env = append(cmd.Env, serviceChildEnv+"="+mode)
		cmd.ExtraFiles = []*os.File{writePipe}
		cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
		cmd.WaitDelay = 40 * time.Second
		if err := cmd.Start(); err != nil {
			readPipe.Close()
			writePipe.Close()
			return nil, nil, fmt.Errorf("start %s: %w", mode, err)
		}
		writePipe.Close()
		done := make(chan error, 1)
		ready := make(chan error, 1)
		children.Add(2)
		go func() {
			defer children.Done()
			defer readPipe.Close()
			var signal [1]byte
			_, err := io.ReadFull(readPipe, signal[:])
			ready <- err
		}()
		go func() {
			defer children.Done()
			err := cmd.Wait()
			done <- err
			if mode != "migrate" {
				if err == nil {
					err = errors.New("service exited unexpectedly")
				}
				exits <- fmt.Errorf("%s: %w", mode, err)
			}
		}()
		return done, ready, nil
	}

	done, _, err := start("migrate")
	if err != nil {
		return err
	}
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	case <-ctx.Done():
		return nil
	}
	for _, mode := range []string{"admin", "api", "ingestor", "syncer", "chatter"} {
		_, ready, err := start(mode)
		if err != nil {
			return err
		}
		if mode == "admin" || mode == "api" {
			timer := time.NewTimer(5 * time.Minute)
			defer timer.Stop()
			select {
			case <-timer.C:
				return fmt.Errorf("%s startup timed out", mode)
			case err := <-ready:
				if err != nil {
					return fmt.Errorf("%s did not become ready: %w", mode, err)
				}
			case err := <-exits:
				return err
			case <-ctx.Done():
				return nil
			}
		}
	}
	select {
	case err := <-exits:
		return err
	case <-ctx.Done():
		return nil
	}
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

func runService(ctx context.Context, cancel context.CancelFunc, arguments *bootstrap.ServerArgs) error {

	// Initialize local variables (runtime variables from Redis)
	err := server.InitLocalVariables()
	if err != nil {
		fmt.Printf("Failed to start %s server: %v\n", *arguments.Mode, err)
		return err
	}

	serverType := bootstrap.GetServerType(arguments)

	// The final service name is resolved after configuration is loaded.
	serverName := bootstrap.TempServerName(serverType, arguments.Name)

	logLevel := bootstrap.SelectedLogLevel(arguments, "")
	higherThanInfo := common.LogLevelHigherThanInfo(logLevel)
	if higherThanInfo {
		logLevel = "info"
	}

	err = common.InitLogger(logLevel, common.FileOutput{}, serverName)
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}

	// Initialize configuration
	var configPath string
	if arguments.ConfigPath != nil {
		configPath = *arguments.ConfigPath
	}

	if err = server.Init(configPath); err != nil {
		common.Error("Failed to initialize configuration", err)
		return err
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
		return err
	}
	globalConfig.SetLogLevel(resolvedLogLevel)

	var port int
	switch serverType {
	case common.ServerTypeAPI:
		port, err = globalConfig.ResolvePort(globalConfig.GetAPIServerConfig().HTTPPort, arguments.Port)
		if err != nil {
			fmt.Printf("Failed to resolve API server port: %v\n", err)
			return err
		}
		globalConfig.SetAPIServerHTTPPort(port)
	case common.ServerTypeAdmin:
		port, err = globalConfig.ResolvePort(globalConfig.GetAdminServerConfig().HTTPPort, arguments.Port)
		if err != nil {
			fmt.Printf("Failed to resolve admin server port: %v\n", err)
			return err
		}
		globalConfig.SetAdminHTTPPort(port)
	case common.ServerTypeIngestion, common.ServerTypeFileSyncer, common.ServerTypeChatter, common.ServerTypeDeepDocGPU, common.ServerTypeDeepDocAscend, common.ServerTypeDeepDocCPU, common.ServerTypeMigrate:
		// No port embedding; the final name is derived below.
	default:
		return fmt.Errorf("invalid server mode: %v", serverType)
	}

	arguments.Name = globalConfig.GetServerName(*arguments.Mode)
	serverName, err = bootstrap.ResolveServerName(*arguments.Mode, arguments.Name, port)
	if err != nil {
		common.Error("invalid server mode", err)
		return err
	}

	auditConfig := globalConfig.GetAuditConfig()
	err = common.InitAudit(auditConfig.Type, auditConfig.GetFileOutput(), serverName)
	if err != nil {
		common.Error("Failed to initialize audit logger", err)
		return err
	}
	defer func() {
		common.SyncAudit()
	}()

	// set server name and log file path
	server.SetServerName(serverName)

	if arguments.EnablePProf {
		if err := startPProf(pprofAddr()); err != nil {
			common.Error("pprof server failed", err)
		}
	}

	switch serverType {
	case common.ServerTypeAPI, common.ServerTypeIngestion, common.ServerTypeChatter:
		err = deepdocpkg.RegisterNativeDeepDoc(arguments)
		if err != nil {
			common.Error("Failed to register in-process DeepDoc backend", err)
			return err
		}
	case common.ServerTypeDeepDocCPU, common.ServerTypeDeepDocGPU, common.ServerTypeDeepDocAscend:
		err = runDeepDoc(ctx, serverName, arguments)
		if err != nil {
			fmt.Printf("Failed to start DEEPDOC: %v\n", err)
			return err
		}

		return nil
	default:
	}

	// Print all configuration settings
	common.Info(fmt.Sprintf("Starting %v server: %s, mode: %s", serverType, serverName, globalConfig.GetMode()))
	server.PrintAll()

	// Initialize database.
	migrate := serverType == common.ServerTypeMigrate
	if migrate {
		common.Info("Running database migrations")
	}

	err = dao.InitDB(ctx, migrate)
	if err != nil {
		common.Fatal("Failed to initialize database", zap.Error(err))
	}
	if migrate {
		common.Info("Database migrations completed")
		return nil
	}

	err = checkDatabaseVersion(ctx)
	if err != nil {
		common.Fatal("Database has a newer version", zap.Error(err))
	}

	// Initialize doc engine
	err = engine.InitDocEngine(ctx)
	if err != nil {
		common.Fatal("Failed to initialize doc engine", zap.Error(err))
	}
	defer engine.Close()

	// Initialize Kvrocks cache
	err = kvrocks.Init(ctx)
	if err != nil {
		common.Fatal("Failed to initialize Kvrocks", zap.Error(err))
	}
	defer kvrocks.Close()

	err = storage.Init(ctx)
	if err != nil {
		common.Error("Failed to initialize storage factory", err)
	}
	defer storage.CloseStorage()

	err = engine.InitMessageQueue()
	if err != nil {
		common.Fatal("Failed to initialize message queue engine", zap.Error(err))
	}

	err = server.InitVariables(kvrocks.Get())
	if err != nil {
		common.Warn("Failed to initialize server variables from Redis, using defaults", zap.String("error", err.Error()))
	}

	// Billing owns the plan catalogue and the points/quota state machine.
	// Both API and chatter execute conversations subject to billing enforcement.
	if serverType == common.ServerTypeAPI || serverType == common.ServerTypeChatter {
		if err = billing.Init(ctx, server.GetConfig().GetBillingConfig()); err != nil {
			common.Fatal("Failed to initialize billing", zap.Error(err))
		}
		if err = billing.Start(ctx); err != nil {
			common.Fatal("Failed to start billing", zap.Error(err))
		}
	}

	err = server.StartServer(ctx, cancel, serverName, serverType)
	if err != nil {
		common.Error("Failed to start EE server", err)
		return err
	}
	defer server.ShutdownServer(ctx)

	if arguments.Name == nil {
		arguments.Name = &serverName
	}

	switch serverType {
	case common.ServerTypeChatter:
		err = chatterpkg.Run(ctx, cancel, serverName, arguments)
		if err != nil {
			fmt.Printf("Failed to start CHATTER: %v\n", err)
			return err
		}
	case common.ServerTypeAPI:
		err = apipkg.Run(ctx, serverName, arguments)
		if err != nil {
			fmt.Printf("Failed to start API server: %v\n", err)
			return err
		}
	case common.ServerTypeAdmin:
		err = admin.Run(ctx, serverName, arguments)
		if err != nil {
			fmt.Printf("Failed to start ADMIN server: %v\n", err)
			return err
		}
	case common.ServerTypeIngestion:
		err = ingestorpkg.Run(ctx, cancel, serverName, arguments)
		if err != nil {
			fmt.Printf("Failed to start INGESTION worker: %v\n", err)
			return err
		}
	case common.ServerTypeFileSyncer:
		err = syncerpkg.Run(ctx, cancel, serverName, arguments)
		if err != nil {
			fmt.Printf("Failed to start SYNCER: %v\n", err)
			return err
		}
	default:
		return fmt.Errorf("invalid server mode: %v", serverType)
	}
	return nil
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
