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
	Mode          *string // admin | api | ingestor | syncer | chatter | deepdoc | migrate
	HelpFlag      *bool   // show help
	VersionFlag   *bool
	LogLevel      *string
	ConfigPath    *string      // Used by admin, api; user defined config path
	InitSuperUser bool         // Used by admin;
	Port          *int         // Used by admin, api
	Name          *string      // server name
	EnablePProf   *bool        // enable pprof
	Ready         func() error // called after service startup

	// DeepdocInferenceConcurrency, when set, overrides the DeepDoc inference
	// concurrency from env/config. nil means "unspecified".
	DeepdocInferenceConcurrency *int
	// DeepdocInferenceCPUCores, when set, overrides the DeepDoc inference CPU-core
	// budget from env/config. nil means "unspecified" (0 means "all cores").
	DeepdocInferenceCPUCores *int
	// DeepdocDevice, when set, selects the DeepDoc inference device
	// (cpu | gpu[:id]). The OSS build accepts only cpu; gpu fails fast at
	// startup with a "not supported" error (GPU is enterprise-only).
	DeepdocDevice *string
	// IngestorTier, when set, restricts which ingestion lanes the ingestor
	// consumes. nil means "unspecified" (default: the lowest-priority lane).
	IngestorTier *string
}

func GetLogLevel(args *ServerArgs) string {
	if args.LogLevel != nil {
		return *args.LogLevel
	}
	return "warn"
}
