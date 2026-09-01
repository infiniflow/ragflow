//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//
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

// Package servermode holds server-mode policy decisions that do not depend on
// the cgo-linked DeepDoc native backend, so they stay unit-testable without the
// native static libraries.
package servermode

// NeedsDeepDoc reports whether a server mode must register and serve the
// in-process (Go) DeepDoc backend at startup.
//
// Only the modes that actually run the document parsing pipeline need it:
//   - "api":      serves the dataflow debug endpoint, which parses uploaded
//     files in-process, and may parse via other api routes.
//   - "ingestor": runs the document ingestion/parsing pipeline.
//
// "admin" (management UI) and "syncer" (datasource sync) never instantiate the
// DeepDoc analyzer, so they must not fail-fast when ORT + models are absent.
func NeedsDeepDoc(mode string) bool {
	switch mode {
	case "api", "ingestor":
		return true
	default:
		return false
	}
}
