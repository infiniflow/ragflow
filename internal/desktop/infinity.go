//go:build desktop && cgo

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

package desktop

// The document engine is Infinity's embedded build
// (infinity/rust_engine/crates/embedded-ffi), linked into this process. It
// serves the ordinary thrift API on a loopback port, so the engine client is
// the regular one with a different address: no second query path exists.
//
// The TCP entry point is deliberate. The library's unix-socket entry point is
// cfg(unix) only, and a desktop binary has to run on Windows as well. A port
// also sidesteps the 104-byte sun_path limit on macOS, which a deep data
// directory can exceed.
//
// The library is always the static archive, staged into
// ../../.cache/infinity-embedded by build.sh --desktop, which compiles the
// engine from an Infinity checkout when needed. The archive is not named here
// because only the script knows where it was staged, and because Go keys the
// final link on the flag string — a stable name would let a rebuilt engine reuse
// the previously linked binary from the build cache.
//
// Nothing is loaded at runtime, so no run path is declared: there is no shared
// object to find, and the desktop binary ships as one file.

/*
#cgo darwin LDFLAGS: -framework Security -framework CoreFoundation -lc++ -liconv -lresolv
#cgo linux LDFLAGS: -lstdc++ -lpthread -ldl -lm
#cgo windows LDFLAGS: -lws2_32 -luserenv -lbcrypt -lntdll

#include <stdlib.h>

// The handle is opaque on both sides, so it is declared `void*` here: cgo maps
// `void*` to unsafe.Pointer consistently, whereas a `void` typedef would map the
// return value and the parameter of inf_embedded_stop to different Go types.
extern void* inf_embedded_start_tcp(const char* data_dir, const char* host, char** err);
extern int inf_embedded_stop(void* h);
extern const char* inf_embedded_addr(const void* h);
extern void inf_embedded_free(char* s);
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"

	"go.uber.org/zap"

	"ragflow/internal/common"
)

// embeddedInfinity owns one in-process document engine.
type embeddedInfinity struct {
	handle unsafe.Pointer
	addr   string
}

// startEmbeddedInfinity opens the engine on dataDir and serves it on a loopback
// port chosen by the OS. The engine is ready for connections when this returns.
func startEmbeddedInfinity(dataDir string) (*embeddedInfinity, error) {
	// The engine's text analyzers load their dictionaries from the same tree the
	// Go tokenizer uses. The Rust side resolves it through INFINITY_RESOURCE_PATH
	// (analyzer::resource_path), and the two must agree: a full-text index built
	// with a different dictionary set cannot be queried with this one. Setting it
	// here rather than in the shell also covers running the binary directly.
	if dir := common.ResourceDir(); dir != "" {
		if err := os.Setenv("INFINITY_RESOURCE_PATH", dir); err != nil {
			return nil, fmt.Errorf("desktop: export INFINITY_RESOURCE_PATH=%s: %w", dir, err)
		}
		common.Info("Document engine shares the analyzer resource tree",
			zap.String("path", dir))
	}

	cDataDir := C.CString(dataDir)
	defer C.free(unsafe.Pointer(cDataDir))

	var cErr *C.char
	handle := C.inf_embedded_start_tcp(cDataDir, nil, &cErr)
	if handle == nil {
		message := "unknown error"
		if cErr != nil {
			message = C.GoString(cErr)
			C.inf_embedded_free(cErr)
		}
		return nil, fmt.Errorf("desktop: start document engine in %s: %s", dataDir, message)
	}
	return &embeddedInfinity{
		handle: handle,
		addr:   C.GoString(C.inf_embedded_addr(handle)),
	}, nil
}

// Addr is the loopback "host:port" the engine serves thrift on.
func (e *embeddedInfinity) Addr() string { return e.addr }

// Close stops the engine and waits for its checkpoint to finish. The data
// directory is safe to reopen once it returns.
func (e *embeddedInfinity) Close() error {
	if e.handle == nil {
		return nil
	}
	C.inf_embedded_stop(e.handle)
	e.handle = nil
	return nil
}
