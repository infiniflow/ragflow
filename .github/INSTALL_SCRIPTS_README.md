# RAGFlow CLI Installation Scripts

This directory contains installation scripts for the RAGFlow CLI tool across different platforms.

## Quick Start

### Linux & macOS

```bash
curl -sSfL https://your-domain/install.sh | sh
````

Or with specific version:

```bash
curl -sSfL https://your-domain/install.sh | VERSION=v1.0.0 sh
```

Or with custom installation directory:

```bash
curl -sSfL https://your-domain/install.sh | INSTALL_DIR=$HOME/.local/bin sh
```

### Windows (PowerShell)

```powershell
Invoke-WebRequest -Uri "https://your-domain/install.ps1" -OutFile install.ps1
Set-ExecutionPolicy -ExecutionPolicy Bypass -Scope Process
.\install.ps1
```

Or with specific version and installation directory:

```powershell
.\install.ps1 -Version "v1.0.0" -InstallDir "C:\Program Files\RAGFlow"
```

## Scripts Overview

### `install.sh` (Unix/Linux/macOS)

**Features:**

* Auto-detects OS (Linux, macOS) and architecture (amd64, arm64)
* Downloads the correct binary for your platform
* Installs to `/usr/local/bin` by default
* Handles sudo permission if needed
* Verifies installation after completing

**Environment Variables:**

* `VERSION`: Specify release version (default: `latest`)
* `INSTALL_DIR`: Installation directory (default: `/usr/local/bin`)
* `GITHUB_REPO`: GitHub repository (default: `infiniflow/ragflow`)
* `CLI_NAME`: CLI binary name (default: `ragflow_cli`)

**Usage:**

```bash
# Default installation
chmod +x install.sh
./install.sh

# With custom version
VERSION=v1.0.0 ./install.sh

# With custom installation directory
INSTALL_DIR=$HOME/.local/bin ./install.sh

# Download and execute in one command
curl -sSfL https://your-domain/install.sh | sh
```

### `install.ps1` (Windows)

**Features:**

* Auto-detects Windows architecture (amd64, arm64)
* Downloads the Windows `.exe` binary
* Creates installation directory if needed
* Adds installation directory to user PATH automatically
* Verifies installation after completing

**Parameters:**

* `-Version`: Specify release version (default: `latest`)
* `-InstallDir`: Installation directory (default: `$env:PROGRAMFILES\RAGFlow`)
* `-GitHubRepo`: GitHub repository (default: `infiniflow/ragflow`)
* `-CliName`: CLI binary name (default: `ragflow_cli`)

**Usage:**

```powershell
# Set execution policy for current session
Set-ExecutionPolicy -ExecutionPolicy Bypass -Scope Process

# Run with defaults
.\install.ps1

# With custom version
.\install.ps1 -Version "v1.0.0"

# With custom installation directory
.\install.ps1 -InstallDir "C:\Tools\RAGFlow"

# Remote download and execution
Invoke-WebRequest -Uri "https://your-domain/install.ps1" -OutFile install.ps1; `
Set-ExecutionPolicy -ExecutionPolicy Bypass -Scope Process; `
.\install.ps1
```

## Hosting the Installation Scripts

### Option 1: GitHub Releases

The scripts can be hosted as release assets on GitHub.

Users would access them via:

```text
https://github.com/infiniflow/ragflow/releases/download/v1.0.0/install.sh
https://github.com/infiniflow/ragflow/releases/download/v1.0.0/install.ps1
```

### Option 2: Static Web Server

Host the scripts on your domain:

```text
https://your-domain/install.sh
https://your-domain/install.ps1
```

### Option 3: Smart Router Script

Create a single `install.sh` on your server that detects the platform and serves the appropriate script:

```bash
#!/bin/bash

OS=$(uname -s)

case "$OS" in
    Linux|Darwin)
        exec curl -sSfL https://your-domain/install.sh | sh
        ;;
    MINGW*|MSYS*|CYGWIN*)
        exec powershell -Command "Invoke-WebRequest -Uri 'https://your-domain/install.ps1' -OutFile install.ps1; Set-ExecutionPolicy -ExecutionPolicy Bypass -Scope Process; .\install.ps1"
        ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac
```

## Release Binary Naming Convention

The scripts expect binaries to be named with this pattern:

* Linux/macOS: `ragflow_cli-{VERSION}-{OS}-{ARCH}`
* Windows: `ragflow_cli-{VERSION}-windows-{ARCH}.exe`

**Examples:**

* `ragflow_cli-v1.0.0-linux-amd64`
* `ragflow_cli-v1.0.0-linux-arm64`
* `ragflow_cli-v1.0.0-darwin-amd64`
* `ragflow_cli-v1.0.0-darwin-arm64`
* `ragflow_cli-v1.0.0-windows-amd64.exe`
* `ragflow_cli-v1.0.0-windows-arm64.exe`

## GitHub Actions Release Configuration

In `.github/workflows/release.yml`, build binaries for all supported platforms:

```yaml
- name: Build Go CLI release binaries
  run: |
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ragflow_cli-${RELEASE_TAG}-linux-amd64 ./cmd/ragflow_cli
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ragflow_cli-${RELEASE_TAG}-linux-arm64 ./cmd/ragflow_cli
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o ragflow_cli-${RELEASE_TAG}-darwin-amd64 ./cmd/ragflow_cli
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o ragflow_cli-${RELEASE_TAG}-darwin-arm64 ./cmd/ragflow_cli
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ragflow_cli-${RELEASE_TAG}-windows-amd64.exe ./cmd/ragflow_cli
    CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -o ragflow_cli-${RELEASE_TAG}-windows-arm64.exe ./cmd/ragflow_cli
```

## Supported Platforms

| OS      | Architecture | Support |
| ------- | ------------ | ------- |
| Linux   | amd64        | ✅       |
| Linux   | arm64        | ✅       |
| macOS   | amd64        | ✅       |
| macOS   | arm64        | ✅       |
| Windows | amd64        | ✅       |
| Windows | arm64        | ✅       |

## Troubleshooting

### Permission Denied

```bash
chmod +x install.sh
./install.sh
```

### Command Not Found After Installation

Make sure the installation directory is in your PATH:

```bash
export PATH="/usr/local/bin:$PATH"
```

If you installed to `$HOME/.local/bin`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Network Issues

If you have network issues, try with a timeout or retry:

```bash
curl -sSfL --max-time 60 https://your-domain/install.sh | sh
```

### Verify Downloaded Binary

```bash
# Check file checksum if SHA256SUMS is available
shasum -a 256 /usr/local/bin/ragflow_cli

# Test run
ragflow_cli --version
```

## Security Notes

1. **HTTPS Only**: Always serve install scripts over HTTPS
2. **Binary Verification**: Consider signing binaries and verifying signatures in the install script
3. **Checksums**: Provide SHA256 checksums for downloaded binaries
4. **Script Review**: Users should always review scripts before executing with elevated privileges

## License

Apache License 2.0 - See LICENSE file in the repository root.
