# RAGFlow CLI Installation Scripts

RAGFlow publishes static Go CLI binaries as GitHub Release assets for:

- Linux: `amd64`, `arm64`
- macOS: `amd64`, `arm64`
- Windows: `amd64`, `arm64`

The release workflow builds the binaries with `CGO_ENABLED=0`, uploads `SHA256SUMS`, and uploads `install.sh` and `install.ps1`.

## Linux And macOS

Use the script maintained in the RAGFlow repository:

```sh
curl -fsSL https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.sh | sh
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.sh | VERSION=v1.0.0 sh
```

Install to a user-writable directory:

```sh
curl -fsSL https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.sh | INSTALL_DIR="$HOME/.local/bin" sh
```

The Unix installer:

- detects `linux` or `darwin`
- detects `amd64` or `arm64`
- downloads `ragflow-cli-{VERSION}-{OS}-{ARCH}`
- verifies the file with the release `SHA256SUMS`
- installs to `/usr/local/bin` by default

## Windows PowerShell

Run the repository script in PowerShell:

```powershell
irm https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.ps1 | iex
```
Install a specific version:

```powershell
.\install.ps1 -Version "v1.0.0"
```

Install to a custom directory:

```powershell
.\install.ps1 -InstallDir "$env:USERPROFILE\bin"
```

The Windows installer:

- detects `windows/amd64` or `windows/arm64`
- downloads `ragflow-cli-{VERSION}-windows-{ARCH}.exe`
- verifies the file with the release `SHA256SUMS`
- installs to `$env:LOCALAPPDATA\Programs\RAGFlow` by default
- adds the install directory to the user `PATH`

## Release Asset Names

The install scripts expect these names:

```text
ragflow-cli-v1.0.0-linux-amd64
ragflow-cli-v1.0.0-linux-arm64
ragflow-cli-v1.0.0-darwin-amd64
ragflow-cli-v1.0.0-darwin-arm64
ragflow-cli-v1.0.0-windows-amd64.exe
ragflow-cli-v1.0.0-windows-arm64.exe
SHA256SUMS
install.sh
install.ps1
```

By default, both scripts install the latest stable GitHub Release. Set
`VERSION` on Unix or `-Version` on Windows to pin a specific release.
