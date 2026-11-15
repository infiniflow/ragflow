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

package bootstrap

// ServerArgs holds the parsed command-line arguments shared by every server
// mode. It lives in this package (rather than package main) so the mode
// runners and the API server wiring can be moved into their own packages and
// still receive the parsed arguments.
type ServerArgs struct {
	Mode          *string // admin | api | ingestor | syncer | deepdoc | migrate
	HelpFlag      bool
	VersionFlag   bool
	LogLevel      *string
	ConfigPath    *string // Used by admin, api; user defined config path
	InitSuperUser bool    // Used by admin;
	Port          *int    // Used by admin, api
	AdminHost     *string // Used by api, ingestor, syncer, deepdoc for heartbeat
	AdminPort     *int    // Used by api, ingestor, syncer, deepdoc for heartbeat, "ip:port"
	Name          *string // server name
	EnablePProf   bool    // enable pprof

	// DeepdocInferenceConcurrency, when set, overrides the DeepDoc inference
	// concurrency from env/config. nil means "unspecified".
	DeepdocInferenceConcurrency *int
	// IngestorConcurrentWorkers, when set, overrides the ingestor worker
	// count (NATS consumer count K) from env/config. nil means "unspecified".
	IngestorConcurrentWorkers *int
	// IngestorPageConcurrency, when set, overrides the per-document page
	// concurrency (N) from env/config. nil means "unspecified".
	IngestorPageConcurrency *int
	// DeepdocInferenceCPUCores, when set, overrides the DeepDoc inference CPU-core
	// budget from env/config. nil means "unspecified" (0 means "all cores").
	DeepdocInferenceCPUCores *int
}

// SelectedLogLevel resolves the effective log level: an explicit --log-level
// (non-nil) wins, otherwise the configured level, otherwise "info".
func SelectedLogLevel(args *ServerArgs, configured string) string {
	if args.LogLevel != nil {
		return *args.LogLevel
	}

	if configured == "" {
		return "info"
	}

	return configured
}
