# RAGFlow Go documentation facts

Use these facts as the project documentation baseline. If the user explicitly changes a target value, follow that request and update the affected documents consistently.

## Runtime and deployment

- API target port: `9380`.
- Admin target port: `9381`.
- Docker users normally access RAGFlow through Nginx on the published web ports, typically `80` and `443`.
- The Go server entrypoint is `bin/ragflow_server` with API, Admin, Ingestor, and Syncer modes.
- Go Docker deployment uses `docker/.env-go`, `docker/docker-compose-go.yml`, `Dockerfile_go`, and `docker/entrypoint-go.sh`.
- Open-source 1.0 DeepDoc performs layout analysis, OCR, and table recognition with CPU inference.

## Source build

- Use `bash build.sh --all` for the initial native and Go build when dependencies are not prepared.
- Use `bash build.sh --go` for a Go-only rebuild after the native dependencies are already prepared.
- The checked-in resource downloader is `ragflow_deps/download_go_deps.py`; it prepares native libraries and model assets needed by the Go build. This is a build-time preparation step.

## Go CLI

- Regular users should install the prebuilt Go CLI from a RAGFlow Release.
- Linux/macOS installer: `https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.sh`.
- Windows PowerShell installer: `https://raw.githubusercontent.com/infiniflow/ragflow/main/tools/scripts/install.ps1`.
- The Linux/macOS installer defaults to `/usr/local/bin/ragflow-cli`.
- The Windows installer defaults to `%LOCALAPPDATA%\\Programs\\RAGFlow` and adds it to the user PATH.
- Document only CLI commands confirmed by parser, dispatcher, implementation, and safe runtime checks.

## Protected scope

- Only Markdown and MDX files may be changed for documentation tasks.
- `internal/development.md` must not be modified.
