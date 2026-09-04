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

// Package cmdargs is the standalone argv parser for the ragflow_server
// CLI. It is intentionally CGO-free so unit tests can drive the parser
// without the office_oxide / pdfium / pdf_oxide native libraries that
// the rest of the cmd/ tree depends on. See #19270 for the
// `--migrate` standalone-mode contract.
package cmdargs

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ServerArgs holds the parsed shape of the ragflow_server argv.
type ServerArgs struct {
	Mode          *string // admin | api | ingestor | syncer | migrate
	HelpFlag      bool
	VersionFlag   bool
	DebugLog      bool
	MigrateDB     bool
	ConfigPath    *string // Used by admin, api; user defined config path
	InitSuperUser bool    // Used by admin;
	Port          *int    // Used by admin, api
	AdminHost     *string // Used by api, ingestor, syncer for heartbeat
	AdminPort     *int    // Used by api, ingestor, syncer for heartbeat, "ip:port"
	Name          *string // server name
}

// Parse is the production entry point: it parses os.Args via ParseFrom.
func Parse() (*ServerArgs, error) {
	return ParseFrom(os.Args)
}

// ParseFrom is the testable form of the argv parser: it takes the argv
// slice explicitly so unit tests can drive the parser without mutating
// os.Args. The shape of the parsing matches the original parseArgs in
// cmd/ragflow_server.go exactly; only the input source is parameterised.
// See #19270 for the `--migrate` standalone-mode semantics.
func ParseFrom(argv []string) (*ServerArgs, error) {
	args := &ServerArgs{}

	var serverMode string
	var configPath string
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		switch arg {
		case "--admin":
			serverMode = "admin"
			args.Mode = &serverMode
		case "--migrate":
			// `--migrate` is a standalone one-shot mode. It is mutually
			// exclusive with --admin/--api/--ingestor/--syncer because it
			// runs the database migration and exits 0/non-0 instead of
			// starting an HTTP daemon. See #19270.
			args.MigrateDB = true
		case "--ingestor":
			serverMode = "ingestor"
			args.Mode = &serverMode
		case "--api":
			serverMode = "api"
			args.Mode = &serverMode
		case "--syncer":
			serverMode = "syncer"
			args.Mode = &serverMode
		case "-h", "--help":
			args.HelpFlag = true
		case "-v", "--version":
			args.VersionFlag = true
		case "--debug":
			args.DebugLog = true
		case "-f", "--config":
			if i+1 >= len(argv) {
				return nil, fmt.Errorf("%s requires a value", arg)
			}
			i++
			configPath = argv[i]
			args.ConfigPath = &configPath
		case "--init-superuser":
			args.InitSuperUser = true
		case "-p", "--port":
			if i+1 >= len(argv) {
				return nil, errors.New("--port requires a value")
			}
			i++
			port, convErr := strconv.Atoi(argv[i])
			if convErr != nil {
				return nil, fmt.Errorf("invalid port: %w", convErr)
			}
			args.Port = &port
			if port <= 0 || port > 65535 {
				return nil, fmt.Errorf("invalid port: %d", port)
			}
		case "--admin-host":
			if i+1 >= len(argv) {
				return nil, errors.New("--admin-host requires a value")
			}
			i++
			parts := strings.SplitN(argv[i], ":", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return nil, errors.New("--admin-host must be in the form 'ip:port'")
			}
			ip, portStr := parts[0], parts[1]
			port, convErr := strconv.Atoi(portStr)
			if convErr != nil {
				return nil, fmt.Errorf("failed to parse admin port: %w", convErr)
			}
			args.AdminHost = &ip
			args.AdminPort = &port
		case "--name":
			if i+1 >= len(argv) {
				return nil, errors.New("--name requires a value")
			}
			i++
			args.Name = &argv[i]
		default:
			return nil, fmt.Errorf("unknown parameter: %s", arg)
		}
	}

	// Resolve `--migrate` into the standalone migrate mode. `--migrate` is
	// mutually exclusive with --admin/--api/--ingestor/--syncer: when the
	// user passes `--migrate` alone the binary runs the database migration
	// and exits (code 0 on success, non-zero on failure) without starting
	// any HTTP daemon. See #19270.
	if args.MigrateDB {
		if args.Mode != nil {
			return nil, fmt.Errorf("--migrate is a standalone mode and cannot be combined with --%s", *args.Mode)
		}
		mode := "migrate"
		args.Mode = &mode
	}
	return args, nil
}
