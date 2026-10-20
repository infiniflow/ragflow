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

// Single owner for the ingestion-component registration surface.
//
// Plan §2 AD-2 places every component under the same registry as
// the agent canvas; in practice that means cmd entries (server,
// ingestor, ...) must blank-import the ingestion component
// packages so their init() runs. The historical pattern was
// for each cmd entry to repeat the blank imports, which is
// fragile: a new entry that forgets the blank import sees
// "unknown component" at run time.
//
// This package centralises the import list: cmd entries
// blank-import this package, and the Go toolchain evaluates
// the blank imports at link time, triggering each component
// package's init().
//
// LOCATION NOTE: this lives under ingestion/ (not
// ingestion/pipeline/) because pipeline.go imports the
// chunker subpackage, and chunker imports the parent
// ingestion package, which imports pipeline. A wire file
// inside pipeline would close that cycle. By sitting
// alongside pipeline under ingestion/, this package can
// blank-import the component packages without going through
// pipeline.
package wire

import (
	// Component registration: the blank imports trigger each
	// package's init() which calls runtime.DefaultRegistry.Register.
	_ "ragflow/internal/ingestion/component"         // File / Parser / Tokenizer / Extractor
	_ "ragflow/internal/ingestion/component/chunker" // chunker variants
)
