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
OFFICE_OXIDE_VERSION="0.1.2"

# pdfium native library settings — static linking (kognitos/pdfium-static)
PDFIUM_STATIC_PREFIX="${HOME}/ragflow-native-libs/pdfium-static"
PDFIUM_STATIC_VERSION="7809"

# pdf_oxide native library settings — static linking (go-ffi tarball)
PDF_OXIDE_PREFIX="${HOME}/ragflow-native-libs/pdf_oxide"
PDF_OXIDE_VERSION="0.3.67"

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

    if [ -f "$lib_path" ] && [ -f "$header_path" ]; then
        echo "✓ office_oxide native library found at ${OFFICE_OXIDE_PREFIX}"
        return 0
    fi

    echo -e "${RED}Error: office_oxide native library not found${NC}"
    echo "  Expected: ${lib_path}"
    echo "  Run: uv run python3 ragflow_deps/download_deps.py"
    echo "  Or manually download: https://github.com/yfedoseev/office_oxide/releases/download/v${OFFICE_OXIDE_VERSION}/native-linux-x86_64.tar.gz"
    exit 1
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
    echo "  Run: uv run python3 ragflow_deps/download_deps.py"
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
        echo "  pdf_oxide (static) → ${PDF_OXIDE_PREFIX}"
        return 0
    fi

    echo "  pdf_oxide (static) not found"
    echo "  Expected: ${lib_path}"
    echo "  Run: uv run python3 ragflow_deps/download_deps.py"
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

    local strip_flags=()
    [ -n "$STRIP_SYMBOLS" ] && strip_flags=(-ldflags="-s -w")

    echo "Building RAGFlow binary: $RAGFLOW_CLI_BINARY and $RAGFLOW_SERVER_BINARY"
    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct}
        go build "${strip_flags[@]}" -o "$RAGFLOW_CLI_BINARY" cmd/ragflow-cli.go

    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} CGO_ENABLED=1 \
        CGO_CFLAGS="$CGO_CFLAGS" CGO_LDFLAGS="$CGO_LDFLAGS" \
        go build "${strip_flags[@]}" -o "$RAGFLOW_SERVER_BINARY" cmd/ragflow_server.go


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
    export CGO_CFLAGS="-I${OFFICE_OXIDE_PREFIX}/include/office_oxide_c${CGO_CFLAGS:+ $CGO_CFLAGS}"
    export CGO_LDFLAGS="${OFFICE_OXIDE_PREFIX}/lib/liboffice_oxide.a"

    # ── pdfium ────────────────────────────────────────────────────────
    check_pdfium_deps || return 1
    export CGO_LDFLAGS="$CGO_LDFLAGS ${PDFIUM_STATIC_PREFIX}/lib/libpdfium.a"
    # Linux: Chromium-built objects use Clang's .eh_frame format which GNU ld
    # cannot merge. Use lld (LLVM linker) which handles them correctly.
    # --allow-multiple-definition: pdf_oxide and office_oxide are both Rust
    # staticlibs that embed the Rust runtime; linking them together produces
    # duplicate rust_eh_personality symbols.
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
    export CGO_LDFLAGS="$CGO_LDFLAGS ${PDF_OXIDE_PREFIX}/lib/${pdf_oxide_subdir}/libpdf_oxide.a"

    # ── platform-specific system libraries ────────────────────────────
    case "$(uname -s)" in
        Linux)
            export CGO_LDFLAGS="$CGO_LDFLAGS -lm -lpthread -ldl -lrt -lgcc_s -lutil -lc"
            ;;
        Darwin)
            export CGO_LDFLAGS="$CGO_LDFLAGS \
                -framework CoreFoundation -framework Security \
                -framework SystemConfiguration -liconv -lresolv"
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
        go test -count=1 "$@"
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
    trap 'kill "$ADMIN_PID" 2>/dev/null || true' EXIT INT TERM

    # Give admin_server a moment to bind its listening port (9383) before
    # ragflow_server starts sending heartbeats to it.
    sleep 1

    print_section "Starting ingestor (background)"
    "$RAGFLOW_SERVER_BINARY" --ingestor &
    INGESTOR_PID=$!
    trap 'kill "$INGESTOR_PID" 2>/dev/null || true' EXIT INT TERM
    sleep 1

    print_section "Starting RAGFlow server (foreground)"
    "$RAGFLOW_SERVER_BINARY" -- api
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
    --cpp-test      Build C++ test executable (requires --cpp first)
    --go, -g        Build only Go server (requires C++ library to be built)
    --test, -t      Run Go unit tests (sets up CGO env for office_oxide).
                    Any extra args are forwarded to `go test`, e.g.
                    `$0 --test -run TestFoo ./internal/admin/...`
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
    $0 --test       # Run all Go tests
    $0 --test -run TestFoo ./internal/admin/...      # Targeted Go tests
    $0 --run        # Build and run
    $0 --clean      # Clean build artifacts

DEPENDENCIES:
    - cmake >= 4.0
    - go >= 1.24
    - g++ with C++17/23 support
    - office_oxide native library (download with: uv run python3 ragflow_deps/download_deps.py)
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
            echo -e "${RED}Unknown option: $1${NC}"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
