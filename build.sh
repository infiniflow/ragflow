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
CPP_DIR="$PROJECT_ROOT/internal/cpp"
BUILD_DIR="$CPP_DIR/cmake-build-release"
RAGFLOW_SERVER_BINARY="$PROJECT_ROOT/bin/server_main"
ADMIN_SERVER_BINARY="$PROJECT_ROOT/bin/admin_server"
RAGFLOW_CLI_BINARY="$PROJECT_ROOT/bin/ragflow_cli"

# office_oxide native library settings
OFFICE_OXIDE_PREFIX="${HOME}/.office_oxide"
OFFICE_OXIDE_VERSION="0.1.2"

echo -e "${GREEN}=== RAGFlow Go Server Build Script ===${NC}"

# Function to print section headers
print_section() {
    echo -e "\n${YELLOW}>>> $1${NC}"
}

# Check dependencies
check_cpp_deps() {
    print_section "Checking c++ dependencies"

    command -v cmake >/dev/null 2>&1 || { echo -e "${RED}Error: cmake is required but not installed.${NC}"; exit 1; }
    command -v g++ >/dev/null 2>&1 || { echo -e "${RED}Error: g++ is required but not installed.${NC}"; exit 1; }

    # Check for pcre2 library (static .a or shared .so; -lpcre2-8 finds either)
    if [ -f "/usr/lib/x86_64-linux-gnu/libpcre2-8.a" ] \
       || [ -f "/usr/lib/x86_64-linux-gnu/libpcre2-8.so" ] \
       || [ -f "/usr/local/lib/libpcre2-8.a" ] \
       || [ -f "/usr/local/lib/libpcre2-8.so" ]; then
        echo "✓ pcre2 library found"
    else
        echo -e "${YELLOW}Warning: libpcre2-8 not found. You may need to install libpcre2-dev:${NC}"
        echo "  sudo apt-get install libpcre2-dev"
    fi

    echo "✓ Required tools are available"
}

check_go_deps() {
    print_section "Checking go dependencies"
    
    command -v go >/dev/null 2>&1 || { echo -e "${RED}Error: go is required but not installed.${NC}"; exit 1; }

    echo "✓ Required tools are available"
}

# Download and extract a tar.gz from a URL to a target directory
_download_and_extract() {
    local url="$1" target_dir="$2"
    echo "Downloading ${url} ..."
    local tmpfile
    tmpfile="$(mktemp)"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "$url" -o "$tmpfile"
    elif command -v wget >/dev/null 2>&1; then
        wget -q "$url" -O "$tmpfile"
    else
        echo -e "${RED}Error: need curl or wget to download office_oxide${NC}"
        exit 1
    fi
    tar xzf "$tmpfile" -C "$target_dir"
    rm -f "$tmpfile"
}

# Check / install office_oxide native library (Rust → C FFI library)
check_office_oxide_deps() {
    print_section "Checking office_oxide native library"

    local lib_file header_path
    case "$(uname -s)" in
        Linux)  lib_file="liboffice_oxide.so" ;;
        Darwin) lib_file="liboffice_oxide.dylib" ;;
        *)      echo -e "${RED}Unsupported OS for office_oxide${NC}"; exit 1 ;;
    esac

    local lib_path="${OFFICE_OXIDE_PREFIX}/lib/${lib_file}"
    local header_path="${OFFICE_OXIDE_PREFIX}/include/office_oxide_c/office_oxide.h"

    if [ -f "$lib_path" ] && [ -f "$header_path" ]; then
        echo "✓ office_oxide native library found at ${OFFICE_OXIDE_PREFIX}"
        return 0
    fi

    echo "office_oxide native library not found. Installing..."

    # Map platform to the release asset name. Note: the GitHub release archives
    # omit the version number from the native-* asset filenames.
    local asset_name
    case "$(uname -s)" in
        Linux)
            case "$(uname -m)" in
                x86_64)  asset_name="native-linux-x86_64" ;;
                aarch64|arm64) asset_name="native-linux-aarch64" ;;
                *) echo -e "${RED}Unsupported arch: $(uname -m)${NC}"; exit 1 ;;
            esac
            ;;
        Darwin)
            case "$(uname -m)" in
                x86_64)  asset_name="native-macos-x86_64" ;;
                aarch64|arm64) asset_name="native-macos-aarch64" ;;
                *) echo -e "${RED}Unsupported arch: $(uname -m)${NC}"; exit 1 ;;
            esac
            ;;
    esac

    local release_url="https://github.com/yfedoseev/office_oxide/releases/download/v${OFFICE_OXIDE_VERSION}/${asset_name}.tar.gz"

    mkdir -p "${OFFICE_OXIDE_PREFIX}"
    _download_and_extract "$release_url" "${OFFICE_OXIDE_PREFIX}"

    if [ ! -f "$lib_path" ]; then
        echo -e "${RED}Error: Failed to install office_oxide native library (missing ${lib_path})${NC}"
        echo "  Try: curl -fsSL ${release_url} | tar xzf - -C ${OFFICE_OXIDE_PREFIX}"
        exit 1
    fi

    echo -e "${GREEN}✓ office_oxide native library installed${NC}"
}

# Build C++ static library
build_cpp() {
    print_section "Building C++ static library"
    
    mkdir -p "$BUILD_DIR"
    cd "$BUILD_DIR"
    
    echo "Running cmake..."
    cmake .. -DCMAKE_BUILD_TYPE=Release
    
    echo "Building librag_tokenizer_c_api.a..."
    make rag_tokenizer_c_api -j$(nproc)
    
    if [ ! -f "$BUILD_DIR/librag_tokenizer_c_api.a" ]; then
        echo -e "${RED}Error: Failed to build C++ static library${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✓ C++ static library built successfully${NC}"
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

    # Check for pcre2 library — known Linux paths + macOS Homebrew (Apple Silicon
    # at /opt/homebrew, Intel Macs at /usr/local). Checks both .a and .so.
    if [ -f "/usr/lib/x86_64-linux-gnu/libpcre2-8.a" ] \
       || [ -f "/usr/lib/x86_64-linux-gnu/libpcre2-8.so" ] \
       || [ -f "/usr/local/lib/libpcre2-8.a" ] \
       || [ -f "/usr/local/lib/libpcre2-8.so" ] \
       || [ -f "/opt/homebrew/lib/libpcre2-8.a" ]; then
        echo "✓ pcre2 library found"
    else
        if [ "$(uname)" = "Darwin" ]; then
            echo -e "${RED}Error: libpcre2-8 not found. Install with: brew install pcre2${NC}"
            exit 1
        fi
        echo -e "${YELLOW}Warning: libpcre2-8 not found. You may need to install libpcre2-dev:${NC}"
        sudo apt -y install libpcre2-dev
    fi

    # Check / install office_oxide native library
    check_office_oxide_deps

    # Export CGO flags so go build can find office_oxide headers and library
    export CGO_CFLAGS="-I${OFFICE_OXIDE_PREFIX}/include/office_oxide_c${CGO_CFLAGS:+ $CGO_CFLAGS}"
    echo "Exporting CGO_CFLAGS: $CGO_CFLAGS"
    export CGO_LDFLAGS="-L${OFFICE_OXIDE_PREFIX}/lib -loffice_oxide -Wl,-rpath,${OFFICE_OXIDE_PREFIX}/lib${CGO_LDFLAGS:+ $CGO_LDFLAGS}"
    echo "Exporting CGO_LDFLAGS: $CGO_LDFLAGS"

    echo "Building RAGFlow binary: $RAGFLOW_SERVER_BINARY, $ADMIN_SERVER_BINARY, and $RAGFLOW_CLI_BINARY"
    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} CGO_ENABLED=1 \
        CGO_CFLAGS="$CGO_CFLAGS" CGO_LDFLAGS="$CGO_LDFLAGS" \
        go build -o "$RAGFLOW_SERVER_BINARY" cmd/server_main.go
    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} CGO_ENABLED=1 \
        CGO_CFLAGS="$CGO_CFLAGS" CGO_LDFLAGS="$CGO_LDFLAGS" \
        go build -o "$ADMIN_SERVER_BINARY" cmd/admin_server.go
    GOPROXY=${GOPROXY:-https://goproxy.cn,https://proxy.golang.org,direct} CGO_ENABLED=1 \
        CGO_CFLAGS="$CGO_CFLAGS" CGO_LDFLAGS="$CGO_LDFLAGS" \
        go build -o "$RAGFLOW_CLI_BINARY" cmd/ragflow_cli.go

    if [ ! -f "$RAGFLOW_SERVER_BINARY" ]; then
        echo -e "${RED}Error: Failed to build RAGFlow server binary${NC}"
        exit 1
    fi

    if [ ! -f "$ADMIN_SERVER_BINARY" ]; then
        echo -e "${RED}Error: Failed to build Admin server binary${NC}"
        exit 1
    fi

    echo -e "${GREEN}✓ Go ragflow_server built successfully: $RAGFLOW_SERVER_BINARY${NC}"
    echo -e "${GREEN}✓ Go admin_server built successfully: $ADMIN_SERVER_BINARY${NC}"
    echo -e "${GREEN}✓ Go ragflow_cli built successfully: $RAGFLOW_CLI_BINARY${NC}"
}

# Clean build artifacts
clean() {
    print_section "Cleaning build artifacts"
    
    rm -rf "$BUILD_DIR"
    rm -f "$RAGFLOW_SERVER_BINARY"
    rm -f "$ADMIN_SERVER_BINARY"

    echo -e "${GREEN}✓ Build artifacts cleaned${NC}"
}

# Run the server
run() {
    if [ ! -f "$ADMIN_SERVER_BINARY" ]; then
        echo -e "${RED}Error: Binary not found. Build first with --all or --go${NC}"
        exit 1
    fi

    print_section "Starting ADMIN server"
    cd "$PROJECT_ROOT"
    ./admin_server

    if [ ! -f "$RAGFLOW_SERVER_BINARY" ]; then
        echo -e "${RED}Error: Binary not found. Build first with --all or --go${NC}"
        exit 1
    fi
    
    print_section "Starting server"
    cd "$PROJECT_ROOT"
    ./server_main
}

# Show help
show_help() {
    cat << EOF
Usage: $0 [OPTIONS]

Build script for RAGFlow Go server with C++ bindings.

OPTIONS:
    --all, -a       Build everything (C++ library + Go server) [default]
    --cpp, -c       Build only C++ static library
    --go, -g        Build only Go server (requires C++ library to be built)
    --clean, -C     Clean all build artifacts
    --run, -r       Build and run the server
    --help, -h      Show this help message

EXAMPLES:
    $0              # Build everything
    $0 --cpp        # Build only C++ library
    $0 --go         # Build only Go server
    $0 --run        # Build and run
    $0 --clean      # Clean build artifacts

DEPENDENCIES:
    - cmake >= 4.0
    - go >= 1.24
    - g++ with C++17/23 support
    - libpcre2-dev
    - office_oxide native library (auto-downloaded on first build)
EOF
}

# Main function
main() {
    case "${1:-}" in
        --cpp|-c)
            check_cpp_deps
            build_cpp
            ;;
        --go|-g)
            check_go_deps
            build_go
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
            echo "Binary: $RAGFLOW_SERVER_BINARY, $ADMIN_SERVER_BINARY"
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
