#!/bin/sh
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

set -eu

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
GITHUB_REPO="${GITHUB_REPO:-infiniflow/ragflow}"
CLI_NAME="${CLI_NAME:-ragflow-cli}"
VERSION="${VERSION:-latest}"

RELEASE_API="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
RELEASE_BASE_URL=""
OS=""
ARCH=""
FILENAME=""
DOWNLOAD_URL=""
CHECKSUM_URL=""

info() {
    printf '[INFO] %s\n' "$1" >&2
}

warn() {
    printf '[WARN] %s\n' "$1" >&2
}

error() {
    printf '[ERROR] %s\n' "$1" >&2
}

need_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        error "$1 is required but not installed"
        exit 1
    fi
}

run_as_root() {
    if "$@" 2>/dev/null; then
        return 0
    fi

    if command -v sudo >/dev/null 2>&1; then
        info "Requesting sudo permission for: $*"
        sudo "$@"
        return $?
    fi

    error "Permission denied and sudo is not available: $*"
    exit 1
}

detect_platform() {
    os_name="$(uname -s)"
    arch_name="$(uname -m)"

    case "$os_name" in
        Linux)
            OS="linux"
            ;;
        Darwin)
            OS="darwin"
            ;;
        *)
            error "Unsupported OS for install.sh: $os_name"
            error "Use install.ps1 on Windows PowerShell"
            exit 1
            ;;
    esac

    case "$arch_name" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            error "Unsupported architecture: $arch_name"
            exit 1
            ;;
    esac

    info "Detected platform: ${OS}/${ARCH}"
}

resolve_version() {
    if [ "$VERSION" != "latest" ]; then
        info "Using specified version: $VERSION"
        return
    fi

    info "Fetching latest release information"
    response="$(curl -sSfL "$RELEASE_API")"
    VERSION="$(printf '%s\n' "$response" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"

    if [ -z "$VERSION" ]; then
        error "Could not determine latest version from GitHub"
        exit 1
    fi

    info "Latest version: $VERSION"
}

build_urls() {
    FILENAME="${CLI_NAME}-${VERSION}-${OS}-${ARCH}"
    RELEASE_BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}"
    DOWNLOAD_URL="${RELEASE_BASE_URL}/${FILENAME}"
    CHECKSUM_URL="${RELEASE_BASE_URL}/SHA256SUMS"

    info "Download URL: $DOWNLOAD_URL"
}

download_file() {
    url="$1"
    output="$2"

    if ! curl -sSfL "$url" -o "$output"; then
        rm -f "$output"
        error "Failed to download $url"
        exit 1
    fi
}

file_size() {
    stat -f%z "$1" 2>/dev/null || stat -c%s "$1" 2>/dev/null || printf '0'
}

sha256_file() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
        return
    fi

    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
        return
    fi

    error "sha256sum or shasum is required to verify the download"
    exit 1
}

verify_checksum() {
    binary_file="$1"
    sums_file="$2"

    expected="$(awk -v f="$FILENAME" '$2 == f {print $1}' "$sums_file" | head -n 1)"
    if [ -z "$expected" ]; then
        error "No checksum found for $FILENAME in SHA256SUMS"
        exit 1
    fi

    actual="$(sha256_file "$binary_file")"
    if [ "$actual" != "$expected" ]; then
        error "Checksum verification failed for $FILENAME"
        error "Expected: $expected"
        error "Actual:   $actual"
        exit 1
    fi

    info "Checksum verified"
}

download_cli() {
    binary_file="$(mktemp)"
    sums_file="$(mktemp)"

    info "Downloading CLI"
    download_file "$DOWNLOAD_URL" "$binary_file"

    size="$(file_size "$binary_file")"
    if [ "$size" -lt 100000 ]; then
        warn "Downloaded file is unusually small (${size} bytes)"
    fi

    info "Downloading SHA256SUMS"
    download_file "$CHECKSUM_URL" "$sums_file"
    verify_checksum "$binary_file" "$sums_file"
    rm -f "$sums_file"

    printf '%s\n' "$binary_file"
}

install_cli() {
    source_file="$1"
    target_file="${INSTALL_DIR}/${CLI_NAME}"

    info "Installing CLI to $target_file"

    if [ ! -d "$INSTALL_DIR" ]; then
        run_as_root mkdir -p "$INSTALL_DIR"
    fi

    if [ -w "$INSTALL_DIR" ]; then
        mv "$source_file" "$target_file"
        chmod 755 "$target_file"
    else
        run_as_root mv "$source_file" "$target_file"
        run_as_root chmod 755 "$target_file"
    fi

    info "CLI installed successfully at $target_file"
}

verify_installation() {
    cli_path="${INSTALL_DIR}/${CLI_NAME}"

    if [ ! -x "$cli_path" ]; then
        warn "CLI may not be executable: $cli_path"
        return
    fi

    if "$cli_path" --version >/dev/null 2>&1; then
        "$cli_path" --version
        info "Installation verified successfully"
        return
    fi

    if "$cli_path" -h >/dev/null 2>&1 || "$cli_path" --help >/dev/null 2>&1; then
        info "Installation verified successfully"
        return
    fi

    warn "Could not verify CLI execution, but the binary was installed"
}

print_path_notice() {
    case ":$PATH:" in
        *":$INSTALL_DIR:"*)
            ;;
        *)
            warn "$INSTALL_DIR is not in PATH"
            warn "Add it with: export PATH=\"$INSTALL_DIR:\$PATH\""
            ;;
    esac
}

main() {
    need_cmd curl
    need_cmd uname
    need_cmd mktemp
    need_cmd awk
    need_cmd sed

    detect_platform
    resolve_version
    build_urls

    temp_file="$(download_cli)"
    install_cli "$temp_file"
    verify_installation
    print_path_notice

    info "Installation complete"
}

main "$@"
