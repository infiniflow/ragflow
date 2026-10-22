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

// Package bootstrap holds the pure, side-effect-free helpers that the server
// entrypoint uses to derive its identity before the mode-specific runners take
// over. Keeping them here lets cmd/ragflow_server.go stay a thin dispatcher and
// makes the naming rules unit-testable without booting any subsystem.
package bootstrap

import (
	"fmt"
	"ragflow/internal/common"
)

// TempServerName returns the placeholder server name used for the temporary,
// stdout-only logger that exists before the configuration is loaded. An
// explicit --name (non-nil pointer) wins; otherwise the name is
// "<mode>_server".
//
// This is intentionally distinct from ResolveServerName: the final name for
// api/admin additionally embeds the resolved HTTP port, which is only known
// after the configuration is initialized.
func TempServerName(serverType common.ServerType, nameFlag *string) string {
	if nameFlag != nil {
		return *nameFlag
	}
	return string(serverType) + "_server"
}

// ResolveServerName derives the final server name for a mode, preserving the
// precedence and per-mode rules that cmd/ragflow_server.go previously inlined:
//
//   - An explicit --name (non-nil pointer, including an empty string) always
//     wins.
//   - api/admin embed the resolved HTTP port: "<mode>_server_<port>".
//   - ingestor/syncer/deepdoc fall back to the "<mode>_server" placeholder
//     (the port is not part of their name).
//   - migrate uses the fixed literal "migrate".
//
// An unrecognized mode is a hard error, independent of whether a name was
// supplied.
func ResolveServerName(mode string, nameFlag *string, port int) (string, error) {
	switch mode {
	case "api", "admin":
		if nameFlag != nil {
			return *nameFlag, nil
		}
		return fmt.Sprintf("%s_server_%d", mode, port), nil
	case "ingestor", "syncer", "deepdoc":
		if nameFlag != nil {
			return *nameFlag, nil
		}
		return mode + "_server", nil
	case "migrate":
		if nameFlag != nil {
			return *nameFlag, nil
		}
		return "migrate", nil
	default:
		return "", fmt.Errorf("unsupported server mode %q", mode)
	}
}

func GetServerType(args *ServerArgs) common.ServerType {
	switch *args.Mode {
	case "api":
		return common.ServerTypeAPI
	case "admin":
		return common.ServerTypeAdmin
	case "ingestor":
		return common.ServerTypeIngestion
	case "syncer":
		return common.ServerTypeFileSyncer
	case "deepdoc":
		return common.ServerTypeDeepDocCPU
	case "migrate":
		return common.ServerTypeMigrate
	default:
		return common.ServerTypeUnknown
	}
}
