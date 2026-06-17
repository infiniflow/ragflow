# RAGFlow CLI Installation Scripts

RAGFlow publishes static Go CLI binaries as GitHub Release assets for:

- Linux: `amd64`, `arm64`
- macOS: `amd64`, `arm64`
- Windows: `amd64`, `arm64`

The release workflow builds the binaries with `CGO_ENABLED=0`, uploads `SHA256SUMS`, and uploads `install.sh` and `install.ps1`.

## Linux And macOS

Use the hosted script:

```sh
curl -sSfL https://your-domain/install.sh | sh
```

Install a specific version:

```sh
curl -sSfL https://your-domain/install.sh | VERSION=v1.0.0 sh
```

Install to a user-writable directory:

```sh
curl -sSfL https://your-domain/install.sh | INSTALL_DIR="$HOME/.local/bin" sh
```

The Unix installer:

- detects `linux` or `darwin`
- detects `amd64` or `arm64`
- downloads `ragflow_cli-{VERSION}-{OS}-{ARCH}`
- verifies the file with the release `SHA256SUMS`
- installs to `/usr/local/bin` by default

## Windows PowerShell

Use the hosted script:

```powershell
iwr https://your-domain/install.ps1 -OutFile install.ps1
powershell -ExecutionPolicy Bypass -File .\install.ps1
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
- downloads `ragflow_cli-{VERSION}-windows-{ARCH}.exe`
- verifies the file with the release `SHA256SUMS`
- installs to `$env:LOCALAPPDATA\Programs\RAGFlow` by default
- adds the install directory to the user `PATH`

## Release Asset Names

The install scripts expect these names:

```text
ragflow_cli-v1.0.0-linux-amd64
ragflow_cli-v1.0.0-linux-arm64
ragflow_cli-v1.0.0-darwin-amd64
ragflow_cli-v1.0.0-darwin-arm64
ragflow_cli-v1.0.0-windows-amd64.exe
ragflow_cli-v1.0.0-windows-arm64.exe
SHA256SUMS
install.sh
install.ps1
```

## Hosting Options

Use a static domain for the public one-line command:

```text
https://your-domain/install.sh
https://your-domain/install.ps1
```

The same scripts can also be used directly from a GitHub Release asset, for example:

```text
https://github.com/infiniflow/ragflow/releases/download/v1.0.0/install.sh
https://github.com/infiniflow/ragflow/releases/download/v1.0.0/install.ps1
```

By default, both scripts install the latest stable GitHub Release. Set `VERSION` on Unix or `-Version` on Windows to pin a specific release.
