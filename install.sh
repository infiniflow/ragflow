#!/bin/bash
#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
GITHUB_REPO="${GITHUB_REPO:-infiniflow/ragflow}"
RELEASE_API="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
CLI_NAME="${CLI_NAME:-ragflow_cli}"
VERSION="${VERSION:-latest}"

# Function to print colored output
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check required commands
check_dependencies() {
    if ! command -v curl >/dev/null 2>&1; then
        print_error "curl is required but not installed"
        exit 1
    fi

    if ! command -v uname >/dev/null 2>&1; then
        print_error "uname is required but not available"
        exit 1
    fi
}

# Detect OS and Architecture
detect_platform() {
    local os
    local arch

    os=$(uname -s)
    arch=$(uname -m)

    case "$os" in
        Linux)
            OS="linux"
            ;;
        Darwin)
            OS="darwin"
            ;;
        MINGW*|MSYS*|CYGWIN*)
            OS="windows"
            ;;
        *)
            print_error "Unsupported OS: $os"
            exit 1
            ;;
    esac

    case "$arch" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            print_error "Unsupported Architecture: $arch"
            exit 1
            ;;
    esac

    print_info "Detected platform: ${OS}/${ARCH}"
}

# Get the latest release version
get_latest_version() {
    if [ "$VERSION" != "latest" ]; then
        print_info "Using specified version: $VERSION"
        return
    fi

    print_info "Fetching latest release information..."

    local response
    response=$(curl -sSfL "$RELEASE_API" 2>/dev/null || echo "")

    if [ -z "$response" ]; then
        print_error "Failed to fetch release information from GitHub"
        exit 1
    fi

    VERSION=$(echo "$response" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": "//;s/".*$//')

    if [ -z "$VERSION" ]; then
        print_error "Could not determine latest version"
        exit 1
    fi

    print_info "Latest version: $VERSION"
}

# Build the download URL
build_download_url() {
    local filename="${CLI_NAME}-${VERSION}-${OS}-${ARCH}"

    if [ "$OS" = "windows" ]; then
        filename="${filename}.exe"
    fi

    DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${filename}"

    print_info "Download URL: $DOWNLOAD_URL"
}

# Download the CLI binary
download_cli() {
    local temp_file
    temp_file=$(mktemp)

    print_info "Downloading CLI from $DOWNLOAD_URL..."

    if ! curl -sSfL "$DOWNLOAD_URL" -o "$temp_file"; then
        rm -f "$temp_file"
        print_error "Failed to download CLI binary"
        exit 1
    fi

    # Verify file size
    local file_size
    file_size=$(stat -f%z "$temp_file" 2>/dev/null || stat -c%s "$temp_file" 2>/dev/null || echo 0)

    if [ "$file_size" -lt 1000000 ]; then
        print_warn "Downloaded file seems suspiciously small ($file_size bytes)"
    fi

    echo "$temp_file"
}

# Install the CLI binary
install_cli() {
    local temp_file="$1"
    local target_file="$INSTALL_DIR/${CLI_NAME}"

    if [ "$OS" = "windows" ]; then
        target_file="${target_file}.exe"
    fi

    print_info "Installing CLI to $target_file..."

    if [ ! -d "$INSTALL_DIR" ]; then
        print_info "Creating install directory: $INSTALL_DIR"

        if mkdir -p "$INSTALL_DIR" 2>/dev/null; then
            true
        else
            print_info "Requesting sudo permission to create $INSTALL_DIR"
            sudo mkdir -p "$INSTALL_DIR"
        fi
    fi

    # Check if we need sudo
    if [ ! -w "$INSTALL_DIR" ]; then
        print_info "Requesting sudo permission to install to $INSTALL_DIR"

        if ! sudo mv "$temp_file" "$target_file"; then
            rm -f "$temp_file"
            print_error "Failed to install CLI (permission denied)"
            exit 1
        fi

        if ! sudo chmod +x "$target_file"; then
            print_error "Failed to make CLI executable"
            exit 1
        fi
    else
        if ! mv "$temp_file" "$target_file"; then
            rm -f "$temp_file"
            print_error "Failed to install CLI"
            exit 1
        fi

        if ! chmod +x "$target_file"; then
            print_error "Failed to make CLI executable"
            exit 1
        fi
    fi

    print_info "CLI installed successfully at $target_file"
}

# Verify installation
verify_installation() {
    local cli_path="$INSTALL_DIR/${CLI_NAME}"

    if [ "$OS" = "windows" ]; then
        cli_path="${cli_path}.exe"
    fi

    print_info "Verifying installation..."

    if [ ! -x "$cli_path" ]; then
        print_warn "CLI may not be executable. You can check it directly at: $cli_path"
        return
    fi

    if "$cli_path" --version >/dev/null 2>&1; then
        print_info "Installation verified successfully!"
        "$cli_path" --version 2>/dev/null || true
        return
    fi

    if "$cli_path" -h >/dev/null 2>&1; then
        print_info "Installation verified successfully!"
        return
    fi

    print_warn "Could not verify CLI installation, but it may still work"
}

# Print PATH notice
print_path_notice() {
    case ":$PATH:" in
        *":$INSTALL_DIR:"*)
            ;;
        *)
            print_warn "$INSTALL_DIR is not in PATH"
            print_warn "You can run the CLI directly at: $INSTALL_DIR/${CLI_NAME}"
            print_warn "Or add it to PATH, for example:"
            echo
            echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
            echo
            ;;
    esac
}

# Main installation flow
main() {
    echo "=========================================="
    echo "RAGFlow CLI Installer"
    echo "=========================================="
    echo

    check_dependencies
    detect_platform
    get_latest_version
    build_download_url

    local temp_file
    temp_file=$(download_cli)

    install_cli "$temp_file"
    verify_installation
    print_path_notice

    echo
    echo -e "${GREEN}=========================================="
    echo "Installation complete! 🎉"
    echo "==========================================${NC}"
    echo
    print_info "You can now use '${CLI_NAME}' command"
    print_info "Installation directory: ${INSTALL_DIR}"
    echo
}

# Run main function
main "$@"