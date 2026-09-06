#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$SCRIPT_DIR"

# Build directories
CPP_DIR="$PROJECT_ROOT/internal/binding/cpp"
BUILD_DIR="$CPP_DIR/cmake-build-release"
RAGFLOW_SERVER_BINARY="$PROJECT_ROOT/bin/ragflow_server"
RAGFLOW_CLI_BINARY="$PROJECT_ROOT/bin/ragflow-cli"

# Strip symbols from Go binaries (set via --strip / -s)
STRIP_SYMBOLS=""

# Native static library settings. These are the user-cache paths (~/ragflow-native-libs/).
# If /opt/ragflow-native-libs/ exists (pre-seeded in CI runner image), it takes priority
# and skips the network (download_deps.py) fallback.
SYSTEM_DEPS="/opt/ragflow-native-libs"

# office_oxide native library settings — static linking
OFFICE_OXIDE_PREFIX="${HOME}/ragflow-native-libs/office_oxide"
OFFICE_OXIDE_VERSION="0.1.9"

# pdfium native library settings — static linking (kognitos/pdfium-static)
PDFIUM_STATIC_PREFIX="${HOME}/ragflow-native-libs/pdfium-static"
PDFIUM_STATIC_VERSION="7809"

# pdf_oxide native library settings — static linking (go-ffi tarball)
PDF_OXIDE_PREFIX="${HOME}/ragflow-native-libs/pdf_oxide"
PDF_OXIDE_VERSION="0.3.73"

# onnxruntime native library settings — static linking for the in-process
# (Go) DeepDoc backend. libonnxruntime*.a is linked into the server binary
# (--whole-archive + --export-dynamic); OrtGetApiBase is then resolved via
# dlopen(self), so no libonnxruntime.so is needed at runtime. Downloaded by
# ragflow_deps/download_go_deps.py (and ragflow_deps/download_deps.py) into
# onnxruntime/static_lib.
ONNXRUNTIME_STATIC_PREFIX="${HOME}/ragflow-native-libs/onnxruntime/static_lib"

# Copy a dependency from the system pre-seed directory to the user cache.
# Returns 0 if the dep was copied or already exists in cache, 1 otherwise.
_seed_from_system() {
    local dep_name="$1"  # e.g. "pdfium-static", "pdf_oxide", "office_oxide"
    local dep_dir="${HOME}/ragflow-native-libs/${dep_name}"
    local sys_dir="${SYSTEM_DEPS}/${dep_name}"

    echo "check if dep ${dep_name} exists in ${dep_dir} or ${sys_dir}"

    if [ -d "$dep_dir" ]; then
        echo "  ${dep_name} → ${dep_dir} (user cache)"
        return 0  # already cached
    fi
    if [ -d "$sys_dir" ]; then
        echo "  ${dep_name} → ${sys_dir} (system)"
        mkdir -p "$(dirname "$dep_dir")"
        cp -r "$sys_dir" "$dep_dir"
        return 0
    fi
    echo "  ${dep_name} not found in system or user cache"
    return 1
}

echo -e "${GREEN}=== RAGFlow Go Server Build Script ===${NC}"

# Function to print section headers
print_section() {
    echo -e "\n${YELLOW}>>> $1${NC}"
}

# Detect the package-install command for pcre2 development files.
# Outputs the command on stdout; empty string if no supported manager is found.
detect_pcre2_install_cmd() {
    if [ "$(uname)" = "Darwin" ]; then
        echo "brew install pcre2"
    elif command -v apt-get >/dev/null 2>&1; then
        echo "sudo apt-get install -y libpcre2-dev"
    elif command -v zypper >/dev/null 2>&1; then
        echo "sudo zypper install -y pcre2-devel"
    elif command -v dnf >/dev/null 2>&1; then
        echo "sudo dnf install -y pcre2-devel"
    elif command -v pacman >/dev/null 2>&1; then
        echo "sudo pacman -S --noconfirm pcre2"
    else
        echo ""
    fi
}

# Check whether libpcre2-8 is available (static or shared).
check_pcre2() {
    # Prefer pkg-config when available — works across distros.
    if command -v pkg-config >/dev/null 2>&1 && pkg-config --exists libpcre2-8; then
        return 0
    fi
    # Fall back to known library paths:
    #   Debian/Ubuntu  -> /usr/lib/x86_64-linux-gnu
    #   openSUSE/RHEL  -> /usr/lib64
    #   generic Linux  -> /usr/lib, /usr/local/lib
    #   macOS Homebrew -> /opt/homebrew/lib (Apple Silicon), /usr/local/lib (Intel)
    for p in \
        /usr/lib/x86_64-linux-gnu/libpcre2-8.a \
        /usr/lib/x86_64-linux-gnu/libpcre2-8.so \
        /usr/lib64/libpcre2-8.a \
        /usr/lib64/libpcre2-8.so \
        /usr/lib/libpcre2-8.a \
        /usr/lib/libpcre2-8.so \
        /usr/local/lib/libpcre2-8.a \
        /usr/local/lib/libpcre2-8.so \
        /usr/local/lib/libpcre2-8.dylib \
        /opt/homebrew/lib/libpcre2-8.a \
        /opt/homebrew/lib/libpcre2-8.dylib; do
        [ -f "$p" ] && return 0
    done
    return 1
}

# Check dependencies
check_cpp_deps() {
    print_section "Checking c++ dependencies"

    command -v cmake >/dev/null 2>&1 || { echo -e "${RED}Error: cmake is required but not installed.${NC}"; exit 1; }
    command -v clang++ >/dev/null 2>&1 || { echo -e "${RED}Error: clang++ is required but not installed.${NC}"; exit 1; }

    if check_pcre2; then
        echo "✓ pcre2 library found"
    else
        install_cmd="$(detect_pcre2_install_cmd)"
        echo -e "${YELLOW}Warning: libpcre2-8 not found. You may need to install it:${NC}"
        if [ -n "$install_cmd" ]; then
            echo "  $install_cmd"
        else
            echo "  (No supported package manager detected — install pcre2 development files manually)"
        fi
    fi

    echo "✓ Required tools are available"
}

check_go_deps() {
    print_section "Checking go dependencies"

    command -v go >/dev/null 2>&1 || { echo -e "${RED}Error: go is required but not installed.${NC}"; exit 1; }

    echo "✓ Required tools are available"
}

# Check office_oxide native library
check_office_oxide_deps() {
    print_section "Checking office_oxide native library"
    _seed_from_system "office_oxide" || true

    local lib_file="liboffice_oxide.a"
    local lib_path="${OFFICE_OXIDE_PREFIX}/lib/${lib_file}"
    local header_path="${OFFICE_OXIDE_PREFIX}/include/office_oxide_c/office_oxide.h"

    if [ ! -f "$lib_path" ] || [ ! -f "$header_path" ]; then
        echo -e "${RED}Error: office_oxide native library not found${NC}"
        echo "  Expected: ${lib_path}"
        echo "  Run: uv run python3 ragflow_deps/download_go_deps.py"
        echo "  Or manually download: https://github.com/yfedoseev/office_oxide/releases/download/v${OFFICE_OXIDE_VERSION}/native-linux-x86_64.tar.gz"
        exit 1
    fi

    # Verify the on-disk lib matches the pinned version. A stale older lib
    # (e.g. v0.1.7) silently reintroduces the PPT97 content-loss bug
    # (github.com/yfedoseev/office_oxide#85, fixed in v0.1.8): PlainText()
    # on a legacy .ppt returns stale metadata instead of slide text.
    if ! strings "$lib_path" 2>/dev/null | grep -Fxq "$OFFICE_OXIDE_VERSION"; then
        local found_version
        found_version=$(strings "$lib_path" 2>/dev/null | grep -E "^0\.[0-9]+\.[0-9]+$" | head -1)
        echo -e "${RED}Error: office_oxide native lib version mismatch${NC}"
        echo "  Required: v${OFFICE_OXIDE_VERSION}; found: ${found_version:-unknown}"
        echo "  A stale lib silently loses PPT97 (.ppt) slide content. Refresh:"
        echo "    rm -rf ~/ragflow-native-libs/office_oxide ragflow_deps/office_oxide-linux-x86_64.tar.gz"
        echo "    uv run python3 ragflow_deps/download_go_deps.py"
        exit 1
    fi

    echo "✓ office_oxide v${OFFICE_OXIDE_VERSION} native library found at ${OFFICE_OXIDE_PREFIX}"
    return 0
}

# Check pdfium static library.
check_pdfium_deps() {
    _seed_from_system "pdfium-static" || true
    local lib_path="${PDFIUM_STATIC_PREFIX}/lib/libpdfium.a"

    if [ -f "$lib_path" ]; then
        echo "  pdfium (static) → ${PDFIUM_STATIC_PREFIX}"
        return 0
    fi

    echo "  pdfium (static) not found"
    echo "  Expected: ${lib_path}"
    echo "  Run: uv run python3 ragflow_deps/download_go_deps.py"
    echo "  Or: curl -fsSL https://github.com/kognitos/pdfium-static/releases/download/chromium%2F${PDFIUM_STATIC_VERSION}/pdfium-linux-x64-static.tgz | tar xz -C ${PDFIUM_STATIC_PREFIX}"
    return 1
}

# Check pdf_oxide static library.
check_pdf_oxide_deps() {
    _seed_from_system "pdf_oxide" || true
    # Map platform to tarball-internal subdirectory.
    local platform_subdir
    case "$(uname -s)" in
        Linux)
            case "$(uname -m)" in
                x86_64)  platform_subdir="linux_amd64" ;;
                aarch64|arm64) platform_subdir="linux_arm64" ;;
                *) echo "  pdf_oxide (static) → unsupported arch"; return 1 ;;
            esac
            ;;
        Darwin)
            case "$(uname -m)" in
                x86_64)  platform_subdir="darwin_amd64" ;;
                arm64)   platform_subdir="darwin_arm64" ;;
                *) echo "  pdf_oxide (static) → unsupported arch"; return 1 ;;
            esac
            ;;
        *) echo "  pdf_oxide (static) → unsupported OS"; return 1 ;;
    esac

    local lib_path="${PDF_OXIDE_PREFIX}/lib/${platform_subdir}/libpdf_oxide.a"

    if [ -f "$lib_path" ]; then
        # Verify the on-disk lib matches the pinned version. _seed_from_system
        # accepts an existing user-cache directory without inspecting it, so a
        # lib left over from an earlier pin is reused silently and the upgrade
        # becomes a no-op.
        #
        # The marker here differs from office_oxide: instead of a standalone
        # "0.1.9" line it is "pdf_oxide <version>" merged into Rust's string
        # constant pool, so a whole-line match cannot be used. Extract the
        # version and compare it exactly — a substring match would let a pin of
        # "0.3.7" accept a 0.3.73 lib. The "pdf_oxide " prefix keeps bare
        # version numbers of vendored dependencies out of the match.
        local found_version
        found_version=$(strings "$lib_path" 2>/dev/null \
            | grep -oE "pdf_oxide [0-9]+\.[0-9]+\.[0-9]+" | head -1 | cut -d' ' -f2)
        if [ "$found_version" != "$PDF_OXIDE_VERSION" ]; then
            echo -e "${RED}Error: pdf_oxide native lib version mismatch${NC}"
            echo "  Required: v${PDF_OXIDE_VERSION}; found: ${found_version:-unknown}"
            echo "  A stale lib silently reverts PDF parsing fixes. Refresh:"
            echo "    rm -rf ${PDF_OXIDE_PREFIX} ragflow_deps/pdf_oxide-go-ffi-linux-amd64.tar.gz"
            echo "    uv run python3 ragflow_deps/download_go_deps.py"
            return 1
        fi
        echo "  pdf_oxide (static) → ${PDF_OXIDE_PREFIX}"
        return 0
    fi

    echo "  pdf_oxide (static) not found"
    echo "  Expected: ${lib_path}"
    echo "  Run: uv run python3 ragflow_deps/download_go_deps.py"
    echo "  Or: curl -fsSL https://github.com/yfedoseev/pdf_oxide/releases/download/v${PDF_OXIDE_VERSION}/pdf_oxide-go-ffi-linux-amd64.tar.gz | tar xz -C ${PDF_OXIDE_PREFIX}"
    return 1
}

# Build C++ static library
build_cpp() {
    print_section "Building C++ static library"

    mkdir -p "$BUILD_DIR"
    cd "$BUILD_DIR"

    echo "Running cmake..."
    cmake .. -DCMAKE_BUILD_TYPE=Release

    echo "Building librag_tokenizer_c_api.a..."
    local jobs
    jobs="$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1)"
    make rag_tokenizer_c_api -j"$jobs"

    if [ ! -f "$BUILD_DIR/librag_tokenizer_c_api.a" ]; then
        echo -e "${RED}Error: Failed to build C++ static library${NC}"
        exit 1
    fi

    # Rename the tokenizer's bundled re2 symbols into a private namespace so they
    # never collide with onnxruntime's copy of re2 at final link time.
    #
    # Why this is needed: onnxruntime.a and librag_tokenizer_c_api.a both embed a
    # copy of re2, but they are built against different toolchains/libstdc++ and
    # are ABI-incompatible. Symbol-localization approaches (--exclude-libs, linker
    # version scripts) cannot fix it: both re2 copies must live in the same global
    # symbol table, and the linker resolves the tokenizer's re2 references to
    # whichever copy was archived first (here, onnxruntime's), regardless of which
    # copy is localized. SIGSEGV in re2::DFA::InlinedSearchLoop is the symptom.
    #
    # Renaming the tokenizer's re2 symbols into a private prefix gives each copy
    # its own namespace, so the tokenizer calls its own ABI-compatible re2 and
    # onnxruntime keeps calling its own. Verified to eliminate the crash.
    local tok_a="$BUILD_DIR/librag_tokenizer_c_api.a"
    local rename_map="$BUILD_DIR/re2_rename.map"
    if command -v objcopy >/dev/null 2>&1; then
        nm "$tok_a" \
            | awk '$2 ~ /^[TDBRWtdbrwiIVv]$/ && $3 ~ /^_ZN3re2|_ZNK3re2|_ZTVN3re2|_ZTIN3re2|_ZTSN3re2/ { print $3" ragtokre2_"$3 }' \
            | sort -u > "$rename_map"
        if [ -s "$rename_map" ]; then
            objcopy --redefine-syms="$rename_map" "$tok_a"
            echo -e "${GREEN}✓ Renamed $(wc -l < "$rename_map") tokenizer re2 symbols into private namespace (ragtokre2_)${NC}"
        fi
    else
        echo -e "${YELLOW}Warning: objcopy not found, skipping re2 symbol rename (re2 collision with onnxruntime may cause SIGSEGV)${NC}"
    fi

    echo -e "${GREEN}✓ C++ static library built successfully${NC}"
}

# Build C++ test executable
build_cpp_test() {
    print_section "Building C++ test executable"

    if [ ! -d "$BUILD_DIR" ]; then
        echo "Build directory not found, running cmake first..."
        mkdir -p "$BUILD_DIR"
        cd "$BUILD_DIR"
        cmake .. -DCMAKE_BUILD_TYPE=Release
    else
        cd "$BUILD_DIR"
    fi

    echo "Building rag_analyzer_c_test..."
    local jobs
    jobs="$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1)"
    make rag_analyzer_c_test -j"$jobs"

    if [ ! -f "$BUILD_DIR/rag_analyzer_c_test" ]; then
        echo -e "${RED}Error: Failed to build rag_analyzer_c_test${NC}"
        exit 1
    fi

    echo -e "${GREEN}✓ C++ test executable built successfully: $BUILD_DIR/rag_analyzer_c_test${NC}"
}

# Build Go server
build_go() {
    print_section "Building RAGFlow go"

    cd "$PROJECT_ROOT"

    # Check if C++ library exists
    if [ ! -f "$BUILD_DIR/librag_tokenizer_c_api.a" ]; then
        echo -e "${RED}Error: C++ static library not found. Run with --cpp first.${NC}"
        exit 1
    fi

    if check_pcre2; then
        echo "✓ pcre2 library found"
    else
        install_cmd="$(detect_pcre2_install_cmd)"
        if [ -z "$install_cmd" ]; then
            echo -e "${RED}Error: libpcre2-8 not found and no supported package manager detected.${NC}"
            echo "Please install pcre2 development files manually."
            exit 1
        fi
        if [ "$(uname)" = "Darwin" ]; then
            echo -e "${RED}Error: libpcre2-8 not found. Install with: $install_cmd${NC}"
            exit 1
        fi
        echo -e "${YELLOW}Warning: libpcre2-8 not found. Installing with: $install_cmd${NC}"
        eval "$install_cmd"
    fi

    setup_cgo_env

    # The in-process (Go) DeepDoc backend is statically linked against ONNX
    # Runtime (--whole-archive + -Wl,--export-dynamic, see setup_cgo_env). The
    # forked onnxruntime_go binding resolves OrtGetApiBase only at RUNTIME via
    # dlopen(NULL), so a missing ORT archive still lets `go build` SUCCEED and
    # yields a server binary that FAILS AT STARTUP with a fatal
    # "no in-process DeepDoc backend serving". That is exactly the breakage
    # colleagues hit when ORT was not present at build time and setup_cgo_env
    # silently skipped it. Fail the build HERE instead of deferring the breakage
    # to runtime: a server binary without ORT is unusable, not a degraded one.
    if ! printf '%s' "$CGO_LDFLAGS" | grep -q 'libonnxruntime'; then
        echo -e "${RED}Error: ONNX Runtime static libraries are not linked.${NC}" >&2
        echo "  The in-process DeepDoc backend requires libonnxruntime.a to be" >&2
        echo "  statically linked into ragflow_server (--whole-archive +" >&2
        echo "  -Wl,--export-dynamic). Without it the binary compiles but dies" >&2
        echo "  at startup with a fatal 'no in-process DeepDoc backend serving'." >&2
        echo "  Fetch the static libs with:" >&2
        echo "    uv run python3 ragflow_deps/download_go_deps.py" >&2
        echo "  or pre-seed them at /opt/ragflow-native-libs/onnxruntime (CI image)." >&2
        echo "  This production binary must statically include ORT — there is no" >&2
        echo "  ORT-free build path. ORT ends up unlinked only via one of:" >&2
        echo "    - the ORT static_lib dir was never seeded: run" >&2
        echo "      'uv run python3 ragflow_deps/download_go_deps.py', or pre-seed" >&2
        echo "      /opt/ragflow-native-libs/onnxruntime as the CI image does;" >&2
        echo "    - setup_cgo_env did not add libonnxruntime to CGO_LDFLAGS." >&2
        return 1
    fi

    local strip_flags=()
    [ -n "$STRIP_SYMBOLS" ] && strip_flags=(-ldflags="-s -w")

    echo "Building RAGFlow binary: $RAGFLOW_CLI_BINARY and $RAGFLOW_SERVER_BINARY"
    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} CGO_ENABLED=1 \
        go build -tags cgo,static "${strip_flags[@]}" -o "$RAGFLOW_CLI_BINARY" cmd/ragflow-cli.go

    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} CGO_ENABLED=1 \
        CGO_CFLAGS="$CGO_CFLAGS" CGO_LDFLAGS="$CGO_LDFLAGS" \
        go build -tags cgo,static "${strip_flags[@]}" -o "$RAGFLOW_SERVER_BINARY" \
        cmd/ragflow_server.go


    if [ ! -f "$RAGFLOW_SERVER_BINARY" ]; then
        echo -e "${RED}Error: Failed to build RAGFlow main binary${NC}"
        exit 1
    fi

    echo -e "${GREEN}✓ Go ragflow-cli built successfully: $RAGFLOW_CLI_BINARY${NC}"
    echo -e "${GREEN}✓ Go ragflow_server built successfully: $RAGFLOW_SERVER_BINARY${NC}"
}

# Configure CGO flags for native libraries (office_oxide, pdfium, pdf_oxide).
# All three are statically linked — no LD_LIBRARY_PATH or -Wl,-rpath needed.
setup_cgo_env() {
    # ── office_oxide ──────────────────────────────────────────────────
    check_office_oxide_deps

    # Go's build cache keys CGO_LDFLAGS as a string — it does NOT hash the
    # file content of referenced .a archives. So swapping the .a in-place
    # (same path) does NOT invalidate the cache, and `go build` silently
    # reuses a stale binary linked against the old .a.
    #
    # Create a version-stamped symlink directory so the flag string
    # includes the actual linked version. When the .a is upgraded, the
    # path changes → Go cache key changes → automatic relink.
    local office_oxide_lib_dir="${OFFICE_OXIDE_PREFIX}/lib"
    local versioned_lib_dir="${office_oxide_lib_dir}/v${OFFICE_OXIDE_VERSION}"
    mkdir -p "$versioned_lib_dir"
    ln -sf "${office_oxide_lib_dir}/liboffice_oxide.a" \
        "${versioned_lib_dir}/liboffice_oxide.a"

    export CGO_CFLAGS="-I${OFFICE_OXIDE_PREFIX}/include/office_oxide_c${CGO_CFLAGS:+ $CGO_CFLAGS}"
    export CGO_LDFLAGS="${versioned_lib_dir}/liboffice_oxide.a"

    # ── pdfium ────────────────────────────────────────────────────────
    check_pdfium_deps || return 1
    export CGO_LDFLAGS="$CGO_LDFLAGS ${PDFIUM_STATIC_PREFIX}/lib/libpdfium.a"
    # Linux: Chromium-built objects use Clang's .eh_frame format which GNU ld
    # cannot merge. Use lld (LLVM linker) which handles them correctly.
    # --allow-multiple-definition: pdf_oxide and office_oxide are both Rust
    # staticlibs that embed the Rust runtime; linking them together produces
    # duplicate rust_eh_personality / compiler-rt builtins. Those duplicates are
    # ABI-IDENTICAL, so merging them is benign.
    if [ "$(uname -s)" = "Linux" ]; then
        if ! command -v ld.lld >/dev/null 2>&1; then
            echo -e "${RED}Error: ld.lld not found. Install with: sudo apt install lld-20 && sudo ln -s /usr/bin/ld.lld-20 /usr/bin/ld.lld${NC}"
            echo "  lld is required to static-link Chromium-built pdfium (.eh_frame format)"
            return 1
        fi
        export CGO_LDFLAGS="$CGO_LDFLAGS \
            ${PDFIUM_STATIC_PREFIX}/lib/libc++.a \
            ${PDFIUM_STATIC_PREFIX}/lib/libc++abi.a \
            -fuse-ld=lld -Wl,--allow-multiple-definition"
        # The re2 regex-library collision between onnxruntime.a and
        # librag_tokenizer_c_api.a is fixed at the .a level in build_cpp():
        # the tokenizer's bundled re2 symbols are renamed into a private
        # namespace (ragtokre2_) so the two re2 copies never share a symbol.
        # See build_cpp() for details.
    fi

    # ── pdf_oxide ─────────────────────────────────────────────────────
    check_pdf_oxide_deps || return 1
    # The go-ffi tarball places the .a under lib/<platform_subdir>/.
    local pdf_oxide_subdir
    case "$(uname -s)" in
        Linux)
            case "$(uname -m)" in
                x86_64)  pdf_oxide_subdir="linux_amd64" ;;
                aarch64|arm64) pdf_oxide_subdir="linux_arm64" ;;
                *) echo "pdf_oxide: unsupported arch"; return 1 ;;
            esac
            ;;
        Darwin)
            case "$(uname -m)" in
                x86_64)  pdf_oxide_subdir="darwin_amd64" ;;
                arm64)   pdf_oxide_subdir="darwin_arm64" ;;
                *) echo "pdf_oxide: unsupported arch"; return 1 ;;
            esac
            ;;
    esac
    # Version-stamp the archive path so an in-place .a upgrade invalidates
    # Go's build cache. See the office_oxide block above for why the raw path
    # is not enough: the cache key hashes CGO_LDFLAGS as a string, not the
    # contents of the archives it names.
    local pdf_oxide_lib_dir="${PDF_OXIDE_PREFIX}/lib/${pdf_oxide_subdir}"
    local pdf_oxide_versioned_dir="${pdf_oxide_lib_dir}/v${PDF_OXIDE_VERSION}"
    mkdir -p "$pdf_oxide_versioned_dir"
    ln -sf "${pdf_oxide_lib_dir}/libpdf_oxide.a" \
        "${pdf_oxide_versioned_dir}/libpdf_oxide.a"
    export CGO_LDFLAGS="$CGO_LDFLAGS ${pdf_oxide_versioned_dir}/libpdf_oxide.a"

    # ── onnxruntime (static, resolved via dlopen(NULL)) ────────────────
    # macOS native builds of the in-process DeepDoc backend are not supported:
    # ONNX Runtime is statically linked with GNU ld flags (--whole-archive /
    # --export-dynamic) and resolved at runtime via dlopen(NULL); Apple's ld64
    # does not understand these flags. Build on Linux or cross-compile there.
    case "$(uname -s)" in
        Darwin)
            echo "Error: macOS native build of the in-process DeepDoc backend is not supported." >&2
            echo "  ONNX Runtime is linked with GNU ld flags (--whole-archive / --export-dynamic)" >&2
            echo "  and resolved via dlopen(NULL); Apple's ld64 does not support them. Build on Linux." >&2
            return 1
            ;;
    esac
    # Statically link libonnxruntime*.a into the binary. The org Go binding
    # (onnxruntime_go, github.com/infiniflow/onnxruntime_go) resolves
    # OrtGetApiBase with dlopen(NULL), so the symbols must (a) be pulled in
    # wholesale with --whole-archive (ORT registers its execution providers
    # lazily at runtime, beyond what a normal link would keep) and (b) be
    # exported with --export-dynamic so the process-global symbol table
    # dlopen finds them. No libonnxruntime.so is required or supported at
    # runtime; there is no dynamic .so fallback.
    #
    # Seed the static ORT archives from the system pre-bake (/opt, laid down
    # by the CI runner image) into the user cache before the link check
    # below. Mirrors the existing _seed_from_system calls for the other
    # native libs so CI never downloads ORT at build/test time.
    _seed_from_system "onnxruntime" || true
    if [ -d "$ONNXRUNTIME_STATIC_PREFIX" ]; then
        # Collect every .a, but skip GPU-only providers we never build
        # against (would pull in CUDA/cuDNN/TensorRT which we don't ship).
        local ort_a=""
        local seen_version_dir=""
        while IFS= read -r f; do
            case "$(basename "$f")" in
                *cuda*|*tensorrt*|*coreml*|*dml*|*migraphx*) continue ;;
            esac
            # Guard against coexisting stale version dirs: if .a files span
            # more than one onnxruntime-linux-x64-static_lib-* dir, fail fast
            # instead of silently linking two ORT versions (duplicate symbols
            # / wrong version). Re-run `download_deps.py` to prune stale dirs
            # after a version bump, or remove the old dir by hand.
            case "$f" in
                */onnxruntime-linux-x64-static_lib-*/lib/*.a)
                    local vdir="${f#*/onnxruntime-linux-x64-static_lib-}"
                    vdir="${vdir%%/*}"
                    if [ -z "$seen_version_dir" ]; then
                        seen_version_dir="$vdir"
                    elif [ "$seen_version_dir" != "$vdir" ]; then
                        echo "  Error: multiple ONNX Runtime versions found under $ONNXRUNTIME_STATIC_PREFIX" >&2
                        echo "    $seen_version_dir  AND  $vdir" >&2
                        echo "  Remove the stale version dir (or re-run download_deps.py to prune it)." >&2
                        return 1
                    fi
                    ;;
            esac
            ort_a="$ort_a $f"
        done < <(find "$ONNXRUNTIME_STATIC_PREFIX" -type f -name '*.a' 2>/dev/null)

        if [ -n "$ort_a" ]; then
            export CGO_LDFLAGS="$CGO_LDFLAGS -Wl,--export-dynamic -Wl,--whole-archive$ort_a -Wl,--no-whole-archive -lstdc++"
            echo "  onnxruntime (static) → $ONNXRUNTIME_STATIC_PREFIX"
            # The re2 regex-library collision between onnxruntime.a and
            # librag_tokenizer_c_api.a is fixed at the .a level in build_cpp():
            # the tokenizer's bundled re2 symbols are renamed into a private
            # namespace (ragtokre2_) so the two re2 copies never share a symbol
            # name. --export-dynamic is required because OrtGetApiBase is
            # resolved via dlopen(NULL) at runtime (see the block comment above).
        else
            echo "  onnxruntime static_lib dir has no .a files; the in-process DeepDoc backend cannot link ORT" >&2
        fi
    else
        echo "  onnxruntime static_lib not found ($ONNXRUNTIME_STATIC_PREFIX); the in-process DeepDoc backend cannot link ORT" >&2
    fi

    # ── platform-specific system libraries ────────────────────────────
    case "$(uname -s)" in
        Linux)
            export CGO_LDFLAGS="$CGO_LDFLAGS -lm -lpthread -ldl -lrt -lgcc_s -lutil -lc"
            ;;
        Darwin)
            export CGO_LDFLAGS="$CGO_LDFLAGS \
                -framework CoreGraphics -framework CoreFoundation \
                -framework Security -framework SystemConfiguration \
                -liconv -lresolv -lc++"
            ;;
    esac

    echo "CGO_CFLAGS:   $CGO_CFLAGS"
    echo "CGO_LDFLAGS:  $CGO_LDFLAGS"
}

# Run Go unit tests with the same CGO env as `build_go`. Any extra args are
# forwarded to `go test`, e.g. `./build.sh --test -run TestFoo ./internal/admin/...`.
run_go_tests() {
    print_section "Running Go tests"

    cd "$PROJECT_ROOT"
    setup_cgo_env

    if [ "$#" -eq 0 ]; then
        set -- ./...
    fi
    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} CGO_ENABLED=1 \
        CGO_CFLAGS="$CGO_CFLAGS" CGO_LDFLAGS="$CGO_LDFLAGS" \
        go test -tags cgo,static -count=1 "$@"

    run_native_tests
}

# Run the unit tests of the native package (DeepDoc det/DLA/TSR/OCR-rec Go
# ports), now a regular package under the MAIN module gated by the `cgo` build
# tag (same isolation as office_oxide/pdfium). It used to be a nested Go module
# (own go.mod) that the root `./...` never descended into; after the merge it is
# covered by the normal module tests. The build is pure-Go geometry plus the
# cgo onnxruntime binding, so it runs whenever CGO_ENABLED=1 (the same gate the
# rest of the native C libs use). Model-backed integration tests are handled by
# run_native_integration_tests.
run_native_tests() {
    print_section "Running native unit tests"
    ( cd "$PROJECT_ROOT" && \
      GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} \
      CGO_ENABLED=1 \
      go test -tags cgo,static -count=1 ./internal/deepdoc/native/... )
}

# Run the model-backed integration tests of the native package and the
# native_analyzer package (the DocAnalyzer the PDF parser consumes). These
# require MODEL_DIR at runtime; ONNX Runtime is statically linked and resolved
# via dlopen(NULL), so no ORT_LIB is needed. The tests self-skip when MODEL_DIR
# is unset or the binary lacks static ORT (see native_integration_test.go
# skipIfNoModels / native_analyzer_test.go analyzerWithModels). Run under the
# `cgo integration` tier.
#
# Why -race is split instead of applied to the whole suite:
# - The DLA/TSR/OCR-rec/Det golden-comparison tests are single-threaded and
#   deterministic. -race multiplies their (model-heavy) heap ~10x for ZERO
#   race-detection value, and that is what used to OOM-kill the runner
#   (SIGTERM) under `cgo integration`. So they run WITHOUT -race.
# - Only the concurrency-correctness tests (TestInferenceConcurrency* in the
#   native package, and native_analyzer's TestAnalyzerConcurrentBatchAndSingle)
#   actually benefit from -race: they hammer the shared session pools from many
#   goroutines, and the detector catches data races on the pools / per-session
#   in-out tensors / cross-call batch state. Those run WITH -race.
# ORT's C internals are not instrumented, but all our shared mutable state lives
# in Go, which the detector covers. CGO_ENABLED=1 is required for both the build
# and the race runtime.
run_native_integration_tests() {
    print_section "Running native integration tests (golden/comparison, no race)"
    ( cd "$PROJECT_ROOT" && \
      GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} \
      CGO_ENABLED=1 \
      go test -tags "cgo static integration fetch_testdata" -count=1 ./internal/deepdoc/native/... )

    print_section "Running native integration concurrency tests (race detector on)"
    ( cd "$PROJECT_ROOT" && \
      GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} \
      CGO_ENABLED=1 \
      go test -tags "cgo static integration fetch_testdata" -race -count=1 \
      -run 'TestInferenceConcurrency' ./internal/deepdoc/native/... )

    print_section "Running native_analyzer race tests (race detector on)"
    ( cd "$PROJECT_ROOT" && \
      GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} \
      CGO_ENABLED=1 \
      go test -tags "cgo static integration fetch_testdata" -race -count=1 \
      ./internal/deepdoc/parser/pdf/inference/native_analyzer/... )
}

# Run Go tests gated behind a build tag (or space-separated tag list), e.g.
# `./build.sh --test-integration -run TestFoo ./internal/engine/...`.
# See "Go Test Tiers" in AGENTS.md for the tier definitions.
run_go_tests_tagged() {
    local tags="$1"; shift
    print_section "Running Go tests (tags: ${tags})"

    cd "$PROJECT_ROOT"
    setup_cgo_env

    if [ "$#" -eq 0 ]; then
        set -- ./...
    fi
    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} CGO_ENABLED=1 \
        CGO_CFLAGS="$CGO_CFLAGS" CGO_LDFLAGS="$CGO_LDFLAGS" \
        go test -tags "${tags} static" -count=1 "$@"
}

# Clean build artifacts
clean() {
    print_section "Cleaning build artifacts"

    rm -rf "$BUILD_DIR"
    rm -f "$RAGFLOW_SERVER_BINARY"
    rm -f "$RAGFLOW_CLI_BINARY"

    echo -e "${GREEN}✓ Build artifacts cleaned${NC}"
}

# Run the server
run() {
    if [ ! -f "$RAGFLOW_SERVER_BINARY" ]; then
        echo -e "${RED}Error: $RAGFLOW_SERVER_BINARY not found. Build first with --all or --go${NC}"
        exit 1
    fi

    cd "$PROJECT_ROOT"

    # admin_server must be running before ragflow_server, otherwise ragflow_server's
    # heartbeats to admin will error out (see internal/development.md).
    print_section "Starting admin server (background)"
    "$RAGFLOW_SERVER_BINARY" --admin &
    ADMIN_PID=$!
    # One trap for both background services: a second `trap ... EXIT INT TERM`
    # would replace this one rather than add to it, leaving admin_server holding
    # port 9383 after the foreground server exits. INGESTOR_PID is cleared first
    # so a value inherited from the environment cannot be signalled during the
    # window before the ingestor starts.
    INGESTOR_PID=""
    trap 'kill "$ADMIN_PID" ${INGESTOR_PID:+"$INGESTOR_PID"} 2>/dev/null || true' EXIT INT TERM

    # Give admin_server a moment to bind its listening port (9383) before
    # ragflow_server starts sending heartbeats to it.
    sleep 1

    print_section "Starting ingestor (background)"
    "$RAGFLOW_SERVER_BINARY" --ingestor &
    INGESTOR_PID=$!
    sleep 1

    print_section "Starting RAGFlow server (foreground)"
    "$RAGFLOW_SERVER_BINARY" --api
}

# Show help
show_help() {
    # Quoted delimiter so backticks, `$var`, and `\$` in the help text are
    # printed literally instead of being interpreted as command substitution.
    cat << 'EOF'
Usage: $0 [OPTIONS]

Build script for RAGFlow Go server with C++ bindings.

OPTIONS:
    --all, -a       Build everything (C++ library + Go server) [default]
    --cpp, -c       Build only C++ static library
    --cpp-test      Build C++ test executable (builds the C++ library if needed)
    --go, -g        Build only Go server (requires C++ library to be built)
    --test, -t      Run Go unit tests (no build tag). Sets up the CGO env and
                    native static libs (office_oxide/pdfium/pdf_oxide) needed to
                    build (same contract as the Go tier table in AGENTS.md).
                    Extra args are forwarded to `go test`, e.g.
                    `$0 --test -run TestFoo ./internal/admin/...`
    --test-integration   Run Go tests tagged 'integration' (need real services,
                    e.g. MySQL/MinIO/ES/Infinity/LLM). e.g.
                    `$0 --test-integration ./internal/engine/...`
    --test-e2e           Run Go tests tagged 'e2e' (full-pipeline, heavy).
    --test-manual        Run Go tests tagged 'manual' (very slow; local opt-in
                    ONLY, never run in CI).
    --test-all           Run 'integration' + 'e2e' tests (excludes 'manual').
    --test-native        Run the in-process (Go) DeepDoc backend tests tagged
                    'cgo integration' (needs libonnxruntime + the
                    InfiniFlow/deepdoc model snapshot; self-skip otherwise).
                    e.g. `$0 --test-native`
    --clean, -C     Clean all build artifacts
    --run, -r       Build and run the server
    --strip, -s     Strip debug symbols from Go binaries (-ldflags="-s -w")
                    (disabled by default, useful for smaller production binaries)
    --help, -h      Show this help message

EXAMPLES:
    $0              # Build everything
    $0 --cpp        # Build only C++ library
    $0 --go         # Build only Go server
    $0 --cpp-test   # Build C++ test executable
    $0 --test       # Run all Go tests (unit tier, no build tag)
    $0 --test -run TestFoo ./internal/admin/...      # Targeted Go tests
    $0 --test-integration ./internal/engine/...      # integration tier
    $0 --test-e2e                                 # e2e tier
    $0 --test-manual                             # manual tier (very slow)
    $0 --test-all                                # integration + e2e (no manual)
    $0 --run        # Build and run
    $0 --clean      # Clean build artifacts

DEPENDENCIES:
    - cmake >= 4.0
    - go >= 1.26.4
    - clang++ with C++20 support
    - office_oxide native library (download with: uv run python3 ragflow_deps/download_go_deps.py)
    - lld (Linux only): sudo apt install lld-20 && sudo ln -s /usr/bin/ld.lld-20 /usr/bin/ld.lld
    - pcre2 development files
        - Debian/Ubuntu: libpcre2-dev
        - openSUSE/RHEL/Fedora: pcre2-devel
        - macOS (Homebrew): pcre2
EOF
}

# Main function
main() {
    # Parse --strip / -s before other arguments
    local args=()
    for arg in "$@"; do
        case "$arg" in
            --strip|-s) STRIP_SYMBOLS="1" ;;
            *) args+=("$arg") ;;
        esac
    done

    case "${args[0]:-}" in
        --cpp|-c)
            check_cpp_deps
            build_cpp
            ;;
        --cpp-test)
            check_cpp_deps
            build_cpp_test
            ;;
        --go|-g)
            check_go_deps
            build_go
            ;;
        --test|-t)
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                run_go_tests "${args[@]:2}"
            else
                run_go_tests "${args[@]:1}"
            fi
            ;;
        --test-integration)
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                run_go_tests_tagged integration "${args[@]:2}"
            else
                run_go_tests_tagged integration "${args[@]:1}"
            fi
            run_native_integration_tests
            ;;
        --test-e2e)
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                run_go_tests_tagged e2e "${args[@]:2}"
            else
                run_go_tests_tagged e2e "${args[@]:1}"
            fi
            ;;
        --test-manual)
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                run_go_tests_tagged manual "${args[@]:2}"
            else
                run_go_tests_tagged manual "${args[@]:1}"
            fi
            ;;
        --test-all)
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                run_go_tests_tagged "integration e2e" "${args[@]:2}"
            else
                run_go_tests_tagged "integration e2e" "${args[@]:1}"
            fi
            ;;
        --test-native)
            check_go_deps
            if [ "${args[1]:-}" = "--" ]; then
                pkgs=("${args[@]:2}")
            else
                pkgs=("${args[@]:1}")
            fi
            if [ "${#pkgs[@]}" -eq 0 ]; then
                pkgs=(./internal/deepdoc/parser/pdf/inference/native_analyzer/...)
            fi
            run_go_tests_tagged "cgo integration" "${pkgs[@]}"
            run_native_integration_tests
            ;;
        --clean|-C)
            clean
            ;;
        --run|-r)
            check_cpp_deps
            check_go_deps
            build_cpp
            build_go
            run
            ;;
        --help|-h)
            show_help
            ;;
        --all|-a|"")
            check_cpp_deps
            check_go_deps
            build_cpp
            build_go
            echo -e "\n${GREEN}=== Build completed successfully! ===${NC}"
            echo "Binary: $RAGFLOW_SERVER_BINARY, $RAGFLOW_CLI_BINARY"
            ;;
        *)
            echo -e "${RED}Unknown option: ${args[0]}${NC}"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
