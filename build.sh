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
OUTPUT_BINARY="$PROJECT_ROOT/server_main"

echo -e "${GREEN}=== RAGFlow Go Server Build Script ===${NC}"

# Function to print section headers
print_section() {
    echo -e "\n${YELLOW}>>> $1${NC}"
}

# Check dependencies
check_deps() {
    print_section "Checking dependencies"
    
    command -v cmake >/dev/null 2>&1 || { echo -e "${RED}Error: cmake is required but not installed.${NC}"; exit 1; }
    command -v go >/dev/null 2>&1 || { echo -e "${RED}Error: go is required but not installed.${NC}"; exit 1; }
    command -v g++ >/dev/null 2>&1 || { echo -e "${RED}Error: g++ is required but not installed.${NC}"; exit 1; }
    
    # Check for pcre2 library
    if [ -f "/usr/lib/x86_64-linux-gnu/libpcre2-8.a" ] || [ -f "/usr/local/lib/libpcre2-8.a" ]; then
        echo "✓ pcre2 library found"
    else
        echo -e "${YELLOW}Warning: libpcre2-8.a not found. You may need to install libpcre2-dev:${NC}"
        echo "  sudo apt-get install libpcre2-dev"
    fi
    
    echo "✓ All required tools are available"
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
    print_section "Building Go server"
    
    cd "$PROJECT_ROOT"
    
    # Check if C++ library exists
    if [ ! -f "$BUILD_DIR/librag_tokenizer_c_api.a" ]; then
        echo -e "${RED}Error: C++ static library not found. Run with --cpp first.${NC}"
        exit 1
    fi
    
    echo "Building Go binary..."
    CGO_ENABLED=1 go build -o "$OUTPUT_BINARY" ./cmd/server_main.go
    
    if [ ! -f "$OUTPUT_BINARY" ]; then
        echo -e "${RED}Error: Failed to build Go binary${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✓ Go server built successfully: $OUTPUT_BINARY${NC}"
}

# Clean build artifacts
clean() {
    print_section "Cleaning build artifacts"
    
    rm -rf "$BUILD_DIR"
    rm -f "$OUTPUT_BINARY"
    
    echo -e "${GREEN}✓ Build artifacts cleaned${NC}"
}

# Run the server
run() {
    if [ ! -f "$OUTPUT_BINARY" ]; then
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
EOF
}

# Main function
main() {
    case "${1:-}" in
        --cpp|-c)
            check_deps
            build_cpp
            ;;
        --go|-g)
            check_deps
            build_go
            ;;
        --clean|-C)
            clean
            ;;
        --run|-r)
            check_deps
            build_cpp
            build_go
            run
            ;;
        --help|-h)
            show_help
            ;;
        --all|-a|"")
            check_deps
            build_cpp
            build_go
            echo -e "\n${GREEN}=== Build completed successfully! ===${NC}"
            echo "Binary: $OUTPUT_BINARY"
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            show_help
            exit 1
            ;;
    esac
}

main "$@"
